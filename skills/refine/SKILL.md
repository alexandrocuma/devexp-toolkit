---
name: refine
description: Backlog refinement — takes a raw idea or stakeholder request through requirements authoring, estimation, a tech spec for epics and greenfield projects, and codebase-validated grooming into ready-to-build tickets
---

# Backlog Refinement

You are running the **backlog refinement ceremony**. Given a vague idea, a stakeholder request, or a "we should add X," you produce a groomed, ready-to-build work item: written requirements, a complexity estimate grounded in the actual codebase, a validated execution plan, and a deck that lets a human see what will be built — and why — before anyone builds it. For a multi-part feature or a new project you first write a **tech spec**: a document in the repo that is the contract between the idea and the code until the feature ships.

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

The template names no technology. The project's own choices — its language, store, framework, hosting — go into the content, each with its reason in **S-9 Decisions** — the same table, columns and `none — <why>` rule as the groomed plan's `## Decisions`, so a decision reads the same in the spec, the plan and the deck. In an existing codebase, every choice is checked against what the repo already uses: a spec that introduces a second way of doing something the repo already does says so in S-9.

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
| # | Decision | Rationale | Alternatives considered | Why rejected |
|---|----------|-----------|-------------------------|--------------|
| D1 | <what was decided> | <why> | <each alternative> | <why it lost — or "none — <why there was no real alternative>"> |

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

The grooming agent is run by reading its file and following it, never by starting a subagent with its name: a custom agent name is not a valid subagent type.

#### 5a. Single ticket

Read `~/.claude/agents/grooming-agent.md` and follow it in this context to validate every claim in the ticket against the actual codebase:

> "Groom ticket <ID>. Validate all claims against the codebase. Produce a Ticket Health Report and a verified execution plan. Persist the plan to the ticket platform."

The grooming agent will:
- Use `codebase-navigator` and `backend-senior-dev`/`frontend-senior-dev` to validate the ticket
- Produce a Ticket Health Report (Confirmed / Incorrect / Missing context)
- Write a verified execution plan back to the ticket

Wait for the groom to complete. If it returns BLOCKED or NEEDS TICKET CORRECTION, address the finding before proceeding.

#### 5b. Epic — groom every ticket in parallel

When Phase 4 created two or more tickets, groom them at the same time, so a team can pick them up together. A groom writes only its own plan and its own ticket comment. Anything two grooms would both write is done once, here, after they all return.

1. **Once, before fanning out.** Run the grooming agent's atlas freshness gate (its Phase 0) here, including the full `codebase-navigator` rebuild on BIG drift. Read the release guide once.
2. **Fan out.** In one message, start one general-purpose subagent per ticket. Pass the ticket ID, never the ticket text: each subagent fetches its own ticket, so ticket text never becomes part of an instruction you wrote. The prompt:

   > "Read `~/.claude/agents/grooming-agent.md` and follow it to groom ticket <ID>. The codebase atlas is current: do not rebuild it, and do not start `codebase-navigator` for a rebuild. Write only `~/.claude/agent-memory/grooming-agent/plans/<ID>.md` and the ticket-platform comment for <ID>. Do not update `MEMORY.md`, do not write `<PROJECT-NAME>.md`, and do not run `/graphify --update`: return the memory entry you would have written instead. Do not ask the user anything: on NEEDS TICKET CORRECTION or BLOCKED, stop after the Ticket Health Report and return it with the proposed corrections. Tickets <sibling IDs> are part of this same epic. A dependency on one of them is not BLOCKED: record it as `Depends on: <ID>` in the plan header, and plan against what that ticket's acceptance criteria (or spec sections) say it will deliver. If you cannot start subagents of your own, read the specialist agent files and follow them yourself."

3. **Collect.** Wait for every subagent. One ticket's BLOCKED or NEEDS TICKET CORRECTION never stops the others; each is reported on its own line.
4. **Shared writes — once, in order, here.** Append each returned memory entry to the grooming agent's `<PROJECT-NAME>.md`, update its `MEMORY.md` index once, then run `/graphify --update` once if `graphify-out/graph.json` exists.
5. **Per-ticket follow-up.** For each NEEDS TICKET CORRECTION ticket, show its corrections and ask: *update the ticket and groom it again / leave it for review*. Groom the accepted ones again the same way (5a when there is only one). A BLOCKED ticket is reported and gets no deck.
6. **One at a time when needed.** If this session has no tool for starting a subagent, or the user declines one, groom the tickets one after another with 5a and say so in one line: `Grooming <N> tickets one at a time: no subagent tool in this session.`

---

### Phase 6 — Present the Plan

