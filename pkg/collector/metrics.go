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
)
