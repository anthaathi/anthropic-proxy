package database

import (
	"anthropic-proxy/logger"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

// DB holds the database connection
type DB struct {
	*gorm.DB
}

// Config holds database configuration
type Config struct {
	Driver   string // "sqlite" or "postgres"
	DSN      string // Data Source Name / Connection string
	MaxConns int    // Maximum number of connections in pool
}

// NewDB creates a new database connection
func NewDB(cfg Config) (*DB, error) {
	var dialector gorm.Dialector

	// Select appropriate driver
	switch cfg.Driver {
	case "sqlite", "sqlite3":
		dialector = sqlite.Open(cfg.DSN)
	case "postgres", "postgresql":
		dialector = postgres.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	// Configure GORM logger to use our custom logger
	gormCfg := &gorm.Config{
		Logger: gormLogger.New(
			&gormLogAdapter{},
			gormLogger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  gormLogger.Warn,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	// Open database connection
	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL DB for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying database: %w", err)
	}

	// Configure connection pool
	if cfg.MaxConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxConns)
		sqlDB.SetMaxIdleConns(cfg.MaxConns / 2)
	} else {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
	}
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("Database connected successfully",
		"driver", cfg.Driver,
		"maxConns", cfg.MaxConns)

	return &DB{DB: db}, nil
}

// AutoMigrate runs database migrations
func (db *DB) AutoMigrate() error {
	logger.Info("Running database migrations...")

	err := db.DB.AutoMigrate(
		&User{},
		&Token{},
	)

	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	logger.Info("Database migrations completed successfully")
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Health checks the database connection health
func (db *DB) Health() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// gormLogAdapter adapts GORM logger to our custom logger
type gormLogAdapter struct{}

func (l *gormLogAdapter) Printf(format string, args ...interface{}) {
	logger.Debug(fmt.Sprintf(format, args...))
}
