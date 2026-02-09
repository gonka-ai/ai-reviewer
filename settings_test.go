package main

import (
	"testing"
)

func TestFlagParsing(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantCommand   string
		wantRepo      string
		wantPR        string
		wantDryRun    bool
		wantMaxTokens int
	}{
		{
			name:        "PR with flags at the end",
			args:        []string{"ai-reviewer", "pr", "owner/repo", "123", "--dry-run"},
			wantCommand: "pr",
			wantRepo:    "owner/repo",
			wantPR:      "123",
			wantDryRun:  true,
		},
		{
			name:        "Commit with flags in middle",
			args:        []string{"ai-reviewer", "commit", "--dry-run", "owner/repo", "abc1234"},
			wantCommand: "commit",
			wantRepo:    "owner/repo",
			wantDryRun:  true,
		},
		{
			name:          "File with multiple patterns and flags",
			args:          []string{"ai-reviewer", "file", "owner/repo", "main", "path/to/file", "--max-tokens", "1000"},
			wantCommand:   "file",
			wantRepo:      "owner/repo",
			wantMaxTokens: 1000,
		},
		{
			name:        "Flags before command",
			args:        []string{"ai-reviewer", "--dry-run", "pr", "owner/repo", "123"},
			wantCommand: "pr",
			wantRepo:    "owner/repo",
			wantPR:      "123",
			wantDryRun:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRunSettingsFromArgs(tt.args)

			if s.Command != tt.wantCommand {
				t.Errorf("Command = %v, want %v", s.Command, tt.wantCommand)
			}
			if s.Repo != tt.wantRepo {
				t.Errorf("Repo = %v, want %v", s.Repo, tt.wantRepo)
			}
			if s.DryRun != tt.wantDryRun {
				t.Errorf("DryRun = %v, want %v", s.DryRun, tt.wantDryRun)
			}
			if tt.wantMaxTokens != 0 && s.MaxTokens != tt.wantMaxTokens {
				t.Errorf("MaxTokens = %v, want %v", s.MaxTokens, tt.wantMaxTokens)
			}
		})
	}
}
