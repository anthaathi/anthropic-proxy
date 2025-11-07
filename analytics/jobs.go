package analytics

import (
	"anthropic-proxy/logger"
	"time"
)

// CleanupJob handles periodic cleanup and aggregation
type CleanupJob struct {
	service  *Service
	interval time.Duration
	stopChan chan struct{}
}

// NewCleanupJob creates a new cleanup job
func NewCleanupJob(service *Service, interval time.Duration) *CleanupJob {
	if interval <= 0 {
		interval = 24 * time.Hour // Default to daily
	}

	return &CleanupJob{
		service:  service,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start begins the cleanup job
func (j *CleanupJob) Start() {
	logger.Info("Starting analytics cleanup job", "interval", j.interval)

	go func() {
		ticker := time.NewTicker(j.interval)
		defer ticker.Stop()

		// Run immediately on startup
		j.runCleanup()

		for {
			select {
			case <-ticker.C:
				j.runCleanup()
			case <-j.stopChan:
				logger.Info("Stopping analytics cleanup job")
				return
			}
		}
	}()
}

// Stop stops the cleanup job
func (j *CleanupJob) Stop() {
	close(j.stopChan)
}

// runCleanup performs the cleanup and aggregation
func (j *CleanupJob) runCleanup() {
	logger.Info("Running analytics cleanup...")

	if err := j.service.AggregateOldLogs(); err != nil {
		logger.Error("Failed to aggregate old logs", "error", err.Error())
	} else {
		logger.Info("Analytics cleanup completed successfully")
	}
}
