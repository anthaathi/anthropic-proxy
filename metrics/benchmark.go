package metrics

import (
	"anthropic-proxy/config"
	"anthropic-proxy/logger"
	"anthropic-proxy/provider"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// Benchmarker runs periodic benchmarks of providers
type Benchmarker struct {
	providerMgr *provider.Manager
	tracker     *Tracker
	models      []config.Model
	interval    time.Duration
	stopCh      chan struct{}
}

// NewBenchmarker creates a new benchmarker
func NewBenchmarker(providerMgr *provider.Manager, tracker *Tracker, models []config.Model) *Benchmarker {
	return &Benchmarker{
		providerMgr: providerMgr,
		tracker:     tracker,
		models:      models,
		interval:    1 * time.Hour,
		stopCh:      make(chan struct{}),
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
	logger.Debug("Starting benchmark of all providers")

	for _, model := range b.models {
		prov, exists := b.providerMgr.Get(model.Provider)
		if !exists {
			logger.Warn("Provider not found for model",
				"provider", model.Provider,
				"model", model.Name)
			continue
		}

		b.benchmarkProviderModel(prov, model.Name)
	}

	logger.Debug("Benchmark completed")
}

// benchmarkProviderModel tests a single provider-model combination
func (b *Benchmarker) benchmarkProviderModel(prov *provider.Provider, modelName string) {
	// Check if there's a recent real request (within last 1 minute)
	latestSampleTime := b.tracker.GetLatestSampleTime(prov.Name, modelName)
	if !latestSampleTime.IsZero() {
		timeSinceLastRequest := time.Since(latestSampleTime)
		if timeSinceLastRequest < 1*time.Minute {
			logger.Debug("Skipping benchmark - recent request exists",
				"provider", prov.Name,
				"model", modelName,
				"timeSinceLastRequest", timeSinceLastRequest.String())
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

	startTime := time.Now()
	resp, err := prov.Client.ProxyRequest(ctx, "POST", "/v1/messages", bodyBytes, nil)
	duration := time.Since(startTime)

	if err != nil {
		logger.Debug("Benchmark failed",
			"provider", prov.Name,
			"model", modelName,
			"error", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Debug("Benchmark non-200 status",
			"provider", prov.Name,
			"model", modelName,
			"statusCode", resp.StatusCode)
		return
	}

	// Read response body first for better error handling
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Debug("Error reading benchmark response",
			"provider", prov.Name,
			"model", modelName,
			"error", err.Error())
		return
	}

	// Parse streaming response to get token count
	responseStr := string(responseBody)
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
