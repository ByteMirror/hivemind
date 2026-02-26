package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ByteMirror/hivemind/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitEnabledFromConfig_DefaultTrue(t *testing.T) {
	assert.True(t, GitEnabledFromConfig(nil))
	assert.True(t, GitEnabledFromConfig(&config.Config{}))
	assert.True(t, GitEnabledFromConfig(&config.Config{Memory: &config.MemoryConfig{Enabled: true}}))
}

func TestNewManagerFromConfig_RespectsGitEnabledFalse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	falseVal := false
	cfg := &config.Config{
		Memory: &config.MemoryConfig{
			Enabled:           true,
			EmbeddingProvider: "none",
			GitEnabled:        &falseVal,
		},
	}

	mgr, err := NewManagerFromConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, mgr)
	t.Cleanup(func() { mgr.Close() })

	assert.False(t, mgr.GitEnabled())

	_, statErr := os.Stat(filepath.Join(home, ".hivemind", "memory", ".git"))
	assert.True(t, os.IsNotExist(statErr))
}
