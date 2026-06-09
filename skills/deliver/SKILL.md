---
name: deliver
description: Development lifecycle orchestrator — takes a groomed ticket through implementation, observability, testing, code review, and release. Self-contained: no other skills required.
---

# Deliver: Ticket → Production

You are running the **delivery phase** of the development cycle. Given a groomed ticket, you take it all the way to production: implement, instrument, test, review, and release. Every heavy-lifting step delegates to a specialist agent; you orchestrate the sequence and ensure nothing is skipped.

## Triggered by

- `/deliver` — infer ticket from branch name or open PRs
- `/deliver <ticket-id>` — target a specific ticket

## When to Use

When a ticket exists and is ready for implementation. The ticket should have a groom plan (from `/refine` or a prior `/groom` run). If no groom plan exists, this skill runs grooming first. If the ticket doesn't exist yet, run `/refine` first.

---

## Process

### Phase 0 — Load Ticket and Groom Plan

**Detect the ticket:**

```bash
git branch --show-current 2>/dev/null
gh pr view 2>/dev/null | head -5 || glab mr view 2>/dev/null | head -5
ls ~/.claude/agent-memory/grooming-agent/plans/ 2>/dev/null
```

Infer the ticket ID from: branch name (patterns like `feat/PAY-123`, `fix/WFM-456`), open PR title, or user-provided argument.

**Check for a groom plan:**

| State | Action |
|-------|--------|
| Groom plan found for this ticket | Load it as the implementation blueprint |
| No plan, ticket exists | Run `grooming-agent` before proceeding (see below) |
| No ticket, no plan | Tell user to run `/refine` first |

If the groom plan is missing: invoke the `grooming-agent`:
> "Groom ticket <ID>. Validate all claims against the codebase. Produce a Ticket Health Report and verified execution plan. Persist to the ticket platform."

Wait for the grooming-agent to complete before proceeding.

---

### Phase 1 — Assess and Present the Delivery Plan

From the groom plan, classify the implementation type:

| Signal | Implementation approach |
|--------|------------------------|
| New feature, multi-file | `dev-agent` (autonomous) |
| Library or framework upgrade | `migration` agent |
| Bug fix | `dev-agent` with bugfix context |
| Complex cross-cutting change | Suggest `tech-lead` architecture review first |

Present the plan:

```
## Delivering: <ticket-id> — "<title>"

Groom plan loaded:
  Files to change:  N
  Steps:            N
  Risk:             low / medium / high

Delivery sequence:
  1. Implement          → dev-agent (or migration agent)
  2. Instrument         → add observability to new code
  3. Test gaps          → test-gen agent
  4. Code review        → pr-review agent
  5. Release            → changelog + version tag + platform release  [gated]

Proceed? (yes / stop after implementation / skip release)
```

Wait for confirmation. The user can stop at any step.

---

### Phase 2 — Implement

**For new features and bug fixes** — invoke the `dev-agent`:

> "Implement ticket <ID>. The verified execution plan is: <paste groom plan key steps and file list>. Follow the plan exactly. Write tests alongside the implementation. Match existing code conventions."

**For library/framework upgrades** — invoke the `migration` agent:

> "Execute this migration: <library> from <version> to <version>. The groom plan shows: <relevant breaking changes and affected files>."

**For PR feedback** — invoke the `pr-feedback` agent:

> "Implement the review comments on PR <number>. Address each inline comment from the reviewer."

Wait for the implementation agent to complete.

---

### Phase 3 — Instrument

After implementation, detect and fill observability gaps. **Do this inline — no specialist skill needed.**

**3a. Detect existing observability stack:**

```bash
# Detect logger
grep -rn "import\|require\|from" . 2>/dev/null | grep -v node_modules | grep -iE "logger|winston|pino|bunyan|zap|slog|zerolog|logrus|structlog" | head -5

# Find canonical logging example
grep -rl "logger\.\|log\." . 2>/dev/null | grep -v node_modules | grep -v test | head -3
```

Read one of the found files to extract the canonical log call pattern (import style, field format, log levels used).

**3b. Scan the new/changed code for instrumentation gaps:**

```bash
# Find catch blocks without logs in changed files
git diff --name-only HEAD 2>/dev/null | xargs grep -n "catch\|rescue\|except" 2>/dev/null

# Find entry points without logs in changed files
git diff --name-only HEAD 2>/dev/null | xargs grep -n "func.*Handler\|router\.\|app\.get\|app\.post\|@app\.route" 2>/dev/null
```

