# Tech Spec: Visual plan deck

**Status**: approved
**Shape**: epic
**Owner**: Alexandro Cu
**Approved by**: Alexandro Cu
**Approved on**: 2026-11-24
**Approved in**: commit "docs(spec): visual-plan-deck approved"
**Tickets**: —

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

### Non-Goals
- Visual tech specs (Phase 3.5). The spec stays Markdown. A spec deck is a possible follow-up, see S-10.
- A diagram library, a renderer binary or any external dependency. Diagrams are hand-written inline SVG.
- Interactive editing of the deck, or comments in it. Sign-off stays in the chat: approve / adjust / skip.
- Changing what the plan contains apart from the new `## Diagrams` section.
- Changing Phase 6's gate (S never, M+ always) or its approval flow.

## S-3 Architecture

Three stages, each owned by an existing part of the toolkit:

```
ticket ──▶ grooming-agent ──▶ plan <id>.md ──▶ /refine Phase 6 ──▶ <id>.deck.html ──▶ reviewer
             (Phase 7)        + ## Diagrams      (deck author)       inline SVG         approve /
                                                 derive + render                         adjust / skip
```

1. **Source**: `grooming-agent` Phase 7 writes the plan as today and adds a `## Diagrams` section. It holds only the *before → after flow*, the one picture the plan's other sections cannot produce, written as a plain-text edge list.
2. **Derive**: Phase 6 builds every other diagram from sections the plan already has. *Files that change* gives the change map, *Execution Steps* gives the step graph, *Decisions* gives the decision boards and *Risks* plus *Affected Release Targets* give the blast-radius view.
3. **Render**: Phase 6 draws each diagram as inline SVG in the deck, runs the traceability check (now per node), writes the file and publishes it where the host can.

`adjust` goes back through grooming exactly as today, so a diagram can never show something the plan does not.

## S-4 Components

### S-4.1 Plan `## Diagrams` section (`agents/grooming-agent.md`, Phase 7)
Owns the before → after flow. Written in the text format in S-6.1. Required for every plan. For a change with no runtime flow (docs only, say), the section says `Not applicable — <why>` and the deck uses the change map as its hero diagram.

### S-4.2 Diagram catalog (`skills/refine/SKILL.md`, Phase 6 "Slides")
A fixed set of diagram kinds, each tied to one plan section:

| Diagram | Source section | Slide |
|---|---|---|
| Before → after flow | `## Diagrams` | 2 (Problem), and the "what changes" hero |
| Change map: affected areas grouped, new / changed / untouched-but-checked | `## Files that change`, `## Files confirmed NOT changing` | 3 (Scope) |
| Decision board: chosen option vs alternatives, with the deciding reason on the arrow | `## Decisions` row | 4, one per row |
| Step graph: ordered steps as a lane, with dependencies and the files each touches | `## Execution Steps` | 5 |
| Blast radius: changed parts, what they touch, release targets | `## Risks`, `## Affected Release Targets` | 6 |

### S-4.3 Slide layout rules (Phase 6 "The file")
A diagram is the first and largest element on each slide. Text is limited to a one-line headline plus at most three short captions. The full plan text for the slide sits in a closed `<details>` under the diagram, so nothing is lost and the traceability check still has a place to look.

### S-4.4 Traceability check, per node (Phase 6)
Today's check is per step and per decision row. It extends to diagrams: every node and edge label maps to a plan item (a file, step, decision or edge-list line), and no plan step or decision is missing from some diagram.

### S-4.5 Consistency test (`cli/internal/repocheck/consistency_test.go`)
A Go test that fails when the diagram catalog in Phase 6 and the plan template in `grooming-agent` drift apart. For example, Phase 6 names a source section the plan template no longer writes, or the `## Diagrams` section disappears from the template. It follows the pattern of `TestSpecStatus_ValuesAgree`.

## S-5 Data Model

Nothing persisted changes shape apart from one new plan section. The deck stays at `~/.claude/agent-memory/grooming-agent/plans/<id>.deck.html`, is retired with its plan by `/release`, `/cleanup` and `/improve` as today, and is published privately when the host can.

## S-6 Interfaces and Contracts

### S-6.1 `## Diagrams` section format
Plain text, readable without a renderer, so the plan stays useful to agents and to the ticket platform copy:

```
## Diagrams
### Flow: before
user -> /refine : runs
/refine -> deck.html : writes text slides
deck.html -> reviewer : reads 19 slides

### Flow: after
user -> /refine : runs
/refine -> deck.html : writes diagram slides
deck.html -> reviewer : scans diagrams, opens details on demand
```

One `a -> b : label` edge per line. Node names are the plan's own terms (file paths, skill names, components). A node that appears only in *after* is new, one only in *before* is removed, and the deck colours them that way.

