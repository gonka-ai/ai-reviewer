package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type RunLogEntry struct {
	PersonaID   string    `json:"persona_id"`
	Model       string    `json:"model"`
	TokensIn    int       `json:"tokens_in"`
	TokensOut   int       `json:"tokens_out"`
	TimeMS      int64     `json:"time_ms"`
	RawOutput   string    `json:"raw_output,omitempty"`
	Findings    []Finding `json:"findings,omitempty"`
	InputPrice  float64   `json:"input_price,omitempty"`  // Price per million tokens
	OutputPrice float64   `json:"output_price,omitempty"` // Price per million tokens
}

func runPersona(ctx context.Context, persona Persona, prInfo *PRInfo, personaContext *PRContext, config *Config, maxTokensFlag *int, preRunAnalyses map[string][]string, summary string) (string, ModelResult, time.Duration, error) {
	modelCfg, ok := config.ModelMapping[persona.ModelCategory]
	if !ok {
		return "", ModelResult{}, 0, fmt.Errorf("no model mapping for category %s", persona.ModelCategory)
	}

	client, err := GetModelClient(ctx, modelCfg.Provider, modelCfg.Model)
	if err != nil {
		return "", ModelResult{}, 0, fmt.Errorf("error creating client: %w", err)
	}

	maxTokens := modelCfg.MaxTokens
	if persona.MaxTokens > 0 {
		maxTokens = persona.MaxTokens
	}
	if *maxTokensFlag > 0 {
		maxTokens = *maxTokensFlag
	}

	prompt := buildPrompt(persona, personaContext, config.GlobalInstructions, preRunAnalyses, summary)

	personaCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	start := time.Now()
	var result ModelResult
	if persona.Role == "explainer" && persona.Stage == "pre" {
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

type PersonaRun struct {
	Persona Persona
	Context *PRContext
}

func main() {
	initialCwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Error getting current working directory: %v", err)
	}

	maxTokensFlag, concurrencyFlag, repo, prNumber, commitHash, compareTo, filePatterns := initArgs()

	ctx := context.Background()

	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Error getting executable path: %v", err)
	}
	exeDir := filepath.Dir(exePath)

	// Print current working directory
	printCurrentDir()

	// 1. Check dependencies
	fmt.Println("--- Checking dependencies...")
	if err := checkDependencies(); err != nil {
		log.Fatal(err)
	}

	// 2. Resolve target info
	var prInfo *PRInfo
	if filePatterns != nil {
		fmt.Printf("--- Ensuring repo %s is available...\n", repo)
		if err := EnsureRepo(repo); err != nil {
			log.Fatalf("Error ensuring repo: %v", err)
		}

		if err := FetchRefs(repo, "", commitHash); err != nil { // commitHash is used as branch here
			log.Fatalf("Error fetching branch: %v", err)
		}

		prInfo, err = GetFileInfo(repo, commitHash, filePatterns)
		if err != nil {
			log.Fatalf("Error getting file info: %v", err)
		}
	} else if commitHash != "" {
		// If it's a commit, we need to ensure repo is present first to use git commands
		fmt.Printf("--- Ensuring repo %s is available...\n", repo)
		if err := EnsureRepo(repo); err != nil {
			log.Fatalf("Error ensuring repo: %v", err)
		}

		// Ensure we have the commit
		fmt.Printf("--- Fetching commit %s...\n", commitHash)
		if err := FetchCommit(repo, commitHash); err != nil {
			log.Fatalf("Error fetching commit: %v", err)
		}

		// Also fetch comparison commit if specified
		if compareTo != "" {
			fmt.Printf("--- Fetching comparison commit %s...\n", compareTo)
			if err := FetchCommit(repo, compareTo); err != nil {
				log.Fatalf("Error fetching comparison commit: %v", err)
			}
		}

		fmt.Printf("--- Fetching commit info for %s...\n", commitHash)
		prInfo, err = GetCommitInfo(commitHash, compareTo)
		if err != nil {
			log.Fatalf("Error getting commit info: %v", err)
		}
	} else {
		fmt.Printf("--- Fetching PR info for %s #%s...\n", repo, prNumber)
		prInfo, err = GetPRInfo(repo, prNumber)
		if err != nil {
			log.Fatalf("Error getting PR info: %v", err)
		}
	}
	printCurrentDir()

	// 3. Ensure repo and fetch refs (for PRs)
	if commitHash == "" {
		fmt.Printf("--- Ensuring local repository for %s...\n", repo)
		if err := EnsureRepo(repo); err != nil {
			log.Fatalf("Error ensuring repo: %v", err)
		}
		printCurrentDir()

		fmt.Printf("--- Fetching git refs (base: %s)...\n", prInfo.BaseRefName)
		if err := FetchRefs(repo, prNumber, prInfo.BaseRefName); err != nil {
			log.Fatalf("Error fetching refs: %v", err)
		}
		printCurrentDir()
	}

	// 4. Load config and personas
	fmt.Println("--- Loading configuration and personas...")

	var searchPaths []string
	addPath := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		for _, p := range searchPaths {
			if p == abs {
				return
			}
		}
		searchPaths = append(searchPaths, abs)
	}

	addPath(exeDir)
	addPath(initialCwd)
	if cwd, err := os.Getwd(); err == nil {
		addPath(cwd)
	}

	config, err := LoadConfig(searchPaths, repo)
	if err != nil {
		log.Fatalf("Error loading config: %v. Make sure .ai-review/%s/config.yaml exists in one of %v", err, repo, searchPaths)
	}

	personas, err := LoadPersonas(searchPaths, repo)
	if err != nil {
		log.Fatalf("Error loading personas: %v. Make sure .ai-review/%s/personas/*.md exist in one of %v", err, repo, searchPaths)
	}

	// 5. Extract context
	fmt.Println("--- Extracting PR context...")
	var globalContext *PRContext
	if filePatterns != nil {
		globalContext, err = GetPRContext(prInfo, filePatterns, nil, nil)
	} else {
		globalContext, err = GetPRContext(prInfo, nil, nil, nil)
	}
	if err != nil {
		log.Fatalf("Error getting context: %v", err)
	}

	// 5a. Create run directory
	runID := time.Now().Format("2006-01-02_15-04-05")
	targetID := prNumber
	if filePatterns != nil {
		targetID = "file-" + commitHash // branch name
	} else if commitHash != "" {
		targetID = commitHash
	}
	runDir := filepath.Join(initialCwd, ".ai-review", repo, "runs", targetID, runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		log.Fatalf("Error creating run directory: %v", err)
	}
	fmt.Printf("--- Run directory: %s\n", runDir)

	// 6. Filter and Categorize Personas
	fmt.Println("--- Filtering personas...")
	var preRunToRun, preRunToSkip []PersonaRun
	var reviewersToRun, reviewersToSkip []PersonaRun
	var postRunToRun, postRunToSkip []PersonaRun

	for _, p := range personas {
		includes := p.PathFilters
		if len(includes) == 0 && prInfo.BaseRefOid == prInfo.HeadRefOid && !prInfo.IsCommit {
			includes = prInfo.FilePatterns
		}

		var personaContext *PRContext
		if len(includes) > 0 || len(p.ExcludeFilters) > 0 || len(p.RegexFilters) > 0 || (prInfo.BaseRefOid == prInfo.HeadRefOid && !prInfo.IsCommit && prInfo.BaseRefOid != "") {
			var err error
			personaContext, err = GetPRContext(prInfo, includes, p.ExcludeFilters, p.RegexFilters)
			if err != nil {
				fmt.Printf("    Warning: error filtering context for persona %s: %v\n", p.ID, err)
				continue
			}
		} else {
			personaContext = globalContext
		}

		run := PersonaRun{Persona: p, Context: personaContext}
		skip := len(personaContext.ChangedFiles) == 0

		if p.Role == "explainer" {
			if p.Stage == "pre" {
				if skip {
					preRunToSkip = append(preRunToSkip, run)
				} else {
					preRunToRun = append(preRunToRun, run)
				}
			} else {
				if skip {
					postRunToSkip = append(postRunToSkip, run)
				} else {
					postRunToRun = append(postRunToRun, run)
				}
			}
		} else {
			if skip {
				reviewersToSkip = append(reviewersToSkip, run)
			} else {
				reviewersToRun = append(reviewersToRun, run)
			}
		}
	}

	fmt.Println("    To be run:")
	for _, r := range preRunToRun {
		fmt.Printf("      - %s (explainer, pre)\n", r.Persona.ID)
	}
	for _, r := range reviewersToRun {
		fmt.Printf("      - %s (reviewer)\n", r.Persona.ID)
	}
	for _, r := range postRunToRun {
		fmt.Printf("      - %s (explainer, post)\n", r.Persona.ID)
	}

	if len(preRunToSkip) > 0 || len(reviewersToSkip) > 0 || len(postRunToSkip) > 0 {
		fmt.Println("    To be skipped (no matching files):")
		for _, r := range preRunToSkip {
			fmt.Printf("      - %s\n", r.Persona.ID)
		}
		for _, r := range reviewersToSkip {
			fmt.Printf("      - %s\n", r.Persona.ID)
		}
		for _, r := range postRunToSkip {
			fmt.Printf("      - %s\n", r.Persona.ID)
		}
	}

	// 7. Execute Personas
	var stats []RunLogEntry
	var allFindings []Finding
	var postRunOutputs []string
	preRunAnalyses := make(map[string][]string)
	startTimeTotal := time.Now()

	var statsMu sync.Mutex
	var findingsMu sync.Mutex
	var postRunOutputsMu sync.Mutex
	var preRunAnalysesMu sync.Mutex

	concurrency := 3
	if concurrencyFlag != nil {
		concurrency = *concurrencyFlag
	}
	sem := make(chan struct{}, concurrency)

	// Prepare clients for normalization and aggregation
	balancedCfg, ok := config.ModelMapping[string(BestCode)]
	if !ok {
		log.Fatalf("Error: 'balanced' model mapping not found in config.yaml")
	}
	balancedClient, err := GetModelClient(ctx, balancedCfg.Provider, balancedCfg.Model)
	if err != nil {
		log.Fatalf("Error creating balanced client: %v", err)
	}

	fastestCfg, ok := config.ModelMapping[string(FastestGood)]
	if !ok {
		fastestCfg = balancedCfg
	}
	fastestClient, err := GetModelClient(ctx, fastestCfg.Provider, fastestCfg.Model)
	if err != nil {
		fastestClient = balancedClient
	}

	// Stage 1: Pre-run Explainers
	if len(preRunToRun) > 0 {
		fmt.Printf("--- Executing %d pre-run explainers...\n", len(preRunToRun))
		var wg sync.WaitGroup
		for _, run := range preRunToRun {
			wg.Add(1)
			go func(run PersonaRun) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				fmt.Printf("    -> Explainer (Pre): %s\n", run.Persona.ID)
				// Pre-run personas do NOT get pre-run content from other pre-run personas
				prompt, result, elapsed, err := runPersona(ctx, run.Persona, prInfo, run.Context, config, maxTokensFlag, nil, "")
				if err != nil {
					fmt.Printf("Error executing pre-run explainer %s: %v, skipping\n", run.Persona.ID, err)
					return
				}
				fmt.Printf("    <- Finished %s in %s\n", run.Persona.ID, elapsed.Round(time.Millisecond))

				saveToFile(filepath.Join(runDir, run.Persona.ID, "prompt.md"), prompt)
				saveToFile(filepath.Join(runDir, run.Persona.ID, "raw.md"), result.Text)

				analyses, err := ParsePreRunExplainerOutput(result.Text)
				if err != nil {
					fmt.Printf("Warning: error parsing pre-run explainer output for %s: %v\n", run.Persona.ID, err)
				} else {
					parsedData, _ := json.MarshalIndent(analyses, "", "  ")
					saveToFile(filepath.Join(runDir, run.Persona.ID, "parsed.json"), string(parsedData))
					preRunAnalysesMu.Lock()
					for _, a := range analyses {
						preRunAnalyses[a.File] = append(preRunAnalyses[a.File], fmt.Sprintf("%s: %s", run.Persona.ID, a.Analysis))
					}
					preRunAnalysesMu.Unlock()
				}

				entry := RunLogEntry{
					PersonaID:   run.Persona.ID,
					Model:       result.Model,
					TokensIn:    result.TokensIn,
					TokensOut:   result.TokensOut,
					TimeMS:      elapsed.Milliseconds(),
					RawOutput:   result.Text,
					InputPrice:  config.ModelMapping[run.Persona.ModelCategory].InputPricePerMillion,
					OutputPrice: config.ModelMapping[run.Persona.ModelCategory].OutputPricePerMillion,
				}
				statsMu.Lock()
				stats = append(stats, entry)
				statsMu.Unlock()
				logRun(initialCwd, repo, entry)
			}(run)
		}
		wg.Wait()
	}

	// Stage 2: Reviewers
	fmt.Printf("--- Executing %d reviewers...\n", len(reviewersToRun))
	var wg sync.WaitGroup
	for _, run := range reviewersToRun {
		wg.Add(1)
		go func(run PersonaRun) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fmt.Printf("    -> Reviewer: %s\n", run.Persona.ID)
			prompt, result, elapsed, err := runPersona(ctx, run.Persona, prInfo, run.Context, config, maxTokensFlag, preRunAnalyses, "")
			if err != nil {
				fmt.Printf("Error executing reviewer %s: %v, skipping\n", run.Persona.ID, err)
				return
			}
			fmt.Printf("    <- Finished %s in %s\n", run.Persona.ID, elapsed.Round(time.Millisecond))

			saveToFile(filepath.Join(runDir, run.Persona.ID, "prompt.md"), prompt)
			saveToFile(filepath.Join(runDir, run.Persona.ID, "raw.md"), result.Text)

			// Normalization Step
			fmt.Printf("    -> Normalizing findings for %s...\n", run.Persona.ID)
			normStart := time.Now()
			findings, normResult, err := NormalizePersonaOutput(ctx, fastestClient, run.Persona.ID, result.Text)
			normElapsed := time.Since(normStart)
			if err != nil {
				fmt.Printf("Warning: error normalizing findings for %s: %v. Treating as zero findings.\n", run.Persona.ID, err)
			} else {
				findingsMu.Lock()
				allFindings = append(allFindings, findings...)
				findingsMu.Unlock()
				findingsData, _ := json.MarshalIndent(findings, "", "  ")
				saveToFile(filepath.Join(runDir, run.Persona.ID, "findings.json"), string(findingsData))
			}

			// Log Normalization usage
			normEntry := RunLogEntry{
				PersonaID:   "normalization:" + run.Persona.ID,
				Model:       normResult.Model,
				TokensIn:    normResult.TokensIn,
				TokensOut:   normResult.TokensOut,
				TimeMS:      normElapsed.Milliseconds(),
				InputPrice:  fastestCfg.InputPricePerMillion,
				OutputPrice: fastestCfg.OutputPricePerMillion,
			}
			statsMu.Lock()
			stats = append(stats, normEntry)
			statsMu.Unlock()
			logRun(initialCwd, repo, normEntry)

			entry := RunLogEntry{
				PersonaID:   run.Persona.ID,
				Model:       result.Model,
				TokensIn:    result.TokensIn,
				TokensOut:   result.TokensOut,
				TimeMS:      elapsed.Milliseconds(),
				RawOutput:   result.Text,
				Findings:    findings,
				InputPrice:  config.ModelMapping[run.Persona.ModelCategory].InputPricePerMillion,
				OutputPrice: config.ModelMapping[run.Persona.ModelCategory].OutputPricePerMillion,
			}
			statsMu.Lock()
			stats = append(stats, entry)
			statsMu.Unlock()
			logRun(initialCwd, repo, entry)
		}(run)
	}
	wg.Wait()

	// 7. Aggregation Step
	fmt.Println("--- Aggregating all findings...")
	findingsData, _ := json.MarshalIndent(allFindings, "", "  ")
	saveToFile(filepath.Join(runDir, "all_findings.json"), string(findingsData))

	aggStart := time.Now()
	summary, aggResult, err := AggregateFindings(ctx, balancedClient, allFindings)
	aggElapsed := time.Since(aggStart)
	if err != nil {
		fmt.Printf("Error aggregating findings: %v\n", err)
		summary = "Error generating aggregated summary."
	}
	saveToFile(filepath.Join(runDir, "summary.md"), summary)

	// Log Aggregation usage
	aggEntry := RunLogEntry{
		PersonaID:   "aggregator",
		Model:       aggResult.Model,
		TokensIn:    aggResult.TokensIn,
		TokensOut:   aggResult.TokensOut,
		TimeMS:      aggElapsed.Milliseconds(),
		InputPrice:  balancedCfg.InputPricePerMillion,
		OutputPrice: balancedCfg.OutputPricePerMillion,
	}
	statsMu.Lock()
	stats = append(stats, aggEntry)
	statsMu.Unlock()
	logRun(initialCwd, repo, aggEntry)

	// Stage 3: Post-run Explainers
	if len(postRunToRun) > 0 {
		fmt.Printf("--- Executing %d post-run explainers...\n", len(postRunToRun))
		var wg sync.WaitGroup
		for _, run := range postRunToRun {
			wg.Add(1)
			go func(run PersonaRun) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				fmt.Printf("    -> Explainer (Post): %s\n", run.Persona.ID)
				prompt, result, elapsed, err := runPersona(ctx, run.Persona, prInfo, run.Context, config, maxTokensFlag, preRunAnalyses, summary)
				if err != nil {
					fmt.Printf("Error executing post-run explainer %s: %v, skipping\n", run.Persona.ID, err)
					return
				}
				fmt.Printf("    <- Finished %s in %s\n", run.Persona.ID, elapsed.Round(time.Millisecond))

				saveToFile(filepath.Join(runDir, run.Persona.ID, "prompt.md"), prompt)
				saveToFile(filepath.Join(runDir, run.Persona.ID, "raw.md"), result.Text)

				postRunOutputsMu.Lock()
				postRunOutputs = append(postRunOutputs, fmt.Sprintf("### %s\n\n%s", run.Persona.ID, result.Text))
				postRunOutputsMu.Unlock()

				entry := RunLogEntry{
					PersonaID:   run.Persona.ID,
					Model:       result.Model,
					TokensIn:    result.TokensIn,
					TokensOut:   result.TokensOut,
					TimeMS:      elapsed.Milliseconds(),
					RawOutput:   result.Text,
					InputPrice:  config.ModelMapping[run.Persona.ModelCategory].InputPricePerMillion,
					OutputPrice: config.ModelMapping[run.Persona.ModelCategory].OutputPricePerMillion,
				}
				statsMu.Lock()
				stats = append(stats, entry)
				statsMu.Unlock()
				logRun(initialCwd, repo, entry)
			}(run)
		}
		wg.Wait()
	}

	// 8. Report
	fmt.Println("--- Generating report...")
	totalElapsed := time.Since(startTimeTotal)
	report := generateReport(prNumber, commitHash, prInfo.BaseRefOid, prInfo.HeadRefOid, summary, postRunOutputs, stats, totalElapsed, filePatterns)
	fmt.Print(report)
	saveToFile(filepath.Join(runDir, "report.md"), report)
}

