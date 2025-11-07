package metrics

import (
	"sync"
	"time"
)

// Cache stores metrics data in memory
type Cache struct {
	data map[string]*MetricData
	mu   sync.RWMutex
}

// MetricData holds metrics for a provider-model combination
type MetricData struct {
	ProviderName string
	ModelName    string
	TPS          float64  // Current average tokens per second
	Samples      []Sample // Recent samples for averaging
	MaxSamples   int      // Maximum number of samples to keep
}

// Sample represents a single TPS measurement
type Sample struct {
	TPS       float64
	Tokens    int
	DurationS float64
	Timestamp time.Time
}

// NewCache creates a new metrics cache
func NewCache() *Cache {
	return &Cache{
		data: make(map[string]*MetricData),
	}
}

// GetTPS returns the TPS for a provider-model combination
func (c *Cache) GetTPS(providerName, modelName string) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := makeKey(providerName, modelName)
	if data, exists := c.data[key]; exists {
		return data.TPS
	}
	return 0.0
}

// UpdateTPS records a new TPS sample and recalculates the average
func (c *Cache) UpdateTPS(providerName, modelName string, tokens int, durationS float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := makeKey(providerName, modelName)

	// Calculate TPS for this sample
	tps := 0.0
	if durationS > 0 {
		tps = float64(tokens) / durationS
	}

	sample := Sample{
		TPS:       tps,
		Tokens:    tokens,
		DurationS: durationS,
		Timestamp: time.Now(),
	}

	// Get or create metric data
	if _, exists := c.data[key]; !exists {
		c.data[key] = &MetricData{
			ProviderName: providerName,
			ModelName:    modelName,
			Samples:      []Sample{},
			MaxSamples:   5, // Keep last 5 samples
		}
	}

	data := c.data[key]

	// Add sample
	data.Samples = append(data.Samples, sample)

	// Keep only the last N samples
	if len(data.Samples) > data.MaxSamples {
		data.Samples = data.Samples[len(data.Samples)-data.MaxSamples:]
	}

	// Recalculate average TPS
	data.TPS = calculateAverageTPS(data.Samples)
}

// GetAll returns all metric data
func (c *Cache) GetAll() map[string]*MetricData {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[string]*MetricData, len(c.data))
	for key, data := range c.data {
		result[key] = &MetricData{
			ProviderName: data.ProviderName,
			ModelName:    data.ModelName,
			TPS:          data.TPS,
			Samples:      append([]Sample{}, data.Samples...),
			MaxSamples:   data.MaxSamples,
		}
	}
	return result
}

// GetLatestSampleTime returns the timestamp of the most recent sample for a provider-model combination
func (c *Cache) GetLatestSampleTime(providerName, modelName string) time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := makeKey(providerName, modelName)
	if data, exists := c.data[key]; exists {
		if len(data.Samples) > 0 {
			// Return the timestamp of the most recent sample (last in array)
			return data.Samples[len(data.Samples)-1].Timestamp
		}
	}
	return time.Time{} // Return zero time if no samples exist
}

// makeKey creates a cache key from provider and model names
func makeKey(providerName, modelName string) string {
	return providerName + ":" + modelName
}

// calculateAverageTPS computes the average TPS from samples
func calculateAverageTPS(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, sample := range samples {
		sum += sample.TPS
	}
	return sum / float64(len(samples))
}
