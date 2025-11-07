package proxy

import (
	"anthropic-proxy/logger"
	"anthropic-proxy/provider"
	"anthropic-proxy/router"
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// handleStreamingRequest handles a streaming SSE request
func (h *Handler) handleStreamingRequest(c *gin.Context, prov *provider.Provider, body []byte,
	headers map[string]string, choice *router.ProviderChoice, startTime time.Time) bool {

	resp, err := prov.Client.StreamRequest(c.Request.Context(), "POST", "/v1/messages", body, headers)

	// Check for network errors
	if err != nil {
		proxyErr := ClassifyError(0, err, prov.Name)
		LogError(proxyErr)
		h.errorTracker.RecordError(prov.Name, 0)
		return false
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		proxyErr := ClassifyError(resp.StatusCode, nil, prov.Name)
		LogError(proxyErr)
		h.errorTracker.RecordError(prov.Name, resp.StatusCode)
		return false
	}

	// Success! Stream the response
	logger.Debug("Streaming response from provider",
		"provider", prov.Name,
		"model", choice.ActualModel)

	// Set streaming headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Stream the response
	totalTokens := 0
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		logger.Error("Streaming not supported")
		return false
	}

	// Read and forward SSE stream
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logger.Error("Error reading stream from provider",
					"provider", prov.Name,
					"error", err.Error())
			}
			break
		}

		// Write line to client
		_, writeErr := c.Writer.Write(line)
		if writeErr != nil {
			logger.Error("Error writing to client",
				"error", writeErr.Error())
			break
		}

		// Parse SSE data to count tokens
		if bytes.HasPrefix(line, []byte("data: ")) {
			dataStr := string(bytes.TrimPrefix(line, []byte("data: ")))
			dataStr = strings.TrimSpace(dataStr)

			// Skip [DONE] marker
			if dataStr == "[DONE]" {
				continue
			}

			// Parse JSON to extract tokens
			var eventData map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &eventData); err == nil {
				tokens := extractStreamTokens(eventData)
				totalTokens += tokens
			}
		}

		flusher.Flush()
	}

	duration := time.Since(startTime)

	// Record metrics
	if totalTokens > 0 {
		h.tracker.RecordRequest(prov.Name, choice.ActualModel, totalTokens, duration)
		logger.Debug("Stream completed from provider",
			"provider", prov.Name,
			"tokens", totalTokens,
			"duration", duration.Seconds(),
			"tps", float64(totalTokens)/duration.Seconds())
	} else {
		// If we couldn't count tokens, estimate based on response
		h.tracker.RecordRequest(prov.Name, choice.ActualModel, 100, duration)
	}

	// Record success
	h.errorTracker.RecordSuccess(prov.Name)

	return true
}

// extractStreamTokens extracts token count from streaming event data
func extractStreamTokens(eventData map[string]interface{}) int {
	// Check for content_block_delta with text
	if eventType, ok := eventData["type"].(string); ok {
		if eventType == "content_block_delta" {
			if delta, ok := eventData["delta"].(map[string]interface{}); ok {
				if text, ok := delta["text"].(string); ok {
					// Rough estimation: ~4 chars per token
					return len(text) / 4
				}
			}
		}

		// Check for message_delta with usage
		if eventType == "message_delta" {
			if usage, ok := eventData["usage"].(map[string]interface{}); ok {
				if outputTokens, ok := usage["output_tokens"].(float64); ok {
					return int(outputTokens)
				}
			}
		}
	}

	return 0
}
