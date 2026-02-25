package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ByteMirror/hivemind/config"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Column widths for the automations table (visual terminal columns).
const (
	autoColMark     = 2
	autoColName     = 27
	autoColSchedule = 16
	autoColNextRun  = 12
	autoColStatus   = 8
	autoColSep      = 2

	// Total table width: mark + name + sep + schedule + sep + nextRun + sep + status
	autoTableWidth = autoColMark + autoColName + autoColSep + autoColSchedule + autoColSep + autoColNextRun + autoColSep + autoColStatus // 71

	// AutomationsListWidth is the outer rendered width of the modal (table + padding + border).
	AutomationsListWidth = autoTableWidth + 4 + 2 // 77  (padding=2*2, border=2*1)
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

// colSlot renders s into exactly width terminal columns, truncating or padding as needed.
func colSlot(s string, width int) string {
	return runewidth.FillRight(runewidth.Truncate(s, width, "…"), width)
}

// RenderAutomationsList renders the automations manager as a modal.
// When form is non-nil the create/edit form is shown instead of the list.
func RenderAutomationsList(automations []*config.Automation, selectedIdx int, width int, height int, form *AutomationForm) string {
	// Width calculation:
	//   lipgloss Width(n) sets the space BETWEEN borders (includes padding, excludes border chars).
	//   borderChars = left border + right border (1+1 = 2 for rounded)
	//   hPad        = total horizontal frame - borderChars (padding only)
	//   styleWidth  = width - borderChars  → value to pass to .Width()
	//   textWidth   = styleWidth - hPad    → actual text content area
	borderChars := autoBorderStyle.GetBorderLeftSize() + autoBorderStyle.GetBorderRightSize()
	hPad := autoBorderStyle.GetHorizontalFrameSize() - borderChars
	styleWidth := width - borderChars
	textWidth := styleWidth - hPad

	vertFrame := autoBorderStyle.GetVerticalFrameSize()
	innerHeight := height - vertFrame
	if textWidth < 20 {
		textWidth = 20
		styleWidth = textWidth + hPad
	}
	if innerHeight < 5 {
		innerHeight = 5
	}

	if form != nil {
		content := form.Render(textWidth, innerHeight)
		// Hard clamp: lipgloss Height() is a minimum, not a maximum.
		// Truncate the content to innerHeight lines so it never overflows the border.
		lines := strings.Split(content, "\n")
		if len(lines) > innerHeight {
			lines = lines[:innerHeight]
			content = strings.Join(lines, "\n")
		}
		return autoBorderStyle.Width(styleWidth).Height(innerHeight).Render(content)
	}

	return renderList(automations, selectedIdx, textWidth, innerHeight, styleWidth)
}

func renderList(automations []*config.Automation, selectedIdx int, textWidth, innerHeight, styleWidth int) string {
	var sb strings.Builder

	// Header
	sb.WriteString(autoHeaderStyle.Render("⚡ Automations") + "\n")
	sb.WriteString(autoHintStyle.Render("n new  e edit  t toggle  r run now  d delete  esc close") + "\n")
	sb.WriteString(autoDividerStyle.Render(strings.Repeat("─", textWidth)) + "\n\n")

	if len(automations) == 0 {
		empty := autoDisabledStyle.Render(wrapWords("No automations yet. Press 'n' to create one. Automations let you schedule recurring agent tasks — e.g. run a daily code review, sync documentation, or monitor for regressions.", textWidth))
		sb.WriteString(empty)
	} else {
		col := renderColumnHeader()
		sb.WriteString(col + "\n")
		sb.WriteString(autoDividerStyle.Render(strings.Repeat("─", textWidth)) + "\n")

		for i, auto := range automations {
			row := renderAutomationRow(auto, i == selectedIdx, textWidth)
			sb.WriteString(row + "\n")
		}
	}

	return autoBorderStyle.Width(styleWidth).Height(innerHeight).Render(sb.String())
}

func renderColumnHeader() string {
	mark := colSlot("", autoColMark)
	name := colSlot("NAME", autoColName)
	schedule := colSlot("SCHEDULE", autoColSchedule)
	nextRun := colSlot("NEXT RUN", autoColNextRun)
	status := colSlot("STATUS", autoColStatus)
	row := mark + name + "  " + schedule + "  " + nextRun + "  " + status
	return autoColumnHeaderStyle.Render(row)
}

// renderAutomationRow renders a single automation row using slot-based columns.
func renderAutomationRow(auto *config.Automation, selected bool, textWidth int) string {
	enabledMark := "●"
	rowStyle := autoNormalStyle
	if !auto.Enabled {
		enabledMark = "○"
		rowStyle = autoDisabledStyle
	}
	if selected {
		rowStyle = autoSelectedStyle
	}

	mark := colSlot(enabledMark, autoColMark)
	name := colSlot(auto.Name, autoColName)
	schedule := colSlot(auto.Schedule, autoColSchedule)
	nextRun := colSlot(formatNextRun(auto.NextRun), autoColNextRun)
	status := colSlot(enabledTextPlain(auto.Enabled), autoColStatus)

	row := mark + name + "  " + schedule + "  " + nextRun + "  " + status

	if selected {
		// Pad to full textWidth so the highlight bar spans the row
		visLen := runewidth.StringWidth(row)
		if visLen < textWidth {
			row += strings.Repeat(" ", textWidth-visLen)
		}
	}

	return rowStyle.Render(row)
}

// enabledTextPlain returns a plain string (no ANSI) for the status column.
// Colour is applied by the row style instead, so it doesn't break the highlight background.
func enabledTextPlain(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
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

// wrapWords wraps text to maxWidth using terminal-aware width measurement.
func wrapWords(text string, maxWidth int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if runewidth.StringWidth(line)+1+runewidth.StringWidth(w) > maxWidth {
			lines = append(lines, line)
			line = w
		} else {
			line += " " + w
		}
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n")
}
