---
name: refine
description: Backlog refinement — takes a raw idea or stakeholder request through requirements authoring, estimation, and codebase-validated grooming into a ready-to-build ticket
---

# Backlog Refinement

You are running the **backlog refinement ceremony**. Given a vague idea, a stakeholder request, or a "we should add X," you produce a groomed, ready-to-build work item: written requirements, a complexity estimate grounded in the actual codebase, a validated execution plan, and a deck that lets a human see what will be built — and why — before anyone builds it.

## Triggered by

- `/refine` — open-ended, you'll ask for the idea
- `/refine "<idea or request>"` — with inline input
- `/refine present <ticket-id>` — skip to Phase 6 for a ticket that is already groomed (re-present a plan, or present one `/deliver` groomed on its own)

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
- Ships through an external gate or to several targets — per `docs/guides/release.md`, the change needs a store review, a staged rollout, or coordinated releases across targets (e.g. API + mobile app)

Report:
```
Estimate: <S/M/L>  (<N files likely affected>, <risk factors if any>)
Release targets: <target ids likely affected, from docs/guides/release.md — or "no release guide">
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

## Release Targets
- <target id> — <why it is affected>
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

### Phase 6 — Present the Plan

The groomed plan is written for the agents that build from it: paths, line numbers, a drift anchor. This phase turns it into a deck a person can read in a few minutes — what will be built, why each decision went the way it did, and in what order — and asks them to approve it before `/deliver` starts.

**Gate.**

| Groom state | Action |
|-------------|--------|
| BLOCKED, or corrections not yet applied | No deck — a deck of a plan that will change is a deck that misleads. Go to Phase 7 |
| Estimate **S** | Ask: "Small change — want a plan deck before building? (yes / no)". On no, go to Phase 7 with approval `skipped` |
| Estimate **M** or larger | Produce the deck |

**Input is the persisted plan, and only the plan.** Read `~/.claude/agent-memory/grooming-agent/plans/<ID>.md` (or the ticket-platform copy if the local one is missing). The deck adds no claim, step or rationale the plan does not contain — if the deck needs something the plan lacks (most often: a decision with no recorded rationale), the plan is incomplete; re-groom rather than inventing it here.

**Slides, in this order:**

1. **Title** — ticket id and title, estimate, affected release targets, the commit the plan was validated against
2. **Problem** — the problem statement and who it affects
3. **Scope** — what is in, and the non-goals, side by side
4. **One slide per row of the plan's `## Decisions` table** — the decision as the headline, then *why*, then *what else was considered and why it lost*. A row marked `none — <why>` shows that reason; it is not dropped
5. **The steps** — the execution plan in order, one line each with the files it touches; split across slides past seven steps, never truncated
6. **Risks and release impact** — blast-radius notes, security notes, per-target release impact
7. **Open questions** — each with its recommended default
8. **The ask** — "Approve this plan, or tell me what to change"

**Traceability check before writing.** Every execution step and every Decisions row appears in the deck, and nothing appears that is not in the plan. If either fails, fix the deck — do not ship a deck that disagrees with the plan it presents.

**The file.** Write one self-contained HTML document to `~/.claude/agent-memory/grooming-agent/plans/<ID>.deck.html`, next to the plan, so everything that retires the plan retires the deck with it:

- inline CSS only; no external fonts, scripts, images or stylesheets — it must open offline, from a file, years later
- **every string taken from the ticket or the plan is HTML-escaped** (`&`, `<`, `>`, `"`, `'`) — ticket text is written by people and other tools, and a deck can be shared; text in it must never become markup or code
- one `<section>` per slide; arrow keys and scrolling both move between slides, and any script that does this never reads or evaluates slide text
- readable in light and dark (`prefers-color-scheme`), and prints one slide per page (`@media print` with `break-after: page`)
- no slide framework or library is bundled — the structure above is small enough to write by hand, and a dependency would have to be vendored to stay offline

**Publish where the host can.** If this session has a tool that publishes a page or artifact, publish the same deck through it — privately — and report the link. If it has none (other CLIs, or a declined permission), say "deck written to <path>; no publishing tool in this session" and carry on. Never make publishing a precondition for approval.

**Ask for sign-off:**

```
Plan deck: <path>  [<link>]
  Decisions: D   Steps: K   Open questions: Q

Approve this plan? (approve / adjust / skip)
```

| Answer | Action |
|--------|--------|
| **approve** | Stamp the approval into the local plan, directly under the `**Validated against**:` line: `**Approved**: <approver> on <YYYY-MM-DD> (plan deck)`. Then record it on the ticket with the platform's comment operation (the ticket copy is the plan's source of truth — an approval only on this machine is lost on the next one). With no ticket platform, say the approval is recorded locally only |
| **adjust** | Capture what should change, re-invoke the `grooming-agent` with the plan and that change, then rebuild the deck from the refreshed plan and ask again. Never edit the deck by hand to match a change the plan does not contain |
| **skip** | Leave the plan unapproved. `/deliver` will show it as not reviewed |

---

### Phase 7 — Report & Hand Off

```
Refinement complete

  Requirements: N stories, N acceptance criteria
  Ticket:       <url or ID> — "<title>"
  Estimate:     <S/M/L>
  Groom status: READY TO BUILD / corrections applied
  Release:      <affected targets from the groom plan + impact notes, e.g. "api, android — android needs store build">
  Deck:         <path> [+ link] / not produced — <reason>
  Approval:     approved by <name> on <date> / skipped / pending

Next:
  /deliver <ticket-id>   — pick this up and build it
```

---

## Guidelines

- **Requirements describe what the user can do, not how the code works** — no implementation details in acceptance criteria
- **Non-goals are not optional** — a spec without explicit exclusions will be interpreted as "build everything adjacent"
- **Grooming is mandatory before /deliver** — an unvalidated ticket puts implementation at risk; never skip Phase 5
- **The deck presents the plan; it never extends it** — rationale lives in the plan's Decisions table, and a change asked for during sign-off goes back through grooming, not into the slides
- **Atlas absence is a warning, not a blocker** — requirements can be written; grooming accuracy will be reduced without codebase context
