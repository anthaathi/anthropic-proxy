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

	// Validate authentication configuration
	if err := c.validateAuth(); err != nil {
		return err
	}

	return nil
}

// validateAuth validates authentication configuration
func (c *Config) validateAuth() error {
	// Check if we have at least one authentication method
	hasAPIKeys := len(c.Spec.APIKeys) > 0
	hasAuth := c.Spec.Auth != nil

	if !hasAPIKeys && !hasAuth {
		return fmt.Errorf("at least one authentication method must be configured (apiKeys or auth section)")
	}

	// Validate old-style API keys if present
	if hasAPIKeys {
		for i, key := range c.Spec.APIKeys {
			if key == "" {
				return fmt.Errorf("API key at index %d is empty", i)
			}
		}
	}

	// Validate auth configuration if present
	if hasAuth {
		// Validate static keys if present
		for i, key := range c.Spec.Auth.StaticKeys {
			if key == "" {
				return fmt.Errorf("auth.staticKeys: key at index %d is empty", i)
			}
		}

		// Validate database configuration if provided
		if c.Spec.Auth.Database.Driver != "" || c.Spec.Auth.Database.DSN != "" {
			if err := validateDatabaseConfig(c.Spec.Auth.Database); err != nil {
				return fmt.Errorf("auth.database: %w", err)
			}
		}

		// Validate OpenID configuration if enabled
		if c.Spec.Auth.OpenID.Enabled {
			if err := validateOpenIDConfig(c.Spec.Auth.OpenID); err != nil {
				return fmt.Errorf("auth.openid: %w", err)
			}
		}

		// Validate admin UI configuration if enabled
		if c.Spec.Auth.AdminUI.Enabled {
			if err := validateAdminUIConfig(c.Spec.Auth.AdminUI); err != nil {
				return fmt.Errorf("auth.adminUI: %w", err)
			}

			// Admin UI requires OpenID and database
			if !c.Spec.Auth.OpenID.Enabled {
				return fmt.Errorf("auth.adminUI: requires OpenID to be enabled")
			}
			if c.Spec.Auth.Database.Driver == "" {
				return fmt.Errorf("auth.adminUI: requires database to be configured")
			}
		}
	}

	return nil
}

// validateDatabaseConfig validates database configuration
func validateDatabaseConfig(cfg DatabaseConfig) error {
	if cfg.Driver == "" {
		return fmt.Errorf("driver cannot be empty")
	}

	if cfg.Driver != "sqlite" && cfg.Driver != "sqlite3" && cfg.Driver != "postgres" && cfg.Driver != "postgresql" {
		return fmt.Errorf("unsupported driver: %s (supported: sqlite, postgres)", cfg.Driver)
	}

	if cfg.DSN == "" {
		return fmt.Errorf("dsn cannot be empty")
	}

	if cfg.MaxConns < 0 {
		return fmt.Errorf("maxConns cannot be negative")
	}

	return nil
}

// validateOpenIDConfig validates OpenID configuration
func validateOpenIDConfig(cfg OpenIDConfig) error {
	if cfg.ClientID == "" {
		return fmt.Errorf("clientId cannot be empty")
	}

	if cfg.ClientSecret == "" {
		return fmt.Errorf("clientSecret cannot be empty")
	}

	if cfg.RedirectURL == "" {
		return fmt.Errorf("redirectUrl cannot be empty")
	}

	// Validate redirect URL format
	if _, err := url.Parse(cfg.RedirectURL); err != nil {
		return fmt.Errorf("invalid redirectUrl: %w", err)
	}

	// Check that either issuer or manual endpoints are provided
	if cfg.Issuer == "" && (cfg.AuthEndpoint == "" || cfg.TokenEndpoint == "") {
		return fmt.Errorf("either issuer (for auto-discovery) or manual endpoints (authEndpoint, tokenEndpoint) must be provided")
	}

	// Validate issuer URL if provided
	if cfg.Issuer != "" {
		if _, err := url.Parse(cfg.Issuer); err != nil {
			return fmt.Errorf("invalid issuer URL: %w", err)
		}
	}

	return nil
}

// validateAdminUIConfig validates admin UI configuration
func validateAdminUIConfig(cfg AdminUIConfig) error {
	if cfg.SessionSecret == "" {
		return fmt.Errorf("sessionSecret cannot be empty")
	}

	if len(cfg.SessionSecret) < 16 {
		return fmt.Errorf("sessionSecret must be at least 16 characters long")
	}

	if cfg.SessionMaxAge < 0 {
		return fmt.Errorf("sessionMaxAge cannot be negative")
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
