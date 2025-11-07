package config

// Config represents the root configuration structure
type Config struct {
	Spec Spec `yaml:"spec"`
}

// Spec contains the main configuration sections
type Spec struct {
	Providers map[string]Provider `yaml:"providers"`
	Models    []Model             `yaml:"models"`
	APIKeys   []string            `yaml:"apiKeys"` // Deprecated: use Auth.StaticKeys instead
	Retry     *RetryConfig        `yaml:"retry,omitempty"`
	Auth      *AuthConfig         `yaml:"auth,omitempty"`
}

// Provider represents a backend provider configuration
type Provider struct {
	Type     string `yaml:"type"`     // "anthropic" or "openai"
	Endpoint string `yaml:"endpoint"`
	APIKey   string `yaml:"apiKey"`
}

// GetType returns the provider type, defaulting to "anthropic" if not set
func (p *Provider) GetType() string {
	if p.Type == "" {
		return "anthropic"
	}
	return p.Type
}

// Model represents a model configuration
type Model struct {
	Name     string `yaml:"name"`
	Alias    string `yaml:"alias"`
	Context  int    `yaml:"context"`
	Provider string `yaml:"provider"`
	Weight   int    `yaml:"weight"`
	Thinking bool   `yaml:"thinking"`
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
	MaxRetries        int     `yaml:"maxRetries"`
	InitialDelay      string  `yaml:"initialDelay"`
	MaxDelay          string  `yaml:"maxDelay"`
	BackoffMultiplier float64 `yaml:"backoffMultiplier"`
	RetrySameProvider bool    `yaml:"retrySameProvider"`
}

// AuthConfig represents authentication configuration
type AuthConfig struct {
	// Static API keys (backward compatible)
	StaticKeys []string `yaml:"staticKeys,omitempty"`

	// Database configuration
	Database DatabaseConfig `yaml:"database"`

	// OpenID Connect configuration
	OpenID OpenIDConfig `yaml:"openid"`

	// Admin UI configuration
	AdminUI AdminUIConfig `yaml:"adminUI"`

	// Analytics configuration
	DataRetentionDays int `yaml:"dataRetentionDays"` // How many days to keep detailed request logs (default: 30)
}

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	Driver   string `yaml:"driver"`   // "sqlite" or "postgres"
	DSN      string `yaml:"dsn"`      // Data Source Name / Connection string
	MaxConns int    `yaml:"maxConns"` // Maximum number of connections in pool
}

// OpenIDConfig represents OpenID Connect configuration
type OpenIDConfig struct {
	Enabled bool   `yaml:"enabled"`
	Provider string `yaml:"provider"` // "google", "auth0", "keycloak", "custom"

	// Auto-discovery (preferred - provider will fetch .well-known/openid-configuration)
	Issuer string `yaml:"issuer,omitempty"`

	// Manual configuration (if provider doesn't support auto-discovery)
	AuthEndpoint     string `yaml:"authEndpoint,omitempty"`
	TokenEndpoint    string `yaml:"tokenEndpoint,omitempty"`
	UserinfoEndpoint string `yaml:"userinfoEndpoint,omitempty"`
	JWKSUrl          string `yaml:"jwksUrl,omitempty"`

	// OAuth2 credentials
	ClientID     string   `yaml:"clientId"`
	ClientSecret string   `yaml:"clientSecret"`
	RedirectURL  string   `yaml:"redirectUrl"`
	Scopes       []string `yaml:"scopes,omitempty"`
}

// AdminUIConfig represents admin UI configuration
type AdminUIConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Path          string `yaml:"path"`          // Default: "/admin"
	SessionSecret string `yaml:"sessionSecret"` // Secret key for session encryption
	SessionMaxAge int    `yaml:"sessionMaxAge"` // Session max age in seconds (default: 86400 = 24h)
	BaseURL       string `yaml:"baseUrl"`       // Base URL for the proxy (for config display)
}

// GetDefaultScopes returns default OpenID scopes if none are specified
func (o *OpenIDConfig) GetDefaultScopes() []string {
	if len(o.Scopes) > 0 {
		return o.Scopes
	}
	return []string{"openid", "email", "profile"}
}

// GetAdminPath returns the admin UI path with default
func (a *AdminUIConfig) GetAdminPath() string {
	if a.Path == "" {
		return "/admin"
	}
	return a.Path
}

// GetSessionMaxAge returns session max age with default
func (a *AdminUIConfig) GetSessionMaxAge() int {
	if a.SessionMaxAge <= 0 {
		return 86400 // 24 hours
	}
	return a.SessionMaxAge
}

// GetDataRetentionDays returns data retention days with default
func (a *AuthConfig) GetDataRetentionDays() int {
	if a.DataRetentionDays <= 0 {
		return 30 // 30 days default
	}
	return a.DataRetentionDays
}
