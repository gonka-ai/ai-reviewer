package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPersonas(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "personas-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	persona1Content := `---
id: test-reviewer
ai_review: persona
role: reviewer
model_category: best_code
path_filters:
  - "*.go"
regex_filters:
  - "TODO"
---
You are a test reviewer.
`
	persona2Content := `---
id: test-explainer
ai_review: persona
role: explainer
stage: pre
model_category: balanced
---
You are a test explainer.
`

	err = os.WriteFile(filepath.Join(tmpDir, "reviewer.md"), []byte(persona1Content), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(tmpDir, "explainer.md"), []byte(persona2Content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	oh := &OutputHandler{}
	personas, err := LoadPersonas([]string{tmpDir}, "test-repo", "EMPTY", oh)
	if err != nil {
		t.Fatalf("Failed to load personas: %v", err)
	}

	if len(personas) != 2 {
		t.Fatalf("Expected 2 personas, got %d", len(personas))
	}

	var p1, p2 Persona
	for _, p := range personas {
		if p.ID == "test-reviewer" {
			p1 = p
		} else if p.ID == "test-explainer" {
			p2 = p
		}
	}

	if p1.ID != "test-reviewer" {
		t.Errorf("Expected persona 1 ID to be test-reviewer, got %s", p1.ID)
	}
	if p1.Role != "reviewer" {
		t.Errorf("Expected persona 1 role to be reviewer, got %s", p1.Role)
	}
	if len(p1.Filters.IncludeFilters) != 1 || p1.Filters.IncludeFilters[0] != "*.go" {
		t.Errorf("Expected persona 1 include filters to be ['*.go'], got %v", p1.Filters.IncludeFilters)
	}
	if len(p1.Filters.RawRegexFilters) != 1 || p1.Filters.RawRegexFilters[0] != "TODO" {
		t.Errorf("Expected persona 1 regex filters to be ['TODO'], got %v", p1.Filters.RawRegexFilters)
	}
	if p1.Instructions != "You are a test reviewer.\n" {
		t.Errorf("Expected persona 1 instructions to be 'You are a test reviewer.\n', got %q", p1.Instructions)
	}

	if p2.ID != "test-explainer" {
		t.Errorf("Expected persona 2 ID to be test-explainer, got %s", p2.ID)
	}
	if p2.Role != "explainer" {
		t.Errorf("Expected persona 2 role to be explainer, got %s", p2.Role)
	}
	if p2.Stage != "pre" {
		t.Errorf("Expected persona 2 stage to be pre, got %s", p2.Stage)
	}
	if p2.Instructions != "You are a test explainer.\n" {
		t.Errorf("Expected persona 2 instructions to be 'You are a test explainer.\n', got %q", p2.Instructions)
	}
}

func TestLoadPersonas_WithAIReviewFilter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "personas-ai-review-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// This one should be loaded because ai_review matches "persona"
	persona1Content := `---
id: p1
ai_review: persona
---
Instructions 1
`
	// This one should be loaded because it's in a dedicated directory and ai_review is missing
	persona2Content := `---
id: p2
---
Instructions 2
`
	// This one should NOT be loaded because ai_review is "something-else"
	persona3Content := `---
id: p3
ai_review: something-else
---
Instructions 3
`

	dedicatedDir := filepath.Join(tmpDir, ".ai-review", "test-repo", "personas")
	err = os.MkdirAll(dedicatedDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(dedicatedDir, "p1.md"), []byte(persona1Content), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dedicatedDir, "p2.md"), []byte(persona2Content), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dedicatedDir, "p3.md"), []byte(persona3Content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	oh := &OutputHandler{}
	personas, err := LoadPersonas([]string{tmpDir}, "test-repo", "EMPTY", oh)
	if err != nil {
		t.Fatalf("Failed to load personas: %v", err)
	}

	if len(personas) != 2 {
		t.Fatalf("Expected 2 personas (p1, p2), got %d", len(personas))
	}

	seen := make(map[string]bool)
	for _, p := range personas {
		seen[p.ID] = true
	}

	if !seen["p1"] {
		t.Errorf("Expected persona p1 to be loaded")
	}
	if !seen["p2"] {
		t.Errorf("Expected persona p2 to be loaded")
	}
	if seen["p3"] {
		t.Errorf("Persona p3 should NOT have been loaded")
	}
}

func TestScanner_SkipsIrrelevantFilesAndDirs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scanner-skip-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a "runs" directory that should be skipped
	runsDir := filepath.Join(tmpDir, "runs")
	os.MkdirAll(runsDir, 0755)
	os.WriteFile(filepath.Join(runsDir, "bad.md"), []byte("---\ninvalid: yaml\n---\n"), 0644)

	// Create a file without ai_review tag that should be skipped (not in dedicated dir)
	os.WriteFile(filepath.Join(tmpDir, "random.md"), []byte("---\ntitle: Random\n---\nHello"), 0644)

	// Create a valid persona file
	personaContent := "---\nid: valid-p\nai_review: persona\n---\nInstructions"
	os.WriteFile(filepath.Join(tmpDir, "valid.md"), []byte(personaContent), 0644)

	oh := &OutputHandler{}
	personas, err := LoadPersonas([]string{tmpDir}, "test-repo", "EMPTY", oh)
	if err != nil {
		t.Fatalf("LoadPersonas failed: %v", err)
	}

	if len(personas) != 1 {
		t.Errorf("Expected 1 persona, got %d", len(personas))
	} else if personas[0].ID != "valid-p" {
		t.Errorf("Expected persona ID 'valid-p', got %s", personas[0].ID)
	}
}
