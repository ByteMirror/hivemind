package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var cmdPaletteStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#F0A868")).
	Padding(0, 1)

var cmdPaletteTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#F0A868")).
	MarginBottom(1)

// No padding — widths are controlled explicitly in the render loop for grid alignment.
var cmdItemStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#dddddd"))

var cmdSelectedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#1a1a1a")).
	Background(lipgloss.Color("#7EC8D8"))

var cmdDisabledStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#555555"))

var cmdShortcutStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#666666"))

var cmdCategoryStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#F0A868"))

var cmdSearchStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#555555")).
	Padding(0, 1)

var cmdSearchPlaceholderStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#666666"))

var cmdHintStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#666666")).
	MarginTop(1)

// CommandItem represents a single command in the palette.
type CommandItem struct {
	Label       string // "New Instance"
	Description string // "Create a new agent session"
	Shortcut    string // "n" (display hint only)
	Category    string // "Session", "Git", "Settings", etc.
	Action      string // "cmd_new" — identifier returned on selection
	Disabled    bool
}

// CommandPalette displays a searchable command list overlay.
type CommandPalette struct {
	allItems    []CommandItem
	filtered    []filteredCmdItem
	selectedIdx int
	viewOffset  int // index of first visible item for scrolling
	searchQuery string
	width       int
	maxVisible  int
}

type filteredCmdItem struct {
	item    CommandItem
	origIdx int
}

// NewCommandPalette creates a new command palette with the given items.
func NewCommandPalette(items []CommandItem) *CommandPalette {
	c := &CommandPalette{
		allItems:   items,
		width:      56,
		maxVisible: 14,
	}
	c.applyFilter()
	return c
}

func (c *CommandPalette) applyFilter() {
	c.filtered = nil
	query := strings.ToLower(c.searchQuery)
	for i, item := range c.allItems {
		if query == "" || strings.Contains(strings.ToLower(item.Label), query) ||
			strings.Contains(strings.ToLower(item.Description), query) ||
			strings.Contains(strings.ToLower(item.Category), query) {
			c.filtered = append(c.filtered, filteredCmdItem{
				item:    item,
				origIdx: i,
			})
		}
	}
	if c.selectedIdx >= len(c.filtered) {
		c.selectedIdx = len(c.filtered) - 1
	}
	if c.selectedIdx < 0 {
		c.selectedIdx = 0
	}
	c.viewOffset = 0
	c.skipToNonDisabled(1)
	c.clampViewOffset()
}

func (c *CommandPalette) skipToNonDisabled(direction int) {
	if len(c.filtered) == 0 {
		return
	}
	start := c.selectedIdx
	for c.filtered[c.selectedIdx].item.Disabled {
		c.selectedIdx += direction
		if c.selectedIdx >= len(c.filtered) {
			c.selectedIdx = 0
		}
		if c.selectedIdx < 0 {
			c.selectedIdx = len(c.filtered) - 1
		}
		if c.selectedIdx == start {
			break
		}
	}
}

// clampViewOffset slides the visible window to keep selectedIdx in view.
func (c *CommandPalette) clampViewOffset() {
	if c.selectedIdx >= c.viewOffset+c.maxVisible {
		c.viewOffset = c.selectedIdx - c.maxVisible + 1
	}
	if c.selectedIdx < c.viewOffset {
		c.viewOffset = c.selectedIdx
	}
	if c.viewOffset < 0 {
		c.viewOffset = 0
	}
}

// HandleKeyPress processes key events.
// Returns (action, closed): action is non-empty when an item is selected.
func (c *CommandPalette) HandleKeyPress(msg tea.KeyMsg) (string, bool) {
	switch msg.String() {
	case "esc":
		return "", true
	case "enter":
		if c.selectedIdx < len(c.filtered) && !c.filtered[c.selectedIdx].item.Disabled {
			return c.filtered[c.selectedIdx].item.Action, true
		}
		return "", false
	case "up", "ctrl+k":
		for i := c.selectedIdx - 1; i >= 0; i-- {
			if !c.filtered[i].item.Disabled {
				c.selectedIdx = i
				break
			}
		}
		c.clampViewOffset()
	case "down", "ctrl+j":
		for i := c.selectedIdx + 1; i < len(c.filtered); i++ {
			if !c.filtered[i].item.Disabled {
				c.selectedIdx = i
				break
			}
		}
		c.clampViewOffset()
	case "backspace":
		if len(c.searchQuery) > 0 {
			runes := []rune(c.searchQuery)
			c.searchQuery = string(runes[:len(runes)-1])
			c.applyFilter()
		}
	default:
		if msg.Type == tea.KeyRunes {
			c.searchQuery += string(msg.Runes)
			c.applyFilter()
		} else if msg.Type == tea.KeySpace {
			c.searchQuery += " "
			c.applyFilter()
		}
	}
	return "", false
}

