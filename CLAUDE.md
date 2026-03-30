# DevExp Framework — CLAUDE.md

This file gives you everything you need to work effectively in this repository.

## What This Repo Is

This is the **devexp framework** — a curated collection of Claude Code agents and skills that improve the development experience. The repo is designed to be cloned and installed, distributing a consistent set of AI-powered development capabilities to any machine.

The framework has three types of components:

- **Agents** — specialized Claude sub-agents that handle domain-specific tasks (code review, codebase navigation, autonomous implementation, frontend review, execution tracing). Stored in `agents/`.
- **Skills** — reusable slash-command behaviors that Claude can invoke mid-conversation. Stored in `skills/`. Each skill is an instruction set that shapes how Claude behaves when invoked.
- **Hooks** — safety and quality guards that intercept tool calls automatically. Stored in `hooks/`. Each hook is a separate file: shell scripts for Claude Code, JS modules for opencode.

---

## How Agents Work

Agents live as Markdown files in `~/.claude/agents/` on the user's machine. Each file is loaded by Claude Code and made available as a sub-agent that can be launched with the `Agent` tool.

### How to Invoke Custom Agents (Important)

Custom agents are **role and instruction definitions** — they shape how Claude behaves, they are not separate processes. The `Agent` tool's `subagent_type` parameter only accepts a hardcoded set of built-in types (`dev-agent`, `general-purpose`, `test-runner`, etc.) — **custom agent names are not valid `subagent_type` values**.

The correct way to use a custom agent:
1. A task comes in that matches a custom agent's description
2. Read `~/.claude/agents/<name>.md` (or `agents/<name>.md` in this repo)
3. Adopt its role and follow its instructions directly — no spawning needed

**Never call `Agent` tool with `subagent_type` set to a custom agent name.** It will fail. Instead, read the agent file and execute its instructions in the current context.

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

