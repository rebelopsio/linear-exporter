package collector

import "github.com/prometheus/client_golang/prometheus"

// Prometheus metrics
var (
	issuesByPriority = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_issues_by_priority",
			Help: "Number of issues by priority level (0=no priority, 1=urgent, 2=high, 3=medium, 4=low)",
		},
		[]string{"priority", "status"},
	)

	issuesByStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_issues_by_status",
			Help: "Number of issues by status",
		},
		[]string{"status"},
	)

	issuesByTeam = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_issues_by_team",
			Help: "Number of open issues by team",
		},
		[]string{"team"},
	)

	issuesByCycle = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_issues_by_cycle",
			Help: "Number of issues by cycle",
		},
		[]string{"cycle", "priority"},
	)

	issueAgeHours = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "linear_issue_age_hours",
			Help:    "Age of issues in hours",
			Buckets: []float64{24, 72, 168, 336, 720, 1440}, // 1d, 3d, 1w, 2w, 1m, 2m
		},
		[]string{"status", "priority"},
	)

	totalIssuesTracked = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "linear_total_issues_tracked",
			Help: "Total number of issues being tracked across all states and priorities",
		},
	)

	scrapeErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "linear_exporter_scrape_errors_total",
			Help: "Total number of scrape errors",
		},
	)

	scrapeDurationSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "linear_exporter_scrape_duration_seconds",
			Help: "Duration of the last scrape in seconds",
		},
	)

	issuesCompletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "linear_issues_completed_total",
			Help: "Total number of completed issues by cycle",
		},
		[]string{"cycle"},
	)

	issuesRemainingByCycle = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_issues_remaining",
			Help: "Issues not yet completed by cycle",
		},
		[]string{"cycle"},
	)

	issuesByProject = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_issues_by_project",
			Help: "Number of issues by project and status",
		},
		[]string{"project", "status"},
	)

	projectProgress = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_project_progress",
			Help: "Project completion progress from Linear (0.0 to 1.0)",
		},
		[]string{"project"},
	)

	projectInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_project_info",
			Help: "Project metadata (value is always 1, use for label joins)",
		},
		[]string{"project", "status", "lead", "team", "priority", "start_date", "target_date"},
	)

	projectIssueCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_project_issue_count",
			Help: "Total number of issues in a project",
		},
		[]string{"project"},
	)

	projectCompletedCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_project_completed_count",
			Help: "Number of completed (Done) issues in a project",
		},
		[]string{"project"},
	)

	projectOpenCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "linear_project_open_count",
			Help: "Number of open (non-Done, non-Canceled) issues in a project",
		},
		[]string{"project"},
	)

	totalProjects = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "linear_projects_total",
			Help: "Total number of active Linear projects",
		},
	)
)
