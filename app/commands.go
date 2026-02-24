package app

import (
	"fmt"
	"strconv"

	"github.com/ByteMirror/hivemind/config"
	"github.com/ByteMirror/hivemind/log"
	"github.com/ByteMirror/hivemind/memory"
	"github.com/ByteMirror/hivemind/session"
	"github.com/ByteMirror/hivemind/ui"
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
		{Label: "Memory Browser", Description: "Browse, edit and delete memory files", Shortcut: "M", Category: "System", Action: "cmd_memory_browser"},
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
	case "cmd_memory_browser":
		return m.openMemoryBrowser()
	case "cmd_help":
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	}
	return m, nil
}

// buildMemorySettingItems returns settings items for the Memory section.
// Provider-specific fields are only included when relevant.
func buildMemorySettingItems(cfg *config.Config) []overlay.SettingItem {
	mem := cfg.Memory
	enabled := "false"
	provider := "none"
	claudeModel := memory.DefaultRerankModel
	openAIKey := ""
	openAIModel := "text-embedding-3-small"
	ollamaURL := "http://localhost:11434"
	ollamaModel := "nomic-embed-text"
	injectCount := "5"

	if mem != nil {
		if mem.Enabled {
			enabled = "true"
		}
		if mem.EmbeddingProvider != "" {
			provider = mem.EmbeddingProvider
		}
		if mem.ClaudeModel != "" {
			claudeModel = mem.ClaudeModel
		}
		if mem.OpenAIAPIKey != "" {
			openAIKey = mem.OpenAIAPIKey
		}
		if mem.OpenAIModel != "" {
			openAIModel = mem.OpenAIModel
		}
		if mem.OllamaURL != "" {
			ollamaURL = mem.OllamaURL
		}
		if mem.OllamaModel != "" {
			ollamaModel = mem.OllamaModel
		}
		if mem.StartupInjectCount > 0 {
			injectCount = strconv.Itoa(mem.StartupInjectCount)
		}
	}

	items := []overlay.SettingItem{
		{
			Label:       "── Memory ──",
			Description: "",
			Type:        overlay.SettingText,
			Value:       "",
			Key:         "memory_section_header",
		},
		{
			Label:       "Memory enabled",
			Description: "Persistent knowledge base shared across all agents",
			Type:        overlay.SettingToggle,
			Value:       enabled,
			Key:         "memory.enabled",
		},
		{
			Label:       "Search provider",
			Description: "claude = re-rank via local claude CLI (works with Max), openai/ollama = embeddings",
			Type:        overlay.SettingPicker,
			Value:       provider,
			Options:     []string{"none", "claude", "openai", "ollama"},
			Key:         "memory.embedding_provider",
		},
	}

	switch provider {
	case "claude":
		items = append(items, overlay.SettingItem{
			Label:       "Claude model",
			Description: "Model for re-ranking (default: claude-haiku-4-5-20251001)",
			Type:        overlay.SettingText,
			Value:       claudeModel,
			Key:         "memory.claude_model",
		})
	case "openai":
		items = append(items, overlay.SettingItem{
			Label:       "OpenAI API key",
			Description: "sk-... key from platform.openai.com",
			Type:        overlay.SettingText,
			Value:       openAIKey,
			Key:         "memory.openai_api_key",
		}, overlay.SettingItem{
			Label:       "OpenAI model",
			Description: "Embedding model (default: text-embedding-3-small)",
			Type:        overlay.SettingText,
			Value:       openAIModel,
			Key:         "memory.openai_model",
		})
	case "ollama":
		items = append(items, overlay.SettingItem{
			Label:       "Ollama URL",
			Description: "Ollama server URL (default: http://localhost:11434)",
			Type:        overlay.SettingText,
			Value:       ollamaURL,
			Key:         "memory.ollama_url",
		}, overlay.SettingItem{
			Label:       "Ollama model",
			Description: "Embedding model (default: nomic-embed-text)",
			Type:        overlay.SettingText,
			Value:       ollamaModel,
			Key:         "memory.ollama_model",
		})
	}

	items = append(items, overlay.SettingItem{
		Label:       "Startup snippets",
		Description: "Memory snippets injected into CLAUDE.md at agent start (default: 5)",
		Type:        overlay.SettingText,
		Value:       injectCount,
		Key:         "memory.startup_inject_count",
	})

	return items
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

	items = append(items, buildMemorySettingItems(m.appConfig)...)
	m.settingsOverlay = overlay.NewSettingsOverlay(items)
	m.state = stateSettings
	return m, nil
}

