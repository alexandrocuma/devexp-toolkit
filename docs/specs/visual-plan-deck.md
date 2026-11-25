# Tech Spec: Visual plan deck

**Status**: in-build
**Shape**: epic
**Owner**: Alexandro Cu
**Approved by**: Alexandro Cu
**Approved on**: 2026-11-24
**Approved in**: commit "docs(spec): visual-plan-deck approved"
**Tickets**: #236 (M1), #237 (M2), #238 (M3), #242 (M4)

## S-1 Overview

`/refine` Phase 6 turns a groomed plan into an HTML deck so a person can approve the work before `/deliver` builds it (#221). The deck works, but it is text. A real one (#234) has 19 slides of headlines and paragraphs, so a reviewer still has to read the plan to see what is being built. This spec makes the deck **visual first**. Every slide leads with a diagram that shows what changes, how the pieces connect and in what order the work happens. Text becomes the caption and the plan stays the source. The deck still adds nothing the plan does not say, so `grooming-agent` writes the one diagram that cannot be derived from the plan's existing sections into the plan itself.

Problem statement: a reviewer with no context cannot tell from today's deck what will be built, how it fits into what exists, or what each decision trades away, without reading every word.

## S-2 Goals and Non-Goals

### Goals
- A reviewer with no prior context understands what will be built from the diagrams alone, in a few minutes.
- Every deck shows the same core diagrams, so decks are comparable and reviewers know where to look.
- Each decision is shown as the chosen option next to the alternatives it beat, not as a paragraph.
- Diagrams are drawn only from what the plan says. Each node traces to a plan item.
- The deck still opens offline from a file, works in light and dark, and prints one slide per page.
- A deck has at most four slides. A plan that needs more (five to seven) means the feature is too large or the plan is not yet well defined, so it is sent back to be split or re-groomed, never presented.

### Non-Goals
- Visual tech specs (Phase 3.5). The spec stays Markdown. A spec deck is a possible follow-up, see S-10.
- A diagram library, a renderer binary or any external dependency. Diagrams are hand-written inline SVG.
- Interactive editing of the deck, or comments in it. Sign-off stays in the chat: approve / adjust / skip.
- Changing what the plan's sections say. The plan gains the `## Diagrams` section and a `## Risks` section, and Phase 7 fixes the heading names of the sections the deck reads (S-4.1). Their content requirements are unchanged.
- Changing Phase 6's gate (S never, M+ always) or its approval flow.

## S-3 Architecture

Three stages, each owned by an existing part of the toolkit:

```
ticket ──▶ grooming-agent ──▶ plan <id>.md ──▶ /refine Phase 6 ──▶ <id>.deck.html ──▶ reviewer
             (Phase 7)        + ## Diagrams      (deck author)       inline SVG         approve /
                                                 derive + render                         adjust / skip
```

1. **Source**: `grooming-agent` Phase 7 writes the plan under seven fixed headings (S-4.1). One is the new `## Diagrams` section. It holds only the *before → after flow*, the one picture the plan's other sections cannot produce, written as a plain-text edge list.
2. **Derive**: Phase 6 builds every other diagram from the plan's other fixed sections. *Files that change* gives the change map, *Execution Steps* gives the step graph, *Decisions* gives the decision boards and *Risks* plus *Affected Release Targets* give the blast-radius view.
3. **Render**: Phase 6 draws each diagram as inline SVG in the deck, runs the traceability check (now per node), writes the file and publishes it where the host can.

`adjust` goes back through grooming exactly as today, so a diagram can never show something the plan does not.

## S-4 Components

### S-4.1 Plan sections and the `## Diagrams` section (`agents/grooming-agent.md`, Phase 7)
Phase 7 names, exactly and as level-2 headings, the seven sections the deck reads: `## Files that change`, `## Files confirmed NOT changing`, `## Execution Steps`, `## Risks`, `## Affected Release Targets`, `## Decisions` and `## Diagrams`. The names are fixed so the deck and the drift test (S-4.5) can find each section without guessing. `## Risks` is new. It lists what could break and the blast radius, where today's plans put risks in whatever section fits.

`## Diagrams` owns the before → after flow, written in the text format in S-6.1 and required for every plan. For a change with no runtime flow (docs only, say), the section says `Not applicable — <why>` and the deck uses the change map as its hero diagram.

The plan also carries the **short labels** the diagrams show (S-6.3), so the deck copies words and never rewrites them: each execution step, each risk and each Decisions cell the boards draw leads with a bold label of at most five words, and each flow edge label is at most four words. The full text follows the label and still reaches the deck in its `<details>`.

### S-4.2 Diagram catalog (`skills/refine/SKILL.md`, Phase 6 "Slides")
A fixed set of diagram kinds, each tied to one plan section:

| Diagram | Source section | Slide |
|---|---|---|
| Before → after flow | `## Diagrams` | 1 (What) |
| Change map: affected areas grouped, new / changed / untouched-but-checked | `## Files that change`, `## Files confirmed NOT changing` | 2 (Where); 1 when the flow is `Not applicable` |
| Blast radius: changed parts, what they touch, release targets | `## Risks`, `## Affected Release Targets` | 2 (Where) |
| Step graph: ordered steps as a lane, with dependencies and the files each touches | `## Execution Steps` | 3 (How) |
| Decision board: chosen option vs alternatives, with the deciding reason on the arrow | `## Decisions` row | 4 (Why), one per row |

The deck is exactly four slides: **1 What** (ticket, problem, before → after flow), **2 Where** (change map, blast radius → release targets), **3 How** (step graph), **4 Why** (decision boards, open questions, the ask). Title, problem, open questions and the ask are text on slides 1 and 4; they get no slide of their own.

Plans groomed before the headings were fixed name these sections differently (`### Steps`, `Files confirmed safe`, `### Risk`). For such a plan, Phase 6 finds each source by what it holds. Where no `## Risks` exists, it uses the Ticket Health Report's Release Impact and Security Notes. A source it cannot find is stated on its slide, with `adjust` suggested. The diagram is never invented.

### S-4.3 Slide layout rules (Phase 6 "The file")
A plan fits four slides when its step graph has at most seven steps, its `## Decisions` table at most six rows, and every diagram at most 12 nodes. Phase 6 checks this before it writes anything. A plan that does not fit gets no deck: Phase 6 says which part overflowed and how many slides it would have needed, and offers to split the ticket (back to Phase 4) or `adjust` (re-groom). Content is never squeezed, truncated or spread onto a fifth slide.

A diagram is the first and largest element on each slide. Text is limited to a one-line headline plus at most three captions of at most 12 words each.

A diagram is a picture, not the plan's prose in boxes. Every label in it is the plan's short label, copied as written (S-6.3): at most five words in a box (a file path or release target id counts as one label), at most four on an arrow. A step row shows its label and its file, a change-map box shows only its path, and a decision board shows the chosen label, the alternatives' labels and the deciding reason on the arrow. Descriptions, rationale and sub-bullets live only in the slide's `<details>`. A plan item with no short label gets no invented one: the deck shows the item's name or number and asks for `adjust`. The full plan text for the slide sits in a closed `<details>` under the diagram, so nothing is lost and the traceability check still has a place to look.

Every diagram reads top to bottom. Order and cause run down the page:
- **Flows**: one node under the next, with each arrow's label beside it. A return edge, such as adjust, loops back up the side.
- **Steps**: one row per step, in order, with the file it touches on the same row.
- **Decision boards**: the chosen option on top, and what it beat below with an arrow up to the winner.
- **Blast radius**: tiers going down, from changed part to what it touches to release target.
- **Change map**: changed files first, then the files checked and left alone.

Two things of the same kind may sit side by side, such as the before and after flows, but each one reads top to bottom. A diagram never runs left to right.

### S-4.4 Traceability check, per node (Phase 6)
Today's check is per step and per decision row. It extends to diagrams: every node and edge label maps to a plan item (a file, step, decision or edge-list line), and no plan step or decision is missing from some diagram.

### S-4.5 Consistency test (`cli/internal/repocheck/consistency_test.go`)
A Go test that fails when the diagram catalog in Phase 6 and the plan template in `grooming-agent` drift apart. For example, Phase 6 names a source section the plan template no longer writes, or the `## Diagrams` section disappears from the template. It follows the pattern of `TestSpecStatus_ValuesAgree`.

## S-5 Data Model

Nothing persisted changes shape apart from one new plan section. The deck stays at `~/.claude/agent-memory/grooming-agent/plans/<id>.deck.html`, is retired with its plan by `/release`, `/cleanup` and `/improve` as today, and is published privately when the host can.

## S-6 Interfaces and Contracts

### S-6.1 `## Diagrams` section format
Plain text, readable without a renderer, so the plan stays useful to agents and to the ticket platform copy. Each flow sits in a fenced `text` block, so the ticket platform does not read `<id>`, `_` or `*` as markup:

~~~
## Diagrams
### Flow: before
```text
user -> /refine : runs
/refine -> deck.html : writes text slides
deck.html -> reviewer : reads 19 slides
```

### Flow: after
```text
user -> /refine : runs
/refine -> deck.html : writes diagram slides
deck.html -> reviewer : scans diagrams, opens details on demand
```
~~~

Phase 6 reads edges only inside the fences. One `a -> b : label` edge per line. A node keeps the same spelling in *before* and *after*, otherwise an unchanged node would read as one removed and one new. Node names are the plan's own terms (file paths, skill names, components). A node that appears only in *after* is new, one only in *before* is removed, and the deck colours them that way. An edge is identified by its two ends: an edge `a -> b` in both flows with the same label is unchanged, one whose label differs is **changed** (the deck shows the after label and keeps the before label in the diagram's `<desc>`), and an edge only in *after* or only in *before* is new or removed. A change that keeps every node but alters what passes between them therefore still shows.

