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
	issuesByProject.Reset()
	projectProgress.Reset()
	projectInfo.Reset()
	projectIssueCount.Reset()
	projectCompletedCount.Reset()
	projectOpenCount.Reset()

	// Fetch projects for metadata and progress
	if err := e.scrapeProjects(); err != nil {
		log.Printf("Error scraping projects: %v", err)
	}

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
						project {
							id
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

	// Track per-project issue counts
	type projectCounts struct {
		total     int
		completed int
		open      int
		byStatus  map[string]int
	}
	projectStats := map[string]*projectCounts{}

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

		// Per-project tracking
		if issue.Project != nil && issue.Project.Name != "" {
			pName := issue.Project.Name
			if _, ok := projectStats[pName]; !ok {
				projectStats[pName] = &projectCounts{byStatus: map[string]int{}}
			}
			pc := projectStats[pName]
			pc.total++
			pc.byStatus[issue.State.Name]++

			isDone := strings.Contains(strings.ToLower(issue.State.Name), "done")
			isCanceled := strings.Contains(strings.ToLower(issue.State.Name), "cancel") ||
				strings.Contains(strings.ToLower(issue.State.Name), "duplicate")
			if isDone {
				pc.completed++
			}
			if !isDone && !isCanceled {
				pc.open++
			}
		}
	}

	// Emit per-project metrics
	for pName, pc := range projectStats {
		projectIssueCount.WithLabelValues(pName).Set(float64(pc.total))
		projectCompletedCount.WithLabelValues(pName).Set(float64(pc.completed))
		projectOpenCount.WithLabelValues(pName).Set(float64(pc.open))
		for status, count := range pc.byStatus {
			issuesByProject.WithLabelValues(pName, status).Set(float64(count))
		}
	}

	return nil
}

func (e *Exporter) scrapeProjects() error {
	allProjects := []linear.Project{}
	cursor := ""

	for page := 0; page < 5; page++ {
		var afterCursor string
		if cursor != "" {
			afterCursor = fmt.Sprintf(`, after: "%s"`, cursor)
		}

		query := fmt.Sprintf(`
		{
			projects(first: 50%s) {
				nodes {
					id
					name
					progress
					state
					priority
					status {
						name
					}
					lead {
						name
					}
					startDate
					targetDate
					teams {
						nodes {
							name
						}
					}
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
		`, afterCursor)

		data, err := e.graphQL(query)
		if err != nil {
			scrapeErrors.Inc()
			log.Printf("Error fetching projects page %d: %v", page, err)
			break
		}

		var projectData struct {
			Projects linear.ProjectConnection `json:"projects"`
		}
		if err := json.Unmarshal(data, &projectData); err != nil {
			scrapeErrors.Inc()
			log.Printf("Error parsing projects page %d: %v", page, err)
			break
		}

		allProjects = append(allProjects, projectData.Projects.Nodes...)

		if !projectData.Projects.PageInfo.HasNextPage {
			break
		}
		cursor = projectData.Projects.PageInfo.EndCursor
	}

	priorityNames := map[int]string{
		0: "none",
		1: "urgent",
		2: "high",
		3: "medium",
		4: "low",
	}

	totalProjects.Set(float64(len(allProjects)))
	log.Printf("Fetched %d projects from Linear", len(allProjects))

	for _, p := range allProjects {
		projectProgress.WithLabelValues(p.Name).Set(p.Progress)

		statusName := "unknown"
		if p.Status != nil {
			statusName = p.Status.Name
		}
		leadName := "unassigned"
		if p.Lead != nil && p.Lead.Name != "" {
			leadName = p.Lead.Name
		}
		teamName := ""
		if len(p.Teams.Nodes) > 0 {
			teamName = p.Teams.Nodes[0].Name
		}
		pri := "none"
		if p.Priority != nil {
			if name, ok := priorityNames[*p.Priority]; ok {
				pri = name
			}
		}
		startDate := ""
		if p.StartDate != nil {
			startDate = *p.StartDate
		}
		targetDate := ""
		if p.TargetDate != nil {
			targetDate = *p.TargetDate
		}

		projectInfo.WithLabelValues(p.Name, statusName, leadName, teamName, pri, startDate, targetDate).Set(1)
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
	issuesByProject.Collect(ch)
	projectProgress.Collect(ch)
	projectInfo.Collect(ch)
	projectIssueCount.Collect(ch)
	projectCompletedCount.Collect(ch)
	projectOpenCount.Collect(ch)
	totalProjects.Collect(ch)
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
	issuesByProject.Describe(ch)
	projectProgress.Describe(ch)
	projectInfo.Describe(ch)
	projectIssueCount.Describe(ch)
	projectCompletedCount.Describe(ch)
	projectOpenCount.Describe(ch)
	totalProjects.Describe(ch)
}
