# devexp

A curated collection of Claude Code agents, skills, and MCP servers that bring a consistent, expert-level development experience to any project.

Install once. Get autonomous bug fixes, expert code review, codebase navigation, execution tracing, security audits, and 22 specialized development skills — available inside Claude Code and opencode.

---

## What's Included

### Agents

Agents are specialized sub-agents that Claude Code or opencode can spawn to handle domain-specific tasks autonomously.

| Agent | Description |
|-------|-------------|
| **dev-agent** | Autonomous implementation: bug fixes, feature development, legacy rehabilitation, and complex multi-step refactors. Orients itself, plans, executes, and verifies — minimal back-and-forth required. |
| **backend-senior-dev** | Senior backend engineer with 15+ years experience. Structured code reviews covering correctness, security, scalability, and algorithm efficiency. Works in Python, Go, Java, TypeScript, Rust, C#, and more. |
| **frontend-senior-dev** | Senior frontend developer covering React, Vue, Angular, Svelte, TypeScript, and CSS. Reviews for correctness, performance, accessibility, and framework idioms. |
| **codebase-navigator** | Builds and maintains a persistent "codebase atlas" — stack, architecture, layer map, conventions, canonical example — so every other agent knows how things are done in your project. |
| **feature-path-tracer** | Traces a single execution path through code (happy path, failure path, or specific branch) and produces a clear linear summary from entry point to outcome. |
| **arch-review** | Deep architectural health assessment: coupling, cohesion, layering, and structural anti-patterns. Produces scored findings with remediation guidance. |
| **root-cause** | Deep root cause analysis for complex, recurring, or production bugs using 5-Whys methodology and hypothesis testing. |
| **security** | Full security audit: OWASP Top 10, authentication flaws, injection vulnerabilities, data exposure, and cryptographic issues. |
| **performance** | Performance bottleneck identification across the full stack: algorithmic complexity, N+1 queries, blocking I/O, and frontend rendering issues. |
| **pr-review** | Thorough PR review covering bugs, security implications, test coverage, and pattern consistency — before you merge. |
| **test-gen** | Generates comprehensive test suites for untested code: unit tests, integration tests, edge cases, and error paths. |
| **test-runner** | Test execution, coverage analysis, and flaky test detection across unit, integration, and full suites. |
| **dep-map** | Maps module and package dependencies, detects circular dependencies, and identifies unused packages. |
| **migration** | Plans and executes library, framework, or runtime version migrations with step-by-step guidance. |
| **scaffold** | Pattern-matched code generation for new modules, services, and components — matching existing project conventions exactly. |
| **project-manager** | Ticket creation, epic decomposition, and backlog triage — detects GitHub Issues, GitLab Issues, Linear, and Jira automatically. |
| **changelog** | Generates changelogs and release notes from git history using conventional commits. |
| **ci-cd** | CI/CD pipeline debugging, creation, and optimization across GitHub Actions, GitLab CI, and others. |
| **postmortem** | Produces structured blameless incident postmortem documents. |
| **tech-lead** | Architecture Decision Records, design review, and engineering standards documentation. |
| **docs-sync** | Syncs documentation surfaces (CLAUDE.md, README, authoring guides) with actual repo state after changes to agents, skills, hooks, or MCPs. |
| **pr-feedback** | Implements reviewer comments from an existing PR or MR (GitHub and GitLab). |
| **dep-audit** | Dependency vulnerability (CVE) and staleness audit. |
| **runbook** | Generates operational runbooks from actual project config. |
| **grooming-agent** | Autonomous pre-code ticket grooming — fetches a ticket from any platform (Linear, Jira, GitHub Issues, Notion), validates every claim against the codebase, produces a Ticket Health Report, then writes and persists a verified execution plan. |

**opencode-exclusive agents** (in `agents/opencode/`):

| Agent | Description |
|-------|-------------|
| **orchestrator** | Swarm orchestrator — spawns specialist agents in parallel via the Task tool. 13 workflow presets covering full code review, feature implementation, incident response, and more. |