// openMemoryBrowser opens the full-screen memory file browser.
func (m *home) openMemoryBrowser() (tea.Model, tea.Cmd) {
	mgr := session.GetMemoryManager()
	if mgr == nil {
		m.toastManager.Info("Memory is disabled. Enable it in Settings.")
		return m, m.toastTickCmd()
	}
	browser, err := ui.NewMemoryBrowser(mgr)
	if err != nil {
		return m, m.handleError(fmt.Errorf("open memory browser: %w", err))
	}
	browser.SetSize(m.width, m.contentHeight)
	m.memoryBrowser = browser
	m.state = stateMemoryBrowser
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
	case "memory.enabled":
		m.ensureMemoryConfig()
		m.appConfig.Memory.Enabled = item.Value == "true"
		m.restartMemoryManager()
	case "memory.embedding_provider":
		m.ensureMemoryConfig()
		m.appConfig.Memory.EmbeddingProvider = item.Value
		// Rebuild overlay to show/hide provider-specific fields.
		allItems := m.settingsOverlay.Items()
		// Find where memory section starts (first item with key "memory_section_header").
		memStart := len(allItems)
		for i, it := range allItems {
			if it.Key == "memory_section_header" {
				memStart = i
				break
			}
		}
		newItems := append(allItems[:memStart:memStart], buildMemorySettingItems(m.appConfig)...)
		m.settingsOverlay.SetItems(newItems)
		m.restartMemoryManager()
	case "memory.claude_model":
		m.ensureMemoryConfig()
		m.appConfig.Memory.ClaudeModel = item.Value
	case "memory.openai_api_key":
		m.ensureMemoryConfig()
		m.appConfig.Memory.OpenAIAPIKey = item.Value
		m.restartMemoryManager()
	case "memory.openai_model":
		m.ensureMemoryConfig()
		m.appConfig.Memory.OpenAIModel = item.Value
	case "memory.ollama_url":
		m.ensureMemoryConfig()
		m.appConfig.Memory.OllamaURL = item.Value
	case "memory.ollama_model":
		m.ensureMemoryConfig()
		m.appConfig.Memory.OllamaModel = item.Value
	case "memory.startup_inject_count":
		m.ensureMemoryConfig()
		if n, err := strconv.Atoi(item.Value); err == nil && n > 0 {
			m.appConfig.Memory.StartupInjectCount = n
		}
	}

	if err := config.SaveConfig(m.appConfig); err != nil {
		m.handleError(fmt.Errorf("failed to save settings: %w", err))
	}
}

// ensureMemoryConfig initialises Memory config if nil.
func (m *home) ensureMemoryConfig() {
	if m.appConfig.Memory == nil {
		m.appConfig.Memory = &config.MemoryConfig{}
	}
}

// restartMemoryManager re-initialises the memory manager from the current config.
// Called whenever memory.enabled or the provider changes.
func (m *home) restartMemoryManager() {
	mgr, err := memory.NewManagerFromConfig(m.appConfig)
	if err != nil {
		if log.WarningLog != nil {
			log.WarningLog.Printf("memory restart: %v", err)
		}
		return
	}
	injectCount := 5
	if m.appConfig.Memory != nil && m.appConfig.Memory.StartupInjectCount > 0 {
		injectCount = m.appConfig.Memory.StartupInjectCount
	}
	sysBudget := 4000
	if m.appConfig.Memory != nil && m.appConfig.Memory.SystemBudgetChars > 0 {
		sysBudget = m.appConfig.Memory.SystemBudgetChars
	}
	session.SetMemoryManager(mgr, injectCount, sysBudget)
}
