# AI Reviewer

A single-binary Go CLI that reviews a GitHub PR using AI personas.

## Usage

```bash
go build -o ai-review
# Review a PR
./ai-review pr <repo_owner>/<repo_name> <pr_number> [--max-tokens <int>] [--concurrency <int>] [--dry-run]

# Review a specific commit (compared to its parent by default)
./ai-review commit <repo_owner>/<repo_name> <commit_hash> [--compare-to <hash>] [--max-tokens <int>] [--concurrency <int>] [--dry-run]

# Review specific files on a branch
./ai-review file <repo_owner>/<repo_name> <branch_name> <file_pattern...> [--max-tokens <int>] [--concurrency <int>] [--dry-run]

# Review the diff between two branches
./ai-review branches <repo_owner>/<repo_name> <base_branch> <head_branch> [--max-tokens <int>] [--concurrency <int>] [--dry-run]
```

### Global CLI Options

- `--max-tokens <n>`: Override the maximum tokens for AI responses.
- `--concurrency <n>`: Set the maximum number of personas to run concurrently (default: 3).
- `--dry-run`: Scan and report what personas and primers will be applied, but do not execute any AI calls. Useful for testing configuration and filtering logic without incurring costs.

### Examples:
```bash
# Review PR #1234
./ai-review pr google/go-github 1234 --max-tokens 500

# Review commit abc1234
./ai-review commit google/go-github abc1234

# Review commit abc1234 compared to def5678
./ai-review commit google/go-github abc1234 --compare-to def5678

# Review all .go files in config directory on master branch
./ai-review file google/go-github master "config/*.go"

# Review comparison between master and feature branches
./ai-review branches google/go-github master feature

# Dry run to see what would be executed for PR #1234
./ai-review pr google/go-github 1234 --dry-run
```

## Setup

1. Install Go 1.25+
2. Install GitHub CLI (`gh`) and authenticate: `gh auth login`
3. Install Git
4. Set up environment variables for AI providers:
   - `OPENAI_API_KEY`
   - `ANTHROPIC_API_KEY`
   - `GEMINI_API_KEY`

## Configuration

The tool expects a `.ai-review` directory. Configuration and personas can be global (in `.ai-review/`) or specific to a repository (in `.ai-review/<repo_owner>/<repo_name>/`). Repository-specific configurations take precedence.

### Model Mapping

Create `.ai-review/<repo_owner>/<repo_name>/config.yaml`. You can define multiple model categories and their pricing for cost estimation:

```yaml
model_mapping:
  fastest_good:
    provider: gemini
    model: gemini-3-flash-preview
    max_tokens: 16384
    reasoning_level: low    # optional: none | low | medium | high
    # reasoning_level maps to:
    # - Gemini: ThinkingLevel (low | medium | high)
    # - OpenAI: ReasoningEffort (low | medium | high)
    # - Anthropic: Extended Thinking (budget is automatically calculated from max_tokens)
    input_price_per_million: 0.50
    output_price_per_million: 3.00
  balanced:
    provider: gemini
    model: gemini-2.5-flash
    max_tokens: 4096
    input_price_per_million: 0.30
    output_price_per_million: 2.50

primer_types:
  blueprint:
    description: "Architectural blueprints that must be followed for new or modified components."
  constraint:
    description: "Strict constraints or rules that apply to the codebase."
```

`max_tokens` here sets the default limit for the category.

### Discovery and Organization

The tool scans for personas and primers in multiple locations:

- **Dedicated Directories**: Any `.md` file within `.ai-review/personas/`, `.ai-review/primers/`, `.ai-review/waivers/`, or their repository-specific counterparts (e.g., `.ai-review/<owner>/<repo>/personas/`) is automatically loaded. All subdirectories are searched recursively, allowing you to organize them by purpose or to mirror the structure of the project itself. Files in these directories do **not** require an `ai_review` field in their frontmatter.
- **Repository-wide Scanning**: The tool also scans all `.md` files in the repository branch being evaluated. A file is included as a persona, primer, or waiver if it contains an explicit `ai_review: persona`, `ai_review: primer`, or `ai_review: waiver` field in its YAML frontmatter. This allows you to keep these artifacts alongside the code they relate to.

### Personas

Create persona files in the dedicated personas directories or anywhere in your repository (with the `ai_review: persona` field). Personas support several fields in their YAML frontmatter:

```markdown
---
id: security
role: reviewer           # optional: reviewer (default) | explainer
stage: pre              # optional: pre | post (only for explainers)
include_findings: true  # optional: include the aggregated summary report (only for post-run explainers)
include_explainers: ["state-modified"] # optional: list of pre-run explainer IDs to include their analysis for files
exclude_diff: true      # optional: exclude the full unified diff and show stats instead
model_category: best_code
max_tokens: 4096        # optional: overrides model category limit
path_filters:           # optional: only run if these files changed
  - "inference-chain/**/*.go"
exclude_filters:        # optional: ignore these files
  - "**/*_test.go"
regex_filters:          # optional: only include files where changed lines match any of these regexes
  - "TODO"
branch_filters:         # optional: only apply to specific branch globs
  - "main"
  - "release/*"
function_filters:       # optional: only apply if specific functions are modified
  - "ProcessData"
line_numbers_filter:    # optional: list of line ranges
  - start: 10
    end: 20
date_filter: "2025-01-01" # optional: only apply if commit date is BEFORE this date
---
You are a security expert. Review the following PR for security vulnerabilities.
```

