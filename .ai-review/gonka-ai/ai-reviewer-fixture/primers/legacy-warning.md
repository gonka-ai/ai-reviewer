---
id: legacy-warning
ai_review: primer
type: legacy
path_filters:
  - "pkg/legacy/**"
regex_filters:
  - "subnet"
---
Legacy subnet matching code is brittle. Prefer minimal edits and call out behavior changes clearly.
