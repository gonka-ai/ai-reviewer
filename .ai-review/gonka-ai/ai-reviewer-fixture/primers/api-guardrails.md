---
id: api-guardrails
ai_review: primer
type: api
path_filters:
  - "src/api/**/*.go"
---
Keep request validation explicit and avoid introducing privileged handlers without a clear guard.