#### Roles and Stages

- **Reviewer**: (Default) Analyzes the code and produces findings. Findings are automatically normalized into structured data and later aggregated.
- **Explainer (Pre)**: Runs before reviewers. Must output JSON (file-to-analysis mapping). Its analysis is injected into the context of all subsequent personas for that file.
- **Explainer (Post)**: Runs after reviewers. Its full output is included in the final report under an "Explanations" section. Useful for providing human-readable guides or high-level summaries.

### Primers

Primers provide extra context, constraints, or blueprints to personas based on the specific files they are analyzing. They are included in the persona prompt only if the persona is analyzing files that match the primer's filters.

Create primer files in the dedicated primers directories or anywhere in your repository (with the `ai_review: primer` field). They support the same filtering fields as personas (`path_filters`, `exclude_filters`, `regex_filters`, `branch_filters`, `function_filters`, `line_numbers_filter`, `date_filter`):

```markdown
---
id: inference-chain-blueprint
type: blueprint
path_filters:
  - "inference-chain/**/*.go"
---
When modifying the inference chain, ensure that you follow the established patterns:
1. ...
```

The `type` field matches the types defined in `config.yaml` to provide additional intent to the AI.

### Waivers

Waivers allow you to automatically suppress specific findings based on predefined rules. This is useful for ignoring known issues, legacy code patterns, or false positives.

Create waiver files in `.ai-review/waivers/` or anywhere in your repository (with the `ai_review: waiver` field). Waivers use the same filters as Personas and Primers to determine their applicability to a finding's location.

```markdown
---
id: ignore-legacy-auth
model_category: fastest_good
path_filters:
  - "legacy/auth/**/*.go"
date_filter: "2024-01-01"
---
We are aware of the weak hashing in the legacy auth module, but it is scheduled for decommissioning and should not be flagged in new PRs unless the logic is significantly altered.
```

#### How Waivers Work

1. **Location Filtering**: When a reviewer produces a finding, the tool checks for any Waivers whose filters (`path_filters`, `branch_filters`, etc.) match the finding's location.
2. **LLM Validation**: If a waiver's location matches, the tool sends the finding's details, the relevant code diff, and the waiver's instructions to an LLM (specified by `model_category`).
3. **Decision**: The LLM determines if the waiver truly applies to this specific issue.
4. **Reporting**: Waived issues are excluded from the main sections of the report ("Must Fix", etc.) and listed in a separate "Waived Issues" section at the end of the report.

#### Token Limit Precedence

The maximum tokens for a response is determined by (highest priority first):
1. `--max-tokens` CLI flag
2. `max_tokens` in persona frontmatter
3. `max_tokens` in `config.yaml` model mapping

## Repository Storage

By default, the tool clones repositories into a `.repos` directory relative to the current working directory. This directory is organized by owner and repository name (e.g., `.repos/google/go-github`). If you are already inside the target repository (or a subdirectory of it), the tool will use it directly instead of cloning.

A `.gitignore` file is provided in the project root to ensure that the `.repos` directory and the compiled `ai-review` binary are not tracked by version control.

## How it works

The tool executes a multi-stage pipeline:

1. **Fetch Context**: Uses `gh` CLI and `git` to fetch PR details and compute the unified diff.
2. **Pre-run Explainers**: Executes personas with `role: explainer` and `stage: pre`. They provide initial research that is injected into later prompts.
3. **Reviewers**: Executes standard personas. If any **Primers** match the files being analyzed by a persona, they are injected into its prompt as extra context. Each reviewer's raw output is immediately processed by a **Normalization** step (using a cheap model) to extract structured findings (file, line, summary, severity).
4. **Waiver Evaluation**: Any findings produced by reviewers are checked against applicable **Waivers**. If a waiver matches the location and is confirmed by an LLM, the finding is marked as waived.
5. **Post-run Explainers**: Executes personas with `role: explainer` and `stage: post`. They provide high-level context or human instructions.
6. **Aggregation**: All non-waived findings from all reviewers are sent to an **Aggregator** LLM (using the `balanced` model). It deduplicates issues, clusters related findings, and produces a concise Markdown summary.
7. **Reporting**: The final report is printed to stdout and saved to the run directory. Waived findings are listed in a separate section.

## Output and Artifacts

Each run generates a timestamped directory in `.ai-review/<repo_owner>/<repo_name>/runs/<target_id>/<timestamp>/` containing:
- `target_id` is the PR number (for PR reviews), the short commit hash (for commit reviews), `file-<branch_name>` (for file reviews), or `branches-<target_repo>` (for branch reviews).
- `summary.md`: The aggregated Markdown summary.
- `report.md`: The full report including explanations and stats.
- `all_findings.json`: All normalized findings from all personas.
- `persona_name/`: Subdirectories for each persona containing their `raw.md` output and `findings.json` (or `parsed.json`).

Stats and token usage are also appended to `.ai-review/<repo_owner>/<repo_name>/run-log.jsonl`.

## Cost Tracking

The final report includes a "Stats" section with:
- Token usage (In/Out) per persona and pipeline step.
- Estimated cost per step based on prices in `config.yaml`.
- Total estimated cost for the run.
- Usage summary grouped by model.
