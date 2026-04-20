package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type PlannedContext struct {
	Files     []string `json:"files"`
	Functions []string `json:"functions"`
	Concepts  []string `json:"concepts"`
}

type MatchedPrimer struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	MatchedBy  []string `json:"matched_by"`
	Content    string   `json:"content"`
	SourcePath string   `json:"source_path,omitempty"`
}

type ContextPrimersOutput struct {
	Repo      string          `json:"repo"`
	Requested PlannedContext  `json:"requested"`
	Matched   []MatchedPrimer `json:"matched"`
}

func GetPrimersSearchPaths(s *RunSettings) []string {
	searchPaths := []string{}
	searchPaths = append(searchPaths, s.ExeDir)
	searchPaths = append(searchPaths, s.InitialCwd)
	if cwd, err := os.Getwd(); err == nil {
		searchPaths = append(searchPaths, cwd)
	}
	return searchPaths
}

func runContextPrimers(ctx context.Context, s *RunSettings) {
	searchPaths := GetPrimersSearchPaths(s)
	oh := NewOutputHandler("", "") // No run/log dir needed for this command

	primers, err := LoadPrimers(searchPaths, s.Repo, "", oh)
	if err != nil {
		fmt.Printf("Error loading primers: %v\n", err)
		os.Exit(1)
	}

	input := PlannedContext{
		Files:     s.PlannedFiles,
		Functions: s.PlannedFunctions,
		Concepts:  s.PlannedConcepts,
	}

	var matched []MatchedPrimer

	for _, p := range primers {
		isMatch, matchedBy := matchPrimerForContext(p, input)
		if isMatch {
			matched = append(matched, MatchedPrimer{
				ID:         p.ID,
				Type:       p.Type,
				MatchedBy:  matchedBy,
				Content:    p.Content,
				SourcePath: p.SourcePath,
			})
		}
	}

	output := ContextPrimersOutput{
		Repo:      s.Repo,
		Requested: input,
		Matched:   matched,
	}

	if s.ContextFormat == "json" {
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	} else {
		printMarkdownPrimers(output)
	}
}

func matchPrimerForContext(p Primer, input PlannedContext) (bool, []string) {
	var matchedBy []string

	// 1. Regular filter matching
	filterMatched := false
	var filterMatchedBy []string

	if err := p.Filters.Compile(); err == nil {
		if len(input.Files) > 0 {
			var matchedFiles []string
			for _, file := range input.Files {
				if p.Filters.Matches(MatchOptions{
					Filename:  file,
					Functions: input.Functions,
				}) {
					matchedFiles = append(matchedFiles, file)
				}
			}
			if len(matchedFiles) > 0 {
				filterMatched = true
				filterMatchedBy = append(filterMatchedBy, fmt.Sprintf("files:%s", strings.Join(matchedFiles, ",")))
			}
		} else if len(input.Functions) > 0 {
			if p.Filters.Matches(MatchOptions{
				Filename:  "",
				Functions: input.Functions,
			}) {
				filterMatched = true
				filterMatchedBy = append(filterMatchedBy, "functions")
			}
		} else {
			// No files or functions provided, regular filter "matches" by default
			// so that concept-only input can match primers if they don't have other filters.
			// Actually, the requirement says "concept match alone is not enough"
			// and "regular filter match alone is not enough when the primer declares authoring_concepts and the user provided concepts".
			// Let's re-read: "file/function + concept both match and primer is returned"
			// "concept-only input does not match a primer whose normal filters do not match"
			// If no files/functions are provided, normal filters only match if they are empty.
			if p.Filters.IsEmpty() {
				filterMatched = true
			}
		}
	}

	// 2. Concept matching
	conceptMatched := true
	var conceptMatchedBy []string

	if len(p.AuthoringConcepts) > 0 && len(input.Concepts) > 0 {
		conceptMatched = false
		for _, pc := range input.Concepts {
			for _, ac := range p.AuthoringConcepts {
				if pc == ac {
					conceptMatched = true
					conceptMatchedBy = append(conceptMatchedBy, fmt.Sprintf("concept:%s", pc))
				}
			}
		}
	}

	if filterMatched && conceptMatched {
		matchedBy = append(matchedBy, filterMatchedBy...)
		matchedBy = append(matchedBy, conceptMatchedBy...)
		return true, matchedBy
	}

	return false, nil
}

func printMarkdownPrimers(out ContextPrimersOutput) {
	fmt.Printf("# Matched Primers for %s\n\n", out.Repo)
	if len(out.Matched) == 0 {
		fmt.Println("No matching primers found.")
		return
	}

	fmt.Printf("Found %d matching primers.\n\n", len(out.Matched))
	for _, m := range out.Matched {
		fmt.Printf("## Primer: %s\n", m.ID)
		fmt.Printf("- **Type:** %s\n", m.Type)
		if m.SourcePath != "" {
			fmt.Printf("- **Source:** %s\n", m.SourcePath)
		}
		fmt.Printf("- **Matched by:** %s\n\n", strings.Join(m.MatchedBy, ", "))
		fmt.Println("### Content")
		fmt.Println(m.Content)
		fmt.Println("\n---")
	}
}