// ScrollUp moves the selection up by one (for mouse wheel support).
func (c *CommandPalette) ScrollUp() {
	for i := c.selectedIdx - 1; i >= 0; i-- {
		if !c.filtered[i].item.Disabled {
			c.selectedIdx = i
			break
		}
	}
	c.clampViewOffset()
}

// ScrollDown moves the selection down by one (for mouse wheel support).
func (c *CommandPalette) ScrollDown() {
	for i := c.selectedIdx + 1; i < len(c.filtered); i++ {
		if !c.filtered[i].item.Disabled {
			c.selectedIdx = i
			break
		}
	}
	c.clampViewOffset()
}

// SetSize sets the display dimensions for the palette.
func (c *CommandPalette) SetSize(width, height int) {
	if width > 20 {
		c.width = width
	}
	if height > 5 {
		c.maxVisible = height - 5 // account for title, search, hints, borders
	}
}

// cmdTruncatePad truncates s to width runes (appending "..." if truncated)
// and right-pads with spaces to exactly width runes.
func cmdTruncatePad(s string, width int) string {
	runes := []rune(s)
	if len(runes) > width {
		if width > 3 {
			return string(runes[:width-3]) + "..."
		}
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// Render returns the styled palette string.
func (c *CommandPalette) Render() string {
	var b strings.Builder

	b.WriteString(cmdPaletteTitleStyle.Render("Command Palette"))
	b.WriteString("\n")

	// Search bar — its width anchors the overall palette width.
	innerWidth := c.width - 4 // border(1×2) + padding(1×2)
	if innerWidth < 10 {
		innerWidth = 10
	}
	searchText := c.searchQuery
	if searchText == "" {
		searchText = cmdSearchPlaceholderStyle.Render("\uf002 Type to filter...")
	}
	b.WriteString(cmdSearchStyle.Width(innerWidth).Render(searchText))
	b.WriteString("\n")

	if len(c.filtered) == 0 {
		b.WriteString(cmdHintStyle.Render("  No matches"))
	} else {
		// Pre-compute fixed column widths from ALL filtered items once, so the
		// grid never shifts as the selection moves.
		keyColWidth := 1
		catColWidth := 1
		for _, fi := range c.filtered {
			if w := len([]rune(fi.item.Shortcut)); w > keyColWidth {
				keyColWidth = w
			}
			if w := len([]rune(fi.item.Category)); w > catColWidth {
				catColWidth = w
			}
		}
		// Row layout: indicator(2) + " "(1) + name + " "(1) + key + "  "(2) + cat
		const fixedOverhead = 6
		nameColWidth := innerWidth - fixedOverhead - keyColWidth - catColWidth
		if nameColWidth < 10 {
			nameColWidth = 10
		}

		// Visible window driven by viewOffset.
		end := c.viewOffset + c.maxVisible
		if end > len(c.filtered) {
			end = len(c.filtered)
		}
		visible := c.filtered[c.viewOffset:end]

		for i, fi := range visible {
			isSelected := (i + c.viewOffset) == c.selectedIdx

			indicator := "  "
			if isSelected {
				indicator = " \u25b8"
			}

			name := cmdTruncatePad(fi.item.Label, nameColWidth)
			key := cmdTruncatePad(fi.item.Shortcut, keyColWidth)
			cat := cmdTruncatePad(fi.item.Category, catColWidth)

			var line string
			switch {
			case fi.item.Disabled:
				line = cmdDisabledStyle.Render(indicator + " " + name + " " + key + "  " + cat)
			case isSelected:
				line = cmdSelectedStyle.Render(indicator + " " + name + " " + key + "  " + cat)
			default:
				line = indicator + " " +
					cmdItemStyle.Render(name) + " " +
					cmdShortcutStyle.Render(key) + "  " +
					cmdCategoryStyle.Render(cat)
			}
			b.WriteString(line)
			if i < len(visible)-1 {
				b.WriteString("\n")
			}
		}

		// Scroll position indicator when not all items fit.
		above := c.viewOffset
		below := len(c.filtered) - end
		if above > 0 || below > 0 {
			b.WriteString("\n")
			var parts []string
			if above > 0 {
				parts = append(parts, fmt.Sprintf("↑ %d", above))
			}
			if below > 0 {
				parts = append(parts, fmt.Sprintf("↓ %d", below))
			}
			b.WriteString(cmdHintStyle.Render("  " + strings.Join(parts, "  ")))
		}
	}

	b.WriteString("\n")
	b.WriteString(cmdHintStyle.Render("↑↓ navigate • enter select • esc close"))

	return cmdPaletteStyle.Render(b.String())
}
