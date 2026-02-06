package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
)

type Persona struct {
	ID                string `yaml:"id"`
	ColoredID         string
	ModelCategory     string   `yaml:"model_category"`
	MaxTokens         int      `yaml:"max_tokens"`
	PathFilters       []string `yaml:"path_filters"`
	ExcludeFilters    []string `yaml:"exclude_filters"`
	RegexFilters      []string `yaml:"regex_filters"`
	Role              string   `yaml:"role"`  // reviewer (default) | explainer
	Stage             string   `yaml:"stage"` // pre | post
	IncludeFindings   bool     `yaml:"include_findings"`
	IncludeExplainers []string `yaml:"include_explainers"`
	ExcludeDiff       bool     `yaml:"exclude_diff"`
	Instructions      string
}

type PersonaRun struct {
	Persona Persona
	Context *PRContext
}

func LoadPersonas(searchPaths []string, repo string, oh *OutputHandler) ([]Persona, error) {
	personaMap := make(map[string]Persona)
	foundAny := false

	for _, base := range searchPaths {
		// Try repo-specific personas
		personaDir := filepath.Join(base, ".ai-review", repo, "personas")
		files, _ := filepath.Glob(filepath.Join(personaDir, "*.md"))

		// Also try global personas
		globalPersonaDir := filepath.Join(base, ".ai-review/personas")
		globalFiles, _ := filepath.Glob(filepath.Join(globalPersonaDir, "*.md"))

		allFiles := append(files, globalFiles...)
		if len(allFiles) > 0 {
			foundAny = true
		}

		for _, file := range allFiles {
			f, err := os.Open(file)
			if err != nil {
				oh.Printf("Warning: could not open persona file %s: %v\n", file, err)
				continue
			}

			var p Persona
			rest, err := frontmatter.Parse(f, &p)
			f.Close()
			if err != nil {
				oh.Printf("Warning: error parsing frontmatter in %s: %v\n", file, err)
				continue
			}
			p.Instructions = string(rest)
			if p.ID == "" {
				// Fallback to filename without extension as ID
				p.ID = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			}
			p.ColoredID = "\033[32m" + p.ID + "\033[0m"
			if p.Role == "" {
				p.Role = "reviewer"
			}
			personaMap[p.ID] = p
		}
	}

	if !foundAny {
		return nil, fmt.Errorf("no personas found in any of the search paths")
	}

	var personas []Persona
	for _, p := range personaMap {
		personas = append(personas, p)
	}

	return personas, nil
}

func (p Persona) Run(ctx context.Context, rc *RunConfig, rr *RunResults, personaContext *PRContext) (string, ModelResult, time.Duration, error) {
	modelCfg, ok := rc.Config.ModelMapping[p.ModelCategory]
	if !ok {
		return "", ModelResult{}, 0, fmt.Errorf("no model mapping for category %s", p.ModelCategory)
	}

	client, err := GetModelClient(ctx, modelCfg.Provider, modelCfg.Model)
	if err != nil {
		return "", ModelResult{}, 0, fmt.Errorf("error creating client: %w", err)
	}

	maxTokens := modelCfg.MaxTokens
	if p.MaxTokens > 0 {
		maxTokens = p.MaxTokens
	}
	if rc.Settings.MaxTokens > 0 {
		maxTokens = rc.Settings.MaxTokens
	}

	var preRunAnalyses map[string][]string
	var summary string

	if p.Role == "reviewer" || (p.Role == "explainer" && p.Stage == "post") {
		preRunAnalyses = rr.PreRunAnalyses
	}
	if p.Role == "explainer" && p.Stage == "post" {
		summary = rr.Summary
	}

	prompt := buildPrompt(p, personaContext, rc.Config.GlobalInstructions, preRunAnalyses, summary)

	personaCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	start := time.Now()
	var result ModelResult
	if p.Role == "explainer" && p.Stage == "pre" {
		result, err = client.GenerateJSON(personaCtx, prompt, maxTokens)
	} else {
		result, err = client.Generate(personaCtx, prompt, maxTokens)
	}
	if err != nil {
		return prompt, ModelResult{}, 0, err
	}
	elapsed := time.Since(start)

	return prompt, result, elapsed, nil
}

