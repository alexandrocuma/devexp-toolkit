# Development

Authoring guides for contributing to or extending the devexp framework.

## Files

| File | Description | Status |
|------|-------------|--------|
| [agent-authoring-guide.md](agent-authoring-guide.md) | How to write effective agents: frontmatter, system prompts, examples, conventions | ready |
| [skill-authoring-guide.md](skill-authoring-guide.md) | How to write skills: structure, process definition, output format, archetypes | ready |
| [hook-authoring-guide.md](hook-authoring-guide.md) | How to write hooks: shell scripts for Claude Code, JS modules for opencode, registry format | ready |
| [agent-architecture-reference.md](agent-architecture-reference.md) | Structural conventions: Phase 0 pattern, OpenViking/context7 protocols, chaining format | ready |
| [mcp-guide.md](mcp-guide.md) | How to add MCP servers to the registry, handle API keys, manage CLI compatibility | ready |

## Notes

- All services run inside Docker — you do not need Go, Bun, or other runtimes installed locally to work in this repo.
- The typical development workflow: edit a file in `agents/`, `skills/`, or `hooks/` → `./install.sh` → test in Claude Code or opencode → commit.
- For component reference (catalog of agents, skills, hooks), see [`docs/reference/`](../reference/).
