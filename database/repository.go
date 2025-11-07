package database

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

var (
	ErrUserNotFound  = errors.New("user not found")
	ErrTokenNotFound = errors.New("token not found")
)

// Repository provides database operations
type Repository struct {
	db *DB
}

// NewRepository creates a new repository
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// ==================== USER OPERATIONS ====================

// CreateUser creates a new user
func (r *Repository) CreateUser(user *User) error {
	return r.db.Create(user).Error
}

// GetUserByID retrieves a user by ID
func (r *Repository) GetUserByID(id uint) (*User, error) {
	var user User
	err := r.db.First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// GetUserByEmail retrieves a user by email
func (r *Repository) GetUserByEmail(email string) (*User, error) {
	var user User
	err := r.db.Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// GetUserByProviderID retrieves a user by provider and provider user ID
func (r *Repository) GetUserByProviderID(provider, providerUserID string) (*User, error) {
	var user User
	err := r.db.Where("provider = ? AND provider_user_id = ?", provider, providerUserID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// UpdateUser updates a user
func (r *Repository) UpdateUser(user *User) error {
	return r.db.Save(user).Error
}

// UpdateUserLastLogin updates the user's last login time
func (r *Repository) UpdateUserLastLogin(userID uint) error {
	now := time.Now().UTC()
	return r.db.Model(&User{}).Where("id = ?", userID).Update("last_login_at", now).Error
}

// DeleteUser soft deletes a user
func (r *Repository) DeleteUser(id uint) error {
	return r.db.Delete(&User{}, id).Error
}

// ==================== TOKEN OPERATIONS ====================

// CreateToken creates a new token
func (r *Repository) CreateToken(token *Token) error {
	return r.db.Create(token).Error
}

// GetTokenByID retrieves a token by ID
func (r *Repository) GetTokenByID(id uint) (*Token, error) {
	var token Token
	err := r.db.Preload("User").First(&token, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	return &token, err
}

// GetTokenByPrefix retrieves a token by prefix
func (r *Repository) GetTokenByPrefix(prefix string) (*Token, error) {
	var token Token
	err := r.db.Where("token_prefix = ? AND revoked = ? AND deleted_at IS NULL", prefix, false).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	return &token, err
}

// GetTokensByUserID retrieves all tokens for a user
func (r *Repository) GetTokensByUserID(userID uint) ([]Token, error) {
	var tokens []Token
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error
	return tokens, err
}

// GetValidTokensByUserID retrieves all valid (non-revoked, non-expired) tokens for a user
func (r *Repository) GetValidTokensByUserID(userID uint) ([]Token, error) {
	var tokens []Token
	now := time.Now().UTC()
	err := r.db.Where("user_id = ? AND revoked = ? AND (expires_at IS NULL OR expires_at > ?)",
		userID, false, now).Order("created_at DESC").Find(&tokens).Error
	return tokens, err
}

// UpdateToken updates a token
func (r *Repository) UpdateToken(token *Token) error {
	return r.db.Save(token).Error
}

// UpdateTokenLastUsed updates the token's last used time
func (r *Repository) UpdateTokenLastUsed(tokenID uint) error {
	now := time.Now().UTC()
	return r.db.Model(&Token{}).Where("id = ?", tokenID).Update("last_used_at", now).Error
}

// UpdateTokenLastUsedByPrefix updates the token's last used time by prefix
func (r *Repository) UpdateTokenLastUsedByPrefix(prefix string) error {
	now := time.Now().UTC()
	return r.db.Model(&Token{}).Where("token_prefix = ?", prefix).Update("last_used_at", now).Error
}

// RevokeToken revokes a token
func (r *Repository) RevokeToken(id uint) error {
	return r.db.Model(&Token{}).Where("id = ?", id).Update("revoked", true).Error
}

// RevokeTokensByUserID revokes all tokens for a user
func (r *Repository) RevokeTokensByUserID(userID uint) error {
	return r.db.Model(&Token{}).Where("user_id = ?", userID).Update("revoked", true).Error
}

// DeleteToken soft deletes a token
func (r *Repository) DeleteToken(id uint) error {
	return r.db.Delete(&Token{}, id).Error
}

// DeleteExpiredTokens deletes tokens that have expired
func (r *Repository) DeleteExpiredTokens() (int64, error) {
	now := time.Now().UTC()
	result := r.db.Where("expires_at < ?", now).Delete(&Token{})
	return result.RowsAffected, result.Error
}

// CountUserTokens counts the number of tokens for a user
func (r *Repository) CountUserTokens(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&Token{}).Where("user_id = ? AND revoked = ?", userID, false).Count(&count).Error
	return count, err
}
