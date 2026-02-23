package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var settingsStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#F0A868")).
	Padding(0, 1)

var settingsTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#F0A868")).
	MarginBottom(1)

// No padding — widths are controlled explicitly in the render loop.
var settingsItemStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#dddddd"))

var settingsSelectedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#1a1a1a")).
	Background(lipgloss.Color("#7EC8D8"))

var settingsValueOnStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#36CFC9")).
	Bold(true)

var settingsValueOffStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#555555"))

var settingsValueStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFCC00"))

var settingsEditStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#1a1a1a")).
	Background(lipgloss.Color("#FFCC00"))

var settingsHintStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#666666")).
	MarginTop(1)

var settingsDescStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#777777")).
	Italic(true)

var settingsSeparatorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#333333"))

// SettingType defines the kind of setting control.
type SettingType int

const (
	SettingToggle SettingType = iota // ON/OFF switch
	SettingPicker                    // Cycles through Options list
	SettingText                      // Editable text field
)

// SettingItem represents a single configurable setting.
type SettingItem struct {
	Label       string
	Description string
	Type        SettingType
	Value       string   // "true"/"false" for toggle, text for others
	Options     []string // For SettingPicker only
	Key         string   // Identifier, e.g. "notifications_enabled"
}

// SettingsOverlay is an interactive settings screen.
type SettingsOverlay struct {
	items       []SettingItem
	selectedIdx int
	viewOffset  int // first visible item index for scrolling
	maxVisible  int
	width       int
	editing     bool   // true when editing a text field
	editBuffer  string // buffer for text editing
}

// NewSettingsOverlay creates a new settings overlay with the given items.
func NewSettingsOverlay(items []SettingItem) *SettingsOverlay {
	return &SettingsOverlay{
		items:      items,
		width:      60,
		maxVisible: 6,
	}
}

// clampViewOffset slides the visible window to keep selectedIdx in view.
func (s *SettingsOverlay) clampViewOffset() {
	if s.selectedIdx >= s.viewOffset+s.maxVisible {
		s.viewOffset = s.selectedIdx - s.maxVisible + 1
	}
	if s.selectedIdx < s.viewOffset {
		s.viewOffset = s.selectedIdx
	}
	if s.viewOffset < 0 {
		s.viewOffset = 0
	}
}

// HandleKeyPress processes key events.
// Returns (changedKey, closed): changedKey is non-empty when a setting value changed.
func (s *SettingsOverlay) HandleKeyPress(msg tea.KeyMsg) (string, bool) {
	if s.editing {
		return s.handleEditMode(msg)
	}

	switch msg.String() {
	case "esc":
		return "", true
	case "up", "k":
		if s.selectedIdx > 0 {
			s.selectedIdx--
			s.clampViewOffset()
		}
	case "down", "j":
		if s.selectedIdx < len(s.items)-1 {
			s.selectedIdx++
			s.clampViewOffset()
		}
	case "enter", " ":
		if s.selectedIdx >= len(s.items) {
			return "", false
		}
		item := &s.items[s.selectedIdx]
		switch item.Type {
		case SettingToggle:
			if item.Value == "true" {
				item.Value = "false"
			} else {
				item.Value = "true"
			}
			return item.Key, false
		case SettingPicker:
			if len(item.Options) > 0 {
				currentIdx := 0
				for i, opt := range item.Options {
					if opt == item.Value {
						currentIdx = i
						break
					}
				}
				item.Value = item.Options[(currentIdx+1)%len(item.Options)]
				return item.Key, false
			}
		case SettingText:
			s.editing = true
			s.editBuffer = item.Value
			return "", false
		}
	}
	return "", false
}

func (s *SettingsOverlay) handleEditMode(msg tea.KeyMsg) (string, bool) {
	switch msg.String() {
	case "esc":
		s.editing = false
		s.editBuffer = ""
		return "", false
	case "enter":
		item := &s.items[s.selectedIdx]
		item.Value = s.editBuffer
		s.editing = false
		s.editBuffer = ""
		return item.Key, false
	case "backspace":
		if len(s.editBuffer) > 0 {
			runes := []rune(s.editBuffer)
			s.editBuffer = string(runes[:len(runes)-1])
		}
	default:
		if msg.Type == tea.KeyRunes {
			s.editBuffer += string(msg.Runes)
		} else if msg.Type == tea.KeySpace {
			s.editBuffer += " "
		}
	}
	return "", false
}

