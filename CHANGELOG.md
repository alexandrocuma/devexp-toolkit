# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
