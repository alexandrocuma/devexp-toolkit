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
  1. Worktree           → isolate this ticket in its own git worktree
  2. Implement          → dev-agent (or migration agent)
  3. Instrument         → add observability to new code
  4. Test gaps          → test-gen agent
  5. Code review        → pr-review agent
  6. Release            → merge worktree + changelog + version tag + platform release  [gated]

Proceed? (yes / stop after implementation / skip release)
```

Wait for confirmation. The user can stop at any step.

---

### Phase 1.5 — Isolate the Ticket in a Worktree

Before implementing, isolate this ticket's work in its own git worktree. This follows the **worktree-per-ticket convention** documented in `docs/guides/worktree-per-ticket.md` (devexp-toolkit) — that doc is the single source of truth for the trigger, naming scheme, lifecycle, and merge discipline summarized here. Phases 2–5 (implement, instrument, test, review) all run **inside** this worktree; the main checkout is never mutated during delivery.

**Create the worktree** on a fresh branch derived from the ticket id — branch `<type>/<ticket-id>`, directory a sibling of the main checkout so it is never scanned or committed into the primary tree:

```bash
# Run from the main checkout. Branch and directory are both derived from the ticket id.
ticket="<ticket-id>"; type="feat"   # feat | fix | docs | chore — match the ticket
repo="$(basename "$(git rev-parse --show-toplevel)")"
wt="../${repo}-worktrees/${ticket}"
git worktree add -b "${type}/${ticket}" "$wt" 2>/dev/null || git worktree add "$wt" "${type}/${ticket}"
cd "$wt"
```

All subsequent phases operate from this worktree directory.

**Epic sub-tickets:** each independent sub-ticket gets its own worktree on its own branch. Because the worktrees don't share files, sub-tickets with no dependency between them can be delivered in parallel — that parallelism is a free byproduct, not something to request explicitly.

**Single-stream fallback:** if the environment can't support worktrees (no git, or a shallow/non-worktree-capable checkout) or the user explicitly wants the change applied to the current tree, skip worktree creation and deliver in place on the current branch. The merge step in Phase 6 then becomes a no-op; the same merge discipline still applies to whatever integration happens.

---

### Phase 2 — Implement

**Scan for infrastructure changes first:**

```bash
# Detect IaC definition files in the working tree and ticket scope
git diff --name-only HEAD 2>/dev/null | grep -iE "\.(tf|hcl|yaml|yml|json|toml)$" | head -10
find . -maxdepth 3 \( -name "*.tf" -o -name "*.hcl" -o -name "Dockerfile" -o -name "docker-compose*" -o -name "k8s" -o -name "helm" \) 2>/dev/null | grep -v ".git" | grep -v node_modules | head -5
```

If the ticket or changed files include infrastructure definitions, apply the same discipline as application code: read 2–3 existing infrastructure files first, match the project's existing style exactly, and handle them within this implementation step. Never introduce a new IaC tool or pattern without user confirmation.

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

**3d. Surface SLO candidates:**

For each new or modified critical path identified above (API handler, background job, external call, DB write), note the natural SLI attachment point — what would a team measure here (latency, error rate, throughput, queue depth)? List these as candidates in the Phase 7 report. Do not create dashboards, alert configs, or metric code — just surface what exists as observable points so the team can wire them up.

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

**E2E coverage check:**

After unit/integration test gaps are filled, check for E2E coverage:

```bash
# Detect whether the project has an E2E test suite
find . -maxdepth 4 -type d \( -name "e2e" -o -name "integration" -o -name "cypress" -o -name "playwright" -o -name "tests" \) 2>/dev/null | grep -v node_modules | grep -v ".git" | head -5
find . -maxdepth 4 -name "*.spec.*" -o -name "*.e2e.*" -o -name "*_test.*" 2>/dev/null | grep -v node_modules | grep -v ".git" | grep -iE "e2e|integration|spec" | head -5
```

- If an E2E suite exists: check whether the user flows touched by this ticket have corresponding E2E scenarios. If gaps exist, read 2–3 existing E2E test files to learn the project's conventions, then generate scenarios that cover the changed flows.
- If no E2E suite exists: note the gap in the Phase 7 report. Do not scaffold an E2E framework unilaterally.

**Regression check:**

After tests are filled, verify no existing tests newly fail due to the changes:

```bash
git diff --name-only HEAD 2>/dev/null | head -10
```

Run the test suite for all packages containing changed files. If any pre-existing tests now fail, fix the regression before proceeding — do not move to code review with a failing test suite.

**Performance-sensitive path — load test offer:**

Scan the changed files for new or modified API endpoints:

```bash
git diff --name-only HEAD 2>/dev/null | xargs grep -l "Handler\|router\.\|app\.get\|app\.post\|@app\.\|@router\." 2>/dev/null | head -5
```

If new or modified endpoints are detected, check whether the project has a load testing framework:

```bash
find . -maxdepth 4 \( -name "*.k6.js" -o -name "locustfile*" -o -name "artillery*" -o -name "*.gatling.*" \) 2>/dev/null | grep -v node_modules | head -3
```

If a framework exists: offer to generate load test scenarios (smoke / load / stress) for the new endpoints. If the user confirms, read 1-2 existing test files to learn the format, then generate scenarios. If no framework exists: note the gap in the Phase 7 report.

---

### Phase 5 — Code Review

**Pre-review correctness and quality pass:**

Before invoking the automated PR review, run a targeted correctness check on the changed code. Read each changed file and scan for:

- **Unhandled error paths** — `err` returned from a function call and not checked or propagated
- **Null/nil dereferences** — variables used without nil/null guards after potentially-nil operations
- **Off-by-one errors** — loop bounds, slice operations, pagination math
- **Resource leaks** — files, DB connections, HTTP responses opened but not closed in all paths
- **Race conditions** — shared mutable state accessed from goroutines or async paths without synchronization

Fix any issues found before proceeding — these are correctness bugs, not style choices.

**Quality pass** — scan changed files for:
- Functions longer than 50 lines that could be cleanly extracted
- Logic duplicated elsewhere in the project that could be reused
- Magic numbers or strings that should be named constants
- Missing input validation at public function boundaries

Document any quality findings in the PR description for the reviewer. Fix only what's clearly wrong; note the rest as follow-up debt.

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
  Step 1: Merge the ticket worktree into the base branch
  Step 2: Generate changelog entry from commits
  Step 3: Bump version and create git tag
  Step 4: Publish release on the detected platform
  On success: remove the worktree (kept automatically if any step fails)

Confirm release? (yes / no — I'll release manually)
```

