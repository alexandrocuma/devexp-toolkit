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
| **project-manager** | GitHub Issue creation, epic decomposition, and backlog triage. |
| **changelog** | Generates changelogs and release notes from git history using conventional commits. |
| **ci-cd** | CI/CD pipeline debugging, creation, and optimization across GitHub Actions, GitLab CI, and others. |
| **postmortem** | Produces structured blameless incident postmortem documents. |
| **tech-lead** | Architecture Decision Records, design review, and engineering standards documentation. |

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
| `/pr` | Generate a PR description and optionally open it via `gh`. |
| `/changelog` | Generate a changelog entry from git history. |
| `/release` | Full release workflow: version bump, changelog, tag, and GitHub release. |
| `/standup` | Generate a daily standup update from recent git activity. |
| `/ticket` | Create a well-structured GitHub Issue for a bug, feature, or tech-debt item. |
| `/scope` | Break a large feature or epic into atomic tickets with dependencies. |
| `/health` | Generate a codebase health scorecard with RAG status per dimension. |
| `/postmortem` | Generate a structured blameless postmortem document. |

### MCP Servers

MCP (Model Context Protocol) servers extend Claude with additional tool capabilities. devexp manages a registry of curated MCP servers and installs them alongside agents and skills.

| MCP | Description |
|-----|-------------|
| **context7** | Up-to-date library documentation and code examples for any package — fetched at query time, not from training data. |

MCP configuration lives in `mcps/registry.json`. API keys and secrets go in `mcps/.env` (gitignored).

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

### What gets installed where

| Component | Claude Code | opencode |
|-----------|-------------|----------|
| Agents | `~/.claude/agents/` | `~/.config/opencode/agents/` (frontmatter transformed) |
| Skills | `~/.claude/skills/` | `~/.claude/skills/` (same path — opencode reads it natively) |
| MCPs | via `claude mcp add` | `~/.config/opencode/config.json` |

Existing files are backed up automatically before any overwrite.

### Uninstall

```bash
./uninstall.sh          # interactive — prompts for confirmation
./uninstall.sh --yes    # non-interactive
```

Removes only devexp's agents, skills, and MCPs. Your own custom agents and skills are untouched.

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

See `docs/agent-authoring-guide.md` for a comprehensive guide.

---

## Adding a New Skill

1. Create the skill directory and file:
   ```bash
   mkdir -p skills/my-skill
   cp templates/skill-template.md skills/my-skill/skill.md
   ```

2. Fill in the frontmatter and write the skill body.

3. Install and test:
   ```bash
   ./install.sh
   ```

See `docs/skill-authoring-guide.md` for a comprehensive guide.

---

## Adding a New MCP

1. Add an entry to `mcps/registry.json`:
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

2. If the MCP requires an API key, add the key name to `required_env` and document it in `mcps/.env.example`.

3. Run `./install.sh` — the MCP is registered with the CLI automatically.

See `docs/mcp-guide.md` for a full guide to the registry format and secrets handling.

---

## Repo Structure

```
devexp/
├── install.sh                  # Installs agents, skills, and MCPs
├── uninstall.sh                # Removes devexp components
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
│   └── opencode/               # opencode-exclusive agents (installed as-is)
│       └── orchestrator.md
├── skills/                     # Skill subdirectories, each with skill.md
│   ├── bugfix/skill.md
│   ├── feature/skill.md
│   ├── refactor/skill.md
│   ├── docs/skill.md
│   ├── test-gen/skill.md
│   ├── regression/skill.md
│   ├── logic-review/skill.md
│   ├── quality/skill.md
│   ├── api-design/skill.md
│   ├── db-design/skill.md
│   ├── migrate/skill.md
│   ├── explain/skill.md
│   ├── adr/skill.md
│   ├── commit/skill.md
│   ├── pr/skill.md
│   ├── changelog/skill.md
│   ├── release/skill.md
│   ├── standup/skill.md
│   ├── ticket/skill.md
│   ├── scope/skill.md
│   ├── health/skill.md
│   └── postmortem/skill.md
├── mcps/                       # MCP server registry and secrets
│   ├── registry.json           # Curated MCP server list
│   └── .env.example            # Template for API keys (copy to .env)
├── templates/                  # Starting points for new agents and skills
│   ├── agent-template.md
│   └── skill-template.md
└── docs/                       # Authoring guides
    ├── agent-authoring-guide.md
    ├── skill-authoring-guide.md
    └── mcp-guide.md
```

---

## Contributing

Contributions are welcome. To add an agent or skill:

1. Follow the authoring guides in `docs/`.
2. Use the templates in `templates/` as your starting point.
3. Test thoroughly before submitting a PR.
4. Keep descriptions precise — the `description` field is what Claude reads to decide when to use a skill or agent.

The bar for inclusion: does this provide genuine, reusable value across different projects? Highly project-specific agents and skills are better kept in a project's own `.claude/` directory.
