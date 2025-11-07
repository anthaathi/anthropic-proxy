package tui

import (
	"anthropic-proxy/config"
	"anthropic-proxy/metrics"
	"anthropic-proxy/provider"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App represents the TUI application
type App struct {
	tviewApp        *tview.Application
	pages           *tview.Pages
	overviewPage    *OverviewPage
	logsPage        *LogsPage
	configPage      *ConfigPage
	benchmarkPage   *BenchmarkPage
	logBuffer       *LogBuffer
	tracker         *metrics.Tracker
	errorTracker    *metrics.ErrorTracker
	providerManager *provider.Manager
	cfg             *config.Config
	benchmarker     *metrics.Benchmarker
	configUpdater   *config.ConfigUpdater
	configReloader  *config.Reloader
	configManager   *ConfigReloadManager
	stopChan        chan struct{}
}

// NewApp creates a new TUI application
func NewApp(tracker *metrics.Tracker, errorTracker *metrics.ErrorTracker, providerManager *provider.Manager, cfg *config.Config, benchmarker *metrics.Benchmarker) *App {
	app := &App{
		tviewApp:        tview.NewApplication(),
		pages:           tview.NewPages(),
		logBuffer:       NewLogBuffer(1000), // Keep last 1000 log entries
		tracker:         tracker,
		errorTracker:    errorTracker,
		providerManager: providerManager,
		cfg:             cfg,
		benchmarker:     benchmarker,
		stopChan:        make(chan struct{}),
	}

	// Create pages
	app.overviewPage = NewOverviewPage(tracker, errorTracker, providerManager, cfg, benchmarker)
	app.logsPage = NewLogsPage(app.logBuffer)
	app.configPage = NewConfigPage(cfg)
	app.benchmarkPage = NewBenchmarkPage(tracker, providerManager, cfg, benchmarker)

	// Add pages to the page manager
	app.pages.AddPage("overview", app.overviewPage, true, true)
	app.pages.AddPage("logs", app.logsPage, true, false)
	app.pages.AddPage("config", app.configPage, true, false)
	app.pages.AddPage("benchmark", app.benchmarkPage, true, false)

	// Set up navigation
	app.setupNavigation()

	// Set up global key bindings
	app.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q', 'Q':
			app.Stop()
			return nil
		case '1':
			app.SwitchPage("overview")
			return nil
		case '2':
			app.SwitchPage("logs")
			return nil
		case '3':
			app.SwitchPage("config")
			return nil
		case '4':
			app.SwitchPage("benchmark")
			return nil
		}
		return event
	})

	app.tviewApp.SetRoot(app.pages, true)

	// Initialize config reload manager
	app.configManager = NewConfigReloadManager(app)

	return app
}

// setupNavigation sets up navigation between pages
func (a *App) setupNavigation() {
	switchPage := func(pageName string) {
		a.SwitchPage(pageName)
	}

	a.overviewPage.SetupInputCapture(switchPage)
	a.logsPage.SetupInputCapture(switchPage)
	a.configPage.SetupInputCapture(switchPage)
	a.benchmarkPage.SetupInputCapture(switchPage)
}

// SwitchPage switches to the specified page
func (a *App) SwitchPage(pageName string) {
	a.pages.SwitchToPage(pageName)
	a.tviewApp.SetFocus(a.pages)
}

// Run starts the TUI application
func (a *App) Run() error {
	// Start update ticker
	go a.updateLoop()

	// Run the application
	return a.tviewApp.Run()
}

// updateLoop periodically updates the display
func (a *App) updateLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.tviewApp.QueueUpdateDraw(func() {
				// Update the currently visible page
				currentPage, _ := a.pages.GetFrontPage()
				switch currentPage {
				case "overview":
					a.overviewPage.Update()
				case "logs":
					a.logsPage.Update()
				case "config":
					// Config page is static, no need to update frequently
					// a.configPage.Update()
				case "benchmark":
					a.benchmarkPage.Update()
				}
			})
		case <-a.stopChan:
			return
		}
	}
}

// Stop stops the TUI application
func (a *App) Stop() {
	close(a.stopChan)
	a.tviewApp.Stop()
}

// GetLogBuffer returns the log buffer for hooking into the logger
func (a *App) GetLogBuffer() *LogBuffer {
	return a.logBuffer
}

// SetConfigReloader sets the config reloader for TUI integration
func (a *App) SetConfigReloader(updater *config.ConfigUpdater, reloader *config.Reloader) {
	a.configUpdater = updater
	a.configReloader = reloader

	// Set up config manager with updater and reloader
	if a.configManager != nil {
		a.configManager.SetConfigUpdater(updater, reloader)
	}

	// Set callback to update TUI when config changes
	if updater != nil {
		updater.SetConfigChangedCallback(func() {
			// Refresh all pages after config change
			if a.overviewPage != nil {
				a.overviewPage.Update()
			}
		})
	}
}

// SetConfigWatching sets whether config watching is enabled
func (a *App) SetConfigWatching(watching bool) {
	if a.configManager != nil && a.configManager.state != nil {
		a.configManager.state.isWatching = watching
	}

	// Update the overview page status
	if a.overviewPage != nil {
		a.overviewPage.SetConfigWatching(watching)
	}
}

// GetConfigManager returns the config reload manager
func (a *App) GetConfigManager() *ConfigReloadManager {
	return a.configManager
}

// QueueUpdateDraw queues a function to be executed on the main TUI thread
func (a *App) QueueUpdateDraw(f func()) {
	a.tviewApp.QueueUpdateDraw(f)
}