### S-6.2 Deck file contract (unchanged apart from SVG)
- Self-contained: inline CSS and inline SVG; no external font, script, image or stylesheet.
- Every string from the ticket or plan is HTML-escaped, including SVG `<text>` content and `aria-label`s.
- Every `<svg>` has `role="img"`, a `<title>` and a `<desc>` that states the diagram in a sentence, so a screen reader, a text dump or a print with no colour still carries it.
- Colours come from CSS custom properties redefined under `prefers-color-scheme: dark`; meaning never depends on colour alone (new/changed/removed also differ by line style or a label).
- `@media print` keeps one slide per page and the diagrams at full width.

### S-6.3 Short labels in the plan
Grooming writes each label as a bold lead, so it reads in plain text and the deck finds it without guessing:

| Plan item | Label | Example |
|---|---|---|
| Execution step | `**<≤5 words>** — <full step>` | `3. **Slides block** — layout lead-in; items 2-6 name their diagram…` |
| Risk | `**<≤5 words>** — <full risk>` | `- **Escape gap in SVG** — plan text in an attribute…` |
| Decisions, `Decision` cell | `**<≤5 words>** — <full decision>` | `**Anchors over line numbers** — Build on merged #235…` |
| Decisions, `Alternatives considered` cell | each alternative `**<≤5 words>**`, separated by `;` | `**Pin line numbers**; **Free inference**` |
| Decisions, `Why rejected` cell | `**<≤5 words>** — <full reason>` | `**Lines move every edit** — They move with…` |
| Flow edge (S-6.1) | the label after `:` is at most four words | `deck.html -> reviewer : scans diagrams` |

