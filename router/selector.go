package router

import (
	"anthropic-proxy/config"
	"anthropic-proxy/logger"
	"anthropic-proxy/metrics"
	"anthropic-proxy/model"
	"anthropic-proxy/provider"
	"sort"
)

// Selector selects the best provider for a request
type Selector struct {
	modelRegistry *model.Registry
	providerMgr   *provider.Manager
	tracker       *metrics.Tracker
	tpsThreshold  float64
}

// NewSelector creates a new provider selector
func NewSelector(modelRegistry *model.Registry, providerMgr *provider.Manager, tracker *metrics.Tracker) *Selector {
	return &Selector{
		modelRegistry: modelRegistry,
		providerMgr:   providerMgr,
		tracker:       tracker,
		tpsThreshold:  40.0, // Minimum TPS requirement
	}
}

// SelectProviders returns an ordered list of providers to try for a given model
func (s *Selector) SelectProviders(requestedModel string) ([]*ProviderChoice, error) {
	// Find models matching the requested name (exact or alias match)
	matchingModels := s.modelRegistry.FindMatching(requestedModel)

	if len(matchingModels) == 0 {
		return nil, ErrNoModelFound
	}

	// Build list of ALL provider choices (without TPS filtering initially)
	var allChoices []*ProviderChoice
	var goodChoices []*ProviderChoice // Choices meeting TPS threshold

	for _, modelConfig := range matchingModels {
		prov, exists := s.providerMgr.Get(modelConfig.Provider)
		if !exists {
			continue
		}

		// Get TPS for this provider-model combination
		tps := s.tracker.GetTPS(prov.Name, modelConfig.Name)

		choice := &ProviderChoice{
			Provider:    prov,
			Model:       modelConfig,
			Weight:      modelConfig.GetWeight(),
			TPS:         tps,
			ActualModel: modelConfig.Name, // The actual model name to use with the provider
		}

		allChoices = append(allChoices, choice)

		// Separately track choices that meet TPS threshold
		// Exception: if TPS is 0 (no data yet), give it a chance
		if tps == 0 || tps >= s.tpsThreshold {
			goodChoices = append(goodChoices, choice)
		}
	}

	if len(allChoices) == 0 {
		return nil, ErrNoProvidersAvailable
	}

	// Use choices that meet threshold if available, otherwise fall back to all choices
	var choices []*ProviderChoice
	if len(goodChoices) > 0 {
		choices = goodChoices
	} else {
		// Fallback: use all available providers even if they don't meet threshold
		choices = allChoices
		logger.Warn("No providers meet TPS threshold, falling back to fastest available provider",
			"tpsThreshold", s.tpsThreshold)
	}

	// Sort by weight (descending), then TPS (descending)
	sort.Slice(choices, func(i, j int) bool {
		// First priority: weight
		if choices[i].Weight != choices[j].Weight {
			return choices[i].Weight > choices[j].Weight
		}
		// Tiebreaker: TPS
		return choices[i].TPS > choices[j].TPS
	})

	return choices, nil
}

// ProviderChoice represents a provider option with its score
type ProviderChoice struct {
	Provider    *provider.Provider
	Model       *config.Model
	Weight      int
	TPS         float64
	ActualModel string // The actual model name to send to the provider
}
