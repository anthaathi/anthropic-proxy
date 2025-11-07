package proxy

import (
	"anthropic-proxy/model"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ModelsHandler handles /v1/models requests
type ModelsHandler struct {
	registry *model.Registry
}

// NewModelsHandler creates a new models handler
func NewModelsHandler(registry *model.Registry) *ModelsHandler {
	return &ModelsHandler{
		registry: registry,
	}
}

// HandleListModels handles GET /v1/models
func (h *ModelsHandler) HandleListModels(c *gin.Context) {
	models := h.registry.GetAll()

	// Convert to Anthropic-compatible format
	var modelList []map[string]interface{}
	for _, m := range models {
		modelInfo := map[string]interface{}{
			"id":      m.Name,
			"object":  "model",
			"created": 0, // Could add timestamp if needed
			"owned_by": m.Provider,
		}

		// Add alias if present
		if m.Alias != "" {
			modelInfo["alias"] = m.Alias
		}

		// Add context window
		if m.Context > 0 {
			modelInfo["context_window"] = m.Context
		}

		// Add weight
		modelInfo["weight"] = m.GetWeight()

		modelList = append(modelList, modelInfo)
	}

	response := map[string]interface{}{
		"object": "list",
		"data":   modelList,
	}

	c.JSON(http.StatusOK, response)
}
