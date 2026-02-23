package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkMarkdown_SplitsOnHeaders(t *testing.T) {
	md := "# Section A\n\nSome content here.\n\n## Section B\n\nMore content."
	chunks := chunkMarkdown(md)
	require.Len(t, chunks, 2)
	assert.Contains(t, chunks[0].Text, "Section A")
	assert.Contains(t, chunks[0].Text, "Some content here.")
	assert.Contains(t, chunks[1].Text, "Section B")
	assert.Contains(t, chunks[1].Text, "More content.")
}

func TestChunkMarkdown_SingleChunkNoHeaders(t *testing.T) {
	md := "Just a single paragraph with no headers."
	chunks := chunkMarkdown(md)
	require.Len(t, chunks, 1)
	assert.Equal(t, md, chunks[0].Text)
	assert.Equal(t, 1, chunks[0].StartLine)
	assert.Equal(t, 1, chunks[0].EndLine)
}

func TestChunkMarkdown_LongParagraphSplit(t *testing.T) {
	// 900 chars of content, no headers — should split into 2 chunks
	long := ""
	for i := 0; i < 90; i++ {
		long += "word_number_" + string(rune('a'+i%26)) + " "
	}
	chunks := chunkMarkdown(long)
	assert.Greater(t, len(chunks), 1, "long paragraph should be split")
	for _, c := range chunks {
		assert.LessOrEqual(t, len(c.Text), maxChunkChars+50)
	}
}

func TestChunkMarkdown_LineNumbers(t *testing.T) {
	md := "Line one\nLine two\n\n# Header\n\nAfter header"
	chunks := chunkMarkdown(md)
	require.Len(t, chunks, 2)
	assert.Equal(t, 1, chunks[0].StartLine)
	assert.Equal(t, 4, chunks[1].StartLine)
}
