package router

import (
	"errors"
)

var (
	// ErrNoModelFound is returned when no model matches the requested name
	ErrNoModelFound = errors.New("no model found matching the requested name")

	// ErrNoProvidersAvailable is returned when no providers meet the requirements
	ErrNoProvidersAvailable = errors.New("no providers available that meet TPS threshold")

	// ErrAllProvidersFailed is returned when all providers have been tried and failed
	ErrAllProvidersFailed = errors.New("all providers failed")
)

// FallbackManager manages failover logic
type FallbackManager struct {
	selector *Selector
}

// NewFallbackManager creates a new fallback manager
func NewFallbackManager(selector *Selector) *FallbackManager {
	return &FallbackManager{
		selector: selector,
	}
}

// GetOrderedProviders returns an ordered list of providers to try
func (f *FallbackManager) GetOrderedProviders(requestedModel string) ([]*ProviderChoice, error) {
	return f.selector.SelectProviders(requestedModel)
}

// ShouldRetry determines if we should retry with the next provider
func (f *FallbackManager) ShouldRetry(statusCode int, err error) bool {
	// Network errors - should retry
	if err != nil {
		return true
	}

	// Status code based decisions
	switch {
	case statusCode >= 200 && statusCode < 300:
		// Success - no retry needed
		return false
	case statusCode == 401 || statusCode == 403:
		// Auth errors - likely a config issue, but try next provider
		return true
	case statusCode == 429:
		// Rate limit - try next provider
		return true
	case statusCode >= 400 && statusCode < 500:
		// Client errors - try next provider (might work with different provider)
		return true
	case statusCode >= 500:
		// Server errors - definitely retry with next provider
		return true
	default:
		// Unknown status - retry to be safe
		return true
	}
}

// IsClientError checks if the error is a client-side issue
func IsClientError(statusCode int) bool {
	return statusCode >= 400 && statusCode < 500
}

// IsServerError checks if the error is a server-side issue
func IsServerError(statusCode int) bool {
	return statusCode >= 500
}

// IsSuccess checks if the status code indicates success
func IsSuccess(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}
