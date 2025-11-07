package provider

import (
	"anthropic-proxy/logger"
	"anthropic-proxy/retry"
	"anthropic-proxy/transform"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles HTTP communication with a provider
type Client struct {
	endpoint     string
	apiKey       string
	providerType string
	httpClient   *http.Client
}

// NewClient creates a new provider client
func NewClient(endpoint, apiKey, providerType string) *Client {
	return &Client{
		endpoint:     endpoint,
		apiKey:       apiKey,
		providerType: providerType,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // 2 minutes for long streaming requests
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// GetProviderType returns the provider type
func (c *Client) GetProviderType() string {
	return c.providerType
}

// ProxyRequest forwards a request to the provider
func (c *Client) ProxyRequest(ctx context.Context, method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	// Convert request format if needed for OpenAI providers
	requestBody := body
	requestPath := path

	if c.providerType == transform.ProviderTypeOpenAI {
		// Convert Anthropic request to OpenAI format
		convertedBody, err := transform.AnthropicToOpenAIRequest(body)
		if err != nil {
			logger.Error("Failed to convert Anthropic request to OpenAI format", "error", err.Error())
			return nil, fmt.Errorf("failed to convert request format: %w", err)
		}
		requestBody = convertedBody
		// OpenAI uses /v1/chat/completions instead of /v1/messages
		requestPath = "/v1/chat/completions"
	}

	url := c.endpoint + requestPath

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set authorization header based on provider type
	if c.providerType == transform.ProviderTypeOpenAI {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	} else {
		// Anthropic
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	req.Header.Set("Content-Type", "application/json")

	// Copy other headers from the original request
	for key, value := range headers {
		// Don't override Authorization, x-api-key, anthropic-version, or Accept-Encoding
		// Skip Accept-Encoding to prevent compressed responses that we can't decompress
		if key != "Authorization" && key != "x-api-key" && key != "anthropic-version" && key != "Accept-Encoding" {
			req.Header.Set(key, value)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// StreamRequest makes a streaming request to the provider
func (c *Client) StreamRequest(ctx context.Context, method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	// Same as ProxyRequest but ensures we don't close the body prematurely
	return c.ProxyRequest(ctx, method, path, body, headers)
}

// ReadBody reads the response body and closes it
func ReadBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return body, nil
}

// ProxyRequestWithRetry forwards a request to the provider with retry logic
func (c *Client) ProxyRequestWithRetry(ctx context.Context, method, path string, body []byte, headers map[string]string, retryConfig *retry.Config, providerName string) (*http.Response, error) {
	var resp *http.Response
	var lastErr error

	err := retry.Do(ctx, retryConfig, func() error {
		// Make the request
		r, err := c.ProxyRequest(ctx, method, path, body, headers)
		if err != nil {
			logger.Debug("Request failed, will retry",
				"provider", providerName,
				"error", err.Error())
			lastErr = err
			return err
		}

		// Check if status code is retriable
		if retry.IsRetriable(r.StatusCode) {
			// Read and discard the body before retrying
			io.ReadAll(r.Body)
			r.Body.Close()

			logger.Debug("Received retriable status code, will retry",
				"provider", providerName,
				"statusCode", r.StatusCode)
			lastErr = fmt.Errorf("retriable status code: %d", r.StatusCode)
			return lastErr
		}

		// Success or non-retriable error
		resp = r
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("request failed after retries: %w", lastErr)
	}

	return resp, nil
}

