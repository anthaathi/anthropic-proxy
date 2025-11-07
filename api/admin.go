package api

import (
	"anthropic-proxy/analytics"
	"anthropic-proxy/auth"
	"anthropic-proxy/database"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AdminHandler handles admin-only endpoints
type AdminHandler struct {
	analyticsService *analytics.Service
	sessionManager   *auth.SessionManager
	repo             *database.Repository
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(analyticsService *analytics.Service, sessionManager *auth.SessionManager, repo *database.Repository) *AdminHandler {
	return &AdminHandler{
		analyticsService: analyticsService,
		sessionManager:   sessionManager,
		repo:             repo,
	}
}

// RequireAdmin middleware checks if user is admin
func (h *AdminHandler) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := h.sessionManager.GetUserID(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "not authenticated",
				},
			})
			c.Abort()
			return
		}

		user, err := h.repo.GetUserByID(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"type":    "server_error",
					"message": "failed to verify admin status",
				},
			})
			c.Abort()
			return
		}

		if !user.IsAdmin {
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"type":    "permission_error",
					"message": "admin privileges required",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// HandleGetAllUsers returns all users with their usage stats
func (h *AdminHandler) HandleGetAllUsers(c *gin.Context) {
	users, err := h.analyticsService.GetAllUsersAnalytics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to retrieve users",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
	})
}

// HandleGetUserAnalytics returns analytics for a specific user (admin only)
func (h *AdminHandler) HandleGetUserAnalytics(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request",
				"message": "invalid user ID",
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

	analytics, err := h.analyticsService.GetUserAnalytics(uint(userID), limit)
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

// HandlePromoteUser promotes a user to admin
func (h *AdminHandler) HandlePromoteUser(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request",
				"message": "invalid user ID",
			},
		})
		return
	}

	if err := h.repo.PromoteToAdmin(uint(userID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to promote user",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "user promoted to admin successfully",
	})
}

// HandleDemoteUser removes admin privileges from a user
func (h *AdminHandler) HandleDemoteUser(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request",
				"message": "invalid user ID",
			},
		})
		return
	}

	// Prevent demoting yourself
	currentUserID, err := h.sessionManager.GetUserID(c)
	if err == nil && currentUserID == uint(userID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request",
				"message": "cannot demote yourself",
			},
		})
		return
	}

	if err := h.repo.DemoteFromAdmin(uint(userID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to demote user",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "admin privileges removed successfully",
	})
}

// HandleGetSystemAnalytics returns system-wide analytics
func (h *AdminHandler) HandleGetSystemAnalytics(c *gin.Context) {
	users, err := h.analyticsService.GetAllUsersAnalytics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "server_error",
				"message": "failed to retrieve system analytics",
			},
		})
		return
	}

	// Aggregate totals
	var totalRequests, totalTokens int64
	var totalUsers int
	for _, user := range users {
		totalRequests += user.TotalRequests
		totalTokens += user.TotalTokens
		totalUsers++
	}

	c.JSON(http.StatusOK, gin.H{
		"total_users":    totalUsers,
		"total_requests": totalRequests,
		"total_tokens":   totalTokens,
		"users":          users,
	})
}
