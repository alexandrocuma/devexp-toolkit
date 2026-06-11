# devexp

A curated collection of Claude Code agents, skills, and MCP servers that bring a consistent, expert-level development experience to any project.

Install once. Get autonomous bug fixes, expert code review, codebase navigation, execution tracing, and security audits — all driven by four commands that cover the full development lifecycle.

---

## Quick Start

```bash
# Install (no clone required)
curl -fsSL https://raw.githubusercontent.com/alexandrocuma/devexp-toolkit/main/scripts/remote-install.sh | bash
```

Then use four commands to cover the full development cycle:

| Command | When |
|---------|------|
| `/devxp` | First time on a repo — orient, create CLAUDE.md and docs/ |
| `/refine` | Turn an idea into a groomed, ready-to-build ticket |
| `/deliver <ticket>` | Implement, test, review, and release a ticket |
| `/improve` | Sprint end — health scorecard, cleanup, debt triage, retro |

```
/devxp  →  /refine  →  /deliver  →  /improve  →  next sprint
```

→ Full walkthrough: [docs/guides/quickstart.md](docs/guides/quickstart.md)

---

## What's Included

### Agents

Agents are specialized sub-agents that Claude Code or opencode can spawn to handle domain-specific tasks autonomously — code review, root cause analysis, security audits, migrations, ticket grooming, and more. 34 agents cover the full SDLC, plus an opencode-exclusive swarm orchestrator.

→ Browse the catalog: [`agents/`](agents/) · Full reference: [`docs/reference/agents.md`](docs/reference/agents.md)

### Skills

Five slash commands cover the full development lifecycle. Everything else — ~30 specialist capabilities — runs automatically inside them as agents.

| Command | When to use |
|---------|-------------|
| `/devxp` | First time on a repo — orient, ensure CLAUDE.md and docs/ exist, get routing recommendations |
| `/refine` | Turn a raw idea into a groomed, ready-to-build ticket |
| `/deliver <ticket>` | Implement, test, review, and release a ticket end-to-end |
| `/improve` | Sprint end — health scorecard, cleanup, debt triage, retro |
| `/graphify` | Build a persistent knowledge graph from this codebase |

The orchestrators handle everything internally — implementation, testing, code review, instrumentation, release, health checks, and more. You never need to learn sub-commands.

→ Browse the catalog: [`skills/`](skills/) · Full reference: [`docs/reference/skills.md`](docs/reference/skills.md)

### Hooks

Hooks are safety and quality guards that run automatically on every tool call — no configuration needed. They work identically in Claude Code (shell scripts) and opencode (JS plugin modules). 10 hooks cover secret protection, destructive command blocking, large-file confirmation, and lint/format/test-on-save.

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

`install.sh` builds the `devexp` Go CLI from `cli/` and execs `devexp install`. Because `devexp` reads agents, skills, hooks, and MCPs live from disk when run inside a clone, local edits take effect immediately — no rebuild needed.

The installer detects which AI coding CLI(s) you have installed and asks which to target — **Claude Code**, **opencode**, or both.

### Common flags

```bash
./install.sh --dry-run               # preview what would be installed, no changes made
./install.sh --model sonnet          # skip the model prompt
./install.sh --reinstall-mcps        # remove registry MCPs then re-add them (forces a config refresh)
./install.sh --agents-only           # only install agents
./install.sh --skills-only           # only install skills
./install.sh --mcps-only             # only register MCP servers
```

### What gets installed where

| Component | Claude Code | opencode |
|-----------|-------------|----------|
| Agents | `~/.claude/agents/` | `~/.config/opencode/agents/` (frontmatter transformed) |
| Skills | `~/.claude/skills/` | `~/.config/opencode/commands/` (flat `.md`, `name:` stripped) |
| Hooks | `~/.claude/settings.json` (shell scripts, per-tool matchers) | `~/.config/opencode/plugins/devexp-plugin.js` (JS modules) |
| MCPs | via `claude mcp add` | `~/.config/opencode/config.json` |

Existing files are backed up automatically before any overwrite. `install.sh` is idempotent.

### Restart services

After a machine restart or session, MCP services may have stopped:

```bash
./start-services.sh            # start anything that isn't running
./start-services.sh --status   # check service health without starting
```

### Uninstall

```bash
./uninstall.sh          # interactive — prompts for confirmation
./uninstall.sh --yes    # non-interactive
```

Removes only devexp's agents, skills, hooks, and MCPs. Your own custom agents and skills are untouched.

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
/bugfix

There's a null pointer exception in the order service when the shipping
address is missing a country code.
```

```
/commit

I fixed the encoding bug in the payment processor — special characters in
names no longer cause payment failures.
```

---

## Repo Structure

```
devexp-toolkit/
├── install.sh                  # Thin wrapper — builds the devexp Go CLI and execs `devexp install`
├── uninstall.sh                # Removes devexp components
├── start-services.sh           # Starts/checks MCP services (ui-inspector)
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
│   └── opencode/                 # opencode-exclusive agents (installed as-is)
│       └── orchestrator.md
├── skills/                       # 5 user-facing slash commands, each with SKILL.md
│   ├── devxp/
│   ├── refine/
│   ├── deliver/
│   ├── improve/
│   └── graphify/
├── hooks/                         # Safety and quality hooks
│   ├── registry.json              # Source of truth for all hooks
│   ├── claude-code/                # Shell scripts registered in ~/.claude/settings.json
│   └── opencode/                   # JS modules composed into a single plugin
├── mcps/                          # MCP server registry and secrets
│   ├── registry.json              # Curated MCP server list (context7, ui-inspector)
│   ├── .env.example                # Template for API keys (copy to .env)
│   └── ui-inspector/                # ui-inspector MCP server (headless Chromium via Playwright)
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
