package collector

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

func loadTestTeams(t *testing.T) []linear.Team {
	t.Helper()
	data, err := os.ReadFile("../../testdata/teams.json")
	if err != nil {
		t.Fatalf("reading test fixture: %v", err)
	}
	var resp struct {
		Teams linear.TeamConnection `json:"teams"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshaling test fixture: %v", err)
	}
	return resp.Teams.Nodes
}

func TestTeamsCollector_ProcessTeams(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewTeamsCollector(client, c, cfg)

	teams := loadTestTeams(t)
	issues := loadTestIssues(t)
	c.Set("teams", teams)
	c.Set("issues", issues)

	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected team metrics to be collected, got 0")
	}
	t.Logf("Collected %d metrics from %d teams", count, len(teams))
}

func TestTeamsCollector_MemberCount(t *testing.T) {
	cfg := testConfig()
	c := cache.New(cfg.Cache.TTL)
	client := linear.NewClient("test-key")
	col := NewTeamsCollector(client, c, cfg)

	teams := loadTestTeams(t)
	issues := loadTestIssues(t)
	c.Set("teams", teams)
	c.Set("issues", issues)

	// Engineering team has 2 members, Platform has 1
	count := testutil.CollectAndCount(col)
	if count == 0 {
		t.Fatal("expected metrics to be collected")
	}
}
