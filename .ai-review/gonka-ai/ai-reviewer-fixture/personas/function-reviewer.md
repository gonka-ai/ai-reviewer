---
id: function-reviewer
ai_review: persona
model_category: balanced
path_filters:
  - "src/api/**/*.go"
function_filters:
  - "HandleAdminExport"
---
Only review diffs that add or modify the HandleAdminExport function.
