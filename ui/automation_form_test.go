package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAutomationForm_GetValuesIncludesAgentAndProject(t *testing.T) {
	agents := []string{"claude", "codex"}
	projects := []AutomationRepoOption{
		{Label: "hivemind", Path: "/repos/hivemind"},
		{Label: "api", Path: "/repos/api"},
	}

	form := NewAutomationForm(
		"Daily review",
		"daily",
		"Review recent changes",
		"claude",
		"/repos/hivemind",
		agents,
		projects,
		false,
	)

	// Move focus to Agent, then pick the next option.
	form.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	form.HandleKey(tea.KeyMsg{Type: tea.KeyRight})

	// Move focus to Project, then pick the next option.
	form.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	form.HandleKey(tea.KeyMsg{Type: tea.KeyRight})

	name, schedule, instructions, agent, projectPath := form.GetValues()
	if name != "Daily review" {
		t.Fatalf("name = %q, want %q", name, "Daily review")
	}
	if schedule != "daily" {
		t.Fatalf("schedule = %q, want %q", schedule, "daily")
	}
	if instructions != "Review recent changes" {
		t.Fatalf("instructions = %q, want %q", instructions, "Review recent changes")
	}
	if agent != "codex" {
		t.Fatalf("agent = %q, want %q", agent, "codex")
	}
	if projectPath != "/repos/api" {
		t.Fatalf("project path = %q, want %q", projectPath, "/repos/api")
	}
}

func TestAutomationForm_UnknownDefaultsFallBackToFirstOption(t *testing.T) {
	form := NewAutomationForm(
		"Nightly",
		"@02:00",
		"Run maintenance checks",
		"unknown-agent",
		"/repos/missing",
		[]string{"claude", "codex"},
		[]AutomationRepoOption{
			{Label: "repo-a", Path: "/repos/a"},
			{Label: "repo-b", Path: "/repos/b"},
		},
		false,
	)

	_, _, _, agent, projectPath := form.GetValues()
	if agent != "claude" {
		t.Fatalf("agent default = %q, want %q", agent, "claude")
	}
	if projectPath != "/repos/a" {
		t.Fatalf("project default = %q, want %q", projectPath, "/repos/a")
	}
}
