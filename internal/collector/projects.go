package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/config"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

type ProjectsCollector struct {
	client *linear.Client
	cache  *cache.Cache
	cfg    *config.Config

	projectIssuesTotal          *prometheus.GaugeVec
	projectCompletedIssuesTotal *prometheus.GaugeVec
	projectProgressPercent      *prometheus.GaugeVec
	projectMilestoneTotal       *prometheus.GaugeVec
	projectMilestonesCompleted  *prometheus.GaugeVec
	projectTargetDate           *prometheus.GaugeVec
	projectHealth               *prometheus.GaugeVec
	projectScopeCreepRatio      *prometheus.GaugeVec

	// Backward-compat
	projectProgress       *prometheus.GaugeVec
	projectInfo           *prometheus.GaugeVec
	projectIssueCount     *prometheus.GaugeVec
	projectCompletedCount *prometheus.GaugeVec
	projectOpenCount      *prometheus.GaugeVec
	totalProjects         prometheus.Gauge
}

func NewProjectsCollector(client *linear.Client, c *cache.Cache, cfg *config.Config) *ProjectsCollector {
	return &ProjectsCollector{
		client: client,
		cache:  c,
		cfg:    cfg,

		projectIssuesTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_issues_total",
			Help: "Issues in a project by state",
		}, []string{"project", "state"}),

		projectCompletedIssuesTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_completed_issues_total",
			Help: "Completed issues in a project",
		}, []string{"project"}),

		projectProgressPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_progress_percent",
			Help: "Project completion percentage",
		}, []string{"project"}),

		projectMilestoneTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_milestone_total",
			Help: "Total milestones in a project",
		}, []string{"project"}),

		projectMilestonesCompleted: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_milestones_completed_total",
			Help: "Completed milestones in a project",
		}, []string{"project"}),

		projectTargetDate: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_target_date_timestamp",
			Help: "Project target date as unix timestamp",
		}, []string{"project"}),

		projectHealth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_health",
			Help: "Project health status (0=onTrack, 1=atRisk, 2=offTrack)",
		}, []string{"project"}),

		projectScopeCreepRatio: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_scope_creep_ratio",
			Help: "Ratio of issues added after project start vs original scope",
		}, []string{"project"}),

		// Backward-compat
		projectProgress: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_progress",
			Help: "Project completion progress from Linear (0.0 to 1.0)",
		}, []string{"project"}),

		projectInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_info",
			Help: "Project metadata (value is always 1, use for label joins)",
		}, []string{"project", "status", "lead", "team", "priority", "start_date", "target_date"}),

		projectIssueCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_issue_count",
			Help: "Total number of issues in a project",
		}, []string{"project"}),

		projectCompletedCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_completed_count",
			Help: "Number of completed issues in a project",
		}, []string{"project"}),

		projectOpenCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_project_open_count",
			Help: "Number of open issues in a project",
		}, []string{"project"}),

		totalProjects: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "linear_projects_total",
			Help: "Total number of active Linear projects",
		}),
	}
}

func (c *ProjectsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.projectIssuesTotal.Describe(ch)
	c.projectCompletedIssuesTotal.Describe(ch)
	c.projectProgressPercent.Describe(ch)
	c.projectMilestoneTotal.Describe(ch)
	c.projectMilestonesCompleted.Describe(ch)
	c.projectTargetDate.Describe(ch)
	c.projectHealth.Describe(ch)
	c.projectScopeCreepRatio.Describe(ch)
	c.projectProgress.Describe(ch)
	c.projectInfo.Describe(ch)
	c.projectIssueCount.Describe(ch)
	c.projectCompletedCount.Describe(ch)
	c.projectOpenCount.Describe(ch)
	c.totalProjects.Describe(ch)
}

func (c *ProjectsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if cached, ok := c.cache.Get("projects"); ok {
		projects := cached.([]linear.Project)
		c.processProjects(projects, ch)
		return
	}

	projects, err := c.client.FetchAllProjects(ctx, c.cfg.Linear.ProjectIDs)
	if err != nil {
		slog.Error("Failed to fetch projects", "error", err)
		return
	}

	c.cache.Set("projects", projects)
	c.processProjects(projects, ch)
}

