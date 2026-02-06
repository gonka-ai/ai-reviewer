package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Finding struct {
	Source       string  `json:"source"`
	File         string  `json:"file"`
	LineStart    *int    `json:"line_start,omitempty"`
	LineEnd      *int    `json:"line_end,omitempty"`
	Summary      string  `json:"summary"`
	Details      string  `json:"details,omitempty"`
	SeverityHint string  `json:"severity_hint"` // low | medium | high | unknown
	Confidence   float64 `json:"confidence"`    // 0.0–1.0
}

type NormalizationResponse struct {
	Findings []Finding `json:"findings"`
}

type PreRunAnalysis struct {
	File     string `json:"file"`
	Analysis string `json:"analysis"`
}

type PreRunExplainerResponse struct {
	Files []PreRunAnalysis `json:"files"`
}

const PreRunExplainerSystemPrompt = `
You are a pre-run explainer for automated code review.
Your task is to analyze each file in the diff and provide critical information or research as requested.

You MUST:
- Provide analysis for EACH file mentioned in the diff.
- Keep the analysis concise and focused on the requested research.
- Output ONLY valid JSON in the specified format.

OUTPUT FORMAT (JSON ONLY):
{
  "files": [
    {
      "file": "<path/to/file>",
      "analysis": "<your analysis for this file>"
    }
  ]
}
`

const NormalizationSystemPrompt = `
You are a normalization extractor for automated code review.

Your task is to convert a reviewer’s raw output into a list of concrete findings.

You MUST:
	•	Remove all reasoning, commentary, and self-reflection
	•	Extract only actionable issues explicitly mentioned
	•	Preserve file names and line numbers exactly as stated
	•	Preserve attribution to the source persona
	•	Lower confidence if locations are approximate

You MUST NOT:
	•	Invent new issues
	•	Re-analyze the code
	•	Guess or fabricate line numbers
	•	Add severity not implied by the reviewer

If the reviewer reports no issues, return an empty list.

OUTPUT FORMAT (JSON ONLY)

{
  "findings": [
    {
      "source": "<persona_id>",
      "file": "<path>",
      "line_start": <int or null>,
      "line_end": <int or null>,
      "summary": "<short, concrete issue>",
      "details": "<optional, one sentence>",
      "severity_hint": "low | medium | high | unknown",
      "confidence": <float between 0.0 and 1.0>
    }
  ]
}

Return only valid JSON. No extra text.
`

const AggregatorSystemPrompt = `
You are an aggregator for automated code review findings.
Your goal is to turn many individual findings into a concise, human-usable review.

The aggregator:
	•	Deduplicates similar findings
	•	Clusters related issues
	•	Assigns final severity
	•	Produces short, actionable lists
	•	Produces one-line summaries per persona
	•	Includes a file/line number reference when applicable


The aggregator must not:
	•	Analyze code
	•	Invent new findings
	•	Add fake precision

You must produce Markdown with four sections:
	1.	Must Fix
	2.	Review Carefully
	3.	Consider
	4.	Persona Summaries

Plus a short executive summary paragraph at the top.

Example structure:

## Summary
<3–5 sentences, high level>

## ❗ Must Fix
- <issue> (sources: @persona{persona1}, @persona{persona2})

## ⚠️ Review Carefully
- <issue> (sources: @persona{persona1})

## 💭 Consider
- <issue> (sources: @persona{persona3}, ./filename.go; lines 10–15)

## Persona Summaries
- @persona{Persona1}: ❌ Significant issues
- @persona{Persona2}: ⚠️ Minor issues
- @persona{Persona3}: ✅ Looks reasonable

Instructions for aggregation:
	•	Cut all chatter
	•	Prefer bullet points
	•	Preserve source attribution
	•	Upgrade severity when multiple personas agree
	•	Downgrade severity for low-confidence findings
	•	Explicitly call out disagreements
	•	Use @persona{ID} whenever you refer to a persona's ID.
`

func extractJSON(text string) string {
	if idx := strings.Index(text, "```json"); idx != -1 {
		text = text[idx+7:]
		if endIdx := strings.Index(text, "```"); endIdx != -1 {
			text = text[:endIdx]
		}
	} else if idx := strings.Index(text, "```"); idx != -1 {
		text = text[idx+3:]
		if endIdx := strings.Index(text, "```"); endIdx != -1 {
			text = text[:endIdx]
		}
	}
	return strings.TrimSpace(text)
}

func NormalizePersonaOutput(ctx context.Context, client ModelClient, personaID, rawOutput string) ([]Finding, ModelResult, error) {
	prompt := fmt.Sprintf("%s\n\n--- PERSONA OUTPUT (%s) ---\n%s", NormalizationSystemPrompt, personaID, rawOutput)

	normCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	result, err := client.GenerateJSON(normCtx, prompt, 0)
	if err != nil {
		return nil, ModelResult{}, err
	}

	text := extractJSON(result.Text)

	var response NormalizationResponse
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		return nil, result, fmt.Errorf("error unmarshaling normalization response: %w", err)
	}

	return response.Findings, result, nil
}

func ParsePreRunExplainerOutput(rawOutput string) ([]PreRunAnalysis, error) {
	text := extractJSON(rawOutput)
	var response PreRunExplainerResponse
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling pre-run explainer response: %w", err)
	}
	return response.Files, nil
}

func AggregateFindings(ctx context.Context, client ModelClient, findings []Finding) (string, ModelResult, error) {
	if len(findings) == 0 {
		return "## Summary\nNo issues found by any persona.", ModelResult{}, nil
	}

	findingsJSON, err := json.Marshal(NormalizationResponse{Findings: findings})
	if err != nil {
		return "", ModelResult{}, err
	}

	prompt := fmt.Sprintf("%s\n\n--- FINDINGS ---\n%s", AggregatorSystemPrompt, string(findingsJSON))

	// Aggregator uses balanced, which might take a bit longer than normalization but 5m is plenty as per main.go's default
	result, err := client.Generate(ctx, prompt, 0)
	if err != nil {
		return "", ModelResult{}, err
	}

	return result.Text, result, nil
}
