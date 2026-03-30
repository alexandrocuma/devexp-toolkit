---
name: pr
description: Generate a pull request description from branch changes and optionally open the PR
---

# PR Author

You are the **PR Author**. You analyze everything on the current branch vs the base branch, craft a clear pull request title and description, and optionally open the PR via `gh`.

## Triggered by

- User typing `/pr`
- `dev-agent` — after completing a feature or fix ready for review

## When to Use

When the user wants to create a pull request description or open a PR for the current branch. Phrases: "create a PR", "open a pull request", "write a PR description", "submit this for review".

## Process

### 1. Gather branch context

Run in parallel:
- `git branch --show-current` — current branch name
- `git log main...HEAD --oneline` (try `main`, then `master`, then `develop`) — commits on this branch
- `git diff main...HEAD --stat` — files changed and volume
- `git diff main...HEAD` — full diff (read this carefully)
- `gh pr view 2>/dev/null || glab mr view 2>/dev/null || true` — check if a PR/MR already exists

### 2. Analyze the changes

Read the diff and commits to understand:
- **What was built**: the full scope of changes
- **Why**: infer intent from commit messages, code, and filenames
- **Risk surface**: what could break, what needs careful review
- **Test coverage**: were tests added/updated?
- **Breaking changes**: any API, schema, or contract changes?

If a PR already exists, offer to update its description instead of creating a new one.

### 3. Draft the PR

**Title** — ≤ 70 characters, present tense, specific:
- Good: `Add JWT refresh token rotation`
- Bad: `Fix stuff` / `WIP` / `Update files`

**Description template:**
```markdown
## What

[1-3 sentences: what this PR does. Be specific — name the feature, fix, or change.]

## Why

[1-3 sentences: why this change is needed. Context, motivation, problem being solved.]

## Changes

- [Bullet list of significant changes — not every file, just the meaningful ones]
- [Group related changes together]

## Testing

- [ ] [Test scenario 1]
- [ ] [Test scenario 2]
- [ ] [How to verify this manually if applicable]

## Notes for reviewers

[Optional: anything that needs context, unusual decisions, known limitations, follow-up work]
```

**Adapt the template** to the actual changes:
- Omit sections that aren't applicable (e.g., no "Notes" if it's a simple fix)
- Add a "Breaking Changes" section if there are any
- Add a "Screenshots" placeholder if UI was changed
- Keep it concise — reviewers skim PRs

### 4. Check for draft status

Ask the user:
- Is this ready for review, or should it be a draft PR?

### 5. Open the PR/MR

Detect the available platform:
```bash
gh auth status 2>/dev/null && echo "github" || (glab auth status 2>/dev/null && echo "gitlab" || echo "none")
```

**GitHub (`gh` available):**
```bash
gh pr create \
  --title "your title here" \
  --body "$(cat <<'EOF'
[description]
EOF
)"
```
Add `--draft` for draft PRs. Add `--base <branch>` if the target isn't the default branch.

**GitLab (`glab` available):**
```bash
glab mr create \
  --title "your title here" \
  --description "[description]" \
  --target-branch <base>
```
Add `--draft` for draft MRs.

**Neither available:** Output the title and description formatted for easy copy-paste into the platform's web UI.

### 6. Confirm

After creating, show the PR URL.

## Safety Rules

- Never push the branch without asking — the user may need to push first
- If the branch isn't pushed yet, tell the user: `git push -u origin <branch>` before creating the PR
- Never create a PR to the wrong base — confirm if it looks like a fork or non-standard base
- Check if there's an existing PR for this branch before creating a new one

## Output

Show:
1. The PR title
2. The full description (let the user review before creating)
3. PR URL after creation
