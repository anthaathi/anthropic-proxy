package proxy

import (
	"anthropic-proxy/logger"
	"anthropic-proxy/provider"
	"anthropic-proxy/router"
	"anthropic-proxy/transform"
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
	headers map[string]string, choice *router.ProviderChoice, startTime time.Time, attemptNumber int, modelName string) bool {

	// Log request if request logger is enabled
	if h.requestLogger != nil {
		h.requestLogger.LogRequest(prov.Name, modelName, "POST", "/v1/messages", headers, body, attemptNumber, true)
	}

	resp, err := prov.Client.StreamRequest(c.Request.Context(), "POST", "/v1/messages", body, headers)

	// Check for network errors
	if err != nil {
		proxyErr := ClassifyError(0, err, prov.Name)
		LogError(proxyErr)
		h.errorTracker.RecordError(prov.Name, choice.ActualModel, 0)

		// Log failed response
		if h.requestLogger != nil {
			duration := time.Since(startTime)
			h.requestLogger.LogResponse(prov.Name, modelName, 0, nil, nil, duration, 0, attemptNumber, false, proxyErr.Message, true)
		}

		return false
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		proxyErr := ClassifyError(resp.StatusCode, nil, prov.Name)
		LogError(proxyErr)
		h.errorTracker.RecordError(prov.Name, choice.ActualModel, resp.StatusCode)

		// Log failed response
		if h.requestLogger != nil {
			duration := time.Since(startTime)
			respHeaders := extractHeaders(resp.Header)
			h.requestLogger.LogResponse(prov.Name, modelName, resp.StatusCode, respHeaders, nil, duration, 0, attemptNumber, false, proxyErr.Message, true)
		}

		return false
	}

	// Success! Stream the response
	logger.Debug("Streaming response from provider",
		"provider", prov.Name,
		"model", choice.ActualModel,
		"providerType", prov.Type)

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

	// Buffer to accumulate stream data for logging
	var streamBuffer bytes.Buffer

	// Check if we need to convert OpenAI stream to Anthropic format
	if prov.Type == transform.ProviderTypeOpenAI {
		// Handle OpenAI streaming with conversion
		totalTokens = h.handleOpenAIStream(c, resp, &streamBuffer, choice.ActualModel, flusher)
	} else {
		// Handle native Anthropic streaming
		totalTokens = h.handleAnthropicStream(c, resp, &streamBuffer, flusher)
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
	h.errorTracker.RecordSuccess(prov.Name, choice.ActualModel)

	// Log successful streaming response
	if h.requestLogger != nil {
		respHeaders := extractHeaders(resp.Header)
		h.requestLogger.LogResponse(prov.Name, modelName, resp.StatusCode, respHeaders, streamBuffer.Bytes(), duration, totalTokens, attemptNumber, true, "", true)
	}

	return true
}

// handleAnthropicStream handles native Anthropic SSE streaming
func (h *Handler) handleAnthropicStream(c *gin.Context, resp *http.Response, streamBuffer *bytes.Buffer, flusher http.Flusher) int {
	totalTokens := 0
	reader := bufio.NewReader(resp.Body)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logger.Error("Error reading Anthropic stream", "error", err.Error())
			}
			break
		}

		// Accumulate for logging
		if h.requestLogger != nil {
			streamBuffer.Write(line)
		}

		// Write line to client
		if _, writeErr := c.Writer.Write(line); writeErr != nil {
			logger.Error("Error writing to client", "error", writeErr.Error())
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

	return totalTokens
}

// handleOpenAIStream handles OpenAI SSE streaming and converts to Anthropic format
func (h *Handler) handleOpenAIStream(c *gin.Context, resp *http.Response, streamBuffer *bytes.Buffer, model string, flusher http.Flusher) int {
	totalTokens := 0
	reader := bufio.NewReader(resp.Body)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logger.Error("Error reading OpenAI stream", "error", err.Error())
			}
			break
		}

		// Accumulate for logging
		if h.requestLogger != nil {
			streamBuffer.Write(line)
		}

		// Parse SSE data
		if bytes.HasPrefix(line, []byte("data: ")) {
			dataStr := string(bytes.TrimPrefix(line, []byte("data: ")))
			dataStr = strings.TrimSpace(dataStr)

			// Check for [DONE] marker
			if dataStr == "[DONE]" {
				// Write [DONE] marker in Anthropic format
				c.Writer.Write([]byte("data: [DONE]\n\n"))
				flusher.Flush()
				continue
			}

			// Parse OpenAI chunk
			var openaiChunk transform.OpenAIStreamChunk
			if err := json.Unmarshal([]byte(dataStr), &openaiChunk); err != nil {
				logger.Error("Failed to parse OpenAI stream chunk", "error", err.Error())
				continue
			}

			// Convert to Anthropic events
			anthropicEvents, err := transform.OpenAIStreamToAnthropicStream([]byte(dataStr), model)
			if err != nil {
				logger.Error("Failed to convert OpenAI stream to Anthropic format", "error", err.Error())
				continue
			}

			// Write each converted event
			for _, event := range anthropicEvents {
				// Write SSE format: "event: message_start\ndata: {...}\n\n"
				var eventType string
				var eventData map[string]interface{}
				json.Unmarshal([]byte(event), &eventData)
				if t, ok := eventData["type"].(string); ok {
					eventType = t
				}

				// Write event type if present
				if eventType != "" {
					c.Writer.Write([]byte("event: " + eventType + "\n"))
				}

				// Write data
				c.Writer.Write([]byte("data: " + event + "\n\n"))
				flusher.Flush()

				// Count tokens
				tokens := extractStreamTokens(eventData)
				totalTokens += tokens
			}
		} else {
			// Forward non-data lines as-is (comments, blank lines)
			c.Writer.Write(line)
			flusher.Flush()
		}
	}

	return totalTokens
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
