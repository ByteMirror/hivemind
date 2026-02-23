package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// DefaultRerankModel is the default Claude model used for re-ranking.
const DefaultRerankModel = "claude-haiku-4-5-20251001"

const rerankTimeout = 30 * time.Second

// ClaudeReranker uses a local claude CLI process to re-rank FTS5 keyword
// results by semantic relevance. It works with any claude auth method —
// API key or Max subscription — because auth is handled by the claude binary.
type ClaudeReranker struct {
	model      string
	claudePath string // resolved path to claude binary
}

// NewClaudeReranker creates a ClaudeReranker.
// If model is empty, defaults to claude-haiku-4-5-20251001.
// Returns an error if the claude binary is not found in PATH.
func NewClaudeReranker(model string) (*ClaudeReranker, error) {
	if model == "" {
		model = DefaultRerankModel
	}
	path, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude binary not found in PATH: %w", err)
	}
	return &ClaudeReranker{model: model, claudePath: path}, nil
}

// Name implements Reranker.
func (r *ClaudeReranker) Name() string { return "claude-reranker:" + r.model }

// Rerank re-orders results by semantic relevance to the query.
// On any error (timeout, parse failure, claude unavailable), it returns
// the original results unchanged — search still works via FTS5 order.
func (r *ClaudeReranker) Rerank(query string, results []SearchResult) ([]SearchResult, error) {
	if len(results) <= 1 {
		return results, nil
	}

	prompt := buildRerankPrompt(query, results)

	ctx, cancel := context.WithTimeout(context.Background(), rerankTimeout)
	defer cancel()

	// -p flag: non-interactive print mode; read prompt from stdin.
	cmd := exec.CommandContext(ctx, r.claudePath, "-p", "--model", r.model)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		return results, nil // graceful fallback
	}

	indices, err := parseRankIndices(string(out), len(results))
	if err != nil {
		return results, nil // graceful fallback
	}

	return applyRankIndices(results, indices), nil
}

func buildRerankPrompt(query string, results []SearchResult) string {
	var b strings.Builder
	b.WriteString("You are a relevance ranker for a developer memory store.\n")
	b.WriteString("Given the search query and text snippets below, return ONLY a JSON\n")
	b.WriteString("array of snippet indices sorted from most to least relevant.\n")
	b.WriteString("Example response for 4 snippets: [2,0,3,1]\n\n")
	fmt.Fprintf(&b, "Query: %q\n\nSnippets:\n", query)
	for i, r := range results {
		fmt.Fprintf(&b, "[%d] %s\n", i, truncate(r.Snippet, 300))
	}
	return b.String()
}

// jsonArrayRE matches a JSON array of integers, e.g. [2, 0, 3, 1].
var jsonArrayRE = regexp.MustCompile(`\[\s*\d[\d\s,]*\]`)

func parseRankIndices(output string, n int) ([]int, error) {
	match := jsonArrayRE.FindString(strings.TrimSpace(output))
	if match == "" {
		return nil, fmt.Errorf("no integer JSON array in response: %q", output)
	}
	var indices []int
	if err := json.Unmarshal([]byte(match), &indices); err != nil {
		return nil, err
	}
	if len(indices) == 0 {
		return nil, fmt.Errorf("empty index array")
	}
	return indices, nil
}

func applyRankIndices(results []SearchResult, indices []int) []SearchResult {
	seen := make(map[int]bool, len(indices))
	out := make([]SearchResult, 0, len(results))
	for _, idx := range indices {
		if idx >= 0 && idx < len(results) && !seen[idx] {
			out = append(out, results[idx])
			seen[idx] = true
		}
	}
	// Append any results the model omitted to preserve completeness.
	for i, r := range results {
		if !seen[i] {
			out = append(out, r)
		}
	}
	return out
}
