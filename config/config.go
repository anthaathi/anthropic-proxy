package config

// Config represents the root configuration structure
type Config struct {
	Spec Spec `yaml:"spec"`
}

// Spec contains the main configuration sections
type Spec struct {
	Providers map[string]Provider `yaml:"providers"`
	Models    []Model             `yaml:"models"`
	APIKeys   []string            `yaml:"apiKeys"`
	Retry     *RetryConfig        `yaml:"retry,omitempty"`
}

// Provider represents a backend provider configuration
type Provider struct {
	Endpoint string `yaml:"endpoint"`
	APIKey   string `yaml:"apiKey"`
}

// Model represents a model configuration
type Model struct {
	Name     string `yaml:"name"`
	Alias    string `yaml:"alias"`
	Context  int    `yaml:"context"`
	Provider string `yaml:"provider"`
	Weight   int    `yaml:"weight"`
}

// GetWeight returns the weight with a default of 1 if not set
func (m *Model) GetWeight() int {
	if m.Weight <= 0 {
		return 1
	}
	return m.Weight
}

// RetryConfig represents retry configuration
type RetryConfig struct {
	MaxRetries        int    `yaml:"maxRetries"`
	InitialDelay      string `yaml:"initialDelay"`
	MaxDelay          string `yaml:"maxDelay"`
	BackoffMultiplier float64 `yaml:"backoffMultiplier"`
}
