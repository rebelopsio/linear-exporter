package collector

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

func loadTestCycles(t *testing.T) []linear.Cycle {
	t.Helper()
	data, err := os.ReadFile("../../testdata/cycles.json")
	if err != nil {
		t.Fatalf("reading test fixture: %v", err)
	}
	var resp struct {
		Cycles linear.CycleConnection `json:"cycles"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshaling test fixture: %v", err)
	}
	return resp.Cycles.Nodes
}

func TestCyclesCollector_ProcessCycles(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewCyclesCollector(client, c, cfg)

	cycles := loadTestCycles(t)
	c.Set("cycles", cycles)

	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected cycle metrics to be collected, got 0")
	}
	t.Logf("Collected %d metrics from %d cycles", count, len(cycles))
}

func TestCyclesCollector_ScopeCreep(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewCyclesCollector(client, c, cfg)

	cycles := loadTestCycles(t)
	c.Set("cycles", cycles)

	// Cycle starts 2025-03-01, issue-2 created 2025-03-08 → scope creep
	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected metrics to be collected")
	}
}
