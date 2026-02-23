package app

import (
	"github.com/ByteMirror/hivemind/log"
	"github.com/ByteMirror/hivemind/session"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// startOnboarding creates the companion chat agent and starts it asynchronously.
// The companion lives in ~/.hivemind/chats/companion/ and runs with IsChat=true.
func (m *home) startOnboarding() (tea.Model, tea.Cmd) {
	slug := "companion"

	if err := session.EnsureAgentDir(slug); err != nil {
		log.WarningLog.Printf("onboarding: EnsureAgentDir: %v", err)
	}
	if err := session.CopyTemplatesToAgentDir(slug); err != nil {
		log.WarningLog.Printf("onboarding: CopyTemplatesToAgentDir: %v", err)
	}

	personalityDir, err := session.GetAgentPersonalityDir(slug)
	if err != nil {
		log.WarningLog.Printf("onboarding: GetAgentPersonalityDir: %v", err)
		return m, nil
	}

	companion, err := session.NewInstance(session.InstanceOptions{
		Title:           slug,
		Path:            m.primaryRepoPath,
		Program:         m.program,
		IsChat:          true,
		PersonalityDir:  personalityDir,
		SkipPermissions: true,
	})
	if err != nil {
		log.WarningLog.Printf("onboarding: NewInstance: %v", err)
		return m, nil
	}

	m.allInstances = append(m.allInstances, companion)

	startCmd := func() tea.Msg {
		if err := companion.Start(true); err != nil {
			return onboardingStartedMsg{err: err}
		}
		return onboardingStartedMsg{err: nil}
	}

	return m, startCmd
}

// onboardingStartedMsg is sent when the companion instance start attempt completes.
type onboardingStartedMsg struct {
	err error
}

// viewOnboarding renders the first-launch centered panel showing the companion's output.
// It uses m.width and m.contentHeight for layout since there is no separate height field.
func (m *home) viewOnboarding() string {
	totalHeight := m.contentHeight + 2 // approximate full terminal height

	companion := m.findInstanceByTitle("companion")
	if companion == nil || !companion.Started() {
		msg := "Starting companion..."
		return lipgloss.Place(m.width, totalHeight, lipgloss.Center, lipgloss.Center, msg)
	}

	panelWidth := m.width * 60 / 100
	if panelWidth < 60 {
		panelWidth = 60
	}
	panelHeight := totalHeight * 70 / 100
	if panelHeight < 20 {
		panelHeight = 20
	}

	// Use the cached terminal content updated by the previewTickMsg loop.
	preview := m.onboardingContent
	if preview == "" {
		preview = "Connecting..."
	}

	panel := lipgloss.NewStyle().
		Width(panelWidth).
		Height(panelHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Render(preview)

	return lipgloss.Place(m.width, totalHeight, lipgloss.Center, lipgloss.Center, panel)
}