| File | Agent Name | Purpose | Example Trigger Phrase |
|------|-----------|---------|------------------------|
| `arch-review.md` | arch-review | Deep architectural health assessment with scored findings | "Review the architecture before we start the refactor" |
| `backend-senior-dev.md` | backend-senior-dev | Expert backend code review and architecture analysis | "Review my new auth endpoint" / "I just wrote the service layer, can you take a look?" |
| `codebase-navigator.md` | codebase-navigator | Builds and maintains a shared codebase atlas for all agents | "Orient yourself in this codebase before we start" / "Where does authentication logic live?" |
| `dep-map.md` | dep-map | Maps module and package dependencies, detects cycles | "Map the dependencies before we start moving things around" / "We're getting a circular dependency error" |
| `dev-agent.md` | dev-agent | Autonomous implementation: bugs, features, refactors | "Fix the bug where payments fail for users with special characters" / "Add rate limiting to all public API endpoints" |
| `feature-path-tracer.md` | feature-path-tracer | Traces a single execution path through code | "Trace how the POST /auth/login endpoint works end-to-end" / "What happens when a payment fails in checkout?" |
| `frontend-senior-dev.md` | frontend-senior-dev | Expert frontend code review and UI architecture guidance | "Review my new React component" / "I'm fetching data in every component, is there a better way?" |
| `migration.md` | migration | Plan and execute library/framework/runtime version migrations | "Migrate this project from React 17 to React 18" / "Plan the upgrade from Node 18 to Node 22" |
| `performance.md` | performance | Performance bottleneck identification and optimization | "Our API is getting slow under load, find out why" / "The user search feels really sluggish" |
| `pr-review.md` | pr-review | Thorough PR review across bugs, security, patterns, and tests | "Review PR #42" / "Do a full review of this pull request before we merge" |
| `root-cause.md` | root-cause | Deep root cause analysis using 5-Whys and hypothesis testing | "We've patched this crash three times and it keeps coming back" / "We had an outage last night and aren't sure what caused it" |
| `security.md` | security | Full security audit: OWASP Top 10, auth, data exposure | "Run a security audit before we deploy" / "Check for authentication vulnerabilities" |
| `test-gen.md` | test-gen | Generate comprehensive test suites for untested code | "Generate tests for the payment module" / "Write a test suite for this service" |
| `test-runner.md` | test-runner | Test execution, coverage analysis, flaky test detection | "Run the tests and tell me what's failing" / "What's our test coverage like?" |
| `project-manager.md` | project-manager | Ticket creation, epic decomposition, backlog triage — detects GitHub Issues, GitLab Issues, Linear, and Jira automatically | "Create a ticket for adding user authentication" / "Break down the notifications epic into tasks" |
| `scaffold.md` | scaffold | Pattern-matched code generation for new modules, services, and components | "Scaffold a new payments service" / "Create a UserNotifications component" |
| `changelog.md` | changelog | Changelog and release notes generation from git history | "Generate the changelog since the last release" / "What changed between v1.2 and v1.3?" |
| `docs-sync.md` | docs-sync | Syncs documentation surfaces (CLAUDE.md, README, authoring guides) with actual repo state after changes to agents, skills, hooks, or MCPs | "Sync the docs after these agent changes" / "Update the documentation to reflect the new hooks" |
| `ci-cd.md` | ci-cd | CI/CD pipeline debugging, creation, and optimization | "Our GitHub Actions pipeline is failing, debug it" / "Add a test step to the CI pipeline" |
| `postmortem.md` | postmortem | Structured blameless incident postmortem documents | "Write a postmortem for last night's database outage" |
| `tech-lead.md` | tech-lead | Architecture Decision Records, design review, engineering standards | "Write an ADR for switching to PostgreSQL" / "Review this microservice design" |
| `pr-feedback.md` | pr-feedback | Implements reviewer comments from an existing PR or MR (GitHub and GitLab) | "Implement the reviewer comments on PR #58" / "Address the feedback on my open PR" |
| `dep-audit.md` | dep-audit | Dependency vulnerability (CVE) and staleness audit | "Audit our dependencies for vulnerabilities" / "Which packages are outdated or have known CVEs?" |
| `runbook.md` | runbook | Generates operational runbooks from actual project config | "Generate a runbook for deploying this service" / "Write an on-call runbook for the API" |
| `grooming-agent.md` | grooming-agent | Autonomous pre-code ticket grooming — fetches a ticket from any platform (Linear, Jira, GitHub Issues, Notion), validates every claim against the codebase, produces a Ticket Health Report, then writes and persists a verified execution plan | "Groom PAY-1179 before I start coding" / "Groom PAY-1189, WFM1-900, and FNM1-710 for the sprint" |

**opencode-exclusive agents** live in `agents/opencode/` — written directly in opencode frontmatter format, installed as-is (no transformation). They use opencode-only capabilities like the Task tool for true parallel subagent spawning.

| File | Agent Name | Purpose | Example Trigger Phrase |
|------|-----------|---------|------------------------|
| `opencode/orchestrator.md` | orchestrator | Swarm orchestrator: spawns specialist agents in parallel via Task tool (13 workflow presets) | "Do a full review of this PR — security, performance, and architecture all at once" / "Run the full audit workflow" |

---

## How Skills Work

Skills live as Markdown files at `~/.claude/skills/<name>/SKILL.md`. Each skill is invoked using a slash command: `/skill-name` in Claude Code.

