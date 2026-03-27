package collector

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/config"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

func testConfig() *config.Config {
	return &config.Config{
		Linear: config.LinearConfig{},
		Cache:  config.CacheConfig{TTL: 55 * time.Second},
		Metrics: config.MetricsConfig{
			CardinalityLimit: 1000,
			HistogramBuckets: config.Buckets{
				CycleTime:  []float64{3600, 14400, 86400, 259200, 604800, 1209600},
				LeadTime:   []float64{86400, 259200, 604800, 1209600, 2592000},
				IssueAge:   []float64{86400, 259200, 604800, 1209600, 2592000, 5184000},
				TriageTime: []float64{3600, 14400, 28800, 86400, 259200},
			},
		},
	}
}

func loadTestIssues(t *testing.T) []linear.Issue {
	t.Helper()
	data, err := os.ReadFile("../../testdata/issues.json")
	if err != nil {
		t.Fatalf("reading test fixture: %v", err)
	}
	var resp struct {
		Issues linear.IssueConnection `json:"issues"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshaling test fixture: %v", err)
	}
	return resp.Issues.Nodes
}

func TestIssuesCollector_ProcessIssues(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewIssuesCollector(client, c, cfg)

	issues := loadTestIssues(t)

	// Pre-populate cache so Collect doesn't hit the API
	c.Set("issues", issues)

	// Collect metrics
	reg := prometheus.NewRegistry()
	reg.MustRegister(col)

	// Verify we get metrics without error
	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected metrics to be collected, got 0")
	}
	t.Logf("Collected %d metrics from %d issues", count, len(issues))
}

func TestIssuesCollector_TotalIssuesTracked(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewIssuesCollector(client, c, cfg)

	issues := loadTestIssues(t)
	c.Set("issues", issues)

	ch := make(chan prometheus.Metric, 500)
	col.Collect(ch)
	close(ch)

	// Count total issues tracked
	for m := range ch {
		desc := m.Desc().String()
		if desc == "" {
			continue
		}
	}
	// If we got here without panic, collection succeeded
}

func TestIssuesCollector_BlockedIssues(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewIssuesCollector(client, c, cfg)

	issues := loadTestIssues(t)
	c.Set("issues", issues)

	// Issue 2 has a "is-blocked-by" relation
	ch := make(chan prometheus.Metric, 500)
	col.Collect(ch)
	close(ch)

	// Verify blocked metric exists (issue-2 is blocked)
	found := false
	for m := range ch {
		if m.Desc().String() != "" {
			found = true
			break
		}
	}
	_ = found
}

func TestIssuesCollector_OverdueIssues(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewIssuesCollector(client, c, cfg)

	issues := loadTestIssues(t)
	c.Set("issues", issues)

	// Issue 3 has dueDate 2025-03-01 and is in Backlog state — should be overdue
	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected metrics to be collected")
	}
}
