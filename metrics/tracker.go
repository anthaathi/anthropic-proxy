package metrics

import (
	"time"
)

// Tracker tracks TPS metrics for requests
type Tracker struct {
	cache *Cache
}

// NewTracker creates a new metrics tracker
func NewTracker() *Tracker {
	return &Tracker{
		cache: NewCache(),
	}
}

// GetCache returns the underlying cache
func (t *Tracker) GetCache() *Cache {
	return t.cache
}

// RecordRequest records metrics for a completed request
func (t *Tracker) RecordRequest(providerName, modelName string, tokens int, duration time.Duration) {
	durationS := duration.Seconds()
	t.cache.UpdateTPS(providerName, modelName, tokens, durationS)
}

// GetTPS returns the current TPS for a provider-model combination
func (t *Tracker) GetTPS(providerName, modelName string) float64 {
	return t.cache.GetTPS(providerName, modelName)
}

// MeetsThreshold checks if a provider-model meets the minimum TPS threshold
func (t *Tracker) MeetsThreshold(providerName, modelName string, threshold float64) bool {
	tps := t.GetTPS(providerName, modelName)

	// If no samples yet, assume it meets threshold (give it a chance)
	if tps == 0.0 {
		return true
	}

	return tps >= threshold
}

// GetAllMetrics returns all tracked metrics
func (t *Tracker) GetAllMetrics() map[string]*MetricData {
	return t.cache.GetAll()
}

// GetLatestSampleTime returns the timestamp of the most recent sample for a provider-model combination
func (t *Tracker) GetLatestSampleTime(providerName, modelName string) time.Time {
	return t.cache.GetLatestSampleTime(providerName, modelName)
}
