package provider

import (
	"anthropic-proxy/logger"
	"anthropic-proxy/retry"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles HTTP communication with a provider
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new provider client
func NewClient(endpoint, apiKey string) *Client {
	return &Client{
		endpoint: endpoint,
		apiKey:   apiKey,
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

// ProxyRequest forwards a request to the provider
func (c *Client) ProxyRequest(ctx context.Context, method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	url := c.endpoint + path

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set authorization header with provider's API key
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Copy other headers from the original request
	for key, value := range headers {
		// Don't override Authorization
		if key != "Authorization" {
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

