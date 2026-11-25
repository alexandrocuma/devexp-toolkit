# 0007. Make plan-deck diagrams accessible by contract

**Date**: 2026-11-25
**Status**: Accepted
**Deciders**: Alexandro Cu (spec owner)

---

## Context

A visual-first deck must not be sight-only. Reviewers may use a screen reader, print in black and white, or not distinguish the colours used for new, changed and removed.

## Decision

Every `<svg>` has `role="img"`, `aria-labelledby` pointing at its `<title>`, and a `<desc>` stating the diagram in one sentence. New, changed, removed and checked-but-unchanged differ by a label or a line style as well as by colour. Colours are CSS custom properties redefined for dark mode.

## Consequences

### Positive
- The `<desc>` gives the traceability check and a printout a text form of each diagram.

### Negative
- Every diagram carries a little more markup.

### Neutral
- None recorded

## Alternatives Considered

### Alternative 1: Leave it to the author
**Why not chosen**: Without a rule it is skipped, and a colour-only legend fails in print and for colour-blind reviewers.

## Implementation Notes

From the [visual plan deck spec](../../specs/visual-plan-deck.md), decision D7. Shipped in v0.12.0: `skills/refine/SKILL.md` Phase 6 and `agents/grooming-agent.md` Phase 7.
