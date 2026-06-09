# DevExp Framework — CLAUDE.md

A curated collection of Claude Code agents, skills, hooks, and MCP servers that brings a consistent, expert-level development experience to any project. Install once, distribute to your team.

**Components**: `agents/` (31 agents) · `skills/` (36 skills) · `hooks/` (6 safety guards) · `mcps/` (MCP registry)

> New to a repo? Run `/devxp` first — it orients on the codebase, ensures `CLAUDE.md` and `docs/` exist or are current, and routes you to the right skill next.

---

## Dev Workflow

| Task | Command |
|------|---------|
| Install everything | `./install.sh` |
| Dry-run (preview) | `./install.sh --dry-run` |
| Restart MCP services | `./start-services.sh` |
| Uninstall | `./uninstall.sh` |

After editing any file in `agents/`, `skills/`, or `hooks/` — run `./install.sh` to deploy. The script is idempotent.

---

## Repo Structure

| Directory | What goes here |
|-----------|---------------|
| `agents/<name>.md` | One file per agent — frontmatter + system prompt |
| `skills/<name>/SKILL.md` | One subdirectory per skill |
| `hooks/claude-code/<name>.sh` | Shell hook for Claude Code |
| `hooks/opencode/<name>.js` | JS hook module for opencode |
| `hooks/registry.json` | Source of truth for all hooks |
| `mcps/registry.json` | Source of truth for all MCP servers |
| `templates/` | Starting points for new agents and skills |
| `docs/` | All documentation — start at `docs/README.md` |

---

## Must-Know Gotchas

- **Never call `Agent` tool with a custom agent name as `subagent_type`** — custom agents are role definitions, not spawnable types. Read `agents/<name>.md` and execute its instructions in the current context.
- **Never edit deployed files directly** — always edit source in this repo and re-run `./install.sh`. Deployed files at `~/.claude/agents/` etc. will be overwritten on next install.
- **`devexp-plugin.js` must be updated when adding a hook** — import the new JS module and add it to the `Promise.all([...])` array in `hooks/opencode/devexp-plugin.js`.
- **CLAUDE.md is an indexer only** — directives and navigation pointers only. Content belongs in `docs/`. See [`docs/guides/docs-architecture.md`](docs/guides/docs-architecture.md).

---

## Documentation

Start at [`docs/README.md`](docs/README.md) for the full index.

| I need to know... | Go to |
|---|---|
| Full agent catalog + how agents work | [`docs/reference/agents.md`](docs/reference/agents.md) |
| Full skill catalog + how skills work | [`docs/reference/skills.md`](docs/reference/skills.md) |
| Hooks reference + registry format | [`docs/reference/hooks.md`](docs/reference/hooks.md) |
| MCP registry format + adding MCPs | [`docs/reference/mcps.md`](docs/reference/mcps.md) |
| Install / uninstall / start-services | [`docs/guides/install.md`](docs/guides/install.md) |
| Team distribution + devexp.config.json | [`docs/guides/team-distribution.md`](docs/guides/team-distribution.md) |
| CLAUDE.md-as-indexer pattern | [`docs/guides/docs-architecture.md`](docs/guides/docs-architecture.md) |
| How to write a new agent | [`docs/development/agent-authoring-guide.md`](docs/development/agent-authoring-guide.md) |
| How to write a new skill | [`docs/development/skill-authoring-guide.md`](docs/development/skill-authoring-guide.md) |
| How to write a new hook | [`docs/development/hook-authoring-guide.md`](docs/development/hook-authoring-guide.md) |
| Agent structural conventions | [`docs/development/agent-architecture-reference.md`](docs/development/agent-architecture-reference.md) |
