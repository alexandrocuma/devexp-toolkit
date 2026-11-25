# Architecture Decision Records

Decisions that shaped how the devexp framework is built. Read before implementing significant changes.

## Decisions

| ADR | Title | Status | Impact |
|-----|-------|--------|--------|
| [0001](0001-use-hand-written-inline-svg-for-plan-deck-diagrams.md) | Use hand-written inline SVG for plan-deck diagrams | Accepted | `/refine` plan deck, `grooming-agent` plans |
| [0002](0002-derive-plan-deck-diagrams-from-existing-plan-sections.md) | Derive plan-deck diagrams from existing plan sections | Accepted | `/refine` plan deck, `grooming-agent` plans |
| [0003](0003-store-the-change-flow-as-a-plain-text-edge-list.md) | Store a change's flow in the plan as a plain-text edge list | Accepted | `/refine` plan deck, `grooming-agent` plans |
| [0004](0004-fix-the-plan-deck-diagram-catalog-to-plan-headings.md) | Fix the plan-deck diagram catalog to the plan's headings | Accepted | `/refine` plan deck, `grooming-agent` plans |
| [0005](0005-present-a-plan-in-at-most-four-diagram-first-slides.md) | Present a plan in at most four diagram-first slides | Accepted | `/refine` plan deck, `grooming-agent` plans |
| [0006](0006-build-the-visual-deck-on-the-parallel-epic-deck-rewrite.md) | Build the visual deck on the parallel epic-deck rewrite | Accepted | `/refine` plan deck, `grooming-agent` plans |
| [0007](0007-make-plan-deck-diagrams-accessible-by-contract.md) | Make plan-deck diagrams accessible by contract | Accepted | `/refine` plan deck, `grooming-agent` plans |

## Status Values

- **Accepted** — active, follow this
- **Superseded** — replaced by a later ADR (linked in the entry)
- **Deprecated** — no longer applies
- **Proposed** — under discussion, not yet binding

## Notes

When a new architectural decision is made (e.g. adding a new CLI target, changing the hook registration mechanism, adopting a new transport type for MCPs), document it with the `tech-lead` agent (read `agents/tech-lead.md` and follow its ADR phase) — the standalone `/adr` skill was folded into `tech-lead` in `13f3cf8`. `tech-lead` writes the ADR under `docs/adr/` by default, so place it in this folder (`docs/architecture/adr/NNNN-<title>.md`) and add its row to the table above.
