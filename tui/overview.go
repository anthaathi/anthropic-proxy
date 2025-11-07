package tui

import (
	"anthropic-proxy/config"
	"anthropic-proxy/metrics"
	"anthropic-proxy/provider"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// OverviewPage represents the overview/dashboard page
type OverviewPage struct {
	*tview.Flex
	metricsTable    *tview.Table
	requestsTable   *tview.Table
	providersTable  *tview.Table
	summaryText     *tview.TextView
	tracker         *metrics.Tracker
	errorTracker    *metrics.ErrorTracker
	providerManager *provider.Manager
	cfg             *config.Config
	benchmarker     *metrics.Benchmarker
	isConfigWatching bool
}

// NewOverviewPage creates a new overview page
func NewOverviewPage(tracker *metrics.Tracker, errorTracker *metrics.ErrorTracker, providerManager *provider.Manager, cfg *config.Config, benchmarker *metrics.Benchmarker) *OverviewPage {
	page := &OverviewPage{
		Flex:            tview.NewFlex(),
		metricsTable:    tview.NewTable(),
		requestsTable:   tview.NewTable(),
		providersTable:  tview.NewTable(),
		summaryText:     tview.NewTextView(),
		tracker:         tracker,
		errorTracker:    errorTracker,
		providerManager: providerManager,
		cfg:             cfg,
		benchmarker:     benchmarker,
	}

	// Configure metrics table
	page.metricsTable.SetBorder(true).SetTitle(" Model Metrics (TPS) ")

	// Configure requests table
	page.requestsTable.SetBorder(true).SetTitle(" Recent Requests ")

	// Configure providers table
	page.providersTable.SetBorder(true).SetTitle(" Provider Status ")

	// Configure summary text
	page.summaryText.
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetBorder(true).
		SetTitle(" Summary ")

	// Layout: Top section with summary, middle row with metrics and requests side by side, bottom with providers
	topSection := page.summaryText

	middleRow := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(page.metricsTable, 0, 1, false).
		AddItem(page.requestsTable, 0, 1, false)

	page.SetDirection(tview.FlexRow).
		AddItem(topSection, 5, 0, false).
		AddItem(middleRow, 0, 1, false).
		AddItem(page.providersTable, 0, 1, false)

	return page
}

// Update refreshes the overview display
func (p *OverviewPage) Update() {
	p.updateSummary()
	p.updateMetricsTable()
	p.updateRequestsTable()
	p.updateProvidersTable()
}

// updateSummary updates the summary text
func (p *OverviewPage) updateSummary() {
	allErrors := p.errorTracker.GetAll()

	totalRequests := 0
	totalSuccess := 0
	totalErrors := 0

	for _, errorData := range allErrors {
		totalRequests += errorData.TotalRequests
		totalSuccess += errorData.SuccessCount
		totalErrors += errorData.ErrorCount
	}

	errorRate := 0.0
	if totalRequests > 0 {
		errorRate = float64(totalErrors) / float64(totalRequests) * 100
	}

	port := "8080" // default port
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	// Get benchmark status
	benchmarkStatus := ""
	if p.benchmarker != nil {
		status := p.benchmarker.GetStatus()
		benchmarkStatus = fmt.Sprintf(" [green]Bench:[white] %s", status.LastRunTime.Format("15:04:05"))
		if status.IsRunning {
			benchmarkStatus = " [yellow]Bench:[white] Running..."
		}
	}

	// Check if config watching is enabled
	configStatus := " [red]Config:[white] Static"
	if p.isConfigWatching {
		configStatus = " [green]Config:[white] Watching [yellow]🔄[white]"
	}

	summary := fmt.Sprintf(
		"[green]Server:[white] http://localhost:%s  [green]Providers:[white] %d  [green]Models:[white] %d%s%s\n"+
			"[green]Requests:[white] %d  [green]Success:[white] %d  [red]Errors:[white] %d  [yellow]Error Rate:[white] %.2f%%\n"+
			"[grey]Updated: %s  |  Press Tab to switch pages, q to quit",
		port,
		p.providerManager.Size(),
		len(p.cfg.Spec.Models),
		benchmarkStatus,
		configStatus,
		totalRequests,
		totalSuccess,
		totalErrors,
		errorRate,
		time.Now().Format("15:04:05"),
	)

	p.summaryText.SetText(summary)
}

// updateMetricsTable updates the metrics table
func (p *OverviewPage) updateMetricsTable() {
	allMetrics := p.tracker.GetAllMetrics()

	// Clear table
	p.metricsTable.Clear()

	// Set headers
	headers := []string{"Provider", "Model", "TPS", "Samples", "Weight", "Status"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false).
			SetExpansion(1)
		p.metricsTable.SetCell(0, col, cell)
	}

	// Convert map to slice for sorting
	var metricsList []*metrics.MetricData
	for _, metricData := range allMetrics {
		metricsList = append(metricsList, metricData)
	}

	// Sort by TPS (descending)
	sort.Slice(metricsList, func(i, j int) bool {
		return metricsList[i].TPS > metricsList[j].TPS
	})

	// Add data rows
	row := 1
	for _, metricData := range metricsList {
		// Find model weight
		weight := 1
		for _, model := range p.cfg.Spec.Models {
			if model.Provider == metricData.ProviderName && model.Name == metricData.ModelName {
				weight = model.GetWeight()
				break
			}
		}

		// Determine status based on TPS
		status := "[green]GOOD[white]"
		statusColor := tcell.ColorGreen
		if metricData.TPS < 40 {
			status = "[yellow]SLOW[white]"
			statusColor = tcell.ColorYellow
		}
		if metricData.TPS == 0 {
			status = "[grey]IDLE[white]"
			statusColor = tcell.ColorGray
		}

		// Add cells
		p.metricsTable.SetCell(row, 0, tview.NewTableCell(metricData.ProviderName).SetAlign(tview.AlignLeft))
		p.metricsTable.SetCell(row, 1, tview.NewTableCell(truncateString(metricData.ModelName, 30)).SetAlign(tview.AlignLeft))
		p.metricsTable.SetCell(row, 2, tview.NewTableCell(fmt.Sprintf("%.2f", metricData.TPS)).SetAlign(tview.AlignRight))
		p.metricsTable.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", len(metricData.Samples))).SetAlign(tview.AlignCenter))
		p.metricsTable.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%d", weight)).SetAlign(tview.AlignCenter))
		p.metricsTable.SetCell(row, 5, tview.NewTableCell(status).SetTextColor(statusColor).SetAlign(tview.AlignCenter))

		row++
	}

	if row == 1 {
		p.metricsTable.SetCell(1, 0, tview.NewTableCell("[grey]No metrics available yet...").
			SetAlign(tview.AlignCenter).
			SetExpansion(len(headers)))
	}
}

