package auth

import (
	"anthropic-proxy/config"
	"anthropic-proxy/logger"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCClient handles OpenID Connect authentication
type OIDCClient struct {
	provider     *oidc.Provider
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier
	config       config.OpenIDConfig
}

// UserInfo represents user information from the OpenID provider
type UserInfo struct {
	Sub           string `json:"sub"`            // Subject (unique user ID)
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
}

// NewOIDCClient creates a new OpenID Connect client
func NewOIDCClient(cfg config.OpenIDConfig) (*OIDCClient, error) {
	if !cfg.Enabled {
		return nil, errors.New("OpenID Connect is not enabled")
	}

	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("client ID and client secret are required")
	}

	ctx := context.Background()

	var provider *oidc.Provider
	var err error

	// Use auto-discovery if issuer is provided
	if cfg.Issuer != "" {
		provider, err = oidc.NewProvider(ctx, cfg.Issuer)
		if err != nil {
			return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
		}
		logger.Info("OIDC provider initialized with auto-discovery", "issuer", cfg.Issuer)
	} else {
		// Manual configuration
		if cfg.AuthEndpoint == "" || cfg.TokenEndpoint == "" {
			return nil, errors.New("either issuer or manual endpoints (authEndpoint, tokenEndpoint) must be provided")
		}
		// For manual configuration, we can't use auto-discovery
		// We'll set up OAuth2 config manually below
		logger.Info("OIDC provider initialized with manual configuration")
	}

	// Get scopes
	scopes := cfg.GetDefaultScopes()

	// Set up OAuth2 configuration
	oauth2Config := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       scopes,
	}

	// Set endpoints
	if provider != nil {
		oauth2Config.Endpoint = provider.Endpoint()
	} else {
		// Manual endpoint configuration
		oauth2Config.Endpoint = oauth2.Endpoint{
			AuthURL:  cfg.AuthEndpoint,
			TokenURL: cfg.TokenEndpoint,
		}
	}

	// Create ID token verifier
	var verifier *oidc.IDTokenVerifier
	if provider != nil {
		verifier = provider.Verifier(&oidc.Config{
			ClientID: cfg.ClientID,
		})
	}

	client := &OIDCClient{
		provider:     provider,
		oauth2Config: oauth2Config,
		verifier:     verifier,
		config:       cfg,
	}

	logger.Info("OIDC client initialized successfully",
		"provider", cfg.Provider,
		"scopes", scopes)

	return client, nil
}

// GenerateAuthURL generates the OAuth2 authorization URL with state and nonce
func (c *OIDCClient) GenerateAuthURL() (authURL string, state string, nonce string, err error) {
	// Generate random state
	state, err = generateRandomString(32)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate state: %w", err)
	}

	// Generate random nonce
	nonce, err = generateRandomString(32)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Build auth URL with state and nonce
	authURL = c.oauth2Config.AuthCodeURL(state,
		oidc.Nonce(nonce),
		oauth2.AccessTypeOffline, // Request refresh token
	)

	return authURL, state, nonce, nil
}

// ExchangeCode exchanges the authorization code for tokens
func (c *OIDCClient) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	// Exchange code for token
	oauth2Token, err := c.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	logger.Debug("Successfully exchanged authorization code for tokens")
	return oauth2Token, nil
}

// VerifyIDToken verifies the ID token and extracts claims
func (c *OIDCClient) VerifyIDToken(ctx context.Context, rawIDToken string, nonce string) (*oidc.IDToken, error) {
	if c.verifier == nil {
		return nil, errors.New("ID token verification not available with manual configuration")
	}

	// Verify ID token
	idToken, err := c.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	// Verify nonce if provided
	if nonce != "" {
		if idToken.Nonce != nonce {
			return nil, errors.New("nonce mismatch")
		}
	}

	logger.Debug("ID token verified successfully", "subject", idToken.Subject)
	return idToken, nil
}

// GetUserInfo retrieves user information from the provider
func (c *OIDCClient) GetUserInfo(ctx context.Context, oauth2Token *oauth2.Token) (*UserInfo, error) {
	// Extract ID token from OAuth2 token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("no id_token in OAuth2 token")
	}

	// Verify ID token (without nonce check here, as it was checked during login)
	var userInfo UserInfo
	if c.verifier != nil {
		idToken, err := c.verifier.Verify(ctx, rawIDToken)
		if err != nil {
			return nil, fmt.Errorf("failed to verify ID token: %w", err)
		}

		// Extract claims from ID token
		if err := idToken.Claims(&userInfo); err != nil {
			return nil, fmt.Errorf("failed to parse claims: %w", err)
		}
	}

	// If provider supports UserInfo endpoint, fetch additional info
	if c.provider != nil {
		userInfoEndpoint, err := c.provider.UserInfo(ctx, oauth2.StaticTokenSource(oauth2Token))
		if err != nil {
			logger.Warn("Failed to get user info endpoint", "error", err.Error())
		} else if err := userInfoEndpoint.Claims(&userInfo); err != nil {
			logger.Warn("Failed to fetch user info from endpoint, using ID token claims", "error", err.Error())
		}
	}

	logger.Debug("Retrieved user info", "email", userInfo.Email, "name", userInfo.Name)
	return &userInfo, nil
}

// RefreshToken refreshes an OAuth2 token if it has a refresh token
func (c *OIDCClient) RefreshToken(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	token := &oauth2.Token{
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(-time.Hour), // Force refresh
	}

	tokenSource := c.oauth2Config.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	logger.Debug("Successfully refreshed OAuth2 token")
	return newToken, nil
}

// generateRandomString generates a cryptographically secure random string
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}
