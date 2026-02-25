package session

import (
	"sync"
	"time"

	"github.com/ByteMirror/hivemind/log"
)

// closeWg tracks in-flight Kill() calls so the TUI can wait for them to
// finish before exiting (e.g. letting the memory autosave sleep complete).
var closeWg sync.WaitGroup

// WaitForAllClosing blocks until all in-flight Kill() calls finish or the
// timeout elapses. Called from the TUI quit handler to avoid exiting while
// instances are still shutting down.
func WaitForAllClosing(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		closeWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.WarningLog.Printf("WaitForAllClosing: timed out after %v", timeout)
	}
}

const (
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
//
// Callers are responsible for ensuring the instance is in a valid state before
// calling this function (started, not paused, tmux session exists). Kill() uses
// CompareAndSwap to guarantee single-caller semantics before invoking this.
func SendMemoryAutoWritePrompt(inst *Instance) error {
	// Only send the prompt when memory is configured.
	if getMemoryManager() == nil {
		return nil
	}

	// Only act on non-paused instances with an active tmux session.
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

	// Best-effort: if the agent is mid-task, these keystrokes may go to a child
	// process rather than Claude's input buffer. We accept this race — the
	// auto-write is a courtesy prompt, not a guarantee.
	if err := inst.tmuxSession.SendKeys(memoryAutoWritePrompt); err != nil {
		return err
	}

	return nil
}
