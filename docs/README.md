# Documentation

Navigation index for all devexp framework documentation.

## Guides

How-to guides for using and configuring the framework.

| Doc | Description |
|-----|-------------|
| [Quick Start](guides/quickstart.md) | Zero to shipped — install, then use the 5 commands end-to-end |
| [Docs Architecture](guides/docs-architecture.md) | CLAUDE.md-as-indexer pattern + standard docs/ folder tree — apply this in every project |
| [Install & Services](guides/install.md) | install.sh flags, start-services.sh, uninstall.sh, and CLI installation paths |
| [Team Distribution](guides/team-distribution.md) | Fork and customise devexp for your organisation via devexp.config.json |
| [Worktree-per-Ticket](guides/worktree-per-ticket.md) | How delivery isolates each ticket in its own git worktree — trigger, naming, lifecycle, merge discipline |

→ Full index: [docs/guides/README.md](guides/README.md)

## Reference

Component catalogs and configuration schemas.

| Doc | Description |
|-----|-------------|
| [Agents](reference/agents.md) | How agents work, full agent catalog (34 agents), adding a new agent |
| [Skills](reference/skills.md) | The 5 user-facing commands, what each orchestrator does, adding a new skill |
| [Hooks](reference/hooks.md) | How hooks work, registry format, hook catalog, CLI compatibility |
| [MCPs](reference/mcps.md) | MCP registry format, MCPs in repo, API keys, docker-backed MCPs |

→ Full index: [docs/reference/README.md](reference/README.md)

## Development

Authoring guides for contributing to or extending the framework.

| Doc | Description |
|-----|-------------|
| [Agent Authoring Guide](development/agent-authoring-guide.md) | Frontmatter, system prompts, examples, and conventions |
| [Skill Authoring Guide](development/skill-authoring-guide.md) | Structure, process definition, output format, archetypes |
| [Hook Authoring Guide](development/hook-authoring-guide.md) | Shell scripts for Claude Code, JS modules for opencode, registry format |
| [Agent Architecture Reference](development/agent-architecture-reference.md) | Phase 0 pattern, graphify/context7 protocols, chaining format |
| [MCP Guide](development/mcp-guide.md) | Adding MCP servers, API keys, CLI compatibility |

→ Full index: [docs/development/README.md](development/README.md)

## Architecture

| Doc | Description |
|-----|-------------|
| [ADR Index](architecture/adr/README.md) | All architecture decisions with status |

→ Full index: [docs/architecture/README.md](architecture/README.md)
