package proxy

import (
	"anthropic-proxy/metrics"
	"anthropic-proxy/provider"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthHandler handles /health requests
type HealthHandler struct {
	providerMgr  *provider.Manager
	tracker      *metrics.Tracker
	errorTracker *metrics.ErrorTracker
	tpsThreshold float64
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(providerMgr *provider.Manager, tracker *metrics.Tracker, errorTracker *metrics.ErrorTracker) *HealthHandler {
	return &HealthHandler{
		providerMgr:  providerMgr,
		tracker:      tracker,
		errorTracker: errorTracker,
		tpsThreshold: 40.0,
	}
}

// HandleHealth handles GET /health
func (h *HealthHandler) HandleHealth(c *gin.Context) {
	providers := h.providerMgr.GetAll()

	healthyCount := 0
	providerStatus := make(map[string]interface{})

	for _, prov := range providers {
		errorRate := h.errorTracker.GetErrorRate(prov.Name)
		isHealthy := errorRate < 0.5 // Less than 50% error rate

		status := map[string]interface{}{
			"healthy":    isHealthy,
			"error_rate": errorRate,
		}

		if isHealthy {
			healthyCount++
		}

		providerStatus[prov.Name] = status
	}

	// Overall health status
	overallHealthy := healthyCount > 0

	statusCode := http.StatusOK
	if !overallHealthy {
		statusCode = http.StatusServiceUnavailable
	}

	response := map[string]interface{}{
		"status":           getStatusString(overallHealthy),
		"healthy_providers": healthyCount,
		"total_providers":   len(providers),
		"providers":         providerStatus,
		"tps_threshold":     h.tpsThreshold,
	}

	c.JSON(statusCode, response)
}

// getStatusString converts boolean to status string
func getStatusString(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "unhealthy"
}
