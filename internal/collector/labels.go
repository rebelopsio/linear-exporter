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

type LabelsCollector struct {
	client *linear.Client
	cache  *cache.Cache
	cfg    *config.Config

	labelIssuesTotal *prometheus.GaugeVec
}

func NewLabelsCollector(client *linear.Client, c *cache.Cache, cfg *config.Config) *LabelsCollector {
	return &LabelsCollector{
		client: client,
		cache:  c,
		cfg:    cfg,

		labelIssuesTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_label_issues_total",
			Help: "Issues per label by state",
		}, []string{"label", "state"}),
	}
}

func (c *LabelsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.labelIssuesTotal.Describe(ch)
}

func (c *LabelsCollector) Collect(ch chan<- prometheus.Metric) {
	// Labels are derived from issues data to avoid extra API calls
	if cached, ok := c.cache.Get("issues"); ok {
		issues := cached.([]linear.Issue)
		c.processLabels(issues, ch)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	issues, err := c.client.FetchAllIssues(ctx, c.cfg.Linear.TeamIDs)
	if err != nil {
		slog.Error("Failed to fetch issues for labels", "error", err)
		return
	}

	c.cache.Set("issues", issues)
	c.processLabels(issues, ch)
}

func (c *LabelsCollector) processLabels(issues []linear.Issue, ch chan<- prometheus.Metric) {
	c.labelIssuesTotal.Reset()

	// Count label-state combinations with cardinality guard
	type key struct{ label, state string }
	counts := map[key]int{}

	for _, issue := range issues {
		for _, label := range issue.Labels.Nodes {
			k := key{label: label.Name, state: issue.State.Name}
			counts[k]++
		}
	}

	limit := c.cfg.Metrics.CardinalityLimit
	if len(counts) > limit {
		slog.Warn("Label cardinality exceeds limit", "count", len(counts), "limit", limit)
	}

	for k, count := range counts {
		c.labelIssuesTotal.WithLabelValues(k.label, k.state).Set(float64(count))
	}

	c.labelIssuesTotal.Collect(ch)
}
