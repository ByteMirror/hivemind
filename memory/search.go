package memory

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type scoredResult struct {
	SearchResult
	vectorScore float32
	bm25Score   float32
}

// datedFileRe matches filenames of the form YYYY-MM-DD.md (dated journal files).
// Only these files are subject to temporal decay; evergreen files like
// global.md, MEMORY.md, hivemind-project.md are exempt.
var datedFileRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.md$`)

// Search performs hybrid search: FTS5 BM25 + optional cosine similarity,
// followed by optional Claude-based re-ranking.
func (m *Manager) Search(query string, opts SearchOpts) ([]SearchResult, error) {
	if opts.MaxResults <= 0 {
		opts.MaxResults = 10
	}

	// Fetch extra candidates when a reranker is configured so it has
	// enough material to work with before trimming to MaxResults.
	fetchLimit := opts.MaxResults * 2
	if m.reranker != nil && fetchLimit < 20 {
		fetchLimit = 20
	}

	m.mu.RLock()

	// 1. BM25 keyword search via FTS5
	bm25Results, err := m.bm25Search(query, fetchLimit)
	if err != nil {
		m.mu.RUnlock()
		return nil, err
	}

	// 2. Vector search (if embeddings are configured)
	var vecResults []scoredResult
	if m.provider.Dims() > 0 {
		vecResults, err = m.vectorSearch(query, fetchLimit)
		if err != nil {
			// Non-fatal: fall back to keyword-only
			vecResults = nil
		}
	}

	// 3. If FTS5 returned nothing but a reranker is configured, fetch all
	// chunks as candidates so the reranker can find semantically relevant
	// content that keyword search missed (e.g. "projects I work on" vs
	// "repositories" — zero keyword overlap but high semantic relevance).
	if len(bm25Results) == 0 && len(vecResults) == 0 && m.reranker != nil {
		bm25Results, err = m.allChunks(fetchLimit)
		if err != nil {
			bm25Results = nil
		}
	}

	// 4. Merge and apply temporal decay under the read lock.
	merged := mergeResults(bm25Results, vecResults, fetchLimit)
	merged = applyTemporalDecay(merged, m)

	m.mu.RUnlock() // release before any external call

	// 5. Convert to []SearchResult for reranker.
	candidates := make([]SearchResult, len(merged))
	for i, r := range merged {
		candidates[i] = r.SearchResult
	}

	// 6. Apply Claude reranker if configured (external subprocess call).
	if m.reranker != nil {
		candidates, _ = m.reranker.Rerank(query, candidates) // graceful fallback on error
	}

	// 7. Trim to MaxResults and apply min score filter.
	var out []SearchResult
	for _, r := range candidates {
		if r.Score >= opts.MinScore {
			out = append(out, r)
			if len(out) >= opts.MaxResults {
				break
			}
		}
	}
	return out, nil
}

func (m *Manager) bm25Search(query string, limit int) ([]scoredResult, error) {
	// FTS5 bm25() returns negative values (lower = better); negate for consistency.
	rows, err := m.db.Query(`
		SELECT c.id, f.path, c.start_line, c.end_line, c.text,
		       -bm25(chunks_fts) as score
		FROM chunks_fts
		JOIN chunks c ON chunks_fts.rowid = c.id
		JOIN files f ON c.file_id = f.id
		WHERE chunks_fts MATCH ?
		ORDER BY score DESC
		LIMIT ?
	`, ftsQuery(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []scoredResult
	for rows.Next() {
		var chunkID int64
		var r scoredResult
		if err := rows.Scan(&chunkID, &r.Path, &r.StartLine, &r.EndLine, &r.Snippet, &r.bm25Score); err != nil {
			continue
		}
		r.Snippet = truncate(r.Snippet, snippetMaxChars)
		results = append(results, r)
	}
	return results, rows.Err()
}

// allChunks returns every indexed chunk with a uniform score of 0.
// Used as fallback candidates when FTS5 produces no hits but a reranker
// is available to do semantic selection.
func (m *Manager) allChunks(limit int) ([]scoredResult, error) {
	rows, err := m.db.Query(`
		SELECT f.path, c.start_line, c.end_line, c.text
		FROM chunks c
		JOIN files f ON c.file_id = f.id
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []scoredResult
	for rows.Next() {
		var r scoredResult
		if err := rows.Scan(&r.Path, &r.StartLine, &r.EndLine, &r.Snippet); err != nil {
			continue
		}
		r.Snippet = truncate(r.Snippet, snippetMaxChars)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (m *Manager) vectorSearch(query string, limit int) ([]scoredResult, error) {
	vecs, err := m.provider.Embed([]string{query})
	if err != nil || len(vecs) == 0 {
		return nil, err
	}
	queryVec := vecs[0]

	// Load all chunk vectors into memory.
	rows, err := m.db.Query(`
		SELECT cv.chunk_id, cv.embedding, c.start_line, c.end_line, c.text, f.path
		FROM chunks_vec cv
		JOIN chunks c ON cv.chunk_id = c.id
		JOIN files f ON c.file_id = f.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []scoredResult
	for rows.Next() {
		var chunkID int64
		var blob []byte
		var r scoredResult
		if err := rows.Scan(&chunkID, &blob, &r.StartLine, &r.EndLine, &r.Snippet, &r.Path); err != nil {
			continue
		}
		chunkVec := deserializeVec(blob)
		r.vectorScore = cosine(queryVec, chunkVec)
		r.Snippet = truncate(r.Snippet, snippetMaxChars)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by vector score and take top-limit.
	sort.Slice(results, func(i, j int) bool {
		return results[i].vectorScore > results[j].vectorScore
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func mergeResults(bm25, vec []scoredResult, limit int) []scoredResult {
	seen := map[string]int{} // key -> index in merged
	var merged []scoredResult

	// Normalize scores before merging
	bm25Norm := normalizeScores(bm25, true)
	vecNorm := normalizeScores(vec, false)

	for _, r := range bm25Norm {
		key := fmt.Sprintf("%s:%d", r.Path, r.StartLine)
		r.Score = 0.4 * r.bm25Score
		seen[key] = len(merged)
		merged = append(merged, r)
	}
	for _, r := range vecNorm {
		key := fmt.Sprintf("%s:%d", r.Path, r.StartLine)
		if idx, ok := seen[key]; ok {
			merged[idx].Score += 0.6 * r.vectorScore
		} else {
			r.Score = 0.6 * r.vectorScore
			merged = append(merged, r)
		}
	}

	sort.Slice(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

// normalizeScores normalizes bm25 or vector scores to 0-1 range.
func normalizeScores(results []scoredResult, isBM25 bool) []scoredResult {
	if len(results) == 0 {
		return results
	}
	var max float32
	for _, r := range results {
		var s float32
		if isBM25 {
			s = r.bm25Score
		} else {
			s = r.vectorScore
		}
		if s > max {
			max = s
		}
	}
	if max == 0 {
		return results
	}
	out := make([]scoredResult, len(results))
	for i, r := range results {
		out[i] = r
		if isBM25 {
			out[i].bm25Score = r.bm25Score / max
		} else {
			out[i].vectorScore = r.vectorScore / max
		}
	}
	return out
}

func applyTemporalDecay(results []scoredResult, m *Manager) []scoredResult {
	now := float64(time.Now().UnixMilli())
	for i := range results {
		// Only apply temporal decay to dated files (YYYY-MM-DD.md).
		// Evergreen files like global.md, MEMORY.md, hivemind-project.md
		// contain stable facts that remain relevant forever and must not decay.
		if !datedFileRe.MatchString(filepath.Base(results[i].Path)) {
			continue
		}
		var mtime int64
		row := m.db.QueryRow("SELECT mtime FROM files WHERE path=?", results[i].Path)
		if err := row.Scan(&mtime); err != nil {
			continue
		}
		ageDays := (now - float64(mtime)) / 86_400_000
		decay := math.Exp(-0.01 * ageDays)
		results[i].Score *= float32(decay)
	}
	// Re-sort after decay.
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results
}

// ftsStopWords are common English words that carry no search signal and
// cause FTS5 to produce low-quality matches when included in queries.
var ftsStopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true, "could": true,
	"should": true, "may": true, "might": true, "i": true, "me": true, "my": true,
	"we": true, "our": true, "you": true, "your": true, "it": true, "its": true,
	"that": true, "this": true, "what": true, "which": true, "who": true, "how": true,
	"all": true, "some": true, "any": true, "no": true, "not": true, "so": true,
	"up": true, "out": true, "about": true, "work": true, "use": true, "get": true,
}

// ftsQuery sanitizes a query string for FTS5 MATCH syntax.
// Stop words are stripped so natural-language queries like "projects I work on"
// reduce to meaningful tokens ("projects") rather than flooding FTS5 with
// high-frequency words that score poorly.
// If stripping removes all tokens, the original query is used as-is.
func ftsQuery(q string) string {
	tokens := strings.Fields(q)
	if len(tokens) == 0 {
		return q
	}
	var meaningful []string
	for _, t := range tokens {
		if !ftsStopWords[strings.ToLower(t)] {
			meaningful = append(meaningful, t)
		}
	}
	if len(meaningful) == 0 {
		meaningful = tokens // fallback: use everything if all were stop words
	}
	quoted := make([]string, len(meaningful))
	for i, t := range meaningful {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, "") + `"`
	}
	return strings.Join(quoted, " OR ")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
