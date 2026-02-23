package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ByteMirror/hivemind/config"
	"github.com/charmbracelet/lipgloss"
)

var (
	autoHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F0A868")).
			MarginBottom(1)

	autoSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#1A1A1A")).
				Background(lipgloss.Color("#F0A868"))

	autoNormalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#DDDDDD"))

	autoDisabledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666666"))

	autoHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)

	autoBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#F0A868")).
			Padding(1, 2)

	autoColumnHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#F0A868")).
				Underline(true)

	autoDividerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#444444"))
)

// RenderAutomationsList renders the automations manager as a large modal.
func RenderAutomationsList(automations []*config.Automation, selectedIdx int, width int, height int) string {
	borderFrame := autoBorderStyle.GetHorizontalFrameSize()
	innerWidth := width - borderFrame
	if innerWidth < 20 {
		innerWidth = 20
	}

	var sb strings.Builder

	// Header
	sb.WriteString(autoHeaderStyle.Render("⚡ Automations") + "\n")
	sb.WriteString(autoHintStyle.Render("n new  e toggle  r run now  d delete  esc close") + "\n")
	sb.WriteString(autoDividerStyle.Render(strings.Repeat("─", innerWidth)) + "\n\n")

	if len(automations) == 0 {
		empty := autoDisabledStyle.Render("No automations yet. Press 'n' to create one.\n\nAutomations let you schedule recurring agent tasks — e.g. run a daily\ncode review, sync documentation, or monitor for regressions.")
		sb.WriteString(empty)
	} else {
		// Column header
		col := renderColumnHeader(innerWidth)
		sb.WriteString(col + "\n")
		sb.WriteString(autoDividerStyle.Render(strings.Repeat("─", innerWidth)) + "\n")

		for i, auto := range automations {
			row := renderAutomationRow(auto, i == selectedIdx, innerWidth)
			sb.WriteString(row + "\n")
		}
	}

	vertFrame := autoBorderStyle.GetVerticalFrameSize()
	innerHeight := height - vertFrame
	if innerHeight < 5 {
		innerHeight = 5
	}

	return autoBorderStyle.Width(innerWidth).Height(innerHeight).Render(sb.String())
}

func renderColumnHeader(width int) string {
	name := fmt.Sprintf("%-28s", "NAME")
	schedule := fmt.Sprintf("%-16s", "SCHEDULE")
	nextRun := fmt.Sprintf("%-12s", "NEXT RUN")
	status := "STATUS"
	row := fmt.Sprintf("  %s  %s  %s  %s", name, schedule, nextRun, status)
	_ = width
	return autoColumnHeaderStyle.Render(row)
}

// renderAutomationRow renders a single automation row.
func renderAutomationRow(auto *config.Automation, selected bool, width int) string {
	enabledMark := "●"
	rowStyle := autoNormalStyle
	if !auto.Enabled {
		enabledMark = "○"
		rowStyle = autoDisabledStyle
	}
	if selected {
		rowStyle = autoSelectedStyle
	}

	nextRunStr := formatNextRun(auto.NextRun)

	name := auto.Name
	if len(name) > 26 {
		name = name[:25] + "…"
	}
	schedule := auto.Schedule
	if len(schedule) > 14 {
		schedule = schedule[:13] + "…"
	}

	row := fmt.Sprintf("  %s %-27s  %-16s  %-12s  %s",
		enabledMark, name, schedule, nextRunStr,
		enabledText(auto.Enabled))

	if selected {
		// Pad to full width so the highlight bar spans the row
		visLen := len([]rune(row))
		if visLen < width {
			row += strings.Repeat(" ", width-visLen)
		}
	}

	return rowStyle.Render(row)
}

func enabledText(enabled bool) string {
	if enabled {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Render("enabled")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("disabled")
}

// formatNextRun formats the NextRun time into a human-readable relative string.
func formatNextRun(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Until(t)
	if d <= 0 {
		return "now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	return fmt.Sprintf("%dd", days)
}
