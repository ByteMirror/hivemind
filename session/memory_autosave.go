package session

import (
	"time"

	"github.com/ByteMirror/hivemind/log"
)

const (
	// memoryAutoWriteWait is the time we wait after sending the auto-write prompt
	// before returning (fire-and-forget, not waiting for completion).
	memoryAutoWriteWait = 1500 * time.Millisecond

	// memoryAutoWriteKillWait is the time we wait between sending the prompt
	// and actually killing the instance, to give the agent a chance to write.
	memoryAutoWriteKillWait = 3 * time.Second

	// memoryAutoWritePrompt is the message sent to the agent before kill.
	memoryAutoWritePrompt = "Before we close: please call memory_write to record any key learnings " +
		"from this session — architecture decisions, patterns discovered, preferences expressed by the user. " +
		"Use scope=\"repo\" for project-specific notes and scope=\"global\" for preferences that apply across " +
		"all projects. Write a dated file (leave file blank to use today's date).\n"
)

// SendMemoryAutoWritePrompt sends a final prompt to the agent asking it to
// persist session learnings to memory. It is fire-and-forget: we do not wait
// for the agent to respond. Only called when memory is configured.
func SendMemoryAutoWritePrompt(inst *Instance) error {
	// Only send the prompt when memory is configured.
	if getMemoryManager() == nil {
		return nil
	}

	// Only act on started, non-paused instances with an active tmux session.
	if !inst.started.Load() {
		return nil
	}
	if inst.Status == Paused {
		return nil
	}
	if inst.tmuxSession == nil {
		return nil
	}
	if !inst.tmuxSession.DoesSessionExist() {
		return nil
	}

	log.InfoLog.Printf("memory-autosave[%s]: sending auto-write prompt", inst.Title)

	if err := inst.tmuxSession.SendKeys(memoryAutoWritePrompt); err != nil {
		return err
	}

	// Brief pause to give the agent a moment to start processing.
	time.Sleep(memoryAutoWriteWait)
	return nil
}
