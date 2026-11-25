# 0004. Fix the plan-deck diagram catalog to the plan's headings

**Date**: 2026-11-25
**Status**: Accepted
**Deciders**: Alexandro Cu (spec owner)

---

## Context

Decks that each pick their own diagrams look different, vary in quality and give reviewers no fixed place to look. A deck can only draw from sections it can find, and plan headings used to be written by habit.

## Decision

Phase 6 has a fixed catalog: flow ← `## Diagrams`, change map ← `## Files that change` + `## Files confirmed NOT changing`, blast radius ← `## Risks` + `## Affected Release Targets`, step graph ← `## Execution Steps`, decision board ← `## Decisions`. `grooming-agent` Phase 7 writes exactly those seven level-2 headings. Older plans are read by content.

## Consequences

### Positive
- Every deck has the same diagrams in the same place.
- The two sides are checked: `TestDeckCatalog_MatchesPlanTemplate` fails when the catalog names a heading Phase 7 no longer writes.

### Negative
- Rewording a heading on one side alone now fails CI, by design.

### Neutral
- Pinning the headings also let `/release` read the targets section by its heading instead of any mention of it.

## Alternatives Considered

### Alternative 1: Pick diagrams per plan
**Why not chosen**: Each deck becomes a design exercise, quality varies, and there is nothing to test.

## Implementation Notes

From the [visual plan deck spec](../../specs/visual-plan-deck.md), decision D4. Shipped in v0.12.0: `skills/refine/SKILL.md` Phase 6 and `agents/grooming-agent.md` Phase 7.
