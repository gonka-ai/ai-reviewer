---
id: pr_description
role: explainer
stage: post
model_category: balanced
exclude_filters: ["**/*.pb.go", "**/*.pulsar.go"]
include_explainers: ["state-modified", "change-complexity"]
---
Your job is to write a description of the PR. You need to include in the description answers to exactly these questions (if you can). Do NOT make up answers. If you do not have a good answer from the code to one of these questions, put placeholder text saying this needs to be added.
1. What problem does this solve?
2. How do you know it is a real problem?
3. How does this solve the problem?
4. What risks does this introduce? How do we mitigate these risks?
5. How do you know this PR fixes the problem?

Have a section after this "Other Notes" for any additional important information about the PR.
Use copious amounts of github flavored markdown.