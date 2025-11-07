package main

import "anthropic-proxy/config"

// ProviderManager interface for config reloader (avoids import cycle)
type ProviderManager interface {
	UpdateProviders(providers map[string]config.Provider)
	Size() int
}

// ModelRegistry interface for config reloader (avoids import cycle)
type ModelRegistry interface {
	UpdateModels(models []config.Model)
	Size() int
}

// AuthService interface for config reloader
type AuthService interface {
	UpdateKeys(keys []string)
	Middleware() interface{} // Return interface{} to avoid gin import in config
}