# Development

Authoring guides for contributing to or extending the devexp framework.

## Files

| File | Description | Status |
|------|-------------|--------|
| [setup.md](setup.md) | Contributor setup: prerequisites, stage/build/run the CLI from source, full dev and test command list, env vars, troubleshooting | ready |
| [conventions.md](conventions.md) | How code is written here: cross-cutting asset rules, Go CLI naming, module structure, error handling, output, config access, style, commits & branches | draft |
| [testing.md](testing.md) | The five CI test suites (Go, Claude Code hook, Kimi hook, opencode hook, installer script), how to write each, pre-commit checklist, coverage & untested areas | draft |
| [agent-authoring-guide.md](agent-authoring-guide.md) | How to write effective agents: frontmatter, system prompts, examples, conventions | ready |
| [skill-authoring-guide.md](skill-authoring-guide.md) | How to write skills: structure, process definition, output format, archetypes | ready |
| [hook-authoring-guide.md](hook-authoring-guide.md) | How to write hooks: shell scripts for Claude Code, JS modules for opencode, the Kimi adapter, per-target registry format | ready |
| [agent-architecture-reference.md](agent-architecture-reference.md) | Structural conventions: Phase 0 pattern, graphify/context7 protocols, chaining format | ready |
| [mcp-guide.md](mcp-guide.md) | How to add MCP servers to the registry, handle API keys, manage CLI compatibility | ready |

## Notes

- `install.sh` now builds and runs the `devexp` Go CLI (`cli/`) — a local Go toolchain is required to run it from a clone. `devexp` reads agents/skills/hooks/MCPs live from disk, so edits take effect immediately without a rebuild.
- The typical development workflow: edit a file in `agents/`, `skills/`, or `hooks/` → `./install.sh` → test in Claude Code or opencode → commit.
- For component reference (catalog of agents, skills, hooks), see [`docs/reference/`](../reference/).