Wait for explicit **yes**. If the user says no or wants to release manually, stop here.

**6a. Merge the ticket worktree:**

Integrate the ticket's branch into the base branch through the project's normal path — merge the open PR, or a direct merge if no PR workflow is used. Merges are **serialized**: if other worktrees are also ready, merge them one at a time so each sees a consistent base. If the merge reports a **conflict, stop and surface it to the user — never auto-resolve.**

```bash
git worktree list   # confirm which worktree/branch belongs to this ticket
```

On a **successful** merge and release, remove the worktree and its branch — the toolkit cleans up after itself:

```bash
git worktree remove "../$(basename "$(git rev-parse --show-toplevel)")-worktrees/<ticket-id>"
git branch -d "<type>/<ticket-id>" 2>/dev/null
```

On **failure** at any release step, **keep the worktree** for inspection — never remove it. (Skip this whole step entirely if delivery ran in the single-stream fallback with no worktree.)

**6b. Generate changelog:**

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

**6c. Bump version:**

Detect the versioning file:
```bash
ls package.json go.mod pyproject.toml Cargo.toml version.go VERSION 2>/dev/null | head -3
```

Apply the appropriate bump (patch for fixes, minor for features, major for breaking changes — infer from commit types). Update the version file.

**6d. Tag and publish:**

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

  Worktree:        <branch/dir created; merged + removed on release / kept on failure / single-stream, none>
  Implementation:  complete — <N files changed>
  Infrastructure:  <N IaC files changed / no infrastructure changes detected>
  Observability:   <N log calls added / skipped — no new entry points>
  SLO candidates:  <list of critical paths with suggested SLI measurement points, or "none identified">
  Tests:           <N unit/integration tests added / already covered>
  E2E coverage:    <N scenarios added / no E2E suite detected / already covered>
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
- **Worktree isolation is the default, merge is deferred** — each ticket is delivered in its own worktree (`docs/guides/worktree-per-ticket.md`); the branch merges only at the release gate, and conflicts always surface to the user — never auto-resolve them
