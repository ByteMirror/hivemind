package session

import "testing"

func TestIsClaudeProgram(t *testing.T) {
	tests := []struct {
		name    string
		program string
		want    bool
	}{
		{name: "bare command", program: "claude", want: true},
		{name: "path command", program: "/usr/local/bin/claude", want: true},
		{name: "command with args", program: "claude --model sonnet", want: true},
		{name: "path with args", program: "/opt/homebrew/bin/claude --model sonnet", want: true},
		{name: "non-claude", program: "codex --model gpt-5", want: false},
		{name: "empty", program: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClaudeProgram(tt.program); got != tt.want {
				t.Fatalf("isClaudeProgram(%q) = %v, want %v", tt.program, got, tt.want)
			}
		})
	}
}
