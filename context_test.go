package main

import (
	"regexp"
	"testing"
	"time"
)

func TestFilterSet_Compile(t *testing.T) {
	fs := &FilterSet{
		RawRegexFilters: []string{"^test.*"},
		IssueRegexes:    []string{"issue.*"},
	}

	err := fs.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(fs.RegexFilters) != 1 {
		t.Errorf("Expected 1 RegexFilter, got %d", len(fs.RegexFilters))
	}
	if len(fs.IssueRegexObjects) != 1 {
		t.Errorf("Expected 1 IssueRegexObject, got %d", len(fs.IssueRegexObjects))
	}

	if !fs.RegexFilters[0].MatchString("test123") {
		t.Error("RegexFilter didn't match")
	}
	if !fs.IssueRegexObjects[0].MatchString("issue456") {
		t.Error("IssueRegexObject didn't match")
	}
}

func TestFilterSet_MatchesPath(t *testing.T) {
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
		{
			name:     "very wild card",
			file:     "toplevel/subnet/file.go",
			includes: []string{"**subnet**"},
			excludes: nil,
			want:     true,
		},
		{
			name:     "very wild card 2",
			file:     "toplevel/dir/msg_subnet_something.go",
			includes: []string{"**subnet**"},
			excludes: nil,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &FilterSet{
				IncludeFilters: tt.includes,
				ExcludeFilters: tt.excludes,
			}
			got := fs.MatchesPath(tt.file)
			if got != tt.want {
				t.Errorf("FilterSet.MatchesPath(%q, %v, %v) = %v, want %v", tt.file, tt.includes, tt.excludes, got, tt.want)
			}
		})
	}
}

func TestFileContext_Matches(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		changedLines []string
		includes     []string
		excludes     []string
		regexes      []string
		want         bool
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
			name:         "Match regex in changed lines (added)",
			file:         "main.go",
			changedLines: []string{"TODO: fix this"},
			includes:     []string{"*.go"},
			excludes:     nil,
			regexes:      []string{"TODO"},
			want:         true,
		},
		{
			name:         "Match regex in changed lines (removed)",
			file:         "main.go",
			changedLines: []string{"FIXME: removed"},
			includes:     []string{"*.go"},
			excludes:     nil,
			regexes:      []string{"FIXME"},
			want:         true,
		},
		{
			name:         "Regex doesn't match changed lines",
			file:         "main.go",
			changedLines: []string{"fmt.Println(\"hello\")"},
			includes:     []string{"*.go"},
			excludes:     nil,
			regexes:      []string{"TODO"},
			want:         false,
		},
		{
			name:         "Excluded file ignores regex match",
			file:         "test.go",
			changedLines: []string{"TODO: fix this"},
			includes:     []string{"*.go"},
			excludes:     []string{"test.go"},
			regexes:      []string{"TODO"},
			want:         false,
		},
		{
			name:         "Match include, no regexes",
			file:         "main.go",
			changedLines: []string{"something"},
			includes:     []string{"*.go"},
			excludes:     nil,
			regexes:      nil,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var compiledRegexes []*regexp.Regexp
			for _, r := range tt.regexes {
				compiledRegexes = append(compiledRegexes, regexp.MustCompile(r))
			}
			fc := FileContext{Filename: tt.file, ChangedLines: tt.changedLines}
			fs := &FilterSet{
				IncludeFilters: tt.includes,
				ExcludeFilters: tt.excludes,
			}
			for _, r := range tt.regexes {
				fs.RegexFilters = append(fs.RegexFilters, regexp.MustCompile(r))
			}
			got := fc.Matches(FileMatchOptions{
				FilterSet: fs,
			})
			if got != tt.want {
				t.Errorf("FileContext(%q).Matches(%v, %v, %v) = %v, want %v", tt.file, tt.includes, tt.excludes, tt.regexes, got, tt.want)
			}
		})
	}
}

