package auth

import (
	"anthropic-proxy/logger"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// Service manages dynamic API key authentication
type Service struct {
	keyMap       map[string]bool
	keyList      []string // Keep order for potential display
	mu           sync.RWMutex
	middleware   gin.HandlerFunc
	tokenManager *TokenManager // Database token manager (optional)
}

// NewService creates a new dynamic authentication service
func NewService() *Service {
	s := &Service{
		keyMap:  make(map[string]bool),
		keyList: make([]string, 0),
	}

	// Create middleware that calls the service's validate method
	s.middleware = s.createMiddleware()
	return s
}

// SetTokenManager sets the token manager for database-backed authentication
func (s *Service) SetTokenManager(tm *TokenManager) {
	s.tokenManager = tm
	logger.Info("Token manager enabled for database authentication")
}

// UpdateKeys updates the list of valid API keys
func (s *Service) UpdateKeys(newKeys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create new key map
	newKeyMap := make(map[string]bool, len(newKeys))
	for _, key := range newKeys {
		newKeyMap[key] = true
	}

	// Update key map and list
	s.keyMap = newKeyMap
	s.keyList = make([]string, len(newKeys))
	copy(s.keyList, newKeys)

	logger.Info("API keys updated", "count", len(newKeys))
}

// GetKeys returns the current list of API keys
func (s *Service) GetKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, len(s.keyList))
	copy(keys, s.keyList)
	return keys
}

// ValidateKey checks if a key is valid (supports both static keys and database tokens)
func (s *Service) ValidateKey(key string) bool {
	// First check static keys (fast path, no database lookup)
	s.mu.RLock()
	isStaticKey := s.keyMap[key]
	s.mu.RUnlock()

	if isStaticKey {
		return true
	}

	// If token manager is available, check database tokens
	if s.tokenManager != nil {
		_, err := s.tokenManager.ValidateToken(key)
		return err == nil
	}

	return false
}

// Middleware returns the authentication middleware
func (s *Service) Middleware() gin.HandlerFunc {
	return s.middleware
}

// createMiddleware creates the middleware function
func (s *Service) createMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "missing authorization header",
				},
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		token := extractBearerToken(authHeader)
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "invalid authorization header format, expected 'Bearer <token>'",
				},
			})
			c.Abort()
			return
		}

		// Validate token using the service
		if !s.ValidateKey(token) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "invalid API key",
				},
			})
			c.Abort()
			return
		}

		// Token is valid, continue
		c.Next()
	}
}