package main

import (
	"anthropic-proxy/auth"
	"anthropic-proxy/config"
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
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

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

	// Initialize retry configuration
	retryConfig := retry.DefaultConfig()
	if cfg.Spec.Retry != nil {
		if cfg.Spec.Retry.MaxRetries > 0 {
			retryConfig.MaxRetries = cfg.Spec.Retry.MaxRetries
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
	}

	// Start benchmark job
	benchmarker := metrics.NewBenchmarker(providerMgr, tracker, cfg.Spec.Models, reqLogger)
	benchmarker.Start()
	defer benchmarker.Stop()

	// Initialize handlers (needed for both modes)
	proxyHandler := proxy.NewHandler(fallbackMgr, tracker, errorTracker, retryConfig, reqLogger)
	modelsHandler := proxy.NewModelsHandler(modelRegistry)
	healthHandler := proxy.NewHealthHandler(providerMgr, tracker, errorTracker)
	countTokensHandler := proxy.NewCountTokensHandler(fallbackMgr)

	// Start HTTP server in background
	srv := startHTTPServer(cfg, proxyHandler, modelsHandler, healthHandler, countTokensHandler, authService)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
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

func startHTTPServer(cfg *config.Config, proxyHandler *proxy.Handler, modelsHandler *proxy.ModelsHandler, healthHandler *proxy.HealthHandler, countTokensHandler *proxy.CountTokensHandler, authService *auth.Service) *http.Server {
	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)

	// Disable Gin's default logging to prevent interference with TUI
	gin.DefaultWriter = nil
	gin.DefaultErrorWriter = nil

	r := gin.New()
	// Use recovery middleware but not the logger middleware
	r.Use(gin.Recovery())

	// Health check (no auth required)
	r.GET("/health", healthHandler.HandleHealth)

	// API routes (with authentication)
	api := r.Group("/v1")
	api.Use(authService.Middleware())
	{
		api.POST("/messages", proxyHandler.HandleMessages)
		api.POST("/messages/count_tokens", countTokensHandler.HandleCountTokens)
		api.GET("/models", modelsHandler.HandleListModels)
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

	// Start server in a goroutine
	go func() {
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
