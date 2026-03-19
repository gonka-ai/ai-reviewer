# Architecture

`ai-reviewer` is a single-binary Go CLI that runs AI-assisted code review over a GitHub PR, a commit, a branch-to-branch diff, or a selected set of files. Its central design idea is that review should be split into specialized personas rather than delegated to one generic model prompt.

At a high level, the tool:

1. Resolves a review target and gathers git/GitHub context.
2. Discovers review artifacts from `.ai-review`.
3. Filters personas and primers down to the relevant files.
4. Executes a staged review pipeline.
5. Writes auditable artifacts for the run.

## Core Design Principles

### Specialized personas

The system treats code review as a set of distinct review lenses. A persona can focus on one kind of risk or domain concern such as security, performance, consensus, tests, or architecture. It can even focus on specific files with specific rules or concerns. This keeps prompts narrower and tends to produce higher-signal findings than a single broad reviewer, especially with lesser equipped models.

### Multi-stage review pipeline

The review process is intentionally staged:

- Pre-run explainers provide structured context before reviewers run.
- Reviewers produce raw findings in natural language.
- A normalization pass converts those findings into structured JSON.
- Waivers suppress findings that are intentionally allowed.
- Post-run explainers add human-readable guidance after findings are known.
- An aggregator turns the remaining findings into a concise final report.

This is the main architectural choice in the repo. The code is built around orchestrating these stages reliably and capturing their outputs.

### Artifact-driven operation

Every run writes prompts, model outputs, normalized findings, summaries, and reports to disk under `.ai-review/<owner>/<repo>/runs/...`. This makes the system inspectable and debuggable. The architecture assumes that AI behavior must be reviewable after the fact.

## Major Concepts

### Run target

A run can operate on one of four target types:

- `pr`: a GitHub pull request
- `commit`: a commit compared to its parent or a specified base
- `file`: a set of files on a branch, treated as whole-file additions
- `branches`: the diff between two refs

The target is parsed into `RunSettings`, then resolved into `PRInfo` and `PRContext`.

### PRInfo

`PRInfo` is the resolved target descriptor. It contains metadata such as title, body, base SHA, head SHA, branch names, commit date, and file patterns.

### PRContext and FileContext

`PRContext` is the reviewable context passed into personas. It contains:

- title and description
- target branch
- commit date
- a slice of `FileContext`

Each `FileContext` contains:

- the filename
- an annotated diff
- added-line content
- function names heuristically extracted from changed lines

This is the core data model used for filtering and prompt generation.

### Personas

A persona is a Markdown file with YAML frontmatter and free-form instructions. Personas are the main executable review units.

Important persona fields include:

- `model_category`
- `path_filters`
- `exclude_filters`
- `regex_filters`
- `branch_filters`
- `function_filters`
- `line_numbers_filter`
- `date_filter`
- `role`
- `stage`

Personas run in one of two roles:

- `reviewer`
- `explainer`

Explainers can run in two stages:

- `pre`
- `post`

### Primers

Primers are contextual documents that are injected into persona prompts when their filters match the files being reviewed. They are used for project-specific blueprints, constraints, and domain background.

### Waivers

Waivers are structured suppression rules. They first match findings by location and filters, then an LLM performs a final decision about whether the waiver applies to the specific finding.

### Model categories

The configuration maps named model categories such as `fastest_good`, `balanced`, `best_code`, and `frontier_best` to concrete provider/model settings. Personas refer to categories rather than hard-coded model names.

The aggregation path now prefers the `balanced` model category, with a compatibility fallback to `best_code` if `balanced` is not configured.

## Repository Layout

The codebase is still small and uses a flat `main` package. The important files are:

