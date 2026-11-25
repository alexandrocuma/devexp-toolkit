# 0001. Use hand-written inline SVG for plan-deck diagrams

**Date**: 2026-11-25
**Status**: Accepted
**Deciders**: Alexandro Cu (spec owner)

---

## Context

`/refine` Phase 6 turns a groomed plan into an HTML deck that must open offline, from a file, years later, with nothing bundled. The visual deck needs real diagrams: boxes joined by labelled edges. The agent writing the deck writes SVG as readily as it writes HTML.

## Decision

Every plan-deck diagram is inline `<svg>` written by hand in the deck file. No diagram library, renderer or external image is used.

## Consequences

### Positive
- Decks still open offline and need no vendored dependency.
- Diagrams can draw edges, which the review relies on.

### Negative
- Each deck's diagrams are drawn fresh, so their layout quality depends on the agent; the layout rules in Phase 6 (top to bottom, portrait `viewBox`) narrow that.

### Neutral
- SVG is markup, so the escape rule extends to SVG text and attributes, with a deny list (`<foreignObject>`, script, `on*`, links built from plan text).

## Alternatives Considered

### Alternative 1: Mermaid or another diagram library
**Why not chosen**: A library has to be vendored, which Phase 6 forbids, or fetched, which breaks offline use.

### Alternative 2: ASCII art in `<pre>`
**Why not chosen**: Doesn't read as a diagram to a non-engineer.

### Alternative 3: CSS-box layouts
**Why not chosen**: Can't draw edges.

## Implementation Notes

From the [visual plan deck spec](../../specs/visual-plan-deck.md), decision D1. Shipped in v0.12.0: `skills/refine/SKILL.md` Phase 6 and `agents/grooming-agent.md` Phase 7.
