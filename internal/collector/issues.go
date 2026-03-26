package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/config"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

type IssuesCollector struct {
	client *linear.Client
	cache  *cache.Cache
	cfg    *config.Config

	// Gauges
	issuesTotal         *prometheus.GaugeVec
	issueEstimatePoints *prometheus.GaugeVec
	issuesOverdueTotal  *prometheus.GaugeVec
	issuesUnestimated   *prometheus.GaugeVec
	issuesBlockedTotal  *prometheus.GaugeVec
	urgentIssuesTotal   *prometheus.GaugeVec
	highPriorityOpen    *prometheus.GaugeVec

	// Backward-compat gauges (preserve existing metric names)
	issuesByPriority   *prometheus.GaugeVec
	issuesByStatus     *prometheus.GaugeVec
	issuesByTeam       *prometheus.GaugeVec
	issuesByCycle      *prometheus.GaugeVec
	totalIssuesTracked prometheus.Gauge
	issuesByProject    *prometheus.GaugeVec

	// Counters
	issuesCreatedTotal   *prometheus.CounterVec
	issuesCompletedTotal *prometheus.CounterVec
	issuesCancelledTotal *prometheus.CounterVec

	// Histograms
	issueAgeSeconds    *prometheus.HistogramVec
	issueCycleTime     *prometheus.HistogramVec
	issueLeadTime      *prometheus.HistogramVec
	issueTriageTime    *prometheus.HistogramVec
	issueFirstResponse *prometheus.HistogramVec

	// Backward-compat
	issueAgeHours *prometheus.HistogramVec
}

func NewIssuesCollector(client *linear.Client, c *cache.Cache, cfg *config.Config) *IssuesCollector {
	buckets := cfg.Metrics.HistogramBuckets
	return &IssuesCollector{
		client: client,
		cache:  c,
		cfg:    cfg,

		issuesTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_total",
			Help: "Total issues by state, priority, assignee, team, project, label, cycle",
		}, []string{"state", "priority", "assignee", "team", "project", "label", "cycle"}),

		issueEstimatePoints: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issue_estimate_points",
			Help: "Sum of issue estimates by team, project, assignee",
		}, []string{"team", "project", "assignee"}),

		issuesOverdueTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_overdue_total",
			Help: "Issues past due date that are not done",
		}, []string{"team", "priority"}),

		issuesUnestimated: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_unestimated_total",
			Help: "Issues without estimates by team, project",
		}, []string{"team", "project"}),

		issuesBlockedTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_blocked_total",
			Help: "Issues with blocking relations",
		}, []string{"team"}),

		urgentIssuesTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_urgent_issues_total",
			Help: "Open urgent (P0) issues",
		}, []string{"team"}),

		highPriorityOpen: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_high_priority_open_total",
			Help: "Open high priority (P1) issues",
		}, []string{"team"}),

		// Backward-compatible metrics
		issuesByPriority: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_by_priority",
			Help: "Number of issues by priority level and status",
		}, []string{"priority", "status"}),

		issuesByStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_by_status",
			Help: "Number of issues by status",
		}, []string{"status"}),

		issuesByTeam: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_by_team",
			Help: "Number of open issues by team",
		}, []string{"team"}),

		issuesByCycle: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_by_cycle",
			Help: "Number of issues by cycle",
		}, []string{"cycle", "priority"}),

		totalIssuesTracked: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "linear_total_issues_tracked",
			Help: "Total number of issues being tracked",
		}),

		issuesByProject: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_by_project",
			Help: "Number of issues by project and status",
		}, []string{"project", "status"}),

		// Counters
		issuesCreatedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linear_issues_created_total",
			Help: "Total issues created by team, project, label",
		}, []string{"team", "project", "label"}),

		issuesCompletedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linear_issues_completed_total",
			Help: "Total issues completed",
		}, []string{"team"}),

		issuesCancelledTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linear_issues_cancelled_total",
			Help: "Total issues cancelled",
		}, []string{"team"}),

		// Histograms
		issueAgeSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "linear_issue_age_seconds",
			Help:    "Age of open issues in seconds by state, priority, team",
			Buckets: buckets.IssueAge,
		}, []string{"state", "priority", "team"}),

		issueCycleTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "linear_issue_cycle_time_seconds",
			Help:    "Time from started to completed in seconds",
			Buckets: buckets.CycleTime,
		}, []string{"team", "priority"}),

		issueLeadTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "linear_issue_lead_time_seconds",
			Help:    "Time from created to completed in seconds",
			Buckets: buckets.LeadTime,
		}, []string{"team", "priority"}),

		issueTriageTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "linear_issue_triage_time_seconds",
			Help:    "Time from created to in_progress in seconds",
			Buckets: buckets.TriageTime,
		}, []string{"team", "priority"}),

		issueFirstResponse: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "linear_issue_first_response_time_seconds",
			Help:    "Time to first response by priority",
			Buckets: buckets.TriageTime,
		}, []string{"priority"}),

		// Backward-compat
		issueAgeHours: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "linear_issue_age_hours",
			Help:    "Age of issues in hours",
			Buckets: []float64{24, 72, 168, 336, 720, 1440},
		}, []string{"status", "priority"}),
	}
}