### Skills

Skills are invoked as slash commands (`/skill-name`) in Claude Code or opencode. They inject structured guidance into the current conversation — shaping how Claude approaches a task without spawning a separate agent.

| Skill | Description |
|-------|-------------|
| `/bugfix` | Root cause analysis and bug fixing with built-in verification. |
| `/feature` | Spec-driven feature implementation with tests and documentation. |
| `/refactor` | Code refactoring for improved structure and maintainability. |
| `/docs` | Documentation generation: API docs, code comments, usage examples, README. |
| `/test-gen` | Generate tests for the current file or function. |
| `/regression` | Verify that fixes and changes don't introduce regressions. |
| `/logic-review` | Review code logic for bugs, edge cases, null dereferences, and race conditions. |
| `/quality` | Code quality review: style, complexity, and SOLID principle adherence. |
| `/api-design` | Design API contracts, endpoints, request/response schemas, and error handling. |
| `/db-design` | Design database schemas, migrations, indexes, and query patterns. |
| `/migrate` | Step-by-step migration guide for a library or framework upgrade. |
| `/explain` | Explain code to a specific audience: junior, new-hire, or non-technical. |
| `/adr` | Write an Architecture Decision Record saved to `docs/adr/`. |
| `/commit` | Craft a conventional commit message and create the commit. |
| `/pr` | Generate a PR/MR description and optionally open it via the detected platform CLI (gh or glab). |
| `/changelog` | Generate a changelog entry from git history. |
| `/release` | Full release workflow: version bump, changelog, tag, and platform release (GitHub, GitLab, or manual). |
| `/standup` | Generate a daily standup update from recent git activity. |
| `/ticket` | Create a well-structured ticket for a bug, feature, or tech-debt item — detects GitHub Issues, GitLab Issues, Linear, and Jira. |
| `/scope` | Break a large feature or epic into atomic tickets with dependencies. |
| `/health` | Generate a codebase health scorecard with RAG status per dimension. |
| `/gen-claude-md` | Crawl a project's docs and codebase to generate a directive CLAUDE.md with architecture map, conventions, and implementation playbooks. |
| `/postmortem` | Generate a structured blameless postmortem document. |
| `/groom` | Pre-code grooming — fetches a ticket, validates its claims against the codebase, challenges wrong assumptions, produces and persists a verified execution plan. |

### Hooks

Hooks are safety and quality guards that run automatically on every tool call — no configuration needed. They work identically in Claude Code (shell scripts) and opencode (JS plugin modules).

| Hook | Trigger | What it does |
|------|---------|--------------|
| **secret-guard** | Any `Read` or `Bash` call | Hard-blocks reads of `.env*`, `.pem`, `.key`, private key files |
| **secret-in-write-guard** | Any `Write` or `Edit` call | Hard-blocks writing content containing secret patterns (API keys, GitHub tokens, private key blocks, etc.) |
| **dangerous-cmd-guard** | Any `Bash` call | Hard-blocks `rm -rf /`, fork bombs, `DROP DATABASE`, `git push --force`, `git reset --hard`, `git clean`, `DROP/TRUNCATE TABLE` |
| **large-file-guard** | Any `Write` call | Asks for confirmation before overwriting a file with >500 lines |
| **lint-on-save** | After `Write` or `Edit` | Runs the project linter on edited source files (JS/TS, Python, Go, Ruby) |
| **format-on-save** | After `Write` or `Edit` | Runs the project formatter in-place on edited source files (JS/TS, Python, Go, Ruby) |
| **test-on-save** | After `Write` or `Edit` | Runs the associated test file after editing a source file — skips silently if no test file found (JS/TS, Go, Python, Ruby) |

Hook configuration lives in `hooks/registry.json`. Each hook is a separate file in `hooks/claude-code/` (shell scripts) and `hooks/opencode/` (JS modules).