File paths and release target ids are their own labels. A `none — <why>` row keeps that form; its board shows `none` and the reason's first five words.

## S-7 Cross-cutting Concerns

### S-7.1 Security
Ticket and plan text is untrusted. SVG is markup, so the existing escape rule now covers SVG text, titles, descriptions and attributes, not just HTML body text. No `<foreignObject>`, no `<script>` inside SVG, no `href`/`xlink:href` built from plan text.

### S-7.2 Observability
Not applicable: the deck is a local file produced in a chat session. The Phase 6 sign-off summary gains a count: `Diagrams: N` next to Decisions, Steps and Open questions.

### S-7.3 Errors and Edge Cases
- Plan has no `## Diagrams` (groomed before this ships): the deck builds without the flow diagram, says so on slide 1, and suggests re-grooming. It never invents the flow.
- Plan too large for four slides (more than seven steps, more than six decisions, or a diagram past 12 nodes): no deck. Phase 6 names what overflowed and offers split or `adjust` (S-4.3).
- `Not applicable` flow: the change map becomes the hero diagram on slide 1, and slide 2 keeps the blast radius.
- Plan item with no short label, or a label past its word limit (a plan groomed before S-6.3): the deck shows the item's name or number in its place, never a rewording, and the sign-off suggests `adjust`.
- Plan groomed before the headings were fixed: sources are found by what they hold (S-4.2). A missing source is stated, never invented.