- [main.go](ai-reviewer/main.go): entrypoint and top-level pipeline execution
- [settings.go](/Users/johnlong/GolandProjects/ai-reviewer/settings.go): CLI parsing, run configuration, run planning, output/logging helpers
- [context.go](/Users/johnlong/GolandProjects/ai-reviewer/context.go): git/GitHub context extraction, diff parsing, filter matching, prompt construction
- [scanner.go](/Users/johnlong/GolandProjects/ai-reviewer/scanner.go): discovery and loading of personas, primers, and waivers
- [persona.go](/Users/johnlong/GolandProjects/ai-reviewer/persona.go): persona model and execution logic
- [primer.go](/Users/johnlong/GolandProjects/ai-reviewer/primer.go): primer model and matching
- [waiver.go](/Users/johnlong/GolandProjects/ai-reviewer/waiver.go): waiver model and evaluation
- [pipeline.go](/Users/johnlong/GolandProjects/ai-reviewer/pipeline.go): structured finding model, normalization, aggregation prompts
- [models.go](/Users/johnlong/GolandProjects/ai-reviewer/models.go): provider adapters for OpenAI, Anthropic, and Gemini
- [config.go](/Users/johnlong/GolandProjects/ai-reviewer/config.go): config loading and merging
- [git.go](/Users/johnlong/GolandProjects/ai-reviewer/git.go): repository acquisition and ref fetching

## End-to-End Flow

### 1. CLI parsing

`NewRunSettingsFromArgs` parses the subcommand and flags into `RunSettings`.

### 2. Run configuration

`NewRunConfig` is the main planning step. It:

- ensures the target repository is available locally
- fetches the required refs or commits
- resolves the target into `PRInfo`
- computes search paths for `.ai-review` content
- loads config, personas, primers, and waivers
- extracts the global review context
- filters personas into runnable and skipped groups
- initializes shared model clients used for normalization and aggregation

This function acts as the current composition root of the application.

### 3. Artifact discovery

Artifacts are loaded from two sources in precedence order:

1. Markdown files committed in the target repository at the reviewed head SHA, using explicit `ai_review` frontmatter
2. Repo-scoped local directories such as `.ai-review/<owner>/<repo>/personas`, `.ai-review/<owner>/<repo>/primers`, and `.ai-review/<owner>/<repo>/waivers`

The scanner deduplicates by artifact ID. Discovery issues are surfaced as warnings when partial results are still available.

### 4. Filtering and run planning

Before execution, each persona is evaluated against the current context. Filtering currently supports:

- path filters
- exclude filters
- regex filters over added lines
- branch filters
- function filters
- line number filters over changed lines
- date filters

Each persona gets a narrowed `PRContext` containing only the files relevant to it. Matching primers are computed against that same narrowed context.

### 5. Persona execution

Runnable personas are executed concurrently with a semaphore-limited worker pattern.

For each persona:

- a prompt is built from persona instructions, target metadata, changed files, diff or diff stats, matching primers, optional pre-run explainer output, and optional aggregated findings
- the configured model is invoked
- raw output is saved to disk

If the persona is a reviewer, its output is then normalized into structured findings.

If the persona is a pre-run explainer, its JSON output is parsed into per-file analyses and stored for later personas.

If the persona is a post-run explainer, its output is collected for the final report.

### 6. Normalization

Reviewer output is intentionally treated as unstructured text first. A cheaper model then normalizes it into `Finding` objects with:

- source persona
- file
- optional line range
- summary
- details
- severity hint
- confidence

This normalization step gives downstream code a stable structure even when reviewer prompts vary.

### 7. Waiver evaluation

Each structured finding is compared against matching waivers. Waiver filtering uses the same file-level filters as the rest of the system, plus optional line-range checks. If a waiver matches, an LLM receives the finding, the relevant diff, and waiver instructions, then returns whether the finding should be suppressed.

Waived findings are removed from the main finding set and retained separately for reporting.

### 8. Aggregation

All remaining findings are sent to the aggregator prompt, which produces the final summary in Markdown. The aggregator is responsible for:

- deduplicating similar findings
- clustering related issues
- assigning presentation severity
- preserving persona attribution

The final aggregated summary is the main user-facing review result.

### 9. Reporting

