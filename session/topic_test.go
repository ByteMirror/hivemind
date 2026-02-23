package session

import (
	"testing"
)

func TestTopicWorktreeMode_Helpers(t *testing.T) {
	tests := []struct {
		mode       TopicWorktreeMode
		wantShared bool
		wantMain   bool
	}{
		{TopicWorktreeModePerInstance, false, false},
		{TopicWorktreeModeShared, true, false},
		{TopicWorktreeModeMainRepo, false, true},
	}
	for _, tc := range tests {
		topic := &Topic{WorktreeMode: tc.mode}
		if got := topic.IsSharedWorktree(); got != tc.wantShared {
			t.Errorf("mode %q IsSharedWorktree(): got %v, want %v", tc.mode, got, tc.wantShared)
		}
		if got := topic.IsMainRepo(); got != tc.wantMain {
			t.Errorf("mode %q IsMainRepo(): got %v, want %v", tc.mode, got, tc.wantMain)
		}
	}
}

func TestNewTopic_DefaultMode(t *testing.T) {
	topic := NewTopic(TopicOptions{Name: "t", Path: "/repo"})
	if topic.WorktreeMode != TopicWorktreeModePerInstance {
		t.Errorf("default WorktreeMode: got %q, want %q", topic.WorktreeMode, TopicWorktreeModePerInstance)
	}
}
