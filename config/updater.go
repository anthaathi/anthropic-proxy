package config

import (
	"anthropic-proxy/logger"
	"fmt"
	"strings"
)

// ProviderManager interface for updating providers
type ProviderManager interface {
	UpdateProviders(providers map[string]Provider)
	Size() int
}

// ModelRegistry interface for updating models
type ModelRegistry interface {
	UpdateModels(models []Model)
	Size() int
}

// AuthService interface for updating API keys
type AuthService interface {
	UpdateKeys(keys []string)
}

// ConfigUpdater handles configuration updates with confirmation prompts
type ConfigUpdater struct {
	providerMgr     ProviderManager
	modelRegistry   ModelRegistry
	authService     AuthService
	currentConfig   *Config
	onConfigChanged func() // Callback for when config changes
}

// NewConfigUpdater creates a new configuration updater
func NewConfigUpdater(providerMgr ProviderManager, modelRegistry ModelRegistry, authService AuthService) *ConfigUpdater {
	return &ConfigUpdater{
		providerMgr:   providerMgr,
		modelRegistry: modelRegistry,
		authService:   authService,
	}
}

// SetConfigChangedCallback sets a callback to be called when config changes
func (u *ConfigUpdater) SetConfigChangedCallback(callback func()) {
	u.onConfigChanged = callback
}

// TryReload attempts to reload configuration with confirmation
func (u *ConfigUpdater) TryReload(configPath string) error {
	logger.Info("Detected config file change, validating new configuration")

	// Load and validate new config
	newConfig, err := Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load new config: %w", err)
	}

	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("new config validation failed: %w", err)
	}

	// Show what will change
	changes := u.calculateChanges(u.currentConfig, newConfig)
	if len(changes) == 0 {
		logger.Info("No configuration changes detected")
		return nil
	}

	// Ask for confirmation
	if !u.confirmChanges(changes) {
		logger.Info("Configuration reload cancelled by user")
		return nil
	}

	// Apply the changes
	if err := u.applyConfig(newConfig); err != nil {
		return fmt.Errorf("failed to apply new config: %w", err)
	}

	u.currentConfig = newConfig
	logger.Info("Configuration reloaded successfully", "changes", len(changes))

	// Trigger callback if set
	if u.onConfigChanged != nil {
		u.onConfigChanged()
	}

	return nil
}

// TryReloadWithCallback attempts to reload with a callback function for UI
func (u *ConfigUpdater) TryReloadWithCallback(configPath string, uiCallback func([]ConfigChange, func(), func())) error {
	logger.Info("Detected config file change, validating new configuration")

	// Load and validate new config
	newConfig, err := Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load new config: %w", err)
	}

	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("new config validation failed: %w", err)
	}

	// Calculate changes
	changes := u.calculateChanges(u.currentConfig, newConfig)
	if len(changes) == 0 {
		logger.Info("No configuration changes detected")
		return nil
	}

	// TUI mode MUST provide a callback - no fallback to prevent terminal corruption
	if uiCallback == nil {
		logger.Error("TryReloadWithCallback called without callback - this indicates a bug")
		return fmt.Errorf("no UI callback provided for config reload")
	}

	// Use UI callback
	uiCallback(changes, func() {
		// User confirmed - apply changes asynchronously with panic recovery
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Panic recovered in config reload goroutine",
						"panic", fmt.Sprintf("%v", r))
				}
			}()

			if err := u.applyConfig(newConfig); err != nil {
				logger.Error("Config reload failed", "error", err)
			} else {
				u.currentConfig = newConfig
				logger.Info("Configuration reloaded successfully", "changes", len(changes))

				// Trigger callback if set
				if u.onConfigChanged != nil {
					u.onConfigChanged()
				}
			}
		}()
	}, func() {
		// User cancelled
		logger.Info("Configuration reload cancelled by user")
	})
	return nil
}

