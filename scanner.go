package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/adrg/frontmatter"
)

type ScanTarget struct {
	ID           string
	AIReview     string
	Instructions string
	Raw          interface{} // Pointer to Persona or Primer
}

type Scanner struct {
	SearchPaths []string
	Repo        string
	HeadSHA     string
	OH          *OutputHandler
}

func NewScanner(searchPaths []string, repo string, headSHA string, oh *OutputHandler) *Scanner {
	return &Scanner{
		SearchPaths: searchPaths,
		Repo:        repo,
		HeadSHA:     headSHA,
		OH:          oh,
	}
}

func (s *Scanner) Load(expectedType string, targetFactory func() interface{}) ([]ScanTarget, error) {
	resultsMap := make(map[string]ScanTarget)

	pluralType := expectedType + "s"
	dedicatedPaths := []string{}
	for _, base := range s.SearchPaths {
		dedicatedPaths = append(dedicatedPaths, filepath.Join(base, ".ai-review", s.Repo, pluralType))
		dedicatedPaths = append(dedicatedPaths, filepath.Join(base, ".ai-review", pluralType))
	}

	// 1. Dedicated directories (local)
	dedicated, _ := s.scanFiles(dedicatedPaths, true, expectedType, targetFactory)
	for _, res := range dedicated {
		resultsMap[res.ID] = res
	}

	// 2. All search paths (local, for files with explicit ai_review)
	other, _ := s.scanFiles(s.SearchPaths, false, expectedType, targetFactory)
	for _, res := range other {
		if _, ok := resultsMap[res.ID]; !ok {
			resultsMap[res.ID] = res
		}
	}

	// 3. Repo branch
	if s.HeadSHA != "" {
		repoResults, _ := s.scanRepo(s.HeadSHA, expectedType, targetFactory)
		for _, res := range repoResults {
			if _, ok := resultsMap[res.ID]; !ok {
				resultsMap[res.ID] = res
			}
		}
	}

	var final []ScanTarget
	for _, res := range resultsMap {
		final = append(final, res)
	}
	return final, nil
}

func (s *Scanner) scanFiles(paths []string, isDedicated bool, expectedType string, targetFactory func() interface{}) ([]ScanTarget, error) {
	var results []ScanTarget
	seenIDs := make(map[string]bool)

	for _, root := range paths {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".md") {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			if res, ok := s.processFile(path, content, expectedType, isDedicated, targetFactory); ok {
				if !seenIDs[res.ID] {
					results = append(results, *res)
					seenIDs[res.ID] = true
				}
			}
			return nil
		})
	}
	return results, nil
}

func (s *Scanner) scanRepo(headSHA string, expectedType string, targetFactory func() interface{}) ([]ScanTarget, error) {
	cmd := exec.Command("git", "ls-tree", "-r", "--name-only", headSHA)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	allFiles := strings.Split(strings.TrimSpace(string(out)), "\n")

	var results []ScanTarget
	seenIDs := make(map[string]bool)

	for _, file := range allFiles {
		if !strings.HasSuffix(strings.ToLower(file), ".md") {
			continue
		}

		cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", headSHA, file))
		content, err := cmd.Output()
		if err != nil {
			continue
		}

		if res, ok := s.processFile(file, content, expectedType, s.isRepoDedicated(file, expectedType), targetFactory); ok {
			if !seenIDs[res.ID] {
				results = append(results, *res)
				seenIDs[res.ID] = true
			}
		}
	}
	return results, nil
}

func (s *Scanner) processFile(path string, content []byte, expectedType string, isDedicated bool, targetFactory func() interface{}) (*ScanTarget, bool) {
	target := targetFactory()
	rest, err := frontmatter.Parse(bytes.NewReader(content), target)
	if err != nil {
		return nil, false
	}

	aiReview, id := getAIReviewAndID(target, path)

	included := false
	if aiReview == expectedType {
		included = true
	} else if aiReview == "" && isDedicated {
		included = true
	}

	if !included {
		return nil, false
	}

	if id == "" {
		id = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		setID(target, id)
	}

	return &ScanTarget{
		ID:           id,
		AIReview:     aiReview,
		Instructions: string(rest),
		Raw:          target,
	}, true
}

func (s *Scanner) isRepoDedicated(path string, expectedType string) bool {
	pluralType := expectedType + "s"
	dedicated1 := ".ai-review/" + s.Repo + "/" + pluralType + "/"
	dedicated2 := ".ai-review/" + pluralType + "/"
	return strings.HasPrefix(path, dedicated1) || strings.HasPrefix(path, dedicated2)
}

func getAIReviewAndID(target interface{}, path string) (string, string) {
	switch v := target.(type) {
	case *Persona:
		return v.AIReview, v.ID
	case *Primer:
		return v.AIReview, v.ID
	}
	return "", ""
}

func setID(target interface{}, id string) {
	switch v := target.(type) {
	case *Persona:
		v.ID = id
	case *Primer:
		v.ID = id
	}
}
