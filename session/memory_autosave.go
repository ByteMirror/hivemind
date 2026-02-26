package session

import (
	"fmt"
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
// calling this function. Kill() uses CompareAndSwap to guarantee single-caller
// semantics before invoking this.
//
// Returns sent=true only when the prompt was successfully delivered.
func SendMemoryAutoWritePrompt(inst *Instance) (sent bool, err error) {
	// Only send the prompt when memory is configured.
	if getMemoryManager() == nil {
		return false, nil
	}
	if inst.tmuxSession == nil {
		return false, nil
	}
	if !inst.tmuxSession.DoesSessionExist() {
		return false, nil
	}

	log.InfoLog.Printf("memory-autosave[%s]: sending auto-write prompt", inst.Title)

	// Prefer tmux server-side send-keys so the prompt is delivered reliably
	// even when no client is attached.
	if err := inst.tmuxSession.SendTextViaTmux(memoryAutoWritePrompt); err == nil {
		return true, nil
	}

	// Fallback to direct PTY write for compatibility with environments where
	// tmux send-keys is unavailable.
	if err := inst.tmuxSession.SendKeys(memoryAutoWritePrompt); err != nil {
		return false, fmt.Errorf("send auto-write prompt: %w", err)
	}
	return true, nil
}

// KillInstanceAsync runs Kill() in a background goroutine.
// The operation is tracked via closeWg so shutdown can wait for completion.
func KillInstanceAsync(inst *Instance) {
	if inst == nil {
		return
	}
	closeWg.Add(1)
	go func() {
		defer closeWg.Done()
		if err := inst.Kill(); err != nil {
			log.ErrorLog.Printf("kill[%s]: %v", inst.Title, err)
		}
	}()
}
