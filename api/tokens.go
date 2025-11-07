package api

import (
	"anthropic-proxy/auth"
	"anthropic-proxy/database"
	"anthropic-proxy/logger"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TokenHandler handles token management endpoints
type TokenHandler struct {
	tokenManager   *auth.TokenManager
	sessionManager *auth.SessionManager
	repo           *database.Repository
}

// NewTokenHandler creates a new token handler
func NewTokenHandler(tokenManager *auth.TokenManager, sessionManager *auth.SessionManager, repo *database.Repository) *TokenHandler {
	return &TokenHandler{
		tokenManager:   tokenManager,
		sessionManager: sessionManager,
		repo:           repo,
	}
}

// CreateTokenRequest represents a request to create a new token
type CreateTokenRequest struct {
	Name          string `json:"name" binding:"required"`
	ExpiresInDays int    `json:"expiresInDays"`
}

// HandleListTokens lists all tokens for the authenticated user
func (h *TokenHandler) HandleListTokens(c *gin.Context) {
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

	tokens, err := h.tokenManager.GetUserTokens(userID)
	if err != nil {
		logger.Error("Failed to list tokens", "user_id", userID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to retrieve tokens",
			},
		})
		return
	}

	// Format tokens for response (hide sensitive data)
	response := make([]gin.H, len(tokens))
	for i, token := range tokens {
		response[i] = gin.H{
			"id":           token.ID,
			"name":         token.Name,
			"prefix":       "sk-" + token.TokenPrefix + "-***",
			"created_at":   token.CreatedAt,
			"last_used_at": token.LastUsedAt,
			"expires_at":   token.ExpiresAt,
			"revoked":      token.Revoked,
			"is_valid":     token.IsValid(),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tokens": response,
	})
}

// HandleCreateToken creates a new API token
func (h *TokenHandler) HandleCreateToken(c *gin.Context) {
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

	var req CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request",
				"message": "invalid request body: " + err.Error(),
			},
		})
		return
	}

	// Validate expiration days
	if req.ExpiresInDays < 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request",
				"message": "expiresInDays must be positive or 0 (never expires)",
			},
		})
		return
	}

	// Check token limit (max 50 tokens per user)
	count, err := h.repo.CountUserTokens(userID)
	if err != nil {
		logger.Error("Failed to count user tokens", "user_id", userID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to create token",
			},
		})
		return
	}

	if count >= 50 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "limit_exceeded",
				"message": "maximum number of tokens (50) reached",
			},
		})
		return
	}

	// Generate token
	tokenString, token, err := h.tokenManager.GenerateToken(userID, req.Name, req.ExpiresInDays)
	if err != nil {
		logger.Error("Failed to generate token", "user_id", userID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to create token",
			},
		})
		return
	}

	logger.Info("Created new token", "user_id", userID, "token_id", token.ID, "name", req.Name)

	c.JSON(http.StatusCreated, gin.H{
		"id":         token.ID,
		"token":      tokenString, // Only returned once!
		"name":       token.Name,
		"prefix":     "sk-" + token.TokenPrefix + "-***",
		"created_at": token.CreatedAt,
		"expires_at": token.ExpiresAt,
		"message":    "Save this token securely. It will not be shown again.",
	})
}

// HandleRevokeToken revokes a token
func (h *TokenHandler) HandleRevokeToken(c *gin.Context) {
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

	tokenID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request",
				"message": "invalid token ID",
			},
		})
		return
	}

	// Get token to verify ownership
	token, err := h.repo.GetTokenByID(uint(tokenID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"type":    "not_found",
					"message": "token not found",
				},
			})
			return
		}
		logger.Error("Failed to get token", "token_id", tokenID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to revoke token",
			},
		})
		return
	}

	// Verify token belongs to the authenticated user
	if token.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"type":    "forbidden",
				"message": "you can only revoke your own tokens",
			},
		})
		return
	}

	// Revoke token
	if err := h.tokenManager.RevokeToken(uint(tokenID)); err != nil {
		logger.Error("Failed to revoke token", "token_id", tokenID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to revoke token",
			},
		})
		return
	}

	logger.Info("Revoked token", "user_id", userID, "token_id", tokenID)

	c.JSON(http.StatusOK, gin.H{
		"message": "token revoked successfully",
	})
}

// HandleUpdateToken updates a token (currently only supports renaming)
func (h *TokenHandler) HandleUpdateToken(c *gin.Context) {
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

	tokenID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request",
				"message": "invalid token ID",
			},
		})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request",
				"message": "invalid request body",
			},
		})
		return
	}

	// Get token to verify ownership
	token, err := h.repo.GetTokenByID(uint(tokenID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"type":    "not_found",
					"message": "token not found",
				},
			})
			return
		}
		logger.Error("Failed to get token", "token_id", tokenID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to update token",
			},
		})
		return
	}

	// Verify token belongs to the authenticated user
	if token.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"type":    "forbidden",
				"message": "you can only update your own tokens",
			},
		})
		return
	}

	// Update token name
	token.Name = req.Name
	if err := h.repo.UpdateToken(token); err != nil {
		logger.Error("Failed to update token", "token_id", tokenID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to update token",
			},
		})
		return
	}

	logger.Info("Updated token", "user_id", userID, "token_id", tokenID, "new_name", req.Name)

	c.JSON(http.StatusOK, gin.H{
		"message": "token updated successfully",
		"token": gin.H{
			"id":   token.ID,
			"name": token.Name,
		},
	})
}