// SetCurrentConfig sets the current configuration reference
func (u *ConfigUpdater) SetCurrentConfig(cfg *Config) {
	u.currentConfig = cfg
}

// ConfigChange represents a detected change in configuration
type ConfigChange struct {
	Type        string // "provider", "model", "apikey"
	Action      string // "added", "removed", "updated", "changed"
	Name        string
	Description string
}

// calculateChanges determines what changed between old and new config
func (u *ConfigUpdater) calculateChanges(oldConfig, newConfig *Config) []ConfigChange {
	var changes []ConfigChange

	// Debug logging
	if oldConfig == nil {
		logger.Info("Old config is nil!")
		return changes
	}
	if newConfig == nil {
		logger.Info("New config is nil!")
		return changes
	}

	logger.Info("Comparing configs",
		"old_providers", len(oldConfig.Spec.Providers),
		"new_providers", len(newConfig.Spec.Providers),
		"old_models", len(oldConfig.Spec.Models),
		"new_models", len(newConfig.Spec.Models),
		"old_apikeys", len(oldConfig.Spec.APIKeys),
		"new_apikeys", len(newConfig.Spec.APIKeys))

	// Provider changes
	oldProviders := oldConfig.Spec.Providers
	newProviders := newConfig.Spec.Providers

	// Added providers
	for name, newProvider := range newProviders {
		if _, exists := oldProviders[name]; !exists {
			changes = append(changes, ConfigChange{
				Type:        "provider",
				Action:      "added",
				Name:        name,
				Description: fmt.Sprintf("Provider '%s' with endpoint '%s'", name, newProvider.Endpoint),
			})
		}
	}

	// Removed providers
	for name, oldProvider := range oldProviders {
		if _, exists := newProviders[name]; !exists {
			changes = append(changes, ConfigChange{
				Type:        "provider",
				Action:      "removed",
				Name:        name,
				Description: fmt.Sprintf("Provider '%s' with endpoint '%s'", name, oldProvider.Endpoint),
			})
		}
	}

	// Updated providers
	for name, newProvider := range newProviders {
		if oldProvider, exists := oldProviders[name]; exists {
			if oldProvider.Endpoint != newProvider.Endpoint || oldProvider.APIKey != newProvider.APIKey {
				desc := fmt.Sprintf("Provider '%s'", name)
				if oldProvider.Endpoint != newProvider.Endpoint {
					desc += fmt.Sprintf(" endpoint: %s → %s", oldProvider.Endpoint, newProvider.Endpoint)
				}
				if oldProvider.APIKey != newProvider.APIKey {
					desc += " API key updated"
				}
				changes = append(changes, ConfigChange{
					Type:        "provider",
					Action:      "updated",
					Name:        name,
					Description: desc,
				})
			}
		}
	}

	// Model changes
	oldModels := oldConfig.Spec.Models
	newModels := newConfig.Spec.Models

	// Simple model comparison (since models are arrays)
	// This is a basic implementation - could be more sophisticated
	if len(oldModels) != len(newModels) {
		changes = append(changes, ConfigChange{
			Type:        "model",
			Action:      "changed",
			Name:        "models",
			Description: fmt.Sprintf("Model count: %d → %d", len(oldModels), len(newModels)),
		})
	}

	// API key changes - create maps for easier comparison
	oldKeys := oldConfig.Spec.APIKeys
	newKeys := newConfig.Spec.APIKeys

	logger.Info("API key details",
		"old_count", len(oldKeys),
		"new_count", len(newKeys))

	oldKeyMap := make(map[string]bool)
	for i, key := range oldKeys {
		oldKeyMap[key] = true
		prefix := key
		if len(key) > 10 {
			prefix = key[:10] + "..."
		}
		logger.Info("Old API key", "index", i, "prefix", prefix)
	}

	newKeyMap := make(map[string]bool)
	for i, key := range newKeys {
		newKeyMap[key] = true
		prefix := key
		if len(key) > 10 {
			prefix = key[:10] + "..."
		}
		logger.Info("New API key", "index", i, "prefix", prefix)
	}

	// Check for added keys
	addedCount := 0
	for key := range newKeyMap {
		if !oldKeyMap[key] {
			addedCount++
			prefix := key
			if len(key) > 10 {
				prefix = key[:10] + "..."
			}
			logger.Info("Detected added API key", "prefix", prefix)
		}
	}

	// Check for removed keys
	removedCount := 0
	for key := range oldKeyMap {
		if !newKeyMap[key] {
			removedCount++
			prefix := key
			if len(key) > 10 {
				prefix = key[:10] + "..."
			}
			logger.Info("Detected removed API key", "prefix", prefix)
		}
	}

	if addedCount > 0 || removedCount > 0 {
		logger.Info("API key change summary", "added", addedCount, "removed", removedCount)

		desc := fmt.Sprintf("API keys: %d → %d", len(oldKeys), len(newKeys))
		if addedCount > 0 {
			desc += fmt.Sprintf(" (+%d added", addedCount)
			if removedCount > 0 {
				desc += fmt.Sprintf(", -%d removed", removedCount)
			}
			desc += ")"
		} else if removedCount > 0 {
			desc += fmt.Sprintf(" (-%d removed)", removedCount)
		}

		changes = append(changes, ConfigChange{
			Type:        "apikey",
			Action:      "changed",
			Name:        "apikeys",
			Description: desc,
		})
	}

	// Retry config changes
	if (oldConfig.Spec.Retry == nil) != (newConfig.Spec.Retry == nil) {
		changes = append(changes, ConfigChange{
			Type:        "retry",
			Action:      "changed",
			Name:        "retry",
			Description: "Retry configuration added/removed",
		})
	} else if oldConfig.Spec.Retry != nil && newConfig.Spec.Retry != nil {
		if *oldConfig.Spec.Retry != *newConfig.Spec.Retry {
			changes = append(changes, ConfigChange{
				Type:        "retry",
				Action:      "changed",
				Name:        "retry",
				Description: "Retry configuration updated",
			})
		}
	}

	return changes
}

