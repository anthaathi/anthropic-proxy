package config

import (
	"fmt"
	"net/url"
)

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Check if we have at least one provider
	if len(c.Spec.Providers) == 0 {
		return fmt.Errorf("at least one provider must be configured")
	}

	// Validate providers
	for name, provider := range c.Spec.Providers {
		if err := validateProvider(name, provider); err != nil {
			return err
		}
	}

	// Check if we have at least one model
	if len(c.Spec.Models) == 0 {
		return fmt.Errorf("at least one model must be configured")
	}

	// Validate models
	for i, model := range c.Spec.Models {
		if err := c.validateModel(i, model); err != nil {
			return err
		}
	}

	// Check if we have at least one API key
	if len(c.Spec.APIKeys) == 0 {
		return fmt.Errorf("at least one API key must be configured")
	}

	// Validate API keys are not empty
	for i, key := range c.Spec.APIKeys {
		if key == "" {
			return fmt.Errorf("API key at index %d is empty", i)
		}
	}

	return nil
}

// validateProvider checks if a provider configuration is valid
func validateProvider(name string, p Provider) error {
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	// Validate provider type
	providerType := p.GetType()
	if providerType != "anthropic" && providerType != "openai" {
		return fmt.Errorf("provider %s: type must be either 'anthropic' or 'openai', got '%s'", name, providerType)
	}

	if p.Endpoint == "" {
		return fmt.Errorf("provider %s: endpoint cannot be empty", name)
	}

	// Validate endpoint is a valid URL
	if _, err := url.Parse(p.Endpoint); err != nil {
		return fmt.Errorf("provider %s: invalid endpoint URL: %w", name, err)
	}

	if p.APIKey == "" {
		return fmt.Errorf("provider %s: API key cannot be empty", name)
	}

	return nil
}

// validateModel checks if a model configuration is valid
func (c *Config) validateModel(index int, m Model) error {
	if m.Name == "" {
		return fmt.Errorf("model at index %d: name cannot be empty", index)
	}

	if m.Provider == "" {
		return fmt.Errorf("model %s: provider cannot be empty", m.Name)
	}

	// Check if the referenced provider exists
	if _, exists := c.Spec.Providers[m.Provider]; !exists {
		return fmt.Errorf("model %s: provider %s does not exist", m.Name, m.Provider)
	}

	if m.Context <= 0 {
		return fmt.Errorf("model %s: context must be positive", m.Name)
	}

	if m.Weight < 0 {
		return fmt.Errorf("model %s: weight cannot be negative", m.Name)
	}

	return nil
}
