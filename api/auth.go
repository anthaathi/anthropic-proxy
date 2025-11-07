package api

import (
	"anthropic-proxy/auth"
	"anthropic-proxy/database"
	"anthropic-proxy/logger"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	oidcClient     *auth.OIDCClient
	sessionManager *auth.SessionManager
	repo           *database.Repository
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(oidcClient *auth.OIDCClient, sessionManager *auth.SessionManager, repo *database.Repository) *AuthHandler {
	return &AuthHandler{
		oidcClient:     oidcClient,
		sessionManager: sessionManager,
		repo:           repo,
	}
}

// HandleLogin initiates the OAuth login flow
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	// Generate auth URL with state and nonce
	authURL, state, nonce, err := h.oidcClient.GenerateAuthURL()
	if err != nil {
		logger.Error("Failed to generate auth URL", "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to initiate login",
			},
		})
		return
	}

	// Store state and nonce in session
	if err := h.sessionManager.SetOAuthState(c, state, nonce); err != nil {
		logger.Error("Failed to save OAuth state", "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to initiate login",
			},
		})
		return
	}

	logger.Debug("Redirecting to OAuth provider", "url", authURL)

	// Redirect to OAuth provider
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// HandleCallback handles the OAuth callback
func (h *AuthHandler) HandleCallback(c *gin.Context) {
	// Get authorization code and state from query params
	code := c.Query("code")
	returnedState := c.Query("state")

	if code == "" || returnedState == "" {
		logger.Warn("OAuth callback missing code or state")
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"error": "Invalid OAuth callback: missing code or state",
		})
		return
	}

	// Retrieve and verify state from session
	storedState, nonce, err := h.sessionManager.GetAndClearOAuthState(c)
	if err != nil {
		logger.Error("Failed to retrieve OAuth state", "error", err.Error())
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"error": "Invalid OAuth state",
		})
		return
	}

	if storedState != returnedState {
		logger.Warn("OAuth state mismatch", "expected", storedState, "got", returnedState)
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"error": "OAuth state mismatch (possible CSRF attack)",
		})
		return
	}

	// Exchange authorization code for tokens
	oauth2Token, err := h.oidcClient.ExchangeCode(c.Request.Context(), code)
	if err != nil {
		logger.Error("Failed to exchange code", "error", err.Error())
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Failed to complete authentication",
		})
		return
	}

	// Verify ID token with nonce
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if ok && rawIDToken != "" {
		_, err := h.oidcClient.VerifyIDToken(c.Request.Context(), rawIDToken, nonce)
		if err != nil {
			logger.Warn("ID token verification failed", "error", err.Error())
			// Continue anyway, as some providers may not support nonce
		}
	}

	// Get user info from ID token
	userInfo, err := h.oidcClient.GetUserInfo(c.Request.Context(), oauth2Token)
	if err != nil {
		logger.Error("Failed to get user info", "error", err.Error())
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Failed to retrieve user information",
		})
		return
	}

	logger.Info("User authenticated via OAuth", "email", userInfo.Email, "name", userInfo.Name)

	// Find or create user in database
	user, err := h.findOrCreateUser(userInfo)
	if err != nil {
		logger.Error("Failed to create user", "error", err.Error())
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Failed to create user account",
		})
		return
	}

	// Update last login time
	if err := h.repo.UpdateUserLastLogin(user.ID); err != nil {
		logger.Warn("Failed to update last login time", "user_id", user.ID, "error", err.Error())
	}

	// Create session
	if err := h.sessionManager.CreateSession(c, user.ID, user.Email, user.Name); err != nil {
		logger.Error("Failed to create session", "error", err.Error())
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Failed to create session",
		})
		return
	}

	logger.Info("User logged in successfully", "user_id", user.ID, "email", user.Email)

	// Redirect to admin UI
	c.Redirect(http.StatusTemporaryRedirect, "/admin")
}

// HandleLogout logs the user out
func (h *AuthHandler) HandleLogout(c *gin.Context) {
	// Destroy session
	if err := h.sessionManager.DestroySession(c); err != nil {
		logger.Error("Failed to destroy session", "error", err.Error())
	}

	logger.Debug("User logged out")

	// Redirect to login page or return JSON
	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{
			"message": "logged out successfully",
		})
	} else {
		c.Redirect(http.StatusTemporaryRedirect, "/admin/login")
	}
}

// findOrCreateUser finds an existing user or creates a new one
func (h *AuthHandler) findOrCreateUser(userInfo *auth.UserInfo) (*database.User, error) {
	// Try to find user by email
	user, err := h.repo.GetUserByEmail(userInfo.Email)
	if err == nil {
		// User found, return it
		return user, nil
	}

	if !errors.Is(err, database.ErrUserNotFound) {
		// Unexpected error
		return nil, err
	}

	// User not found, check if this is the first user
	userCount, err := h.repo.GetUserCount()
	if err != nil {
		logger.Error("Failed to get user count", "error", err.Error())
		// Continue anyway, just won't be admin
		userCount = 1
	}

	isFirstUser := userCount == 0

	// User not found, create new user
	now := time.Now().UTC()
	user = &database.User{
		Email:          userInfo.Email,
		Name:           userInfo.Name,
		ProviderUserID: userInfo.Sub,
		Provider:       "openid", // Generic provider name
		IsAdmin:        isFirstUser, // First user becomes admin
		LastLoginAt:    &now,
	}

	if err := h.repo.CreateUser(user); err != nil {
		return nil, err
	}

	if isFirstUser {
		logger.Info("Created first user with admin privileges", "user_id", user.ID, "email", user.Email)
	} else {
		logger.Info("Created new user", "user_id", user.ID, "email", user.Email)
	}

	return user, nil
}

// HandleGetUser returns the current user's information
func (h *AuthHandler) HandleGetUser(c *gin.Context) {
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

	user, err := h.repo.GetUserByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"type":    "not_found",
					"message": "user not found",
				},
			})
			return
		}
		logger.Error("Failed to get user", "user_id", userID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to retrieve user",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            user.ID,
		"email":         user.Email,
		"name":          user.Name,
		"is_admin":      user.IsAdmin,
		"created_at":    user.CreatedAt,
		"last_login_at": user.LastLoginAt,
	})
}
