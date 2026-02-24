package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFrontmatter_Valid(t *testing.T) {
	content := "---\ndescription: User preferences\nread-only: true\n---\n# Preferences\nGo is great.\n"
	fm, body := ParseFrontmatter(content)
	assert.Equal(t, "User preferences", fm.Description)
	assert.True(t, fm.ReadOnly)
	assert.Equal(t, "# Preferences\nGo is great.\n", body)
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just Markdown\n\nNo frontmatter here.\n"
	fm, body := ParseFrontmatter(content)
	assert.Equal(t, Frontmatter{}, fm)
	assert.Equal(t, content, body)
}

func TestParseFrontmatter_EmptyFile(t *testing.T) {
	fm, body := ParseFrontmatter("")
	assert.Equal(t, Frontmatter{}, fm)
	assert.Equal(t, "", body)
}

func TestParseFrontmatter_DescriptionOnly(t *testing.T) {
	content := "---\ndescription: Project notes\n---\nSome body text.\n"
	fm, body := ParseFrontmatter(content)
	assert.Equal(t, "Project notes", fm.Description)
	assert.False(t, fm.ReadOnly)
	assert.Equal(t, "Some body text.\n", body)
}

func TestFormatFrontmatter_RoundTrip(t *testing.T) {
	original := Frontmatter{Description: "Test file", ReadOnly: true}
	body := "# Hello\nWorld.\n"

	formatted := FormatFrontmatter(original, body)
	parsed, parsedBody := ParseFrontmatter(formatted)
	assert.Equal(t, original.Description, parsed.Description)
	assert.Equal(t, original.ReadOnly, parsed.ReadOnly)
	assert.Equal(t, body, parsedBody)
}

func TestFormatFrontmatter_ZeroValue(t *testing.T) {
	body := "# No metadata\n"
	result := FormatFrontmatter(Frontmatter{}, body)
	assert.Equal(t, body, result, "zero-value frontmatter should return body unchanged")
}

func TestReadFileFrontmatter(t *testing.T) {
	mgr := newTestManager(t)
	content := "---\ndescription: Hardware info\n---\n# Setup\nMacBook Pro M4.\n"
	require.NoError(t, os.WriteFile(filepath.Join(mgr.dir, "setup.md"), []byte(content), 0600))

	fm, err := mgr.ReadFileFrontmatter("setup.md")
	require.NoError(t, err)
	assert.Equal(t, "Hardware info", fm.Description)
	assert.False(t, fm.ReadOnly)
}

func TestReadFileFrontmatter_NoFrontmatter(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, os.WriteFile(filepath.Join(mgr.dir, "plain.md"), []byte("# Plain\nNo FM.\n"), 0600))

	fm, err := mgr.ReadFileFrontmatter("plain.md")
	require.NoError(t, err)
	assert.Equal(t, Frontmatter{}, fm)
}
