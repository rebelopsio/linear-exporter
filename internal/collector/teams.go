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

type TeamsCollector struct {
	client *linear.Client
	cache  *cache.Cache
	cfg    *config.Config

	teamMembersTotal   *prometheus.GaugeVec
	teamIssuesTotal    *prometheus.GaugeVec
	teamWIPIssues      *prometheus.GaugeVec
	teamWIPLimitRatio  *prometheus.GaugeVec
	teamThroughput     *prometheus.GaugeVec
}

func NewTeamsCollector(client *linear.Client, c *cache.Cache, cfg *config.Config) *TeamsCollector {
	return &TeamsCollector{
		client: client,
		cache:  c,
		cfg:    cfg,

		teamMembersTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_team_members_total",
			Help: "Number of members per team",
		}, []string{"team"}),

		teamIssuesTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_team_issues_total",
			Help: "Issues per team by state",
		}, []string{"team", "state"}),

		teamWIPIssues: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_team_wip_issues_total",
			Help: "In-progress issues per team",
		}, []string{"team"}),

		teamWIPLimitRatio: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_team_wip_limit_ratio",
			Help: "WIP to member ratio per team",
		}, []string{"team"}),

		teamThroughput: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_team_throughput_issues_per_week",
			Help: "Issues completed in rolling 7-day window per team",
		}, []string{"team"}),
	}
}

func (c *TeamsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.teamMembersTotal.Describe(ch)
	c.teamIssuesTotal.Describe(ch)
	c.teamWIPIssues.Describe(ch)
	c.teamWIPLimitRatio.Describe(ch)
	c.teamThroughput.Describe(ch)
}

func (c *TeamsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// We need both teams and issues for team-level metrics
	var teams []linear.Team
	var issues []linear.Issue

	if cached, ok := c.cache.Get("teams"); ok {
		teams = cached.([]linear.Team)
	} else {
		var err error
		teams, err = c.client.FetchAllTeams(ctx, c.cfg.Linear.TeamIDs)
		if err != nil {
			slog.Error("Failed to fetch teams", "error", err)
			return
		}
		c.cache.Set("teams", teams)
	}

	if cached, ok := c.cache.Get("issues"); ok {
		issues = cached.([]linear.Issue)
	} else {
		var err error
		issues, err = c.client.FetchAllIssues(ctx, c.cfg.Linear.TeamIDs)
		if err != nil {
			slog.Error("Failed to fetch issues for teams", "error", err)
			return
		}
		c.cache.Set("issues", issues)
	}

	c.processTeams(teams, issues, ch)
}

func (c *TeamsCollector) processTeams(teams []linear.Team, issues []linear.Issue, ch chan<- prometheus.Metric) {
	c.teamMembersTotal.Reset()
	c.teamIssuesTotal.Reset()
	c.teamWIPIssues.Reset()
	c.teamWIPLimitRatio.Reset()
	c.teamThroughput.Reset()

	now := time.Now()
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)

	// Index issues by team
	type teamStats struct {
		byState     map[string]int
		wip         int
		completedWeek int
		memberCount int
	}
	stats := map[string]*teamStats{}

	for _, t := range teams {
		stats[t.Name] = &teamStats{
			byState:     map[string]int{},
			memberCount: len(t.Members.Nodes),
		}
		c.teamMembersTotal.WithLabelValues(t.Name).Set(float64(len(t.Members.Nodes)))
	}

	for _, issue := range issues {
		team := issue.Team.Name
		if team == "" {
			continue
		}
		ts, ok := stats[team]
		if !ok {
			ts = &teamStats{byState: map[string]int{}}
			stats[team] = ts
		}

		ts.byState[issue.State.Name]++

		if issue.State.Type == "started" {
			ts.wip++
		}

		// Rolling 7-day throughput
		if issue.State.Type == "completed" && issue.CompletedAt != nil && issue.CompletedAt.After(sevenDaysAgo) {
			ts.completedWeek++
		}
	}

	for team, ts := range stats {
		for state, count := range ts.byState {
			c.teamIssuesTotal.WithLabelValues(team, state).Set(float64(count))
		}
		c.teamWIPIssues.WithLabelValues(team).Set(float64(ts.wip))
		c.teamThroughput.WithLabelValues(team).Set(float64(ts.completedWeek))

		if ts.memberCount > 0 {
			c.teamWIPLimitRatio.WithLabelValues(team).Set(float64(ts.wip) / float64(ts.memberCount))
		}
	}

	c.teamMembersTotal.Collect(ch)
	c.teamIssuesTotal.Collect(ch)
	c.teamWIPIssues.Collect(ch)
	c.teamWIPLimitRatio.Collect(ch)
	c.teamThroughput.Collect(ch)
}
