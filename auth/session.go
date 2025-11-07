package auth

import (
	"anthropic-proxy/config"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

const (
	sessionName = "anthropic_proxy_session"
	userIDKey   = "user_id"
	emailKey    = "email"
	nameKey     = "name"
	stateKey    = "oauth_state"
	nonceKey    = "oauth_nonce"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrUserNotInSession = errors.New("user not in session")
)

// SessionManager handles session management
type SessionManager struct {
	store *sessions.CookieStore
	maxAge int
}

// NewSessionManager creates a new session manager
func NewSessionManager(cfg config.AdminUIConfig) *SessionManager {
	// Create cookie store with secret key
	store := sessions.NewCookieStore([]byte(cfg.SessionSecret))

	// Configure session options
	maxAge := cfg.GetSessionMaxAge()
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	}

	return &SessionManager{
		store:  store,
		maxAge: maxAge,
	}
}

// SetSecure sets whether cookies should be secure (HTTPS only)
func (sm *SessionManager) SetSecure(secure bool) {
	sm.store.Options.Secure = secure
}

// CreateSession creates a new session with user information
func (sm *SessionManager) CreateSession(c *gin.Context, userID uint, email, name string) error {
	session, err := sm.store.Get(c.Request, sessionName)
	if err != nil {
		return err
	}

	session.Values[userIDKey] = userID
	session.Values[emailKey] = email
	session.Values[nameKey] = name

	return session.Save(c.Request, c.Writer)
}

// GetUserID retrieves the user ID from the session
func (sm *SessionManager) GetUserID(c *gin.Context) (uint, error) {
	session, err := sm.store.Get(c.Request, sessionName)
	if err != nil {
		return 0, err
	}

	userID, ok := session.Values[userIDKey].(uint)
	if !ok {
		// Try converting from other integer types
		if id, ok := session.Values[userIDKey].(int); ok {
			return uint(id), nil
		}
		if id, ok := session.Values[userIDKey].(int64); ok {
			return uint(id), nil
		}
		return 0, ErrUserNotInSession
	}

	return userID, nil
}

// GetUserEmail retrieves the user email from the session
func (sm *SessionManager) GetUserEmail(c *gin.Context) (string, error) {
	session, err := sm.store.Get(c.Request, sessionName)
	if err != nil {
		return "", err
	}

	email, ok := session.Values[emailKey].(string)
	if !ok {
		return "", ErrUserNotInSession
	}

	return email, nil
}

// GetUserName retrieves the user name from the session
func (sm *SessionManager) GetUserName(c *gin.Context) (string, error) {
	session, err := sm.store.Get(c.Request, sessionName)
	if err != nil {
		return "", err
	}

	name, ok := session.Values[nameKey].(string)
	if !ok {
		return "", ErrUserNotInSession
	}

	return name, nil
}

// DestroySession destroys the session
func (sm *SessionManager) DestroySession(c *gin.Context) error {
	session, err := sm.store.Get(c.Request, sessionName)
	if err != nil {
		return err
	}

	// Set max age to -1 to delete the cookie
	session.Options.MaxAge = -1
	session.Values = make(map[interface{}]interface{})

	return session.Save(c.Request, c.Writer)
}

// SetOAuthState stores OAuth state and nonce in the session
func (sm *SessionManager) SetOAuthState(c *gin.Context, state, nonce string) error {
	session, err := sm.store.Get(c.Request, sessionName)
	if err != nil {
		return err
	}

	session.Values[stateKey] = state
	session.Values[nonceKey] = nonce

	return session.Save(c.Request, c.Writer)
}

// GetAndClearOAuthState retrieves and clears OAuth state and nonce from the session
func (sm *SessionManager) GetAndClearOAuthState(c *gin.Context) (state, nonce string, err error) {
	session, err := sm.store.Get(c.Request, sessionName)
	if err != nil {
		return "", "", err
	}

	state, ok1 := session.Values[stateKey].(string)
	nonce, ok2 := session.Values[nonceKey].(string)

	if !ok1 || !ok2 {
		return "", "", errors.New("OAuth state not found in session")
	}

	// Clear state and nonce
	delete(session.Values, stateKey)
	delete(session.Values, nonceKey)

	if err := session.Save(c.Request, c.Writer); err != nil {
		return "", "", err
	}

	return state, nonce, nil
}

// IsAuthenticated checks if the user is authenticated
func (sm *SessionManager) IsAuthenticated(c *gin.Context) bool {
	_, err := sm.GetUserID(c)
	return err == nil
}

// RequireAuth is a middleware that requires authentication
func (sm *SessionManager) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sm.IsAuthenticated(c) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "authentication required",
				},
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
