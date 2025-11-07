package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Resolve environment variables and special values
	if err := cfg.resolveValues(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// resolveValues processes environment variables and special values in the config
func (c *Config) resolveValues() error {
	// Resolve provider API keys
	for name, provider := range c.Spec.Providers {
		resolved, err := resolveValue(provider.APIKey)
		if err != nil {
			return fmt.Errorf("failed to resolve API key for provider %s: %w", name, err)
		}
		provider.APIKey = resolved
		c.Spec.Providers[name] = provider
	}

	// Resolve API keys
	for i, key := range c.Spec.APIKeys {
		resolved, err := resolveValue(key)
		if err != nil {
			return fmt.Errorf("failed to resolve API key at index %d: %w", i, err)
		}
		c.Spec.APIKeys[i] = resolved
	}

	return nil
}

// resolveValue resolves environment variables and special values
func resolveValue(value string) (string, error) {
	// Handle $RANDOM_KEY
	if value == "$RANDOM_KEY" {
		return generateRandomKey()
	}

	// Handle env.VAR_NAME
	if strings.HasPrefix(value, "env.") {
		envVar := strings.TrimPrefix(value, "env.")
		envValue := os.Getenv(envVar)
		if envValue == "" {
			return "", fmt.Errorf("environment variable %s is not set", envVar)
		}
		return envValue, nil
	}

	// Return as-is (static value)
	return value, nil
}

// generateRandomKey creates a cryptographically secure random API key
func generateRandomKey() (string, error) {
	bytes := make([]byte, 32) // 32 bytes = 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}
	return "sk-proxy-" + base64.URLEncoding.EncodeToString(bytes), nil
}
