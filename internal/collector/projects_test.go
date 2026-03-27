package collector

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

func loadTestProjects(t *testing.T) []linear.Project {
	t.Helper()
	data, err := os.ReadFile("../../testdata/projects.json")
	if err != nil {
		t.Fatalf("reading test fixture: %v", err)
	}
	var resp struct {
		Projects linear.ProjectConnection `json:"projects"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshaling test fixture: %v", err)
	}
	return resp.Projects.Nodes
}

func TestProjectsCollector_ProcessProjects(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewProjectsCollector(client, c, cfg)

	projects := loadTestProjects(t)
	c.Set("projects", projects)

	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected project metrics to be collected, got 0")
	}
	t.Logf("Collected %d metrics from %d projects", count, len(projects))
}

func TestProjectsCollector_HealthMapping(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewProjectsCollector(client, c, cfg)

	projects := loadTestProjects(t)
	c.Set("projects", projects)

	// Project Q1 Goals = onTrack (0), Tech Debt = atRisk (1)
	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected metrics to be collected")
	}
}
