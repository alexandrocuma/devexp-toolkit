# Documentation

Navigation index for all devexp framework documentation.

## Development Kit — start here

Contributing to this repo? Read in order: [Architecture Overview](architecture/overview.md) → [Setup](development/setup.md) → [Conventions](development/conventions.md) → [Testing](development/testing.md) → [Development Workflows](guides/workflows.md) → [Release Guide](guides/release.md). Each is also listed in its section below.

## Guides

How-to guides for using and configuring the framework.

| Doc | Description |
|-----|-------------|
| [Quick Start](guides/quickstart.md) | Zero to shipped — install, then use the lifecycle commands end-to-end |
| [Docs Architecture](guides/docs-architecture.md) | CLAUDE.md-as-indexer pattern + standard docs/ folder tree — apply this in every project |
| [Install & Services](guides/install.md) | install.sh flags, uninstall.sh, and CLI installation paths |
| [Team Distribution](guides/team-distribution.md) | Fork and customise devexp for your organisation via devexp.config.json |
| [Development Workflows](guides/workflows.md) | Step-by-step recipes with real paths: add an agent/skill/hook/MCP, change the Go CLI, fix a bug, change registry/config schemas, add config or a dependency, ship |
| [Worktree-per-Ticket](guides/worktree-per-ticket.md) | How delivery isolates each ticket in its own git worktree — trigger, naming, lifecycle, merge discipline |
| [Release Targets](guides/release-targets.md) | How releases ship to deploys, app stores and registries via the per-repo release guide — cut vs. ship, gates, target states |
| [Release Guide](guides/release.md) | This repo's release guide: `cli` target (tag → goreleaser → GitHub Releases) and `toolkit-clone` target (`main`), cut, promote, rollback, post-release checks |
| [Cleanup Safety](guides/cleanup-safety.md) | Rules for safely deleting artifacts — dry-run, id guards, never touch shared state — followed by deliver C1, improve C2, and the /cleanup command |

→ Full index: [docs/guides/README.md](guides/README.md)

## Reference

Component catalogs and configuration schemas.

| Doc | Description |
|-----|-------------|
| [Agents](reference/agents.md) | How agents work, full agent catalog (34 agents), adding a new agent |
| [Skills](reference/skills.md) | The 8 user-facing commands, what each orchestrator does, adding a new skill |
| [Hooks](reference/hooks.md) | How hooks work, registry format, hook catalog, CLI compatibility |
| [MCPs](reference/mcps.md) | MCP registry format, MCPs in repo, API keys and secrets, server lifecycle, CLI compatibility |

→ Full index: [docs/reference/README.md](reference/README.md)

## Development

Authoring guides for contributing to or extending the framework.

| Doc | Description |
|-----|-------------|
| [Setup](development/setup.md) | Contributor setup: prerequisites, stage/build/run the CLI from source, full dev and test command list, env vars, troubleshooting |
| [Conventions](development/conventions.md) | How code is written here: cross-cutting asset rules, Go CLI naming, module structure, error handling, output, config access, style, commits & branches |
| [Testing](development/testing.md) | The four CI test suites (Go, Claude Code hook, opencode hook, installer script), how to write each, pre-commit checklist, coverage & untested areas |
| [Agent Authoring Guide](development/agent-authoring-guide.md) | Frontmatter, system prompts, examples, and conventions |
| [Skill Authoring Guide](development/skill-authoring-guide.md) | Structure, process definition, output format, archetypes |
| [Hook Authoring Guide](development/hook-authoring-guide.md) | Shell scripts for Claude Code, JS modules for opencode, registry format |
| [Agent Architecture Reference](development/agent-architecture-reference.md) | Phase 0 pattern, graphify/context7 protocols, chaining format |
| [MCP Guide](development/mcp-guide.md) | Adding MCP servers, API keys, CLI compatibility |

→ Full index: [docs/development/README.md](development/README.md)

## Architecture

| Doc | Description |
|-----|-------------|
| [Architecture Overview](architecture/overview.md) | How the system is organised: asset tree + Go installer CLI, layer map, traced install flow for both targets, runtime hook flow, external deps, known gaps, reference implementation |
| [ADR Index](architecture/adr/README.md) | All architecture decisions with status |

→ Full index: [docs/architecture/README.md](architecture/README.md)