func (c *ProjectsCollector) processProjects(projects []linear.Project, ch chan<- prometheus.Metric) {
	c.projectIssuesTotal.Reset()
	c.projectCompletedIssuesTotal.Reset()
	c.projectProgressPercent.Reset()
	c.projectMilestoneTotal.Reset()
	c.projectMilestonesCompleted.Reset()
	c.projectTargetDate.Reset()
	c.projectHealth.Reset()
	c.projectScopeCreepRatio.Reset()
	c.projectProgress.Reset()
	c.projectInfo.Reset()
	c.projectIssueCount.Reset()
	c.projectCompletedCount.Reset()
	c.projectOpenCount.Reset()

	c.totalProjects.Set(float64(len(projects)))

	for _, p := range projects {
		name := p.Name

		// Progress
		c.projectProgressPercent.WithLabelValues(name).Set(p.Progress * 100)
		c.projectProgress.WithLabelValues(name).Set(p.Progress)

		// Health
		c.projectHealth.WithLabelValues(name).Set(linear.HealthToFloat(p.Health))

		// Milestones
		milestoneCount := len(p.Milestones.Nodes)
		c.projectMilestoneTotal.WithLabelValues(name).Set(float64(milestoneCount))
		// Linear doesn't have a direct "completed milestone" field, count from progress
		c.projectMilestonesCompleted.WithLabelValues(name).Set(0) // Would need deeper query

		// Target date
		if p.TargetDate != nil {
			if td, err := time.Parse("2006-01-02", *p.TargetDate); err == nil {
				c.projectTargetDate.WithLabelValues(name).Set(float64(td.Unix()))
			}
		}

		// Issue counts by state
		var completed, open, total int
		stateCounts := map[string]int{}
		for _, issue := range p.Issues.Nodes {
			total++
			stateCounts[issue.State.Name]++
			if issue.State.Type == "completed" {
				completed++
			} else if issue.State.Type != "canceled" {
				open++
			}
		}

		for state, count := range stateCounts {
			c.projectIssuesTotal.WithLabelValues(name, state).Set(float64(count))
		}
		c.projectCompletedIssuesTotal.WithLabelValues(name).Set(float64(completed))
		c.projectIssueCount.WithLabelValues(name).Set(float64(total))
		c.projectCompletedCount.WithLabelValues(name).Set(float64(completed))
		c.projectOpenCount.WithLabelValues(name).Set(float64(open))

		// Scope creep: issues created after project start
		if p.StartDate != nil {
			if startDate, err := time.Parse("2006-01-02", *p.StartDate); err == nil {
				addedAfter := 0
				for _, issue := range p.Issues.Nodes {
					if issue.CreatedAt.After(startDate) {
						addedAfter++
					}
				}
				if total > 0 {
					c.projectScopeCreepRatio.WithLabelValues(name).Set(float64(addedAfter) / float64(total))
				}
			}
		}

		// Backward-compat project info
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
			pri = linear.PriorityName(*p.Priority)
		}
		startDate := ""
		if p.StartDate != nil {
			startDate = *p.StartDate
		}
		targetDate := ""
		if p.TargetDate != nil {
			targetDate = *p.TargetDate
		}
		c.projectInfo.WithLabelValues(name, statusName, leadName, teamName, pri, startDate, targetDate).Set(1)
	}

	c.projectIssuesTotal.Collect(ch)
	c.projectCompletedIssuesTotal.Collect(ch)
	c.projectProgressPercent.Collect(ch)
	c.projectMilestoneTotal.Collect(ch)
	c.projectMilestonesCompleted.Collect(ch)
	c.projectTargetDate.Collect(ch)
	c.projectHealth.Collect(ch)
	c.projectScopeCreepRatio.Collect(ch)
	c.projectProgress.Collect(ch)
	c.projectInfo.Collect(ch)
	c.projectIssueCount.Collect(ch)
	c.projectCompletedCount.Collect(ch)
	c.projectOpenCount.Collect(ch)
	c.totalProjects.Collect(ch)
}
