# devexp

A curated collection of Claude Code agents, skills, and MCP servers that bring a consistent, expert-level development experience to any project.

Install once. Get autonomous bug fixes, expert code review, codebase navigation, execution tracing, and security audits — all driven by six lifecycle commands (plus the `/graphify` and `/cleanup` utilities).

---

## Quick Start

```bash
# Install (no clone required)
curl -fsSL https://raw.githubusercontent.com/alexandrocuma/devexp-toolkit/main/scripts/remote-install.sh | bash
```

Then use these commands to cover the full development cycle:

| Command | When |
|---------|------|
| `/devxp` | First time on a repo — orient, create CLAUDE.md and docs/ |
| `/refine` | Turn an idea into a groomed, ready-to-build ticket |
| `/deliver <ticket>` | Implement, test, review, and hand off to release |
| `/release [<ticket>]` | Release phase — merge, changelog, version, tag, publish. Gated. Also finishes a deferred release |
| `/improve` | Sprint end — health scorecard, cleanup, debt triage, retro |
| `/monitor [<surface>]` | Operate — review the deployed system's health, scored, anytime |
| `/cleanup [<ticket>]` | On-demand — retire finished/abandoned worktrees and orphaned delivery artifacts |

```
/devxp  →  /refine  →  /deliver  →  /release  →  /improve  →  next sprint
                                               /monitor  →  review the deployed system, anytime
```

→ Full walkthrough: [docs/guides/quickstart.md](docs/guides/quickstart.md)

---

## What's Included

### Agents

Agents are specialized sub-agents that Claude Code or opencode can spawn to handle domain-specific tasks autonomously — code review, root cause analysis, security audits, migrations, ticket grooming, and more. 34 agents cover the full SDLC, plus an opencode-exclusive swarm orchestrator.

→ Browse the catalog: [`agents/`](agents/) · Full reference: [`docs/reference/agents.md`](docs/reference/agents.md)

### Skills

Six lifecycle orchestrators cover the full development cycle; the `/graphify` and `/cleanup` utilities handle knowledge-graph builds and on-demand artifact retirement. Everything else — ~30 specialist capabilities — runs automatically inside them as agents.

| Command | When to use |
|---------|-------------|
| `/devxp` | First time on a repo — orient, ensure CLAUDE.md and docs/ exist, get routing recommendations |
| `/refine` | Turn a raw idea into a groomed, ready-to-build ticket |
| `/deliver <ticket>` | Implement, test, review, then hand off to `/release` |
| `/release [<ticket>]` | Release phase — gated merge, changelog, version bump, tag, platform release |
| `/improve` | Sprint end — health scorecard, cleanup, debt triage, retro |
| `/monitor [<surface>]` | Operate — review the deployed system's health, scored, anytime |
| `/graphify` | Build a persistent knowledge graph from this codebase |
| `/cleanup [<ticket>]` | Retire finished/abandoned worktrees and orphaned delivery artifacts on demand |

The orchestrators handle everything internally — implementation, testing, code review, instrumentation, release, health checks, and more. You never need to learn sub-commands.

→ Browse the catalog: [`skills/`](skills/) · Full reference: [`docs/reference/skills.md`](docs/reference/skills.md)

### Hooks

Hooks run automatically on matching tool calls — no configuration needed. 10 hooks ship in `hooks/registry.json`: 7 enabled by default — safety guards for secret protection, destructive command blocking and large-file confirmation, plus advisory lint/format/test-on-save — and 3 opt-in `graphify-*` hooks. The installer registers them for **Claude Code** (shell scripts) and installs the **opencode** plugin (JS modules).

→ Browse the catalog: [`hooks/`](hooks/) · Full reference: [`docs/reference/hooks.md`](docs/reference/hooks.md)

### MCP Servers

MCP (Model Context Protocol) servers extend Claude with additional tool capabilities. devexp manages a registry of curated MCP servers and installs them alongside agents, skills, and hooks.

→ Browse the catalog: [`mcps/`](mcps/) · Full reference: [`docs/reference/mcps.md`](docs/reference/mcps.md)

---

## Installation

The Quick Start above is the fastest path for end users — no clone, no Go toolchain. The steps below are for contributors working on devexp itself.

```bash
git clone https://github.com/alexandrocuma/devexp-toolkit.git
cd devexp-toolkit
./install.sh
```

