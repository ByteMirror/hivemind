package memory

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type scoredResult struct {
	SearchResult
	vectorScore float32
	bm25Score   float32
}

// Search performs hybrid search: FTS5 BM25 + optional cosine similarity.
func (m *Manager) Search(query string, opts SearchOpts) ([]SearchResult, error) {
	if opts.MaxResults <= 0 {
		opts.MaxResults = 10
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. BM25 keyword search via FTS5
	bm25Results, err := m.bm25Search(query, opts.MaxResults*2)
	if err != nil {
		return nil, err
	}

	// 2. Vector search (if embeddings are configured)
	var vecResults []scoredResult
	if m.provider.Dims() > 0 {
		vecResults, err = m.vectorSearch(query, opts.MaxResults*2)
		if err != nil {
			// Non-fatal: fall back to keyword-only
			vecResults = nil
		}
	}

	// 3. Merge results
	merged := mergeResults(bm25Results, vecResults, opts.MaxResults)

	// 4. Apply temporal decay
	merged = applyTemporalDecay(merged, m)

	// 5. Apply min score filter and return
	var out []SearchResult
	for _, r := range merged {
		if r.Score >= opts.MinScore {
			out = append(out, r.SearchResult)
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

// ftsQuery sanitizes a query string for FTS5 MATCH syntax.
func ftsQuery(q string) string {
	tokens := strings.Fields(q)
	if len(tokens) == 0 {
		return q
	}
	quoted := make([]string, len(tokens))
	for i, t := range tokens {
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
