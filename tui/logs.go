package tui

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LogsPage represents the logs viewer page
type LogsPage struct {
	*tview.Flex
	textView     *tview.TextView
	statusBar    *tview.TextView
	logBuffer    *LogBuffer
	filterLevel  slog.Level
	autoScroll   bool
	searchTerm   string
}

// NewLogsPage creates a new logs page
func NewLogsPage(logBuffer *LogBuffer) *LogsPage {
	page := &LogsPage{
		Flex:        tview.NewFlex(),
		textView:    tview.NewTextView(),
		statusBar:   tview.NewTextView(),
		logBuffer:   logBuffer,
		filterLevel: slog.LevelDebug, // Show all logs by default
		autoScroll:  true,
	}

	// Configure text view
	page.textView.
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			if page.autoScroll {
				page.textView.ScrollToEnd()
			}
		})

	page.textView.SetBorder(true).SetTitle(" Logs (Press 's' to toggle auto-scroll, 1-4 to filter level) ")

	// Configure status bar
	page.statusBar.
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	page.updateStatusBar()

	// Layout
	page.SetDirection(tview.FlexRow).
		AddItem(page.textView, 0, 1, true).
		AddItem(page.statusBar, 1, 0, false)

	return page
}

// Update refreshes the logs display
func (p *LogsPage) Update() {
	entries := p.logBuffer.GetAll()

	// Filter by level
	filtered := make([]LogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Level >= p.filterLevel {
			// Apply search filter if set
			if p.searchTerm == "" || strings.Contains(strings.ToLower(entry.Message), strings.ToLower(p.searchTerm)) {
				filtered = append(filtered, entry)
			}
		}
	}

	// Build display text
	var builder strings.Builder
	for _, entry := range filtered {
		builder.WriteString(FormatLogEntry(entry))
		builder.WriteString("\n")
	}

	p.textView.SetText(builder.String())
	p.updateStatusBar()
}

// updateStatusBar updates the status bar text
func (p *LogsPage) updateStatusBar() {
	levelStr := p.getLevelString()
	autoScrollStr := "ON"
	if !p.autoScroll {
		autoScrollStr = "OFF"
	}

	searchStr := ""
	if p.searchTerm != "" {
		searchStr = fmt.Sprintf(" | Search: [yellow]%s[white]", p.searchTerm)
	}

	entries := p.logBuffer.GetAll()
	status := fmt.Sprintf("Logs: [green]%d[white] | Filter: [cyan]%s[white] | Auto-scroll: [yellow]%s[white]%s | [grey]s[white]=toggle scroll [grey]1-4[white]=filter [grey]q[white]=quit",
		len(entries), levelStr, autoScrollStr, searchStr)

	p.statusBar.SetText(status)
}

// getLevelString returns a string representation of the current filter level
func (p *LogsPage) getLevelString() string {
	switch p.filterLevel {
	case slog.LevelDebug:
		return "ALL"
	case slog.LevelInfo:
		return "INFO+"
	case slog.LevelWarn:
		return "WARN+"
	case slog.LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ToggleAutoScroll toggles auto-scroll on/off
func (p *LogsPage) ToggleAutoScroll() {
	p.autoScroll = !p.autoScroll
	p.updateStatusBar()
}

// SetFilterLevel sets the minimum log level to display
func (p *LogsPage) SetFilterLevel(level slog.Level) {
	p.filterLevel = level
	p.Update()
}

// SetSearchTerm sets the search term for filtering logs
func (p *LogsPage) SetSearchTerm(term string) {
	p.searchTerm = term
	p.Update()
}

// SetupInputCapture sets up input handling for the logs page
func (p *LogsPage) SetupInputCapture(switchPage func(string)) {
	p.textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 's', 'S':
			p.ToggleAutoScroll()
			return nil
		case '1':
			p.SetFilterLevel(slog.LevelDebug) // All logs
			return nil
		case '2':
			p.SetFilterLevel(slog.LevelInfo) // INFO and above
			return nil
		case '3':
			p.SetFilterLevel(slog.LevelWarn) // WARN and above
			return nil
		case '4':
			p.SetFilterLevel(slog.LevelError) // ERROR only
			return nil
		}

		switch event.Key() {
		case tcell.KeyTab:
			switchPage("config")
			return nil
		case tcell.KeyBacktab:
			switchPage("overview")
			return nil
		}

		return event
	})
}
