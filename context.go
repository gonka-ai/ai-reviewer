package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type PRInfo struct {
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	BaseRefName  string   `json:"baseRefName"`
	BaseRefOid   string   `json:"baseRefOid"`
	HeadRefName  string   `json:"headRefName"`
	HeadRefOid   string   `json:"headRefOid"`
	IsCommit     bool     `json:"isCommit"`
	FilePatterns []string `json:"filePatterns"`
}

type PRContext struct {
	Title       string
	Description string
	Files       []FileContext
}

type FileContext struct {
	Filename   string
	Diff       string   // Annotated diff for this file
	AddedLines []string // Only the content of the added lines
}

func (f FileContext) Matches(includes, excludes []string, regexes []*regexp.Regexp) bool {
	if !MatchesFilters(f.Filename, includes, excludes) {
		return false
	}
	if len(regexes) == 0 {
		return true
	}
	for _, line := range f.AddedLines {
		for _, re := range regexes {
			if re.MatchString(line) {
				return true
			}
		}
	}
	return false
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
	var addedLines []string
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

		if strings.HasPrefix(diffLine, "+") && !strings.HasPrefix(diffLine, "+++ ") {
			addedLines = append(addedLines, strings.TrimPrefix(diffLine, "+"))
		}
	}
	return FileContext{Filename: filename, Diff: fd, AddedLines: addedLines}
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

	return &pr, nil
}

func GetCommitInfo(commitHash, compareTo string) (*PRInfo, error) {
	fmt.Printf("    -> Getting info for commit %s...\n", commitHash)

	// Get commit message (title and body)
	cmd := exec.Command("git", "show", "-s", "--format=%s%n%n%b", commitHash)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error getting commit message: %w, output: %s", err, string(output))
	}

	fullMsg := strings.TrimSpace(string(output))
	parts := strings.SplitN(fullMsg, "\n", 2)
	title := parts[0]
	body := ""
	if len(parts) > 1 {
		body = strings.TrimSpace(parts[1])
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

	// Resolve patterns to actual files
	files, err := GetFilesForPatterns(branch, filePatterns, nil)
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
		BaseRefName:  branch,
		HeadRefName:  branch,
		FilePatterns: filePatterns,
	}, nil
}

func GetPRContext(prInfo *PRInfo, includeFilters, excludeFilters, regexFilters []string) (*PRContext, error) {
	var compiledRegexes []*regexp.Regexp
	for _, r := range regexFilters {
		re, err := regexp.Compile(r)
		if err != nil {
			return nil, fmt.Errorf("invalid regex %s: %w", r, err)
		}
		compiledRegexes = append(compiledRegexes, re)
	}

	if prInfo.BaseRefOid == prInfo.HeadRefOid && !prInfo.IsCommit && prInfo.BaseRefOid != "" {
		// This is "file" mode, we want to see the whole content of the files as if it were a new file
		files, err := GetFilesForPatterns(prInfo.HeadRefOid, includeFilters, excludeFilters)
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

			fileCtx := FileContext{
				Filename:   file,
				Diff:       AnnotateDiff(diffBuilder.String()),
				AddedLines: lines,
			}

			if fileCtx.Matches(nil, nil, compiledRegexes) {
				finalFiles = append(finalFiles, fileCtx)
			}
		}

		return &PRContext{
			Title:       prInfo.Title,
			Description: prInfo.Body,
			Files:       finalFiles,
		}, nil
	}

	diff, err := GetDiff(prInfo.BaseRefOid, prInfo.HeadRefOid, includeFilters, excludeFilters)
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

		fileCtx := ParseAnnotatedFileDiff("diff --git " + fd)
		if fileCtx.Filename != "" && fileCtx.Matches(nil, nil, compiledRegexes) {
			finalFiles = append(finalFiles, fileCtx)
		}
	}

	return &PRContext{
		Title:       prInfo.Title,
		Description: prInfo.Body,
		Files:       finalFiles,
	}, nil
}

func MatchesFilters(file string, includes, excludes []string) bool {
	if !pathIncluded(file, includes) {
		return false
	}

	if len(excludes) > 0 && pathIncluded(file, excludes) {
		return false
	}

	return true
}

func pathIncluded(path string, globs []string) bool {
	if len(globs) == 0 {
		return true
	}
	path = strings.TrimPrefix(path, "./")
	for _, glob := range globs {
		g := strings.TrimPrefix(glob, "./")

		matched, _ := doublestar.Match(g, path)
		if matched {
			return true
		}

		// If the glob ends with a slash, it should match everything inside that directory.
		if strings.HasSuffix(g, "/") {
			matched, _ = doublestar.Match(g+"**", path)
			if matched {
				return true
			}
		}

		// Keep substring match for simple directory/file names
		if strings.Contains(path, g) {
			return true
		}
	}
	return false
}

func GetFilesForPatterns(branch string, includeFilters, excludeFilters []string) ([]string, error) {
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

		if MatchesFilters(file, includeFilters, excludeFilters) {
			result = append(result, file)
		}
	}

	return result, nil
}

func GetDiff(baseSHA, headSHA string, includeFilters, excludeFilters []string) (string, error) {
	// Triple-dot (A...B) means diff from common ancestor of A and B to B.
	// Double-dot (A..B) means diff from A to B.
	// For PRs we usually want triple-dot.
	// For "ai-review commit" we probably want double-dot if a base is specified,
	// or triple-dot if comparing to parent (which ends up being same as double-dot).
	// Let's use triple-dot as it's generally safer for PR-like workflows.
	args := []string{"diff", fmt.Sprintf("%s...%s", baseSHA, headSHA)}
	if len(includeFilters) > 0 || len(excludeFilters) > 0 {
		args = append(args, "--")
		for _, f := range includeFilters {
			args = append(args, f)
		}
		for _, f := range excludeFilters {
			args = append(args, ":(exclude)"+f)
		}
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running git diff: %w, output: %s", err, string(output))
	}
	return AnnotateDiff(string(output)), nil
}

var hunkHeaderRegexp = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func AnnotateDiff(diff string) string {
	var result strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(diff))
	currentLine := 0

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

	return result.String()
}

func GetChangedFiles(baseSHA, headSHA string, includeFilters, excludeFilters []string) ([]string, error) {
	args := []string{"diff", "--name-only", fmt.Sprintf("%s...%s", baseSHA, headSHA)}
	if len(includeFilters) > 0 || len(excludeFilters) > 0 {
		args = append(args, "--")
		for _, f := range includeFilters {
			args = append(args, f)
		}
		for _, f := range excludeFilters {
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
