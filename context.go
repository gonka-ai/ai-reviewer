package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/karagenc/go-pathspec"
)

type PRInfo struct {
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	BaseRefName  string    `json:"baseRefName"`
	BaseRefOid   string    `json:"baseRefOid"`
	HeadRefName  string    `json:"headRefName"`
	HeadRefOid   string    `json:"headRefOid"`
	IsCommit     bool      `json:"isCommit"`
	CommitDate   time.Time `json:"commitDate"`
	FilePatterns []string  `json:"filePatterns"`
}

type PRContext struct {
	Title       string
	Description string
	Files       []FileContext
	Branch      string
	CommitDate  time.Time
}

type FileContext struct {
	Filename     string
	Diff         string   // Annotated diff for this file
	ChangedLines []string // Content of both added and removed lines
	Functions    []string
}

type FilterSet struct {
	IncludeFilters    []string         `yaml:"path_filters"`
	ExcludeFilters    []string         `yaml:"exclude_filters"`
	RegexFilters      []*regexp.Regexp `yaml:"-"`
	RawRegexFilters   []string         `yaml:"regex_filters"`
	BranchFilters     []string         `yaml:"branch_filters"`
	FunctionFilters   []string         `yaml:"function_filters"`
	LineNumberFilters []LineRange      `yaml:"line_numbers_filter"`
	DateFilter        string           `yaml:"date_filter"`
}

type MatchOptions struct {
	Filename           string
	Branch             string
	Functions          []string
	CommitDate         time.Time
	ChangedLineNumbers []int
	ChangedLines       []string
}

