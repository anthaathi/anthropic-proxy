package metrics

import (
	"anthropic-proxy/config"
	"anthropic-proxy/logger"
	"anthropic-proxy/provider"
	"anthropic-proxy/transform"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// RequestLogger interface to avoid circular dependency
type RequestLogger interface {
	LogRequest(provider, model, method, path string, headers map[string]string, body []byte, attemptNumber int, isStreaming bool) error
	LogResponse(provider, model string, statusCode int, headers map[string]string, body []byte, duration time.Duration, tokenCount, attemptNumber int, success bool, errorMsg string, isStreaming bool) error
}

// Benchmarker runs periodic benchmarks of providers
type Benchmarker struct {
	providerMgr    *provider.Manager
	tracker        *Tracker
	models         []config.Model
	interval       time.Duration
	stopCh         chan struct{}
	lastRunTime    time.Time
	nextRunTime    time.Time
	isRunning      bool
	history        []BenchmarkResult
	historyMutex   sync.RWMutex
	requestLogger  RequestLogger
}

// BenchmarkResult represents a single benchmark result
type BenchmarkResult struct {
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	TPS          float64   `json:"tps"`
	Tokens       int       `json:"tokens"`
	DurationS    float64   `json:"duration_s"`
	Timestamp    time.Time `json:"timestamp"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// BenchmarkStatus represents the current status of benchmarking
type BenchmarkStatus struct {
	IsRunning        bool      `json:"is_running"`
	LastRunTime      time.Time `json:"last_run_time"`
	NextRunTime      time.Time `json:"next_run_time"`
	TotalBenchmarks  int       `json:"total_benchmarks"`
	SuccessCount     int       `json:"success_count"`
	FailureCount     int       `json:"failure_count"`
	Interval         string    `json:"interval"`
}

// NewBenchmarker creates a new benchmarker
func NewBenchmarker(providerMgr *provider.Manager, tracker *Tracker, models []config.Model, requestLogger RequestLogger) *Benchmarker {
	now := time.Now()
	return &Benchmarker{
		providerMgr:   providerMgr,
		tracker:       tracker,
		models:        models,
		interval:      1 * time.Hour,
		stopCh:        make(chan struct{}),
		nextRunTime:   now.Add(30 * time.Second), // First run after 30 seconds
		history:       make([]BenchmarkResult, 0),
		requestLogger: requestLogger,
	}
}

// Start begins the periodic benchmark job
func (b *Benchmarker) Start() {
	ticker := time.NewTicker(b.interval)
	go func() {
		// Run once at startup (after a delay)
		time.Sleep(30 * time.Second)
		b.runBenchmark()

		// Then run periodically
		for {
			select {
			case <-ticker.C:
				b.runBenchmark()
			case <-b.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop stops the benchmark job
func (b *Benchmarker) Stop() {
	close(b.stopCh)
}

// runBenchmark tests all provider-model combinations
func (b *Benchmarker) runBenchmark() {
	b.isRunning = true
	b.lastRunTime = time.Now()
	b.nextRunTime = b.lastRunTime.Add(b.interval)

	logger.Debug("Starting benchmark of all providers")

	for _, model := range b.models {
		prov, exists := b.providerMgr.Get(model.Provider)
		if !exists {
			logger.Warn("Provider not found for model",
				"provider", model.Provider,
				"model", model.Name)
			// Store failed result
			b.storeResult(BenchmarkResult{
				Provider:     model.Provider,
				Model:        model.Name,
				Timestamp:    time.Now(),
				Success:      false,
				ErrorMessage: "Provider not found",
			})
			continue
		}

		b.benchmarkProviderModel(prov, model.Name)
	}

	b.isRunning = false
	logger.Debug("Benchmark completed")
}

// benchmarkProviderModel tests a single provider-model combination
func (b *Benchmarker) benchmarkProviderModel(prov *provider.Provider, modelName string) {
	result := BenchmarkResult{
		Provider:  prov.Name,
		Model:     modelName,
		Timestamp: time.Now(),
		Success:   false,
	}

	// Check if there's a recent real request (within last 1 minute)
	latestSampleTime := b.tracker.GetLatestSampleTime(prov.Name, modelName)
	if !latestSampleTime.IsZero() {
		timeSinceLastRequest := time.Since(latestSampleTime)
		if timeSinceLastRequest < 1*time.Minute {
			logger.Debug("Skipping benchmark - recent request exists",
				"provider", prov.Name,
				"model", modelName,
				"timeSinceLastRequest", timeSinceLastRequest.String())
			// Store skipped result
			result.Success = true
			result.ErrorMessage = "Skipped - recent request exists"
			b.storeResult(result)
			return
		}
	}

	// Create a simple test request with streaming enabled
	requestBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": "Say 'hello' in exactly one word."},
		},
		"max_tokens": 10,
		"stream":     true, // Use streaming since provider supports it
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		logger.Debug("Error marshaling benchmark request",
			"provider", prov.Name,
			"model", modelName,
			"error", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Determine the actual API path based on provider type
	apiPath := "/v1/messages"
	if prov.Type == transform.ProviderTypeOpenAI {
		apiPath = "/v1/chat/completions"
	}

	// Build full URL
	fullURL := prov.Endpoint + apiPath

	logger.Info("Benchmark request starting",
		"provider", prov.Name,
		"providerType", prov.Type,
		"model", modelName,
		"fullURL", fullURL,
		"requestBody", string(bodyBytes))

	// Log request to file if logger is enabled
	if b.requestLogger != nil {
		b.requestLogger.LogRequest(prov.Name, modelName, "POST", fullURL, nil, bodyBytes, 0, true)
	}

	startTime := time.Now()
	resp, err := prov.Client.ProxyRequest(ctx, "POST", "/v1/messages", bodyBytes, nil)
	duration := time.Since(startTime)

	if err != nil {
		logger.Error("Benchmark failed - request error",
			"provider", prov.Name,
			"providerType", prov.Type,
			"model", modelName,
			"error", err.Error(),
			"duration", duration.String())
		result.ErrorMessage = err.Error()

		// Log failed response to file
		if b.requestLogger != nil {
			b.requestLogger.LogResponse(prov.Name, modelName, 0, nil, nil, duration, 0, 0, false, err.Error(), true)
		}

		b.storeResult(result)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Read error response body
		errorBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			errorBody = []byte(fmt.Sprintf("Failed to read error body: %v", readErr))
		}

		logger.Error("Benchmark non-200 status",
			"provider", prov.Name,
			"providerType", prov.Type,
			"model", modelName,
			"statusCode", resp.StatusCode,
			"duration", duration.String(),
			"errorBody", string(errorBody))

		result.ErrorMessage = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(errorBody))

		// Log failed response to file with error body
		if b.requestLogger != nil {
			b.requestLogger.LogResponse(prov.Name, modelName, resp.StatusCode, nil, errorBody, duration, 0, 0, false, result.ErrorMessage, true)
		}

		b.storeResult(result)
		return
	}

	logger.Info("Benchmark response received",
		"provider", prov.Name,
		"providerType", prov.Type,
		"model", modelName,
		"statusCode", resp.StatusCode,
		"duration", duration.String())

	// Read response body first for better error handling
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Error reading benchmark response body",
			"provider", prov.Name,
			"providerType", prov.Type,
			"model", modelName,
			"error", err.Error())
		result.ErrorMessage = err.Error()

		// Log failed response to file
		if b.requestLogger != nil {
			b.requestLogger.LogResponse(prov.Name, modelName, resp.StatusCode, nil, nil, duration, 0, 0, false, err.Error(), true)
		}

		b.storeResult(result)
		return
	}

	logger.Info("Benchmark response body received",
		"provider", prov.Name,
		"providerType", prov.Type,
		"model", modelName,
		"bodyLength", len(responseBody),
		"bodyPreview", truncateString(string(responseBody), 500))

	// Convert OpenAI response to Anthropic format if needed
	finalResponseBody := responseBody
	if prov.Type == transform.ProviderTypeOpenAI {
		logger.Info("Converting OpenAI response to Anthropic format",
			"provider", prov.Name,
			"model", modelName)

		// For OpenAI providers, convert the response format
		convertedBody, err := convertOpenAIStreamToAnthropic(responseBody, modelName)
		if err != nil {
			logger.Error("Error converting OpenAI stream response",
				"provider", prov.Name,
				"model", modelName,
				"error", err.Error(),
				"rawResponse", truncateString(string(responseBody), 1000))

			// Try to extract tokens from raw OpenAI response as fallback
			logger.Info("Attempting fallback token extraction from raw OpenAI response",
				"provider", prov.Name,
				"model", modelName)

			tokens, _ := parseOpenAIStreamingResponse(string(responseBody))
			if tokens > 0 {
				logger.Info("Fallback token extraction successful",
					"provider", prov.Name,
					"model", modelName,
					"tokens", tokens)
				b.tracker.RecordRequest(prov.Name, modelName, tokens, duration)
				tps := b.tracker.GetTPS(prov.Name, modelName)
				result.Success = true
				result.TPS = tps
				result.Tokens = tokens
				result.DurationS = duration.Seconds()
			} else {
				logger.Error("Fallback token extraction failed",
					"provider", prov.Name,
					"model", modelName)
				result.ErrorMessage = fmt.Sprintf("conversion error: %v", err)
			}
			b.storeResult(result)
			return
		}

		logger.Info("OpenAI response converted successfully",
			"provider", prov.Name,
			"model", modelName,
			"convertedLength", len(convertedBody))

		finalResponseBody = convertedBody
	}

	// Parse streaming response to get token count
	responseStr := string(finalResponseBody)
	tokens, inputTokens := parseStreamingResponse(responseStr)

	if tokens == 0 {
		logger.Debug("Could not extract tokens from benchmark response",
			"provider", prov.Name,
			"model", modelName)
		// Use max_tokens as fallback for TPS calculation
		tokens = 10
	}

	// Record metrics
	b.tracker.RecordRequest(prov.Name, modelName, tokens, duration)

	tps := b.tracker.GetTPS(prov.Name, modelName)
	logger.Debug("Benchmark result",
		"provider", prov.Name,
		"model", modelName,
		"tps", tps,
		"outputTokens", tokens,
		"inputTokens", inputTokens,
		"duration", duration.Seconds())

	// Log successful response to file
	if b.requestLogger != nil {
		b.requestLogger.LogResponse(prov.Name, modelName, resp.StatusCode, nil, finalResponseBody, duration, tokens, 0, true, "", true)
	}

	// Store successful result
	result.Success = true
	result.TPS = tps
	result.Tokens = tokens
	result.DurationS = duration.Seconds()
	b.storeResult(result)
}

// extractTokenCount extracts the total token count from the response
func extractTokenCount(response map[string]interface{}) int {
	// Try to get usage.output_tokens or similar fields
	if usage, ok := response["usage"].(map[string]interface{}); ok {
		if outputTokens, ok := usage["output_tokens"].(float64); ok {
			return int(outputTokens)
		}
		// Try alternative field names
		if completionTokens, ok := usage["completion_tokens"].(float64); ok {
			return int(completionTokens)
		}
	}
	// Default to 10 (our request max_tokens) if we can't find it
	return 10
}

// parseStreamingResponse extracts token counts from a streaming SSE response
func parseStreamingResponse(sseData string) (outputTokens int, inputTokens int) {
	lines := strings.Split(sseData, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			dataJSON := strings.TrimPrefix(line, "data: ")
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(dataJSON), &data); err == nil {
				eventType := data["type"]

				// Check for usage in message_start
				if eventType == "message_start" {
					if msg, ok := data["message"].(map[string]interface{}); ok {
						if usage, ok := msg["usage"].(map[string]interface{}); ok {
							if it, ok := usage["input_tokens"].(float64); ok {
								inputTokens = int(it)
							}
						}
					}
				}

				// Check for usage in message_delta (final usage)
				if eventType == "message_delta" {
					if usage, ok := data["usage"].(map[string]interface{}); ok {
						if ot, ok := usage["output_tokens"].(float64); ok {
							outputTokens = int(ot)
						}
					}
				}

				// Check for content_block_delta to count tokens
				if eventType == "content_block_delta" {
					if delta, ok := data["delta"].(map[string]interface{}); ok {
						// Check for regular text
						if text, ok := delta["text"].(string); ok {
							// Rough estimate: ~4 chars per token
							textTokens := len(text) / 4
							if textTokens == 0 && len(text) > 0 {
								textTokens = 1 // At least 1 token for any non-empty text
							}
							outputTokens += textTokens
						}
						// Check for thinking tokens (Claude's extended thinking)
						if thinking, ok := delta["thinking"].(string); ok {
							// Count thinking tokens
							thinkingTokens := len(thinking) / 4
							if thinkingTokens == 0 && len(thinking) > 0 {
								thinkingTokens = 1
							}
							outputTokens += thinkingTokens
						}
					}
				}
			}
		}
	}

	return outputTokens, inputTokens
}

// TestRequest is a helper for manual testing
func TestRequest(body []byte) bool {
	var req map[string]interface{}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
		return false
	}
	// Check if this looks like a small test request
	if maxTokens, ok := req["max_tokens"].(float64); ok {
		return maxTokens <= 20
	}
	return false
}

// storeResult stores a benchmark result in history
func (b *Benchmarker) storeResult(result BenchmarkResult) {
	b.historyMutex.Lock()
	defer b.historyMutex.Unlock()

	// Add to history
	b.history = append(b.history, result)

	// Keep only the last 100 results to prevent memory growth
	maxHistory := 100
	if len(b.history) > maxHistory {
		b.history = b.history[len(b.history)-maxHistory:]
	}
}

// GetHistory returns the benchmark history
func (b *Benchmarker) GetHistory() []BenchmarkResult {
	b.historyMutex.RLock()
	defer b.historyMutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]BenchmarkResult, len(b.history))
	copy(result, b.history)
	return result
}

// GetStatus returns the current benchmark status
func (b *Benchmarker) GetStatus() BenchmarkStatus {
	b.historyMutex.RLock()
	defer b.historyMutex.RUnlock()

	successCount := 0
	failureCount := 0
	for _, result := range b.history {
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	return BenchmarkStatus{
		IsRunning:       b.isRunning,
		LastRunTime:     b.lastRunTime,
		NextRunTime:     b.nextRunTime,
		TotalBenchmarks: len(b.history),
		SuccessCount:    successCount,
		FailureCount:    failureCount,
		Interval:        b.interval.String(),
	}
}

// RunManualBenchmark triggers a manual benchmark run
func (b *Benchmarker) RunManualBenchmark() {
	// Run in goroutine to avoid blocking
	go b.runBenchmark()
}

// convertOpenAIStreamToAnthropic converts OpenAI streaming response to Anthropic format
func convertOpenAIStreamToAnthropic(openaiStream []byte, model string) ([]byte, error) {
	responseStr := string(openaiStream)
	lines := strings.Split(responseStr, "\n")

	var anthropicLines []string
	messageStarted := false
	contentBlockStarted := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			dataJSON := strings.TrimPrefix(line, "data: ")

			if dataJSON == "[DONE]" {
				// Add message_stop
				stopEvent := map[string]interface{}{"type": "message_stop"}
				stopJSON, _ := json.Marshal(stopEvent)
				anthropicLines = append(anthropicLines, "data: "+string(stopJSON))
				continue
			}

			var chunk transform.OpenAIStreamChunk
			if err := json.Unmarshal([]byte(dataJSON), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]

			// Handle first chunk - send message_start and content_block_start
			if choice.Delta.Role != "" && !messageStarted {
				messageStarted = true
				startEvent := map[string]interface{}{
					"type": "message_start",
					"message": map[string]interface{}{
						"id":    chunk.ID,
						"type":  "message",
						"role":  "assistant",
						"model": model,
						"usage": map[string]int{
							"input_tokens":  0,
							"output_tokens": 0,
						},
					},
				}
				startJSON, _ := json.Marshal(startEvent)
				anthropicLines = append(anthropicLines, "data: "+string(startJSON))
			}

			if choice.Delta.Content != "" && !contentBlockStarted {
				contentBlockStarted = true
				blockStartEvent := map[string]interface{}{
					"type":  "content_block_start",
					"index": 0,
					"content_block": map[string]interface{}{
						"type": "text",
						"text": "",
					},
				}
				blockJSON, _ := json.Marshal(blockStartEvent)
				anthropicLines = append(anthropicLines, "data: "+string(blockJSON))
			}

			// Handle content delta
			if choice.Delta.Content != "" {
				deltaEvent := map[string]interface{}{
					"type":  "content_block_delta",
					"index": 0,
					"delta": map[string]interface{}{
						"type": "text_delta",
						"text": choice.Delta.Content,
					},
				}
				deltaJSON, _ := json.Marshal(deltaEvent)
				anthropicLines = append(anthropicLines, "data: "+string(deltaJSON))
			}

			// Handle finish
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				// content_block_stop
				blockStopEvent := map[string]interface{}{
					"type":  "content_block_stop",
					"index": 0,
				}
				blockStopJSON, _ := json.Marshal(blockStopEvent)
				anthropicLines = append(anthropicLines, "data: "+string(blockStopJSON))

				// message_delta
				stopReason := "end_turn"
				if *choice.FinishReason == "length" {
					stopReason = "max_tokens"
				}
				deltaEvent := map[string]interface{}{
					"type": "message_delta",
					"delta": map[string]interface{}{
						"stop_reason": stopReason,
					},
					"usage": map[string]int{
						"output_tokens": 0,
					},
				}
				deltaJSON, _ := json.Marshal(deltaEvent)
				anthropicLines = append(anthropicLines, "data: "+string(deltaJSON))
			}
		}
	}

	return []byte(strings.Join(anthropicLines, "\n")), nil
}

// parseOpenAIStreamingResponse extracts tokens from OpenAI streaming format
func parseOpenAIStreamingResponse(sseData string) (outputTokens int, inputTokens int) {
	lines := strings.Split(sseData, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			dataJSON := strings.TrimPrefix(line, "data: ")
			if dataJSON == "[DONE]" {
				continue
			}

			var chunk transform.OpenAIStreamChunk
			if err := json.Unmarshal([]byte(dataJSON), &chunk); err == nil {
				if len(chunk.Choices) > 0 {
					choice := chunk.Choices[0]
					if choice.Delta.Content != "" {
						// Rough estimate: ~4 chars per token
						textTokens := len(choice.Delta.Content) / 4
						if textTokens == 0 && len(choice.Delta.Content) > 0 {
							textTokens = 1
						}
						outputTokens += textTokens
					}
				}
			}
		}
	}

	return outputTokens, inputTokens
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
