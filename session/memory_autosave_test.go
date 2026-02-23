package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSendMemoryAutoWritePrompt_NoopWhenMemoryDisabled verifies that
// SendMemoryAutoWritePrompt returns nil immediately when no memory manager
// is configured (the common case for users who have not enabled memory).
func TestSendMemoryAutoWritePrompt_NoopWhenMemoryDisabled(t *testing.T) {
	// Ensure global memory manager is nil for this test.
	// We save and restore in case another test has set it.
	memMu.Lock()
	prev := globalMemMgr
	globalMemMgr = nil
	memMu.Unlock()
	defer func() {
		memMu.Lock()
		globalMemMgr = prev
		memMu.Unlock()
	}()

	inst := &Instance{
		Title: "test-instance",
	}
	// getMemoryManager() returns nil so the function should return early with nil.
	err := SendMemoryAutoWritePrompt(inst)
	assert.NoError(t, err)
}

// TestSendMemoryAutoWritePrompt_NoopWhenPaused verifies that the prompt is
// skipped for a paused instance even when memory is configured.
// A paused instance has no active tmux session.
func TestSendMemoryAutoWritePrompt_NoopWhenPaused(t *testing.T) {
	// Set a real memory manager via SetMemoryManager.
	mgr, dir := newTestMemoryManager(t)
	_ = dir
	SetMemoryManager(mgr, 5)
	defer SetMemoryManager(nil, 0)

	inst := &Instance{
		Title:  "test-instance",
		Status: Paused,
	}
	inst.started.Store(true)

	// Instance is paused — SendMemoryAutoWritePrompt must return nil without
	// attempting to contact the (nil) tmux session.
	err := SendMemoryAutoWritePrompt(inst)
	assert.NoError(t, err)
}

// TestSendMemoryAutoWritePrompt_NoopWhenNotStarted verifies that the prompt
// is skipped for an instance that has never been started.
func TestSendMemoryAutoWritePrompt_NoopWhenNotStarted(t *testing.T) {
	mgr, dir := newTestMemoryManager(t)
	_ = dir
	SetMemoryManager(mgr, 5)
	defer SetMemoryManager(nil, 0)

	inst := &Instance{
		Title: "test-instance",
	}
	// started is false (default zero value of atomic.Bool).

	err := SendMemoryAutoWritePrompt(inst)
	assert.NoError(t, err)
}