func (fs *FilterSet) Matches(opts MatchOptions) bool {
	if !fs.MatchesPath(opts.Filename) {
		return false
	}

	if len(fs.BranchFilters) > 0 {
		if !pathIncluded(opts.Branch, fs.BranchFilters) {
			return false
		}
	}

	if len(fs.FunctionFilters) > 0 {
		matched := false
	loop:
		for _, ff := range fs.FunctionFilters {
			for _, fn := range opts.Functions {
				if fn == ff {
					matched = true
					break loop
				}
			}
		}
		if !matched {
			return false
		}
	}

	if fs.DateFilter != "" && !opts.CommitDate.IsZero() {
		cutoff, err := time.Parse("2006-01-02", fs.DateFilter)
		if err == nil {
			if !opts.CommitDate.Before(cutoff) {
				return false
			}
		}
	}

	if len(fs.LineNumberFilters) > 0 {
		matched := false
		for _, line := range opts.ChangedLineNumbers {
			for _, r := range fs.LineNumberFilters {
				if line >= r.Start && line <= r.End {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(fs.RegexFilters) == 0 {
		return true
	}
	for _, line := range opts.ChangedLines {
		for _, re := range fs.RegexFilters {
			if re.MatchString(line) {
				return true
			}
		}
	}
	return false
}

type FileMatchOptions struct {
	FilterSet  *FilterSet
	Branch     string
	CommitDate time.Time
}

func (f FileContext) Matches(opts FileMatchOptions) bool {
	if opts.FilterSet == nil {
		return true
	}
	return opts.FilterSet.Matches(MatchOptions{
		Filename:           f.Filename,
		Branch:             opts.Branch,
		Functions:          f.Functions,
		CommitDate:         opts.CommitDate,
		ChangedLineNumbers: f.ChangedLineNumbers(),
		ChangedLines:       f.ChangedLines,
	})
}

func (f FileContext) HasChangedLinesInRanges(ranges []LineRange) bool {
	if len(ranges) == 0 {
		return true
	}

	for _, line := range f.ChangedLineNumbers() {
		for _, r := range ranges {
			if line >= r.Start && line <= r.End {
				return true
			}
		}
	}

	return false
}

func (f FileContext) ChangedLineNumbers() []int {
	seen := make(map[int]struct{})
	var lines []int

	for _, line := range strings.Split(f.Diff, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		lineNo, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		diffLine := parts[1]
		if !strings.HasPrefix(diffLine, "+") && !strings.HasPrefix(diffLine, "-") {
			continue
		}
		if strings.HasPrefix(diffLine, "+++ ") || strings.HasPrefix(diffLine, "--- ") {
			continue
		}
		if _, ok := seen[lineNo]; ok {
			continue
		}

		seen[lineNo] = struct{}{}
		lines = append(lines, lineNo)
	}

	return lines
}

func (ctx *PRContext) ChangedFiles() []string {
	var files []string
	for _, f := range ctx.Files {
		files = append(files, f.Filename)
	}
	return files
}

func (ctx *PRContext) FullDiff() string {
	var sb strings.Builder
	for _, f := range ctx.Files {
		sb.WriteString(f.Diff)
	}
	return sb.String()
}

func ParseAnnotatedFileDiff(fd string) FileContext {
	lines := strings.Split(fd, "\n")
	var filename string
	var changedLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ b/") {
			filename = strings.TrimPrefix(line, "+++ b/")
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		diffLine := parts[1]

		if (strings.HasPrefix(diffLine, "+") && !strings.HasPrefix(diffLine, "+++ ")) ||
			(strings.HasPrefix(diffLine, "-") && !strings.HasPrefix(diffLine, "--- ")) {
			content := diffLine[1:]
			changedLines = append(changedLines, content)
		}
	}
	return FileContext{Filename: filename, Diff: fd, ChangedLines: changedLines}
}

func GetPRInfo(repo, prNumber string) (*PRInfo, error) {
	fmt.Printf("    -> Running gh pr view %s...\n", prNumber)
	cmd := exec.Command("gh", "pr", "view", prNumber, "-R", repo, "--json", "title,body,baseRefName,baseRefOid,headRefName,headRefOid")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error running gh pr view: %w, output: %s", err, string(output))
	}

	var pr PRInfo
	if err := json.Unmarshal(output, &pr); err != nil {
		return nil, fmt.Errorf("error unmarshaling gh output: %w", err)
	}

	// Fetch commit date for the head of the PR
	cmd = exec.Command("git", "show", "-s", "--format=%cI", pr.HeadRefOid)
	dateOutput, err := cmd.Output()
	if err == nil {
		dateStr := strings.TrimSpace(string(dateOutput))
		if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			pr.CommitDate = t
		}
	}

	return &pr, nil
}

func GetCommitInfo(commitHash, compareTo string) (*PRInfo, error) {
	fmt.Printf("    -> Getting info for commit %s...\n", commitHash)

	// Get commit message and date
	cmd := exec.Command("git", "show", "-s", "--format=%s%n%n%b%n--DATE--%n%cI", commitHash)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error getting commit info: %w, output: %s", err, string(output))
	}

	fullContent := string(output)
	parts := strings.Split(fullContent, "\n--DATE--\n")
	msgPart := strings.TrimSpace(parts[0])
	dateStr := ""
	if len(parts) > 1 {
		dateStr = strings.TrimSpace(parts[1])
	}

	msgLines := strings.SplitN(msgPart, "\n", 2)
	title := msgLines[0]
	body := ""
	if len(msgLines) > 1 {
		body = strings.TrimSpace(msgLines[1])
	}

	var commitDate time.Time
	if dateStr != "" {
		if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			commitDate = t
		}
	}

	// Get full SHA for head
	cmd = exec.Command("git", "rev-parse", commitHash)
	headSHA, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error resolving commit hash: %w", err)
	}

	baseSHA := compareTo
	if baseSHA == "" {
		// Default to parent commit
		cmd = exec.Command("git", "rev-parse", commitHash+"^")
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("error getting parent commit: %w", err)
		}
		baseSHA = strings.TrimSpace(string(out))
	} else {
		// Resolve compareTo to full SHA
		cmd = exec.Command("git", "rev-parse", compareTo)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("error resolving comparison commit: %w", err)
		}
		baseSHA = strings.TrimSpace(string(out))
	}

	return &PRInfo{
		Title:       title,
		Body:        body,
		BaseRefOid:  baseSHA,
		HeadRefOid:  strings.TrimSpace(string(headSHA)),
		IsCommit:    true,
		CommitDate:  commitDate,
		BaseRefName: baseSHA[:8], // Short SHA for display
		HeadRefName: strings.TrimSpace(string(headSHA))[:8],
	}, nil
}

