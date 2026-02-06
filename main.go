package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func main() {
	s := NewRunSettings()

	var err error
	ctx := context.Background()

	// 1. Check dependencies
	fmt.Println("--- Checking dependencies...")
	if err := checkDependencies(); err != nil {
		log.Fatal(err)
	}

	// 2. Load RunConfig (Discovered settings)
	runConfig, err := NewRunConfig(ctx, s)
	if err != nil {
		log.Fatal(err)
	}

	// 7. Execute Personas
	runResults := NewRunResults()

	concurrency := s.Concurrency
	sem := make(chan struct{}, concurrency)

	// Stage 1: Pre-run Explainers
	runPersonas(ctx, runConfig.PreRunToRun, runConfig, runResults, sem, "pre-run explainers")

	// Stage 2: Reviewers
	runPersonas(ctx, runConfig.ReviewersToRun, runConfig, runResults, sem, "reviewers")

	// 7. Aggregation Step
	runConfig.OutputHandler.Println("--- Aggregating all findings...")
	findingsData, _ := json.MarshalIndent(runResults.AllFindings, "", "  ")
	runConfig.OutputHandler.SaveRunFile("all_findings.json", string(findingsData))

	aggStart := time.Now()
	summary, aggResult, err := AggregateFindings(ctx, runConfig.BalancedClient, runResults.AllFindings)
	runResults.Summary = summary
	aggElapsed := time.Since(aggStart)
	if err != nil {
		runConfig.OutputHandler.Printf("Error aggregating findings: %v\n", err)
		runResults.Summary = "Error generating aggregated summary."
	}
	runConfig.OutputHandler.SaveRunFile("summary.md", runResults.Summary)

	// Log Aggregation usage
	balancedCfg := runConfig.Config.ModelMapping[string(BestCode)]
	aggEntry := RunLogEntry{
		PersonaID:   "aggregator",
		Model:       aggResult.Model,
		TokensIn:    aggResult.TokensIn,
		TokensOut:   aggResult.TokensOut,
		TimeMS:      aggElapsed.Milliseconds(),
		InputPrice:  balancedCfg.InputPricePerMillion,
		OutputPrice: balancedCfg.OutputPricePerMillion,
	}
	runResults.AddStat(aggEntry)
	runConfig.OutputHandler.LogRun(aggEntry)

	// Stage 3: Post-run Explainers
	runPersonas(ctx, runConfig.PostRunToRun, runConfig, runResults, sem, "post-run explainers")

	// 8. Report
	runConfig.OutputHandler.Println("--- Generating report...")
	runResults.Finish()
	runResults.Report = generateReport(s.PRNumber, s.CommitHash, runConfig.PRInfo.BaseRefOid, runConfig.PRInfo.HeadRefOid, runResults, s.FilePatterns, runConfig.OutputHandler)
	runConfig.OutputHandler.Printf("%s", runConfig.OutputHandler.Highlight(runResults.Report))
	runConfig.OutputHandler.SaveRunFile("report.md", runConfig.OutputHandler.StripMarkers(runResults.Report))
}

func runPersonas(ctx context.Context, personas []PersonaRun, rc *RunConfig, rr *RunResults, sem chan struct{}, stageLabel string) {
	if len(personas) == 0 {
		return
	}
	rc.OutputHandler.Printf("--- Executing %d %s...\n", len(personas), stageLabel)
	var wg sync.WaitGroup
	for _, run := range personas {
		wg.Add(1)
		go func(run PersonaRun) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := run.Execute(ctx, rc, rr); err != nil {
				rc.OutputHandler.Printf("Error executing %s %s: %v, skipping\n", stageLabel, run.Persona.ColoredID, err)
			}
		}(run)
	}
	wg.Wait()
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

func generateReport(prNumber, commitHash, baseSHA, headSHA string, rr *RunResults, filePatterns []string, oh *OutputHandler) string {
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
	out.WriteString(rr.Summary)
	out.WriteString("\n\n")

	if len(rr.PostRunOutputs) > 0 {
		out.WriteString("## Explanations\n\n")
		for _, output := range rr.PostRunOutputs {
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

	for _, s := range rr.Stats {
		cost := (float64(s.TokensIn) * s.InputPrice / 1000000.0) +
			(float64(s.TokensOut) * s.OutputPrice / 1000000.0)

		out.WriteString(fmt.Sprintf("- %s (%s): In: %d, Out: %d, Time: %dms, Cost: $%.6f\n", oh.MarkPersona(s.PersonaID), s.Model, s.TokensIn, s.TokensOut, s.TimeMS, cost))
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
	out.WriteString(fmt.Sprintf("Total Wall Time: %s\n", rr.TotalElapsed.Round(time.Millisecond)))

	out.WriteString("\n### Usage by Model\n")
	for model, ms := range modelStats {
		out.WriteString(fmt.Sprintf("- %s: %d tokens (In: %d, Out: %d), Cost: $%.6f\n", model, ms.in+ms.out, ms.in, ms.out, ms.cost))
	}
	out.WriteString(fmt.Sprintf("\n### Estimated Total Cost: $%.6f\n", totalCost))

	return out.String()
}
