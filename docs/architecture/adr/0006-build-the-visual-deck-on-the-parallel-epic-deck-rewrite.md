# 0006. Build the visual deck on the parallel epic-deck rewrite

**Date**: 2026-11-25
**Status**: Accepted
**Deciders**: Alexandro Cu (spec owner)

---

## Context

Two changes rewrote `/refine` Phase 6 at once: #234 (decks for an epic's tickets built in parallel by subagents that copy Phase 6's rules word for word) and this spec. Editing the same text in parallel guaranteed a conflict.

## Decision

#234 merged first, and the visual deck was built on top of it. Every new deck rule sits inside the paragraphs deck subagents copy word for word.

## Consequences

### Positive
- Epic decks get the visual rules with no second mechanism.

### Negative
- A rule placed outside the copied paragraphs silently skips epic decks.

### Neutral
- `TestRefineDeckSVGRulesTravelToSubagents` fails when a bold-led Phase 6 rule paragraph is not in the subagents' copy list.

## Alternatives Considered

### Alternative 1: Land the visual deck first and rebase #234
**Why not chosen**: #234 was already groomed with an approved deck, and its word-for-word rule meant this spec's rules would travel to subagents once both landed.

## Implementation Notes

From the [visual plan deck spec](../../specs/visual-plan-deck.md), decision D6. Shipped in v0.12.0: `skills/refine/SKILL.md` Phase 6 and `agents/grooming-agent.md` Phase 7.
