# Architecture

Architecture documentation and decisions for the devexp framework.

## Files

| File | Description | Status |
|------|-------------|--------|
| [overview.md](overview.md) | How the system is organised: asset tree + Go installer CLI, layer map, traced install flow for all three targets (Claude Code, opencode, Kimi Code CLI), runtime hook flow, external deps, known gaps, reference implementation | ready |
| [ADR Index](adr/README.md) | All architecture decision records | ready |

## Notes

- ADRs capture decisions that are **not** obvious from reading the code — the why, not just the what.
- Before adding a significant new pattern (new hook mechanism, new CLI target, new MCP type), write an ADR first.
- For implementation conventions derived from architectural choices, see [`docs/development/`](../development/).