func GetFileInfo(repo, branch string, filePatterns []string) (*PRInfo, error) {
	fmt.Printf("    -> Getting info for branch %s, files %v...\n", branch, filePatterns)

	// Get head SHA of branch
	cmd := exec.Command("git", "rev-parse", branch)
	headSHAOut, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error resolving branch %s: %w", branch, err)
	}
	headSHA := strings.TrimSpace(string(headSHAOut))

	// Get commit date
	cmd = exec.Command("git", "show", "-s", "--format=%cI", headSHA)
	dateOutput, err := cmd.Output()
	var commitDate time.Time
	if err == nil {
		dateStr := strings.TrimSpace(string(dateOutput))
		if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			commitDate = t
		}
	}

	// Resolve patterns to actual files
	fs := &FilterSet{IncludeFilters: filePatterns}
	files, err := fs.GetFilesForPatterns(branch)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found matching patterns %v on branch %s", filePatterns, branch)
	}

	return &PRInfo{
		Title:        fmt.Sprintf("Review of %d files on %s", len(files), branch),
		Body:         fmt.Sprintf("Reviewing files: %s", strings.Join(files, ", ")),
		BaseRefOid:   headSHA, // We'll use this as a hack for GetPRContext
		HeadRefOid:   headSHA,
		IsCommit:     false,
		CommitDate:   commitDate,
		BaseRefName:  branch,
		HeadRefName:  branch,
		FilePatterns: filePatterns,
	}, nil
}

func GetBranchesInfo(repo, base, head string) (*PRInfo, error) {
	fmt.Printf("    -> Getting comparison info for branches %s...%s...\n", base, head)

	resolveRef := func(ref string) (string, error) {
		// Try resolving as is
		cmd := exec.Command("git", "rev-parse", ref)
		out, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}

		// Try resolving as origin/ref
		cmd = exec.Command("git", "rev-parse", "origin/"+ref)
		out, err = cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}

		// Try resolving as FETCH_HEAD if it was just fetched
		// But we have two refs, so FETCH_HEAD is not reliable.

		return "", fmt.Errorf("error resolving ref %s: %w", ref, err)
	}

	// Get head SHA of base
	baseSHA, err := resolveRef(base)
	if err != nil {
		return nil, fmt.Errorf("error resolving base branch %s: %w", base, err)
	}

	// Get head SHA of head
	headSHA, err := resolveRef(head)
	if err != nil {
		return nil, fmt.Errorf("error resolving head branch %s: %w", head, err)
	}

	// Get commit date of head
	cmd := exec.Command("git", "show", "-s", "--format=%cI", headSHA)
	dateOutput, err := cmd.Output()
	var commitDate time.Time
	if err == nil {
		dateStr := strings.TrimSpace(string(dateOutput))
		if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			commitDate = t
		}
	}

	return &PRInfo{
		Title:       fmt.Sprintf("Review comparison %s...%s", base, head),
		Body:        fmt.Sprintf("Comparing branch %s (base) with %s (head)", base, head),
		BaseRefOid:  baseSHA,
		HeadRefOid:  headSHA,
		IsCommit:    false,
		CommitDate:  commitDate,
		BaseRefName: base,
		HeadRefName: head,
	}, nil
}

