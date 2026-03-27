package collector

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

func TestHealthCollector_BuildInfo(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewHealthCollector(client, c, cfg)

	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected health metrics to be collected, got 0")
	}
	t.Logf("Collected %d health metrics", count)
}

func TestHealthCollector_RecordScrape(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewHealthCollector(client, c, cfg)

	col.RecordScrape("issues", 2*time.Second, nil)

	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected metrics after recording scrape")
	}
}

func TestHealthCollector_SLABreaches(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewHealthCollector(client, c, cfg)

	issues := loadTestIssues(t)
	c.Set("issues", issues)

	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected metrics with SLA breach data")
	}
}
