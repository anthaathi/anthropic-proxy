package tui

import (
	"anthropic-proxy/config"
	"anthropic-proxy/metrics"
	"anthropic-proxy/provider"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// BenchmarkPage represents the benchmark overview page
type BenchmarkPage struct {
	*tview.Flex
	summaryText     *tview.TextView
	resultsTable    *tview.Table
	historyTable    *tview.Table
	tracker         *metrics.Tracker
	providerManager *provider.Manager
	cfg             *config.Config
	benchmarker     *metrics.Benchmarker
}

// NewBenchmarkPage creates a new benchmark page
func NewBenchmarkPage(tracker *metrics.Tracker, providerManager *provider.Manager, cfg *config.Config, benchmarker *metrics.Benchmarker) *BenchmarkPage {
	page := &BenchmarkPage{
		Flex:            tview.NewFlex(),
		summaryText:     tview.NewTextView(),
		resultsTable:    tview.NewTable(),
		historyTable:    tview.NewTable(),
		tracker:         tracker,
		providerManager: providerManager,
		cfg:             cfg,
		benchmarker:     benchmarker,
	}

	// Configure summary text
	page.summaryText.
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetBorder(true).
		SetTitle(" Benchmark Summary ")

	// Configure results table
	page.resultsTable.SetBorder(true).SetTitle(" Latest Benchmark Results ")

	// Configure history table
	page.historyTable.SetBorder(true).SetTitle(" Benchmark History ")

	// Layout: Top section with summary, middle row with results and history side by side
	topSection := page.summaryText

	middleRow := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(page.resultsTable, 0, 1, false).
		AddItem(page.historyTable, 0, 1, false)

	page.SetDirection(tview.FlexRow).
		AddItem(topSection, 7, 0, false).
		AddItem(middleRow, 0, 1, false)

	return page
}

// Update refreshes the benchmark display
func (p *BenchmarkPage) Update() {
	p.updateSummary()
	p.updateResultsTable()
	p.updateHistoryTable()
}

// updateSummary updates the summary text
func (p *BenchmarkPage) updateSummary() {
	totalProviderModels := 0
	healthyProviderModels := 0
	lastBenchmarkTime := "Never"
	nextBenchmarkTime := "Unknown"
	totalBenchmarks := 0
	successCount := 0
	failureCount := 0

	// Count total provider-model combinations and healthy ones
	for _, model := range p.cfg.Spec.Models {
		_, exists := p.providerManager.Get(model.Provider)
		if exists {
			totalProviderModels++
			tps := p.tracker.GetTPS(model.Provider, model.Name)
			if tps > 0 && tps >= 40 {
				healthyProviderModels++
			}
		}
	}

	// Get benchmark status from benchmarker
	if p.benchmarker != nil {
		status := p.benchmarker.GetStatus()
		if !status.LastRunTime.IsZero() {
			lastBenchmarkTime = formatTimeSince(status.LastRunTime)
		}
		if !status.NextRunTime.IsZero() {
			if time.Now().Before(status.NextRunTime) {
				nextBenchmarkTime = fmt.Sprintf("In %s", formatDuration(time.Until(status.NextRunTime)))
			} else {
				nextBenchmarkTime = "Soon"
			}
		}
		totalBenchmarks = status.TotalBenchmarks
		successCount = status.SuccessCount
		failureCount = status.FailureCount
	}

	successRate := 0.0
	if totalProviderModels > 0 {
		successRate = float64(healthyProviderModels) / float64(totalProviderModels) * 100
	}

	benchmarkSuccessRate := 0.0
	if totalBenchmarks > 0 {
		benchmarkSuccessRate = float64(successCount) / float64(totalBenchmarks) * 100
	}

	summary := fmt.Sprintf(
		"[green]Provider-Models:[white] %d  [green]Healthy:[white] %d  [yellow]Success Rate:[white] %.1f%%\n"+
			"[green]Last Benchmark:[white] %s  [green]Next Benchmark:[white] %s\n"+
			"[green]Total Benchmarks:[white] %d  [green]Success:[white] %d  [red]Failed:[white] %d  [yellow]Bench Success:[white] %.1f%%\n"+
			"[grey]Press 'r' to run manual benchmark, Tab/1-4 to switch pages, q to quit",
		totalProviderModels,
		healthyProviderModels,
		successRate,
		lastBenchmarkTime,
		nextBenchmarkTime,
		totalBenchmarks,
		successCount,
		failureCount,
		benchmarkSuccessRate,
	)

	p.summaryText.SetText(summary)
}

// updateResultsTable updates the latest benchmark results table
func (p *BenchmarkPage) updateResultsTable() {
	allMetrics := p.tracker.GetAllMetrics()

	// Clear table
	p.resultsTable.Clear()

	// Set headers
	headers := []string{"Provider", "Model", "TPS", "Tokens", "Duration", "Last Run", "Status"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false).
			SetExpansion(1)
		p.resultsTable.SetCell(0, col, cell)
	}

	// Convert map to slice for sorting
	type BenchmarkResult struct {
		Provider     string
		Model        string
		TPS          float64
		Tokens       int
		Duration     float64
		LastRun      time.Time
		SampleCount  int
	}

	var results []BenchmarkResult
	for _, metricData := range allMetrics {
		if len(metricData.Samples) > 0 {
			// Get the most recent sample
			latestSample := metricData.Samples[len(metricData.Samples)-1]
			results = append(results, BenchmarkResult{
				Provider:     metricData.ProviderName,
				Model:        metricData.ModelName,
				TPS:          metricData.TPS,
				Tokens:       latestSample.Tokens,
				Duration:     latestSample.DurationS,
				LastRun:      latestSample.Timestamp,
				SampleCount:  len(metricData.Samples),
			})
		}
	}

	// Sort by TPS (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].TPS > results[j].TPS
	})

	// Add data rows
	row := 1
	for _, result := range results {
		// Determine status based on TPS
		status := "[green]GOOD[white]"
		statusColor := tcell.ColorGreen
		if result.TPS < 40 {
			status = "[yellow]SLOW[white]"
			statusColor = tcell.ColorYellow
		}
		if result.TPS == 0 {
			status = "[red]FAILED[white]"
			statusColor = tcell.ColorRed
		}

		// Add cells
		p.resultsTable.SetCell(row, 0, tview.NewTableCell(result.Provider).SetAlign(tview.AlignLeft))
		p.resultsTable.SetCell(row, 1, tview.NewTableCell(truncateString(result.Model, 30)).SetAlign(tview.AlignLeft))
		p.resultsTable.SetCell(row, 2, tview.NewTableCell(fmt.Sprintf("%.2f", result.TPS)).SetAlign(tview.AlignRight))
		p.resultsTable.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", result.Tokens)).SetAlign(tview.AlignRight))
		p.resultsTable.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%.2fs", result.Duration)).SetAlign(tview.AlignRight))
		p.resultsTable.SetCell(row, 5, tview.NewTableCell(formatTimeSince(result.LastRun)).SetAlign(tview.AlignCenter))
		p.resultsTable.SetCell(row, 6, tview.NewTableCell(status).SetTextColor(statusColor).SetAlign(tview.AlignCenter))

		row++
	}

	if row == 1 {
		p.resultsTable.SetCell(1, 0, tview.NewTableCell("[grey]No benchmark results available yet...").
			SetAlign(tview.AlignCenter).
			SetExpansion(len(headers)))
	}
}

