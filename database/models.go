package database

import (
	"time"

	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	Email          string         `gorm:"uniqueIndex;not null" json:"email"`
	Name           string         `json:"name"`
	ProviderUserID string         `gorm:"index" json:"provider_user_id"`
	Provider       string         `json:"provider"` // "google", "auth0", etc.
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	LastLoginAt    *time.Time     `json:"last_login_at,omitempty"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	Tokens         []Token        `gorm:"foreignKey:UserID" json:"tokens,omitempty"`
}

// Token represents an API token
type Token struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	UserID      uint           `gorm:"index;not null" json:"user_id"`
	TokenHash   string         `gorm:"uniqueIndex;not null" json:"-"`          // bcrypt hash of full token
	TokenPrefix string         `gorm:"index;not null;size:16" json:"prefix"`   // First 8 chars for display/lookup
	Name        string         `gorm:"size:255" json:"name"`                   // User-friendly name
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	LastUsedAt  *time.Time     `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
	Revoked     bool           `gorm:"default:false;index" json:"revoked"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	User        User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName overrides the table name for User
func (User) TableName() string {
	return "users"
}

// TableName overrides the table name for Token
func (Token) TableName() string {
	return "tokens"
}

// IsExpired checks if the token has expired
func (t *Token) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*t.ExpiresAt)
}

// IsValid checks if the token is valid (not revoked, not expired, not deleted)
func (t *Token) IsValid() bool {
	if t.Revoked {
		return false
	}
	if t.IsExpired() {
		return false
	}
	if t.DeletedAt.Valid {
		return false
	}
	return true
}