func GetPRContext(prInfo *PRInfo, fs *FilterSet) (*PRContext, error) {
	if fs != nil {
		for _, r := range fs.RawRegexFilters {
			re, err := regexp.Compile(r)
			if err != nil {
				return nil, fmt.Errorf("invalid regex %s: %w", r, err)
			}
			fs.RegexFilters = append(fs.RegexFilters, re)
		}
	}

	if prInfo.BaseRefOid == prInfo.HeadRefOid && !prInfo.IsCommit && prInfo.BaseRefOid != "" {
		// This is "file" mode, we want to see the whole content of the files as if it were a new file
		fsToUse := fs
		if fsToUse == nil {
			fsToUse = &FilterSet{}
		}
		files, err := fsToUse.GetFilesForPatterns(prInfo.HeadRefOid)
		if err != nil {
			return nil, err
		}

		var finalFiles []FileContext
		for _, file := range files {
			// Get content of the file at HeadRefOid
			cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", prInfo.HeadRefOid, file))
			content, err := cmd.Output()
			if err != nil {
				// Maybe it's a directory or doesn't exist anymore
				continue
			}

			var diffBuilder strings.Builder
			diffBuilder.WriteString(fmt.Sprintf("+++ b/%s\n", file))
			contentStr := string(content)
			lines := strings.Split(contentStr, "\n")
			// Fake a diff chunk
			diffBuilder.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))
			for _, line := range lines {
				diffBuilder.WriteString("+" + line + "\n")
			}

			annDiff, funcs := AnnotateDiff(diffBuilder.String())
			fileCtx := FileContext{
				Filename:     file,
				Diff:         annDiff,
				ChangedLines: lines,
				Functions:    funcs,
			}

			if fileCtx.Matches(FileMatchOptions{
				FilterSet:  fs,
				Branch:     prInfo.HeadRefName,
				CommitDate: prInfo.CommitDate,
			}) {
				finalFiles = append(finalFiles, fileCtx)
			}
		}

		return &PRContext{
			Title:       prInfo.Title,
			Description: prInfo.Body,
			Files:       finalFiles,
			Branch:      prInfo.HeadRefName,
			CommitDate:  prInfo.CommitDate,
		}, nil
	}

	fsToUse := fs
	if fsToUse == nil {
		fsToUse = &FilterSet{}
	}
	diff, err := fsToUse.GetDiff(prInfo.BaseRefOid, prInfo.HeadRefOid)
	if err != nil {
		return nil, err
	}

	var finalFiles []FileContext

	// Split diff into files
	// Git diff output starts with "diff --git" for each file
	fileDiffs := strings.Split(diff, "diff --git ")
	for i, fd := range fileDiffs {
		if i == 0 && !strings.HasPrefix(fd, "diff --git ") && fd != "" {
			// Header before first file diff if any
			continue
		}
		if fd == "" {
			continue
		}

		annDiff, funcs := AnnotateDiff("diff --git " + fd)
		fileCtx := ParseAnnotatedFileDiff(annDiff)
		fileCtx.Functions = funcs

		if fileCtx.Filename != "" && fileCtx.Matches(FileMatchOptions{
			FilterSet:  fs,
			Branch:     prInfo.HeadRefName,
			CommitDate: prInfo.CommitDate,
		}) {
			finalFiles = append(finalFiles, fileCtx)
		}
	}

	return &PRContext{
		Title:       prInfo.Title,
		Description: prInfo.Body,
		Files:       finalFiles,
		Branch:      prInfo.HeadRefName,
		CommitDate:  prInfo.CommitDate,
	}, nil
}

func (fs *FilterSet) MatchesPath(path string) bool {
	if !pathIncluded(path, fs.IncludeFilters) {
		return false
	}

	if len(fs.ExcludeFilters) > 0 && pathIncluded(path, fs.ExcludeFilters) {
		return false
	}

	return true
}

func pathIncluded(path string, globs []string) bool {
	if len(globs) == 0 {
		return true
	}
	// pathspec.Match trims leading ./ from path, but it doesn't trim it from patterns.
	// So we trim it from patterns ourselves.
	cleanGlobs := make([]string, len(globs))
	for i, g := range globs {
		cleanGlobs[i] = strings.TrimPrefix(g, "./")
	}

	spec, err := pathspec.FromLines(cleanGlobs...)
	if err != nil {
		return false
	}
	return spec.Match(path)
}

