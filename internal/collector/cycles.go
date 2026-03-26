package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/config"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

type CyclesCollector struct {
	client *linear.Client
	cache  *cache.Cache
	cfg    *config.Config

	cycleIssuesTotal     *prometheus.GaugeVec
	cycleCompletedIssues *prometheus.GaugeVec
	cycleScopePoints     *prometheus.GaugeVec
	cycleCompletedPoints *prometheus.GaugeVec
	cycleAddedIssues     *prometheus.GaugeVec
	cycleRemovedIssues   *prometheus.GaugeVec
	cycleProgressPercent *prometheus.GaugeVec
	cycleStartTimestamp  *prometheus.GaugeVec
	cycleEndTimestamp    *prometheus.GaugeVec
	cycleVelocityPoints  *prometheus.GaugeVec

	// Backward-compat
	issuesCompletedByCycle *prometheus.GaugeVec
	issuesRemainingByCycle *prometheus.GaugeVec
}

func NewCyclesCollector(client *linear.Client, c *cache.Cache, cfg *config.Config) *CyclesCollector {
	return &CyclesCollector{
		client: client,
		cache:  c,
		cfg:    cfg,

		cycleIssuesTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_issues_total",
			Help: "Total issues in a cycle by team and state",
		}, []string{"cycle", "team", "state"}),

		cycleCompletedIssues: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_completed_issues_total",
			Help: "Completed issues in a cycle",
		}, []string{"cycle", "team"}),

		cycleScopePoints: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_scope_points",
			Help: "Total estimate points in a cycle",
		}, []string{"cycle", "team"}),

		cycleCompletedPoints: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_completed_points",
			Help: "Completed estimate points in a cycle",
		}, []string{"cycle", "team"}),

		cycleAddedIssues: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_added_issues_total",
			Help: "Issues added after cycle start (scope creep)",
		}, []string{"cycle", "team"}),

		cycleRemovedIssues: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_removed_issues_total",
			Help: "Issues removed from cycle",
		}, []string{"cycle", "team"}),

		cycleProgressPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_progress_percent",
			Help: "Cycle completion percentage",
		}, []string{"cycle", "team"}),

		cycleStartTimestamp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_start_timestamp",
			Help: "Cycle start time as unix timestamp",
		}, []string{"cycle", "team"}),

		cycleEndTimestamp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_end_timestamp",
			Help: "Cycle end time as unix timestamp",
		}, []string{"cycle", "team"}),

		cycleVelocityPoints: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_velocity_points",
			Help: "Completed points per cycle (velocity tracking)",
		}, []string{"cycle", "team"}),

		// Backward-compat
		issuesCompletedByCycle: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_cycle_issues_completed",
			Help: "Completed issues per cycle (compat)",
		}, []string{"cycle"}),

		issuesRemainingByCycle: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_issues_remaining",
			Help: "Issues not yet completed by cycle",
		}, []string{"cycle"}),
	}
}

func (c *CyclesCollector) Describe(ch chan<- *prometheus.Desc) {
	c.cycleIssuesTotal.Describe(ch)
	c.cycleCompletedIssues.Describe(ch)
	c.cycleScopePoints.Describe(ch)
	c.cycleCompletedPoints.Describe(ch)
	c.cycleAddedIssues.Describe(ch)
	c.cycleRemovedIssues.Describe(ch)
	c.cycleProgressPercent.Describe(ch)
	c.cycleStartTimestamp.Describe(ch)
	c.cycleEndTimestamp.Describe(ch)
	c.cycleVelocityPoints.Describe(ch)
	c.issuesCompletedByCycle.Describe(ch)
	c.issuesRemainingByCycle.Describe(ch)
}

func (c *CyclesCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if cached, ok := c.cache.Get("cycles"); ok {
		cycles := cached.([]linear.Cycle)
		c.processCycles(cycles, ch)
		return
	}

	cycles, err := c.client.FetchAllCycles(ctx, c.cfg.Linear.TeamIDs)
	if err != nil {
		slog.Error("Failed to fetch cycles", "error", err)
		return
	}

	c.cache.Set("cycles", cycles)
	c.processCycles(cycles, ch)
}

func (c *CyclesCollector) processCycles(cycles []linear.Cycle, ch chan<- prometheus.Metric) {
	c.cycleIssuesTotal.Reset()
	c.cycleCompletedIssues.Reset()
	c.cycleScopePoints.Reset()
	c.cycleCompletedPoints.Reset()
	c.cycleAddedIssues.Reset()
	c.cycleRemovedIssues.Reset()
	c.cycleProgressPercent.Reset()
	c.cycleStartTimestamp.Reset()
	c.cycleEndTimestamp.Reset()
	c.cycleVelocityPoints.Reset()
	c.issuesCompletedByCycle.Reset()
	c.issuesRemainingByCycle.Reset()

	for _, cycle := range cycles {
		cycleName := fmt.Sprintf("Cycle %d", cycle.Number)
		team := cycle.Team.Name

		c.cycleStartTimestamp.WithLabelValues(cycleName, team).Set(float64(cycle.StartsAt.Unix()))
		c.cycleEndTimestamp.WithLabelValues(cycleName, team).Set(float64(cycle.EndsAt.Unix()))
		c.cycleProgressPercent.WithLabelValues(cycleName, team).Set(cycle.Progress * 100)

		var totalPoints, completedPoints float64
		var completedCount, remainingCount, addedAfterStart int

		// Count by state
		stateCounts := map[string]int{}
		for _, issue := range cycle.Issues.Nodes {
			stateCounts[issue.State.Name]++

			est := 0.0
			if issue.Estimate != nil {
				est = *issue.Estimate
			}
			totalPoints += est

			if issue.State.Type == "completed" {
				completedCount++
				completedPoints += est
			} else if issue.State.Type != "canceled" {
				remainingCount++
			}

			// Scope creep: issue created after cycle started
			if issue.CreatedAt.After(cycle.StartsAt) {
				addedAfterStart++
			}
		}

		for state, count := range stateCounts {
			c.cycleIssuesTotal.WithLabelValues(cycleName, team, state).Set(float64(count))
		}

		c.cycleCompletedIssues.WithLabelValues(cycleName, team).Set(float64(completedCount))
		c.cycleScopePoints.WithLabelValues(cycleName, team).Set(totalPoints)
		c.cycleCompletedPoints.WithLabelValues(cycleName, team).Set(completedPoints)
		c.cycleAddedIssues.WithLabelValues(cycleName, team).Set(float64(addedAfterStart))
		c.cycleVelocityPoints.WithLabelValues(cycleName, team).Set(completedPoints)

		// Backward-compat
		c.issuesCompletedByCycle.WithLabelValues(cycleName).Set(float64(completedCount))
		c.issuesRemainingByCycle.WithLabelValues(cycleName).Set(float64(remainingCount))
	}

	c.cycleIssuesTotal.Collect(ch)
	c.cycleCompletedIssues.Collect(ch)
	c.cycleScopePoints.Collect(ch)
	c.cycleCompletedPoints.Collect(ch)
	c.cycleAddedIssues.Collect(ch)
	c.cycleRemovedIssues.Collect(ch)
	c.cycleProgressPercent.Collect(ch)
	c.cycleStartTimestamp.Collect(ch)
	c.cycleEndTimestamp.Collect(ch)
	c.cycleVelocityPoints.Collect(ch)
	c.issuesCompletedByCycle.Collect(ch)
	c.issuesRemainingByCycle.Collect(ch)
}
