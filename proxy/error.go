package proxy

import (
	"anthropic-proxy/logger"
	"fmt"
	"net/http"
)

// ErrorType represents different types of errors
type ErrorType string

const (
	ErrorTypeNetwork     ErrorType = "network_error"
	ErrorTypeAuth        ErrorType = "authentication_error"
	ErrorTypeRateLimit   ErrorType = "rate_limit_error"
	ErrorTypeClient      ErrorType = "client_error"
	ErrorTypeServer      ErrorType = "server_error"
	ErrorTypeUnknown     ErrorType = "unknown_error"
	ErrorTypeNoProviders ErrorType = "no_providers_error"
)

// ProxyError represents an error during proxying
type ProxyError struct {
	Type       ErrorType
	StatusCode int
	Message    string
	Provider   string
	Err        error
}

// Error implements the error interface
func (e *ProxyError) Error() string {
	if e.Provider != "" {
		return fmt.Sprintf("[%s] %s: %s (status: %d)", e.Provider, e.Type, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("%s: %s (status: %d)", e.Type, e.Message, e.StatusCode)
}

// ClassifyError determines the error type based on status code and error
func ClassifyError(statusCode int, err error, providerName string) *ProxyError {
	proxyErr := &ProxyError{
		StatusCode: statusCode,
		Provider:   providerName,
		Err:        err,
	}

	// Network errors
	if err != nil {
		proxyErr.Type = ErrorTypeNetwork
		proxyErr.Message = fmt.Sprintf("network error: %v", err)
		return proxyErr
	}

	// Classify by status code
	switch {
	case statusCode >= 200 && statusCode < 300:
		// Not an error
		return nil

	case statusCode == 401 || statusCode == 403:
		proxyErr.Type = ErrorTypeAuth
		proxyErr.Message = "authentication or authorization failed"

	case statusCode == 429:
		proxyErr.Type = ErrorTypeRateLimit
		proxyErr.Message = "rate limit exceeded"

	case statusCode >= 400 && statusCode < 500:
		proxyErr.Type = ErrorTypeClient
		proxyErr.Message = fmt.Sprintf("client error: status %d", statusCode)

	case statusCode >= 500:
		proxyErr.Type = ErrorTypeServer
		proxyErr.Message = fmt.Sprintf("server error: status %d", statusCode)

	default:
		proxyErr.Type = ErrorTypeUnknown
		proxyErr.Message = fmt.Sprintf("unknown error: status %d", statusCode)
	}

	return proxyErr
}

// LogError logs a proxy error with context
func LogError(err *ProxyError) {
	if err == nil {
		return
	}
	logger.Error("Proxy error",
		"provider", err.Provider,
		"type", err.Type,
		"statusCode", err.StatusCode,
		"message", err.Message)
}

// ShouldRetry determines if an error is retriable
func ShouldRetry(err *ProxyError) bool {
	if err == nil {
		return false
	}

	// Retry on network errors, server errors, and rate limits
	switch err.Type {
	case ErrorTypeNetwork, ErrorTypeServer, ErrorTypeRateLimit:
		return true
	case ErrorTypeAuth, ErrorTypeClient:
		// Also retry auth and client errors with different providers
		// as they might work with a different provider's API
		return true
	default:
		return false
	}
}

// CreateErrorResponse creates a JSON error response
func CreateErrorResponse(statusCode int, errorType, message string) map[string]interface{} {
	return map[string]interface{}{
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
		},
	}
}

// RespondWithError sends an error response to the client
func RespondWithError(w http.ResponseWriter, statusCode int, errorType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := CreateErrorResponse(statusCode, errorType, message)
	// Note: In real implementation, use json.NewEncoder(w).Encode(response)
	_ = response
}
