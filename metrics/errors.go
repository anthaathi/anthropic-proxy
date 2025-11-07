package metrics

import (
	"sync"
	"time"
)

// ErrorTracker tracks error rates per provider
type ErrorTracker struct {
	data map[string]*ErrorData
	mu   sync.RWMutex
}

// ErrorData holds error statistics for a provider
type ErrorData struct {
	ProviderName    string
	TotalRequests   int
	SuccessCount    int
	ErrorCount      int
	LastError       time.Time
	LastErrorStatus int
	ErrorRate       float64
}

// NewErrorTracker creates a new error tracker
func NewErrorTracker() *ErrorTracker {
	return &ErrorTracker{
		data: make(map[string]*ErrorData),
	}
}

// RecordSuccess records a successful request
func (e *ErrorTracker) RecordSuccess(providerName string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.data[providerName]; !exists {
		e.data[providerName] = &ErrorData{
			ProviderName: providerName,
		}
	}

	data := e.data[providerName]
	data.TotalRequests++
	data.SuccessCount++
	data.ErrorRate = calculateErrorRate(data.SuccessCount, data.ErrorCount)
}

// RecordError records a failed request
func (e *ErrorTracker) RecordError(providerName string, statusCode int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.data[providerName]; !exists {
		e.data[providerName] = &ErrorData{
			ProviderName: providerName,
		}
	}

	data := e.data[providerName]
	data.TotalRequests++
	data.ErrorCount++
	data.LastError = time.Now()
	data.LastErrorStatus = statusCode
	data.ErrorRate = calculateErrorRate(data.SuccessCount, data.ErrorCount)
}

// GetErrorRate returns the error rate for a provider
func (e *ErrorTracker) GetErrorRate(providerName string) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if data, exists := e.data[providerName]; exists {
		return data.ErrorRate
	}
	return 0.0
}

// GetData returns error data for a provider
func (e *ErrorTracker) GetData(providerName string) *ErrorData {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if data, exists := e.data[providerName]; exists {
		// Return a copy
		return &ErrorData{
			ProviderName:    data.ProviderName,
			TotalRequests:   data.TotalRequests,
			SuccessCount:    data.SuccessCount,
			ErrorCount:      data.ErrorCount,
			LastError:       data.LastError,
			LastErrorStatus: data.LastErrorStatus,
			ErrorRate:       data.ErrorRate,
		}
	}
	return nil
}

// GetAll returns all error data
func (e *ErrorTracker) GetAll() map[string]*ErrorData {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string]*ErrorData, len(e.data))
	for key, data := range e.data {
		result[key] = &ErrorData{
			ProviderName:    data.ProviderName,
			TotalRequests:   data.TotalRequests,
			SuccessCount:    data.SuccessCount,
			ErrorCount:      data.ErrorCount,
			LastError:       data.LastError,
			LastErrorStatus: data.LastErrorStatus,
			ErrorRate:       data.ErrorRate,
		}
	}
	return result
}

// calculateErrorRate computes error rate as a percentage
func calculateErrorRate(successCount, errorCount int) float64 {
	total := successCount + errorCount
	if total == 0 {
		return 0.0
	}
	return float64(errorCount) / float64(total)
}
