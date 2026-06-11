# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
