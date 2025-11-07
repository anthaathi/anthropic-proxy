package tui

import (
	"anthropic-proxy/config"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConfigPage represents the configuration viewer page
type ConfigPage struct {
	*tview.Flex
	textView *tview.TextView
	cfg      *config.Config
}

// NewConfigPage creates a new config page
func NewConfigPage(cfg *config.Config) *ConfigPage {
	page := &ConfigPage{
		Flex:     tview.NewFlex(),
		textView: tview.NewTextView(),
		cfg:      cfg,
	}

	// Configure text view
	page.textView.
		SetDynamicColors(true).
		SetScrollable(true).
		SetWordWrap(true)

	page.textView.SetBorder(true).SetTitle(" Configuration (Read-Only) ")

	// Layout
	page.SetDirection(tview.FlexRow).
		AddItem(page.textView, 0, 1, true)

	// Initial render
	page.Update()

	return page
}

// Update refreshes the configuration display
func (p *ConfigPage) Update() {
	var builder strings.Builder

	// Title
	builder.WriteString("[yellow::b]Anthropic Proxy Configuration[white]\n\n")

	// Providers section
	builder.WriteString("[cyan::b]Providers:[white]\n")
	if len(p.cfg.Spec.Providers) == 0 {
		builder.WriteString("  [grey]No providers configured[white]\n")
	} else {
		for name, provider := range p.cfg.Spec.Providers {
			builder.WriteString(fmt.Sprintf("  [green]%s[white]\n", name))
			builder.WriteString(fmt.Sprintf("    Endpoint: [grey]%s[white]\n", provider.Endpoint))

			// Mask API key for security
			apiKeyDisplay := maskAPIKey(provider.APIKey)
			builder.WriteString(fmt.Sprintf("    API Key:  [grey]%s[white]\n", apiKeyDisplay))
			builder.WriteString("\n")
		}
	}

	// Models section
	builder.WriteString("[cyan::b]Models:[white]\n")
	if len(p.cfg.Spec.Models) == 0 {
		builder.WriteString("  [grey]No models configured[white]\n")
	} else {
		for i, model := range p.cfg.Spec.Models {
			builder.WriteString(fmt.Sprintf("  [yellow]Model %d:[white]\n", i+1))
			builder.WriteString(fmt.Sprintf("    Name:     [grey]%s[white]\n", model.Name))
			builder.WriteString(fmt.Sprintf("    Alias:    [grey]%s[white]\n", model.Alias))
			builder.WriteString(fmt.Sprintf("    Provider: [grey]%s[white]\n", model.Provider))
			builder.WriteString(fmt.Sprintf("    Context:  [grey]%d[white]\n", model.Context))
			builder.WriteString(fmt.Sprintf("    Weight:   [grey]%d[white]\n", model.GetWeight()))
			builder.WriteString("\n")
		}
	}

	// Retry configuration
	builder.WriteString("[cyan::b]Retry Configuration:[white]\n")
	if p.cfg.Spec.Retry != nil {
		builder.WriteString(fmt.Sprintf("  Max Retries:        [grey]%d[white]\n", p.cfg.Spec.Retry.MaxRetries))
		builder.WriteString(fmt.Sprintf("  Initial Delay:      [grey]%s[white]\n", p.cfg.Spec.Retry.InitialDelay))
		builder.WriteString(fmt.Sprintf("  Max Delay:          [grey]%s[white]\n", p.cfg.Spec.Retry.MaxDelay))
		builder.WriteString(fmt.Sprintf("  Backoff Multiplier: [grey]%.1f[white]\n", p.cfg.Spec.Retry.BackoffMultiplier))
		builder.WriteString(fmt.Sprintf("  Retry Same Provider:[grey]%t[white]\n", p.cfg.Spec.Retry.RetrySameProvider))
	} else {
		builder.WriteString("  [grey]Using default failover-first strategy (no same-provider retries)[white]\n")
	}
	builder.WriteString("\n")

	// API Keys section
	builder.WriteString("[cyan::b]API Keys:[white]\n")
	if len(p.cfg.Spec.APIKeys) == 0 {
		builder.WriteString("  [grey]No API keys configured[white]\n")
	} else {
		for i, key := range p.cfg.Spec.APIKeys {
			maskedKey := maskAPIKey(key)
			builder.WriteString(fmt.Sprintf("  [grey]%d. %s[white]\n", i+1, maskedKey))
		}
	}
	builder.WriteString("\n")

	// Routing info
	builder.WriteString("[cyan::b]Routing Logic:[white]\n")
	builder.WriteString("  1. Models with TPS < 40 are excluded\n")
	builder.WriteString("  2. Models are sorted by weight (higher first)\n")
	builder.WriteString("  3. If weights are equal, higher TPS wins\n")
	builder.WriteString("  4. On failure, immediately tries the next provider (unless same-provider retries are enabled)\n")
	builder.WriteString("  5. Continues until a provider succeeds or all providers fail\n")
	builder.WriteString("\n")

	// Help text
	builder.WriteString("[grey]Press Tab to return to Overview | q to quit[white]")

	p.textView.SetText(builder.String())
}

// SetupInputCapture sets up input handling for the config page
func (p *ConfigPage) SetupInputCapture(switchPage func(string)) {
	p.textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			switchPage("overview")
			return nil
		case tcell.KeyBacktab:
			switchPage("overview")
			return nil
		}
		return event
	})
}

// Helper functions

// maskAPIKey masks an API key for display, showing only first and last few characters
func maskAPIKey(key string) string {
	if key == "" {
		return "[grey]<empty>[white]"
	}

	// Handle environment variable references
	if strings.HasPrefix(key, "env.") {
		return fmt.Sprintf("[grey]<from env: %s>[white]", key)
	}

	// Handle random key placeholder
	if key == "$RANDOM_KEY" {
		return "[grey]<auto-generated>[white]"
	}

	// Mask actual keys
	if len(key) <= 12 {
		return strings.Repeat("*", len(key))
	}

	// Show first 4 and last 4 characters
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}
