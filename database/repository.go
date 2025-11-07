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

// ==================== ADDITIONAL USER OPERATIONS ====================

// GetUserCount returns the total number of users
func (r *Repository) GetUserCount() (int64, error) {
	var count int64
	err := r.db.Model(&User{}).Count(&count).Error
	return count, err
}

// GetAllUsers retrieves all users (for admin)
func (r *Repository) GetAllUsers() ([]User, error) {
	var users []User
	err := r.db.Order("created_at DESC").Find(&users).Error
	return users, err
}

// PromoteToAdmin promotes a user to admin
func (r *Repository) PromoteToAdmin(userID uint) error {
	return r.db.Model(&User{}).Where("id = ?", userID).Update("is_admin", true).Error
}

// DemoteFromAdmin removes admin privileges from a user
func (r *Repository) DemoteFromAdmin(userID uint) error {
	return r.db.Model(&User{}).Where("id = ?", userID).Update("is_admin", false).Error
}

// ==================== REQUEST LOG OPERATIONS ====================

// CreateRequestLog creates a new request log entry
func (r *Repository) CreateRequestLog(log *RequestLog) error {
	return r.db.Create(log).Error
}

// GetUserRequestLogs retrieves recent request logs for a user
func (r *Repository) GetUserRequestLogs(userID uint, limit int, offset int) ([]RequestLog, error) {
	var logs []RequestLog
	err := r.db.Where("user_id = ?", userID).
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error
	return logs, err
}

// GetAllRequestLogs retrieves recent request logs across all users (admin)
func (r *Repository) GetAllRequestLogs(limit int, offset int) ([]RequestLog, error) {
	var logs []RequestLog
	err := r.db.Preload("User").
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error
	return logs, err
}

// CountUserRequestLogs counts total request logs for a user
func (r *Repository) CountUserRequestLogs(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&RequestLog{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// DeleteOldRequestLogs deletes request logs older than the specified time
func (r *Repository) DeleteOldRequestLogs(before time.Time) (int64, error) {
	result := r.db.Where("timestamp < ?", before).Delete(&RequestLog{})
	return result.RowsAffected, result.Error
}

// GetRequestLogsToAggregate retrieves logs that need to be aggregated
func (r *Repository) GetRequestLogsToAggregate(before time.Time) ([]RequestLog, error) {
	var logs []RequestLog
	err := r.db.Where("timestamp < ?", before).Find(&logs).Error
	return logs, err
}

// ==================== USAGE SUMMARY OPERATIONS ====================

// GetOrCreateMonthlySummary gets or creates a monthly usage summary
func (r *Repository) GetOrCreateMonthlySummary(userID uint, year, month int) (*UsageSummary, error) {
	var summary UsageSummary
	err := r.db.Where("user_id = ? AND year = ? AND month = ?", userID, year, month).
		First(&summary).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new summary
		summary = UsageSummary{
			UserID:  userID,
			Year:    year,
			Month:   month,
			Models:  "{}",
			Providers: "{}",
		}
		if err := r.db.Create(&summary).Error; err != nil {
			return nil, err
		}
		return &summary, nil
	}

	if err != nil {
		return nil, err
	}

	return &summary, nil
}

// UpdateMonthlySummary updates a monthly usage summary
func (r *Repository) UpdateMonthlySummary(summary *UsageSummary) error {
	return r.db.Save(summary).Error
}

// GetUserSummaries retrieves usage summaries for a user
func (r *Repository) GetUserSummaries(userID uint, limit int) ([]UsageSummary, error) {
	var summaries []UsageSummary
	err := r.db.Where("user_id = ?", userID).
		Order("year DESC, month DESC").
		Limit(limit).
		Find(&summaries).Error
	return summaries, err
}

// GetAllUserSummaries retrieves recent usage summaries for all users (admin)
func (r *Repository) GetAllUserSummaries(limit int) ([]UsageSummary, error) {
	var summaries []UsageSummary
	err := r.db.Preload("User").
		Order("year DESC, month DESC").
		Limit(limit).
		Find(&summaries).Error
	return summaries, err
}

// GetUserTotalUsage calculates total usage stats for a user
func (r *Repository) GetUserTotalUsage(userID uint) (totalRequests, totalTokens int64, err error) {
	var summary struct {
		TotalRequests int64
		TotalTokens   int64
	}

	err = r.db.Model(&UsageSummary{}).
		Select("COALESCE(SUM(total_requests), 0) as total_requests, COALESCE(SUM(total_tokens), 0) as total_tokens").
		Where("user_id = ?", userID).
		Scan(&summary).Error

	return summary.TotalRequests, summary.TotalTokens, err
}
