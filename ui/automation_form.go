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
)

// AutomationForm is an inline multi-field form for creating or editing automations.
// All three fields are visible and editable in a single screen.
type AutomationForm struct {
	nameInput     textinput.Model
	scheduleInput textinput.Model
	instrArea     textarea.Model
	focusedField  int // 0=name, 1=schedule, 2=instructions
	Submitted     bool
	Canceled      bool
	IsEditing     bool
}

// NewAutomationForm creates a form pre-populated with the given values.
// Set isEditing=false for a new automation (empty values), true for edit.
func NewAutomationForm(name, schedule, instructions string, isEditing bool) *AutomationForm {
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

	return &AutomationForm{
		nameInput:     ni,
		scheduleInput: si,
		instrArea:     ta,
		focusedField:  0,
		IsEditing:     isEditing,
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
		f.setFocus((f.focusedField + 1) % 3)
		return false
	case "shift+tab":
		f.setFocus((f.focusedField + 2) % 3)
		return false
	}

	// Forward keystroke to the active field.
	switch f.focusedField {
	case 0:
		f.nameInput, _ = f.nameInput.Update(msg)
	case 1:
		f.scheduleInput, _ = f.scheduleInput.Update(msg)
	case 2:
		f.instrArea, _ = f.instrArea.Update(msg)
	}
	return false
}

func (f *AutomationForm) setFocus(idx int) {
	f.focusedField = idx
	if idx == 0 {
		f.nameInput.Focus()
		f.scheduleInput.Blur()
		f.instrArea.Blur()
	} else if idx == 1 {
		f.nameInput.Blur()
		f.scheduleInput.Focus()
		f.instrArea.Blur()
	} else {
		f.nameInput.Blur()
		f.scheduleInput.Blur()
		f.instrArea.Focus()
	}
}

// GetValues returns the current field values.
func (f *AutomationForm) GetValues() (name, schedule, instructions string) {
	return strings.TrimSpace(f.nameInput.Value()),
		strings.TrimSpace(f.scheduleInput.Value()),
		strings.TrimSpace(f.instrArea.Value())
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
	// Fixed lines in Render output (counted precisely, no margins):
	//   title(1) + divider+blank(2) + name_label(1) + name_field+blank(4)
	//   + sched_label(1) + sched_hint(1) + sched_field+blank(4)
	//   + instr_label(1) + instr_field_border(2) + blank(1) + footer(1) = 19
	const fixedLines = 19
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

	// Schedule field
	sb.WriteString(f.fieldLabel("Schedule", f.focusedField == 1) + "\n")
	sb.WriteString(formHintStyle.Render("  hourly · daily · weekly · every 4h · every 30m · @06:00") + "\n")
	sb.WriteString(f.wrapField(f.scheduleInput.View(), f.focusedField == 1, fieldW) + "\n\n")

	// Instructions field (no trailing blank — footer follows immediately)
	sb.WriteString(f.fieldLabel("Instructions", f.focusedField == 2) + "\n")
	sb.WriteString(f.wrapField(f.instrArea.View(), f.focusedField == 2, fieldW) + "\n")

	sb.WriteString(autoHintStyle.Render("tab next field  shift+tab prev  ctrl+s save  esc cancel"))

	return sb.String()
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