### MCP Servers

MCP (Model Context Protocol) servers extend Claude with additional tool capabilities. devexp manages a registry of curated MCP servers and installs them alongside agents, skills, and hooks.

| MCP | Transport | Description |
|-----|-----------|-------------|
| **context7** | stdio | Up-to-date library documentation and code examples for any package — fetched at query time, not from training data. |
| **openviking** | http | Context database for AI agents — tiered memory (L0/L1/L2), semantic retrieval, and document ingestion via filesystem paradigm (viking://). Requires `OPENVIKING_VLM_API_KEY` and `OPENVIKING_VLM_MODEL`. |

MCP configuration lives in `mcps/registry.json`. API keys and secrets go in `mcps/.env` (gitignored). MCPs with a `docker_compose` field are started automatically by the installer via `docker compose up -d`.

---

## Installation

```bash
git clone https://github.com/your-username/devexp.git
cd devexp
./install.sh
```

The installer detects which AI coding CLI(s) you have installed and asks which to target. It supports **Claude Code** and **opencode**. If both are present, you can install for either or both.

```
devexp Framework Installer
────────────────────────────────────────

[devexp] Detected: Claude Code and opencode

  [1] Claude Code only
  [2] opencode only
  [3] Both

Install for which CLI? [1/2/3]:
```

After installation, restart your CLI to activate the new agents and skills.

### Preview before installing

```bash
./install.sh --dry-run
```

Prints what would be installed without making any changes.

### Reinstall process-managed services from scratch

```bash
./install.sh --reinstall-openviking   # wipe ~/.openviking/venv, kill server, regenerate ov.conf
./install.sh --reinstall-jina         # wipe ~/.openviking/jina-venv (or stop Docker container), restart Jina
./install.sh --reinstall-openviking --reinstall-jina   # both at once
```

During a normal install, setup is skipped automatically if the services are already running. Use these flags if a service is in a bad state or you want to pick up a newer version.

### What gets installed where

| Component | Claude Code | opencode |
|-----------|-------------|----------|
| Agents | `~/.claude/agents/` | `~/.config/opencode/agents/` (frontmatter transformed) |
| Skills | `~/.claude/skills/` | `~/.claude/skills/` (same path — opencode reads it natively) |
| Hooks | `~/.claude/settings.json` (shell scripts, per-tool matchers) | `~/.config/opencode/plugins/devexp-plugin.js` (JS modules) |
| MCPs | via `claude mcp add` | `~/.config/opencode/config.json` |

Existing files are backed up automatically before any overwrite.

### Restart Services (without data loss)

After a machine restart or session, MCP services may have stopped. Use `start-services.sh` to bring them back — it **never touches your data or venvs**:

```bash
./start-services.sh            # start anything that isn't running
./start-services.sh --status   # check service health without starting
```

What it does:
- **Jina** (Docker): checks health via HTTP, restarts the container if needed — no model re-download
- **OpenViking** (Python): restarts the server process using the existing venv and `~/.openviking/ov.conf` — no data wipe, no index rebuild

> **Do not use `./install.sh --reinstall-openviking`** unless you want a full wipe. That deletes the venv and all indexed knowledge.

After running `start-services.sh`, reconnect your MCP in Claude Code (via `/mcp`) or opencode.

### Uninstall

```bash
./uninstall.sh          # interactive — prompts for confirmation
./uninstall.sh --yes    # non-interactive
```

Removes only devexp's agents, skills, hooks, and MCPs. Your own custom agents and skills are untouched.

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

## Adding a New Agent

1. Copy the template:
   ```bash
   cp templates/agent-template.md agents/my-agent.md
   ```

2. Fill in the frontmatter: `name`, `description` (with `<example>` blocks), `tools`, `color`.

3. Write the system prompt body — follow the style of existing agents.

4. Install and test:
   ```bash
   ./install.sh
   ```

5. Restart your CLI to activate.

See `docs/development/agent-authoring-guide.md` for a comprehensive guide.

---

## Adding a New Skill

1. Create the skill directory and file:
   ```bash
   mkdir -p skills/my-skill
   cp templates/skill-template.md skills/my-skill/SKILL.md
   ```

2. Fill in the frontmatter and write the skill body.

3. Install and test:
   ```bash
   ./install.sh
   ```

See `docs/development/skill-authoring-guide.md` for a comprehensive guide.

---

## Adding a New Hook

1. Create `hooks/claude-code/<hook-name>.sh` — the shell script Claude Code will run:
   ```bash
   #!/usr/bin/env bash
   # devexp hook: <hook-name>
   # Event: PreToolUse | Matcher: <ToolName>
   set -euo pipefail
   input=$(cat)
   # ... guard logic ...
   exit 0
   ```

2. Create `hooks/opencode/<hook-name>.js` — the opencode JS module:
   ```js
   import { ... } from './utils.js';
   export async function myHook(_ctx) {
     return {
       'tool.execute.before': async (input, output) => { /* ... */ },
     };
   }
   ```

3. Register the module in `hooks/opencode/devexp-plugin.js`:
   ```js
   import { myHook } from './my-hook.js';
   // add to Promise.all([...]) and merge its handlers
   ```

4. Add an entry to `hooks/registry.json`:
   ```json
   {
     "name": "my-hook",
     "description": "What this hook does",
     "claude_code": { "event": "PreToolUse", "matcher": "Bash", "script": "hooks/claude-code/my-hook.sh" },
     "opencode":    { "event": "tool.execute.before", "plugin": "hooks/opencode/devexp-plugin.js" },
     "enabled": true
   }
   ```

5. Run `./install.sh` — the hook is registered automatically.

See `docs/development/hook-authoring-guide.md` for a full guide.

---

## Adding a New MCP

1. Add an entry to `mcps/registry.json`. For a stdio MCP:
   ```json
   {
     "name": "my-mcp",
     "description": "What this MCP does",
     "command": "npx",
     "args": ["-y", "my-mcp-package"],
     "scope": "user",
     "env": {},
     "required_env": []
   }
   ```
   For an HTTP/SSE MCP (locally-hosted server):
   ```json
   {
     "name": "my-mcp",
     "description": "What this MCP does",
     "transport": "http",
     "url": "http://localhost:PORT/mcp",
     "docker_compose": "mcps/my-mcp/docker-compose.yml",
     "scope": "user",
     "env": {},
     "required_env": ["MY_MCP_API_KEY"],
     "setup_instructions": "Set MY_MCP_API_KEY in mcps/.env and re-run ./install.sh"
   }
   ```

2. If the MCP requires an API key, add the key name to `required_env`, add `setup_instructions`, and document the key in `mcps/.env.example`.

3. If the MCP runs as a Docker service, add a `docker_compose` field and create `mcps/<name>/docker-compose.yml`. The installer will run `docker compose up -d` automatically.

4. Run `./install.sh` — the MCP is registered with the CLI automatically.

See `docs/development/mcp-guide.md` for a full guide to the registry format and secrets handling.

---

## Repo Structure

```
devexp/
├── install.sh                  # Installs agents, skills, and MCPs
├── uninstall.sh                # Removes devexp components
├── start-services.sh           # Restarts MCP services (Jina + OpenViking) without data loss
├── CLAUDE.md                   # Instructions for Claude when working in this repo
├── agents/                     # Agent markdown files (Claude Code format)
│   ├── dev-agent.md
│   ├── backend-senior-dev.md
│   ├── frontend-senior-dev.md
│   ├── codebase-navigator.md
│   ├── feature-path-tracer.md
│   ├── arch-review.md
│   ├── root-cause.md
│   ├── security.md
│   ├── performance.md
│   ├── pr-review.md
│   ├── test-gen.md
│   ├── test-runner.md
│   ├── dep-map.md
│   ├── migration.md
│   ├── scaffold.md
│   ├── project-manager.md
│   ├── changelog.md
│   ├── ci-cd.md
│   ├── postmortem.md
│   ├── tech-lead.md
│   ├── docs-sync.md
│   ├── pr-feedback.md
│   ├── dep-audit.md
│   ├── runbook.md
│   └── opencode/               # opencode-exclusive agents (installed as-is)
│       └── orchestrator.md
├── skills/                     # Skill subdirectories, each with SKILL.md
│   ├── bugfix/SKILL.md
│   ├── feature/SKILL.md
│   ├── refactor/SKILL.md
│   ├── docs/SKILL.md
│   ├── test-gen/SKILL.md
│   ├── regression/SKILL.md
│   ├── logic-review/SKILL.md
│   ├── quality/SKILL.md
│   ├── api-design/SKILL.md
│   ├── db-design/SKILL.md
│   ├── migrate/SKILL.md
│   ├── explain/SKILL.md
│   ├── adr/SKILL.md
│   ├── commit/SKILL.md
│   ├── pr/SKILL.md
│   ├── changelog/SKILL.md
│   ├── release/SKILL.md
│   ├── standup/SKILL.md
│   ├── ticket/SKILL.md
│   ├── scope/SKILL.md
│   ├── health/SKILL.md
│   ├── gen-claude-md/SKILL.md
│   └── postmortem/SKILL.md
├── hooks/                      # Safety and quality hooks (one file per hook)
│   ├── registry.json           # Hook registry — source of truth for all hooks
│   ├── claude-code/            # Shell scripts registered in ~/.claude/settings.json
│   │   ├── secret-guard.sh             # Blocks reads of .env and key files (matcher: Read|Bash)
│   │   ├── secret-in-write-guard.sh    # Blocks writing secret patterns in content (matcher: Write|Edit)
│   │   ├── dangerous-cmd-guard.sh      # Hard-blocks destructive shell commands (matcher: Bash)
│   │   ├── large-file-guard.sh         # Confirms large file overwrites (matcher: Write)
│   │   ├── lint-on-save.sh             # Runs project linter after edits (PostToolUse)
│   │   └── format-on-save.sh           # Runs project formatter after edits (PostToolUse)
│   └── opencode/               # JS modules composed into a single plugin
│       ├── devexp-plugin.js        # Entry point — imports and composes all hook modules
│       ├── utils.js                # Shared helpers: findRoot, which, runLinter, countLines
│       ├── secret-guard.js
│       ├── secret-in-write-guard.js
│       ├── dangerous-cmd-guard.js
│       ├── large-file-guard.js
│       ├── lint-on-save.js
│       └── format-on-save.js
├── mcps/                       # MCP server registry and secrets
│   ├── registry.json           # Curated MCP server list
│   ├── .env.example            # Template for API keys (copy to .env)
│   └── openviking/             # OpenViking MCP (HTTP, pip-based)
│       ├── server.py           # Self-contained MCP server (port 2033)
│       └── ov.conf.example     # Config template (copied to ~/.openviking/ov.conf)
├── templates/                  # Starting points for new agents and skills
│   ├── agent-template.md
│   └── skill-template.md
└── docs/                       # Project documentation
    ├── README.md               # Navigation index
    └── development/            # Authoring guides for contributors
        ├── agent-authoring-guide.md
        ├── skill-authoring-guide.md
        └── mcp-guide.md
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

→ Full guide: `docs/team-distribution.md`

---

## Contributing

Contributions are welcome. To add an agent or skill:

1. Follow the authoring guides in `docs/`.
2. Use the templates in `templates/` as your starting point.
3. Test thoroughly before submitting a PR.
4. Keep descriptions precise — the `description` field is what Claude reads to decide when to use a skill or agent.

The bar for inclusion: does this provide genuine, reusable value across different projects? Highly project-specific agents and skills are better kept in a project's own `.claude/` directory.
