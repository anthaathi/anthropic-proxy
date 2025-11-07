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
	IsAdmin        bool           `gorm:"default:false;index" json:"is_admin"`
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

// RequestLog represents a detailed log of an API request
type RequestLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"index;not null" json:"user_id"`
	TokenID      *uint     `gorm:"index" json:"token_id,omitempty"`
	Model        string    `gorm:"index;size:100" json:"model"`
	Provider     string    `gorm:"index;size:100" json:"provider"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `gorm:"index" json:"total_tokens"`
	Duration     int64     `json:"duration"` // Duration in milliseconds
	Status       string    `gorm:"index;size:20" json:"status"` // "success" or "error"
	Error        string    `gorm:"size:500" json:"error,omitempty"`
	Timestamp    time.Time `gorm:"index;not null" json:"timestamp"`
	User         User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName overrides the table name for RequestLog
func (RequestLog) TableName() string {
	return "request_logs"
}

// UsageSummary represents aggregated monthly usage statistics
type UsageSummary struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	UserID           uint      `gorm:"index;not null" json:"user_id"`
	Year             int       `gorm:"index;not null" json:"year"`
	Month            int       `gorm:"index;not null" json:"month"`
	TotalRequests    int64     `json:"total_requests"`
	SuccessRequests  int64     `json:"success_requests"`
	ErrorRequests    int64     `json:"error_requests"`
	TotalTokens      int64     `json:"total_tokens"`
	TotalInputTokens int64     `json:"total_input_tokens"`
	TotalOutputTokens int64     `json:"total_output_tokens"`
	Models           string    `gorm:"type:text" json:"models"` // JSON map of model -> count
	Providers        string    `gorm:"type:text" json:"providers"` // JSON map of provider -> count
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	User             User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName overrides the table name for UsageSummary
func (UsageSummary) TableName() string {
	return "usage_summaries"
}