When a skill is invoked, its `SKILL.md` content is injected into the conversation context, shaping Claude's behavior for that task.

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
| pr | `/pr` | Generate a PR/MR description and optionally open it via the detected platform CLI (gh or glab) |
| quality | `/quality` | Reviews code quality, style, complexity, and maintainability |
| refactor | `/refactor` | Code refactoring for improved structure and maintainability |
| regression | `/regression` | Ensures fixes don't introduce regressions |
| standup | `/standup` | Generate a daily standup update from recent git activity |
| test-gen | `/test-gen` | Generate tests for the current file or function |
| adr | `/adr` | Write an Architecture Decision Record saved to docs/adr/ |
| changelog | `/changelog` | Generate a changelog entry from git history using conventional commits |
| release | `/release` | Full release workflow: version bump, changelog, tag, and platform release (GitHub, GitLab, or manual) |
| postmortem | `/postmortem` | Generate a structured blameless postmortem document |
| ticket | `/ticket` | Create a well-structured ticket for a bug, feature, or tech-debt item — detects GitHub Issues, GitLab Issues, Linear, and Jira |
| scope | `/scope` | Break a large feature or epic into atomic tickets with dependencies |
| health | `/health` | Generate a codebase health scorecard with RAG status per dimension |
| gen-claude-md | `/gen-claude-md` | Crawl a project's docs and codebase to generate a directive CLAUDE.md with architecture map, conventions, and implementation playbooks |
| review-pr | `/review-pr` | Surgical pre-merge code review using RISEN framework — diffs `origin/<base>...origin/<branch>` to avoid stale local refs |
| groom | `/groom` | Pre-code grooming — fetches a ticket, validates its claims against the codebase, challenges wrong assumptions, produces and persists a verified execution plan |

---

## Install and Uninstall

The installer is CLI-agnostic. It detects which AI coding CLI(s) are installed and asks which to target. Supported CLIs: **Claude Code** and **opencode**.

### install.sh

```bash
./install.sh                         # interactive
./install.sh --dry-run               # preview what would be installed, no changes made
./install.sh --model sonnet          # skip model prompt, use claude-sonnet-4-6
./install.sh --model opus            # skip model prompt, use claude-opus-4-6
./install.sh --reinstall-openviking  # wipe and reinstall the OpenViking MCP from scratch
./install.sh --reinstall-jina        # wipe and reinstall the Jina embeddings server from scratch
./install.sh --mcps-only             # only register MCP servers — skip agents, skills, and hooks
./install.sh --agents-only           # only install agents — skip skills, hooks, and MCPs
./install.sh --skills-only           # only install skills — skip agents, hooks, and MCPs
```

Behavior:
- Detects `claude` and/or `opencode` in PATH and prompts which to install for
- **Claude Code**: copies agents to `~/.claude/agents/`, skills to `~/.claude/skills/`, registers MCPs via `claude mcp add`
- **opencode**: transforms agent frontmatter (model aliases, tool mapping, adds `mode: subagent`) and installs to `~/.config/opencode/agents/`; skills go to `~/.claude/skills/` (opencode reads this path natively); MCPs are written to `~/.config/opencode/config.json`
- Backs up any conflicting files before overwriting
- If neither CLI is detected, still allows manual target selection
- Skips OpenViking and Jina setup if the services are already running (healthy PID); use `--reinstall-openviking` or `--reinstall-jina` to force a clean reinstall

### start-services.sh

Use this after a machine restart or when MCP services have stopped. **Never wipes data or venvs.**

```bash
./start-services.sh            # start anything that isn't running
./start-services.sh --status   # check service health without starting
```

Behavior:
- **Jina** (Docker): health-checks via HTTP, restarts the container if needed — no model re-download
- **OpenViking** (Python): restarts the server process using the existing venv and `~/.openviking/ov.conf` — no data wipe, no index rebuild
- Safe to run at any time — skips services that are already running

> Do **not** use `./install.sh --reinstall-openviking` to restart — it wipes the venv and all indexed knowledge. Use `start-services.sh` instead.

After running, reconnect your MCP in Claude Code (`/mcp`) or opencode.

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
| Skills | `~/.claude/skills/` | `~/.config/opencode/commands/` (flat `.md`, `name:` stripped) |
| Hooks | `~/.claude/settings.json` (shell scripts, per-tool matchers) | `~/.config/opencode/plugins/devexp-plugin.js` (JS modules) |
| `CLAUDE.md` / `AGENTS.md` | `~/.claude/CLAUDE.md` | `~/.config/opencode/AGENTS.md` (or project root) |
| Agent tools | All Claude tools | `read/write/edit/bash/glob/grep/webfetch/websearch` only |
| `Agent`, `Skill`, `Task*` tools | Supported | No opencode equivalent — dropped at transform |

---

## MCP Servers

MCP servers extend Claude's capabilities with external tools (documentation lookup, databases, APIs, etc.). The devexp framework manages MCPs alongside agents and skills.

