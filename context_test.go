package main

import (
	"regexp"
	"testing"
)

func TestMatchesFilters(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		includes []string
		excludes []string
		want     bool
	}{
		{
			name:     "No filters",
			file:     "main.go",
			includes: nil,
			excludes: nil,
			want:     true,
		},
		{
			name:     "Match include, no excludes",
			file:     "main.go",
			includes: []string{"*.go"},
			excludes: nil,
			want:     true,
		},
		{
			name:     "No match include, no excludes",
			file:     "README.md",
			includes: []string{"*.go"},
			excludes: nil,
			want:     false,
		},
		{
			name:     "Match include, no match exclude",
			file:     "main.go",
			includes: []string{"*.go"},
			excludes: []string{"test.go"},
			want:     true,
		},
		{
			name:     "Match include, match exclude",
			file:     "test.go",
			includes: []string{"*.go"},
			excludes: []string{"test.go"},
			want:     false,
		},
		{
			name:     "Substring match in include",
			file:     "pkg/utils/helper.go",
			includes: []string{"utils"},
			excludes: nil,
			want:     true,
		},
		{
			name:     "Substring match in exclude",
			file:     "pkg/utils/helper.go",
			includes: []string{"pkg"},
			excludes: []string{"utils"},
			want:     false,
		},
		{
			name:     "Empty includes matches everything",
			file:     "anyfile.txt",
			includes: []string{},
			excludes: nil,
			want:     true,
		},
		{
			name:     "Excludes only",
			file:     "main.go",
			includes: nil,
			excludes: []string{"*.go"},
			want:     false,
		},
		{
			name:     "Excludes only, no match",
			file:     "README.md",
			includes: nil,
			excludes: []string{"*.go"},
			want:     true,
		},
		{
			name:     "Wildcard match in subdirectory (no match because pattern is not anchored and not recursive)",
			file:     "pkg/utils/helper.go",
			includes: []string{"utils/*.go"},
			excludes: nil,
			want:     false,
		},
		{
			name:     "Directory match in subdirectory (passes via strings.Contains)",
			file:     "pkg/utils/helper.go",
			includes: []string{"utils/"},
			excludes: nil,
			want:     true,
		},
		{
			name:     "Wildcard directories match",
			file:     "pks/utils/helper.go",
			includes: []string{"pks/**/*.*"},
			excludes: nil,
			want:     true,
		},
		{
			name:     "Deep wildcard directories match",
			file:     "pkg/utils/subdir/more/helper.go",
			includes: []string{"pkg/**/*.go"},
			excludes: nil,
			want:     true,
		},
		{
			name:     "Wildcard directories match, no match",
			file:     "pks/utils/helper.go",
			includes: []string{"pks/**/*.py"},
			excludes: nil,
			want:     false,
		},
		{
			name:     "Directories with ./",
			file:     "pks/utils/helper.go",
			includes: []string{"./pks/**/"},
			excludes: nil,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesFilters(tt.file, tt.includes, tt.excludes)
			if got != tt.want {
				t.Errorf("MatchesFilters(%q, %v, %v) = %v, want %v", tt.file, tt.includes, tt.excludes, got, tt.want)
			}
		})
	}
}

func TestFileContext_Matches(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		addedLines []string
		includes   []string
		excludes   []string
		regexes    []string
		want       bool
	}{
		{
			name:     "No filters",
			file:     "main.go",
			includes: nil,
			excludes: nil,
			regexes:  nil,
			want:     true,
		},
		{
			name:       "Match regex in added lines",
			file:       "main.go",
			addedLines: []string{"TODO: fix this"},
			includes:   []string{"*.go"},
			excludes:   nil,
			regexes:    []string{"TODO"},
			want:       true,
		},
		{
			name:       "Regex doesn't match added lines",
			file:       "main.go",
			addedLines: []string{"fmt.Println(\"hello\")"},
			includes:   []string{"*.go"},
			excludes:   nil,
			regexes:    []string{"TODO"},
			want:       false,
		},
		{
			name:       "Excluded file ignores regex match",
			file:       "test.go",
			addedLines: []string{"TODO: fix this"},
			includes:   []string{"*.go"},
			excludes:   []string{"test.go"},
			regexes:    []string{"TODO"},
			want:       false,
		},
		{
			name:       "Match include, no regexes",
			file:       "main.go",
			addedLines: []string{"something"},
			includes:   []string{"*.go"},
			excludes:   nil,
			regexes:    nil,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var compiledRegexes []*regexp.Regexp
			for _, r := range tt.regexes {
				compiledRegexes = append(compiledRegexes, regexp.MustCompile(r))
			}
			fc := FileContext{Filename: tt.file, AddedLines: tt.addedLines}
			got := fc.Matches(tt.includes, tt.excludes, compiledRegexes)
			if got != tt.want {
				t.Errorf("FileContext(%q).Matches(%v, %v, %v) = %v, want %v", tt.file, tt.includes, tt.excludes, tt.regexes, got, tt.want)
			}
		})
	}
}

func TestPathIncluded(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		globs []string
		want  bool
	}{
		{
			name:  "Empty globs",
			path:  "main.go",
			globs: nil,
			want:  true,
		},
		{
			name:  "Direct match",
			path:  "main.go",
			globs: []string{"main.go"},
			want:  true,
		},
		{
			name:  "Wildcard match",
			path:  "main.go",
			globs: []string{"*.go"},
			want:  true,
		},
		{
			name:  "No match",
			path:  "main.go",
			globs: []string{"*.md"},
			want:  false,
		},
		{
			name:  "Substring match",
			path:  "pkg/utils/helper.go",
			globs: []string{"utils"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathIncluded(tt.path, tt.globs)
			if got != tt.want {
				t.Errorf("pathIncluded(%q, %v) = %v, want %v", tt.path, tt.globs, got, tt.want)
			}
		})
	}
}