## S-8 Milestones

| ID | Milestone | Delivers | Depends on |
|----|-----------|----------|------------|
| M1 | Plans carry a flow diagram | S-4.1, S-6.1 | — |
| M2 | Deck renders the diagram catalog | S-4.2, S-4.3, S-4.4, S-6.2, S-7 | M1, #234 merged |
| M3 | Drift test + docs | S-4.5, CHANGELOG, `docs/reference/skills.md` | M1, M2 |
| M4 | Short labels: plan writes them, deck draws only them | S-4.1, S-4.3, S-6.3, S-7.3 | M1, M2, M3 |

## S-9 Decisions

| # | Decision | Rationale | Alternatives considered | Why rejected |
|---|----------|-----------|-------------------------|--------------|
| D1 | Diagrams are hand-written inline SVG → ADR-0001 | Phase 6 requires a deck that opens offline years later with nothing bundled. An LLM writes SVG directly as well as it writes HTML | Mermaid or another diagram library; ASCII art in `<pre>`; CSS-box layouts | A library has to be vendored (Phase 6 forbids it) or fetched (breaks offline). ASCII doesn't read as a diagram to a non-engineer. CSS boxes can't draw edges |
| D2 | Most diagrams are derived from existing plan sections; only the before → after flow is new plan content → ADR-0002 | The deck may not add claims the plan lacks. Files, steps, decisions and risks are already structured, and the flow is the one picture they cannot produce | Let the deck author draw whatever explains best; have grooming write every diagram | Free drawing breaks the "deck never extends the plan" rule. Writing every diagram in the plan duplicates what its sections already say |
| D3 | The flow is a plain-text edge list (`a -> b : label`) in the plan → ADR-0003 | The plan is read by agents and copied to the ticket platform. An edge list reads fine as text and is trivial to render | Mermaid syntax in the plan; SVG in the plan | Mermaid ties the plan to one tool, which conflicts with the toolkit's tool-agnostic rule. SVG in the plan is unreadable to the agents that build from it |
| D4 | A fixed diagram catalog, one kind per plan section → ADR-0004 | Every deck looks the same, so reviewers know where to look and the drift test has something to check | Pick diagrams per plan | Each deck becomes a design exercise, quality varies, and there is nothing to test |
| D5 | Diagram first, text capped at a headline plus three captions, full text in a closed `<details>` → ADR-0005 | The point is to read by looking. Keeping the full text one click away keeps traceability and nothing is lost | Remove the text; keep text and add diagrams beside it | Removing text loses the rationale that the decision slides exist for. Text beside the diagram is today's deck with pictures, and the reading load stays |
| D6 | Build on #234, which merges first → ADR-0006 | #234 rewrites Phase 6 delivery (parallel deck subagents carrying Phase 6's rules verbatim). Editing the same text in parallel means a guaranteed conflict | Land this first and rebase #234 | #234 is already groomed with an approved deck. Its verbatim-rules decision (D9 there) means this spec's new rules travel to subagents automatically once both land |
| D7 | Accessibility is part of the contract: `<title>` + `<desc>` on every SVG, meaning never by colour alone → ADR-0007 | Visual-first must not mean sight-only. The `<desc>` also gives the traceability check and a printout a text form | Leave it to the author | Without a rule it is skipped, and a colour-only legend fails in print and for colour-blind reviewers |
| D8 | Short labels are written by grooming in the plan, and the deck copies them as written | The deck may not reword the plan, and per-node traceability needs each label to be the plan's own words | The deck condenses each label from the full text; a separate label column in each table | Condensing is rewriting: it breaks "the deck never extends the plan". A new column would change the Decisions table shape that the spec, the plan and the deck share |

