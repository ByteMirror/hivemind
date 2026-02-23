package memory

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

const maxChunkChars = 800

// Chunk is a text segment from a memory file.
type Chunk struct {
	StartLine int
	EndLine   int
	Text      string
	Hash      string
}

// chunkMarkdown splits Markdown text into chunks by headers and paragraph
// boundaries, with a hard limit of maxChunkChars per chunk.
func chunkMarkdown(text string) []Chunk {
	lines := strings.Split(text, "\n")
	var chunks []Chunk
	var buf []string
	startLine := 1

	flush := func(endLine int) {
		content := strings.TrimSpace(strings.Join(buf, "\n"))
		if content == "" {
			return
		}
		// If chunk is too long, split further.
		if len(content) > maxChunkChars {
			sub := splitLong(content, startLine)
			chunks = append(chunks, sub...)
		} else {
			chunks = append(chunks, makeChunk(content, startLine, endLine))
		}
		buf = nil
	}

	for i, line := range lines {
		lineNum := i + 1
		isHeader := strings.HasPrefix(line, "#")
		if isHeader && len(buf) > 0 {
			flush(lineNum - 1)
			startLine = lineNum
		}
		buf = append(buf, line)
	}
	flush(len(lines))

	return chunks
}

func splitLong(text string, startLine int) []Chunk {
	var chunks []Chunk
	for len(text) > maxChunkChars {
		// Split at last space before limit.
		idx := strings.LastIndex(text[:maxChunkChars], " ")
		if idx <= 0 {
			idx = maxChunkChars
		}
		chunks = append(chunks, makeChunk(strings.TrimSpace(text[:idx]), startLine, startLine))
		text = strings.TrimSpace(text[idx:])
	}
	if text != "" {
		chunks = append(chunks, makeChunk(text, startLine, startLine))
	}
	return chunks
}

func makeChunk(text string, start, end int) Chunk {
	return Chunk{
		StartLine: start,
		EndLine:   end,
		Text:      text,
		Hash:      hashText(text),
	}
}

func hashText(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}