func initArgs() (*int, *int, string, string, string, string, []string) {
	prCmd := flag.NewFlagSet("pr", flag.ExitOnError)
	prMaxTokens := prCmd.Int("max-tokens", 0, "Override max tokens for AI response")
	prConcurrency := prCmd.Int("concurrency", 3, "Max concurrent personas")

	commitCmd := flag.NewFlagSet("commit", flag.ExitOnError)
	commitMaxTokens := commitCmd.Int("max-tokens", 0, "Override max tokens for AI response")
	commitConcurrency := commitCmd.Int("concurrency", 3, "Max concurrent personas")
	compareTo := commitCmd.String("compare-to", "", "Specific commit to compare to (default: parent)")

	fileCmd := flag.NewFlagSet("file", flag.ExitOnError)
	fileMaxTokens := fileCmd.Int("max-tokens", 0, "Override max tokens for AI response")
	fileConcurrency := fileCmd.Int("concurrency", 3, "Max concurrent personas")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "pr":
		prCmd.Parse(os.Args[2:])
		args := prCmd.Args()
		if len(args) < 2 {
			printUsage()
			os.Exit(1)
		}
		return prMaxTokens, prConcurrency, args[0], args[1], "", "", nil
	case "commit":
		commitCmd.Parse(os.Args[2:])
		args := commitCmd.Args()
		if len(args) < 2 {
			printUsage()
			os.Exit(1)
		}
		return commitMaxTokens, commitConcurrency, args[0], "", args[1], *compareTo, nil
	case "file":
		fileCmd.Parse(os.Args[2:])
		args := fileCmd.Args()
		if len(args) < 3 {
			printUsage()
			os.Exit(1)
		}
		repo := args[0]
		branch := args[1]
		files := args[2:]
		return fileMaxTokens, fileConcurrency, repo, "", branch, "", files
	default:
		fmt.Println("Unknown command:", os.Args[1])
		printUsage()
		os.Exit(1)
	}
	return nil, nil, "", "", "", "", nil
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  ai-reviewer pr <repo> <pr-number> [--max-tokens <n>] [--concurrency <n>]")
	fmt.Println("  ai-reviewer commit <repo> <commit-hash> [--compare-to <hash>] [--max-tokens <n>] [--concurrency <n>]")
	fmt.Println("  ai-reviewer file <repo> <branch> <file1> <file2> ... [--max-tokens <n>] [--concurrency <n>]")
}

