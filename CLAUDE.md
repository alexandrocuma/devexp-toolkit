# DevExp Framework — CLAUDE.md

This file gives you everything you need to work effectively in this repository.

## What This Repo Is

This is the **devexp framework** — a curated collection of Claude Code agents and skills that improve the development experience. The repo is designed to be cloned and installed, distributing a consistent set of AI-powered development capabilities to any machine.

The framework has two types of components:

- **Agents** — specialized Claude sub-agents that handle domain-specific tasks (code review, codebase navigation, autonomous implementation, frontend review, execution tracing). Stored in `agents/`.
- **Skills** — reusable slash-command behaviors that Claude can invoke mid-conversation. Stored in `skills/`. Each skill is an instruction set that shapes how Claude behaves when invoked.

---

## How Agents Work

Agents live as Markdown files in `~/.claude/agents/` on the user's machine. Each file is loaded by Claude Code and made available as a sub-agent that can be launched with the `Agent` tool.

### File Format

Every agent file is a Markdown file with YAML frontmatter followed by a system-prompt body:

```markdown
---
name: my-agent
description: "One-line description of when to use this agent and what it does.

<example>
Context: ...
user: ...
assistant: ...
</example>"
tools: Read, Write, Edit, Bash, Glob, Grep, Agent, WebFetch, WebSearch, Skill
model: sonnet
color: cyan
memory: user
---

# Agent body / system prompt

Full instructions for how this agent should behave...
```

### Frontmatter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Identifier used to reference the agent |
| `description` | Yes | When-to-use guidance shown to the orchestrating agent. Include `<example>` blocks — they dramatically improve invocation accuracy. |
| `tools` | Yes | Comma-separated list of tools the agent can access |
| `model` | No | `sonnet` (default) or `opus` |
| `color` | No | Terminal color for identification: `cyan`, `green`, `yellow`, `red`, `purple`, `blue` |
| `memory` | No | Set to `user` to give the agent persistent memory across sessions |

### Agents in This Repo

| File | Agent Name | Purpose |
|------|-----------|---------|
| `arch-review.md` | arch-review | Deep architectural health assessment with scored findings |
| `backend-senior-dev.md` | backend-senior-dev | Expert backend code review and architecture analysis |
| `codebase-navigator.md` | codebase-navigator | Builds and maintains a shared codebase atlas for all agents |
| `dep-map.md` | dep-map | Maps module and package dependencies, detects cycles |
| `dev-agent.md` | dev-agent | Autonomous implementation: bugs, features, refactors |
| `feature-path-tracer.md` | feature-path-tracer | Traces a single execution path through code |
| `frontend-senior-dev.md` | frontend-senior-dev | Expert frontend code review and UI architecture guidance |
| `migration.md` | migration | Plan and execute library/framework/runtime version migrations |
| `performance.md` | performance | Performance bottleneck identification and optimization |
| `pr-review.md` | pr-review | Thorough PR review across bugs, security, patterns, and tests |
| `root-cause.md` | root-cause | Deep root cause analysis using 5-Whys and hypothesis testing |
| `security.md` | security | Full security audit: OWASP Top 10, auth, data exposure |
| `test-gen.md` | test-gen | Generate comprehensive test suites for untested code |
| `test-runner.md` | test-runner | Test execution, coverage analysis, flaky test detection |
| `project-manager.md` | project-manager | GitHub Issue creation, epic decomposition, backlog triage |
| `scaffold.md` | scaffold | Pattern-matched code generation for new modules, services, and components |
| `changelog.md` | changelog | Changelog and release notes generation from git history |
| `ci-cd.md` | ci-cd | CI/CD pipeline debugging, creation, and optimization |
| `postmortem.md` | postmortem | Structured blameless incident postmortem documents |
| `tech-lead.md` | tech-lead | Architecture Decision Records, design review, engineering standards |

**opencode-exclusive agents** live in `agents/opencode/` — written directly in opencode frontmatter format, installed as-is (no transformation). They use opencode-only capabilities like the Task tool for true parallel subagent spawning.

| File | Agent Name | Purpose |
|------|-----------|---------|
| `opencode/orchestrator.md` | orchestrator | Swarm orchestrator: spawns specialist agents in parallel via Task tool (13 workflow presets) |

