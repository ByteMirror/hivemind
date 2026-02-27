package app

import "testing"

func TestBuildAutomationRepoOptions_DisambiguatesDuplicateRepoNames(t *testing.T) {
	repos := []string{
		"/Users/fabian/client/hivemind",
		"/Users/fabian/sandbox/hivemind",
	}

	opts := buildAutomationRepoOptions(repos)
	if len(opts) != 2 {
		t.Fatalf("len(opts) = %d, want 2", len(opts))
	}

	if opts[0].Path != repos[0] {
		t.Fatalf("opts[0].Path = %q, want %q", opts[0].Path, repos[0])
	}
	if opts[1].Path != repos[1] {
		t.Fatalf("opts[1].Path = %q, want %q", opts[1].Path, repos[1])
	}

	if opts[0].Label == "hivemind" || opts[1].Label == "hivemind" {
		t.Fatalf("duplicate repo names must be disambiguated, got labels %q and %q", opts[0].Label, opts[1].Label)
	}
}

func TestBuildAutomationRepoOptions_UsesBaseNameWhenUnique(t *testing.T) {
	opts := buildAutomationRepoOptions([]string{
		"/Users/fabian/Github/hivemind",
		"/Users/fabian/Github/another-repo",
	})
	if len(opts) != 2 {
		t.Fatalf("len(opts) = %d, want 2", len(opts))
	}

	if opts[0].Label != "hivemind" {
		t.Fatalf("opts[0].Label = %q, want %q", opts[0].Label, "hivemind")
	}
	if opts[1].Label != "another-repo" {
		t.Fatalf("opts[1].Label = %q, want %q", opts[1].Label, "another-repo")
	}
}