// updateRequestsTable updates the recent requests table
func (p *OverviewPage) updateRequestsTable() {
	allMetrics := p.tracker.GetAllMetrics()

	// Clear table
	p.requestsTable.Clear()

	// Set headers
	headers := []string{"Provider", "Model", "Tokens", "Duration", "TPS"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false).
			SetExpansion(1)
		p.requestsTable.SetCell(0, col, cell)
	}

	// Collect all samples from all metrics
	type RequestSample struct {
		Provider  string
		Model     string
		Tokens    int
		DurationS float64
		TPS       float64
		Timestamp time.Time
	}

	var allSamples []RequestSample
	for _, metricData := range allMetrics {
		for _, sample := range metricData.Samples {
			allSamples = append(allSamples, RequestSample{
				Provider:  metricData.ProviderName,
				Model:     metricData.ModelName,
				Tokens:    sample.Tokens,
				DurationS: sample.DurationS,
				TPS:       sample.TPS,
				Timestamp: sample.Timestamp,
			})
		}
	}

	// Sort by timestamp (most recent first)
	sort.Slice(allSamples, func(i, j int) bool {
		return allSamples[i].Timestamp.After(allSamples[j].Timestamp)
	})

	// Show the last 10 samples
	maxSamples := 10
	if len(allSamples) < maxSamples {
		maxSamples = len(allSamples)
	}

	row := 1
	for i := 0; i < maxSamples; i++ {
		sample := allSamples[i]

		// Determine TPS color
		tpsColor := tcell.ColorGreen
		if sample.TPS < 40 {
			tpsColor = tcell.ColorYellow
		}

		p.requestsTable.SetCell(row, 0, tview.NewTableCell(truncateString(sample.Provider, 15)).SetAlign(tview.AlignLeft))
		p.requestsTable.SetCell(row, 1, tview.NewTableCell(truncateString(sample.Model, 20)).SetAlign(tview.AlignLeft))
		p.requestsTable.SetCell(row, 2, tview.NewTableCell(fmt.Sprintf("%d", sample.Tokens)).SetAlign(tview.AlignRight))
		p.requestsTable.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%.2fs", sample.DurationS)).SetAlign(tview.AlignRight))
		p.requestsTable.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%.1f", sample.TPS)).SetTextColor(tpsColor).SetAlign(tview.AlignRight))

		row++
	}

	if row == 1 {
		p.requestsTable.SetCell(1, 0, tview.NewTableCell("[grey]No requests yet...").
			SetAlign(tview.AlignCenter).
			SetExpansion(len(headers)))
	}
}