`install.sh` builds the `devexp` Go CLI from `cli/` (only when `bin/devexp` doesn't exist yet) and execs `devexp install`. Because `devexp` reads agents, skills, hooks, and MCPs live from disk when run inside a clone, editing them needs no rebuild — just re-run `./install.sh`. After pulling Go changes under `cli/`, rebuild with `rm bin/devexp && ./install.sh` ([Updating](docs/guides/install.md#updating)).

The installer detects which AI coding CLI(s) you have installed — **Claude Code**, **opencode** and **Kimi Code CLI** — and when more than one is present it asks which to target, in any combination. Without a terminal it installs for all of them. Kimi Code gets MCP servers, agents, skills and hooks, and `./uninstall.sh` removes them again ([details](docs/guides/install.md#kimi-code-cli)).

### Common flags

Any of these flags except `--model` skips the interactive wizard. So does having no terminal: a bare `devexp install` with stdin redirected or piped installs everything for every detected CLI instead of prompting.

```bash
./install.sh --dry-run               # preview what would be installed, no changes made
./install.sh --reinstall-mcps        # remove registry MCPs then re-add them (forces a config refresh)
./install.sh --agents-only           # only install agents
./install.sh --skills-only           # only install skills
./install.sh --mcps-only             # only register MCP servers
./install.sh --agents-only --model sonnet   # also rewrite the model: line of agents that declare one
./install.sh --target claude,opencode       # install only for these CLIs (claude, opencode, kimi)
```

### What gets installed where

`$KIMI` is `$KIMI_CODE_HOME`, or `~/.kimi-code` when unset. MCP servers, agents, skills and hooks are all written there.

| Component | Claude Code | opencode | Kimi Code CLI |
|-----------|-------------|----------|---------------|
| Agents | `~/.claude/agents/` | `~/.config/opencode/agents/` (frontmatter transformed) | `$KIMI/agents/` (frontmatter transformed) |
| Skills | `~/.claude/skills/` | `~/.config/opencode/commands/` (flat `.md`, `name:` stripped) | `$KIMI/skills/<name>/` (with supporting files) |
| Hooks | `~/.claude/settings.json` (shell scripts, per-tool matchers) | `~/.config/opencode/plugins/devexp.js` + `devexp/` | `[[hooks]]` block in `$KIMI/config.toml`, running scripts copied to `$KIMI/hooks/` |
| MCPs | via `claude mcp add` | `~/.config/opencode/config.json` | `mcpServers` key of `$KIMI/mcp.json` |

For Claude Code, existing agents and skills are backed up before any overwrite (the opencode install has no backup step). `install.sh` is idempotent.

### Uninstall

```bash
./uninstall.sh          # interactive — prompts for confirmation
./uninstall.sh --yes    # non-interactive
```

Removes only devexp's agents, skills, hooks, and MCPs. Your own custom agents and skills are untouched. (It doesn't yet remove opencode skills from `~/.config/opencode/commands/` — see [Known gaps](docs/architecture/overview.md#known-gaps).)

→ Full installation guide: [docs/guides/install.md](docs/guides/install.md)

---

## MCP Setup

Most MCP servers work without any configuration. For servers that require API keys:

1. Copy the example env file:
   ```bash
   cp mcps/.env.example mcps/.env
   ```

2. Edit `mcps/.env` and fill in your values:
   ```bash
   SOME_API_KEY=your_key_here
   ```

3. Run `./install.sh` — values are read from `mcps/.env` at install time and stored in the CLI's config.

`mcps/.env` is gitignored. Never commit real secrets.

---

## Usage Examples

### Autonomous bug fix

```
Use the dev-agent to fix the authentication bug — users with special characters
in their email address can't log in.
```

The dev-agent traces the code path, identifies the root cause, implements a fix matching the project's existing patterns, adds a regression test, and reports what changed.

### Code review

```
Use the backend-senior-dev agent to review my new payment processing service.
```

You get a structured review: summary, good patterns identified, critical issues, significant improvements, and a verdict.

### Map a new codebase before starting work

```
Use the codebase-navigator to map this project before we start working.
```

The navigator builds a persistent atlas (saved across sessions) covering stack, architecture, layer naming, conventions, and the canonical example. Every other agent reads this atlas automatically.

### Trace a code path

```
Use the feature-path-tracer to trace what happens when a user submits the
checkout form — happy path only.
```

### Run a full workflow with the orchestrator (opencode)

```
Use the orchestrator to run a full code review on the new payment module.
```

The orchestrator spawns backend-senior-dev, security, and performance agents in parallel and merges their findings.

### Use a skill directly

```
/refine

There's a null pointer exception in the order service when the shipping
address is missing a country code.
```

```
/deliver PAY-123
```

---

## Repo Structure

```
devexp-toolkit/
├── install.sh                  # Thin wrapper — builds the devexp Go CLI and execs `devexp install`
├── uninstall.sh                # Removes devexp components
├── CLAUDE.md                   # Instructions for Claude when working in this repo
├── devexp.config.json          # Team distribution config (model, disabled agents/hooks, custom MCPs)
├── devexp.config.schema.json   # JSON schema for devexp.config.json
├── .goreleaser.yaml             # Release build config (darwin/linux × amd64/arm64)
├── cli/                          # devexp Go CLI source (cobra + promptui)
│   ├── main.go
│   ├── cmd/                      # CLI commands (install, root)
│   └── internal/
│       ├── agents/               # Agent install + opencode frontmatter transform
│       ├── config/               # devexp.config.json loading
│       ├── hooks/                 # Hook registry + install logic
│       ├── mcp/                   # MCP registry + install logic
│       ├── repo/                  # Asset resolution — embedded vs. live filesystem
│       ├── skills/                # Skill install logic
│       └── ui/                    # Interactive prompts
├── agents/                       # 34 agent markdown files (Claude Code format)
│   └── opencode/                 # opencode-exclusive agents (installed with model-line substitution only)
│       └── orchestrator.md
├── skills/                       # 8 user-facing slash commands, each with SKILL.md
│   ├── devxp/
│   ├── refine/
│   ├── deliver/
│   ├── release/
│   ├── improve/
│   ├── monitor/
│   ├── graphify/
│   └── cleanup/
├── hooks/                         # Safety and quality hooks
│   ├── registry.json              # Source of truth for all hooks
│   ├── claude-code/                # Shell scripts registered in ~/.claude/settings.json
│   └── opencode/                   # JS modules composed into a single opencode plugin
├── mcps/                          # MCP server registry and secrets
│   ├── registry.json              # Curated MCP server list (context7, ui-inspector)
│   └── .env.example                # Template for API keys (copy to .env)
├── scripts/                        # Install, build, and packaging helper scripts
│   ├── remote-install.sh           # curl | bash entry point — downloads a release binary
│   └── stage-assets.sh             # Copies agents/skills/hooks/mcps into cli/internal/assets for go:embed
├── templates/                      # Starting points for new agents and skills
│   ├── agent-template.md
│   └── skill-template.md
└── docs/                            # Project documentation — start at docs/README.md
    ├── architecture/
    ├── development/                 # Authoring guides for contributors
    ├── guides/                       # Install, quickstart, team distribution
    └── reference/                    # Full agent, skill, hook, and MCP catalogs
```

---

## Team Distribution

Fork this repo and edit `devexp.config.json` to customise what gets installed for your org — disable agents you don't use, set a default model, and add org-internal MCP servers.

```json
{
  "model": "sonnet",
  "agents": { "disabled": ["scaffold"] },
  "hooks":  { "disabled": ["lint-on-save"] },
  "mcps": [
    {
      "name": "our-internal-docs",
      "command": "npx",
      "args": ["-y", "@our-org/docs-mcp"],
      "required_env": ["ORG_DOCS_TOKEN"]
    }
  ]
}
```

The config is read automatically by `./install.sh` — no extra flags needed. Secrets go in `mcps/.env` (gitignored).

→ Full guide: [`docs/guides/team-distribution.md`](docs/guides/team-distribution.md)

---

## Contributing

Contributions are welcome. Each component type has its own "Adding a New X" guide in its directory's README:

- Agents: [`agents/README.md`](agents/README.md)
- Skills: [`skills/README.md`](skills/README.md)
- Hooks: [`hooks/README.md`](hooks/README.md)
- MCPs: [`mcps/README.md`](mcps/README.md)

General guidance:

1. Use the templates in `templates/` as your starting point.
2. Test thoroughly before submitting a PR.
3. Keep descriptions precise — the `description` field is what Claude reads to decide when to use a skill or agent.

The bar for inclusion: does this provide genuine, reusable value across different projects? Highly project-specific agents and skills are better kept in a project's own `.claude/` directory.
