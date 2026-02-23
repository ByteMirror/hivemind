package app

import (
	"fmt"
	"strconv"

	"github.com/ByteMirror/hivemind/config"
	"github.com/ByteMirror/hivemind/session"
	"github.com/ByteMirror/hivemind/ui/overlay"

	tea "github.com/charmbracelet/bubbletea"
)

// buildCommandItems returns all available commands for the palette,
// reflecting current app state (e.g. disabling actions when nothing is selected).
func (m *home) buildCommandItems() []overlay.CommandItem {
	selected := m.list.GetSelectedInstance()
	noSelection := selected == nil
	notRunning := noSelection || !selected.Started() || selected.Paused()
	notPaused := noSelection || selected.Status != session.Paused

	items := []overlay.CommandItem{
		// Session
		{Label: "New Instance", Description: "Create a new agent session", Shortcut: "n", Category: "Session", Action: "cmd_new"},
		{Label: "New Instance with Prompt", Description: "Create session and enter prompt", Shortcut: "N", Category: "Session", Action: "cmd_new_prompt"},
		{Label: "New Instance (Skip Perms)", Description: "Create with --dangerously-skip-permissions", Shortcut: "S", Category: "Session", Action: "cmd_new_skip"},
		{Label: "Kill Instance", Description: "Kill the selected session", Shortcut: "D", Category: "Session", Action: "cmd_kill", Disabled: noSelection},
		{Label: "Toggle Auto-Accept", Description: "Toggle auto-accept on instance or topic", Shortcut: "y", Category: "Session", Action: "cmd_auto_yes", Disabled: noSelection},

		// Focus
		{Label: "Focus Agent", Description: "Enter focus mode on the agent pane", Shortcut: "↵", Category: "Focus", Action: "cmd_focus", Disabled: notRunning},
		{Label: "Zen Mode", Description: "Full terminal attach (ctrl+q to exit)", Shortcut: "Z", Category: "Focus", Action: "cmd_zen", Disabled: notRunning},

		// Git
		{Label: "Push Branch", Description: "Commit and push branch to GitHub", Shortcut: "p", Category: "Git", Action: "cmd_push", Disabled: noSelection},
		{Label: "Create Pull Request", Description: "Create a PR from this branch", Shortcut: "P", Category: "Git", Action: "cmd_create_pr", Disabled: noSelection},
		{Label: "Checkout (Pause)", Description: "Commit changes and pause session", Shortcut: "c", Category: "Git", Action: "cmd_checkout", Disabled: notRunning},
		{Label: "Resume", Description: "Resume a paused session", Shortcut: "r", Category: "Git", Action: "cmd_resume", Disabled: notPaused},

		// Navigation
		{Label: "Switch Repository", Description: "Switch or toggle active repositories", Shortcut: "R", Category: "Navigation", Action: "cmd_repo_switch"},
		{Label: "Search", Description: "Search topics and instances", Shortcut: "/", Category: "Navigation", Action: "cmd_search"},
		{Label: "New Topic", Description: "Create a new topic group", Shortcut: "T", Category: "Navigation", Action: "cmd_new_topic"},
		{Label: "Move to Topic", Description: "Move instance to a topic", Shortcut: "m", Category: "Navigation", Action: "cmd_move_to", Disabled: noSelection},

		// System
		{Label: "Settings", Description: "Configure application settings", Shortcut: "", Category: "System", Action: "cmd_settings"},
		{Label: "Help", Description: "Show keyboard shortcuts", Shortcut: "?", Category: "System", Action: "cmd_help"},
	}

	return items
}

// executeCommandPaletteAction dispatches the action string from the command palette.
func (m *home) executeCommandPaletteAction(action string) (tea.Model, tea.Cmd) {
	switch action {
	case "cmd_new":
		if _, errCmd := m.createNewInstance(false); errCmd != nil {
			return m, errCmd
		}
		return m, nil
	case "cmd_new_prompt":
		if _, errCmd := m.createNewInstance(false); errCmd != nil {
			return m, errCmd
		}
		m.promptAfterName = true
		return m, nil
	case "cmd_new_skip":
		if _, errCmd := m.createNewInstance(true); errCmd != nil {
			return m, errCmd
		}
		return m, nil
	case "cmd_kill":
		// Synthesize the key to reuse the confirmation flow
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	case "cmd_auto_yes":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	case "cmd_focus":
		selected := m.list.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() {
			return m, nil
		}
		return m, m.enterFocusMode()
	case "cmd_zen":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
	case "cmd_push":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	case "cmd_create_pr":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	case "cmd_checkout":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	case "cmd_resume":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	case "cmd_repo_switch":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	case "cmd_search":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	case "cmd_new_topic":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	case "cmd_move_to":
		return m.handleDefaultKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	case "cmd_settings":
		return m.openSettings()
	case "cmd_help":
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	}
	return m, nil
}

// openSettings builds the settings overlay from the current config.
func (m *home) openSettings() (tea.Model, tea.Cmd) {
	notifications := "true"
	if !m.appConfig.AreNotificationsEnabled() {
		notifications = "false"
	}
	skipHooks := "true"
	if !m.appConfig.ShouldSkipGitHooks() {
		skipHooks = "false"
	}
	autoYes := strconv.FormatBool(m.appConfig.AutoYes)

	items := []overlay.SettingItem{
		{
			Label:       "Notifications",
			Description: "Desktop notifications when agent finishes",
			Type:        overlay.SettingToggle,
			Value:       notifications,
			Key:         "notifications_enabled",
		},
		{
			Label:       "Skip Git Hooks",
			Description: "Pass --no-verify to git commit",
			Type:        overlay.SettingToggle,
			Value:       skipHooks,
			Key:         "skip_git_hooks",
		},
		{
			Label:       "Auto-Accept",
			Description: "Automatically accept all agent prompts",
			Type:        overlay.SettingToggle,
			Value:       autoYes,
			Key:         "auto_yes",
		},
		{
			Label:       "Default Agent",
			Description: "Program to run in new instances",
			Type:        overlay.SettingPicker,
			Value:       m.appConfig.DefaultProgram,
			Options:     []string{"claude", "aider", "gemini", "codex", "amp"},
			Key:         "default_program",
		},
		{
			Label:       "Branch Prefix",
			Description: "Prefix for git branches (e.g. username/)",
			Type:        overlay.SettingText,
			Value:       m.appConfig.BranchPrefix,
			Key:         "branch_prefix",
		},
	}

	m.settingsOverlay = overlay.NewSettingsOverlay(items)
	m.state = stateSettings
	return m, nil
}

// applySettingChange reads the changed value from the overlay and persists it.
func (m *home) applySettingChange(key string) {
	item := m.settingsOverlay.GetItem(key)
	if item == nil {
		return
	}

	switch key {
	case "notifications_enabled":
		val := item.Value == "true"
		m.appConfig.NotificationsEnabled = &val
	case "skip_git_hooks":
		val := item.Value == "true"
		m.appConfig.SkipGitHooks = &val
	case "auto_yes":
		m.appConfig.AutoYes = item.Value == "true"
	case "default_program":
		m.appConfig.DefaultProgram = item.Value
	case "branch_prefix":
		m.appConfig.BranchPrefix = item.Value
	}

	if err := config.SaveConfig(m.appConfig); err != nil {
		m.handleError(fmt.Errorf("failed to save settings: %w", err))
	}
}
