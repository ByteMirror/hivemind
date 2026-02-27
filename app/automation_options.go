package app

import (
	"path/filepath"
	"strings"

	"github.com/ByteMirror/hivemind/config"
	"github.com/ByteMirror/hivemind/ui"
)

var defaultAutomationAgents = []string{"claude", "codex", "aider", "gemini", "amp"}

func buildAutomationAgentOptions(currentProgram string) []string {
	seen := make(map[string]struct{})
	options := make([]string, 0, len(defaultAutomationAgents)+1)

	if current := normalizeProgramCommand(currentProgram); current != "" {
		options = append(options, current)
		seen[current] = struct{}{}
	}

	for _, opt := range defaultAutomationAgents {
		if _, ok := seen[opt]; ok {
			continue
		}
		options = append(options, opt)
		seen[opt] = struct{}{}
	}

	return options
}

func normalizeProgramCommand(program string) string {
	parts := strings.Fields(strings.TrimSpace(program))
	if len(parts) == 0 {
		return ""
	}
	return filepath.Base(parts[0])
}

func buildAutomationRepoOptions(repoPaths []string) []ui.AutomationRepoOption {
	unique := make([]string, 0, len(repoPaths))
	seenPaths := make(map[string]struct{}, len(repoPaths))
	baseCount := make(map[string]int)

	for _, rp := range repoPaths {
		rp = strings.TrimSpace(rp)
		if rp == "" {
			continue
		}
		if _, exists := seenPaths[rp]; exists {
			continue
		}
		seenPaths[rp] = struct{}{}
		unique = append(unique, rp)
		baseCount[filepath.Base(rp)]++
	}

	opts := make([]ui.AutomationRepoOption, 0, len(unique))
	for _, rp := range unique {
		base := filepath.Base(rp)
		label := base
		if baseCount[base] > 1 {
			parent := filepath.Base(filepath.Dir(rp))
			if parent != "" && parent != "." && parent != string(filepath.Separator) {
				label = parent + "/" + base
			}
		}
		opts = append(opts, ui.AutomationRepoOption{
			Label: label,
			Path:  rp,
		})
	}
	return opts
}

func (m *home) automationAgentOptions() []string {
	return buildAutomationAgentOptions(m.program)
}

func (m *home) automationRepoOptions() []ui.AutomationRepoOption {
	repos := m.activeRepoPaths
	if len(repos) == 0 && m.primaryRepoPath != "" {
		repos = []string{m.primaryRepoPath}
	}
	return buildAutomationRepoOptions(repos)
}

func (m *home) automationRepoOptionsWithSelected(repoPath string) []ui.AutomationRepoOption {
	opts := m.automationRepoOptions()
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return opts
	}
	for _, opt := range opts {
		if opt.Path == repoPath {
			return opts
		}
	}
	extra := buildAutomationRepoOptions([]string{repoPath})
	if len(extra) == 0 {
		return opts
	}
	return append(opts, extra[0])
}

func (m *home) defaultAutomationProgram() string {
	options := m.automationAgentOptions()
	if len(options) > 0 {
		return options[0]
	}
	return "claude"
}

func (m *home) defaultAutomationRepoPath() string {
	if m.sidebar != nil {
		if rp := m.sidebar.GetSelectedRepoPath(); rp != "" {
			return rp
		}
	}
	repoOptions := m.automationRepoOptions()
	if len(repoOptions) > 0 {
		return repoOptions[0].Path
	}
	return m.primaryRepoPath
}

func (m *home) resolveAutomationProgram(auto *config.Automation) string {
	if auto != nil {
		if program := strings.TrimSpace(auto.Program); program != "" {
			return program
		}
	}

	if program := strings.TrimSpace(m.program); program != "" {
		return program
	}
	if m.appConfig != nil {
		if program := strings.TrimSpace(m.appConfig.DefaultProgram); program != "" {
			return program
		}
	}
	return "claude"
}

func (m *home) resolveAutomationRepoPath(auto *config.Automation) string {
	if auto != nil {
		if repoPath := strings.TrimSpace(auto.RepoPath); repoPath != "" {
			return repoPath
		}
	}
	return m.defaultAutomationRepoPath()
}