// updateProvidersTable updates the providers table
func (p *OverviewPage) updateProvidersTable() {
	allErrors := p.errorTracker.GetAll()

	// Clear table
	p.providersTable.Clear()

	// Set headers
	headers := []string{"Provider", "Total Requests", "Success", "Errors", "Error Rate", "Last Error", "Status Code"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false).
			SetExpansion(1)
		p.providersTable.SetCell(0, col, cell)
	}

	// Get all providers
	providers := p.providerManager.GetAll()

	// Create a slice with provider data for sorting
	type ProviderStats struct {
		Provider      *provider.Provider
		TotalRequests int
	}

	var providerStats []ProviderStats
	for _, prov := range providers {
		totalReq := 0
		if errorData := allErrors[prov.Name]; errorData != nil {
			totalReq = errorData.TotalRequests
		}
		providerStats = append(providerStats, ProviderStats{
			Provider:      prov,
			TotalRequests: totalReq,
		})
	}

	// Sort by total requests (descending), then by name (ascending)
	sort.Slice(providerStats, func(i, j int) bool {
		if providerStats[i].TotalRequests == providerStats[j].TotalRequests {
			return providerStats[i].Provider.Name < providerStats[j].Provider.Name
		}
		return providerStats[i].TotalRequests > providerStats[j].TotalRequests
	})

	// Add data rows
	row := 1
	for _, stats := range providerStats {
		prov := stats.Provider
		errorData := allErrors[prov.Name]

		totalReq := 0
		success := 0
		errors := 0
		errorRate := 0.0
		lastError := "-"
		statusCode := "-"

		if errorData != nil {
			totalReq = errorData.TotalRequests
			success = errorData.SuccessCount
			errors = errorData.ErrorCount
			errorRate = errorData.ErrorRate * 100

			if !errorData.LastError.IsZero() {
				lastError = formatTimeSince(errorData.LastError)
			}
			if errorData.LastErrorStatus > 0 {
				statusCode = fmt.Sprintf("%d", errorData.LastErrorStatus)
			}
		}

		// Determine error rate color
		errorRateColor := tcell.ColorGreen
		if errorRate > 10 {
			errorRateColor = tcell.ColorRed
		} else if errorRate > 5 {
			errorRateColor = tcell.ColorYellow
		}

		p.providersTable.SetCell(row, 0, tview.NewTableCell(prov.Name).SetAlign(tview.AlignLeft))
		p.providersTable.SetCell(row, 1, tview.NewTableCell(fmt.Sprintf("%d", totalReq)).SetAlign(tview.AlignRight))
		p.providersTable.SetCell(row, 2, tview.NewTableCell(fmt.Sprintf("%d", success)).SetAlign(tview.AlignRight).SetTextColor(tcell.ColorGreen))
		p.providersTable.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", errors)).SetAlign(tview.AlignRight).SetTextColor(tcell.ColorRed))
		p.providersTable.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%.1f%%", errorRate)).SetAlign(tview.AlignRight).SetTextColor(errorRateColor))
		p.providersTable.SetCell(row, 5, tview.NewTableCell(lastError).SetAlign(tview.AlignCenter))
		p.providersTable.SetCell(row, 6, tview.NewTableCell(statusCode).SetAlign(tview.AlignCenter))

		row++
	}

	if row == 1 {
		p.providersTable.SetCell(1, 0, tview.NewTableCell("[grey]No providers configured...").
			SetAlign(tview.AlignCenter).
			SetExpansion(len(headers)))
	}
}

// SetupInputCapture sets up input handling for the overview page
func (p *OverviewPage) SetupInputCapture(switchPage func(string)) {
	p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			switchPage("config")
			return nil
		case tcell.KeyBacktab:
			switchPage("config")
			return nil
		}
		return event
	})
}

// SetConfigWatching sets whether config watching is enabled
func (p *OverviewPage) SetConfigWatching(watching bool) {
	p.isConfigWatching = watching
}

// Helper functions

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatTimeSince(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return fmt.Sprintf("%ds ago", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm ago", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(duration.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(duration.Hours()/24))
}