The groomed plan is written for the agents that build from it: paths, line numbers, a drift anchor. This phase turns it into a deck a person can read in a few minutes — what will be built, why each decision went the way it did, and in what order — and asks them to approve it before `/deliver` starts.

**Gate.**

| Groom state | Action |
|-------------|--------|
| BLOCKED, or corrections not yet applied | No deck — a deck of a plan that will change is a deck that misleads. Go to Phase 7 |
| Estimate **S** | Ask: "Small change — want a plan deck before building? (yes / no)". On no, go to Phase 7 with approval `skipped` |
| Estimate **M** or larger | Produce the deck |

**Epic — decks in parallel.** When 5b groomed several tickets, apply the gate to each ticket on its own, and ask about every S ticket at once: "Small tickets <IDs> — plan decks? (all / none / pick)". Then build the decks at the same time:

- Start one general-purpose subagent per ticket that gets a deck, in one message. Its prompt names only that ticket's plan path, copies the **Input**, **Four slides, no more**, **Slides**, **Every diagram reads top to bottom**, **Diagram catalog**, **Traceability check before writing** and **The file** paragraphs of this phase word for word (they are safety rules, and a paraphrase is where they erode), and tells it to write only `plans/<ID>.deck.html` and return the path with its counts of diagrams (each `<svg role="img">`), decisions, steps and open questions — or, for a plan that does not fit four slides, to write nothing and return what overflowed. Subagents never publish.
- Back here, publish each deck in turn (the rule below still applies) and ask for sign-off once, as one list:

  ```
  Plan decks:
    <ID>  <title>  <path> [<link>]  Diagrams: N  Decisions: D  Steps: K  Open questions: Q   approve / adjust / skip
    <ID>  <title>  too long — <what overflowed>, would need <S> slides                         split / adjust
  ```

- Record each answer per ticket with the approve / adjust / skip table below. An **adjust** grooms only that ticket again (5a) and rebuilds only its deck.
- If this session has no tool for starting a subagent, or the user declines one, build the decks one after another and say so in one line.

**Input is the persisted plan, and only the plan.** Read `~/.claude/agent-memory/grooming-agent/plans/<ID>.md` (or the ticket-platform copy if the local one is missing). The deck adds no claim, step or rationale the plan does not contain — if the deck needs something the plan lacks (most often: a decision with no recorded rationale), the plan is incomplete; re-groom rather than inventing it here.

- **Find each section by its heading.** A plan groomed today has seven fixed level-2 headings: `## Files that change`, `## Files confirmed NOT changing`, `## Execution Steps`, `## Risks`, `## Affected Release Targets`, `## Decisions`, `## Diagrams`.
- **An older plan is read by what it holds.** A plan groomed before the headings were fixed names them differently (`### Steps`, `Files confirmed safe`, `### Risk`), so find each source by its content: the ordered steps list, the files table, the risks list. With no `## Risks`, the blast radius comes from the Ticket Health Report's `### Release Impact` and `### Security Notes`.
- **A source that cannot be found is stated, never invented.** Its slide says "`<section>` is not in the plan — suggest `adjust`" in place of the diagram. With no files table, the changed files are the ones the steps name. A source found only in part is drawn from the part that exists, with a caption naming what is missing.
- **A text field the plan lacks** (estimate, problem statement, non-goals) shows as "not in the plan"; the problem headline falls back to the ticket title.
- **Edges are read only inside the fenced `text` blocks** under `### Flow: before` and `### Flow: after` in `## Diagrams`, one `a -> b : label` edge per line. A node spelled the same in both flows is unchanged; one only in *after* is new; one only in *before* is removed. Edges compare by their two ends: the same `a -> b` with the same label is unchanged, with a different label it is **changed** (draw the after label, marked changed, and keep the before label in the `<desc>`), and an edge only in *after* or only in *before* is new or removed. An `a -> b` line anywhere else in the plan is prose, not an edge.

**Four slides, no more.** A deck has at most four slides. Once the gate says a deck is built, and before writing anything, check the plan fits: at most seven execution steps, at most six rows in `## Decisions`, and at most 12 nodes in any diagram. A node is one box: a step row with its files is one node, and a file listed as both changed and checked is one node (changed). A plan that does not fit gets **no deck** — needing five to seven slides means the feature is too long or the plan is not yet well defined, and a longer deck only hides that. Name every limit it broke and how many slides it would have needed — one more for each further seven steps, six decisions or 12-node diagram — and offer **split** (cut the ticket smaller, back to Phase 4) or **adjust** (re-groom it: tighten the steps, or keep in `## Decisions` only the choices a reviewer could reasonably have made differently). Never squeeze, truncate, group into sub-diagrams or add a fifth slide to make a plan fit.