func (c *IssuesCollector) Describe(ch chan<- *prometheus.Desc) {
	c.issuesTotal.Describe(ch)
	c.issueEstimatePoints.Describe(ch)
	c.issuesOverdueTotal.Describe(ch)
	c.issuesUnestimated.Describe(ch)
	c.issuesBlockedTotal.Describe(ch)
	c.urgentIssuesTotal.Describe(ch)
	c.highPriorityOpen.Describe(ch)
	c.issuesByPriority.Describe(ch)
	c.issuesByStatus.Describe(ch)
	c.issuesByTeam.Describe(ch)
	c.issuesByCycle.Describe(ch)
	c.totalIssuesTracked.Describe(ch)
	c.issuesByProject.Describe(ch)
	c.issuesCreatedTotal.Describe(ch)
	c.issuesCompletedTotal.Describe(ch)
	c.issuesCancelledTotal.Describe(ch)
	c.issueAgeSeconds.Describe(ch)
	c.issueCycleTime.Describe(ch)
	c.issueLeadTime.Describe(ch)
	c.issueTriageTime.Describe(ch)
	c.issueFirstResponse.Describe(ch)
	c.issueAgeHours.Describe(ch)
}

func (c *IssuesCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Check cache
	if cached, ok := c.cache.Get("issues"); ok {
		issues := cached.([]linear.Issue)
		c.processIssues(issues, ch)
		return
	}

	issues, err := c.client.FetchAllIssues(ctx, c.cfg.Linear.TeamIDs)
	if err != nil {
		slog.Error("Failed to fetch issues", "error", err)
		return
	}

	c.cache.Set("issues", issues)
	c.processIssues(issues, ch)
}