---

## How Skills Work

Skills live as Markdown files at `~/.claude/skills/<name>/skill.md`. Each skill is invoked using a slash command: `/skill-name` in Claude Code.

When a skill is invoked, its `skill.md` content is injected into the conversation context, shaping Claude's behavior for that task.

### File Format

```markdown
---
name: my-skill
description: One-line description of what this skill does
---

# Skill Title

Body: instructions, process steps, output format...
```

### Skills in This Repo

| Skill | Slash Command | Description |
|-------|---------------|-------------|
| bugfix | `/bugfix` | Root cause analysis and bug fixing with built-in verification |
| commit | `/commit` | Craft a conventional commit message and create the commit |
| explain | `/explain` | Explain code to a specific audience (junior, new-hire, non-technical) |
| docs | `/docs` | Documentation generation (API docs, comments, examples, README) |
| api-design | `/api-design` | Designs API contracts, endpoints, schemas, and error handling |
| db-design | `/db-design` | Designs database schemas, migrations, and indexes |
| feature | `/feature` | Spec-driven feature implementation with tests and documentation |
| logic-review | `/logic-review` | Reviews code logic for bugs, edge cases, and dysfunction |
| migrate | `/migrate` | Step-by-step migration guide for a library or framework upgrade |
| pr | `/pr` | Generate a PR description and optionally open the PR via gh |
| quality | `/quality` | Reviews code quality, style, complexity, and maintainability |
| refactor | `/refactor` | Code refactoring for improved structure and maintainability |
| regression | `/regression` | Ensures fixes don't introduce regressions |
| standup | `/standup` | Generate a daily standup update from recent git activity |
| test-gen | `/test-gen` | Generate tests for the current file or function |
| adr | `/adr` | Write an Architecture Decision Record saved to docs/adr/ |
| changelog | `/changelog` | Generate a changelog entry from git history using conventional commits |
| release | `/release` | Full release workflow: version bump, changelog, tag, and GitHub release |
| postmortem | `/postmortem` | Generate a structured blameless postmortem document |
| ticket | `/ticket` | Create a well-structured GitHub Issue for a bug, feature, or tech-debt item |
| scope | `/scope` | Break a large feature or epic into atomic tickets with dependencies |
| health | `/health` | Generate a codebase health scorecard with RAG status per dimension |
| init-claude | `/init-claude` | Crawl a project's docs and codebase to generate a directive CLAUDE.md with architecture map, conventions, and implementation playbooks |

---

## Install and Uninstall

The installer is CLI-agnostic. It detects which AI coding CLI(s) are installed and asks which to target. Supported CLIs: **Claude Code** and **opencode**.

### install.sh

```bash
./install.sh                     # interactive
./install.sh --dry-run           # preview what would be installed, no changes made
./install.sh --model sonnet      # skip model prompt, use claude-sonnet-4-6
./install.sh --model opus        # skip model prompt, use claude-opus-4-6
```

Behavior:
- Detects `claude` and/or `opencode` in PATH and prompts which to install for
- **Claude Code**: copies agents to `~/.claude/agents/`, skills to `~/.claude/skills/`, registers MCPs via `claude mcp add`
- **opencode**: transforms agent frontmatter (model aliases, tool mapping, adds `mode: subagent`) and installs to `~/.config/opencode/agents/`; skills go to `~/.claude/skills/` (opencode reads this path natively); MCPs are written to `~/.config/opencode/config.json`
- Backs up any conflicting files before overwriting
- If neither CLI is detected, still allows manual target selection

### uninstall.sh

```bash
./uninstall.sh          # interactive (prompts for confirmation)
./uninstall.sh --yes    # non-interactive
```

Behavior:
- Detects which CLIs have devexp installed and asks which to remove from
- Removes agents from the appropriate directory for each CLI
- Skills (`~/.claude/skills/`) are only removed if uninstalling from all CLIs that use them

### CLI compatibility notes

| Component | Claude Code | opencode |
|-----------|-------------|----------|
| Agents | `~/.claude/agents/` | `~/.config/opencode/agents/` (transformed) |
| Skills | `~/.claude/skills/` | `~/.claude/skills/` (same path, read natively) |
| `CLAUDE.md` | `~/.claude/CLAUDE.md` | read as fallback if no `AGENTS.md` exists |
| Agent tools | All Claude tools | `read/write/edit/bash/glob/grep/webfetch/websearch` only |
| `Agent`, `Skill`, `Task*` tools | Supported | No opencode equivalent — dropped at transform |

