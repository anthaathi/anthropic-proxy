package retry

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Config holds retry configuration
type Config struct {
	MaxRetries         int
	InitialDelay       time.Duration
	MaxDelay           time.Duration
	BackoffMultiplier  float64
}

// DefaultConfig returns a default retry configuration
func DefaultConfig() *Config {
	return &Config{
		MaxRetries:        3,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          5 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// Func is a function that can be retried
type Func func() error

// Do executes the given function with retry logic and exponential backoff
func Do(ctx context.Context, config *Config, fn Func) error {
	if config == nil {
		config = DefaultConfig()
	}

	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Don't sleep after the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Calculate backoff delay with exponential backoff
		delay := time.Duration(float64(config.InitialDelay) * math.Pow(config.BackoffMultiplier, float64(attempt)))
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}

		// Sleep with context cancellation support
		select {
		case <-time.After(delay):
			// Continue to next retry
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", config.MaxRetries, lastErr)
}

// IsRetriable determines if an error should be retried based on status code
func IsRetriable(statusCode int) bool {
	// Retry on:
	// - 0 (network error)
	// - 429 (rate limit)
	// - 5xx (server errors)
	if statusCode == 0 || statusCode == 429 || (statusCode >= 500 && statusCode < 600) {
		return true
	}
	return false
}
