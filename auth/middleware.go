package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Middleware creates a Gin middleware for API key authentication
func Middleware(validKeys []string) gin.HandlerFunc {
	// Create a map for O(1) lookup
	keyMap := make(map[string]bool, len(validKeys))
	for _, key := range validKeys {
		keyMap[key] = true
	}

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

		// Validate token
		if !keyMap[token] {
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

// extractBearerToken extracts the token from "Bearer <token>" format
func extractBearerToken(authHeader string) string {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
