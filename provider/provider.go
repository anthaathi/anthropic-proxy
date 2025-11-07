package provider

import (
	"anthropic-proxy/config"
	"anthropic-proxy/logger"
	"sync"
)

// Manager manages provider configurations and health
type Manager struct {
	providers map[string]*Provider
	mu        sync.RWMutex
}

// Provider represents a backend provider with its configuration
type Provider struct {
	Name     string
	Type     string // "anthropic" or "openai"
	Endpoint string
	APIKey   string
	Client   *Client
}

// NewManager creates a new provider manager
func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]*Provider),
	}
}

// Load initializes providers from configuration
func (m *Manager) Load(providers map[string]config.Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, providerConfig := range providers {
		provider := &Provider{
			Name:     name,
			Type:     providerConfig.GetType(),
			Endpoint: providerConfig.Endpoint,
			APIKey:   providerConfig.APIKey,
			Client:   NewClient(providerConfig.Endpoint, providerConfig.APIKey, providerConfig.GetType()),
		}
		m.providers[name] = provider
	}
}

// Get returns a provider by name
func (m *Manager) Get(name string) (*Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	provider, exists := m.providers[name]
	return provider, exists
}

// GetAll returns all providers
func (m *Manager) GetAll() []*Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	providers := make([]*Provider, 0, len(m.providers))
	for _, provider := range m.providers {
		providers = append(providers, provider)
	}
	return providers
}

// Size returns the number of providers
func (m *Manager) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.providers)
}

// UpdateProviders updates the provider list with new configurations
func (m *Manager) UpdateProviders(newProviders map[string]config.Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track which providers are still present
	activeProviders := make(map[string]bool)

	// Update existing providers and add new ones
	for name, providerConfig := range newProviders {
		activeProviders[name] = true
		providerType := providerConfig.GetType()

		if existingProvider, exists := m.providers[name]; exists {
			// Check if provider configuration actually changed
			if existingProvider.Type != providerType || existingProvider.Endpoint != providerConfig.Endpoint || existingProvider.APIKey != providerConfig.APIKey {
				logger.Info("Updating provider configuration",
					"provider", name,
					"oldEndpoint", existingProvider.Endpoint,
					"newEndpoint", providerConfig.Endpoint,
					"type", providerType)

				// Create new provider with updated config
				updatedProvider := &Provider{
					Name:     name,
					Type:     providerType,
					Endpoint: providerConfig.Endpoint,
					APIKey:   providerConfig.APIKey,
					Client:   NewClient(providerConfig.Endpoint, providerConfig.APIKey, providerType),
				}
				m.providers[name] = updatedProvider
				logger.Info("Provider updated successfully", "provider", name)
			}
		} else {
			// Add new provider
			provider := &Provider{
				Name:     name,
				Type:     providerType,
				Endpoint: providerConfig.Endpoint,
				APIKey:   providerConfig.APIKey,
				Client:   NewClient(providerConfig.Endpoint, providerConfig.APIKey, providerType),
			}
			m.providers[name] = provider
			logger.Info("Provider added successfully", "provider", name)
		}
	}

	// Remove providers that are no longer in the config
	for name := range m.providers {
		if !activeProviders[name] {
			delete(m.providers, name)
			logger.Info("Provider removed", "provider", name)
		}
	}

	logger.Info("Provider update completed", "total", len(m.providers))
}
