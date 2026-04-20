package main

import (
	"testing"
)

func TestMatchPrimerForContext(t *testing.T) {
	p := Primer{
		ID:                "test-primer",
		AuthoringConcepts: []string{"auth-c1", "auth-c2"},
		Filters: FilterSet{
			IncludeFilters:  []string{"**/include-file.go"},
			FunctionFilters: []string{"includeFunc"},
		},
	}

	tests := []struct {
		name      string
		input     PlannedContext
		wantMatch bool
		wantBy    []string
	}{
		{
			name:      "no match - concept-only input does not match primer with filters",
			input:     PlannedContext{Concepts: []string{"auth-c1"}},
			wantMatch: false,
		},
		{
			name:      "match by file and function (no concepts provided)",
			input:     PlannedContext{Files: []string{"src/include-file.go"}, Functions: []string{"includeFunc"}},
			wantMatch: true,
			wantBy:    []string{"files:src/include-file.go"},
		},
		{
			name:      "no match - wrong file",
			input:     PlannedContext{Files: []string{"src/wrong-file.go"}, Functions: []string{"includeFunc"}},
			wantMatch: false,
		},
		{
			name:      "match by concept and file",
			input:     PlannedContext{Concepts: []string{"auth-c1"}, Files: []string{"src/include-file.go"}, Functions: []string{"includeFunc"}},
			wantMatch: true,
			wantBy:    []string{"files:src/include-file.go", "concept:auth-c1"},
		},
		{
			name:      "no match - concept provided but no file match",
			input:     PlannedContext{Concepts: []string{"auth-c1"}, Files: []string{"src/wrong-file.go"}},
			wantMatch: false,
		},
		{
			name:      "no match - file provided but no concept match",
			input:     PlannedContext{Concepts: []string{"wrong-concept"}, Files: []string{"src/include-file.go"}, Functions: []string{"includeFunc"}},
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMatch, gotBy := matchPrimerForContext(p, tt.input)
			if gotMatch != tt.wantMatch {
				t.Errorf("%s: matchPrimerForContext() gotMatch = %v, want %v", tt.name, gotMatch, tt.wantMatch)
			}
			if len(gotBy) != len(tt.wantBy) {
				t.Errorf("%s: matchPrimerForContext() gotBy = %v, want %v", tt.name, gotBy, tt.wantBy)
			}
			for i, v := range tt.wantBy {
				if i < len(gotBy) && gotBy[i] != v {
					t.Errorf("%s: matchPrimerForContext() gotBy[%d] = %v, want %v", tt.name, i, gotBy[i], v)
				}
			}
		})
	}
}

func TestMatchPrimerForContext_NoConcepts(t *testing.T) {
	p := Primer{
		ID: "test-primer-no-concepts",
		Filters: FilterSet{
			IncludeFilters: []string{"**/include-file.go"},
		},
	}

	tests := []struct {
		name      string
		input     PlannedContext
		wantMatch bool
		wantBy    []string
	}{
		{
			name:      "match by file even if user provided concepts",
			input:     PlannedContext{Concepts: []string{"any-concept"}, Files: []string{"src/include-file.go"}},
			wantMatch: true,
			wantBy:    []string{"files:src/include-file.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMatch, gotBy := matchPrimerForContext(p, tt.input)
			if gotMatch != tt.wantMatch {
				t.Errorf("%s: matchPrimerForContext() gotMatch = %v, want %v", tt.name, gotMatch, tt.wantMatch)
			}
			if len(gotBy) != len(tt.wantBy) {
				t.Errorf("%s: matchPrimerForContext() gotBy = %v, want %v", tt.name, gotBy, tt.wantBy)
			}
		})
	}
}

func TestMatchPrimerForContext_NoFilters(t *testing.T) {
	p := Primer{
		ID:                "test-primer-no-filters",
		AuthoringConcepts: []string{"auth-c1"},
	}

	tests := []struct {
		name      string
		input     PlannedContext
		wantMatch bool
		wantBy    []string
	}{
		{
			name:      "match by concept when no other filters exist",
			input:     PlannedContext{Concepts: []string{"auth-c1"}},
			wantMatch: true,
			wantBy:    []string{"concept:auth-c1"},
		},
		{
			name:      "no match by concept if file provided but doesn't match (though here it matches any file)",
			input:     PlannedContext{Concepts: []string{"auth-c1"}, Files: []string{"any-file.go"}},
			wantMatch: true,
			wantBy:    []string{"files:any-file.go", "concept:auth-c1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMatch, _ := matchPrimerForContext(p, tt.input)
			if gotMatch != tt.wantMatch {
				t.Errorf("%s: matchPrimerForContext() gotMatch = %v, want %v", tt.name, gotMatch, tt.wantMatch)
			}
		})
	}
}
