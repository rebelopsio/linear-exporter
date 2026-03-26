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

type MembersCollector struct {
	client *linear.Client
	cache  *cache.Cache
	cfg    *config.Config

	memberAssignedIssues  *prometheus.GaugeVec
	memberCompletedIssues *prometheus.GaugeVec
	memberCycleTime       *prometheus.HistogramVec
}

func NewMembersCollector(client *linear.Client, c *cache.Cache, cfg *config.Config) *MembersCollector {
	buckets := cfg.Metrics.HistogramBuckets
	return &MembersCollector{
		client: client,
		cache:  c,
		cfg:    cfg,

		memberAssignedIssues: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_member_assigned_issues_total",
			Help: "Assigned issues per member by state and priority",
		}, []string{"assignee", "state", "priority"}),

		memberCompletedIssues: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linear_member_completed_issues_total",
			Help: "Completed issues per member in rolling window",
		}, []string{"assignee", "window"}),

		memberCycleTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "linear_member_cycle_time_seconds",
			Help:    "Cycle time per assignee",
			Buckets: buckets.CycleTime,
		}, []string{"assignee"}),
	}
}

func (c *MembersCollector) Describe(ch chan<- *prometheus.Desc) {
	c.memberAssignedIssues.Describe(ch)
	c.memberCompletedIssues.Describe(ch)
	c.memberCycleTime.Describe(ch)
}

func (c *MembersCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if cached, ok := c.cache.Get("users"); ok {
		users := cached.([]linear.User)
		c.processMembers(users, ch)
		return
	}

	users, err := c.client.FetchAllUsers(ctx)
	if err != nil {
		slog.Error("Failed to fetch users", "error", err)
		return
	}

	c.cache.Set("users", users)
	c.processMembers(users, ch)
}

func (c *MembersCollector) processMembers(users []linear.User, ch chan<- prometheus.Metric) {
	c.memberAssignedIssues.Reset()
	c.memberCompletedIssues.Reset()
	c.memberCycleTime.Reset()

	now := time.Now()
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)
	thirtyDaysAgo := now.Add(-30 * 24 * time.Hour)

	for _, user := range users {
		if !user.Active {
			continue
		}

		var completed7d, completed30d int

		for _, issue := range user.AssignedIssues.Nodes {
			priority := linear.PriorityName(issue.Priority)
			state := issue.State.Name

			c.memberAssignedIssues.WithLabelValues(user.Name, state, priority).Inc()

			if issue.State.Type == "completed" && issue.CompletedAt != nil {
				if issue.CompletedAt.After(sevenDaysAgo) {
					completed7d++
				}
				if issue.CompletedAt.After(thirtyDaysAgo) {
					completed30d++
				}

				// Cycle time per member
				if issue.StartedAt != nil {
					ct := issue.CompletedAt.Sub(*issue.StartedAt).Seconds()
					c.memberCycleTime.WithLabelValues(user.Name).Observe(ct)
				}
			}
		}

		c.memberCompletedIssues.WithLabelValues(user.Name, "7d").Set(float64(completed7d))
		c.memberCompletedIssues.WithLabelValues(user.Name, "30d").Set(float64(completed30d))
	}

	c.memberAssignedIssues.Collect(ch)
	c.memberCompletedIssues.Collect(ch)
	c.memberCycleTime.Collect(ch)
}