func (fs *FilterSet) GetFilesForPatterns(branch string) ([]string, error) {
	cmd := exec.Command("git", "ls-tree", "-r", "--name-only", branch)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error running git ls-tree: %w", err)
	}
	allFiles := strings.Split(strings.TrimSpace(string(out)), "\n")

	var result []string
	for _, file := range allFiles {
		if file == "" {
			continue
		}

		if fs.MatchesPath(file) {
			result = append(result, file)
		}
	}

	return result, nil
}

func (fs *FilterSet) GetDiff(baseSHA, headSHA string) (string, error) {
	// Triple-dot (A...B) means diff from common ancestor of A and B to B.
	// Double-dot (A..B) means diff from A to B.
	// For PRs we usually want triple-dot.
	// For "ai-review commit" we probably want double-dot if a base is specified,
	// or triple-dot if comparing to parent (which ends up being same as double-dot).
	// Let's use triple-dot as it's generally safer for PR-like workflows.
	args := []string{"diff", fmt.Sprintf("%s...%s", baseSHA, headSHA)}
	if len(fs.IncludeFilters) > 0 || len(fs.ExcludeFilters) > 0 {
		args = append(args, "--")
		for _, f := range fs.IncludeFilters {
			args = append(args, f)
		}
		for _, f := range fs.ExcludeFilters {
			args = append(args, ":(exclude)"+f)
		}
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running git diff: %w, output: %s", err, string(output))
	}
	return string(output), nil
}

