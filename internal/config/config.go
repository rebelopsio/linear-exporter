// Package config handles configuration loading from environment variables and YAML files.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Linear  LinearConfig  `mapstructure:"linear"`
	Scrape  ScrapeConfig  `mapstructure:"scrape"`
	Cache   CacheConfig   `mapstructure:"cache"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Server  ServerConfig  `mapstructure:"server"`
}

type LinearConfig struct {
	APIKey     string   `mapstructure:"api_key"`
	TeamIDs    []string `mapstructure:"team_ids"`
	ProjectIDs []string `mapstructure:"project_ids"`
}

type ScrapeConfig struct {
	IssuesInterval   time.Duration `mapstructure:"issues_interval"`
	CyclesInterval   time.Duration `mapstructure:"cycles_interval"`
	ProjectsInterval time.Duration `mapstructure:"projects_interval"`
	MembersInterval  time.Duration `mapstructure:"members_interval"`
}

type CacheConfig struct {
	TTL time.Duration `mapstructure:"ttl"`
}

type MetricsConfig struct {
	CardinalityLimit int     `mapstructure:"cardinality_limit"`
	HistogramBuckets Buckets `mapstructure:"histogram_buckets"`
}

type Buckets struct {
	CycleTime  []float64 `mapstructure:"cycle_time"`
	LeadTime   []float64 `mapstructure:"lead_time"`
	IssueAge   []float64 `mapstructure:"issue_age"`
	TriageTime []float64 `mapstructure:"triage_time"`
}

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/linear-exporter/")

	// Defaults
	v.SetDefault("server.port", "8080")
	v.SetDefault("scrape.issues_interval", "60s")
	v.SetDefault("scrape.cycles_interval", "120s")
	v.SetDefault("scrape.projects_interval", "300s")
	v.SetDefault("scrape.members_interval", "300s")
	v.SetDefault("cache.ttl", "55s")
	v.SetDefault("metrics.cardinality_limit", 1000)
	v.SetDefault("metrics.histogram_buckets.cycle_time", []float64{3600, 14400, 86400, 259200, 604800, 1209600})
	v.SetDefault("metrics.histogram_buckets.lead_time", []float64{86400, 259200, 604800, 1209600, 2592000})
	v.SetDefault("metrics.histogram_buckets.issue_age", []float64{86400, 259200, 604800, 1209600, 2592000, 5184000})
	v.SetDefault("metrics.histogram_buckets.triage_time", []float64{3600, 14400, 28800, 86400, 259200})

	// Environment variable mapping
	v.SetEnvPrefix("")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// LINEAR_API_KEY takes precedence
	if key := os.Getenv("LINEAR_API_KEY"); key != "" {
		v.Set("linear.api_key", key)
	}
	if port := os.Getenv("PORT"); port != "" {
		v.Set("server.port", port)
	}

	// Try to read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		slog.Info("No config file found, using defaults and environment variables")
	} else {
		slog.Info("Loaded config file", "path", v.ConfigFileUsed())
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if cfg.Linear.APIKey == "" {
		return nil, fmt.Errorf("LINEAR_API_KEY is required (set via environment or config file)")
	}

	return &cfg, nil
}
