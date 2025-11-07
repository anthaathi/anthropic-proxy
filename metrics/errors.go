package metrics

import (
    "sync"
    "time"
)

// ErrorTracker tracks error rates per provider/model combination
type ErrorTracker struct {
    providerData map[string]*ErrorData
    modelData    map[string]*ErrorData
    mu           sync.RWMutex
}

// ErrorData holds error statistics for a provider or provider/model pair
type ErrorData struct {
    ProviderName    string
    ModelName       string
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
        providerData: make(map[string]*ErrorData),
        modelData:    make(map[string]*ErrorData),
    }
}

// RecordSuccess records a successful request
func (e *ErrorTracker) RecordSuccess(providerName, modelName string) {
    e.mu.Lock()
    defer e.mu.Unlock()

    providerEntry := e.ensureProviderEntry(providerName)
    providerEntry.TotalRequests++
    providerEntry.SuccessCount++
    providerEntry.ErrorRate = calculateErrorRate(providerEntry.SuccessCount, providerEntry.ErrorCount)

    if modelName != "" {
        modelEntry := e.ensureModelEntry(providerName, modelName)
        modelEntry.TotalRequests++
        modelEntry.SuccessCount++
        modelEntry.ErrorRate = calculateErrorRate(modelEntry.SuccessCount, modelEntry.ErrorCount)
    }
}

// RecordError records a failed request
func (e *ErrorTracker) RecordError(providerName, modelName string, statusCode int) {
    e.mu.Lock()
    defer e.mu.Unlock()

    providerEntry := e.ensureProviderEntry(providerName)
    providerEntry.TotalRequests++
    providerEntry.ErrorCount++
    providerEntry.LastError = time.Now()
    providerEntry.LastErrorStatus = statusCode
    providerEntry.ErrorRate = calculateErrorRate(providerEntry.SuccessCount, providerEntry.ErrorCount)

    if modelName != "" {
        modelEntry := e.ensureModelEntry(providerName, modelName)
        modelEntry.TotalRequests++
        modelEntry.ErrorCount++
        modelEntry.LastError = time.Now()
        modelEntry.LastErrorStatus = statusCode
        modelEntry.ErrorRate = calculateErrorRate(modelEntry.SuccessCount, modelEntry.ErrorCount)
    }
}

// GetErrorRate returns the aggregated error rate for a provider
func (e *ErrorTracker) GetErrorRate(providerName string) float64 {
    e.mu.RLock()
    defer e.mu.RUnlock()

    if data, exists := e.providerData[providerName]; exists {
        return data.ErrorRate
    }
    return 0.0
}

// GetData returns aggregated error data for a provider
func (e *ErrorTracker) GetData(providerName string) *ErrorData {
    e.mu.RLock()
    defer e.mu.RUnlock()

    if data, exists := e.providerData[providerName]; exists {
        return copyErrorData(data)
    }
    return nil
}

// GetModelData returns error data for a provider/model combination
func (e *ErrorTracker) GetModelData(providerName, modelName string) *ErrorData {
    if modelName == "" {
        return nil
    }

    e.mu.RLock()
    defer e.mu.RUnlock()

    key := makeModelKey(providerName, modelName)
    if data, exists := e.modelData[key]; exists {
        return copyErrorData(data)
    }
    return nil
}

// GetAll returns aggregated error data for all providers
func (e *ErrorTracker) GetAll() map[string]*ErrorData {
    e.mu.RLock()
    defer e.mu.RUnlock()

    result := make(map[string]*ErrorData, len(e.providerData))
    for key, data := range e.providerData {
        result[key] = copyErrorData(data)
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

func (e *ErrorTracker) ensureProviderEntry(providerName string) *ErrorData {
    if entry, exists := e.providerData[providerName]; exists {
        return entry
    }

    entry := &ErrorData{ProviderName: providerName}
    e.providerData[providerName] = entry
    return entry
}

func (e *ErrorTracker) ensureModelEntry(providerName, modelName string) *ErrorData {
    key := makeModelKey(providerName, modelName)
    if entry, exists := e.modelData[key]; exists {
        return entry
    }

    entry := &ErrorData{ProviderName: providerName, ModelName: modelName}
    e.modelData[key] = entry
    return entry
}

func makeModelKey(providerName, modelName string) string {
    return providerName + "::" + modelName
}

func copyErrorData(data *ErrorData) *ErrorData {
    if data == nil {
        return nil
    }

    return &ErrorData{
        ProviderName:    data.ProviderName,
        ModelName:       data.ModelName,
        TotalRequests:   data.TotalRequests,
        SuccessCount:    data.SuccessCount,
        ErrorCount:      data.ErrorCount,
        LastError:       data.LastError,
        LastErrorStatus: data.LastErrorStatus,
        ErrorRate:       data.ErrorRate,
    }
}