func TestFileContext_MatchesLineNumberFilters(t *testing.T) {
	fc := FileContext{
		Filename: "main.go",
		Diff:     "+++ b/main.go\n@@ -1,2 +10,3 @@\n10:+first\n11: context\n12:-old\n12:+new\n",
	}

	if !fc.Matches(FileMatchOptions{FilterSet: &FilterSet{LineNumberFilters: []LineRange{{Start: 10, End: 10}}}}) {
		t.Fatalf("expected line range 10-10 to match changed lines")
	}
	if !fc.Matches(FileMatchOptions{FilterSet: &FilterSet{LineNumberFilters: []LineRange{{Start: 12, End: 12}}}}) {
		t.Fatalf("expected line range 12-12 to match changed lines")
	}
	if fc.Matches(FileMatchOptions{FilterSet: &FilterSet{LineNumberFilters: []LineRange{{Start: 20, End: 25}}}}) {
		t.Fatalf("expected line range 20-25 not to match changed lines")
	}
}

func TestFilterSet_Matches(t *testing.T) {
	tests := []struct {
		name string
		fs   FilterSet
		opts MatchOptions
		want bool
	}{
		{
			name: "Include path filter match",
			fs:   FilterSet{IncludeFilters: []string{"*.go"}},
			opts: MatchOptions{Filename: "main.go"},
			want: true,
		},
		{
			name: "Exclude path filter match",
			fs:   FilterSet{ExcludeFilters: []string{"test.go"}},
			opts: MatchOptions{Filename: "test.go"},
			want: false,
		},
		{
			name: "Branch filter match",
			fs:   FilterSet{BranchFilters: []string{"main"}},
			opts: MatchOptions{Branch: "main", Filename: "main.go"},
			want: true,
		},
		{
			name: "Branch filter no match",
			fs:   FilterSet{BranchFilters: []string{"main"}},
			opts: MatchOptions{Branch: "feature", Filename: "main.go"},
			want: false,
		},
		{
			name: "Function filter match",
			fs:   FilterSet{FunctionFilters: []string{"MainFunc"}},
			opts: MatchOptions{Functions: []string{"HelperFunc", "MainFunc"}, Filename: "main.go"},
			want: true,
		},
		{
			name: "Function filter no match",
			fs:   FilterSet{FunctionFilters: []string{"OtherFunc"}},
			opts: MatchOptions{Functions: []string{"HelperFunc", "MainFunc"}, Filename: "main.go"},
			want: false,
		},
		{
			name: "Date filter match (before cutoff)",
			fs:   FilterSet{DateFilter: "2023-01-01"},
			opts: MatchOptions{CommitDate: time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC), Filename: "main.go"},
			want: true,
		},
		{
			name: "Date filter no match (after cutoff)",
			fs:   FilterSet{DateFilter: "2023-01-01"},
			opts: MatchOptions{CommitDate: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC), Filename: "main.go"},
			want: false,
		},
		{
			name: "Line number filter match",
			fs:   FilterSet{LineNumberFilters: []LineRange{{Start: 10, End: 20}}},
			opts: MatchOptions{ChangedLineNumbers: []int{15}, Filename: "main.go"},
			want: true,
		},
		{
			name: "Line number filter no match",
			fs:   FilterSet{LineNumberFilters: []LineRange{{Start: 10, End: 20}}},
			opts: MatchOptions{ChangedLineNumbers: []int{25}, Filename: "main.go"},
			want: false,
		},
		{
			name: "Regex filter match",
			fs:   FilterSet{RegexFilters: []*regexp.Regexp{regexp.MustCompile("TODO")}},
			opts: MatchOptions{ChangedLines: []string{"// TODO: fix this"}, Filename: "main.go"},
			want: true,
		},
		{
			name: "Regex filter no match",
			fs:   FilterSet{RegexFilters: []*regexp.Regexp{regexp.MustCompile("TODO")}},
			opts: MatchOptions{ChangedLines: []string{"fmt.Println(\"hello\")"}, Filename: "main.go"},
			want: false,
		},
		{
			name: "Multiple filters match (Path and Branch)",
			fs:   FilterSet{IncludeFilters: []string{"*.go"}, BranchFilters: []string{"main"}},
			opts: MatchOptions{Filename: "main.go", Branch: "main"},
			want: true,
		},
		{
			name: "Multiple filters (Path match, Branch mismatch)",
			fs:   FilterSet{IncludeFilters: []string{"*.go"}, BranchFilters: []string{"main"}},
			opts: MatchOptions{Filename: "main.go", Branch: "feature"},
			want: false,
		},
		{
			name: "Issue regex match (Summary)",
			fs: FilterSet{
				IssueRegexes: []string{"leak"},
			},
			opts: MatchOptions{FindingSummary: "Goroutine leak detected"},
			want: true,
		},
		{
			name: "Issue regex match (Details)",
			fs: FilterSet{
				IssueRegexes: []string{"leak"},
			},
			opts: MatchOptions{FindingDetails: "Wait, there's a memory leak here."},
			want: true,
		},
		{
			name: "Issue regex no match",
			fs: FilterSet{
				IssueRegexes: []string{"leak"},
			},
			opts: MatchOptions{FindingSummary: "Fixed bug"},
			want: false,
		},
		{
			name: "Any filter with issue regex",
			fs: FilterSet{
				Any: []FilterSet{
					{
						IncludeFilters: []string{"*.go"},
						IssueRegexes:   []string{"GoLeak"},
					},
					{
						IncludeFilters: []string{"*.js"},
						IssueRegexes:   []string{"JsLeak"},
					},
				},
			},
			opts: MatchOptions{
				Filename:       "test.go",
				FindingSummary: "GoLeak found",
			},
			want: true,
		},
		{
			name: "Any filter with issue regex (no match)",
			fs: FilterSet{
				Any: []FilterSet{
					{
						IncludeFilters: []string{"*.go"},
						IssueRegexes:   []string{"GoLeak"},
					},
					{
						IncludeFilters: []string{"*.js"},
						IssueRegexes:   []string{"JsLeak"},
					},
				},
			},
			opts: MatchOptions{
				Filename:       "test.js",
				FindingSummary: "GoLeak found",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.fs.Compile()
			if got := tt.fs.Matches(tt.opts); got != tt.want {
				t.Errorf("%s: FilterSet.Matches() = %v, want %v", tt.name, got, tt.want)
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
		{
			name:  "Recursive substring match with **",
			path:  "toplevel/subnet/file.go",
			globs: []string{"**subnet**"},
			want:  true,
		},
		{
			name:  "Glob without slashes matches anywhere",
			path:  "toplevel/dir/file.go",
			globs: []string{"dir"},
			want:  true,
		},
		{
			name:  "Wildcard without slashes matches anywhere",
			path:  "toplevel/dir/file.go",
			globs: []string{"*.go"},
			want:  true,
		},
		{
			name:  "Directory match with trailing slash",
			path:  "toplevel/dir/file.go",
			globs: []string{"toplevel/dir/"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathIncluded(tt.path, tt.globs, true)
			if got != tt.want {
				t.Errorf("pathIncluded(%q, %v) = %v, want %v", tt.path, tt.globs, got, tt.want)
			}
		})
	}
}

func TestFileContext_HasChangedLinesInRanges(t *testing.T) {
	fc := FileContext{
		Diff: "10:+first\n11: context\n12:-old\n12:+new\n",
	}

	tests := []struct {
		name   string
		ranges []LineRange
		want   bool
	}{
		{
			name:   "Empty ranges match all",
			ranges: nil,
			want:   true,
		},
		{
			name:   "Match start range",
			ranges: []LineRange{{Start: 10, End: 10}},
			want:   true,
		},
		{
			name:   "Match end range",
			ranges: []LineRange{{Start: 12, End: 12}},
			want:   true,
		},
		{
			name:   "No match",
			ranges: []LineRange{{Start: 20, End: 30}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fc.HasChangedLinesInRanges(tt.ranges); got != tt.want {
				t.Errorf("HasChangedLinesInRanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileContext_ChangedLineNumbers(t *testing.T) {
	fc := FileContext{
		Diff: "10:+first\n11: context\n12:-old\n12:+new\n13: no change\n",
	}

	got := fc.ChangedLineNumbers()
	want := []int{10, 12}

	if len(got) != len(want) {
		t.Errorf("ChangedLineNumbers() returned %d lines, want %d", len(got), len(want))
	}

	for i, v := range got {
		if v != want[i] {
			t.Errorf("ChangedLineNumbers()[%d] = %d, want %d", i, v, want[i])
		}
	}
}

func TestParseAnnotatedFileDiff(t *testing.T) {
	diff := `+++ b/main.go
10:+func main() {
11: 	fmt.Println("hello")
12:-	// old comment
12:+	// new comment
13: }`

	fc := ParseAnnotatedFileDiff(diff)

	if fc.Filename != "main.go" {
		t.Errorf("ParseAnnotatedFileDiff() Filename = %q, want %q", fc.Filename, "main.go")
	}

	if len(fc.ChangedLines) != 3 {
		t.Fatalf("ParseAnnotatedFileDiff() ChangedLines length = %d, want 3", len(fc.ChangedLines))
	}

	// Order in ParseAnnotatedFileDiff:
	// 10:+ -> appends "func main() {"
	// 12:- -> appends "	// old comment" (wait, does it include whitespace?)
	// 12:+ -> appends "	// new comment"

	// Looking at code: content := diffLine[1:]

	expected := []string{"func main() {", "\t// old comment", "\t// new comment"}
	for i, v := range fc.ChangedLines {
		if v != expected[i] {
			t.Errorf("ChangedLines[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestPromptBuilder_Build(t *testing.T) {
	p := Persona{
		ID:           "test-persona",
		Instructions: "Test instructions",
	}
	ctx := &PRContext{
		Title:       "Test PR",
		Description: "Test description",
		Files: []FileContext{
			{
				Filename: "main.go",
				Diff:     "+++ b/main.go\n10:+func main() {}\n",
			},
		},
	}
	pb := &PromptBuilder{
		Persona:            p,
		PRContext:          ctx,
		GlobalInstructions: "Global instructions",
	}

	prompt, breakdown := pb.Build()

	if breakdown.TotalChars != len(prompt) {
		t.Errorf("breakdown.TotalChars = %d, want %d", breakdown.TotalChars, len(prompt))
	}

	foundInstructions := false
	foundGlobalInstructions := false
	foundMetadata := false
	foundFileList := false
	foundDiff := false

	for _, entry := range breakdown.Entries {
		switch entry.Category {
		case "instructions":
			foundInstructions = true
		case "global_instructions":
			foundGlobalInstructions = true
		case "metadata":
			foundMetadata = true
		case "file_list":
			foundFileList = true
		case "diff":
			foundDiff = true
		}
	}

	if !foundInstructions {
		t.Errorf("Category 'instructions' not found in breakdown")
	}
	if !foundGlobalInstructions {
		t.Errorf("Category 'global_instructions' not found in breakdown")
	}
	if !foundMetadata {
		t.Errorf("Category 'metadata' not found in breakdown")
	}
	if !foundFileList {
		t.Errorf("Category 'file_list' not found in breakdown")
	}
	if !foundDiff {
		t.Errorf("Category 'diff' not found in breakdown")
	}

	// Verify diff subcategory is filename
	for _, entry := range breakdown.Entries {
		if entry.Category == "diff" {
			if entry.Subcategory != "main.go" {
				t.Errorf("diff subcategory = %q, want %q", entry.Subcategory, "main.go")
			}
		}
	}
}
