package proxy

import (
	"anthropic-proxy/logger"
	"anthropic-proxy/metrics"
	"anthropic-proxy/provider"
	"anthropic-proxy/retry"
	"anthropic-proxy/router"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler handles proxy requests
type Handler struct {
	fallbackMgr  *router.FallbackManager
	tracker      *metrics.Tracker
	errorTracker *metrics.ErrorTracker
	retryConfig  *retry.Config
}

// NewHandler creates a new proxy handler
func NewHandler(fallbackMgr *router.FallbackManager, tracker *metrics.Tracker, errorTracker *metrics.ErrorTracker, retryConfig *retry.Config) *Handler {
	return &Handler{
		fallbackMgr:  fallbackMgr,
		tracker:      tracker,
		errorTracker: errorTracker,
		retryConfig:  retryConfig,
	}
}

// HandleMessages handles POST /v1/messages requests
func (h *Handler) HandleMessages(c *gin.Context) {
	// Read request body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, CreateErrorResponse(400, "invalid_request", "failed to read request body"))
		return
	}

	// Parse request to get model name
	var requestBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &requestBody); err != nil {
		c.JSON(http.StatusBadRequest, CreateErrorResponse(400, "invalid_request", "invalid JSON in request body"))
		return
	}

	// Extract model name
	modelName, ok := requestBody["model"].(string)
	if !ok || modelName == "" {
		c.JSON(http.StatusBadRequest, CreateErrorResponse(400, "invalid_request", "model field is required"))
		return
	}

	// Check if this is a streaming request
	isStreaming := false
	if stream, ok := requestBody["stream"].(bool); ok {
		isStreaming = stream
	}

	// Get ordered list of providers to try
	providerChoices, err := h.fallbackMgr.GetOrderedProviders(modelName)
	if err != nil {
		logger.Error("Error selecting providers for model",
			"model", modelName,
			"error", err.Error())
		c.JSON(http.StatusBadGateway, CreateErrorResponse(502, "no_providers", err.Error()))
		return
	}

	// Copy headers to forward
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 && key != "Authorization" {
			headers[key] = values[0]
		}
	}

	// Try each provider in order
	var lastError *ProxyError
	for _, choice := range providerChoices {
		// Update request body with the actual model name for this provider
		requestBody["model"] = choice.ActualModel
		updatedBody, err := json.Marshal(requestBody)
		if err != nil {
			continue
		}

		logger.Debug("Trying provider for model",
			"provider", choice.Provider.Name,
			"model", modelName,
			"weight", choice.Weight,
			"tps", choice.TPS)

		// Make the request
		startTime := time.Now()

		if isStreaming {
			// Handle streaming request
			success := h.handleStreamingRequest(c, choice.Provider, updatedBody, headers, choice, startTime)
			if success {
				return // Success, response already sent
			}
			// Failed, try next provider
		} else {
			// Handle non-streaming request
			success, proxyErr := h.handleNonStreamingRequest(c, choice.Provider, updatedBody, headers, choice, startTime)
			if success {
				return // Success, response already sent
			}
			lastError = proxyErr
		}
	}

	// All providers failed
	logger.Error("All providers failed for model",
		"model", modelName)

	if lastError != nil {
		c.JSON(http.StatusBadGateway, CreateErrorResponse(502, "all_providers_failed",
			"All providers failed: "+lastError.Message))
	} else {
		c.JSON(http.StatusBadGateway, CreateErrorResponse(502, "all_providers_failed",
			"All providers failed to process the request"))
	}
}

// handleNonStreamingRequest handles a non-streaming request to a provider
func (h *Handler) handleNonStreamingRequest(c *gin.Context, prov *provider.Provider, body []byte,
	headers map[string]string, choice *router.ProviderChoice, startTime time.Time) (bool, *ProxyError) {

	resp, err := prov.Client.ProxyRequestWithRetry(c.Request.Context(), "POST", "/v1/messages", body, headers, h.retryConfig, prov.Name)
	duration := time.Since(startTime)

	// Check for network errors
	if err != nil {
		proxyErr := ClassifyError(0, err, prov.Name)
		LogError(proxyErr)
		h.errorTracker.RecordError(prov.Name, 0)
		return false, proxyErr
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		proxyErr := ClassifyError(resp.StatusCode, nil, prov.Name)
		LogError(proxyErr)
		h.errorTracker.RecordError(prov.Name, resp.StatusCode)
		return false, proxyErr
	}

	// Success! Read and return response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Error reading response from provider",
			"provider", prov.Name,
			"error", err.Error())
		return false, ClassifyError(0, err, prov.Name)
	}

	// Extract token count and record metrics
	var responseData map[string]interface{}
	if err := json.Unmarshal(responseBody, &responseData); err == nil {
		tokens := extractTokenCount(responseData)
		h.tracker.RecordRequest(prov.Name, choice.ActualModel, tokens, duration)
		logger.Debug("Request succeeded with provider",
			"provider", prov.Name,
			"tokens", tokens,
			"duration", duration.Seconds(),
			"tps", float64(tokens)/duration.Seconds())
	}

	// Record success
	h.errorTracker.RecordSuccess(prov.Name)

	// Copy response headers
	for key, values := range resp.Header {
		if len(values) > 0 {
			c.Header(key, values[0])
		}
	}

	// Send response
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), responseBody)
	return true, nil
}

// extractTokenCount extracts token count from response
func extractTokenCount(response map[string]interface{}) int {
	if usage, ok := response["usage"].(map[string]interface{}); ok {
		if outputTokens, ok := usage["output_tokens"].(float64); ok {
			return int(outputTokens)
		}
		if completionTokens, ok := usage["completion_tokens"].(float64); ok {
			return int(completionTokens)
		}
	}
	return 0
}
