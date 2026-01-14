// Package store provides persistence for scheduling state.
package store

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	cleanupRunsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "schedularr_cleanup_runs_total",
		Help: "Total number of cleanup runs executed",
	})

	cleanupEntriesRemoved = promauto.NewCounter(prometheus.CounterOpts{
		Name: "schedularr_cleanup_entries_removed_total",
		Help: "Total number of schedule history entries removed by cleanup",
	})

	cleanupDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "schedularr_cleanup_duration_seconds",
		Help:    "Duration of cleanup operations in seconds",
		Buckets: prometheus.DefBuckets,
	})

	cleanupErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "schedularr_cleanup_errors_total",
		Help: "Total number of cleanup errors",
	})
)

// Cleaner handles periodic cleanup of old schedule history entries.
type Cleaner struct {
	store     *Store
	interval  time.Duration
	retention time.Duration
	logger    *slog.Logger

	// Synchronization
	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
	doneChan chan struct{}
}

// CleanerOption is a function that configures a Cleaner.
type CleanerOption func(*Cleaner)

// WithLogger sets the logger for the Cleaner.
func WithLogger(logger *slog.Logger) CleanerOption {
	return func(c *Cleaner) {
		c.logger = logger
	}
}

// NewCleaner creates a new Cleaner that will periodically remove old schedule history entries.
// The interval specifies how often cleanup runs, and retention specifies how long to keep entries.
func NewCleaner(store *Store, interval, retention time.Duration, opts ...CleanerOption) *Cleaner {
	c := &Cleaner{
		store:     store,
		interval:  interval,
		retention: retention,
		logger:    slog.Default(),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Start begins the background cleanup goroutine.
// It returns immediately after starting the goroutine.
// Use Stop to gracefully shut down the cleaner.
func (c *Cleaner) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil // Already running
	}

	c.running = true
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	go c.run(ctx)

	c.logger.Info("schedule history cleaner started",
		"interval", c.interval.String(),
		"retention", c.retention.String())

	return nil
}

// Stop gracefully shuts down the cleanup goroutine and waits for it to finish.
func (c *Cleaner) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stopChan)
	c.mu.Unlock()

	// Wait for the goroutine to finish
	<-c.doneChan

	c.logger.Info("schedule history cleaner stopped")
}

// IsRunning returns true if the cleaner is currently running.
func (c *Cleaner) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// RunOnce performs a single cleanup operation. This is useful for testing
// or for triggering cleanup manually.
func (c *Cleaner) RunOnce(ctx context.Context) (int64, error) {
	return c.cleanup(ctx)
}

func (c *Cleaner) run(ctx context.Context) {
	defer close(c.doneChan)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Run cleanup immediately on start
	if _, err := c.cleanup(ctx); err != nil {
		c.logger.Error("initial cleanup failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			c.logger.Debug("cleaner context canceled")
			return
		case <-c.stopChan:
			c.logger.Debug("cleaner stop signal received")
			return
		case <-ticker.C:
			if _, err := c.cleanup(ctx); err != nil {
				c.logger.Error("cleanup failed", "error", err)
			}
		}
	}
}

func (c *Cleaner) cleanup(ctx context.Context) (int64, error) {
	start := time.Now()

	removed, err := c.store.CleanupScheduleHistory(ctx, c.retention)
	duration := time.Since(start).Seconds()

	cleanupRunsTotal.Inc()
	cleanupDuration.Observe(duration)

	if err != nil {
		cleanupErrors.Inc()
		c.logger.Error("cleanup operation failed",
			"duration_seconds", duration,
			"error", err)
		return 0, err
	}

	cleanupEntriesRemoved.Add(float64(removed))

	if removed > 0 {
		c.logger.Info("cleanup completed",
			"entries_removed", removed,
			"duration_seconds", duration,
			"retention", c.retention.String())
	} else {
		c.logger.Debug("cleanup completed, no entries removed",
			"duration_seconds", duration,
			"retention", c.retention.String())
	}

	return removed, nil
}
