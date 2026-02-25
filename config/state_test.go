package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWorktreePath(t *testing.T) {
	configDir, err := GetConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	worktreeDir := filepath.Join(configDir, "worktrees")

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"worktree path", filepath.Join(worktreeDir, "some-branch_abc123"), true},
		{"worktree dir itself", worktreeDir, true},
		{"nested worktree", filepath.Join(worktreeDir, "user", "branch_123"), true},
		{"normal repo", "/Users/someone/repos/myproject", false},
		{"empty string", "", false},
		{"config dir itself", configDir, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsWorktreePath(tt.path))
		})
	}
}
