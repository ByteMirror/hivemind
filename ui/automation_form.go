package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	formTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F0A868")) // no MarginBottom — keep line count predictable

	formLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F0A868"))

	formLabelDimStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888"))

	formHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555")).
			Italic(true)

	formActiveFieldStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#F0A868")).
				Padding(0, 1)

	formInactiveFieldStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#444444")).
				Padding(0, 1)

	formSelectedChoiceStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#1A1A1A")).
				Background(lipgloss.Color("#F0A868")).
				Padding(0, 1)

	formChoiceStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA")).
			Padding(0, 1)
)

// AutomationRepoOption describes a selectable project in the automation form.
type AutomationRepoOption struct {
	Label string
	Path  string
}

// AutomationForm is an inline multi-field form for creating or editing automations.
// All key fields are visible and editable in a single screen.
type AutomationForm struct {
	nameInput     textinput.Model
	scheduleInput textinput.Model
	instrArea     textarea.Model
	focusedField  int // 0=name, 1=agent, 2=project, 3=schedule, 4=instructions
	Submitted     bool
	Canceled      bool
	IsEditing     bool

	agentOptions []string
	agentIdx     int
	repoOptions  []AutomationRepoOption
	repoIdx      int
}

// NewAutomationForm creates a form pre-populated with the given values.
// Set isEditing=false for a new automation (empty values), true for edit.
func NewAutomationForm(
	name, schedule, instructions, agent, repoPath string,
	agentOptions []string,
	repoOptions []AutomationRepoOption,
	isEditing bool,
) *AutomationForm {
	ni := textinput.New()
	ni.Placeholder = "e.g. Daily code review"
	ni.SetValue(name)
	ni.CharLimit = 64
	ni.Focus()

	si := textinput.New()
	si.Placeholder = "e.g. daily, every 4h, @06:00"
	si.SetValue(schedule)
	si.CharLimit = 32

	ta := textarea.New()
	ta.Placeholder = "Prompt sent to the agent when this automation runs…"
	ta.SetValue(instructions)
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.Blur()

	agents, selectedAgentIdx := normalizeAgents(agentOptions, agent)
	repos, selectedRepoIdx := normalizeRepoOptions(repoOptions, repoPath)

	return &AutomationForm{
		nameInput:     ni,
		scheduleInput: si,
		instrArea:     ta,
		focusedField:  0,
		IsEditing:     isEditing,
		agentOptions:  agents,
		agentIdx:      selectedAgentIdx,
		repoOptions:   repos,
		repoIdx:       selectedRepoIdx,
	}
}

// HandleKey processes a key event. Returns true when the form should close.
func (f *AutomationForm) HandleKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+s", "ctrl+d":
		f.Submitted = true
		return true
	case "esc":
		f.Canceled = true
		return true
	case "tab":
		f.setFocus((f.focusedField + 1) % 5)
		return false
	case "shift+tab":
		f.setFocus((f.focusedField + 4) % 5)
		return false
	}

	switch f.focusedField {
	case 0:
		f.nameInput, _ = f.nameInput.Update(msg)
	case 1:
		if f.handleChoiceKey(msg, &f.agentIdx, len(f.agentOptions)) {
			return false
		}
	case 2:
		if f.handleChoiceKey(msg, &f.repoIdx, len(f.repoOptions)) {
			return false
		}
	case 3:
		f.scheduleInput, _ = f.scheduleInput.Update(msg)
	case 4:
		f.instrArea, _ = f.instrArea.Update(msg)
	}
	return false
}

func (f *AutomationForm) handleChoiceKey(msg tea.KeyMsg, idx *int, count int) bool {
	if count == 0 {
		return false
	}
	switch msg.String() {
	case "left", "h", "up", "k":
		*idx = (*idx - 1 + count) % count
		return true
	case "right", "l", "down", "j":
		*idx = (*idx + 1) % count
		return true
	default:
		return false
	}
}

func (f *AutomationForm) setFocus(idx int) {
	f.focusedField = idx
	switch idx {
	case 0:
		f.nameInput.Focus()
		f.scheduleInput.Blur()
		f.instrArea.Blur()
	case 3:
		f.nameInput.Blur()
		f.scheduleInput.Focus()
		f.instrArea.Blur()
	case 4:
		f.nameInput.Blur()
		f.scheduleInput.Blur()
		f.instrArea.Focus()
	default:
		f.nameInput.Blur()
		f.scheduleInput.Blur()
		f.instrArea.Blur()
	}
}

// GetValues returns the current field values.
func (f *AutomationForm) GetValues() (name, schedule, instructions, agent, repoPath string) {
	agent = ""
	if len(f.agentOptions) > 0 && f.agentIdx >= 0 && f.agentIdx < len(f.agentOptions) {
		agent = strings.TrimSpace(f.agentOptions[f.agentIdx])
	}
	repoPath = ""
	if len(f.repoOptions) > 0 && f.repoIdx >= 0 && f.repoIdx < len(f.repoOptions) {
		repoPath = strings.TrimSpace(f.repoOptions[f.repoIdx].Path)
	}

	return strings.TrimSpace(f.nameInput.Value()),
		strings.TrimSpace(f.scheduleInput.Value()),
		strings.TrimSpace(f.instrArea.Value()),
		agent,
		repoPath
}

