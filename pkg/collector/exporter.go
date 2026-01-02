// Package collector contains logic for the exporter
package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rebelopsio/linear-exporter/pkg/linear"
)

type Exporter struct {
	apiKey string
	mu     sync.Mutex
}

func NewExporter(apiKey string) *Exporter {
	return &Exporter{
		apiKey: apiKey,
	}
}

func (e *Exporter) graphQL(query string) (json.RawMessage, error) {
	body := map[string]string{
		"query": query,
	}
	payload, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "https://api.linear.app/graphql", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", e.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result linear.LinearResponse
	_ = json.Unmarshal(data, &result)
	return result.Data, nil
}

func (e *Exporter) scrapeIssues() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	start := time.Now()
	defer func() {
		scrapeDurationSeconds.Set(time.Since(start).Seconds())
	}()

	// Clear previous metrics
	issuesByPriority.Reset()
	issuesByStatus.Reset()
	issuesByTeam.Reset()
	issuesByCycle.Reset()
	issueAgeHours.Reset()
	issuesRemainingByCycle.Reset()

	// Fetch issues in batches with cursor-based pagination
	// Filter to open issues and issues in cycles for more relevant data
	allIssues := []linear.Issue{}
	cursor := ""
	pageSize := 50 // Smaller pages to avoid timeouts
	maxPages := 20 // Reasonable limit to prevent runaway queries

	for page := range maxPages {
		var afterCursor string
		if cursor != "" {
			afterCursor = fmt.Sprintf(`, after: "%s"`, cursor)
		}

		query := fmt.Sprintf(`
		{
			issues(first: %d%s) {
				edges {
					node {
						id
						identifier
						title
						priority
						state {
							name
						}
						team {
							name
						}
						cycle {
							name
							number
						}
						createdAt
						updatedAt
					}
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
		`, pageSize, afterCursor)

		data, err := e.graphQL(query)
		if err != nil {
			scrapeErrors.Inc()
			log.Printf("Error fetching issues page %d: %v", page, err)
			break // Continue with what we have
		}

		var issueData struct {
			Issues linear.IssueConnection `json:"issues"`
		}

		if err := json.Unmarshal(data, &issueData); err != nil {
			scrapeErrors.Inc()
			log.Printf("Error parsing issues page %d: %v", page, err)
			break
		}

		allIssues = append(allIssues, issueDataToIssues(issueData.Issues)...)

		if !issueData.Issues.PageInfo.HasNextPage {
			break
		}

		cursor = issueData.Issues.PageInfo.EndCursor
	}

	// Process all collected issues
	priorityNames := map[int]string{
		0: "none",
		1: "urgent",
		2: "high",
		3: "medium",
		4: "low",
	}

	now := time.Now()
	totalIssuesTracked.Set(float64(len(allIssues)))

	for _, issue := range allIssues {
		// Issues by priority and status
		priorityName := priorityNames[issue.Priority]
		if priorityName == "" {
			priorityName = "unknown"
		}
		issuesByPriority.WithLabelValues(priorityName, issue.State.Name).Inc()

		// Issues by status
		issuesByStatus.WithLabelValues(issue.State.Name).Inc()

		// Issues by team
		teamName := issue.Team.Name
		if teamName == "" {
			teamName = "unassigned"
		}
		issuesByTeam.WithLabelValues(teamName).Inc()

		// Issues by cycle (use number since name is often null in Linear API)
		cycleName := "backlog"
		if issue.Cycle != nil && issue.Cycle.Number != nil {
			cycleName = fmt.Sprintf("Cycle %d", *issue.Cycle.Number)
		}
		issuesByCycle.WithLabelValues(cycleName, priorityName).Inc()

		// Issues completed total
		if strings.Contains(strings.ToLower(issue.State.Name), "done") ||
			strings.Contains(strings.ToLower(issue.State.Name), "cancel") {
			issuesCompletedTotal.WithLabelValues(cycleName).Inc()
		}

		// Issues remaining by cycle
		if !strings.Contains(strings.ToLower(issue.State.Name), "done") &&
			!strings.Contains(strings.ToLower(issue.State.Name), "cancel") {
			issuesRemainingByCycle.WithLabelValues(cycleName).Inc()
		}

		// Issue age (only for non-resolved issues)
		if !strings.Contains(strings.ToLower(issue.State.Name), "done") &&
			!strings.Contains(strings.ToLower(issue.State.Name), "cancel") {
			ageHours := now.Sub(issue.CreatedAt).Hours()
			issueAgeHours.WithLabelValues(issue.State.Name, priorityName).Observe(ageHours)
		}
	}

	return nil
}

func issueDataToIssues(conn linear.IssueConnection) []linear.Issue {
	issues := make([]linear.Issue, len(conn.Edges))
	for i, edge := range conn.Edges {
		issues[i] = edge.Node
	}
	return issues
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	if err := e.scrapeIssues(); err != nil {
		log.Printf("Error scraping issues: %v", err)
	}

	issuesByPriority.Collect(ch)
	issuesByStatus.Collect(ch)
	issuesByTeam.Collect(ch)
	issuesByCycle.Collect(ch)
	issueAgeHours.Collect(ch)
	totalIssuesTracked.Collect(ch)
	scrapeErrors.Collect(ch)
	scrapeDurationSeconds.Collect(ch)
	issuesCompletedTotal.Collect(ch)
	issuesRemainingByCycle.Collect(ch)
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	issuesByPriority.Describe(ch)
	issuesByStatus.Describe(ch)
	issuesByTeam.Describe(ch)
	issuesByCycle.Describe(ch)
	issueAgeHours.Describe(ch)
	totalIssuesTracked.Describe(ch)
	scrapeErrors.Describe(ch)
	scrapeDurationSeconds.Describe(ch)
	issuesCompletedTotal.Describe(ch)
	issuesRemainingByCycle.Describe(ch)
}