**3c. Add missing log calls** following the canonical pattern exactly:
- Entry points → log at `info` level: operation name + key input identifiers (never log credentials or full request bodies)
- Error paths → log at `error` level: error message + error cause + key identifiers
- Silent state changes (DB writes, external API calls) → log at `info` level: operation + outcome

If no logging library is detected in the project, note it and skip — do not add a logging dependency unilaterally.

---

### Phase 4 — Fill Test Gaps

Check test coverage of the changed code:

```bash
git diff --name-only HEAD 2>/dev/null | head -10
```

If the implementation agent wrote tests (most do), verify they exist:

```bash
# Check for test files alongside changed files
git diff --name-only HEAD 2>/dev/null | while read f; do
  base="${f%.*}"
  ls "${base}.test."* "${base}_test."* "${base}.spec."* 2>/dev/null && echo "COVERED: $f" || echo "UNCOVERED: $f"
done 2>/dev/null
```

If uncovered files exist — invoke the `test-gen` agent:

> "Generate tests for these files: <list>. Match the project's existing test framework and conventions. Focus on the acceptance criteria from ticket <ID>: <list criteria>."

---

### Phase 5 — Code Review

Create a PR if one doesn't exist:

```bash
gh pr view 2>/dev/null || gh pr create --title "<ticket-id>: <title>" --body "Closes <ticket-id>" 2>/dev/null
glab mr view 2>/dev/null || glab mr create --title "<ticket-id>: <title>" --description "Closes <ticket-id>" 2>/dev/null
```

Invoke the `pr-review` agent:

> "Review PR <number> on branch <branch>. Focus on: (1) correctness against the acceptance criteria from ticket <ID>, (2) security implications if the change touches auth or data handling, (3) whether the implementation matches the project's existing patterns. Post findings as inline review comments."

Wait for the pr-review agent. If it posts findings, address them — either via `pr-feedback` agent or manually — before proceeding to release.

---

### Phase 6 — Release  *(gated — requires explicit confirmation)*

Before running anything, confirm:

```
PR is approved and tests pass.

Ready to release?
  Step 1: Generate changelog entry from commits
  Step 2: Bump version and create git tag
  Step 3: Publish release on GitHub/GitLab

Confirm release? (yes / no — I'll release manually)
```

Wait for explicit **yes**. If the user says no or wants to release manually, stop here.

**6a. Generate changelog:**

```bash
# Get commits since last tag
git log $(git describe --tags --abbrev=0 2>/dev/null)..HEAD --oneline 2>/dev/null | head -20
```

Group commits by type (feat, fix, perf, refactor, docs, chore). Write a changelog entry:

```markdown
## [<new-version>] — <date>

### Features
- <feat: description from commit message>

### Bug Fixes
- <fix: description>

### Performance
- <perf: description>
```

Prepend this entry to `CHANGELOG.md` (create it if it doesn't exist).

**6b. Bump version:**

Detect the versioning file:
```bash
ls package.json go.mod pyproject.toml Cargo.toml version.go VERSION 2>/dev/null | head -3
```

Apply the appropriate bump (patch for fixes, minor for features, major for breaking changes — infer from commit types). Update the version file.

**6c. Tag and publish:**

```bash
git add CHANGELOG.md <version-file>
git commit -m "chore: release v<version>"
git tag -a "v<version>" -m "Release v<version>"
git push && git push --tags
```

Publish the release on the detected platform:
```bash
# GitHub
gh release create "v<version>" --title "v<version>" --notes "<changelog entry>" 2>/dev/null

# GitLab
glab release create "v<version>" --name "v<version>" --notes "<changelog entry>" 2>/dev/null
```

---

### Phase 7 — Report & Hand Off

```
## Delivered: <ticket-id> — "<title>"

  Implementation:  complete — <N files changed>
  Observability:   <N log calls added / skipped — no new entry points>
  Tests:           <N tests added / already covered>
  Review:          <findings addressed / approved>
  Release:         v<version> published / pending manual release

Next:
  /improve   — run a health check now that new code is live
```

---

## Guidelines

- **Groom plan is the blueprint** — pass it to `dev-agent`; the agent should not re-derive what grooming already established
- **Instrumentation is inline, not delegated** — detecting and adding log calls is straightforward enough to do here; a specialist skill is not required
- **Release is the only hard gate** — every other step can be skipped; release requires explicit confirmation because it's irreversible and affects shared systems
- **Architecture check is a suggestion, not a gate** — surface it for high-complexity tickets; never block on it
- **Test coverage, not test count** — if the implementation agent wrote tests, verify they cover the acceptance criteria, not just that they exist
