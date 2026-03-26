package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rebelopsio/linear-exporter/internal/cache"
	"github.com/rebelopsio/linear-exporter/internal/collector"
	"github.com/rebelopsio/linear-exporter/internal/config"
	"github.com/rebelopsio/linear-exporter/internal/linear"
)

var ready atomic.Bool

func main() {
	// Structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting Linear Exporter",
		"version", collector.Version,
		"commit", collector.Commit,
	)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize shared dependencies
	client := linear.NewClient(cfg.Linear.APIKey)
	ttlCache := cache.New(cfg.Cache.TTL)

	// Create collectors
	issuesCollector := collector.NewIssuesCollector(client, ttlCache, cfg)
	cyclesCollector := collector.NewCyclesCollector(client, ttlCache, cfg)
	projectsCollector := collector.NewProjectsCollector(client, ttlCache, cfg)
	teamsCollector := collector.NewTeamsCollector(client, ttlCache, cfg)
	membersCollector := collector.NewMembersCollector(client, ttlCache, cfg)
	labelsCollector := collector.NewLabelsCollector(client, ttlCache, cfg)
	healthCollector := collector.NewHealthCollector(client, ttlCache, cfg)

	// Register all collectors
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		issuesCollector,
		cyclesCollector,
		projectsCollector,
		teamsCollector,
		membersCollector,
		labelsCollector,
		healthCollector,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready.Load() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "ready")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "not ready")
		}
	})
	// Backward-compat: /health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	addr := ":" + cfg.Server.Port
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Warm the cache with an initial scrape
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		slog.Info("Performing initial data fetch...")
		start := time.Now()

		issues, err := client.FetchAllIssues(ctx, cfg.Linear.TeamIDs)
		if err != nil {
			slog.Error("Initial issues fetch failed", "error", err)
		} else {
			ttlCache.Set("issues", issues)
			slog.Info("Initial issues fetch complete", "count", len(issues))
		}
		healthCollector.RecordScrape("issues", time.Since(start), err)
		healthCollector.RecordRequests("issues", client.RequestCount())

		ready.Store(true)
		slog.Info("Initial fetch complete, exporter ready", "duration", time.Since(start))
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("Listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-stop
	slog.Info("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Shutdown error", "error", err)
	}
	slog.Info("Exporter stopped")
}
