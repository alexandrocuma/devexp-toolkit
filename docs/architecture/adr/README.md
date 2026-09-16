# Architecture Decision Records

Decisions that shaped how the devexp framework is built. Read before implementing significant changes.

## Decisions

| ADR | Title | Status | Impact |
|-----|-------|--------|--------|
| — | No ADRs yet | — | — |

## Status Values

- **Accepted** — active, follow this
- **Superseded** — replaced by a later ADR (linked in the entry)
- **Deprecated** — no longer applies
- **Proposed** — under discussion, not yet binding

## Notes

When a new architectural decision is made (e.g. adding a new CLI target, changing the hook registration mechanism, adopting a new transport type for MCPs), document it with the `tech-lead` agent (read `agents/tech-lead.md` and follow its ADR phase) — the standalone `/adr` skill was folded into `tech-lead` in `13f3cf8`. `tech-lead` writes the ADR under `docs/adr/` by default, so place it in this folder (`docs/architecture/adr/NNNN-<title>.md`) and add its row to the table above.
