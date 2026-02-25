package session

import (
	"testing"
)

func TestInstance_ReviewFields_RoundTrip(t *testing.T) {
	inst := &Instance{
		Title:        "test",
		AutomationID: "auto-1",
	}
	data := inst.ToInstanceData()

	if data.AutomationID != "auto-1" {
		t.Errorf("AutomationID: got %q, want %q", data.AutomationID, "auto-1")
	}

	restored, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData: %v", err)
	}
	if restored.AutomationID != "auto-1" {
		t.Errorf("restored AutomationID: got %q", restored.AutomationID)
	}
}

func TestInstance_GetWorkingPath(t *testing.T) {
	t.Run("returns repo path when no worktree", func(t *testing.T) {
		inst := &Instance{Path: "/my/repo"}
		inst.started.Store(true)
		if got := inst.GetWorkingPath(); got != "/my/repo" {
			t.Errorf("got %q, want /my/repo", got)
		}
	})
}
