# 0008. Write plan-deck diagram labels in the plan

**Date**: 2026-11-25
**Status**: Accepted
**Deciders**: Alexandro Cu (spec owner)

---

## Context

The first diagram-first decks (ADR-0005) still read like documents. Their boxes and arrows held the plan's full sentences: a decision board carried the whole rationale and rejection text, a step row its description, a change-map box its sub-bullets, an edge label a full clause. Short labels need someone to write them, and the deck may not add or reword anything the plan says (ADR-0002).

## Decision

Grooming writes a **short label** for every item the deck draws: a bold lead of at most five words before the item's full text (each step, each risk, and the `Decision`, `Alternatives considered` and `Why rejected` cells of `## Decisions`). Flow edge labels are at most four words. The deck copies these labels as written: at most five words in a box (a file path or release target id is one label), at most four on an arrow, captions at most 12 words. Full text stays in the slide's `<details>`. An item with no label shows its name or number, never a rewording, and the sign-off suggests `adjust`.

## Consequences

### Positive
- A reviewer reads each diagram at a glance; the paragraphs are one click away.
- Every label is the plan's own words, so per-node traceability stays exact.

### Negative
- Plans groomed before this have no labels; their decks show numbers until re-groomed.
- A five-word label can hide why a decision won; the reason has to be read in `<details>`.

### Neutral
- The Decisions table keeps its five columns: the label is a bold lead inside the cell.

## Alternatives Considered

### Alternative 1: The deck condenses labels from the full text
**Why not chosen**: Condensing is rewording; the deck would extend what the plan says.

### Alternative 2: A separate label column, or a labels section
**Why not chosen**: Changes the Decisions table shape the spec, the plan and the deck share; a separate section repeats every item.

## Implementation Notes

From the [visual plan deck spec](../../specs/visual-plan-deck.md), decision D8 and section S-6.3 (milestone M4, #242). `agents/grooming-agent.md` Phase 7 and `skills/refine/SKILL.md` Phase 6; `TestShortLabels_LimitsAgree` fails CI when the two sides' limits drift apart.
