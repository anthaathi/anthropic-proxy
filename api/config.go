package api

import (
	"anthropic-proxy/auth"
	"anthropic-proxy/config"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// ConfigHandler handles configuration endpoints
type ConfigHandler struct {
	config         *config.Config
	sessionManager *auth.SessionManager
	tokenManager   *auth.TokenManager
}

// NewConfigHandler creates a new config handler
func NewConfigHandler(cfg *config.Config, sessionManager *auth.SessionManager, tokenManager *auth.TokenManager) *ConfigHandler {
	return &ConfigHandler{
		config:         cfg,
		sessionManager: sessionManager,
		tokenManager:   tokenManager,
	}
}

// HandleGetConfig returns configuration info and setup commands
func (h *ConfigHandler) HandleGetConfig(c *gin.Context) {
	userID, err := h.sessionManager.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"type":    "authentication_error",
				"message": "not authenticated",
			},
		})
		return
	}

	// Get base URL from config or construct from request
	baseURL := ""
	if h.config.Spec.Auth != nil && h.config.Spec.Auth.AdminUI.BaseURL != "" {
		baseURL = h.config.Spec.Auth.AdminUI.BaseURL
	} else {
		// Construct from request
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := c.Request.Host
		baseURL = fmt.Sprintf("%s://%s", scheme, host)
	}

	// Get user's tokens
	tokens, err := h.tokenManager.GetUserTokens(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to retrieve tokens",
			},
		})
		return
	}

	// Filter to only valid (non-revoked, non-expired) tokens
	var validTokens []string
	for _, token := range tokens {
		if token.IsValid() {
			validTokens = append(validTokens, token.TokenPrefix+"...")
		}
	}

	// Determine example token text
	exampleToken := "your-api-token"
	if len(validTokens) > 0 {
		exampleToken = validTokens[0]
	}

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Generate configuration commands
	commands := map[string]string{
		"anthropic_base_url":    fmt.Sprintf("export ANTHROPIC_BASE_URL=%s", baseURL),
		"anthropic_auth_token":  fmt.Sprintf("export ANTHROPIC_AUTH_TOKEN=%s  # Use your API key from the tokens page", exampleToken),
		"api_timeout":           "export API_TIMEOUT_MS=600000",
		"disable_traffic":       "export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	}

	// Full configuration block
	fullConfig := fmt.Sprintf(`# Claude Code Configuration for this proxy
export ANTHROPIC_BASE_URL=%s
export ANTHROPIC_AUTH_TOKEN=%s  # Use your API key
export API_TIMEOUT_MS=600000
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1

# Then use Claude Code as normal
# Your requests will be routed through this proxy`,
		baseURL,
		exampleToken,
	)

	c.JSON(http.StatusOK, gin.H{
		"base_url":     baseURL,
		"port":         port,
		"commands":     commands,
		"full_config":  fullConfig,
		"valid_tokens": validTokens,
		"has_tokens":   len(validTokens) > 0,
	})
}
