package main

import (
	"regexp"
)

type Primer struct {
	ID             string   `yaml:"id"`
	AIReview       string   `yaml:"ai_review"`
	Type           string   `yaml:"type"`
	PathFilters    []string `yaml:"path_filters"`
	ExcludeFilters []string `yaml:"exclude_filters"`
	RegexFilters   []string `yaml:"regex_filters"`
	Content        string
}

type PrimerMatch struct {
	Primer Primer
	Files  []string
}

func LoadPrimers(searchPaths []string, repo string, headSHA string, oh *OutputHandler) ([]Primer, error) {
	scanner := NewScanner(searchPaths, repo, headSHA, oh)
	results, err := scanner.Load("primer", func() interface{} { return &Primer{} })
	if err != nil {
		return nil, err
	}

	var primers []Primer
	for _, res := range results {
		p := res.Raw.(*Primer)
		p.Content = res.Instructions
		primers = append(primers, *p)
	}

	return primers, nil
}

func (rc *RunConfig) FindMatches(personaContext *PRContext) []PrimerMatch {
	var matches []PrimerMatch

	// Pre-compile regexes for all primers
	type compiledPrimer struct {
		primer  Primer
		regexes []*regexp.Regexp
	}
	var compiledPrimers []compiledPrimer
	for _, p := range rc.Primers {
		var regexes []*regexp.Regexp
		for _, r := range p.RegexFilters {
			re, err := regexp.Compile(r)
			if err != nil {
				rc.OutputHandler.Printf("    Warning: invalid regex %s in primer %s: %v\n", r, p.ID, err)
				continue
			}
			regexes = append(regexes, re)
		}
		compiledPrimers = append(compiledPrimers, compiledPrimer{primer: p, regexes: regexes})
	}

	for _, cp := range compiledPrimers {
		var matchedFiles []string
		for _, fileCtx := range personaContext.Files {
			if fileCtx.Matches(cp.primer.PathFilters, cp.primer.ExcludeFilters, cp.regexes) {
				matchedFiles = append(matchedFiles, fileCtx.Filename)
			}
		}

		if len(matchedFiles) > 0 {
			matches = append(matches, PrimerMatch{
				Primer: cp.primer,
				Files:  matchedFiles,
			})
		}
	}

	return matches
}