## S-10 Open Questions

- [ ] Should the tech spec (Phase 3.5) also get a visual review page at approval time? Recommended default: **no, follow-up**. Spec decks have different content (no files or steps yet) and deserve their own catalog. Does not block any milestone.
- [x] Is the edge-list format enough for the flow, or do we need swim-lanes (who does each step)? **Answered at approval: edge list.** An optional `lane:` prefix can come later if decks show the need. This no longer blocks M1.
- [ ] Should a plan groomed before this change be re-groomed automatically when presented? Recommended default: **no**. Say the flow is missing and suggest `adjust`. Does not block.

## Amendments
| Date | Section | Change | Reason |
|------|---------|--------|--------|
| 2026-11-24 | S-2, S-3, S-4.1 | Phase 7 fixes seven plan headings, including a new `## Risks`. The Non-Goal now says only section content stays unchanged | Grooming #236–#238 found that Phase 7 fixes only `## Spec` and `## Decisions`, and past plans name the other sections differently. The deck and the drift test had nothing reliable to read |
| 2026-11-24 | S-4.2, S-7.3 | Plans groomed before the headings were fixed have their sources found by what they hold, with the Health Report as the fallback for risks. A missing source is stated | Existing plans must still produce a deck, without inventing diagrams |
| 2026-11-24 | S-6.1 | Flows sit in fenced `text` blocks. Node names keep one spelling across before and after | GitHub reads `<id>`, `_` and `*` in bare lines as markup. Inconsistent spelling turns an unchanged node into a removed node plus a new one |
| 2026-11-25 | S-4.3 | Every diagram reads top to bottom, with a rule per diagram kind. Side by side is allowed only for two things of the same kind | The owner reviewed the first visual deck (#237) and asked for vertical diagrams, required by the skill and not just by one deck |
| 2026-11-25 | S-6.1 | Edges are compared too, by their two ends: same ends with a different label is a changed edge, shown with its after label and the before label kept in `<desc>` | Delivering #237, a dry-run deck from plan 237 showed no change at all: its flow keeps every node and changes only what passes along the edges, and S-6.1 marked only nodes |
| 2026-11-25 | S-2, S-4.2, S-4.3, S-7.3 | A deck has at most four slides — What (flow), Where (change map, blast radius), How (steps), Why (decisions, open questions, the ask). A plan needing more gets no deck and is sent back to be split or re-groomed; diagrams are no longer split into sub-diagrams | The owner: a deck that needs five to seven slides means the feature is too long or the plan is not yet well defined. A dry-run deck from plan 237 came to 17 slides |
| 2026-11-25 | S-4.3, S-7.3 | The Why slide holds up to six decision boards, two per row; the fit limit is six `## Decisions` rows, not four | Owner decision after the four-slide dry runs: with four, every plan of this spec (236, 237, 238) was sent back on decision count alone, including two S tickets |
| 2026-11-25 | S-4.1, S-4.3, S-6.3, S-7.3, S-8, S-9 | Diagrams show only short labels (≤5 words per box, ≤4 per arrow, captions ≤12 words) that grooming writes into the plan as bold leads; the deck copies them and keeps full text in `<details>`. New M4 and D8; status back to `in-build` | The owner reviewed the published #237 deck: its diagrams repeat the plan's paragraphs inside boxes and on arrows, so they read like a document, not a schema |