func printCurrentDir() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("Warning: Could not get current working directory: %v", err)
	} else {
		absPath, _ := filepath.Abs(cwd)
		fmt.Printf("Current working directory: %s\n", absPath)
	}
}

func checkDependencies() error {
	_, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("github cli (gh) is not installed")
	}
	_, err = exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is not installed")
	}
	return nil
}

func logRun(initialCwd, repo string, entry RunLogEntry) {
	// Note: we assume we are in the repo root where .ai-review exists
	logDir := filepath.Join(initialCwd, ".ai-review", repo)
	_ = os.MkdirAll(logDir, 0755)
	f, err := os.OpenFile(filepath.Join(logDir, "run-log.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	data, _ := json.Marshal(entry)
	f.Write(append(data, '\n'))
}

func saveToFile(path, content string) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Printf("Warning: could not create directory for %s: %v\n", path, err)
		return
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Printf("Warning: could not save to %s: %v\n", path, err)
	}
}

func generateReport(prNumber, commitHash, baseSHA, headSHA, summary string, postRunOutputs []string, stats []RunLogEntry, totalTime time.Duration, filePatterns []string) string {
	var out strings.Builder
	out.WriteString("# AI Review Report\n\n")
	if filePatterns != nil {
		out.WriteString(fmt.Sprintf("## Files on %s\n", headSHA))
		out.WriteString(fmt.Sprintf("- **Patterns:** `%v`\n", filePatterns))
	} else if prNumber != "" {
		out.WriteString(fmt.Sprintf("## PR #%s\n", prNumber))
	} else {
		out.WriteString(fmt.Sprintf("## Commit %s\n", headSHA[:8]))
	}
	out.WriteString(fmt.Sprintf("- **Base Commit:** `%s`\n", baseSHA))
	out.WriteString(fmt.Sprintf("- **Head Commit:** `%s`\n\n", headSHA))
	out.WriteString(summary)
	out.WriteString("\n\n")

	if len(postRunOutputs) > 0 {
		out.WriteString("## Explanations\n\n")
		for _, output := range postRunOutputs {
			out.WriteString(output)
			out.WriteString("\n\n")
		}
	}

	out.WriteString("## Stats\n")
	totalIn := 0
	totalOut := 0
	totalCost := 0.0

	type mStats struct {
		in, out int
		cost    float64
	}
	modelStats := make(map[string]mStats)

	for _, s := range stats {
		cost := (float64(s.TokensIn) * s.InputPrice / 1000000.0) +
			(float64(s.TokensOut) * s.OutputPrice / 1000000.0)

		out.WriteString(fmt.Sprintf("- %s (%s): In: %d, Out: %d, Time: %dms, Cost: $%.6f\n", s.PersonaID, s.Model, s.TokensIn, s.TokensOut, s.TimeMS, cost))
		totalIn += s.TokensIn
		totalOut += s.TokensOut
		totalCost += cost

		ms := modelStats[s.Model]
		ms.in += s.TokensIn
		ms.out += s.TokensOut
		ms.cost += cost
		modelStats[s.Model] = ms
	}

	out.WriteString(fmt.Sprintf("\nTotal Tokens: %d (In: %d, Out: %d)\n", totalIn+totalOut, totalIn, totalOut))
	out.WriteString(fmt.Sprintf("Total Wall Time: %s\n", totalTime.Round(time.Millisecond)))

	out.WriteString("\n### Usage by Model\n")
	for model, ms := range modelStats {
		out.WriteString(fmt.Sprintf("- %s: %d tokens (In: %d, Out: %d), Cost: $%.6f\n", model, ms.in+ms.out, ms.in, ms.out, ms.cost))
	}
	out.WriteString(fmt.Sprintf("\n### Estimated Total Cost: $%.6f\n", totalCost))

	return out.String()
}