var hunkHeaderRegexp = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func AnnotateDiff(diff string) (string, []string) {
	var result strings.Builder
	var functions []string
	scanner := bufio.NewScanner(strings.NewReader(diff))
	currentLine := 0
	funcRegex := regexp.MustCompile(`(?:func|function|class|def|method|type)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "@@ ") {
			matches := hunkHeaderRegexp.FindStringSubmatch(line)
			if len(matches) > 1 {
				startLine, _ := strconv.Atoi(matches[1])
				currentLine = startLine
			}
			result.WriteString(line + "\n")
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++ ") {
			result.WriteString(fmt.Sprintf("%d:%s\n", currentLine, line))
			matches := funcRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				functions = append(functions, matches[1])
			}
			currentLine++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "--- ") {
			result.WriteString(fmt.Sprintf("%d:%s\n", currentLine, line))
		} else if strings.HasPrefix(line, " ") {
			result.WriteString(fmt.Sprintf("%d:%s\n", currentLine, line))
			currentLine++
		} else {
			result.WriteString(line + "\n")
		}
	}

	return result.String(), functions
}

func (fs *FilterSet) GetChangedFiles(baseSHA, headSHA string) ([]string, error) {
	args := []string{"diff", "--name-only", fmt.Sprintf("%s...%s", baseSHA, headSHA)}
	if len(fs.IncludeFilters) > 0 || len(fs.ExcludeFilters) > 0 {
		args = append(args, "--")
		for _, f := range fs.IncludeFilters {
			args = append(args, f)
		}
		for _, f := range fs.ExcludeFilters {
			args = append(args, ":(exclude)"+f)
		}
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error running git diff --name-only: %w, output: %s", err, string(output))
	}
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(files) == 1 && files[0] == "" {
		return []string{}, nil
	}
	return files, nil
}

func buildPrompt(p Persona, ctx *PRContext, globalInstructions string, preRunAnalyses map[string][]string, summary string, matchedPrimers []PrimerMatch, primerTypes map[string]PrimerType) string {
	var fileList strings.Builder
	for _, file := range ctx.ChangedFiles() {
		fileList.WriteString(fmt.Sprintf("- %s", file))
		if len(p.IncludeExplainers) > 0 {
			if analyses, ok := preRunAnalyses[file]; ok {
				for _, analysis := range analyses {
					// Check if this analysis is from one of the included explainers
					// Analysis format is "PersonaID: description" (based on pipeline.go and how preRunAnalyses is populated)
					parts := strings.SplitN(analysis, ": ", 2)
					if len(parts) > 0 {
						explainerID := parts[0]
						included := false
						for _, id := range p.IncludeExplainers {
							if id == explainerID {
								included = true
								break
							}
						}
						if included {
							fileList.WriteString(fmt.Sprintf("\n  - Explainer Analysis: %s", analysis))
						}
					}
				}
			}
		}
		fileList.WriteString("\n")
	}

	findingsText := ""
	if p.IncludeFindings && summary != "" {
		findingsText = fmt.Sprintf("\n---\n# AGGREGATED REPORT\n%s\n", summary)
	}

	primersSection := ""
	if len(matchedPrimers) > 0 {
		var sb strings.Builder
		sb.WriteString("\n---\n# PRIMERS\n")
		sb.WriteString("The following primers apply to certain files being analyzed. Primers provide extra context, constraints, or blueprints for specific types of changes.\n\n")
		for _, pm := range matchedPrimers {
			typeName := pm.Primer.Type
			typeDesc := ""
			if pt, ok := primerTypes[typeName]; ok {
				typeDesc = pt.Description
			}

			sb.WriteString(fmt.Sprintf("## Primer: %s (Type: %s)\n", pm.Primer.ID, typeName))
			if typeDesc != "" {
				sb.WriteString(fmt.Sprintf("**Type Intent:** %s\n\n", typeDesc))
			}
			sb.WriteString(fmt.Sprintf("**Applies to:**\n\n- %s", strings.Join(pm.Files, "\n- ")))
			sb.WriteString("### Content:\n")
			sb.WriteString(pm.Primer.Content)
			sb.WriteString("\n\n")
		}
		primersSection = sb.String()
	}

	diffSection := ""
	fullDiff := ctx.FullDiff()
	if p.ExcludeDiff {
		// Calculate diff stats
		addedLines := 0
		deletedLines := 0
		scanner := bufio.NewScanner(strings.NewReader(fullDiff))
		for scanner.Scan() {
			line := scanner.Text()
			// Extract original diff line (after line number and colon)
			parts := strings.SplitN(line, ":", 2)
			if len(parts) < 2 {
				continue
			}
			diffLine := parts[1]
			if strings.HasPrefix(diffLine, "+") && !strings.HasPrefix(diffLine, "+++ ") {
				addedLines++
			} else if strings.HasPrefix(diffLine, "-") && !strings.HasPrefix(diffLine, "--- ") {
				deletedLines++
			}
		}
		diffSection = fmt.Sprintf("\n---\n# DIFF STATS\n%d files changed, %d lines added, %d lines deleted. (Full diff excluded by configuration)\n", len(ctx.Files), addedLines, deletedLines)
	} else {
		fence := "```"
		diffSection = fmt.Sprintf(`
---
# UNIFIED DIFF
This is a unified diff format where each line is prefixed with a line number (e.g., "123:"). Lines starting with "-" after the line number indicate removed lines, lines starting with "+" after the line number indicate added lines, and lines starting with " " (space) are unchanged context lines. Diff chunks begin with "@@ -old_start,old_count +new_start,new_count @@" headers that may include function/class context, and are preceded by +++ lines indicating the file being modified.

%sdiff
%s
%s
`, fence, fullDiff, fence)
	}

	prompt := fmt.Sprintf(`# PERSONA INSTRUCTIONS
%s
%s
%s

---
# PR METADATA
## Title
%s

## Description
%s

---
# CHANGED FILES
%s
%s
`, p.Instructions, findingsText, primersSection, ctx.Title, ctx.Description, fileList.String(), diffSection)

	if globalInstructions != "" {
		prompt += fmt.Sprintf("\n---\n# STANDARD INSTRUCTIONS\n%s\n", globalInstructions)
	}

	if p.Role == "explainer" && p.Stage == "pre" {
		prompt = PreRunExplainerSystemPrompt + "\n\n" + prompt
	}

	return prompt
}
