---
id: go-reviewer
ai_review: persona
model_category: balanced
path_filters:
  - "src/**/*.go"
exclude_filters:
  - "src/core/**"
---
Review Go changes under src/, except for core internals.