// GetItem returns a pointer to the setting item with the given key.
func (s *SettingsOverlay) GetItem(key string) *SettingItem {
	for i := range s.items {
		if s.items[i].Key == key {
			return &s.items[i]
		}
	}
	return nil
}

// SetSize sets the display dimensions for the overlay.
func (s *SettingsOverlay) SetSize(width, height int) {
	if width > 20 {
		s.width = width
	}
	if height > 10 {
		// Each item takes 2 lines (label+value row, description row).
		// Overhead: title(2) + separator(1) + hint(1) + border+padding(2) = 6
		s.maxVisible = (height - 6) / 2
		if s.maxVisible < 1 {
			s.maxVisible = 1
		}
	}
}

// Render returns the styled settings string.
func (s *SettingsOverlay) Render() string {
	var b strings.Builder

	b.WriteString(settingsTitleStyle.Render("Settings"))
	b.WriteString("\n")

	innerWidth := s.width - 4 // border(1×2) + padding(1×2)
	if innerWidth < 20 {
		innerWidth = 20
	}

	// Separator line under title.
	b.WriteString(settingsSeparatorStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// Pre-compute value column width from all possible values (toggles, pickers, text).
	valueColWidth := 5 // minimum: "[ON]"
	for _, item := range s.items {
		switch item.Type {
		case SettingToggle:
			if w := 5; w > valueColWidth { // "[OFF]" = 5 chars
				valueColWidth = w
			}
		case SettingPicker:
			for _, opt := range item.Options {
				if w := len([]rune(opt)); w > valueColWidth {
					valueColWidth = w
				}
			}
		case SettingText:
			if w := len([]rune(item.Value)); w > valueColWidth {
				valueColWidth = w
			}
		}
	}
	// indicator(2) + " "(1) + label + " "(2) + value(valueColWidth)
	const rowOverhead = 5
	labelColWidth := innerWidth - rowOverhead - valueColWidth
	if labelColWidth < 10 {
		labelColWidth = 10
	}

	// Visible window.
	end := s.viewOffset + s.maxVisible
	if end > len(s.items) {
		end = len(s.items)
	}
	visible := s.items[s.viewOffset:end]

	for i, item := range visible {
		absIdx := i + s.viewOffset
		isSelected := absIdx == s.selectedIdx

		indicator := "  "
		if isSelected {
			indicator = " \u25b8"
		}

		// Build styled value.
		var valueStr string
		switch item.Type {
		case SettingToggle:
			if item.Value == "true" {
				valueStr = settingsValueOnStyle.Render("[ON]")
			} else {
				valueStr = settingsValueOffStyle.Render("[OFF]")
			}
		case SettingPicker:
			valueStr = settingsValueStyle.Render(item.Value)
		case SettingText:
			if s.editing && isSelected {
				displayVal := s.editBuffer
				if displayVal == "" {
					displayVal = " "
				}
				valueStr = settingsEditStyle.Render(displayVal)
			} else {
				valueStr = settingsValueStyle.Render(item.Value)
			}
		}

		label := cmdTruncatePad(item.Label, labelColWidth)

		// Label + value row.
		if isSelected && !s.editing {
			b.WriteString(settingsSelectedStyle.Render(indicator+" "+label) + "  " + valueStr)
		} else {
			b.WriteString(settingsItemStyle.Render(indicator+" "+label) + "  " + valueStr)
		}

		// Description row — always visible, dimmed.
		if item.Description != "" {
			b.WriteString("\n")
			desc := cmdTruncatePad(item.Description, innerWidth-4)
			b.WriteString(settingsDescStyle.Render("    " + desc))
		}

		if i < len(visible)-1 {
			b.WriteString("\n")
		}
	}

	// Scroll position indicator when not all items fit.
	above := s.viewOffset
	below := len(s.items) - end
	if above > 0 || below > 0 {
		b.WriteString("\n")
		var parts []string
		if above > 0 {
			parts = append(parts, fmt.Sprintf("↑ %d", above))
		}
		if below > 0 {
			parts = append(parts, fmt.Sprintf("↓ %d", below))
		}
		b.WriteString(settingsHintStyle.Render("  " + strings.Join(parts, "  ")))
	}

	b.WriteString("\n")
	if s.editing {
		b.WriteString(settingsHintStyle.Render("type to edit • enter save • esc cancel"))
	} else {
		b.WriteString(settingsHintStyle.Render("↑↓ navigate • enter toggle/edit • esc close"))
	}

	return settingsStyle.Render(b.String())
}
