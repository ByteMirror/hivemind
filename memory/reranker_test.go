package memory

import (
	"testing"
)

func TestParseRankIndices_Valid(t *testing.T) {
	cases := []struct {
		name   string
		output string
		n      int
		want   []int
	}{
		{"clean array", "[2,0,3,1]", 4, []int{2, 0, 3, 1}},
		{"with spaces", "[ 1, 0, 2 ]", 3, []int{1, 0, 2}},
		{"embedded in text", "Here you go: [2,0,1] done.", 3, []int{2, 0, 1}},
		{"single element", "[0]", 1, []int{0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRankIndices(tc.output, tc.n)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %d, want %d", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseRankIndices_Invalid(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"no array", "I cannot rank these."},
		{"empty array", "[]"},
		{"non-integer array", `["a","b"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseRankIndices(tc.output, 3)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestApplyRankIndices(t *testing.T) {
	results := []SearchResult{
		{Path: "a.md", Snippet: "alpha"},
		{Path: "b.md", Snippet: "beta"},
		{Path: "c.md", Snippet: "gamma"},
	}

	t.Run("reorders correctly", func(t *testing.T) {
		got := applyRankIndices(results, []int{2, 0, 1})
		if got[0].Path != "c.md" || got[1].Path != "a.md" || got[2].Path != "b.md" {
			t.Errorf("unexpected order: %v", got)
		}
	})

	t.Run("appends omitted results", func(t *testing.T) {
		// Model only returns indices for 2 of 3 results
		got := applyRankIndices(results, []int{1, 0})
		if len(got) != 3 {
			t.Fatalf("expected 3 results, got %d", len(got))
		}
		if got[0].Path != "b.md" || got[1].Path != "a.md" || got[2].Path != "c.md" {
			t.Errorf("unexpected order: %v", got)
		}
	})

	t.Run("deduplicates repeated indices", func(t *testing.T) {
		got := applyRankIndices(results, []int{0, 0, 1})
		if len(got) != 3 {
			t.Fatalf("expected 3 results, got %d", len(got))
		}
	})
}

func TestBuildRerankPrompt_ContainsQueryAndSnippets(t *testing.T) {
	results := []SearchResult{
		{Snippet: "I use Go for backend."},
		{Snippet: "MacBook Pro M3."},
	}
	prompt := buildRerankPrompt("programming language", results)
	if !contains(prompt, `"programming language"`) {
		t.Error("prompt missing query")
	}
	if !contains(prompt, "[0]") || !contains(prompt, "[1]") {
		t.Error("prompt missing snippet indices")
	}
	if !contains(prompt, "I use Go for backend.") {
		t.Error("prompt missing snippet text")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
