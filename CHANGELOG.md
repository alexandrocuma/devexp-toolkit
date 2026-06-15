# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.0] - 2026-06-15

Epic #54 — `/monitor`, a new operate-phase orchestrator for reviewing the health of deployed systems.

### Added

- **`/monitor` — deployed-system health review (5th orchestrator).** A new operate-phase command that assesses the *running* system rather than the codebase. It detects stack surfaces by category (cloud/infra, dashboards, logging, alerting, tracing/metrics) from repo signals and already-authenticated connectors, reviews each surface live (read-only connector query) or via a config-as-code fallback, and produces a change-independent, equal-weighted composite health score with a ranked, actionable anomaly list. Vendor-agnostic (names no platform in its prompt) and credential-safe (never triggers auth or stores secrets). `/monitor <surface>` scopes the review to a single detected surface. (#55, #56, #57, #58)
- **Persisted health review + `/improve` reconciliation.** `/monitor` persists its scored report to `.devexp/system-health-review.md`; `/improve`'s Observability Maturity dimension now defers to that artifact when present — one home for "is the system healthy?" The existing `observability` baseline key is reused for trends with no schema migration. (#59)

### Changed

- Documentation surface reframed from five commands to six (five lifecycle orchestrators — `/devxp`, `/refine`, `/deliver`, `/improve`, `/monitor` — plus the `/graphify` utility) across the skills catalog, READMEs, CLAUDE.md, and quickstart. (#60)

## [0.4.0] - 2026-06-15

Epic #32 — fresh memory, worktree-per-ticket delivery, and completion cleanup.

### Added

- **Worktree-per-ticket delivery.** `/deliver` isolates each ticket in its own git worktree (create → work → merge-at-release-gate → remove), with a single-stream fallback; each epic sub-ticket gets its own worktree so independent work runs in parallel. `/improve` isolates parallel cleanup streams the same way. New `docs/guides/worktree-per-ticket.md` convention. (#36, #37, #38)
- **Memory freshness.** `codebase-navigator`'s coarse 30-day rebuild window is replaced by a canonical Drift Classification (CURRENT/SMALL/BIG keyed on *what changed*, not *how long ago*); `dev-agent` and `grooming-agent` run a cheap freshness gate before trusting the atlas; persisted plans are stamped with the commit they were validated against, and `/deliver` re-grooms on big drift. (#33, #34, #35)
- **Completion cleanup.** `/deliver` gains a final phase that retires the ticket's artifacts (worktree, persisted plan, groom session, `/tmp` scratch) on successful completion; `/improve` gains a repo-wide hygiene sweep for orphaned artifacts. Both follow the new `docs/guides/cleanup-safety.md` deletion-safety rules (dry-run, validated id guards, prefix-anchored globs). (#39, #40)
- Allow/deny test suites for the `dangerous-cmd-guard` hook (claude-code and opencode). (#50)

### Changed

- `dangerous-cmd-guard` now blocks unanchored wildcard deletes in sensitive directories (`/tmp/*`, `~/.claude/.../*`, and quoted variants) as an execution-time backstop for the cleanup phases. (#50)
- Toolkit-internal `docs/` references in the orchestrator skills are labeled as maintainer-only, since `docs/` is not installed into a user's environment. (#51)

### Fixed

- `dangerous-cmd-guard` no longer false-positives on a force flag that appears elsewhere in a command (e.g. a short flag inside a commit message); the force-push rule now matches only an argument of the same push command. (#50)

## [0.3.0] - 2026-06-13

### Added

- `devexp install` now performs manifest-based stale-file cleanup: each run records the agent/skill files it installs in `~/.claude/.devexp-manifest.json` (and `~/.config/opencode/.devexp-manifest.json` for opencode), and removes any from a prior run that are no longer shipped in the current version.
- Skill directories are now backed up to the timestamped backup folder before being overwritten, matching the existing behavior for agents.
- Stale hook pruning: hook entries whose backing script no longer exists on disk (because the hook was removed from `hooks/registry.json`) are automatically dropped from `settings.json`; user-authored hooks are left untouched.
- "Updating" section added to `docs/guides/install.md` documenting the update path, what's overwritten vs. preserved, stale-file/hook cleanup, and the one-time pre-manifest baseline caveat.

### Changed

- Disabling an agent or skill in `devexp.config.json` now removes it from disk on the next install (previously it only skipped updates, leaving the old copy in place).

### Fixed

- CI: bumped `actions/checkout` to v6 and `actions/setup-go` to v6 (with `cache-dependency-path: cli/go.sum`) in both workflows, and `goreleaser/goreleaser-action` to v7 — resolves the Node 20 deprecation warning and go.sum cache-restore failure seen on the v0.2.0 release run.

## [0.2.0] - 2026-06-13

### Added

- Canonical **Persistent Agent Memory** section rolled out to 25 agents (8 Category B, 9 analysis, 8 workflow) — every memory-enabled agent now follows a consistent format for storing and recalling project-specific context across sessions.
- "Persistent Agent Memory" checklist item added to the Agent File Checklist so new agents adopt the canonical pattern by default.
- Agent-memory duplication mapping convention: documents `~/.claude/agent-memory/graphify-out/` and wires a manual cross-agent duplication check into `/improve` Phase 3.
- Table-driven test coverage for the `cli/` Go module (previously 0%), covering `cli/cmd/install.go`, `cli/internal/config`, and `cli/internal/repo`.

### Changed

- Go table-driven tests now use `map[string]struct{}` keyed by test name across the `cli/` test suite.
- Removed 9 Python install scripts superseded by the Go CLI (`devexp install`); `install.sh` now defers entirely to `bin/devexp`.

### Fixed

- Atlas freshness checks: `/devxp` and codebase-navigator now validate dated Gotchas/Technical Debt entries against git history before trusting them, and feature-path-tracer's memory follows the index+topic-file convention instead of duplicating the atlas. codebase-navigator self-heals old-format atlases on the next run.
- CI: stage embedded assets before running `go test` so embed-dependent tests pass.
- Bumped Go toolchain to 1.25.11 and Cobra/Viper to v1.10.2/v1.21.0 to resolve stdlib and dependency CVEs.

## [0.1.0] - 2026-06-11

Initial release.

### Added

- `devexp` Go CLI (`cli/`) — installs agents, skills, hooks, and MCPs for Claude Code and opencode. No-clone installation via `curl | bash`, which downloads a release binary and runs `devexp install`.
- 34 specialist agents covering the full SDLC — implementation (`dev-agent`), code review (`backend-senior-dev`, `frontend-senior-dev`, `pr-review`), analysis (`root-cause`, `arch-review`, `impact-analysis`, `data-flow`), quality (`security`, `performance`, `test-gen`, `test-runner`, `dep-audit`), release (`changelog`, `ci-cd`, `postmortem`), and more — plus an opencode-exclusive swarm orchestrator.
- 5 user-facing slash commands covering the full development lifecycle: `/devxp` (orient), `/refine` (groom tickets), `/deliver` (implement, test, review, release), `/improve` (health check, cleanup, retro), and `/graphify` (build a persistent knowledge graph).
- 10 safety/quality hooks — secret protection, destructive command blocking, large-file confirmation, lint/format/test-on-save, plus an optional `graphify-*` set — implemented for both Claude Code (shell scripts) and opencode (JS plugin).
- Curated MCP server registry (`context7`, `ui-inspector`) with automatic install and Docker-backed service support.
- Team distribution via `devexp.config.json` — disable agents/hooks, set a default model, and register org-internal MCPs.
- GoReleaser-based release pipeline producing darwin/linux (amd64/arm64) binaries.
