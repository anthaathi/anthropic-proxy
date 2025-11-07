package tui

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Level     slog.Level
	Message   string
	Attrs     map[string]interface{}
}

// LogBuffer is a ring buffer for storing log entries
type LogBuffer struct {
	entries []LogEntry
	maxSize int
	mu      sync.RWMutex
	offset  int
}

// NewLogBuffer creates a new log buffer
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		entries: make([]LogEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add adds a log entry to the buffer
func (lb *LogBuffer) Add(entry LogEntry) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if len(lb.entries) >= lb.maxSize {
		// Ring buffer behavior: remove oldest
		lb.entries = append(lb.entries[1:], entry)
	} else {
		lb.entries = append(lb.entries, entry)
	}
}

// GetAll returns all log entries
func (lb *LogBuffer) GetAll() []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	result := make([]LogEntry, len(lb.entries))
	copy(result, lb.entries)
	return result
}

// AddLog adds a log message with level for quick logging
func (lb *LogBuffer) AddLog(message string, level string) {
	slogLevel := slog.LevelInfo
	switch level {
	case "DEBUG":
		slogLevel = slog.LevelDebug
	case "WARN", "WARNING":
		slogLevel = slog.LevelWarn
	case "ERROR":
		slogLevel = slog.LevelError
	case "SUCCESS":
		slogLevel = slog.LevelInfo // Use INFO for success
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     slogLevel,
		Message:   message,
		Attrs:     make(map[string]interface{}),
	}

	lb.Add(entry)
}

// TUIHandler is a custom slog handler that captures logs for the TUI
type TUIHandler struct {
	buffer *LogBuffer
	level  slog.Level
}

// NewTUIHandler creates a new TUI handler
func NewTUIHandler(buffer *LogBuffer, level slog.Level) *TUIHandler {
	return &TUIHandler{
		buffer: buffer,
		level:  level,
	}
}

// Enabled reports whether the handler handles records at the given level
func (h *TUIHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle handles the log record
func (h *TUIHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]interface{})
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})

	entry := LogEntry{
		Timestamp: r.Time,
		Level:     r.Level,
		Message:   r.Message,
		Attrs:     attrs,
	}

	h.buffer.Add(entry)
	return nil
}

// WithAttrs returns a new handler with additional attributes
func (h *TUIHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// For simplicity, return the same handler
	return h
}

// WithGroup returns a new handler with a group name
func (h *TUIHandler) WithGroup(name string) slog.Handler {
	// For simplicity, return the same handler
	return h
}

// FormatLogEntry formats a log entry as a colored string
func FormatLogEntry(entry LogEntry) string {
	// ANSI color codes
	var levelColor string
	var levelStr string

	switch entry.Level {
	case slog.LevelDebug:
		levelColor = "[grey]"
		levelStr = "DEBUG"
	case slog.LevelInfo:
		levelColor = "[green]"
		levelStr = "INFO "
	case slog.LevelWarn:
		levelColor = "[yellow]"
		levelStr = "WARN "
	case slog.LevelError:
		levelColor = "[red]"
		levelStr = "ERROR"
	default:
		levelColor = "[white]"
		levelStr = "     "
	}

	timestamp := entry.Timestamp.Format("15:04:05.000")

	// Format attributes
	attrStr := ""
	if len(entry.Attrs) > 0 {
		for k, v := range entry.Attrs {
			attrStr += fmt.Sprintf(" [darkgray]%s[white]=[darkgray]%v[white]", k, v)
		}
	}

	return fmt.Sprintf("[darkgray]%s[white] %s%s[white] %s%s",
		timestamp, levelColor, levelStr, entry.Message, attrStr)
}
