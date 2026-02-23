package session

import (
	"testing"
)

func TestChatInstanceRoundTrip(t *testing.T) {
	inst := &Instance{
		Title:          "aria",
		IsChat:         true,
		PersonalityDir: "/tmp/chats/aria",
		Status:         Ready,
		Program:        "claude",
	}
	data := inst.ToInstanceData()
	if !data.IsChat {
		t.Error("IsChat not preserved in serialization")
	}
	if data.PersonalityDir != inst.PersonalityDir {
		t.Errorf("PersonalityDir in data = %q, want %q", data.PersonalityDir, inst.PersonalityDir)
	}

	restored := &Instance{
		Title:          data.Title,
		IsChat:         data.IsChat,
		PersonalityDir: data.PersonalityDir,
		Status:         data.Status,
		Program:        data.Program,
	}
	if !restored.IsChat {
		t.Error("IsChat not restored from deserialization")
	}
	if restored.PersonalityDir != inst.PersonalityDir {
		t.Errorf("PersonalityDir = %q, want %q", restored.PersonalityDir, inst.PersonalityDir)
	}
}
