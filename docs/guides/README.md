# Guides

How-to guides for using, configuring, and understanding the devexp framework.

## Files

| File | Description | Status |
|------|-------------|--------|
| [quickstart.md](quickstart.md) | Zero to shipped — install, then use the 6 lifecycle orchestrators to orient, refine, deliver, release, improve, and monitor | ready |
| [docs-architecture.md](docs-architecture.md) | CLAUDE.md-as-indexer pattern + standard docs/ folder tree — apply this in every project | ready |
| [install.md](install.md) | install.sh, uninstall.sh — flags, behavior, CLI paths | ready |
| [team-distribution.md](team-distribution.md) | Forking and customizing devexp for your organisation via devexp.config.json | ready |
| [workflows.md](workflows.md) | Step-by-step recipes with real paths: add an agent/skill/hook/MCP, change the Go CLI, fix a bug, change registry/config schemas, add config or a dependency, ship | ready |
| [worktree-per-ticket.md](worktree-per-ticket.md) | Worktree-per-ticket delivery convention — trigger, naming scheme, lifecycle, failure handling, merge discipline | ready |
| [release-targets.md](release-targets.md) | Release targets and the per-repo release guide — cut vs. ship, per-target gates, target states, which lifecycle command reads what | ready |
| [release.md](release.md) | This repo's release guide: `cli` target (tag → goreleaser → GitHub Releases) and `toolkit-clone` target (`main`), cut, promote, rollback, post-release checks | draft |
| [cleanup-safety.md](cleanup-safety.md) | Canonical deletion-safety rules for toolkit cleanup — dry-run, scoped id guards, shared-state protection, staleness-gated memory pruning | ready |

## Notes

- `docs-architecture.md` is the canonical reference for the CLAUDE.md indexer pattern — link to it from any project that adopts this framework.
