package provider

import (
	"sync"
	"time"
)

// HealthTracker tracks provider health metrics
type HealthTracker struct {
	stats map[string]*HealthStats
	mu    sync.RWMutex
}

// HealthStats stores health information for a provider
type HealthStats struct {
	SuccessCount int
	FailureCount int
	LastSuccess  time.Time
	LastFailure  time.Time
}

// NewHealthTracker creates a new health tracker
func NewHealthTracker() *HealthTracker {
	return &HealthTracker{
		stats: make(map[string]*HealthStats),
	}
}

// RecordSuccess records a successful request
func (h *HealthTracker) RecordSuccess(providerName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.stats[providerName]; !exists {
		h.stats[providerName] = &HealthStats{}
	}

	h.stats[providerName].SuccessCount++
	h.stats[providerName].LastSuccess = time.Now()
}

// RecordFailure records a failed request
func (h *HealthTracker) RecordFailure(providerName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.stats[providerName]; !exists {
		h.stats[providerName] = &HealthStats{}
	}

	h.stats[providerName].FailureCount++
	h.stats[providerName].LastFailure = time.Now()
}

// GetStats returns health stats for a provider
func (h *HealthTracker) GetStats(providerName string) *HealthStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats, exists := h.stats[providerName]
	if !exists {
		return &HealthStats{}
	}

	// Return a copy to avoid race conditions
	return &HealthStats{
		SuccessCount: stats.SuccessCount,
		FailureCount: stats.FailureCount,
		LastSuccess:  stats.LastSuccess,
		LastFailure:  stats.LastFailure,
	}
}

// GetErrorRate returns the error rate for a provider (0.0 to 1.0)
func (h *HealthTracker) GetErrorRate(providerName string) float64 {
	stats := h.GetStats(providerName)
	total := stats.SuccessCount + stats.FailureCount
	if total == 0 {
		return 0.0
	}
	return float64(stats.FailureCount) / float64(total)
}

// IsHealthy checks if a provider is considered healthy
func (h *HealthTracker) IsHealthy(providerName string) bool {
	errorRate := h.GetErrorRate(providerName)
	// Consider unhealthy if error rate is above 50%
	return errorRate < 0.5
}
