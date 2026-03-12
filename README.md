# devexp

A collection of Claude Code agents and skills that bring a consistent, high-quality development experience to any project.

Install once. Get expert code review, autonomous implementation, codebase navigation, execution tracing, and 15 specialized development skills — all available inside Claude Code.

---

## What's Included

### Agents

Agents are specialized sub-agents that Claude Code can spawn to handle domain-specific tasks.

| Agent | Description |
|-------|-------------|
| **dev-agent** | Autonomous development: bug fixes, feature implementation, legacy rehabilitation, and complex multi-step refactors. Orients itself, plans, executes, and verifies — minimal back-and-forth required. |
| **backend-senior-dev** | Senior backend engineer with 15+ years experience. Performs structured code reviews covering correctness, security, scalability, and algorithm efficiency. Works in Python, Go, Java, TypeScript, Rust, C#, and more. |
| **frontend-senior-dev** | Senior frontend developer covering React, Vue, Angular, Svelte, TypeScript, and CSS. Reviews for correctness, performance, accessibility, and framework idioms. Pragmatic — picks battles wisely. |
| **codebase-navigator** | Builds and maintains a persistent "codebase atlas" — stack, architecture, layer map, conventions, canonical example — so every other agent knows how things are done in your project. |
| **feature-path-tracer** | Traces a single execution path through code (happy path, failure path, or specific branch) and produces a clear linear summary from entry point to outcome. |

### Skills

Skills are invokable via slash commands in Claude Code (`/skill-name`). They shape how Claude approaches a specific task.

| Skill | Description |
|-------|-------------|
| `/bugfix` | Root cause analysis and bug fixing with built-in verification. Merges investigation, fix implementation, and post-fix verification into one workflow. |
| `/test` | Test execution, coverage analysis, flaky test detection across unit, integration, and full suite. |
| `/docs` | Documentation generation covering API docs, code comments, usage examples, and README files. |
| `/api-design` | Designs API contracts, endpoints, request/response schemas, and error handling. |
| `/arch-review` | Reviews architecture patterns, design decisions, coupling/cohesion, and structural issues. |
| `/db-design` | Designs database schemas, migrations, indexes, and query optimization strategies. |
| `/dep-map` | Maps module, package, and file dependencies across the codebase. Finds circular dependencies and unused packages. |
| `/feature` | Spec-driven feature implementation with tests and documentation. |
| `/logic-review` | Reviews code logic for bugs, edge cases, null dereferences, race conditions, and resource leaks. |
| `/perf` | Performance profiling and bottleneck identification covering CPU, memory, I/O, and database queries. |
| `/quality` | Reviews code quality, style, complexity metrics, and SOLID principle adherence. |
| `/refactor` | Code refactoring for improved structure and maintainability using safe, incremental techniques. |
| `/regression` | Ensures bug fixes and changes don't introduce regressions across smoke, targeted, and integration test levels. |
| `/root-cause` | Deep root cause analysis for complex, recurring, or production bugs using 5 Whys methodology. |
| `/security` | Security audit for injection vulnerabilities, authentication flaws, data exposure, and cryptographic issues. |

---

## Quick Start

```bash
git clone https://github.com/your-username/devexp.git
cd devexp
./install.sh
```

Then restart Claude Code. Your agents and skills are now active.

### Verify the install

After restarting Claude Code, you can verify agents are available by asking:

```
Use the dev-agent to fix the bug in src/...
```

Or invoke a skill directly:

```
/bugfix

There's a null pointer exception in the order service when the shipping
address is missing a country code.
```

### Uninstall

```bash
./uninstall.sh
```

This removes only devexp's agents and skills. Your own custom agents are untouched. Existing files are backed up automatically before any overwrite.

---

## Usage Examples

### Autonomous bug fix

```
Use the dev-agent to fix the authentication bug — users with special characters
in their email address can't log in.
```

The dev-agent will trace the code path, identify the root cause, implement a fix matching the project's existing patterns, add a regression test, and report what it changed.

### Code review

```
Use the backend-senior-dev agent to review my new payment processing service.
```

You'll get a structured review: summary, good patterns identified, critical issues, significant improvements, and a verdict.

### Understand a new codebase

```
Use the codebase-navigator to map this project before we start working.
```

The navigator builds a persistent atlas (saved across sessions) covering the stack, architecture, layer naming, conventions, and canonical example. Other agents read this atlas automatically.

### Trace a code path

```
Use the feature-path-tracer to trace what happens when a user submits the
checkout form — happy path only.
```

### Use a skill directly

```
/security

Review the authentication module for vulnerabilities.
```

---

## Creating New Agents

1. Copy the template:
   ```bash
   cp templates/agent-template.md agents/my-agent.md
   ```

2. Edit the frontmatter and body following the template comments.

3. Install and test:
   ```bash
   ./install.sh
   ```

See `docs/agent-authoring-guide.md` for a comprehensive guide.

---

## Creating New Skills

1. Create the skill directory and file:
   ```bash
   mkdir -p skills/my-skill
   cp templates/skill-template.md skills/my-skill/skill.md
   ```

2. Edit the frontmatter and body.

3. Install and test:
   ```bash
   ./install.sh
   ```

See `docs/skill-authoring-guide.md` for a comprehensive guide.

---

## Repo Structure

```
devexp/
├── install.sh              # Copies agents and skills to ~/.claude/
├── uninstall.sh            # Removes devexp agents and skills from ~/.claude/
├── CLAUDE.md               # Instructions for Claude when working in this repo
├── agents/                 # Agent markdown files
│   ├── backend-senior-dev.md
│   ├── codebase-navigator.md
│   ├── dev-agent.md
│   ├── feature-path-tracer.md
│   └── frontend-senior-dev.md
├── skills/                 # Skill subdirectories, each with skill.md
│   ├── bugfix/skill.md
│   ├── test/skill.md
│   ├── docs/skill.md
│   ├── api-design/skill.md
│   ├── arch-review/skill.md
│   ├── db-design/skill.md
│   ├── dep-map/skill.md
│   ├── feature/skill.md
│   ├── logic-review/skill.md
│   ├── perf/skill.md
│   ├── quality/skill.md
│   ├── refactor/skill.md
│   ├── regression/skill.md
│   ├── root-cause/skill.md
│   └── security/skill.md
├── templates/              # Starting points for new agents and skills
│   ├── agent-template.md
│   └── skill-template.md
└── docs/                   # Authoring guides
    ├── agent-authoring-guide.md
    └── skill-authoring-guide.md
```

---

## Contributing

Contributions are welcome. To add an agent or skill:

1. Follow the authoring guides in `docs/`
2. Use the templates in `templates/` as your starting point
3. Test thoroughly before submitting a PR
4. Keep descriptions precise — the description field is what Claude reads to decide when to use a skill or agent

The bar for inclusion is: does this provide genuine, reusable value across different projects? Highly project-specific agents/skills are better kept in a project's own `.claude/` directory.
