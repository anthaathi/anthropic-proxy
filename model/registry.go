package model

import (
	"anthropic-proxy/config"
	"anthropic-proxy/logger"
	"fmt"
	"sync"
)

// Registry manages model configurations
type Registry struct {
	models []*config.Model // slice of models to support duplicates with different aliases
	mu     sync.RWMutex
}

// NewRegistry creates a new model registry
func NewRegistry() *Registry {
	return &Registry{
		models: make([]*config.Model, 0),
	}
}

// Load populates the registry with models from configuration
func (r *Registry) Load(models []config.Model) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range models {
		model := &models[i]
		r.models = append(r.models, model)
		logger.Debug("Loaded model",
			"name", model.Name,
			"alias", model.Alias,
			"provider", model.Provider)
	}
}

// GetByName returns a model by its exact name (returns first match)
func (r *Registry) GetByName(name string) (*config.Model, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, model := range r.models {
		if model.Name == name {
			return model, true
		}
	}
	return nil, false
}

// FindMatching returns all models that match the given name (exact or alias match)
func (r *Registry) FindMatching(requestedName string) []*config.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matches []*config.Model

	// First check for exact name matches
	for _, model := range r.models {
		if model.Name == requestedName {
			matches = append(matches, model)
		}
	}

	// If exact matches found, return them
	if len(matches) > 0 {
		return matches
	}

	// Check for alias matches (including wildcards)
	for _, model := range r.models {
		if model.Alias != "" && MatchAlias(model.Alias, requestedName) {
			matches = append(matches, model)
		}
	}

	return matches
}

// GetAll returns all registered models
func (r *Registry) GetAll() []*config.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make([]*config.Model, 0, len(r.models))
	for _, model := range r.models {
		models = append(models, model)
	}
	return models
}

// Size returns the number of models in the registry
func (r *Registry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.models)
}

// String returns a string representation for debugging
func (r *Registry) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return fmt.Sprintf("Registry{models: %d}", len(r.models))
}

// UpdateModels updates the model registry with new configurations
func (r *Registry) UpdateModels(newModels []config.Model) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a new slice of models
	updatedModels := make([]*config.Model, 0, len(newModels))

	for i := range newModels {
		model := &newModels[i]
		updatedModels = append(updatedModels, model)
		logger.Debug("Updated model",
			"name", model.Name,
			"alias", model.Alias,
			"provider", model.Provider)
	}

	// Replace the entire models slice atomically
	r.models = updatedModels

	logger.Info("Model registry updated", "total", len(r.models))
}