### S-6.2 Deck file contract (unchanged apart from SVG)
- Self-contained: inline CSS and inline SVG; no external font, script, image or stylesheet.
- Every string from the ticket or plan is HTML-escaped, including SVG `<text>` content and `aria-label`s.
- Every `<svg>` has `role="img"`, a `<title>` and a `<desc>` that states the diagram in a sentence, so a screen reader, a text dump or a print with no colour still carries it.
- Colours come from CSS custom properties redefined under `prefers-color-scheme: dark`; meaning never depends on colour alone (new/changed/removed also differ by line style or a label).
- `@media print` keeps one slide per page and the diagrams at full width.

## S-7 Cross-cutting Concerns

### S-7.1 Security
Ticket and plan text is untrusted. SVG is markup, so the existing escape rule now covers SVG text, titles, descriptions and attributes, not just HTML body text. No `<foreignObject>`, no `<script>` inside SVG, no `href`/`xlink:href` built from plan text.

### S-7.2 Observability
Not applicable: the deck is a local file produced in a chat session. The Phase 6 sign-off summary gains a count: `Diagrams: N` next to Decisions, Steps and Open questions.

### S-7.3 Errors and Edge Cases
- Plan has no `## Diagrams` (groomed before this ships): the deck builds without the flow diagram, says so on slide 2, and suggests re-grooming. It never invents the flow.
- Diagram too dense (more than ~12 nodes): split into grouped sub-diagrams, the same rule as "split steps past seven, never truncate".
- `Not applicable` flow: the change map becomes the hero diagram.

## S-8 Milestones

| ID | Milestone | Delivers | Depends on |
|----|-----------|----------|------------|
| M1 | Plans carry a flow diagram | S-4.1, S-6.1 | — |
| M2 | Deck renders the diagram catalog | S-4.2, S-4.3, S-4.4, S-6.2, S-7 | M1, #234 merged |
| M3 | Drift test + docs | S-4.5, CHANGELOG, `docs/reference/skills.md` | M1, M2 |

## S-9 Decisions

| # | Decision | Rationale | Alternatives considered | Why rejected |
|---|----------|-----------|-------------------------|--------------|
| D1 | Diagrams are hand-written inline SVG | Phase 6 requires a deck that opens offline years later with nothing bundled. An LLM writes SVG directly as well as it writes HTML | Mermaid or another diagram library; ASCII art in `<pre>`; CSS-box layouts | A library has to be vendored (Phase 6 forbids it) or fetched (breaks offline). ASCII doesn't read as a diagram to a non-engineer. CSS boxes can't draw edges |
| D2 | Most diagrams are derived from existing plan sections; only the before → after flow is new plan content | The deck may not add claims the plan lacks. Files, steps, decisions and risks are already structured, and the flow is the one picture they cannot produce | Let the deck author draw whatever explains best; have grooming write every diagram | Free drawing breaks the "deck never extends the plan" rule. Writing every diagram in the plan duplicates what its sections already say |
| D3 | The flow is a plain-text edge list (`a -> b : label`) in the plan | The plan is read by agents and copied to the ticket platform. An edge list reads fine as text and is trivial to render | Mermaid syntax in the plan; SVG in the plan | Mermaid ties the plan to one tool, which conflicts with the toolkit's tool-agnostic rule. SVG in the plan is unreadable to the agents that build from it |
| D4 | A fixed diagram catalog, one kind per plan section | Every deck looks the same, so reviewers know where to look and the drift test has something to check | Pick diagrams per plan | Each deck becomes a design exercise, quality varies, and there is nothing to test |
| D5 | Diagram first, text capped at a headline plus three captions, full text in a closed `<details>` | The point is to read by looking. Keeping the full text one click away keeps traceability and nothing is lost | Remove the text; keep text and add diagrams beside it | Removing text loses the rationale that the decision slides exist for. Text beside the diagram is today's deck with pictures, and the reading load stays |
| D6 | Build on #234, which merges first | #234 rewrites Phase 6 delivery (parallel deck subagents carrying Phase 6's rules verbatim). Editing the same text in parallel means a guaranteed conflict | Land this first and rebase #234 | #234 is already groomed with an approved deck. Its verbatim-rules decision (D9 there) means this spec's new rules travel to subagents automatically once both land |
| D7 | Accessibility is part of the contract: `<title>` + `<desc>` on every SVG, meaning never by colour alone | Visual-first must not mean sight-only. The `<desc>` also gives the traceability check and a printout a text form | Leave it to the author | Without a rule it is skipped, and a colour-only legend fails in print and for colour-blind reviewers |

## S-10 Open Questions

- [ ] Should the tech spec (Phase 3.5) also get a visual review page at approval time? Recommended default: **no, follow-up**. Spec decks have different content (no files or steps yet) and deserve their own catalog. Does not block any milestone.
- [x] Is the edge-list format enough for the flow, or do we need swim-lanes (who does each step)? **Answered at approval: edge list.** An optional `lane:` prefix can come later if decks show the need. This no longer blocks M1.
- [ ] Should a plan groomed before this change be re-groomed automatically when presented? Recommended default: **no**. Say the flow is missing and suggest `adjust`. Does not block.

## Amendments
| Date | Section | Change | Reason |
|------|---------|--------|--------|