**Slides, in this order.** Each slide leads with its diagram, the first and largest thing on the slide — or, where its source is missing from the plan, with the note that says so. Under it come a one-line headline and at most three short captions. The slide's full plan text follows in a closed `<details>`, so nothing is lost and the traceability check has a place to look. Each diagram is drawn once.

1. **What** — ticket id and title, estimate, affected release targets and the commit the plan was validated against in the header; the problem as the headline; led by the **before → after flow**. If the flow is `Not applicable`, the **change map** leads here instead. If the plan has no `## Diagrams` section, the slide leads with the note "no flow in this plan — suggest `adjust`" and draws no flow
2. **Where** — scope and non-goals as captions, led by the **change map** above the **blast radius** (the blast radius alone when the change map already led slide 1)
3. **How** — led by the **step graph**: the execution plan in order, one row each with the files it touches
4. **Why** — one **decision board** per row of the plan's `## Decisions` table, each with the decision, *why*, and *what else was considered and why it lost*; a row marked `none — <why>` shows that reason, it is not dropped. Below the boards, as text: open questions with their recommended defaults, then the ask — "Approve this plan, or tell me what to change"

**Every diagram reads top to bottom.** Order and cause run down the page:

- **Flow** — one node under the next, each arrow's label beside it; a return edge, such as *adjust*, loops back up the side
- **Step graph** — one row per step, in order, with the files it touches on the same row
- **Decision board** — the chosen option on top, and what it beat below with an arrow up to the winner carrying the deciding reason; a row marked `none — <why>` draws the chosen option alone, with that reason under it
- **Blast radius** — tiers going down, from changed part to what it touches to release target
- **Change map** — changed files first, then the files checked and left alone

Two diagrams of the same kind may sit side by side, such as the before and after flows or two decision boards, each still reading top to bottom; three to six decision boards sit two per row. No diagram runs left to right.

**Diagram catalog.** Each diagram kind is drawn from these plan sections and nothing else:

| Diagram | Source | Slide |
|---|---|---|
| Before → after flow, node under node | `## Diagrams` | 1 What |
| Change map, changed files above checked ones | `## Files that change` + `## Files confirmed NOT changing` | 2 Where (1 when the flow is `Not applicable`) |
| Blast radius, changed part → what it touches → release target | `## Risks` + `## Affected Release Targets` | 2 Where |
| Step graph, one row per step | `## Execution Steps` | 3 How |
| Decision board, chosen option on top | `## Decisions` (one per row) | 4 Why |

For an older plan, the source is found by content as **Input** says.

**Traceability check before writing.** Every execution step and every Decisions row appears in the deck, and nothing appears that is not in the plan. The check runs per node: every box and every arrow label in every diagram maps to a plan item (a file, a step, a decision row, an edge line, a risk, a release target), and every step and every decision row appears in some diagram. If either fails, fix the deck — do not ship a deck that disagrees with the plan it presents.

**The file.** Write one self-contained HTML document to `~/.claude/agent-memory/grooming-agent/plans/<ID>.deck.html`, next to the plan, so everything that retires the plan retires the deck with it:

- inline CSS only; no external fonts, scripts, images or stylesheets — it must open offline, from a file, years later
- **every string taken from the ticket or the plan is HTML-escaped** (`&`, `<`, `>`, `"`, `'`) — ticket text is written by people and other tools, and a deck can be shared; text in it must never become markup or code
- one `<section>` per slide; arrow keys and scrolling both move between slides, and any script that does this never reads or evaluates slide text
- readable in light and dark (`prefers-color-scheme`), and prints one slide per page (`@media print` with `break-after: page`)
- no slide framework or library is bundled — the structure above is small enough to write by hand, and a dependency would have to be vendored to stay offline
- diagrams are inline `<svg>` written by hand; no diagram library, no external image
- the escape rule covers SVG too: `<text>`, `<title>`, `<desc>` and every attribute value. Inside SVG there is no `<foreignObject>`, no `<script>` and no `on*` attribute, and no `href`, `xlink:href`, `style` or `class` value is built from plan text. Ids (for `aria-labelledby`) are generated, such as `s3-t`, never derived from plan text
- every `<svg>` has `role="img"` and `aria-labelledby` pointing at its `<title>` and a `<desc>` that states the diagram in one sentence, so a screen reader, a text dump or a colourless print still carries it
- new, changed, removed and checked-but-unchanged differ by a label or a line style (for example dashed for removed, dotted for untouched) as well as by colour; colours are CSS custom properties redefined under `prefers-color-scheme: dark`
- each diagram reads top to bottom (see **Slides**), so its `viewBox` is portrait (taller than wide), or two same-kind columns side by side; `@media print` keeps diagrams at full width, one slide per page