func (pr PersonaRun) Execute(ctx context.Context, rc *RunConfig, rr *RunResults) error {
	roleStr := strings.Title(pr.Persona.Role)
	if pr.Persona.Role == "explainer" {
		roleStr = fmt.Sprintf("Explainer (%s)", strings.Title(pr.Persona.Stage))
	}
	rc.OutputHandler.Printf("    -> %s: %s\n", roleStr, pr.Persona.ColoredID)

	prompt, result, elapsed, err := pr.Persona.Run(ctx, rc, rr, pr.Context)
	if err != nil {
		return fmt.Errorf("error executing %s: %w", pr.Persona.ID, err)
	}
	rc.OutputHandler.Printf("    <- Finished %s in %s\n", pr.Persona.ColoredID, elapsed.Round(time.Millisecond))

	rc.OutputHandler.SaveRunFile(filepath.Join(pr.Persona.ID, "prompt.md"), prompt)
	rc.OutputHandler.SaveRunFile(filepath.Join(pr.Persona.ID, "raw.md"), result.Text)

	// Stage-specific logic
	var findings []Finding
	switch pr.Persona.Role {
	case "explainer":
		if pr.Persona.Stage == "pre" {
			analyses, err := ParsePreRunExplainerOutput(result.Text)
			if err != nil {
				rc.OutputHandler.Printf("Warning: error parsing pre-run explainer output for %s: %v\n", pr.Persona.ColoredID, err)
			} else {
				parsedData, _ := json.MarshalIndent(analyses, "", "  ")
				rc.OutputHandler.SaveRunFile(filepath.Join(pr.Persona.ID, "parsed.json"), string(parsedData))
				for _, a := range analyses {
					rr.AddPreRunAnalysis(a.File, fmt.Sprintf("%s: %s", pr.Persona.ID, a.Analysis))
				}
			}
		} else {
			rr.AddPostRunOutput(fmt.Sprintf("### %s\n\n%s", rc.OutputHandler.MarkPersona(pr.Persona.ID), result.Text))
		}
	case "reviewer":
		rc.OutputHandler.Printf("    -> Normalizing findings for %s...\n", pr.Persona.ColoredID)
		normStart := time.Now()
		var normResult ModelResult
		var err error
		findings, normResult, err = NormalizePersonaOutput(ctx, rc.FastestClient, pr.Persona.ID, result.Text)
		normElapsed := time.Since(normStart)
		if err != nil {
			rc.OutputHandler.Printf("Warning: error normalizing findings for %s: %v. Treating as zero findings.\n", pr.Persona.ColoredID, err)
		} else {
			rr.AddFindings(findings)
			findingsData, _ := json.MarshalIndent(findings, "", "  ")
			rc.OutputHandler.SaveRunFile(filepath.Join(pr.Persona.ID, "findings.json"), string(findingsData))
		}

		// Log Normalization usage
		// We need to find the model config for the fastest model to get its price
		// Actually we can just look it up from rc.Config.ModelMapping[string(FastestGood)] or just use the one used in NewRunConfig
		fastestCfg := rc.Config.ModelMapping[string(FastestGood)]
		if fastestCfg.Model == "" { // Fallback if not found
			fastestCfg = rc.Config.ModelMapping[string(Balanced)]
		}

		normEntry := RunLogEntry{
			PersonaID:   "normalization:" + pr.Persona.ID,
			Model:       normResult.Model,
			TokensIn:    normResult.TokensIn,
			TokensOut:   normResult.TokensOut,
			TimeMS:      normElapsed.Milliseconds(),
			InputPrice:  fastestCfg.InputPricePerMillion,
			OutputPrice: fastestCfg.OutputPricePerMillion,
		}
		rr.AddStat(normEntry)
		rc.OutputHandler.LogRun(normEntry)
	}

	entry := RunLogEntry{
		PersonaID:   pr.Persona.ID,
		Model:       result.Model,
		TokensIn:    result.TokensIn,
		TokensOut:   result.TokensOut,
		TimeMS:      elapsed.Milliseconds(),
		RawOutput:   result.Text,
		Findings:    findings,
		InputPrice:  rc.Config.ModelMapping[pr.Persona.ModelCategory].InputPricePerMillion,
		OutputPrice: rc.Config.ModelMapping[pr.Persona.ModelCategory].OutputPricePerMillion,
	}

	rr.AddStat(entry)
	rc.OutputHandler.LogRun(entry)

	return nil
}
