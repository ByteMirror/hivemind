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
)

// RenderAutomationsList renders the automations manager screen as a table.
func RenderAutomationsList(automations []*config.Automation, selectedIdx int, width int) string {
	var sb strings.Builder

	sb.WriteString(autoHeaderStyle.Render("Automations") + "\n")
	sb.WriteString(autoHintStyle.Render("n: new  e: toggle  r: run now  d: delete  esc: back") + "\n\n")

	if len(automations) == 0 {
		sb.WriteString(autoDisabledStyle.Render("No automations yet. Press 'n' to create one."))
		return autoBorderStyle.Width(width - 4).Render(sb.String())
	}

	for i, auto := range automations {
		row := renderAutomationRow(auto, i == selectedIdx, width)
		sb.WriteString(row + "\n")
	}

	return autoBorderStyle.Width(width - 4).Render(sb.String())
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
	scheduleStr := auto.Schedule

	// Truncate name if needed.
	name := auto.Name
	maxNameLen := 24
	if len(name) > maxNameLen {
		name = name[:maxNameLen-1] + "…"
	}

	row := fmt.Sprintf("  %s  %-26s  %-14s  next: %s",
		enabledMark, name, scheduleStr, nextRunStr)

	_ = width // width reserved for future truncation
	return rowStyle.Render(row)
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
