package session

import (
	"testing"
	"time"
)

func TestFromTopicData_MigratesLegacySharedWorktree(t *testing.T) {
	// Old JSON had shared_worktree:true — should become Shared mode
	data := TopicData{
		Name:           "my-topic",
		SharedWorktree: true,
		Path:           "/repo",
		CreatedAt:      time.Now(),
	}
	topic := FromTopicData(data)
	if topic.WorktreeMode != TopicWorktreeModeShared {
		t.Errorf("legacy shared_worktree:true → got %q, want %q", topic.WorktreeMode, TopicWorktreeModeShared)
	}
}

func TestFromTopicData_MigratesLegacyNonShared(t *testing.T) {
	// Old JSON had shared_worktree:false — should become PerInstance mode
	data := TopicData{
		Name:           "my-topic",
		SharedWorktree: false,
		Path:           "/repo",
		CreatedAt:      time.Now(),
	}
	topic := FromTopicData(data)
	if topic.WorktreeMode != TopicWorktreeModePerInstance {
		t.Errorf("legacy shared_worktree:false → got %q, want %q", topic.WorktreeMode, TopicWorktreeModePerInstance)
	}
}

func TestFromTopicData_UsesExplicitWorktreeMode(t *testing.T) {
	// New JSON has worktree_mode set — should be used directly, ignoring shared_worktree
	data := TopicData{
		Name:         "my-topic",
		WorktreeMode: TopicWorktreeModeMainRepo,
		Path:         "/repo",
		CreatedAt:    time.Now(),
	}
	topic := FromTopicData(data)
	if topic.WorktreeMode != TopicWorktreeModeMainRepo {
		t.Errorf("explicit worktree_mode → got %q, want %q", topic.WorktreeMode, TopicWorktreeModeMainRepo)
	}
}

func TestToTopicData_RoundTrip(t *testing.T) {
	original := NewTopic(TopicOptions{
		Name:         "round-trip",
		WorktreeMode: TopicWorktreeModeMainRepo,
		Path:         "/repo",
	})
	data := original.ToTopicData()
	if data.WorktreeMode != TopicWorktreeModeMainRepo {
		t.Errorf("ToTopicData WorktreeMode: got %q, want %q", data.WorktreeMode, TopicWorktreeModeMainRepo)
	}
	restored := FromTopicData(data)
	if restored.WorktreeMode != TopicWorktreeModeMainRepo {
		t.Errorf("round-trip WorktreeMode: got %q, want %q", restored.WorktreeMode, TopicWorktreeModeMainRepo)
	}
}
