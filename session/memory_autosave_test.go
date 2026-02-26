package session

import (
	"io"
	stdlog "log"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ByteMirror/hivemind/cmd"
	hlog "github.com/ByteMirror/hivemind/log"
	"github.com/ByteMirror/hivemind/session/tmux"
	"github.com/stretchr/testify/assert"
)

type mockCmdExec struct {
	runCalls []string
	runErr   error
}

func (m *mockCmdExec) Run(c *exec.Cmd) error {
	m.runCalls = append(m.runCalls, strings.Join(c.Args, " "))
	return m.runErr
}

func (m *mockCmdExec) Output(c *exec.Cmd) ([]byte, error) {
	return []byte{}, nil
}

var _ cmd.Executor = (*mockCmdExec)(nil)

func initTestLoggers() {
	if hlog.InfoLog == nil {
		hlog.InfoLog = stdlog.New(io.Discard, "", 0)
	}
	if hlog.WarningLog == nil {
		hlog.WarningLog = stdlog.New(io.Discard, "", 0)
	}
	if hlog.ErrorLog == nil {
		hlog.ErrorLog = stdlog.New(io.Discard, "", 0)
	}
}

// TestSendMemoryAutoWritePrompt_NoopWhenMemoryDisabled verifies that
// SendMemoryAutoWritePrompt returns immediately when no memory manager
// is configured (the common case for users who have not enabled memory).
func TestSendMemoryAutoWritePrompt_NoopWhenMemoryDisabled(t *testing.T) {
	initTestLoggers()

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
	// getMemoryManager() returns nil so the function should return early.
	sent, err := SendMemoryAutoWritePrompt(inst)
	assert.NoError(t, err)
	assert.False(t, sent)
}

// TestSendMemoryAutoWritePrompt_NoopWhenNoSession verifies that prompt delivery
// is skipped when no tmux session is available.
func TestSendMemoryAutoWritePrompt_NoopWhenNoSession(t *testing.T) {
	initTestLoggers()

	// Set a real memory manager via SetMemoryManager.
	mgr, dir := newTestMemoryManager(t)
	_ = dir
	SetMemoryManager(mgr, 5, 0)
	defer SetMemoryManager(nil, 0, 0)

	inst := &Instance{
		Title:  "test-instance",
		Status: Paused,
	}
	inst.started.Store(true)

	sent, err := SendMemoryAutoWritePrompt(inst)
	assert.NoError(t, err)
	assert.False(t, sent)
}

// TestSendMemoryAutoWritePrompt_SendsForPausedSession verifies that paused
// sessions are still prompted when the tmux session is alive.
func TestSendMemoryAutoWritePrompt_SendsForPausedSession(t *testing.T) {
	initTestLoggers()

	mgr, dir := newTestMemoryManager(t)
	_ = dir
	SetMemoryManager(mgr, 5, 0)
	defer SetMemoryManager(nil, 0, 0)

	execMock := &mockCmdExec{}
	tmuxSession := tmux.NewTmuxSessionWithDeps("test-instance", "claude", false, nil, execMock)

	inst := &Instance{
		Title:       "test-instance",
		Status:      Paused,
		tmuxSession: tmuxSession,
	}
	inst.started.Store(true)

	sent, err := SendMemoryAutoWritePrompt(inst)
	assert.NoError(t, err)
	assert.True(t, sent)
	assert.GreaterOrEqual(t, len(execMock.runCalls), 3)
	assert.Contains(t, execMock.runCalls[0], "has-session")
	assert.Contains(t, execMock.runCalls[1], "send-keys")
	assert.Contains(t, execMock.runCalls[2], "Enter")
}

// TestKill_NoAutosaveDelayWhenPromptNotSent verifies Kill doesn't sleep for the
// autosave delay when prompt delivery is skipped.
func TestKill_NoAutosaveDelayWhenPromptNotSent(t *testing.T) {
	initTestLoggers()

	mgr, dir := newTestMemoryManager(t)
	_ = dir
	SetMemoryManager(mgr, 5, 0)
	defer SetMemoryManager(nil, 0, 0)

	inst := &Instance{Title: "kill-no-prompt"}
	inst.started.Store(true)

	start := time.Now()
	err := inst.Kill()
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Less(t, elapsed, 2*time.Second)
}