// confirmChanges asks user to confirm configuration changes
// This is ONLY for CLI mode (server mode with --watch flag without TUI)
// TUI mode MUST use TryReloadWithCallback with a modal callback instead
func (u *ConfigUpdater) confirmChanges(changes []ConfigChange) bool {
	// This function should never be called in TUI mode
	// If it is, it's a bug and we should not use fmt.Printf
	fmt.Printf("\n🔄 Configuration changes detected:\n")
	fmt.Printf("%s\n", strings.Repeat("=", 50))

	for i, change := range changes {
		fmt.Printf("%d. [%s] %s\n", i+1, strings.ToUpper(change.Type), change.Description)
	}

	fmt.Printf("%s\n", strings.Repeat("=", 50))
	fmt.Printf("Total changes: %d\n", len(changes))

	var response string
	for {
		fmt.Printf("\nApply these changes? (y/n): ")
		fmt.Scanln(&response)
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "y" || response == "yes" {
			return true
		} else if response == "n" || response == "no" {
			return false
		} else {
			fmt.Printf("Please enter 'y' or 'n'\n")
		}
	}
}

// applyConfig applies the new configuration
func (u *ConfigUpdater) applyConfig(newConfig *Config) error {
	// Update providers
	u.providerMgr.UpdateProviders(newConfig.Spec.Providers)

	// Update models
	u.modelRegistry.UpdateModels(newConfig.Spec.Models)

	// Update API keys
	if u.authService != nil {
		u.authService.UpdateKeys(newConfig.Spec.APIKeys)
	}

	logger.Info("Configuration applied successfully")
	return nil
}

// MockConfirmChanges for testing without user input
func MockConfirmChanges(changes []ConfigChange) bool {
	logger.Info("Mock: Auto-accepting configuration changes", "count", len(changes))
	return true
}