### Registry

MCP servers are declared in `mcps/registry.json`. Two transport types are supported: **stdio** (default) and **HTTP/SSE**.

**stdio MCP (default):**

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

**HTTP/SSE MCP (for locally-hosted servers):**

```json
[
  {
    "name": "my-mcp",
    "description": "Short description of what this MCP provides",
    "transport": "http",
    "url": "http://localhost:1234/mcp",
    "docker_compose": "mcps/my-mcp/docker-compose.yml",
    "scope": "user",
    "env": {},
    "required_env": ["MY_MCP_API_KEY"],
    "setup_instructions": "Human-readable setup guidance shown when required_env keys are missing"
  }
]
```

| Field | Description |
|-------|-------------|
| `name` | Unique MCP identifier |
| `description` | What this MCP provides |
| `transport` | `"http"` for streamable-HTTP transport; `"sse"` for legacy SSE-only servers; omit for stdio (default) |
| `url` | Server URL — required when `transport` is `"http"` or `"sse"` |
| `command` | Executable to run — used for stdio MCPs |
| `args` | Arguments passed to the command — used for stdio MCPs |
| `docker_compose` | Path to a Docker Compose file (relative to repo root); installer auto-starts these services |
| `scope` | `"user"` (global) or `"project"` (Claude Code only) |
| `env` | Static environment variables to pass |
| `required_env` | Env vars that must be set — installer shows a loud `[REQUIRED]` warning with setup instructions if missing |
| `setup_instructions` | Human-readable text shown when `required_env` keys are absent |

### MCPs in This Repo

