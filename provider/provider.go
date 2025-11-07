package provider

import (
	"anthropic-proxy/config"
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
			Endpoint: providerConfig.Endpoint,
			APIKey:   providerConfig.APIKey,
			Client:   NewClient(providerConfig.Endpoint, providerConfig.APIKey),
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
