package auth

import (
	"anthropic-proxy/database"
	"anthropic-proxy/logger"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// Token format: sk-{prefix}-{secret}
	tokenPrefix  = "sk"
	prefixLength = 8
	secretLength = 32
	bcryptCost   = 12
)

var (
	ErrInvalidToken = errors.New("invalid token format")
	ErrExpiredToken = errors.New("token has expired")
	ErrRevokedToken = errors.New("token has been revoked")
)

// TokenManager handles token generation and validation
type TokenManager struct {
	repo  *database.Repository
	cache *tokenCache
}

// NewTokenManager creates a new token manager
func NewTokenManager(repo *database.Repository) *TokenManager {
	return &TokenManager{
		repo:  repo,
		cache: newTokenCache(),
	}
}

// GenerateToken generates a new API token for a user
func (tm *TokenManager) GenerateToken(userID uint, name string, expiresInDays int) (tokenString string, token *database.Token, err error) {
	// Generate random prefix (for quick lookup and display)
	prefixBytes := make([]byte, prefixLength)
	if _, err := rand.Read(prefixBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate prefix: %w", err)
	}
	prefix := base64.URLEncoding.EncodeToString(prefixBytes)[:prefixLength]

	// Generate random secret (the actual authentication token)
	secretBytes := make([]byte, secretLength)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	secret := base64.URLEncoding.EncodeToString(secretBytes)[:secretLength]

	// Create full token string: sk-{prefix}-{secret}
	tokenString = fmt.Sprintf("%s-%s-%s", tokenPrefix, prefix, secret)

	// Hash the full token for storage
	hash, err := bcrypt.GenerateFromPassword([]byte(tokenString), bcryptCost)
	if err != nil {
		return "", nil, fmt.Errorf("failed to hash token: %w", err)
	}

	// Calculate expiration if specified
	var expiresAt *time.Time
	if expiresInDays > 0 {
		exp := time.Now().UTC().Add(time.Duration(expiresInDays) * 24 * time.Hour)
		expiresAt = &exp
	}

	// Create token record
	token = &database.Token{
		UserID:      userID,
		TokenHash:   string(hash),
		TokenPrefix: prefix,
		Name:        name,
		ExpiresAt:   expiresAt,
		Revoked:     false,
	}

	// Save to database
	if err := tm.repo.CreateToken(token); err != nil {
		return "", nil, fmt.Errorf("failed to create token: %w", err)
	}

	logger.Info("Generated new API token",
		"user_id", userID,
		"token_id", token.ID,
		"prefix", prefix,
		"name", name)

	return tokenString, token, nil
}

// ValidateToken validates a token string and returns the associated token record
func (tm *TokenManager) ValidateToken(tokenString string) (*database.Token, error) {
	// Parse token format: sk-{prefix}-{secret}
	parts := strings.Split(tokenString, "-")
	if len(parts) != 3 || parts[0] != tokenPrefix {
		return nil, ErrInvalidToken
	}

	prefix := parts[1]

	// Check cache first
	if cachedToken := tm.cache.Get(prefix); cachedToken != nil {
		// Verify hash
		if err := bcrypt.CompareHashAndPassword([]byte(cachedToken.TokenHash), []byte(tokenString)); err != nil {
			// Hash doesn't match, remove from cache and continue to DB lookup
			tm.cache.Remove(prefix)
		} else {
			// Cache hit and valid
			if !cachedToken.IsValid() {
				return nil, ErrRevokedToken
			}
			// Update last used time asynchronously
			go tm.updateLastUsed(cachedToken.ID)
			return cachedToken, nil
		}
	}

	// Look up token by prefix in database
	token, err := tm.repo.GetTokenByPrefix(prefix)
	if err != nil {
		if errors.Is(err, database.ErrTokenNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Verify token hash
	if err := bcrypt.CompareHashAndPassword([]byte(token.TokenHash), []byte(tokenString)); err != nil {
		logger.Warn("Token hash mismatch", "prefix", prefix)
		return nil, ErrInvalidToken
	}

	// Check if token is valid (not revoked, not expired)
	if token.Revoked {
		return nil, ErrRevokedToken
	}

	if token.IsExpired() {
		return nil, ErrExpiredToken
	}

	// Cache the token for future requests
	tm.cache.Set(prefix, token)

	// Update last used time asynchronously
	go tm.updateLastUsed(token.ID)

	return token, nil
}

// RevokeToken revokes a token
func (tm *TokenManager) RevokeToken(tokenID uint) error {
	token, err := tm.repo.GetTokenByID(tokenID)
	if err != nil {
		return err
	}

	// Remove from cache
	tm.cache.Remove(token.TokenPrefix)

	// Revoke in database
	if err := tm.repo.RevokeToken(tokenID); err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	logger.Info("Revoked token", "token_id", tokenID, "prefix", token.TokenPrefix)
	return nil
}

// RevokeUserTokens revokes all tokens for a user
func (tm *TokenManager) RevokeUserTokens(userID uint) error {
	// Get all user tokens to remove from cache
	tokens, err := tm.repo.GetTokensByUserID(userID)
	if err != nil {
		return err
	}

	// Remove from cache
	for _, token := range tokens {
		tm.cache.Remove(token.TokenPrefix)
	}

	// Revoke in database
	if err := tm.repo.RevokeTokensByUserID(userID); err != nil {
		return fmt.Errorf("failed to revoke user tokens: %w", err)
	}

	logger.Info("Revoked all tokens for user", "user_id", userID, "count", len(tokens))
	return nil
}

// GetUserTokens retrieves all tokens for a user
func (tm *TokenManager) GetUserTokens(userID uint) ([]database.Token, error) {
	return tm.repo.GetTokensByUserID(userID)
}

// updateLastUsed updates the token's last used timestamp
func (tm *TokenManager) updateLastUsed(tokenID uint) {
	if err := tm.repo.UpdateTokenLastUsed(tokenID); err != nil {
		logger.Error("Failed to update token last used time",
			"token_id", tokenID,
			"error", err.Error())
	}
}

// InvalidateCache removes a token from the cache
func (tm *TokenManager) InvalidateCache(prefix string) {
	tm.cache.Remove(prefix)
}

// tokenCache provides a simple in-memory cache for tokens
type tokenCache struct {
	data map[string]*database.Token
	mu   sync.RWMutex
	ttl  time.Duration
}

func newTokenCache() *tokenCache {
	cache := &tokenCache{
		data: make(map[string]*database.Token),
		ttl:  5 * time.Minute, // Cache tokens for 5 minutes
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

func (c *tokenCache) Get(prefix string) *database.Token {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[prefix]
}

func (c *tokenCache) Set(prefix string, token *database.Token) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[prefix] = token
}

func (c *tokenCache) Remove(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, prefix)
}

func (c *tokenCache) cleanup() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		// Clear entire cache periodically to prevent stale data
		c.data = make(map[string]*database.Token)
		c.mu.Unlock()
	}
}