| Name | Transport | Description |
|------|-----------|-------------|
| context7 | stdio | Up-to-date library documentation and code examples for any package |
| openviking | http | Context database for AI agents — tiered memory (L0/L1/L2), semantic retrieval, and document ingestion via filesystem paradigm (viking://) |

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
- Shows a loud red `[REQUIRED]` warning with the MCP's `setup_instructions` for any MCP whose `required_env` keys are missing from both the file and the shell — the MCP is skipped until the keys are provided

`mcps/.env.example` is committed to the repo and documents what keys are expected. Never commit `mcps/.env`.

### Docker-backed MCPs

MCPs with a `docker_compose` field run as local Docker services. The installer automatically starts them:

```bash
# installer runs this for each docker_compose MCP:
docker compose -f <docker_compose_path> up -d
```

### Adding a New MCP

1. Add an entry to `mcps/registry.json`
2. If it needs secrets, add the key names to `required_env`, set `setup_instructions`, and document the keys in `mcps/.env.example`
3. If it runs as a local Docker service, add a `docker_compose` field pointing to the Compose file and create the file under `mcps/<name>/docker-compose.yml`
4. Run `./install.sh` to register it (or `--dry-run` to preview)

### CLI compatibility

| | Claude Code | opencode |
|---|---|---|
| Install method | `claude mcp add --scope <scope>` | Written to `~/.config/opencode/config.json` |
| Uninstall method | `claude mcp remove` | Entry removed from config.json |

---

## Hooks

Hooks are safety and quality guards that intercept tool calls automatically — no user action required. They are implemented differently per CLI but behave identically from the user's perspective.

### How Hooks Work

**Claude Code** hooks are shell scripts registered in `~/.claude/settings.json` under `PreToolUse` or `PostToolUse` events. Claude Code calls the script with a JSON payload on stdin and reads the response:

- **Hard block** — print reason to stderr, `exit 2`. Claude stops the tool call entirely.
- **Soft block (ask)** — output `{"hookSpecificOutput": {"permissionDecision": "ask"}}` to stdout, `exit 0`. Claude pauses and asks the user to confirm.
- **Allow** — `exit 0` with no output.

**opencode** hooks are JS modules composed into a single plugin (`devexp-plugin.js`) registered in `~/.config/opencode/config.json`. Handlers receive `(input, output)` and:

- **Block** — `throw new Error("reason")`. opencode stops the tool call.
- **Allow** — return without throwing.

### File Structure

```
hooks/
  registry.json               # Source of truth — one entry per hook
  claude-code/                # One .sh file per hook
  └── secret-guard.sh
  └── secret-in-write-guard.sh
  └── dangerous-cmd-guard.sh
  └── large-file-guard.sh
  └── lint-on-save.sh
  └── format-on-save.sh
  opencode/                   # One .js module per hook + shared utils + entry point
  └── utils.js                # Shared: findRoot, which, runLinter, countLines
  └── secret-guard.js
  └── secret-in-write-guard.js
  └── dangerous-cmd-guard.js
  └── large-file-guard.js
  └── lint-on-save.js
  └── format-on-save.js
  └── devexp-plugin.js        # Composes all modules into a single plugin export
  └── package.json            # { "type": "module" } — required for ESM
```

### Registry Format

`hooks/registry.json` is the source of truth. Each entry:

```json
{
  "name": "hook-name",
  "description": "What this hook does",
  "claude_code": {
    "event":   "PreToolUse",
    "matcher": "Bash",
    "script":  "hooks/claude-code/hook-name.sh"
  },
  "opencode": {
    "event":  "tool.execute.before",
    "plugin": "hooks/opencode/devexp-plugin.js"
  },
  "enabled": true
}
```

| Field | Description |
|-------|-------------|
| `name` | Unique hook identifier — used in `devexp.config.json` `hooks.disabled` list |
| `claude_code.event` | `PreToolUse` or `PostToolUse` |
| `claude_code.matcher` | Regex matched against tool name (e.g. `"Bash"`, `"Read"`, `"Write\|Edit"`) |
| `claude_code.script` | Path to the shell script, relative to repo root |
| `opencode.event` | `tool.execute.before` or `file.edited` |
| `opencode.plugin` | Always `hooks/opencode/devexp-plugin.js` — the single entry point |
| `enabled` | Set to `false` to skip this hook for all users |

### Hooks in This Repo

| Hook | Event | Matcher | What it does |
|------|-------|---------|--------------|
| `secret-guard` | PreToolUse | `Read\|Bash` | Hard-blocks reads of `.env*`, `.pem`, `.key`, private key files |
| `secret-in-write-guard` | PreToolUse | `Write\|Edit` | Hard-blocks writing content that contains secret patterns (API keys, GitHub tokens, private key blocks, etc.) |
| `dangerous-cmd-guard` | PreToolUse | `Bash` | Hard-blocks `rm -rf /`, fork bombs, `DROP DATABASE`, `git push --force`, `git reset --hard`, `git clean`, `DROP/TRUNCATE TABLE` |
| `large-file-guard` | PreToolUse | `Write` | Asks for confirmation before overwriting a file with >500 lines |
| `lint-on-save` | PostToolUse | `Write\|Edit` | Runs the project linter on edited source files (JS/TS → biome/eslint, Python → ruff/flake8, Go → go vet, Ruby → rubocop) |
| `format-on-save` | PostToolUse | `Write\|Edit` | Runs the project formatter in-place on edited source files (JS/TS → biome/prettier, Python → ruff/black, Go → gofmt, Ruby → rubocop) |
| `test-on-save` | PostToolUse | `Write\|Edit` | Runs the associated test file after editing a source file — skips silently if no test file found (JS/TS → jest/vitest, Go → go test, Python → pytest, Ruby → rspec) |

### CLI Compatibility

| | Claude Code | opencode |
|---|---|---|
| Hook scripts | `hooks/claude-code/*.sh` (one per hook) | `hooks/opencode/*.js` (one module per hook) |
| Entry point | Each script registered separately in `settings.json` | Single `devexp-plugin.js` registered in `config.json` |
| Block mechanism | `exit 2` + stderr | `throw new Error(...)` |
| Confirm/ask | `permissionDecision: "ask"` JSON output | Not supported — hard block instead |
| Advisory output | stderr (PostToolUse) | `console.log` (file.edited) |

---

## Adding a New Hook

1. Create `hooks/claude-code/<hook-name>.sh`:
   ```bash
   #!/usr/bin/env bash
   # devexp hook: <hook-name>
   # Event: PreToolUse | Matcher: <ToolName>
   set -euo pipefail
   input=$(cat)
   # extract fields with python3 -c "..."
   # hard block: echo "reason" >&2; exit 2
   # soft block: python3 -c "print(json.dumps({'hookSpecificOutput': {'permissionDecision': 'ask', ...}}))"
   exit 0
   ```

2. Create `hooks/opencode/<hook-name>.js`:
   ```js
   import { ... } from './utils.js';
   export async function myHookName(_ctx) {
     return {
       'tool.execute.before': async (input, output) => {
         if (input.tool !== 'target-tool') return;
         // throw new Error('reason') to block
       },
     };
   }
   ```

3. Register the module in `hooks/opencode/devexp-plugin.js` — import it and add it to the `Promise.all([...])` array.

4. Add the entry to `hooks/registry.json`.

5. `chmod +x hooks/claude-code/<hook-name>.sh` and run `./install.sh`.

Read `docs/development/hook-authoring-guide.md` for detailed guidance.

---

## Adding a New Agent

1. Copy `templates/agent-template.md` to `agents/<agent-name>.md`
2. Fill in the frontmatter: `name`, `description` (with `<example>` blocks), `tools`, `color`
3. Write the system prompt body — follow the style of existing agents
4. Run `./install.sh` to deploy to `~/.claude/agents/`
5. Restart Claude Code to activate

Read `docs/development/agent-authoring-guide.md` for detailed guidance on writing effective agents.

For structural conventions (Phase 0 pattern, OpenViking/context7 protocols, Chaining format, tool declarations), see `docs/development/agent-architecture-reference.md`.

---

## Adding a New Skill

1. Create `skills/<skill-name>/` directory
2. Copy `templates/skill-template.md` to `skills/<skill-name>/SKILL.md`
3. Fill in the frontmatter and write the skill body
4. Add a "Triggered by" section listing agents or skills that invoke it
5. Run `./install.sh` to deploy to `~/.claude/skills/`
6. The skill is immediately available as `/<skill-name>` in Claude Code

Read `docs/development/skill-authoring-guide.md` for detailed guidance on writing effective skills.

---

## Team Distribution

Teams can fork this repo and customise `devexp.config.json` to control what gets installed.

| Field | Purpose |
|-------|---------|
| `model` | Default model for all agents (overridden by `--model` flag) |
| `agents.disabled` | Agent names to skip (e.g. `["scaffold", "orchestrator"]`) |
| `skills.disabled` | Skill names to skip (e.g. `["gen-claude-md"]`) |
| `hooks.disabled` | Hook names from `hooks/registry.json` to skip |
| `mcps` | Additional MCP entries merged with `mcps/registry.json` at install time |

The schema is in `devexp.config.schema.json`. Full guide: `docs/team-distribution.md`.

---

## Repo Conventions

- Agent files go in `agents/` — one file per agent, named `<agent-name>.md`
- Skill files go in `skills/<skill-name>/SKILL.md` — each skill in its own subdirectory
- Skill directory names use short, descriptive kebab-case names without a namespace prefix (e.g., `bugfix`, not `devexp-bugfix`)
- Skills include a "Triggered by" section listing which agents/skills invoke them
- Hook files go in `hooks/claude-code/<hook-name>.sh` and `hooks/opencode/<hook-name>.js` — one file per hook per CLI
- Every hook has an entry in `hooks/registry.json` — this is the source of truth
- The opencode entry point (`hooks/opencode/devexp-plugin.js`) must be updated when adding a new hook module
- Never modify deployed hook scripts directly — always edit the source in this repo and re-run `install.sh`
- Templates are in `templates/` — use them as starting points, not final output
- Authoring guides are in `docs/development/`

---

## Development Workflow

When working in this repo, the typical workflow is:

1. Edit a file in `agents/`, `skills/`, or `hooks/`
2. Run `./install.sh` to deploy the change
3. Test in Claude Code or opencode
4. Commit when satisfied

The install script is idempotent — safe to run multiple times.
