package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	apiURL            = "https://api.linear.app/graphql"
	defaultPageSize   = 50
	nestedPageSize    = 5  // For queries with deeply nested connections (cycles, projects, users)
	nestedIssuesLimit = 50 // Max issues to fetch per nested connection
	maxRetries        = 3
)

type Client struct {
	apiKey     string
	httpClient *http.Client
	mu         sync.Mutex

	// Rate limit tracking
	rateLimitRemaining int
	rateLimitReset     time.Time

	// Request counting
	requestCount int64
	requestMu    sync.Mutex
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		rateLimitRemaining: 1500, // Linear default
	}
}

// RequestCount returns and resets the total API requests made since last call.
func (c *Client) RequestCount() int64 {
	c.requestMu.Lock()
	defer c.requestMu.Unlock()
	count := c.requestCount
	c.requestCount = 0
	return count
}

// RateLimitRemaining returns the last known rate limit remaining value.
func (c *Client) RateLimitRemaining() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rateLimitRemaining
}

func (c *Client) Query(ctx context.Context, query string, variables map[string]any, result any) error {
	return c.queryWithRetry(ctx, query, variables, result, 0)
}

func (c *Client) queryWithRetry(ctx context.Context, query string, variables map[string]any, result any, attempt int) error {
	// Check rate limit before making request
	c.mu.Lock()
	if c.rateLimitRemaining <= 5 && time.Now().Before(c.rateLimitReset) {
		wait := time.Until(c.rateLimitReset)
		c.mu.Unlock()
		slog.Warn("Rate limit nearly exhausted, waiting", "wait", wait)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		c.mu.Unlock()
	}

	body := map[string]any{
		"query": query,
	}
	if variables != nil {
		body["variables"] = variables
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if attempt < maxRetries {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			slog.Warn("Request failed, retrying", "attempt", attempt+1, "backoff", backoff, "error", err)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			return c.queryWithRetry(ctx, query, variables, result, attempt+1)
		}
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Track rate limits
	c.updateRateLimits(resp.Header)

	// Track request count
	c.requestMu.Lock()
	c.requestCount++
	c.requestMu.Unlock()

	if resp.StatusCode == http.StatusTooManyRequests {
		if attempt < maxRetries {
			backoff := time.Duration(math.Pow(2, float64(attempt+1))) * time.Second
			slog.Warn("Rate limited, backing off", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			return c.queryWithRetry(ctx, query, variables, result, attempt+1)
		}
		return fmt.Errorf("rate limited after %d retries", maxRetries)
	}

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var gqlResp GraphQLResponse
	if err := json.Unmarshal(data, &gqlResp); err != nil {
		return fmt.Errorf("unmarshaling response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}

	if result != nil {
		if err := json.Unmarshal(gqlResp.Data, result); err != nil {
			return fmt.Errorf("unmarshaling data: %w", err)
		}
	}

	return nil
}

func (c *Client) updateRateLimits(headers http.Header) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if remaining := headers.Get("X-RateLimit-Remaining"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			c.rateLimitRemaining = val
		}
	}
	if reset := headers.Get("X-RateLimit-Reset"); reset != "" {
		if val, err := strconv.ParseInt(reset, 10, 64); err == nil {
			c.rateLimitRemaining = int(val) // remaining from requests-remaining header
			c.rateLimitReset = time.Unix(val, 0)
		}
	}
}

// Paginate runs a paginated GraphQL query, calling handler for each page.
// The queryFn receives a cursor (empty string for first page) and returns the query string.
// The handler receives the raw JSON data and should return the next cursor and whether there are more pages.
func (c *Client) Paginate(ctx context.Context, queryFn func(cursor string) (string, map[string]any), handler func(json.RawMessage) (string, bool, error)) error {
	cursor := ""
	for page := 0; ; page++ {
		query, vars := queryFn(cursor)
		var raw json.RawMessage
		if err := c.Query(ctx, query, vars, &raw); err != nil {
			return fmt.Errorf("page %d: %w", page, err)
		}

		nextCursor, hasMore, err := handler(raw)
		if err != nil {
			return fmt.Errorf("handling page %d: %w", page, err)
		}

		if !hasMore {
			break
		}
		cursor = nextCursor
	}
	return nil
}

// FetchAllIssues fetches all issues with full pagination.
func (c *Client) FetchAllIssues(ctx context.Context, teamIDs []string) ([]Issue, error) {
	var allIssues []Issue

	filter := ""
	if len(teamIDs) > 0 {
		b, _ := json.Marshal(teamIDs)
		filter = fmt.Sprintf(`, filter: { team: { id: { in: %s } } }`, string(b))
	}

	err := c.Paginate(ctx,
		func(cursor string) (string, map[string]any) {
			after := ""
			if cursor != "" {
				after = fmt.Sprintf(`, after: "%s"`, cursor)
			}
			q := fmt.Sprintf(`{
				issues(first: %d%s%s) {
					nodes {
						id
						identifier
						title
						priority
						estimate
						dueDate
						startedAt
						completedAt
						canceledAt
						createdAt
						updatedAt
						state { id name type }
						team { id name key }
						assignee { id name }
						project { id name slugId }
						cycle { id number name }
						labels { nodes { id name } }
						relations { nodes { type } }
					}
					pageInfo { hasNextPage endCursor }
				}
			}`, defaultPageSize, after, filter)
			return q, nil
		},
		func(data json.RawMessage) (string, bool, error) {
			var resp struct {
				Issues IssueConnection `json:"issues"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", false, err
			}
			allIssues = append(allIssues, resp.Issues.Nodes...)
			return resp.Issues.PageInfo.EndCursor, resp.Issues.PageInfo.HasNextPage, nil
		},
	)
	return allIssues, err
}

// FetchAllCycles fetches all cycles for specified teams (or all teams).
func (c *Client) FetchAllCycles(ctx context.Context, teamIDs []string) ([]Cycle, error) {
	var allCycles []Cycle

	filter := ""
	if len(teamIDs) > 0 {
		b, _ := json.Marshal(teamIDs)
		filter = fmt.Sprintf(`, filter: { team: { id: { in: %s } } }`, string(b))
	}

	err := c.Paginate(ctx,
		func(cursor string) (string, map[string]any) {
			after := ""
			if cursor != "" {
				after = fmt.Sprintf(`, after: "%s"`, cursor)
			}
			q := fmt.Sprintf(`{
				cycles(first: %d%s%s) {
					nodes {
						id
						name
						number
						startsAt
						endsAt
						completedAt
						progress
						team { id name key }
						issues(first: %d) {
							nodes {
								id
								priority
								estimate
								state { id name type }
								assignee { id name }
								labels { nodes { id name } }
								createdAt
								completedAt
								startedAt
							}
						}
					}
					pageInfo { hasNextPage endCursor }
				}
			}`, nestedPageSize, after, filter, nestedIssuesLimit)
			return q, nil
		},
		func(data json.RawMessage) (string, bool, error) {
			var resp struct {
				Cycles CycleConnection `json:"cycles"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", false, err
			}
			allCycles = append(allCycles, resp.Cycles.Nodes...)
			return resp.Cycles.PageInfo.EndCursor, resp.Cycles.PageInfo.HasNextPage, nil
		},
	)
	return allCycles, err
}

// FetchAllProjects fetches all projects.
func (c *Client) FetchAllProjects(ctx context.Context, projectIDs []string) ([]Project, error) {
	var allProjects []Project

	filter := ""
	if len(projectIDs) > 0 {
		b, _ := json.Marshal(projectIDs)
		filter = fmt.Sprintf(`, filter: { id: { in: %s } }`, string(b))
	}

	err := c.Paginate(ctx,
		func(cursor string) (string, map[string]any) {
			after := ""
			if cursor != "" {
				after = fmt.Sprintf(`, after: "%s"`, cursor)
			}
			q := fmt.Sprintf(`{
				projects(first: %d%s%s) {
					nodes {
						id
						name
						slugId
						progress
						state
						health
						priority
						startDate
						targetDate
						status { name }
						lead { id name }
						teams { nodes { id name key } }
						projectMilestones { nodes { id name targetDate sortOrder } }
						issues(first: %d) {
							nodes {
								id
								state { id name type }
								completedAt
								createdAt
							}
						}
					}
					pageInfo { hasNextPage endCursor }
				}
			}`, nestedPageSize, after, filter, nestedIssuesLimit)
			return q, nil
		},
		func(data json.RawMessage) (string, bool, error) {
			var resp struct {
				Projects ProjectConnection `json:"projects"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", false, err
			}
			allProjects = append(allProjects, resp.Projects.Nodes...)
			return resp.Projects.PageInfo.EndCursor, resp.Projects.PageInfo.HasNextPage, nil
		},
	)
	return allProjects, err
}

// FetchAllTeams fetches all teams with their members and workflow states.
func (c *Client) FetchAllTeams(ctx context.Context, teamIDs []string) ([]Team, error) {
	var allTeams []Team

	filter := ""
	if len(teamIDs) > 0 {
		b, _ := json.Marshal(teamIDs)
		filter = fmt.Sprintf(`, filter: { id: { in: %s } }`, string(b))
	}

	err := c.Paginate(ctx,
		func(cursor string) (string, map[string]any) {
			after := ""
			if cursor != "" {
				after = fmt.Sprintf(`, after: "%s"`, cursor)
			}
			q := fmt.Sprintf(`{
				teams(first: %d%s%s) {
					nodes {
						id
						name
						key
						members { nodes { id name } }
						states { nodes { id name type } }
					}
					pageInfo { hasNextPage endCursor }
				}
			}`, defaultPageSize, after, filter)
			return q, nil
		},
		func(data json.RawMessage) (string, bool, error) {
			var resp struct {
				Teams TeamConnection `json:"teams"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", false, err
			}
			allTeams = append(allTeams, resp.Teams.Nodes...)
			return resp.Teams.PageInfo.EndCursor, resp.Teams.PageInfo.HasNextPage, nil
		},
	)
	return allTeams, err
}

// FetchAllUsers fetches all active users with their assigned issues.
func (c *Client) FetchAllUsers(ctx context.Context) ([]User, error) {
	var allUsers []User

	err := c.Paginate(ctx,
		func(cursor string) (string, map[string]any) {
			after := ""
			if cursor != "" {
				after = fmt.Sprintf(`, after: "%s"`, cursor)
			}
			q := fmt.Sprintf(`{
				users(first: %d%s, filter: { active: { eq: true } }) {
					nodes {
						id
						name
						active
						assignedIssues(first: %d) {
							nodes {
								id
								priority
								estimate
								state { id name type }
								team { id name key }
								project { id name slugId }
								createdAt
								completedAt
								startedAt
								cycle { id number }
							}
						}
					}
					pageInfo { hasNextPage endCursor }
				}
			}`, nestedPageSize, after, nestedIssuesLimit)
			return q, nil
		},
		func(data json.RawMessage) (string, bool, error) {
			var resp struct {
				Users UserConnection `json:"users"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", false, err
			}
			allUsers = append(allUsers, resp.Users.Nodes...)
			return resp.Users.PageInfo.EndCursor, resp.Users.PageInfo.HasNextPage, nil
		},
	)
	return allUsers, err
}

// FetchWorkflowStates fetches all workflow states across teams.
func (c *Client) FetchWorkflowStates(ctx context.Context, teamIDs []string) ([]WorkflowState, []TeamRef, error) {
	// We return states alongside their team references
	type stateWithTeam struct {
		State WorkflowState
		Team  TeamRef
	}
	var results []stateWithTeam

	teams, err := c.FetchAllTeams(ctx, teamIDs)
	if err != nil {
		return nil, nil, err
	}

	var states []WorkflowState
	var teamRefs []TeamRef
	for _, t := range teams {
		ref := TeamRef{ID: t.ID, Name: t.Name, Key: t.Key}
		for _, s := range t.States.Nodes {
			states = append(states, s)
			teamRefs = append(teamRefs, ref)
			_ = results // suppress unused
		}
	}

	return states, teamRefs, nil
}
