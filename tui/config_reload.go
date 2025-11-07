package tui

import (
	"anthropic-proxy/config"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConfigReloadState manages the config reload state
type ConfigReloadState struct {
	isWatching      bool
	lastReloadTime  string
	reloadCount     int
	pendingChanges  []config.ConfigChange
}

// ConfigReloadModal shows a proper modal for config reload confirmation
type ConfigReloadModal struct {
	app          *App
	modal        *tview.Modal
	changes      []config.ConfigChange
	onConfirm    func()
	onCancel     func()
	state        *ConfigReloadState
}

// NewConfigReloadModal creates a new config reload modal
func NewConfigReloadModal(app *App, changes []config.ConfigChange, state *ConfigReloadState) *ConfigReloadModal {
	modal := &ConfigReloadModal{
		app:     app,
		changes: changes,
		state:   state,
	}

	modal.createModal()
	return modal
}

// createModal creates the modal dialog with proper styling
func (m *ConfigReloadModal) createModal() {
	// Build the modal text with proper formatting
	text := "[::b]🔄 Configuration Changes Detected[::-]\n\n"

	// Group changes by type for better readability
	providerChanges := []config.ConfigChange{}
	modelChanges := []config.ConfigChange{}
	authChanges := []config.ConfigChange{}
	otherChanges := []config.ConfigChange{}

	for _, change := range m.changes {
		switch change.Type {
		case "provider":
			providerChanges = append(providerChanges, change)
		case "model":
			modelChanges = append(modelChanges, change)
		case "apikey", "auth":
			authChanges = append(authChanges, change)
		default:
			otherChanges = append(otherChanges, change)
		}
	}

	// Display changes grouped by type
	if len(providerChanges) > 0 {
		text += "[yellow]📡 Providers:[white]\n"
		for _, change := range providerChanges {
			text += m.formatChange(change)
		}
		text += "\n"
	}

	if len(modelChanges) > 0 {
		text += "[yellow]🤖 Models:[white]\n"
		for _, change := range modelChanges {
			text += m.formatChange(change)
		}
		text += "\n"
	}

	if len(authChanges) > 0 {
		text += "[yellow]🔐 Authentication:[white]\n"
		for _, change := range authChanges {
			text += m.formatChange(change)
		}
		text += "\n"
	}

	if len(otherChanges) > 0 {
		text += "[yellow]⚙️ Configuration:[white]\n"
		for _, change := range otherChanges {
			text += m.formatChange(change)
		}
		text += "\n"
	}

	// Summary
	text += fmt.Sprintf("[::b]Total Changes: %d[::-]\n\n", len(m.changes))
	text += "[::b]Apply these changes? (Y/N)::-"

	// Create modal with buttons
	m.modal = tview.NewModal().
		SetText(text).
		AddButtons([]string{"[green]✓ Apply (Y)[-]", "[red]✗ Cancel (N)[-]"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			m.handleDone(buttonIndex)
		})

	// Set up keyboard shortcuts
	m.modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'y', 'Y':
			m.handleDone(0) // Apply
			return nil
		case 'n', 'N':
			m.handleDone(1) // Cancel
			return nil
		case 'q', 'Q':
			m.handleDone(1) // Cancel on Q
			return nil
		}
		return event
	})

	// Style the modal
	m.modal.SetBackgroundColor(tcell.ColorBlack)
	m.modal.SetBorderColor(tcell.ColorYellow)
	m.modal.SetTitle("🔄 Configuration Reload")
	m.modal.SetTextColor(tcell.ColorWhite)
}

// formatChange formats a single change for display
func (m *ConfigReloadModal) formatChange(change config.ConfigChange) string {
	var icon, color string
	switch change.Action {
	case "added":
		icon = "✓"
		color = "[green]"
	case "removed":
		icon = "✗"
		color = "[red]"
	case "updated", "changed":
		icon = "↻"
		color = "[yellow]"
	default:
		icon = "•"
		color = "[white]"
	}

	return fmt.Sprintf("  %s%s %s %s[-]\n", color, icon, strings.Title(change.Type), change.Description)
}

// handleDone handles the user's response
func (m *ConfigReloadModal) handleDone(buttonIndex int) {
	// Remove the modal
	m.app.pages.RemovePage("configReload")

	// Switch back to overview page
	m.app.pages.SwitchToPage("overview")

	if buttonIndex == 0 {
		// User confirmed
		m.state.reloadCount++
		m.state.lastReloadTime = fmt.Sprintf("%d:%02d:%02d",
			time.Now().Hour(), time.Now().Minute(), time.Now().Second())

		// Add success message to logs
		m.app.logBuffer.AddLog(fmt.Sprintf("✓ Config reloaded successfully (%d changes applied)", len(m.changes)), "SUCCESS")

		if m.onConfirm != nil {
			m.onConfirm()
		}
	} else {
		// User cancelled
		m.app.logBuffer.AddLog("✗ Config reload cancelled by user", "INFO")

		if m.onCancel != nil {
			m.onCancel()
		}
	}
}

// Show shows the modal
func (m *ConfigReloadModal) Show() {
	m.app.pages.AddPage("configReload", m.modal, true, true)
	m.app.pages.SwitchToPage("configReload")
}

// ConfigReloadManager manages the config reload functionality
type ConfigReloadManager struct {
	app              *App
	configUpdater    *config.ConfigUpdater
	configReloader   *config.Reloader
	state            *ConfigReloadState
}

// NewConfigReloadManager creates a new config reload manager
func NewConfigReloadManager(app *App) *ConfigReloadManager {
	return &ConfigReloadManager{
		app:   app,
		state: &ConfigReloadState{
			isWatching: false,
		},
	}
}

// SetConfigUpdater sets the config updater and reloader
func (m *ConfigReloadManager) SetConfigUpdater(updater *config.ConfigUpdater, reloader *config.Reloader) {
	m.configUpdater = updater
	m.configReloader = reloader
	m.state.isWatching = (reloader != nil)

	// Set up callback to show modal when config changes
	if updater != nil {
		updater.SetConfigChangedCallback(func() {
			// This is called after config is successfully reloaded
			m.updateOverviewPage()
		})
	}
}

// SetConfigChangeHandler sets the handler for showing config change prompts
func (m *ConfigReloadManager) SetConfigChangeHandler(handler func([]config.ConfigChange, func(), func())) {
	// This handler will be used by the config updater
	// We'll need to store this and pass it to the TryReloadWithCallback call
	if m.configUpdater != nil {
		// Create a wrapper that calls the config updater with our handler
		// This will be called when config changes are detected
	}
}

// ShowConfigReloadPrompt shows the config reload prompt
func (m *ConfigReloadManager) ShowConfigReloadPrompt(changes []config.ConfigChange, onConfirm func(), onCancel func()) {
	if len(changes) == 0 {
		return
	}

	modal := NewConfigReloadModal(m.app, changes, m.state)
	modal.onConfirm = func() {
		m.state.pendingChanges = nil
		if onConfirm != nil {
			onConfirm()
		}
	}
	modal.onCancel = func() {
		m.state.pendingChanges = nil
		if onCancel != nil {
			onCancel()
		}
	}
	modal.Show()
}

// updateOverviewPage triggers an update of the overview page
func (m *ConfigReloadManager) updateOverviewPage() {
	if m.app.overviewPage != nil {
		m.app.tviewApp.QueueUpdateDraw(func() {
			m.app.overviewPage.Update()
		})
	}
}

// GetState returns the current config reload state
func (m *ConfigReloadManager) GetState() *ConfigReloadState {
	return m.state
}