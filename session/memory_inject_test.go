package session

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertMemorySection_AppendsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/CLAUDE.md"
	require.NoError(t, os.WriteFile(path, []byte("# Existing\n\nContent here.\n"), 0600))

	require.NoError(t, upsertMemorySection(path, "<!-- hivemind-memory-start -->\n## New\n<!-- hivemind-memory-end -->\n"))

	data, _ := os.ReadFile(path)
	s := string(data)
	assert.Contains(t, s, "# Existing")
	assert.Contains(t, s, "## New")
}

func TestUpsertMemorySection_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/CLAUDE.md"
	initial := "# Top\n<!-- hivemind-memory-start -->\n## Old\n<!-- hivemind-memory-end -->\n# Bottom\n"
	require.NoError(t, os.WriteFile(path, []byte(initial), 0600))

	require.NoError(t, upsertMemorySection(path, "<!-- hivemind-memory-start -->\n## New\n<!-- hivemind-memory-end -->\n"))

	data, _ := os.ReadFile(path)
	s := string(data)
	assert.NotContains(t, s, "## Old")
	assert.Contains(t, s, "## New")
	assert.Contains(t, s, "# Top")
	assert.Contains(t, s, "# Bottom")
}
