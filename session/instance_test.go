package session

import (
	"testing"
	"time"
)

func TestInstance_ReviewFields_RoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	inst := &Instance{
		Title:         "test",
		AutomationID:  "auto-1",
		PendingReview: true,
		CompletedAt:   &now,
	}
	data := inst.ToInstanceData()

	if data.AutomationID != "auto-1" {
		t.Errorf("AutomationID: got %q, want %q", data.AutomationID, "auto-1")
	}
	if !data.PendingReview {
		t.Error("PendingReview: expected true")
	}
	if data.CompletedAt == nil || !data.CompletedAt.Equal(now) {
		t.Errorf("CompletedAt: got %v, want %v", data.CompletedAt, now)
	}

	restored, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData: %v", err)
	}
	if restored.AutomationID != "auto-1" {
		t.Errorf("restored AutomationID: got %q", restored.AutomationID)
	}
	if !restored.PendingReview {
		t.Error("restored PendingReview: expected true")
	}
}

func TestSetStatus_SetsReviewForAutomationInstance(t *testing.T) {
	inst := &Instance{
		Title:        "auto-agent",
		AutomationID: "auto-42",
		Status:       Running,
	}
	inst.SetStatus(Ready)

	if !inst.PendingReview {
		t.Error("PendingReview should be true for automation instance")
	}
	if inst.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestSetStatus_NoReviewForManualInstance(t *testing.T) {
	inst := &Instance{
		Title:  "manual-agent",
		Status: Running,
	}
	inst.SetStatus(Ready)

	if inst.PendingReview {
		t.Error("PendingReview should be false for manual instance")
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