---

## MCP Servers

MCP servers extend Claude's capabilities with external tools (documentation lookup, databases, APIs, etc.). The devexp framework manages MCPs alongside agents and skills.

### Registry

MCP servers are declared in `mcps/registry.json`:

```json
[
  {
    "name": "context7",
    "description": "Up-to-date library documentation for any package",
    "command": "npx",
    "args": ["-y", "@upstash/context7-mcp"],
    "scope": "user",
    "env": {},
    "required_env": []
  }
]
```

| Field | Description |
|-------|-------------|
| `name` | Unique MCP identifier |
| `description` | What this MCP provides |
| `command` | Executable to run |
| `args` | Arguments passed to the command |
| `scope` | `"user"` (global) or `"project"` (Claude Code only) |
| `env` | Static environment variables to pass |
| `required_env` | Env vars that must be set — MCP is skipped with a warning if missing |

### MCPs in This Repo

| Name | Description |
|------|-------------|
| context7 | Up-to-date library documentation and code examples for any package |

### API Keys and Secrets

MCPs that need API keys use `mcps/.env` (gitignored):

```bash
cp mcps/.env.example mcps/.env
# edit mcps/.env and fill in your keys
./install.sh
```

The installer loads `mcps/.env` and:
- Passes values as `--env KEY=VALUE` flags to `claude mcp add` (stored permanently in Claude Code's MCP config)
- Writes them into the `env` field in opencode's `config.json`
- Skips any MCP whose `required_env` keys are missing from both the file and the shell, with a clear warning

`mcps/.env.example` is committed to the repo and documents what keys are expected. Never commit `mcps/.env`.

### Adding a New MCP

1. Add an entry to `mcps/registry.json`
2. If it needs secrets, add the key names to `required_env` and document them in `mcps/.env.example`
3. Run `./install.sh` to register it (or `--dry-run` to preview)

### CLI compatibility

| | Claude Code | opencode |
|---|---|---|
| Install method | `claude mcp add --scope <scope>` | Written to `~/.config/opencode/config.json` |
| Uninstall method | `claude mcp remove` | Entry removed from config.json |

---

## Adding a New Agent

1. Copy `templates/agent-template.md` to `agents/<agent-name>.md`
2. Fill in the frontmatter: `name`, `description` (with `<example>` blocks), `tools`, `color`
3. Write the system prompt body — follow the style of existing agents
4. Run `./install.sh` to deploy to `~/.claude/agents/`
5. Restart Claude Code to activate

Read `docs/development/agent-authoring-guide.md` for detailed guidance on writing effective agents.

---

## Adding a New Skill

1. Create `skills/<skill-name>/` directory
2. Copy `templates/skill-template.md` to `skills/<skill-name>/skill.md`
3. Fill in the frontmatter and write the skill body
4. Add a "Triggered by" section listing agents or skills that invoke it
5. Run `./install.sh` to deploy to `~/.claude/skills/`
6. The skill is immediately available as `/<skill-name>` in Claude Code

Read `docs/development/skill-authoring-guide.md` for detailed guidance on writing effective skills.

---

## Repo Conventions

- Agent files go in `agents/` — one file per agent, named `<agent-name>.md`
- Skill files go in `skills/<skill-name>/skill.md` — each skill in its own subdirectory
- Skill directory names use short, descriptive kebab-case names without a namespace prefix (e.g., `bugfix`, not `devexp-bugfix`)
- Skills include a "Triggered by" section listing which agents/skills invoke them
- Never modify agent or skill files in place on `~/.claude/` — always edit the source in this repo and re-run `install.sh`
- Templates are in `templates/` — use them as starting points, not final output
- Authoring guides are in `docs/development/`

---

## Development Workflow

When working in this repo, the typical workflow is:

1. Edit a file in `agents/` or `skills/`
2. Run `./install.sh` to deploy the change
3. Test in Claude Code
4. Commit when satisfied

The install script is idempotent — safe to run multiple times.
