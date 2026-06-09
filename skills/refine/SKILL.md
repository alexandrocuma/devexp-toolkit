---
name: refine
description: Backlog refinement — takes a raw idea or stakeholder request through requirements authoring, estimation, and codebase-validated grooming into a ready-to-build ticket
---

# Backlog Refinement

You are running the **backlog refinement ceremony**. Given a vague idea, a stakeholder request, or a "we should add X," you produce a groomed, ready-to-build work item: written requirements, a complexity estimate grounded in the actual codebase, and a validated execution plan.

## Triggered by

- `/refine` — open-ended, you'll ask for the idea
- `/refine "<idea or request>"` — with inline input

## When to Use

When work begins with an idea that hasn't been structured yet. The output of `/refine` is the input to `/deliver`. If a groomed ticket already exists, skip to `/deliver`.

---

## Process

### Phase 0 — Check Orientation

```bash
ls ~/.claude/agent-memory/codebase-navigator/MEMORY.md 2>/dev/null && echo "atlas: EXISTS" || echo "atlas: MISSING"
ls graphify-out/graph.json 2>/dev/null && echo "graph: EXISTS" || echo "graph: MISSING"
```

If the atlas is missing: "This repo hasn't been oriented yet. Run `/devxp` first for more accurate grooming — or proceed with reduced codebase context. Which do you prefer?"

---

### Phase 1 — Capture the Idea

If no input was given inline, ask: "What's the idea or request? A sentence is enough."

Accept any format: a sentence, a paragraph, a bullet list, a stakeholder quote, a Slack message copy-paste.

Identify the shape:
- **Single atomic change** (one engineer, 1-3 days) → single ticket path
- **Multi-part feature** (multiple layers, multiple engineers) → epic decomposition path

---

### Phase 2 — Write Requirements

#### 2a. Anchor in the codebase

Before writing user stories, search for existing domain terms:

```bash
# Find existing concepts related to the idea
grep -rn "<key noun from the request>" --include="*.ts" --include="*.go" --include="*.py" --include="*.rb" --include="*.js" . 2>/dev/null | head -15
```

If a graphify graph exists, query it:
```bash
graphify query "<the feature area or key concept>"
```

Note: what already exists (so stories don't re-specify it), what's missing, any naming conflicts.

#### 2b. Write user stories

For each distinct user action this idea enables, write one story:

```
## Requirements: <Feature Name>

### Problem Statement
<1-3 sentences. What problem does this solve? For whom?>

### Users Affected
- Primary: <who benefits most>
- Secondary: <who is indirectly affected>
- Not in scope: <explicitly excluded — prevents scope creep>

### User Stories

#### Story 1: <Short title>
As a <role>,
I want to <action>,
so that <outcome/benefit>.

Acceptance Criteria:
- [ ] <Observable behavior — not implementation detail>
- [ ] <Another condition>
- [ ] <Error state handled gracefully>
- [ ] <Edge case that must work>

#### Story 2: ...

### Non-Goals (explicitly out of scope)
- <Thing that seems in-scope but is deferred>

### Open Questions
- [ ] <Anything needing a stakeholder answer before dev starts>
```

Rules:
- Acceptance criteria must be **testable** — "users can log in" is not testable; "given invalid credentials, the form shows an error without redirecting" is
- **Non-goals are mandatory** — every spec must say what's not included
- Use the codebase's own domain terms — don't invent new names for existing concepts

---

### Phase 3 — Estimate Complexity

Ground the estimate in evidence from the codebase:

```bash
# Count files likely to change (adapt glob to the affected area)
find . -name "*.ts" -o -name "*.go" -o -name "*.py" 2>/dev/null | grep -v node_modules | grep -v ".git" | xargs grep -l "<key concept>" 2>/dev/null | wc -l

# Check if tests exist for the affected area
find . -name "*.test.*" -o -name "*_test.*" -o -name "*.spec.*" 2>/dev/null | grep -v node_modules | xargs grep -l "<key concept>" 2>/dev/null | wc -l

# Check for recent changes in the area (complexity signal)
git log --oneline --since="90 days ago" -- <likely affected directory> 2>/dev/null | wc -l
```

Apply these thresholds:

| Size | Criteria |
|------|---------|
| **S** | 1-3 files, well-understood pattern, tests exist → < 1 day |
| **M** | 3-8 files, some discovery needed, tests partially exist → 1-3 days |
| **L** | 8+ files, new pattern or integration, test setup required → 3-5 days |
| **XL** | Cross-cutting, multiple services, unclear scope → must decompose |

Risk multipliers (bump up one size):
- Touches auth, payments, or shared data models
- Requires a new external integration
- No existing tests in the area
- The codebase hasn't touched this area in > 6 months (unknown state)

Report:
```
Estimate: <S/M/L>  (<N files likely affected>, <risk factors if any>)
```

---

### Phase 4 — Create the Work Item

Present the plan before creating anything:

```
Refinement plan:
  Requirements: N stories, N acceptance criteria
  Estimate:     <S/M/L>
  Path:         single ticket / epic (N sub-tickets)

Create ticket(s) now? (yes / adjust first)
```

Wait for confirmation.

Detect the available issue tracker:

| Priority | Signal | Platform |
|----------|--------|----------|
| 1 | `mcp__linear__*` tools present | Linear |
| 2 | `mcp__jira__*` or `mcp__atlassian__*` tools present | Jira |
| 3 | `gh auth status` succeeds | GitHub Issues |
| 4 | `glab auth status` succeeds | GitLab Issues |
| 5 | None | Output formatted markdown |

For a single ticket — create with body:
```markdown
## Problem
<from requirements>

## Acceptance Criteria
- [ ] <criterion>
- [ ] Tests written and passing

## Non-Goals
- <item>

## Estimate: <S/M/L>
```

For an epic — decompose into 3-8 atomic tickets (each S or M), map dependencies, present the critical path before creating.

---

### Phase 5 — Groom Against the Codebase

Once ticket(s) exist, invoke the `grooming-agent` to validate every claim in the ticket against the actual codebase:

> "Groom ticket <ID>. Validate all claims against the codebase. Produce a Ticket Health Report and a verified execution plan. Persist the plan to the ticket platform."

The grooming-agent will:
- Spawn `codebase-navigator` and `backend-senior-dev`/`frontend-senior-dev` to validate the ticket
- Produce a Ticket Health Report (Confirmed / Incorrect / Missing context)
- Write a verified execution plan back to the ticket

Wait for the grooming-agent to complete. If it returns BLOCKED or NEEDS TICKET CORRECTION, address the finding before proceeding.

---

### Phase 6 — Report & Hand Off

```
Refinement complete

  Requirements: N stories, N acceptance criteria
  Ticket:       <url or ID> — "<title>"
  Estimate:     <S/M/L>
  Groom status: READY TO BUILD / corrections applied

Next:
  /deliver <ticket-id>   — pick this up and build it
```

---

## Guidelines

- **Requirements describe what the user can do, not how the code works** — no implementation details in acceptance criteria
- **Non-goals are not optional** — a spec without explicit exclusions will be interpreted as "build everything adjacent"
- **Grooming is mandatory before /deliver** — an unvalidated ticket puts implementation at risk; never skip Phase 5
- **Atlas absence is a warning, not a blocker** — requirements can be written; grooming accuracy will be reduced without codebase context
