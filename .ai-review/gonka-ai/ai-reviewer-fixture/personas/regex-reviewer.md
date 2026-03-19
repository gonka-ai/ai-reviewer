---
id: regex-reviewer
ai_review: persona
model_category: balanced
path_filters:
  - "src/api/**/*.go"
regex_filters:
  - "TODO"
  - "admin"
---
Repo-specific override of the shared regex reviewer. Focus only on API changes with TODOs or admin logic.
