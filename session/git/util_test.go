package git

import (
	"strings"
	"testing"
)

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple lowercase string",
			input:    "feature",
			expected: "feature",
		},
		{
			name:     "string with spaces",
			input:    "new feature branch",
			expected: "new-feature-branch",
		},
		{
			name:     "mixed case string",
			input:    "FeAtUrE BrAnCh",
			expected: "feature-branch",
		},
		{
			name:     "string with special characters",
			input:    "feature!@#$%^&*()",
			expected: "feature",
		},
		{
			name:     "string with allowed special characters",
			input:    "feature/sub_branch.v1",
			expected: "feature/sub_branch.v1",
		},
		{
			name:     "string with multiple dashes",
			input:    "feature---branch",
			expected: "feature-branch",
		},
		{
			name:     "string with leading and trailing dashes",
			input:    "-feature-branch-",
			expected: "feature-branch",
		},
		{
			name:     "string with leading and trailing slashes",
			input:    "/feature/branch/",
			expected: "feature/branch",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "complex mixed case with special chars",
			input:    "USER/Feature Branch!@#$%^&*()/v1.0",
			expected: "user/feature-branch/v1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeBranchName(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeBranchName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsValidBranchName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "valid simple", input: "feature/test", expected: true},
		{name: "empty", input: "", expected: false},
		{name: "double dot", input: "feature..test", expected: false},
		{name: "double slash", input: "feature//test", expected: false},
		{name: "leading slash", input: "/feature", expected: false},
		{name: "ends with dot", input: "feature.", expected: false},
		{name: "contains at brace", input: "feature@{x}", expected: false},
		{name: "contains space", input: "feature test", expected: false},
		{name: "contains quote removed upstream", input: "''", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidBranchName(tt.input)
			if got != tt.expected {
				t.Errorf("isValidBranchName(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMakeSafeBranchName_NeverReturnsInvalid(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		session    string
		wantPrefix string
	}{
		{
			name:       "normal input keeps user prefix",
			prefix:     "user/",
			session:    "feature panel fix",
			wantPrefix: "user/",
		},
		{
			name:       "invalid prefix and session still returns safe branch",
			prefix:     "'",
			session:    "'''",
			wantPrefix: "session/",
		},
		{
			name:       "empty everything falls back",
			prefix:     "",
			session:    "",
			wantPrefix: "session/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeSafeBranchName(tt.prefix, tt.session)
			if !isValidBranchName(got) {
				t.Fatalf("makeSafeBranchName(%q, %q) returned invalid branch %q", tt.prefix, tt.session, got)
			}
			if got == "" {
				t.Fatalf("makeSafeBranchName(%q, %q) returned empty branch", tt.prefix, tt.session)
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(got, tt.wantPrefix) {
				t.Fatalf("makeSafeBranchName(%q, %q) = %q, expected prefix %q", tt.prefix, tt.session, got, tt.wantPrefix)
			}
		})
	}
}
