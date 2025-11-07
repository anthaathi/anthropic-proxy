package requestlog

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// LogEntry represents a single request or response log entry
type LogEntry struct {
	Timestamp     time.Time         `json:"timestamp"`
	Direction     string            `json:"direction"` // "request" or "response"
	Provider      string            `json:"provider"`
	Model         string            `json:"model,omitempty"`
	AttemptNumber int               `json:"attempt_number,omitempty"`
	Method        string            `json:"method,omitempty"`
	Path          string            `json:"path,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Body          string            `json:"body"` // Removed omitempty to always log body (even if empty)
	StatusCode    int               `json:"status_code,omitempty"`
	Duration      float64           `json:"duration_seconds,omitempty"`
	TokenCount    int               `json:"token_count,omitempty"`
	Success       bool              `json:"success"`
	Error         string            `json:"error,omitempty"`
	IsStreaming   bool              `json:"is_streaming,omitempty"`
}

// RequestLogger handles logging of HTTP requests and responses to a file
type RequestLogger struct {
	file *os.File
	mu   sync.Mutex
}

// NewRequestLogger creates a new request logger that writes to the specified file
func NewRequestLogger(filePath string) (*RequestLogger, error) {
	// Open file for append, create if doesn't exist
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &RequestLogger{
		file: file,
	}, nil
}

// LogRequest logs an outgoing request
func (rl *RequestLogger) LogRequest(provider, model, method, path string, headers map[string]string, body []byte, attemptNumber int, isStreaming bool) error {
	// Defensive nil check to prevent panics
	if rl == nil {
		return nil // Silently skip logging if logger is not initialized
	}

	entry := LogEntry{
		Timestamp:     time.Now(),
		Direction:     "request",
		Provider:      provider,
		Model:         model,
		AttemptNumber: attemptNumber,
		Method:        method,
		Path:          path,
		Headers:       rl.sanitizeHeaders(headers),
		Body:          string(body),
		Success:       true, // Requests are always "successful" in terms of being sent
		IsStreaming:   isStreaming,
	}

	return rl.writeEntry(entry)
}

// LogResponse logs an incoming response
func (rl *RequestLogger) LogResponse(provider, model string, statusCode int, headers map[string]string, body []byte, duration time.Duration, tokenCount, attemptNumber int, success bool, errorMsg string, isStreaming bool) error {
	// Defensive nil check to prevent panics
	if rl == nil {
		return nil // Silently skip logging if logger is not initialized
	}

	entry := LogEntry{
		Timestamp:     time.Now(),
		Direction:     "response",
		Provider:      provider,
		Model:         model,
		AttemptNumber: attemptNumber,
		StatusCode:    statusCode,
		Headers:       rl.sanitizeHeaders(headers),
		Body:          string(body),
		Duration:      duration.Seconds(),
		TokenCount:    tokenCount,
		Success:       success,
		Error:         errorMsg,
		IsStreaming:   isStreaming,
	}

	return rl.writeEntry(entry)
}

// writeEntry writes a log entry to the file in JSON Lines format
func (rl *RequestLogger) writeEntry(entry LogEntry) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Marshal to JSON
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Write JSON line
	if _, err := rl.file.Write(jsonBytes); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	// Write newline
	if _, err := rl.file.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// sanitizeHeaders creates a copy of headers with sensitive data masked
func (rl *RequestLogger) sanitizeHeaders(headers map[string]string) map[string]string {
	// Defensive nil check
	if rl == nil || headers == nil {
		return make(map[string]string)
	}

	sanitized := make(map[string]string, len(headers))
	for k, v := range headers {
		// Mask authorization headers
		if k == "Authorization" || k == "X-Api-Key" {
			if len(v) > 10 {
				sanitized[k] = v[:10] + "***REDACTED***"
			} else {
				sanitized[k] = "***REDACTED***"
			}
		} else {
			sanitized[k] = v
		}
	}
	return sanitized
}

// Close closes the log file
func (rl *RequestLogger) Close() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.file != nil {
		return rl.file.Close()
	}
	return nil
}
