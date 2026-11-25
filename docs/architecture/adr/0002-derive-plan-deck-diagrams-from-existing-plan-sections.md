# 0002. Derive plan-deck diagrams from existing plan sections

**Date**: 2026-11-25
**Status**: Accepted
**Deciders**: Alexandro Cu (spec owner)

---

## Context

The plan deck may not add claims the plan lacks: a reviewer approves the plan, not the deck. Files, steps, decisions and risks are already structured plan sections; the before → after flow is the one picture they cannot produce.

## Decision

Every diagram except the flow is derived from a plan section that already exists. Only the flow is new plan content, written by `grooming-agent`.

## Consequences

### Positive
- The deck cannot drift from the plan it presents; every node traces to a plan item, checked per node before writing.
- Grooming writes one extra section, not a set of diagrams.

### Negative
- A plan missing a source section gives a slide that says so instead of a diagram; the remedy is re-grooming.

### Neutral
- None recorded

## Alternatives Considered

### Alternative 1: Let the deck author draw whatever explains best
**Why not chosen**: Breaks the rule that the deck never extends the plan.

### Alternative 2: Have grooming write every diagram
**Why not chosen**: Duplicates what the plan's sections already say.

## Implementation Notes

From the [visual plan deck spec](../../specs/visual-plan-deck.md), decision D2. Shipped in v0.12.0: `skills/refine/SKILL.md` Phase 6 and `agents/grooming-agent.md` Phase 7.
