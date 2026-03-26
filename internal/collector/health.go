package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/config"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

var (
	// Set at build time via ldflags
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type HealthCollector struct {
	client *linear.Client
	cache  *cache.Cache
	cfg    *config.Config

	buildInfo            prometheus.Gauge
	scrapeDuration       *prometheus.HistogramVec
	scrapeErrorsTotal    *prometheus.CounterVec
	rateLimitRemaining   prometheus.Gauge
	apiRequestsTotal     *prometheus.CounterVec
	slaBreachedTotal     *prometheus.GaugeVec
	workflowStateIssues  *prometheus.GaugeVec

	// Backward-compat
	scrapeErrorsCompat   prometheus.Counter
	scrapeDurationCompat prometheus.Gauge
}

func NewHealthCollector(client *linear.Client, c *cache.Cache, cfg *config.Config) *HealthCollector {
	h := &HealthCollector{
		client: client,
		cache:  c,
		cfg:    cfg,

		buildInfo: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "linear_exporter_build_info",
			Help: "Build information for the linear exporter",
			ConstLabels: prometheus.Labels{
				"version": Version,
				"commit":  Commit,
				"date":    BuildDate,
			},
		}),

		scrapeDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "linear_api_scrape_duration_seconds",
			Help:    "Duration of API scrapes by domain",
			Buckets: prometheus.DefBuckets,
		}, []string{"domain"}),

		scrapeErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linear_api_scrape_errors_total",
			Help: "Total scrape errors by domain",
		}, []string{"domain"}),

		rateLimitRemaining: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "linear_api_rate_limit_remaining",
			Help: "Remaining API rate limit",
		}),

		apiRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linear_api_requests_total",
			Help: "Total API requests by operation",
		}, []string{"operation"}),

		slaBreachedTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_sla_breached_total",
			Help: "Issues breaching SLA thresholds",
		}, []string{"priority"}),

		workflowStateIssues: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_workflow_state_issues_total",
			Help: "Issues per workflow state by team",
		}, []string{"team", "state_name", "state_type"}),

		// Backward-compat
		scrapeErrorsCompat: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "linear_exporter_scrape_errors_total",
			Help: "Total number of scrape errors",
		}),

		scrapeDurationCompat: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "linear_exporter_scrape_duration_seconds",
			Help: "Duration of the last scrape in seconds",
		}),
	}

	h.buildInfo.Set(1)
	return h
}

// RecordScrape records the duration and any error for a domain scrape.
func (c *HealthCollector) RecordScrape(domain string, duration time.Duration, err error) {
	c.scrapeDuration.WithLabelValues(domain).Observe(duration.Seconds())
	c.scrapeDurationCompat.Set(duration.Seconds())
	if err != nil {
		c.scrapeErrorsTotal.WithLabelValues(domain).Inc()
		c.scrapeErrorsCompat.Inc()
	}
}

// UpdateRateLimit updates the rate limit gauge.
func (c *HealthCollector) UpdateRateLimit(remaining int) {
	c.rateLimitRemaining.Set(float64(remaining))
}

// RecordRequests records the number of API requests for an operation.
func (c *HealthCollector) RecordRequests(operation string, count int64) {
	c.apiRequestsTotal.WithLabelValues(operation).Add(float64(count))
}

func (c *HealthCollector) Describe(ch chan<- *prometheus.Desc) {
	c.buildInfo.Describe(ch)
	c.scrapeDuration.Describe(ch)
	c.scrapeErrorsTotal.Describe(ch)
	c.rateLimitRemaining.Describe(ch)
	c.apiRequestsTotal.Describe(ch)
	c.slaBreachedTotal.Describe(ch)
	c.workflowStateIssues.Describe(ch)
	c.scrapeErrorsCompat.Describe(ch)
	c.scrapeDurationCompat.Describe(ch)
}

func (c *HealthCollector) Collect(ch chan<- prometheus.Metric) {
	// Update rate limit from client
	c.UpdateRateLimit(c.client.RateLimitRemaining())

	// Compute SLA breaches and workflow states from cached issues
	c.slaBreachedTotal.Reset()
	c.workflowStateIssues.Reset()

	if cached, ok := c.cache.Get("issues"); ok {
		issues := cached.([]linear.Issue)
		c.computeSLAAndWorkflow(issues)
	}

	c.buildInfo.Collect(ch)
	c.scrapeDuration.Collect(ch)
	c.scrapeErrorsTotal.Collect(ch)
	c.rateLimitRemaining.Collect(ch)
	c.apiRequestsTotal.Collect(ch)
	c.slaBreachedTotal.Collect(ch)
	c.workflowStateIssues.Collect(ch)
	c.scrapeErrorsCompat.Collect(ch)
	c.scrapeDurationCompat.Collect(ch)
}

func (c *HealthCollector) computeSLAAndWorkflow(issues []linear.Issue) {
	now := time.Now()
	urgentSLA := 24 * time.Hour
	highSLA := 72 * time.Hour

	slaByPriority := map[string]int{}

	for _, issue := range issues {
		team := issue.Team.Name
		if team == "" {
			team = "unassigned"
		}

		// Workflow state tracking
		c.workflowStateIssues.WithLabelValues(team, issue.State.Name, issue.State.Type).Inc()

		isOpen := issue.State.Type != "completed" && issue.State.Type != "canceled"
		if !isOpen {
			continue
		}

		age := now.Sub(issue.CreatedAt)
		switch {
		case issue.Priority == 1 && age > urgentSLA:
			slaByPriority["urgent"]++
		case issue.Priority == 2 && age > highSLA:
			slaByPriority["high"]++
		}
	}

	for priority, count := range slaByPriority {
		c.slaBreachedTotal.WithLabelValues(priority).Set(float64(count))
	}

	// Log SLA status
	if total := slaByPriority["urgent"] + slaByPriority["high"]; total > 0 {
		slog.Warn("SLA breaches detected", "urgent", slaByPriority["urgent"], "high", slaByPriority["high"])
	}

}