The report step assembles:

- the aggregated summary
- post-run explainer output
- token and timing stats
- cost estimates
- waived findings

It prints the report to stdout and writes run artifacts to disk.

## Prompt Architecture

Prompt construction happens centrally in `buildPrompt`. The prompt usually contains:

- persona instructions
- optional aggregated findings for post explainers
- matching primers and their intended scope
- PR metadata
- changed file list
- annotated unified diff or diff stats
- optional global instructions from config

Pre-run explainers additionally receive a strict JSON-output system prompt.

Normalization and aggregation also use dedicated system-style prompts defined in `pipeline.go`.

## Filtering Model

The filtering model is one of the more important parts of the architecture because it controls cost and signal quality.

Filtering happens in two layers:

- `GetPRContext` narrows the available file set by include, exclude, and regex filters
- `FileContext.Matches` applies full matching logic including branch, function, date, and changed-line-range filters

This means personas and primers are only run when there is concrete evidence that their scope intersects the current changes.

## Model Provider Abstraction

`ModelClient` is the common interface for all providers:

- `Generate`
- `GenerateJSON`

There are provider-specific implementations for:

- OpenAI
- Anthropic
- Gemini

This is a pragmatic abstraction rather than a deep provider framework. It normalizes just enough behavior for the pipeline to treat providers uniformly while still exposing provider-specific configuration such as reasoning effort or thinking level.

## Concurrency Model

Persona execution is the only intentionally concurrent part of the pipeline. Each stage runs as a batch, and a semaphore limits the number of simultaneous personas. The stages themselves remain ordered:

1. pre explainers
2. reviewers
3. waivers
4. aggregation
5. post explainers
6. report

This preserves the data dependencies between stages while still allowing parallel work within a stage.

## Output and Observability

Each run produces a timestamped run directory. Typical artifacts include:

- `summary.md`
- `report.md`
- `all_findings.json`
- per-persona `prompt.md`
- per-persona `raw.md`
- per-persona `findings.json` or `parsed.json`
- `stats.txt`

Additionally, run usage is appended to `run-log.jsonl`.

This artifact-first approach is one of the strongest aspects of the current architecture because it gives maintainers a way to inspect model behavior without reproducing the run from scratch.

## Current Architectural Strengths

- The domain concepts are clear and map well to the problem.
- The staged pipeline is easy to reason about.
- Prompt inputs and outputs are auditable.
- The configuration model is expressive enough for a real project like Gonka.
- The system is extensible through Markdown artifacts instead of code changes alone.

## Current Architectural Limitations

- The code is still organized as a flat `main` package, so orchestration, IO, prompting, discovery, and execution are tightly colocated.
- `NewRunConfig` currently serves as both dependency wiring and business orchestration, which makes it a central hotspot.
- Model client lifecycle is only partially centralized: normalization and aggregation share clients, while persona and waiver execution still construct provider clients on demand.
- Several important behaviors are prompt-driven rather than enforced structurally, which is common in LLM systems but increases the need for good artifacts and testing.

## Extension Points

The architecture is designed to be extended in a few clear ways:

- Add new personas by writing repo-scoped Markdown files under `.ai-review/<owner>/<repo>/personas`.
- Add new project context through repo-scoped primers under `.ai-review/<owner>/<repo>/primers`.
- Add new suppression policy through repo-scoped waivers under `.ai-review/<owner>/<repo>/waivers`.
- Add new model categories in config.
- Add new providers by implementing `ModelClient`.
- Improve reporting or normalization without changing artifact formats.

## Mental Model for Maintainers

If you are new to the repo, the easiest way to think about it is:

- `settings.go` plans the run
- `context.go` builds the reviewable world
- `scanner.go` finds the review artifacts
- `persona.go`, `primer.go`, and `waiver.go` define the review behavior
- `pipeline.go` turns model output into structured review results
- `models.go` talks to providers
- `main.go` executes the stages in order

That is the simplest accurate picture of the system in its current form.
