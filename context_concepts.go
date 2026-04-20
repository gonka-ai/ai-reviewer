package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

type ConceptSource struct {
	Kind       string   `json:"kind"`
	ID         string   `json:"id"`
	SourcePath string   `json:"source_path,omitempty"`
	MatchedBy  []string `json:"matched_by,omitempty"`
}

type Concept struct {
	Name    string          `json:"name"`
	Sources []ConceptSource `json:"sources"`
}

type ConceptsOutput struct {
	Repo      string         `json:"repo"`
	Requested PlannedContext `json:"requested"`
	Concepts  []Concept      `json:"concepts"`
}

func runContextConcepts(ctx context.Context, s *RunSettings) {
	searchPaths := GetPrimersSearchPaths(s)
	oh := NewOutputHandler("", "")

	primers, err := LoadPrimers(searchPaths, s.Repo, "", oh)
	if err != nil {
		fmt.Printf("Error loading primers: %v\n", err)
		os.Exit(1)
	}

	input := PlannedContext{
		Files:     s.PlannedFiles,
		Functions: s.PlannedFunctions,
	}

	conceptMap := make(map[string][]ConceptSource)

	for _, p := range primers {
		isMatch := true
		var matchedBy []string
		if len(input.Files) > 0 || len(input.Functions) > 0 {
			isMatch, matchedBy = matchPrimerForContext(p, input)
		}

		if isMatch {
			for _, c := range p.AuthoringConcepts {
				conceptMap[c] = append(conceptMap[c], ConceptSource{
					Kind:       "primer",
					ID:         p.ID,
					SourcePath: p.SourcePath,
					MatchedBy:  matchedBy,
				})
			}
		}
	}

	var concepts []Concept
	for name, sources := range conceptMap {
		concepts = append(concepts, Concept{
			Name:    name,
			Sources: sources,
		})
	}

	// Sort concepts by name for determinism
	slices.SortFunc(concepts, func(a, b Concept) int {
		return strings.Compare(a.Name, b.Name)
	})

	output := ConceptsOutput{
		Repo:      s.Repo,
		Requested: input,
		Concepts:  concepts,
	}

	switch s.ContextFormat {
	case "json":
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	case "names":
		for _, c := range concepts {
			fmt.Println(c.Name)
		}
	default: // markdown
		printMarkdownConcepts(output)
	}
}

func printMarkdownConcepts(out ConceptsOutput) {
	fmt.Printf("# Available Authoring Concepts for %s\n\n", out.Repo)
	if len(out.Requested.Files) > 0 || len(out.Requested.Functions) > 0 {
		fmt.Println("Scoped by:")
		if len(out.Requested.Files) > 0 {
			fmt.Printf("- Files: %s\n", strings.Join(out.Requested.Files, ", "))
		}
		if len(out.Requested.Functions) > 0 {
			fmt.Printf("- Functions: %s\n", strings.Join(out.Requested.Functions, ", "))
		}
		fmt.Println()
	}

	if len(out.Concepts) == 0 {
		fmt.Println("No authoring concepts found.")
		return
	}

	for _, c := range out.Concepts {
		var sources []string
		for _, s := range c.Sources {
			sources = append(sources, s.ID)
		}
		fmt.Printf("- **%s** (from: %s)\n", c.Name, strings.Join(sources, ", "))
	}
}
