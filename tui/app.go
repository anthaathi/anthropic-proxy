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
	logBuffer       *LogBuffer
	tracker         *metrics.Tracker
	errorTracker    *metrics.ErrorTracker
	providerManager *provider.Manager
	cfg             *config.Config
	stopChan        chan struct{}
}

// NewApp creates a new TUI application
func NewApp(tracker *metrics.Tracker, errorTracker *metrics.ErrorTracker, providerManager *provider.Manager, cfg *config.Config) *App {
	app := &App{
		tviewApp:        tview.NewApplication(),
		pages:           tview.NewPages(),
		logBuffer:       NewLogBuffer(1000), // Keep last 1000 log entries
		tracker:         tracker,
		errorTracker:    errorTracker,
		providerManager: providerManager,
		cfg:             cfg,
		stopChan:        make(chan struct{}),
	}

	// Create pages
	app.overviewPage = NewOverviewPage(tracker, errorTracker, providerManager, cfg)
	app.logsPage = NewLogsPage(app.logBuffer)
	app.configPage = NewConfigPage(cfg)

	// Add pages to the page manager
	app.pages.AddPage("overview", app.overviewPage, true, true)
	app.pages.AddPage("logs", app.logsPage, true, false)
	app.pages.AddPage("config", app.configPage, true, false)

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
		}
		return event
	})

	app.tviewApp.SetRoot(app.pages, true)

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
