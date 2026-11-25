---
name: refine
description: Backlog refinement — takes a raw idea or stakeholder request through requirements authoring, estimation, a tech spec for epics and greenfield projects, and codebase-validated grooming into ready-to-build tickets
---

# Backlog Refinement

You are running the **backlog refinement ceremony**. Given a vague idea, a stakeholder request, or a "we should add X," you produce a groomed, ready-to-build work item: written requirements, a complexity estimate grounded in the actual codebase, and a validated execution plan. For a multi-part feature or a new project you first write a **tech spec**: a document in the repo that is the contract between the idea and the code until the feature ships.

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

Skip that question when the repo has no source code yet (an empty or docs-only repo) — there is nothing to orient on, and the idea is greenfield (Phase 1).

---

### Phase 1 — Capture the Idea

If no input was given inline, ask: "What's the idea or request? A sentence is enough."

Accept any format: a sentence, a paragraph, a bullet list, a stakeholder quote, a Slack message copy-paste.

Identify the shape:
- **Single atomic change** (one engineer, 1-3 days) → single ticket path, no tech spec unless the user asks for one
- **Multi-part feature** (multiple layers, multiple engineers) → tech spec (Phase 3.5), then epic decomposition
- **Greenfield project** (the repo has no source code for the idea's area, or the user says it is a new project or app) → tech spec (Phase 3.5), then epic decomposition. The spec is the only anchor this work has, so it is never skipped

Tell the user which shape you picked and why, in one line. They can override it.

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

For a greenfield project, skip the search — there is no code to anchor in. Say so, and take the domain terms from the user's own words.

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

### Phase 3.5 — Write the Tech Spec  *(epic and greenfield only)*

Skip this phase for a single atomic change, unless the user asked for a spec.

A tech spec **explains** how the whole feature or project will be built; grooming later **orients** each ticket against it. It is the contract between the idea and the code: it lives in the repo until the feature ships, tickets are cut from it, and any change to it after approval is recorded and visible in a diff. It is not an ADR — an ADR records one decision permanently, a spec plans a whole feature and retires when it ships.

#### 3.5a. Where it lives

Write it to `docs/specs/<slug>.md`, where `<slug>` is the feature name in kebab-case. If that file already exists, ask whether to amend it or pick a new slug — never overwrite a spec silently.

Keep `docs/specs/README.md` as the index — create it if missing, add one row per spec:

```markdown
# Specs

| Spec | Status | Tickets |
|------|--------|---------|
| [<Feature name>](<slug>.md) | draft | — |
```

#### 3.5b. Write it

Fill the template below. Reuse Phase 2's output as written — Goals, Non-Goals and Open Questions come from the requirements, never re-asked. Every section keeps its ID; sub-sections number on (`S-4.1`, `S-4.2`) so a ticket can point at exactly what it implements. A section that does not apply says `Not applicable — <why>` rather than disappearing, so the IDs never shift.

The template names no technology. The project's own choices — its language, store, framework, hosting — go into the content, each with its reason in **S-9 Decisions**. In an existing codebase, every choice is checked against what the repo already uses: a spec that introduces a second way of doing something the repo already does says so in S-9.

```markdown
# Tech Spec: <Feature name>

**Status**: draft
**Shape**: epic | greenfield
**Owner**: <who asked for it>
**Approved by**: —
**Approved on**: —
**Approved in**: —
**Tickets**: —

## S-1 Overview
<What is being built and why, in one paragraph. Link the requirements' problem statement.>

## S-2 Goals and Non-Goals
### Goals
- <From Phase 2: what the user can do when this ships>
### Non-Goals
- <From Phase 2 — mandatory>

## S-3 Architecture
<The shape of the system: the parts, where each runs, how a request or event flows through them. Name existing modules this builds on.>

## S-4 Components
### S-4.1 <Component>
<Responsibility, what it owns, what it depends on.>

## S-5 Data Model
<Entities, their fields and relationships, ownership, lifecycle. What is persisted and where.>

## S-6 Interfaces and Contracts
<Every boundary another part depends on: endpoints, events, messages, function signatures, file formats — with inputs, outputs and errors.>

## S-7 Cross-cutting Concerns
### S-7.1 Security
<Who may do what; what data is sensitive; how it is protected.>
### S-7.2 Observability
<What is logged and measured; the signals that say it is healthy.>
### S-7.3 Errors and Edge Cases
<How each failure is reported and recovered from.>

## S-8 Milestones
| ID | Milestone | Delivers | Depends on |
|----|-----------|----------|------------|
| M1 | <name> | <S-x sections it implements> | — |

## S-9 Decisions
### D1 <Decision title>
- **Choice**: <what was decided>
- **Rationale**: <why>
- **Alternatives considered**: <each, and why it was rejected — or "none: <why there was no real alternative>">

## S-10 Open Questions
- [ ] <From Phase 2, plus any the design raised>

## Amendments
| Date | Section | Change | Reason |
|------|---------|--------|--------|
```

#### 3.5c. Approve it

Show the spec (path plus a section-by-section summary) and ask:

```
Tech spec written: docs/specs/<slug>.md  (<N> sections, <N> milestones, <N> decisions, <N> open questions)

Approve this spec? (approve / adjust / stop)
```

- **adjust** — take the change, edit the spec, ask again. Before approval, edits need no Amendments entry.
- **stop** — leave it as `draft`, create no tickets, and end the run. A draft spec is a valid stopping point.
- **approve** — an open question in S-10 that blocks a milestone must be answered or moved to that milestone first; say which ones block. Then set `Status: approved`, `Approved by`, `Approved on`, and record where the approval lives:

  Offer to commit the spec on its own, so the approval is a point in history anyone can diff against. If the current branch is the default branch, create `spec/<slug>` first — a spec reaches the default branch the way any other change does.

  ```bash
  git add docs/specs/<slug>.md docs/specs/README.md
  git commit -m "docs(spec): <slug> approved" -- docs/specs/<slug>.md docs/specs/README.md
  ```

  Set `Approved in: commit "docs(spec): <slug> approved"` *before* committing — the file cannot carry its own SHA, so consumers find the approval by its message:

  ```bash
  approved=$(git log -1 --format=%H --grep='^docs(spec): <slug> approved' -- docs/specs/<slug>.md)
  git diff "$approved" -- docs/specs/<slug>.md   # every change since sign-off
  ```

  If the user declines the commit, store the approved content in the local object database and record its hash as `Approved in: blob <hash> (uncommitted — local to this machine)`:

  ```bash
  git hash-object -w docs/specs/<slug>.md
  ```

  Say plainly that an uncommitted approval can be diffed only on this machine.

**After approval, the spec changes only by amendment.** Every edit appends a row to **Amendments** (date, section ID, what changed, why) in the same change as the edit, and an amendment that is committed uses the message `docs(spec): <slug> amended — <section IDs>`. A change to an approved spec with no Amendments row is a silent change — the thing this document exists to prevent.

Status values, in order: `draft` → `approved` → `in-build` → `implemented`. `/refine` sets the first two; later phases move it on.

---

### Phase 4 — Create the Work Item

**Tickets come only from an approved spec.** When Phase 3.5 ran and the spec is not `approved`, create nothing — go back to 3.5c.

Present the plan before creating anything:

```
Refinement plan:
  Requirements: N stories, N acceptance criteria
  Tech spec:    docs/specs/<slug>.md — approved / none (single ticket)
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

## Spec
- `docs/specs/<slug>.md` — implements <S-x, S-y.z>   ← only when a spec exists
```

For an epic — decompose into 3-8 atomic tickets (each S or M), map dependencies, present the critical path before creating. With a spec, cut the tickets from **S-8 Milestones**: each milestone becomes one or more tickets, a milestone's `Depends on` becomes the ticket dependency, and every ticket's `## Spec` section names the section IDs it implements. A spec section no ticket implements is either out of scope (move it to Non-Goals by amendment) or a missing ticket — say which before creating.

After creating the tickets, write their IDs into the spec's `Tickets` field and the index row in `docs/specs/README.md`. That is bookkeeping, not a design change, so it needs no Amendments row.

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
  Tech spec:    docs/specs/<slug>.md — approved in <commit / blob, uncommitted> / none
  Ticket:       <url or ID> — "<title>"
  Estimate:     <S/M/L>
  Groom status: READY TO BUILD / corrections applied
  Release:      <affected targets from the groom plan + impact notes, e.g. "api, android — android needs store build">

Next:
  /deliver <ticket-id>   — pick this up and build it
```

---

## Guidelines

- **Requirements describe what the user can do, not how the code works** — no implementation details in acceptance criteria
- **Non-goals are not optional** — a spec without explicit exclusions will be interpreted as "build everything adjacent"
- **Grooming is mandatory before /deliver** — an unvalidated ticket puts implementation at risk; never skip Phase 5
- **Atlas absence is a warning, not a blocker** — requirements can be written; grooming accuracy will be reduced without codebase context
- **The spec explains, grooming orients** — the tech spec says how the whole feature will be built; grooming checks each ticket against it and the code. A spec is not an ADR: it plans a feature and retires when it ships, an ADR records one decision for good
- **An approved spec changes only by amendment** — every change after approval gets an Amendments row, so the difference between what was agreed and what is being built is always one diff away
