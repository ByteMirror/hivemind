package ui

import (
	"fmt"
	"strings"

	"github.com/ByteMirror/hivemind/session"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

const (
	TopicTabNotes int = iota
	TopicTabTasks
)

// TopicNotesWindow replaces TabbedWindow when the user is in Notes mode.
// It has two sub-tabs: Notes (glamour markdown) and Tasks (todo list).
type TopicNotesWindow struct {
	activeTab    int
	width        int
	height       int
	notes        string
	tasks        []session.TopicTask
	selectedTask int
}

func NewTopicNotesWindow() *TopicNotesWindow {
	return &TopicNotesWindow{}
}

func (w *TopicNotesWindow) SetSize(width, height int) {
	w.width = AdjustPreviewWidth(width)
	w.height = height
}

func (w *TopicNotesWindow) SetNotes(notes string) { w.notes = notes }

func (w *TopicNotesWindow) SetTasks(tasks []session.TopicTask) {
	w.tasks = tasks
	if w.selectedTask >= len(tasks) {
		w.selectedTask = len(tasks) - 1
	}
	if w.selectedTask < 0 {
		w.selectedTask = 0
	}
}

func (w *TopicNotesWindow) GetActiveTab() int { return w.activeTab }
func (w *TopicNotesWindow) SetActiveTab(tab int) {
	if tab >= 0 && tab < 2 {
		w.activeTab = tab
	}
}
func (w *TopicNotesWindow) Toggle()                       { w.activeTab = (w.activeTab + 1) % 2 }
func (w *TopicNotesWindow) IsInNotesTab() bool            { return w.activeTab == TopicTabNotes }
func (w *TopicNotesWindow) IsInTasksTab() bool            { return w.activeTab == TopicTabTasks }
func (w *TopicNotesWindow) TaskCount() int                { return len(w.tasks) }
func (w *TopicNotesWindow) SelectedTask() int             { return w.selectedTask }
func (w *TopicNotesWindow) GetTasks() []session.TopicTask { return w.tasks }

func (w *TopicNotesWindow) TaskUp() {
	if w.selectedTask > 0 {
		w.selectedTask--
	}
}

func (w *TopicNotesWindow) TaskDown() {
	if w.selectedTask < len(w.tasks)-1 {
		w.selectedTask++
	}
}

// HandleTabClick checks if a click at localX/localY hits a tab. Returns true if tab changed.
func (w *TopicNotesWindow) HandleTabClick(localX, localY int) bool {
	if localY < 0 || localY > 2 {
		return false
	}
	tabWidth := w.width / 2
	if tabWidth == 0 {
		return false
	}
	clicked := localX / tabWidth
	if clicked >= 2 {
		clicked = 1
	}
	if clicked < 0 {
		return false
	}
	if clicked != w.activeTab {
		w.activeTab = clicked
		return true
	}
	return false
}

func (w *TopicNotesWindow) String() string {
	if w.width == 0 || w.height == 0 {
		return ""
	}

	// Tab row — same visual style as TabbedWindow
	tabWidth := w.width / 2
	lastTabWidth := w.width - tabWidth
	tabHeight := activeTabStyle.GetVerticalFrameSize() + 1

	tabLabels := []string{"\uf15c Notes", "\uf0ae Tasks"}
	var renderedTabs []string

	for i, label := range tabLabels {
		width := tabWidth
		if i == 1 {
			width = lastTabWidth
		}
		var style lipgloss.Style
		isActive := i == w.activeTab
		if isActive {
			style = activeTabStyle
		} else {
			style = inactiveTabStyle
		}
		border, _, _, _, _ := style.GetBorder()
		switch {
		case i == 0 && isActive:
			border.BottomLeft = "│"
		case i == 0:
			border.BottomLeft = "├"
		case i == 1 && isActive:
			border.BottomRight = "│"
		case i == 1:
			border.BottomRight = "┤"
		}
		style = style.Border(border)
		style = style.Width(width - style.GetHorizontalFrameSize())
		if isActive {
			renderedTabs = append(renderedTabs, style.Render(GradientText(label, "#F0A868", "#7EC8D8")))
		} else {
			renderedTabs = append(renderedTabs, style.Render(label))
		}
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)

	// Content area
	ws := windowStyle
	innerWidth := w.width - ws.GetHorizontalFrameSize()
	innerHeight := w.height - ws.GetVerticalFrameSize() - tabHeight

	var content string
	switch w.activeTab {
	case TopicTabNotes:
		content = w.renderNotes(innerWidth, innerHeight)
	case TopicTabTasks:
		content = w.renderTasks(innerWidth, innerHeight)
	}

	window := ws.Render(
		lipgloss.Place(innerWidth, innerHeight, lipgloss.Left, lipgloss.Top, content))

	return lipgloss.JoinVertical(lipgloss.Left, row, window)
}

func (w *TopicNotesWindow) renderNotes(width, height int) string {
	if w.notes == "" {
		hint := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"}).
			Render("No notes yet — press e to write")
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, hint)
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(w.notes)
	}
	rendered, err := r.Render(w.notes)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(w.notes)
	}
	// Trim trailing blank lines added by glamour
	rendered = strings.TrimRight(rendered, "\n")
	lines := strings.Split(rendered, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (w *TopicNotesWindow) renderTasks(width, height int) string {
	if len(w.tasks) == 0 {
		hint := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"}).
			Render("No tasks yet — press a to add")
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, hint)
	}

	doneStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"}).
		Strikethrough(true)
	pendingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})
	selectedBg := lipgloss.NewStyle().
		Background(lipgloss.Color("#dde4f0")).
		Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"}).
		Width(width)
	checkDone := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#51bd73")).Render("✓")
	checkPending := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#aaaaaa", Dark: "#555555"}).Render("○")

	var lines []string
	for i, task := range w.tasks {
		check := checkPending
		var text string
		if task.Done {
			check = checkDone
			text = doneStyle.Render(task.Text)
		} else {
			text = pendingStyle.Render(task.Text)
		}
		line := fmt.Sprintf(" %s  %s", check, text)
		if i == w.selectedTask {
			// For selected row, re-render with bg (raw text to avoid nested styling)
			line = selectedBg.Render(fmt.Sprintf(" %s  %s", check, task.Text))
		}
		lines = append(lines, line)
	}

	hint := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#aaaaaa", Dark: "#555555"}).
		Render("\n  a add · space toggle · x delete")

	return strings.Join(lines, "\n") + hint
}
