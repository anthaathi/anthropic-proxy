package proxy

import (
	"anthropic-proxy/logger"
	"anthropic-proxy/router"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CountTokensHandler handles token counting requests
type CountTokensHandler struct {
	fallbackMgr *router.FallbackManager
}

// NewCountTokensHandler creates a new count tokens handler
func NewCountTokensHandler(fallbackMgr *router.FallbackManager) *CountTokensHandler {
	return &CountTokensHandler{
		fallbackMgr: fallbackMgr,
	}
}

// HandleCountTokens handles POST /v1/messages/count_tokens requests
func (h *CountTokensHandler) HandleCountTokens(c *gin.Context) {
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

	// Check if thinking is enabled in the request
	thinkingEnabled := false
	if thinking, ok := requestBody["thinking"].(map[string]interface{}); ok {
		if thinkingType, ok := thinking["type"].(string); ok && thinkingType == "enabled" {
			thinkingEnabled = true
		}
	}

	// Get ordered list of providers to try
	providerChoices, err := h.fallbackMgr.GetOrderedProviders(modelName, thinkingEnabled)
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

		logger.Debug("Trying provider for token counting",
			"provider", choice.Provider.Name,
			"model", choice.ActualModel,
			"alias", modelName)

		// Make the request to count tokens
		resp, err := choice.Provider.Client.ProxyRequest(c.Request.Context(), "POST", "/v1/messages/count_tokens", updatedBody, headers)

		// Check for network errors
		if err != nil {
			proxyErr := ClassifyError(0, err, choice.Provider.Name)
			LogError(proxyErr)
			lastError = proxyErr
			continue
		}
		defer resp.Body.Close()

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			proxyErr := ClassifyError(resp.StatusCode, nil, choice.Provider.Name)
			LogError(proxyErr)
			lastError = proxyErr
			continue
		}

		// Success! Read and return response
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Error reading response from provider",
				"provider", choice.Provider.Name,
				"error", err.Error())
			lastError = ClassifyError(0, err, choice.Provider.Name)
			continue
		}

		// Parse the response to log token count
		var tokenResponse map[string]interface{}
		if err := json.Unmarshal(responseBody, &tokenResponse); err == nil {
			if inputTokens, ok := tokenResponse["input_tokens"].(float64); ok {
				logger.Debug("Token count succeeded with provider",
					"provider", choice.Provider.Name,
					"inputTokens", int(inputTokens))
			}
		}

		// Copy response headers
		for key, values := range resp.Header {
			if len(values) > 0 {
				c.Header(key, values[0])
			}
		}

		// Send response
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), responseBody)
		return
	}

	// All providers failed
	logger.Error("All providers failed for token counting",
		"model", modelName)

	if lastError != nil {
		c.JSON(http.StatusBadGateway, CreateErrorResponse(502, "all_providers_failed",
			"All providers failed: "+lastError.Message))
	} else {
		c.JSON(http.StatusBadGateway, CreateErrorResponse(502, "all_providers_failed",
			"All providers failed to process the token counting request"))
	}
}