func (c *IssuesCollector) processIssues(issues []linear.Issue, ch chan<- prometheus.Metric) {
	// Reset gauges
	c.issuesTotal.Reset()
	c.issueEstimatePoints.Reset()
	c.issuesOverdueTotal.Reset()
	c.issuesUnestimated.Reset()
	c.issuesBlockedTotal.Reset()
	c.urgentIssuesTotal.Reset()
	c.highPriorityOpen.Reset()
	c.issuesByPriority.Reset()
	c.issuesByStatus.Reset()
	c.issuesByTeam.Reset()
	c.issuesByCycle.Reset()
	c.issuesByProject.Reset()
	c.issueAgeHours.Reset()
	c.issueAgeSeconds.Reset()
	c.issueCycleTime.Reset()
	c.issueLeadTime.Reset()
	c.issueTriageTime.Reset()
	c.issueFirstResponse.Reset()

	now := time.Now()
	c.totalIssuesTracked.Set(float64(len(issues)))

	// SLA thresholds
	urgentSLA := 24 * time.Hour
	highSLA := 72 * time.Hour

	// Aggregation maps for estimate points
	type estimateKey struct {
		team, project, assignee string
	}
	estimates := map[estimateKey]float64{}

	// Track SLA breaches
	slaBreaches := 0

	for _, issue := range issues {
		priority := linear.PriorityName(issue.Priority)
		state := issue.State.Name
		stateType := issue.State.Type
		team := issue.Team.Name
		if team == "" {
			team = "unassigned"
		}

		assignee := "unassigned"
		if issue.Assignee != nil {
			assignee = issue.Assignee.Name
		}

		project := ""
		if issue.Project != nil {
			project = issue.Project.Name
		}

		label := ""
		if len(issue.Labels.Nodes) > 0 {
			label = issue.Labels.Nodes[0].Name
		}

		cycleName := "backlog"
		if issue.Cycle != nil {
			cycleName = fmt.Sprintf("Cycle %d", issue.Cycle.Number)
		}

		// New comprehensive metric
		c.issuesTotal.WithLabelValues(state, priority, assignee, team, project, label, cycleName).Inc()

		// Backward-compat metrics
		c.issuesByPriority.WithLabelValues(priority, state).Inc()
		c.issuesByStatus.WithLabelValues(state).Inc()
		c.issuesByTeam.WithLabelValues(team).Inc()
		c.issuesByCycle.WithLabelValues(cycleName, priority).Inc()
		if project != "" {
			c.issuesByProject.WithLabelValues(project, state).Inc()
		}

		isOpen := stateType != "completed" && stateType != "canceled"

		// Estimates
		if issue.Estimate != nil {
			key := estimateKey{team: team, project: project, assignee: assignee}
			estimates[key] += *issue.Estimate
		}

		// Unestimated (open issues only)
		if isOpen && issue.Estimate == nil {
			c.issuesUnestimated.WithLabelValues(team, project).Inc()
		}

		// Overdue
		if isOpen && issue.DueDate != nil {
			if dueDate, err := time.Parse("2006-01-02", *issue.DueDate); err == nil {
				if now.After(dueDate) {
					c.issuesOverdueTotal.WithLabelValues(team, priority).Inc()
				}
			}
		}

		// Blocked
		if isOpen {
			for _, rel := range issue.Relations.Nodes {
				if rel.Type == "is-blocked-by" {
					c.issuesBlockedTotal.WithLabelValues(team).Inc()
					break
				}
			}
		}

		// Urgent / High priority open
		if isOpen && issue.Priority == 1 {
			c.urgentIssuesTotal.WithLabelValues(team).Inc()
			// SLA: urgent open > 24h
			if now.Sub(issue.CreatedAt) > urgentSLA {
				slaBreaches++
			}
		}
		if isOpen && issue.Priority == 2 {
			c.highPriorityOpen.WithLabelValues(team).Inc()
			// SLA: high open > 72h
			if now.Sub(issue.CreatedAt) > highSLA {
				slaBreaches++
			}
		}

		// Issue age (open issues)
		if isOpen {
			age := now.Sub(issue.CreatedAt)
			c.issueAgeSeconds.WithLabelValues(state, priority, team).Observe(age.Seconds())
			c.issueAgeHours.WithLabelValues(state, priority).Observe(age.Hours())
		}

		// Cycle time (started → completed)
		if stateType == "completed" && issue.StartedAt != nil && issue.CompletedAt != nil {
			ct := issue.CompletedAt.Sub(*issue.StartedAt).Seconds()
			c.issueCycleTime.WithLabelValues(team, priority).Observe(ct)
		}

		// Lead time (created → completed)
		if stateType == "completed" && issue.CompletedAt != nil {
			lt := issue.CompletedAt.Sub(issue.CreatedAt).Seconds()
			c.issueLeadTime.WithLabelValues(team, priority).Observe(lt)
		}

		// Triage time (created → started)
		if issue.StartedAt != nil {
			tt := issue.StartedAt.Sub(issue.CreatedAt).Seconds()
			c.issueTriageTime.WithLabelValues(team, priority).Observe(tt)
			c.issueFirstResponse.WithLabelValues(priority).Observe(tt)
		}
	}

	// Emit estimate points
	for key, total := range estimates {
		c.issueEstimatePoints.WithLabelValues(key.team, key.project, key.assignee).Set(total)
	}

	// Collect all metrics into channel
	c.issuesTotal.Collect(ch)
	c.issueEstimatePoints.Collect(ch)
	c.issuesOverdueTotal.Collect(ch)
	c.issuesUnestimated.Collect(ch)
	c.issuesBlockedTotal.Collect(ch)
	c.urgentIssuesTotal.Collect(ch)
	c.highPriorityOpen.Collect(ch)
	c.issuesByPriority.Collect(ch)
	c.issuesByStatus.Collect(ch)
	c.issuesByTeam.Collect(ch)
	c.issuesByCycle.Collect(ch)
	c.totalIssuesTracked.Collect(ch)
	c.issuesByProject.Collect(ch)
	c.issuesCreatedTotal.Collect(ch)
	c.issuesCompletedTotal.Collect(ch)
	c.issuesCancelledTotal.Collect(ch)
	c.issueAgeSeconds.Collect(ch)
	c.issueCycleTime.Collect(ch)
	c.issueLeadTime.Collect(ch)
	c.issueTriageTime.Collect(ch)
	c.issueFirstResponse.Collect(ch)
	c.issueAgeHours.Collect(ch)

	_ = slaBreaches
}
