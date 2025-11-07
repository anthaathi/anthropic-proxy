package main

import (
	"anthropic-proxy/analytics"
	"anthropic-proxy/api"
	"anthropic-proxy/auth"
	"anthropic-proxy/config"
	"anthropic-proxy/database"
	"anthropic-proxy/logger"
	"anthropic-proxy/metrics"
	"anthropic-proxy/model"
	"anthropic-proxy/provider"
	"anthropic-proxy/proxy"
	"anthropic-proxy/requestlog"
	"anthropic-proxy/retry"
	"anthropic-proxy/router"
	"anthropic-proxy/tui"
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed ui/admin/*
var adminUI embed.FS

func main() {
	// Parse command-line flags
	tuiMode := flag.Bool("tui", false, "Run in TUI mode")
	watch := flag.Bool("watch", false, "Watch for config file changes and prompt to reload")
	logFile := flag.String("log-file", "", "Path to file for logging all requests and responses")
	tpsThreshold := flag.Float64("tps-threshold", 40.0, "Minimum TPS threshold for provider selection")
	flag.Parse()

	// Initialize logger
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	// Initialize logger (required for components that log during initialization)
	// For TUI mode, suppress stdout output initially
	if *tuiMode {
		// Use quiet logger to prevent stdout pollution before TUI starts
		logger.InitQuiet(logLevel)
	} else {
		logger.Init(logLevel)
		logger.Info("Loading configuration", "path", configPath)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	if !*tuiMode {
		logger.Info("Configuration loaded successfully",
			"providers", len(cfg.Spec.Providers),
			"models", len(cfg.Spec.Models),
			"apiKeys", len(cfg.Spec.APIKeys))
	}

	// Initialize components
	modelRegistry := model.NewRegistry()
	modelRegistry.Load(cfg.Spec.Models)

	providerMgr := provider.NewManager()
	providerMgr.Load(cfg.Spec.Providers)

	// Create dynamic auth service
	authService := auth.NewService()
	authService.UpdateKeys(cfg.Spec.APIKeys)

	// Initialize authentication components (database, OIDC, etc.)
	var db *database.DB
	var dbRepo *database.Repository
	var oidcClient *auth.OIDCClient
	var sessionManager *auth.SessionManager
	var tokenManager *auth.TokenManager
	var analyticsService *analytics.Service
	var cleanupJob *analytics.CleanupJob

	if cfg.Spec.Auth != nil {
		// Merge old APIKeys with new StaticKeys for backward compatibility
		staticKeys := cfg.Spec.APIKeys
		if len(cfg.Spec.Auth.StaticKeys) > 0 {
			staticKeys = append(staticKeys, cfg.Spec.Auth.StaticKeys...)
		}
		authService.UpdateKeys(staticKeys)

		// Initialize database if configured
		if cfg.Spec.Auth.Database.Driver != "" && cfg.Spec.Auth.Database.DSN != "" {
			dbCfg := database.Config{
				Driver:   cfg.Spec.Auth.Database.Driver,
				DSN:      cfg.Spec.Auth.Database.DSN,
				MaxConns: cfg.Spec.Auth.Database.MaxConns,
			}

			var err error
			db, err = database.NewDB(dbCfg)
			if err != nil {
				log.Printf("WARNING: Failed to connect to database: %v", err)
				log.Printf("Continuing with static keys only")
			} else {
				// Run migrations
				if err := db.AutoMigrate(); err != nil {
					log.Printf("WARNING: Failed to run migrations: %v", err)
					log.Printf("Continuing with static keys only")
					db.Close()
					db = nil
				} else {
					dbRepo = database.NewRepository(db)
					tokenManager = auth.NewTokenManager(dbRepo)
					authService.SetTokenManager(tokenManager)
					logger.Info("Database authentication enabled")

					// Initialize analytics service
					retentionDays := cfg.Spec.Auth.GetDataRetentionDays()
					analyticsService = analytics.NewService(dbRepo, retentionDays)
					logger.Info("Analytics service initialized", "retention_days", retentionDays)

					// Start cleanup job (runs daily)
					cleanupJob = analytics.NewCleanupJob(analyticsService, 24*time.Hour)
					cleanupJob.Start()
					logger.Info("Analytics cleanup job started")
				}
			}
		}

		// Initialize OpenID Connect if enabled
		if cfg.Spec.Auth.OpenID.Enabled {
			var err error
			oidcClient, err = auth.NewOIDCClient(cfg.Spec.Auth.OpenID)
			if err != nil {
				log.Printf("WARNING: Failed to initialize OIDC: %v", err)
				log.Printf("Admin UI will not be available")
			} else {
				logger.Info("OpenID Connect initialized")
			}
		}

		// Initialize session manager if admin UI is enabled
		if cfg.Spec.Auth.AdminUI.Enabled {
			if cfg.Spec.Auth.AdminUI.SessionSecret == "" {
				log.Printf("WARNING: Admin UI session secret not configured")
				log.Printf("Admin UI will not be available")
			} else {
				sessionManager = auth.NewSessionManager(cfg.Spec.Auth.AdminUI)
				logger.Info("Admin UI session manager initialized")
			}
		}
	}

	tracker := metrics.NewTracker()
	errorTracker := metrics.NewErrorTracker()

	// Initialize request logger if --log-file flag is provided
	var reqLogger *requestlog.RequestLogger
	if *logFile != "" {
		var err error
		reqLogger, err = requestlog.NewRequestLogger(*logFile)
		if err != nil {
			log.Fatalf("Failed to initialize request logger: %v", err)
		}
		logger.Info("Request logging enabled", "file", *logFile)
		defer reqLogger.Close()
	}

	selector := router.NewSelector(modelRegistry, providerMgr, tracker, *tpsThreshold)
	fallbackMgr := router.NewFallbackManager(selector)

	// Initialize retry configuration (used only when retrying the same provider is enabled)
	retryConfig := retry.DefaultConfig()
	retrySameProvider := false
	if cfg.Spec.Retry != nil {
		if cfg.Spec.Retry.MaxRetries > 0 {
			retryConfig.MaxRetries = cfg.Spec.Retry.MaxRetries
		} else {
			retryConfig.MaxRetries = 0
		}
		if cfg.Spec.Retry.InitialDelay != "" {
			if d, err := time.ParseDuration(cfg.Spec.Retry.InitialDelay); err == nil {
				retryConfig.InitialDelay = d
			}
		}
		if cfg.Spec.Retry.MaxDelay != "" {
			if d, err := time.ParseDuration(cfg.Spec.Retry.MaxDelay); err == nil {
				retryConfig.MaxDelay = d
			}
		}
		if cfg.Spec.Retry.BackoffMultiplier > 0 {
			retryConfig.BackoffMultiplier = cfg.Spec.Retry.BackoffMultiplier
		}
		if cfg.Spec.Retry.RetrySameProvider {
			retrySameProvider = true
		}
	}

	// Start benchmark job
	benchmarker := metrics.NewBenchmarker(providerMgr, tracker, cfg.Spec.Models, reqLogger)
	benchmarker.Start()
	defer benchmarker.Stop()

	// Initialize handlers (needed for both modes)
	proxyHandler := proxy.NewHandler(fallbackMgr, tracker, errorTracker, retryConfig, retrySameProvider, reqLogger, analyticsService)
	modelsHandler := proxy.NewModelsHandler(modelRegistry)
	healthHandler := proxy.NewHealthHandler(providerMgr, tracker, errorTracker)
	countTokensHandler := proxy.NewCountTokensHandler(fallbackMgr)

	// Start HTTP server in background
	srv := startHTTPServer(cfg, proxyHandler, modelsHandler, healthHandler, countTokensHandler, authService, oidcClient, sessionManager, dbRepo, tokenManager, analyticsService)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		// Stop cleanup job
		if cleanupJob != nil {
			cleanupJob.Stop()
		}
		// Close database connection
		if db != nil {
			db.Close()
		}
	}()

	// Setup config reloader if enabled (for non-TUI mode)
	var configReloader *config.Reloader
	var configUpdater *config.ConfigUpdater
	if *watch && !*tuiMode {
		// Create config updater
		configUpdater = config.NewConfigUpdater(providerMgr, modelRegistry, authService)
		configUpdater.SetCurrentConfig(cfg)

		// Create reloader with CLI confirmation callback
		rel, err := config.NewReloader(configPath, func() error {
			logger.Info("Config file changed, checking for updates...")
			// In server mode, use CLI confirmation
			return configUpdater.TryReload(configPath)
		})
		if err != nil {
			log.Fatalf("Failed to create config reloader: %v", err)
		}

		configReloader = rel

		// Start watching for config changes
		if err := configReloader.Start(); err != nil {
			log.Fatalf("Failed to start config reloader: %v", err)
		}

		logger.Info("Config watching enabled", "file", configPath)
		defer configReloader.Stop()
	}

	// Branch based on mode
	if *tuiMode {
		// In TUI mode, enable config watching by default
		if !*watch {
			*watch = true
		}
		// TUI mode will set up its own config reloader with proper modal callback
		runTUIMode(cfg, tracker, errorTracker, providerMgr, modelRegistry, authService, benchmarker, logLevel, configPath, *watch)
	} else {
		if *watch {
			logger.Info("Config watching enabled", "Press Ctrl+C to exit")
		}
		runServerMode()
	}
}

// customRecoveryMiddleware provides panic recovery without writing to stderr
func customRecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log through our logger instead of writing to stderr
				logger.Error("Panic recovered in HTTP handler",
					"error", fmt.Sprintf("%v", err),
					"path", c.Request.URL.Path)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

// setupAdminRoutes sets up admin UI and authentication routes
func setupAdminRoutes(r *gin.Engine, cfg *config.Config, oidcClient *auth.OIDCClient, sessionManager *auth.SessionManager, dbRepo *database.Repository, tokenManager *auth.TokenManager, analyticsService *analytics.Service) {
	// Create API handlers
	authHandler := api.NewAuthHandler(oidcClient, sessionManager, dbRepo)
	tokenHandler := api.NewTokenHandler(tokenManager, sessionManager, dbRepo)
	analyticsHandler := api.NewAnalyticsHandler(analyticsService, sessionManager)
	adminHandler := api.NewAdminHandler(analyticsService, sessionManager, dbRepo)
	configHandler := api.NewConfigHandler(cfg, sessionManager, tokenManager)

	// Auth flow endpoints (no auth required)
	authGroup := r.Group("/auth")
	{
		authGroup.GET("/login", authHandler.HandleLogin)
		authGroup.GET("/callback", authHandler.HandleCallback)
		authGroup.POST("/logout", authHandler.HandleLogout)
		authGroup.GET("/logout", authHandler.HandleLogout) // Support GET as well
	}

	// Admin UI static files
	adminPath := cfg.Spec.Auth.AdminUI.GetAdminPath()
	adminGroup := r.Group(adminPath)
	{
		// Serve embedded admin UI files
		adminFS, err := fs.Sub(adminUI, "ui/admin")
		if err != nil {
			logger.Error("Failed to load admin UI", "error", err.Error())
		} else {
			// Login page (no auth required)
			adminGroup.GET("/login", func(c *gin.Context) {
				// Check if already authenticated
				if sessionManager.IsAuthenticated(c) {
					c.Redirect(http.StatusTemporaryRedirect, adminPath)
					return
				}
				data, err := fs.ReadFile(adminFS, "login.html")
				if err != nil {
					c.String(http.StatusInternalServerError, "Failed to load login page")
					return
				}
				c.Data(http.StatusOK, "text/html; charset=utf-8", data)
			})

			// Main dashboard (requires auth)
			adminGroup.GET("", func(c *gin.Context) {
				// Check authentication
				if !sessionManager.IsAuthenticated(c) {
					c.Redirect(http.StatusTemporaryRedirect, adminPath+"/login")
					return
				}
				data, err := fs.ReadFile(adminFS, "index.html")
				if err != nil {
					c.String(http.StatusInternalServerError, "Failed to load dashboard")
					return
				}
				c.Data(http.StatusOK, "text/html; charset=utf-8", data)
			})

			// Serve JavaScript file
			adminGroup.GET("/app.js", func(c *gin.Context) {
				data, err := fs.ReadFile(adminFS, "app.js")
				if err != nil {
					c.String(http.StatusInternalServerError, "Failed to load script")
					return
				}
				c.Data(http.StatusOK, "application/javascript; charset=utf-8", data)
			})
		}
	}

	// API endpoints for token management and analytics (requires session auth)
	apiAuthGroup := r.Group("/api/auth")
	apiAuthGroup.Use(sessionManager.RequireAuth())
	{
		apiAuthGroup.GET("/user", authHandler.HandleGetUser)
		apiAuthGroup.GET("/tokens", tokenHandler.HandleListTokens)
		apiAuthGroup.POST("/tokens", tokenHandler.HandleCreateToken)
		apiAuthGroup.DELETE("/tokens/:id", tokenHandler.HandleRevokeToken)
		apiAuthGroup.PUT("/tokens/:id", tokenHandler.HandleUpdateToken)
		apiAuthGroup.GET("/analytics", analyticsHandler.HandleGetUserAnalytics)
		apiAuthGroup.GET("/config", configHandler.HandleGetConfig)
	}

	// Admin API endpoints (requires admin privileges)
	apiAdminGroup := r.Group("/api/admin")
	apiAdminGroup.Use(sessionManager.RequireAuth())
	apiAdminGroup.Use(adminHandler.RequireAdmin())
	{
		apiAdminGroup.GET("/users", adminHandler.HandleGetAllUsers)
		apiAdminGroup.GET("/analytics", adminHandler.HandleGetSystemAnalytics)
		apiAdminGroup.GET("/users/:id/analytics", adminHandler.HandleGetUserAnalytics)
		apiAdminGroup.POST("/users/:id/promote", adminHandler.HandlePromoteUser)
		apiAdminGroup.POST("/users/:id/demote", adminHandler.HandleDemoteUser)
	}

	logger.Info("Admin UI and authentication routes configured",
		"adminPath", adminPath,
		"loginURL", adminPath+"/login")
}

func startHTTPServer(cfg *config.Config, proxyHandler *proxy.Handler, modelsHandler *proxy.ModelsHandler, healthHandler *proxy.HealthHandler, countTokensHandler *proxy.CountTokensHandler, authService *auth.Service, oidcClient *auth.OIDCClient, sessionManager *auth.SessionManager, dbRepo *database.Repository, tokenManager *auth.TokenManager, analyticsService *analytics.Service) *http.Server {
	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)

	// Disable Gin's default logging to prevent interference with TUI
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard

	r := gin.New()
	// Use custom recovery middleware that logs through our logger instead of stderr
	r.Use(customRecoveryMiddleware())

	// Health check (no auth required)
	r.GET("/health", healthHandler.HandleHealth)

	// Setup admin UI and auth routes if configured
	if cfg.Spec.Auth != nil && cfg.Spec.Auth.AdminUI.Enabled && oidcClient != nil && sessionManager != nil && dbRepo != nil {
		setupAdminRoutes(r, cfg, oidcClient, sessionManager, dbRepo, tokenManager, analyticsService)
	}

	// API routes (with authentication)
	apiGroup := r.Group("/v1")
	apiGroup.Use(authService.Middleware())
	{
		apiGroup.POST("/messages", proxyHandler.HandleMessages)
		apiGroup.POST("/messages/count_tokens", countTokensHandler.HandleCountTokens)
		apiGroup.GET("/models", modelsHandler.HandleListModels)
	}

	// Setup HTTP server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic recovered in HTTP server goroutine",
					"panic", fmt.Sprintf("%v", r))
			}
		}()

		logger.Info("Starting server", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", "error", err.Error())
		}
	}()

	return srv
}

func runTUIMode(cfg *config.Config, tracker *metrics.Tracker, errorTracker *metrics.ErrorTracker, providerMgr *provider.Manager, modelRegistry *model.Registry, authService *auth.Service, benchmarker *metrics.Benchmarker, logLevel string, configPath string, watch bool) {
	// Create TUI app
	tuiApp := tui.NewApp(tracker, errorTracker, providerMgr, cfg, benchmarker)

	// Set up config watching with TUI modal callback if enabled
	if watch {
		// Create config updater
		configUpdater := config.NewConfigUpdater(providerMgr, modelRegistry, authService)
		configUpdater.SetCurrentConfig(cfg)

		// Create reloader with TUI modal callback
		configReloader, err := config.NewReloader(configPath, func() error {
			logger.Info("Config file changed, checking for updates...")

			// Use TUI callback to show modal
			return configUpdater.TryReloadWithCallback(configPath, func(changes []config.ConfigChange, onConfirm func(), onCancel func()) {
				// Get the config manager from the TUI app
				configManager := tuiApp.GetConfigManager()
				if configManager != nil {
					// Queue the modal to be shown on the main TUI thread
					tuiApp.QueueUpdateDraw(func() {
						configManager.ShowConfigReloadPrompt(changes, onConfirm, onCancel)
					})
				}
			})
		})

		if err != nil {
			log.Fatalf("Failed to create config reloader: %v", err)
		}

		// Start watching for config changes
		if err := configReloader.Start(); err != nil {
			log.Fatalf("Failed to start config reloader: %v", err)
		}

		// Set config reloader in TUI
		tuiApp.SetConfigReloader(configUpdater, configReloader)

		// Enable config watching in TUI to show status
		tuiApp.SetConfigWatching(true)

		logger.Info("Config watching enabled", "file", configPath)

		// Make sure to stop the reloader when TUI exits
		defer configReloader.Stop()
	}

	// Replace logger with TUI handler
	level := slog.LevelInfo
	switch logLevel {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}

	tuiHandler := tui.NewTUIHandler(tuiApp.GetLogBuffer(), level)
	logger.SetTUIHandler(tuiHandler)

	// Log initial messages
	logger.Info("Starting in TUI mode")
	logger.Info("Configuration loaded",
		"providers", len(cfg.Spec.Providers),
		"models", len(cfg.Spec.Models))

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Handle signals in background with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic recovered in signal handler goroutine",
					"panic", fmt.Sprintf("%v", r))
			}
		}()

		<-sigChan
		tuiApp.Stop()
	}()

	// Run TUI
	if err := tuiApp.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}

func runServerMode() {
	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server")
}
