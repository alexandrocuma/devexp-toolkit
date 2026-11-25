# 0003. Store a change's flow in the plan as a plain-text edge list

**Date**: 2026-11-25
**Status**: Accepted
**Deciders**: Alexandro Cu (spec owner)

---

## Context

Plans are read by agents that build from them and are copied to the ticket platform. The before → after flow must stay readable there, and the toolkit names no specific tool or library.

## Decision

`grooming-agent` writes `## Diagrams` with `### Flow: before` and `### Flow: after`, each a fenced `text` block of `a -> b : label` edges, one per line, in the plan's own terms. A node keeps one spelling across both flows. Nodes only in *after* are new, only in *before* removed. Edges compare by their two ends, so a changed label is a changed edge. A change with no runtime flow writes `Not applicable — <why>`.

## Consequences

### Positive
- Readable as plain text on every ticket platform; the fence stops `<id>`, `_` and `*` becoming markup.
- Trivial to parse and draw; edges are read only inside the fences.

### Negative
- Node identity is spelling alone, so an inconsistent spelling reads as one removed node and one new one.

### Neutral
- `TestGroomingAgent_DiagramsFormat` fails when Phase 7 stops describing this form.

## Alternatives Considered

### Alternative 1: Mermaid syntax in the plan
**Why not chosen**: Ties the plan to one tool, against the toolkit's tool-agnostic rule.

### Alternative 2: SVG in the plan
**Why not chosen**: Unreadable to the agents that build from the plan.

## Implementation Notes

From the [visual plan deck spec](../../specs/visual-plan-deck.md), decision D3. Shipped in v0.12.0: `skills/refine/SKILL.md` Phase 6 and `agents/grooming-agent.md` Phase 7.
