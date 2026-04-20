package main

type Primer struct {
	ID                string    `yaml:"id"`
	AIReview          string    `yaml:"ai_review"`
	Type              string    `yaml:"type"`
	Filters           FilterSet `yaml:",inline"`
	AuthoringConcepts []string  `yaml:"authoring_concepts"`
	Content           string
	SourcePath        string
}

type PrimerMatch struct {
	Primer Primer
	Files  []string
}

func LoadPrimers(searchPaths []string, repo string, headSHA string, oh *OutputHandler) ([]Primer, error) {
	scanner := NewScanner(searchPaths, repo, headSHA, oh)
	results, err := scanner.Load("primer", func() any { return &Primer{} })
	if err != nil && len(results) == 0 {
		return nil, err
	}
	if err != nil {
		oh.Printf("Warning: issues encountered while loading primers: %v\n", err)
	}

	var primers []Primer
	for _, res := range results {
		p := res.Raw.(*Primer)
		p.Content = res.Instructions
		p.SourcePath = res.SourcePath
		primers = append(primers, *p)
	}

	return primers, nil
}

func (rc *RunConfig) FindMatches(personaContext *PRContext) []PrimerMatch {
	var matches []PrimerMatch

	for _, p := range rc.Primers {
		fs := p.Filters
		if err := fs.Compile(); err != nil {
			rc.OutputHandler.Printf("    Warning: error compiling filters for primer %s: %v\n", p.ID, err)
			continue
		}

		var matchedFiles []string
		for _, fileCtx := range personaContext.Files {
			if fileCtx.Matches(FileMatchOptions{
				FilterSet:  &fs,
				Branch:     personaContext.Branch,
				CommitDate: personaContext.CommitDate,
			}) {
				matchedFiles = append(matchedFiles, fileCtx.Filename)
			}
		}

		if len(matchedFiles) > 0 {
			matches = append(matches, PrimerMatch{
				Primer: p,
				Files:  matchedFiles,
			})
		}
	}

	return matches
}