// Render draws the form at the given inner width and height.
func (f *AutomationForm) Render(width, height int) string {
	title := "New Automation"
	if f.IsEditing {
		title = "Edit Automation"
	}

	fieldW := width - 4 // account for field border padding
	if fieldW < 20 {
		fieldW = 20
	}

	f.nameInput.Width = fieldW
	f.scheduleInput.Width = fieldW
	f.instrArea.SetWidth(fieldW)
	// Fixed lines in Render output:
	// title/divider: 3
	// name: 5
	// agent: 5
	// project: 6 (includes selected path line)
	// schedule: 6 (includes hint line)
	// instructions label+footer+spacing: 3
	const fixedLines = 28
	instrLines := height - fixedLines
	if instrLines < 1 {
		instrLines = 1
	}
	f.instrArea.SetHeight(instrLines)

	var sb strings.Builder

	sb.WriteString(formTitleStyle.Render("⚡ "+title) + "\n")
	sb.WriteString(autoDividerStyle.Render(strings.Repeat("─", width)) + "\n\n")

	// Name field
	sb.WriteString(f.fieldLabel("Name", f.focusedField == 0) + "\n")
	sb.WriteString(f.wrapField(f.nameInput.View(), f.focusedField == 0, fieldW) + "\n\n")

	// Agent field
	sb.WriteString(f.fieldLabel("Agent", f.focusedField == 1) + "\n")
	sb.WriteString(f.wrapField(f.renderChoiceLine(f.agentChoiceLabels(), f.agentIdx), f.focusedField == 1, fieldW) + "\n\n")

	// Project field
	sb.WriteString(f.fieldLabel("Project", f.focusedField == 2) + "\n")
	sb.WriteString(f.wrapField(f.renderChoiceLine(f.repoChoiceLabels(), f.repoIdx), f.focusedField == 2, fieldW) + "\n")
	if repoPath := f.selectedRepoPath(); repoPath != "" {
		sb.WriteString(formHintStyle.Render("  "+repoPath) + "\n")
	}
	sb.WriteString("\n")

	// Schedule field
	sb.WriteString(f.fieldLabel("Schedule", f.focusedField == 3) + "\n")
	sb.WriteString(formHintStyle.Render("  hourly · daily · weekly · every 4h · every 30m · @06:00") + "\n")
	sb.WriteString(f.wrapField(f.scheduleInput.View(), f.focusedField == 3, fieldW) + "\n\n")

	// Instructions field (no trailing blank — footer follows immediately)
	sb.WriteString(f.fieldLabel("Instructions", f.focusedField == 4) + "\n")
	sb.WriteString(f.wrapField(f.instrArea.View(), f.focusedField == 4, fieldW) + "\n")

	sb.WriteString(autoHintStyle.Render("tab next field  shift+tab prev  ←/→ select  ctrl+s save  esc cancel"))

	return sb.String()
}

func (f *AutomationForm) selectedRepoPath() string {
	if len(f.repoOptions) == 0 || f.repoIdx < 0 || f.repoIdx >= len(f.repoOptions) {
		return ""
	}
	return f.repoOptions[f.repoIdx].Path
}

func (f *AutomationForm) agentChoiceLabels() []string {
	labels := make([]string, len(f.agentOptions))
	copy(labels, f.agentOptions)
	return labels
}

func (f *AutomationForm) repoChoiceLabels() []string {
	labels := make([]string, len(f.repoOptions))
	for i, opt := range f.repoOptions {
		labels[i] = opt.Label
	}
	return labels
}

func (f *AutomationForm) renderChoiceLine(options []string, selectedIdx int) string {
	if len(options) == 0 {
		return formChoiceStyle.Render("none")
	}
	var parts []string
	for i, option := range options {
		part := formChoiceStyle.Render(option)
		if i == selectedIdx {
			part = formSelectedChoiceStyle.Render(option)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
}

func (f *AutomationForm) fieldLabel(label string, active bool) string {
	if active {
		return formLabelStyle.Render("  " + label)
	}
	return formLabelDimStyle.Render("  " + label)
}

func (f *AutomationForm) wrapField(content string, active bool, width int) string {
	style := formInactiveFieldStyle.Width(width)
	if active {
		style = formActiveFieldStyle.Width(width)
	}
	return style.Render(content)
}

func normalizeAgents(agentOptions []string, selected string) ([]string, int) {
	clean := make([]string, 0, len(agentOptions))
	seen := make(map[string]struct{}, len(agentOptions))
	for _, agent := range agentOptions {
		agent = strings.TrimSpace(agent)
		if agent == "" {
			continue
		}
		if _, ok := seen[agent]; ok {
			continue
		}
		seen[agent] = struct{}{}
		clean = append(clean, agent)
	}
	if len(clean) == 0 {
		clean = []string{"claude"}
	}

	selected = strings.TrimSpace(selected)
	for i, agent := range clean {
		if agent == selected {
			return clean, i
		}
	}
	return clean, 0
}

func normalizeRepoOptions(repoOptions []AutomationRepoOption, selectedPath string) ([]AutomationRepoOption, int) {
	clean := make([]AutomationRepoOption, 0, len(repoOptions))
	seen := make(map[string]struct{}, len(repoOptions))
	for _, repo := range repoOptions {
		repo.Path = strings.TrimSpace(repo.Path)
		repo.Label = strings.TrimSpace(repo.Label)
		if repo.Path == "" {
			continue
		}
		if repo.Label == "" {
			repo.Label = repo.Path
		}
		if _, ok := seen[repo.Path]; ok {
			continue
		}
		seen[repo.Path] = struct{}{}
		clean = append(clean, repo)
	}
	if len(clean) == 0 {
		clean = []AutomationRepoOption{{Label: "(none)", Path: ""}}
	}

	selectedPath = strings.TrimSpace(selectedPath)
	for i, repo := range clean {
		if repo.Path == selectedPath {
			return clean, i
		}
	}
	return clean, 0
}
