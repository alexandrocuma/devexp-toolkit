# Agents

Each file in this directory is a Claude Code sub-agent. Install them by running `../install.sh` from the repo root.

Agents are launched by Claude using the `Agent` tool. The `description` field in each agent's frontmatter is what Claude reads to decide when to invoke it — write descriptions carefully.

**Critical:** Custom agent names are not valid `subagent_type` values. To use one of these agents, read `agents/<name>.md` (or `~/.claude/agents/<name>.md` once installed) and follow its instructions directly — never call the `Agent` tool with a custom agent name as `subagent_type`.

---

## Agent Catalog

34 agents covering the full SDLC — discovery, design, implementation, review, release, incidents, and continuous improvement.

Full catalog with purpose and example trigger phrases: [`docs/reference/agents.md`](../docs/reference/agents.md)

---

## Adding a New Agent

1. Copy `templates/agent-template.md` to `agents/<agent-name>.md`
2. Fill in frontmatter: `name`, `description` (with `<example>` blocks), `tools`, `color`
3. Write the system prompt body — follow the style of existing agents
4. Run `./install.sh` to deploy to `~/.claude/agents/`

Full guide: [`docs/development/agent-authoring-guide.md`](../docs/development/agent-authoring-guide.md)
