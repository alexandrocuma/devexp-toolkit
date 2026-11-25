# 0005. Present a plan in at most four diagram-first slides

**Date**: 2026-11-25
**Status**: Accepted
**Deciders**: Alexandro Cu (spec owner)

---

## Context

A text deck (19 slides for one ticket) made reviewers read the whole plan anyway. A first visual dry run still produced 17 slides. A plan that needs that many slides is a sign the feature is too large or the plan is not yet well defined.

## Decision

Each slide leads with its diagram, then a one-line headline and at most three captions, with the slide's full plan text in a closed `<details>`. A deck has at most four slides: **What** (before → after flow), **Where** (change map, blast radius), **How** (step graph), **Why** (up to six decision boards, open questions, the ask). A plan that does not fit — over seven steps, over six decisions, or a diagram over 12 nodes — gets no deck; Phase 6 names what overflowed and offers **split** (back to Phase 4) or **adjust** (re-groom).

## Consequences

### Positive
- A reviewer reads the change by looking, in four slides, and the full text is one click away.
- Deck length becomes a check on the plan itself.

### Negative
- Plans that would have been presented at length are sent back; some may need a split they would otherwise have skipped.

### Neutral
- Diagrams are never split into sub-diagrams to make a plan fit.

## Alternatives Considered

### Alternative 1: Remove the text
**Why not chosen**: Loses the rationale the decision slides exist for.

### Alternative 2: Keep text and add diagrams beside it
**Why not chosen**: Today's deck with pictures; the reading load stays.

### Alternative 3: One slide per decision and step block, with no cap
**Why not chosen**: Produced 17–19 slides; length hid an unclear plan instead of exposing it.

## Implementation Notes

From the [visual plan deck spec](../../specs/visual-plan-deck.md), decision D5 (as amended 2026-11-25). Shipped in v0.12.0: `skills/refine/SKILL.md` Phase 6 and `agents/grooming-agent.md` Phase 7.