// updateHistoryTable updates the benchmark history table
func (p *BenchmarkPage) updateHistoryTable() {
	// Clear table
	p.historyTable.Clear()

	// Set headers
	headers := []string{"Time", "Provider", "Model", "TPS", "Tokens", "Duration", "Status"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false).
			SetExpansion(1)
		p.historyTable.SetCell(0, col, cell)
	}

	// Get benchmark history
	var history []metrics.BenchmarkResult
	if p.benchmarker != nil {
		history = p.benchmarker.GetHistory()
	}

	// Sort by time (most recent first)
	sort.Slice(history, func(i, j int) bool {
		return history[i].Timestamp.After(history[j].Timestamp)
	})

	// Show the last 20 entries
	maxEntries := 20
	if len(history) < maxEntries {
		maxEntries = len(history)
	}

	row := 1
	for i := 0; i < maxEntries; i++ {
		entry := history[i]

		// Determine TPS and status colors
		tpsColor := tcell.ColorGreen
		statusColor := tcell.ColorGreen
		statusText := "SUCCESS"

		if entry.TPS < 40 && entry.TPS > 0 {
			tpsColor = tcell.ColorYellow
		}
		if entry.TPS == 0 {
			tpsColor = tcell.ColorRed
		}

		if !entry.Success {
			statusColor = tcell.ColorRed
			statusText = "FAILED"
			if strings.Contains(entry.ErrorMessage, "Skipped") {
				statusColor = tcell.ColorYellow
				statusText = "SKIPPED"
			}
		}

		p.historyTable.SetCell(row, 0, tview.NewTableCell(entry.Timestamp.Format("15:04:05")).SetAlign(tview.AlignCenter))
		p.historyTable.SetCell(row, 1, tview.NewTableCell(truncateString(entry.Provider, 15)).SetAlign(tview.AlignLeft))
		p.historyTable.SetCell(row, 2, tview.NewTableCell(truncateString(entry.Model, 20)).SetAlign(tview.AlignLeft))
		p.historyTable.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%.1f", entry.TPS)).SetTextColor(tpsColor).SetAlign(tview.AlignRight))
		p.historyTable.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%d", entry.Tokens)).SetAlign(tview.AlignRight))
		p.historyTable.SetCell(row, 5, tview.NewTableCell(fmt.Sprintf("%.2fs", entry.DurationS)).SetAlign(tview.AlignRight))
		p.historyTable.SetCell(row, 6, tview.NewTableCell(statusText).SetTextColor(statusColor).SetAlign(tview.AlignCenter))

		row++
	}

	if row == 1 {
		p.historyTable.SetCell(1, 0, tview.NewTableCell("[grey]No benchmark history available...").
			SetAlign(tview.AlignCenter).
			SetExpansion(len(headers)))
	}
}

// SetupInputCapture sets up input handling for the benchmark page
func (p *BenchmarkPage) SetupInputCapture(switchPage func(string)) {
	p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			switchPage("overview") // Cycle back to overview
			return nil
		case tcell.KeyBacktab:
			switchPage("config") // Go to config on backtab
			return nil
		}
		switch event.Rune() {
		case 'r', 'R':
			// Run manual benchmark
			p.runManualBenchmark()
			return nil
		}
		return event
	})
}

// runManualBenchmark triggers a manual benchmark run
func (p *BenchmarkPage) runManualBenchmark() {
	if p.benchmarker != nil {
		// Show "Running..." status temporarily by getting current text and adding status
		currentText := p.summaryText.GetText(false)
		p.summaryText.SetText(currentText + "\n[yellow]⏡ Running manual benchmark...[white]")

		// Run benchmark in goroutine to avoid blocking UI
		go func() {
			p.benchmarker.RunManualBenchmark()

			// Update after a short delay to show it completed
			time.Sleep(2 * time.Second)
			p.summaryText.SetText(currentText + "\n[green]✓ Manual benchmark triggered![white]")
		}()
	}
}

// Helper functions

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}