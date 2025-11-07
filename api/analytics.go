package api

import (
	"anthropic-proxy/analytics"
	"anthropic-proxy/auth"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AnalyticsHandler handles analytics endpoints
type AnalyticsHandler struct {
	analyticsService *analytics.Service
	sessionManager   *auth.SessionManager
}

// NewAnalyticsHandler creates a new analytics handler
func NewAnalyticsHandler(analyticsService *analytics.Service, sessionManager *auth.SessionManager) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: analyticsService,
		sessionManager:   sessionManager,
	}
}

// HandleGetUserAnalytics returns analytics for the current user
func (h *AnalyticsHandler) HandleGetUserAnalytics(c *gin.Context) {
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

	// Get limit from query params (default 100)
	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	analytics, err := h.analyticsService.GetUserAnalytics(userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to retrieve analytics",
			},
		})
		return
	}

	c.JSON(http.StatusOK, analytics)
}
