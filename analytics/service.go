package analytics

import (
	"anthropic-proxy/database"
	"anthropic-proxy/logger"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Service handles analytics tracking and aggregation
type Service struct {
	repo             *database.Repository
	retentionDays    int
	mu               sync.Mutex
	aggregationQueue chan *database.RequestLog
}

// NewService creates a new analytics service
func NewService(repo *database.Repository, retentionDays int) *Service {
	if retentionDays <= 0 {
		retentionDays = 30 // Default to 30 days
	}

	s := &Service{
		repo:             repo,
		retentionDays:    retentionDays,
		aggregationQueue: make(chan *database.RequestLog, 100),
	}

	// Start background worker for async processing
	go s.processQueue()

	return s
}

// RecordRequest records a new API request (async)
func (s *Service) RecordRequest(userID uint, tokenID *uint, model, provider string, inputTokens, outputTokens int, duration time.Duration, status, errorMsg string) {
	log := &database.RequestLog{
		UserID:       userID,
		TokenID:      tokenID,
		Model:        model,
		Provider:     provider,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		Duration:     duration.Milliseconds(),
		Status:       status,
		Error:        errorMsg,
		Timestamp:    time.Now().UTC(),
	}

	// Queue for async processing
	select {
	case s.aggregationQueue <- log:
		// Queued successfully
	default:
		// Queue full, process synchronously
		logger.Warn("Analytics queue full, processing synchronously")
		s.processLog(log)
	}
}

// processQueue processes logs from the queue
func (s *Service) processQueue() {
	for log := range s.aggregationQueue {
		s.processLog(log)
	}
}

// processLog saves the log and updates monthly summary
func (s *Service) processLog(log *database.RequestLog) {
	// Save request log
	if err := s.repo.CreateRequestLog(log); err != nil {
		logger.Error("Failed to create request log", "error", err.Error())
		return
	}

	// Update monthly summary asynchronously
	go s.updateMonthlySummary(log)
}

// updateMonthlySummary updates the monthly usage summary
func (s *Service) updateMonthlySummary(log *database.RequestLog) {
	s.mu.Lock()
	defer s.mu.Unlock()

	year := log.Timestamp.Year()
	month := int(log.Timestamp.Month())

	// Get or create monthly summary
	summary, err := s.repo.GetOrCreateMonthlySummary(log.UserID, year, month)
	if err != nil {
		logger.Error("Failed to get monthly summary", "error", err.Error())
		return
	}

	// Update counters
	summary.TotalRequests++
	summary.TotalTokens += int64(log.TotalTokens)
	summary.TotalInputTokens += int64(log.InputTokens)
	summary.TotalOutputTokens += int64(log.OutputTokens)

	if log.Status == "success" {
		summary.SuccessRequests++
	} else {
		summary.ErrorRequests++
	}

	// Update model counts
	if err := s.updateJSONMap(&summary.Models, log.Model); err != nil {
		logger.Error("Failed to update model counts", "error", err.Error())
	}

	// Update provider counts
	if err := s.updateJSONMap(&summary.Providers, log.Provider); err != nil {
		logger.Error("Failed to update provider counts", "error", err.Error())
	}

	// Save updated summary
	if err := s.repo.UpdateMonthlySummary(summary); err != nil {
		logger.Error("Failed to update monthly summary", "error", err.Error())
	}
}

// updateJSONMap updates a JSON map field by incrementing the count
func (s *Service) updateJSONMap(jsonField *string, key string) error {
	var counts map[string]int64
	if err := json.Unmarshal([]byte(*jsonField), &counts); err != nil {
		counts = make(map[string]int64)
	}

	counts[key]++

	updated, err := json.Marshal(counts)
	if err != nil {
		return err
	}

	*jsonField = string(updated)
	return nil
}

// GetUserAnalytics retrieves analytics for a user
func (s *Service) GetUserAnalytics(userID uint, limit int) (*UserAnalytics, error) {
	// Get recent request logs
	logs, err := s.repo.GetUserRequestLogs(userID, limit, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get request logs: %w", err)
	}

	// Get monthly summaries
	summaries, err := s.repo.GetUserSummaries(userID, 12) // Last 12 months
	if err != nil {
		return nil, fmt.Errorf("failed to get summaries: %w", err)
	}

	// Get total usage
	totalRequests, totalTokens, err := s.repo.GetUserTotalUsage(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get total usage: %w", err)
	}

	return &UserAnalytics{
		TotalRequests: totalRequests,
		TotalTokens:   totalTokens,
		RecentLogs:    logs,
		Summaries:     summaries,
	}, nil
}

// GetAllUsersAnalytics retrieves analytics for all users (admin only)
func (s *Service) GetAllUsersAnalytics() ([]UserSummaryStats, error) {
	users, err := s.repo.GetAllUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	var stats []UserSummaryStats
	for _, user := range users {
		totalRequests, totalTokens, err := s.repo.GetUserTotalUsage(user.ID)
		if err != nil {
			logger.Error("Failed to get user total usage", "user_id", user.ID, "error", err.Error())
			continue
		}

		stats = append(stats, UserSummaryStats{
			UserID:        user.ID,
			Email:         user.Email,
			Name:          user.Name,
			IsAdmin:       user.IsAdmin,
			CreatedAt:     user.CreatedAt,
			LastLoginAt:   user.LastLoginAt,
			TotalRequests: totalRequests,
			TotalTokens:   totalTokens,
		})
	}

	return stats, nil
}

// AggregateOldLogs aggregates and deletes old request logs
func (s *Service) AggregateOldLogs() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoffDate := time.Now().UTC().AddDate(0, 0, -s.retentionDays)

	logger.Info("Starting log aggregation", "cutoff_date", cutoffDate)

	// Get logs to aggregate
	logs, err := s.repo.GetRequestLogsToAggregate(cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to get logs to aggregate: %w", err)
	}

	logger.Info("Found logs to aggregate", "count", len(logs))

	// Aggregate logs into monthly summaries
	for _, log := range logs {
		// Update monthly summary (this will happen in-memory, then save)
		s.updateMonthlySummary(&log)
	}

	// Delete old logs
	deleted, err := s.repo.DeleteOldRequestLogs(cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old logs: %w", err)
	}

	logger.Info("Aggregation complete", "deleted_logs", deleted)
	return nil
}

// UserAnalytics represents analytics data for a user
type UserAnalytics struct {
	TotalRequests int64                   `json:"total_requests"`
	TotalTokens   int64                   `json:"total_tokens"`
	RecentLogs    []database.RequestLog   `json:"recent_logs"`
	Summaries     []database.UsageSummary `json:"summaries"`
}

// UserSummaryStats represents summary statistics for a user
type UserSummaryStats struct {
	UserID        uint       `json:"user_id"`
	Email         string     `json:"email"`
	Name          string     `json:"name"`
	IsAdmin       bool       `json:"is_admin"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	TotalRequests int64      `json:"total_requests"`
	TotalTokens   int64      `json:"total_tokens"`
}
