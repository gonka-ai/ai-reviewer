package main

import (
	"strings"
	"testing"
)

func TestGenerateAgentHandoff(t *testing.T) {
	rc := &RunConfig{
		Settings: &RunSettings{
			Repo:     "org/repo",
			Command:  "pr",
			PRNumber: "123",
		},
		PRInfo: &PRInfo{
			Title:       "Test PR",
			Body:        "Test body",
			BaseRefOid:  "base123",
			HeadRefOid:  "head123",
			BaseRefName: "main",
			HeadRefName: "feature",
		},
		RunDir:        ".ai-review/org/repo/runs/123/2026-04-20_14-22-00",
		OutputHandler: NewOutputHandler("", ""),
	}
	rr := NewRunResults()
	rr.Summary = "Summary with @persona{ReviewerA} finding"

	handoff := generateAgentHandoff(rc, rr)

	// Check for required sections
	if !strings.Contains(handoff, "You are helping a human reviewer") {
		t.Errorf("Missing instructional text")
	}
	if !strings.Contains(handoff, "## Review Target Metadata") {
		t.Errorf("Missing Metadata section")
	}
	if !strings.Contains(handoff, "- **Repository:** org/repo") {
		t.Errorf("Missing Repository")
	}
	if !strings.Contains(handoff, "- **PR Number:** 123") {
		t.Errorf("Missing PR Number")
	}
	if !strings.Contains(handoff, "- **PR Title:** Test PR") {
		t.Errorf("Missing PR Title")
	}
	if !strings.Contains(handoff, "- **Base SHA:** base123") {
		t.Errorf("Missing Base SHA")
	}
	if !strings.Contains(handoff, "- **Head SHA:** head123") {
		t.Errorf("Missing Head SHA")
	}
	if !strings.Contains(handoff, "## PR Context") {
		t.Errorf("Missing PR Context section")
	}
	if !strings.Contains(handoff, "Test body") {
		t.Errorf("Missing PR Body")
	}
	if !strings.Contains(handoff, "## Issues to Investigate First") {
		t.Errorf("Missing Issues section")
	}
	if !strings.Contains(handoff, "Summary with ReviewerA finding") {
		t.Errorf("Summary should be present and markers stripped.\nGOT:\n%s", handoff)
	}
	if strings.Contains(handoff, "@persona") {
		t.Errorf("Markers should be stripped")
	}
	if !strings.Contains(handoff, "## Full Artifact Map") {
		t.Errorf("Missing Artifact Map section")
	}
}

func TestGenerateAgentHandoffNoPRInfo(t *testing.T) {
	rc := &RunConfig{
		Settings: &RunSettings{
			Repo:       "org/repo",
			Command:    "commit",
			CommitHash: "head456",
			CompareTo:  "base456",
		},
		PRInfo:        nil,
		RunDir:        ".ai-review/org/repo/runs/head456/2026-04-20_14-22-00",
		OutputHandler: NewOutputHandler("", ""),
	}
	rr := NewRunResults()
	rr.Summary = "Aggregated summary"

	handoff := generateAgentHandoff(rc, rr)

	if !strings.Contains(handoff, "- **Base SHA:** base456") {
		t.Errorf("Missing Base SHA for commit review")
	}
	if !strings.Contains(handoff, "- **Head SHA:** head456") {
		t.Errorf("Missing Head SHA for commit review")
	}
	if strings.Contains(handoff, "## PR Context") {
		t.Errorf("PR Context should be omitted when PRInfo is nil")
	}
}
