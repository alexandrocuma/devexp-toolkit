---
name: changelog
description: Generate a changelog entry from git history using conventional commits
---

# Changelog Generator

You are generating a **changelog** from git commit history. You parse conventional commits, group them by type, and produce a properly formatted changelog entry in Keep a Changelog format.

## Triggered by

- `changelog` agent — for full changelog generation workflows
- `/changelog` — generate for unreleased changes since last tag
- `/changelog v1.3.0` — generate for a specific version

## Process

### 1. Determine the range

**No version specified** (unreleased changes):
```bash
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
if [ -z "$LAST_TAG" ]; then
  git log --format="%H%n%s%n%b%n---COMMIT---" --no-merges
else
  git log "$LAST_TAG..HEAD" --format="%H%n%s%n%b%n---COMMIT---" --no-merges
fi
```

**Specific version specified** (e.g., `/changelog v1.3.0`):
```bash
PREV=$(git tag --sort=-version:refname | grep -A1 "v1.3.0" | tail -1)
git log "$PREV..v1.3.0" --format="%H%n%s%n%b%n---COMMIT---" --no-merges
```

### 2. Parse commits

For each commit, extract:
- **Type**: the conventional commit prefix (`feat`, `fix`, `perf`, `refactor`, `docs`, `chore`, `ci`, `test`, `style`)
- **Breaking**: does the subject end with `!` or does the body contain `BREAKING CHANGE:`?
- **Scope**: content inside `()` if present — `feat(auth):` → scope is `auth`
- **Description**: everything after the `:` prefix
- **SHA**: first 7 characters of the commit hash
- **PR/Issue references**: any `#NNN` in the message

Classification:

| Prefix | Changelog section | Include? |
|--------|-------------------|---------|
| `feat!` / `BREAKING CHANGE` | Breaking Changes | Always |
| `fix!` | Breaking Changes | Always |
| `feat` | Features | Always |
| `fix` | Bug Fixes | Always |
| `perf` | Performance | Always |
| `refactor` | (omit unless significant) | Only if scope indicates user impact |
| `docs` | Documentation | Always |
| `chore`, `ci`, `test`, `style` | (omit) | No |

### 3. Format the entry

**Keep a Changelog format:**

```markdown
## [Unreleased]
```
or
```markdown
## [X.Y.Z] - YYYY-MM-DD
```

Sections in this order (omit empty sections):

```markdown
### Breaking Changes
- **feat(auth)!: remove API key authentication** — API keys are no longer accepted. Use OAuth2 tokens. ([abc1234](../../commit/abc1234))

### Features
- **feat(payments):** add Stripe webhook handling ([#142](../../issues/142)) ([def5678](../../commit/def5678))

### Bug Fixes
- **fix(auth):** resolve token refresh race condition ([ghi9012](../../commit/ghi9012))

### Performance
- **perf(db):** index users.email for faster login queries ([jkl3456](../../commit/jkl3456))

### Documentation
- **docs:** update authentication guide ([mno7890](../../commit/mno7890))
```

Rules:
- Breaking Changes always first
- Each line: `- **<type>(<scope>):** <description> ([<short-sha>](<commit-url>))`
- Include issue links when present: `([#N](<issue-url>))`
- Omit scope when there is none: `- **feat:** <description>`

### 4. Determine version bump (if no version specified)

Based on the commits:
- Any breaking change → **major** bump
- Any `feat` (no breaking) → **minor** bump
- Only `fix`/`perf`/`docs` → **patch** bump

```bash
CURRENT=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
```

Report: "Based on these commits, the recommended next version is **vX.Y.Z** (minor bump due to new features)."

### 5. Update CHANGELOG.md

Read existing CHANGELOG.md if it exists. Insert the new entry:
- After the `# Changelog` header
- Before any existing `## [` entries
- If an `## [Unreleased]` section exists, replace it with the versioned entry

If no CHANGELOG.md exists, create it:

```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

<new entry here>
```

### 6. Report

```
Changelog updated: CHANGELOG.md

Range: v1.2.0..HEAD (or "all unreleased commits")
Commits processed: N
  Breaking changes: N
  Features: N
  Bug fixes: N
  Performance: N
  Documentation: N
  Omitted: N

Recommended version: vX.Y.Z (patch/minor/major)
Run /release to handle the full release workflow.
```

## Edge Cases

- **No conventional commits**: include commits verbatim under an "Other Changes" section, note that conventional commit format is not being used
- **Empty range** (nothing since last tag): report "No changes since last release" — do not create an empty section
- **First release** (no prior tags): process all commits as the initial release