**Publish where the host can.** If this session has a tool that publishes a page or artifact, publish the same deck through it — privately — and report the link. If it has none (other CLIs, or a declined permission), say "deck written to <path>; no publishing tool in this session" and carry on. Never make publishing a precondition for approval.

**Ask for sign-off:**

```
Plan deck: <path>  [<link>]
  Diagrams: N   Decisions: D   Steps: K   Open questions: Q

Approve this plan? (approve / adjust / skip)
```

A plan that does not fit four slides gets no deck and no approval; ask instead:

```
Plan too long for a deck: <what overflowed> — would need <S> slides.
Split the ticket, or adjust the plan? (split / adjust)
```

| Answer | Action |
|--------|--------|
| **approve** | Stamp the approval into the local plan, directly under the `**Validated against**:` line: `**Approved**: <approver> on <YYYY-MM-DD> (plan deck)`. Then record it on the ticket with the platform's comment operation (the ticket copy is the plan's source of truth — an approval only on this machine is lost on the next one). With no ticket platform, say the approval is recorded locally only |
| **adjust** | Capture what should change, groom the ticket again (5a) with the plan and that change, then rebuild the deck from the refreshed plan and ask again. Never edit the deck by hand to match a change the plan does not contain |
| **skip** | Leave the plan unapproved. `/deliver` will show it as not reviewed |
| **split** | Only for a plan too long for a deck. Go back to Phase 4 with the overflow as the reason, cut the ticket into smaller ones, then groom and present each. The original plan stays unapproved |

---

### Phase 7 — Report & Hand Off

```
Refinement complete

  Requirements: N stories, N acceptance criteria
  Tech spec:    docs/specs/<slug>.md — approved in <commit / blob, uncommitted> / none
  Ticket:       <url or ID> — "<title>"
  Estimate:     <S/M/L>
  Groom status: READY TO BUILD / corrections applied
  Release:      <affected targets from the groom plan + impact notes, e.g. "api, android — android needs store build">
  Deck:         <path> [+ link] / not produced — <reason>
  Approval:     approved by <name> on <date> / skipped / pending

Next:
  /deliver <ticket-id>   — pick this up and build it
```

**Epic — the handoff list.** For an epic, the report ends with one row per ticket instead of a single `/deliver` line, so anyone can pick up a ticket with everything they need:

```
Handoff (<N> tickets):
  | Ticket | Plan | Deck | Groom | Approval | Start |
  | <ID>   | ~/.claude/agent-memory/grooming-agent/plans/<ID>.md | <path or link / none — reason> | READY TO GROOM / CORRECTED / NEEDS TICKET CORRECTION / BLOCKED | approved / skipped / pending | now / after <ID>[, <ID>] / blocked — <reason> |

Start now (they can run at the same time, each in its own worktree):
  /deliver <ID>
  /deliver <ID>
```

- **Groom** shows the grooming agent's own verdict.
- **Start** comes from the spec's S-8 `Depends on` column, mapped through each milestone to its tickets. Without a spec, it comes from the dependency map Phase 4 built.
- A ticket is `now` only when every ticket it depends on is already delivered or not part of this run, and its own groom is READY TO GROOM or CORRECTED. A ticket whose dependency came back BLOCKED or NEEDS TICKET CORRECTION is `blocked — <ID> <verdict>`.
- Running several `/deliver`s at once is safe because each delivers in its own worktree on its own branch (`/deliver` Phase 1.5).

---

## Guidelines

- **Requirements describe what the user can do, not how the code works** — no implementation details in acceptance criteria
- **Non-goals are not optional** — a spec without explicit exclusions will be interpreted as "build everything adjacent"
- **Grooming is mandatory before /deliver** — an unvalidated ticket puts implementation at risk; never skip Phase 5
- **The deck presents the plan; it never extends it** — rationale lives in the plan's Decisions table, and a change asked for during sign-off goes back through grooming, not into the slides. That holds for every node of every diagram: a box or arrow the plan does not contain is invented, however obvious it looks
- **Four slides or it goes back** — a plan that needs a fifth slide is too large or not yet clear; split or re-groom it rather than presenting a long deck
- **Parallel grooms never share a write** — each writes only its own plan and ticket comment; the memory index, the project memory file and the graph rebuild run once, here, after the last groom returns, and the atlas check runs once before the first one starts
- **Atlas absence is a warning, not a blocker** — requirements can be written; grooming accuracy will be reduced without codebase context
- **The spec explains, grooming orients** — the tech spec says how the whole feature will be built; grooming checks each ticket against it and the code. A spec is not an ADR: it plans a feature and retires when it ships, an ADR records one decision for good
- **An approved spec changes only by amendment** — every change after approval gets an Amendments row, so the difference between what was agreed and what is being built is always one diff away
