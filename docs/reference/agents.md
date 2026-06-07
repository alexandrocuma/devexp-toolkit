# Agents Reference

## How Agents Work

Agents live as Markdown files in `~/.claude/agents/` on the user's machine. Each file is loaded by Claude Code and made available as a sub-agent that can be launched with the `Agent` tool.

### Critical: How to Invoke Custom Agents

Custom agents are **role and instruction definitions** — they shape how Claude behaves, they are not separate processes. The `Agent` tool's `subagent_type` parameter only accepts a hardcoded set of built-in types — **custom agent names are not valid `subagent_type` values**.

The correct way to use a custom agent:
1. A task comes in that matches a custom agent's description
2. Read `~/.claude/agents/<name>.md` (or `agents/<name>.md` in this repo)
3. Adopt its role and follow its instructions directly — no spawning needed

**Never call `Agent` tool with `subagent_type` set to a custom agent name.** It will fail.

### File Format

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
```

### Frontmatter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Identifier used to reference the agent |
| `description` | Yes | When-to-use guidance shown to the orchestrating agent. Include `<example>` blocks — they dramatically improve invocation accuracy. |
| `tools` | Yes | Comma-separated list of tools the agent can access |
| `model` | No | `sonnet` (default) or `opus` |
| `color` | No | Terminal color: `cyan`, `green`, `yellow`, `red`, `purple`, `blue` |
| `memory` | No | Set to `user` to give the agent persistent memory across sessions |

---

## Agent Catalog

| File | Agent Name | Purpose | Example Trigger Phrase |
|------|-----------|---------|------------------------|
| `arch-review.md` | arch-review | Deep architectural health assessment with scored findings | "Review the architecture before we start the refactor" |
| `backend-senior-dev.md` | backend-senior-dev | Expert backend code review and architecture analysis | "Review my new auth endpoint" |
| `codebase-navigator.md` | codebase-navigator | Builds and maintains a shared codebase atlas for all agents | "Orient yourself in this codebase before we start" |
| `dep-map.md` | dep-map | Maps module and package dependencies, detects cycles | "Map the dependencies before we start moving things around" |
| `dev-agent.md` | dev-agent | Autonomous implementation: bugs, features, refactors | "Fix the bug where payments fail for users with special characters" |
| `feature-path-tracer.md` | feature-path-tracer | Traces a single execution path through code | "Trace how the POST /auth/login endpoint works end-to-end" |
| `frontend-senior-dev.md` | frontend-senior-dev | Expert frontend code review and UI architecture guidance | "Review my new React component" |
| `migration.md` | migration | Plan and execute library/framework/runtime version migrations | "Migrate this project from React 17 to React 18" |
| `performance.md` | performance | Performance bottleneck identification and optimization | "Our API is getting slow under load, find out why" |
| `pr-review.md` | pr-review | Thorough PR review across bugs, security, patterns, and tests | "Review PR #42" |
| `root-cause.md` | root-cause | Deep root cause analysis using 5-Whys and hypothesis testing | "We've patched this crash three times and it keeps coming back" |
| `security.md` | security | Full security audit: OWASP Top 10, auth, data exposure | "Run a security audit before we deploy" |
| `test-gen.md` | test-gen | Generate comprehensive test suites for untested code | "Generate tests for the payment module" |
| `test-runner.md` | test-runner | Test execution, coverage analysis, flaky test detection | "Run the tests and tell me what's failing" |
| `project-manager.md` | project-manager | Ticket creation, epic decomposition, backlog triage — detects GitHub Issues, GitLab Issues, Linear, and Jira automatically | "Create a ticket for adding user authentication" |
| `scaffold.md` | scaffold | Pattern-matched code generation for new modules, services, and components | "Scaffold a new payments service" |
| `changelog.md` | changelog | Changelog and release notes generation from git history | "Generate the changelog since the last release" |
| `docs-sync.md` | docs-sync | Syncs documentation surfaces (CLAUDE.md, README, authoring guides) with actual repo state after changes | "Sync the docs after these agent changes" |
| `ci-cd.md` | ci-cd | CI/CD pipeline debugging, creation, and optimization | "Our GitHub Actions pipeline is failing, debug it" |
| `postmortem.md` | postmortem | Structured blameless incident postmortem documents | "Write a postmortem for last night's database outage" |
| `tech-lead.md` | tech-lead | Architecture Decision Records, design review, engineering standards | "Write an ADR for switching to PostgreSQL" |
| `pr-feedback.md` | pr-feedback | Implements reviewer comments from an existing PR or MR | "Implement the reviewer comments on PR #58" |
| `dep-audit.md` | dep-audit | Dependency vulnerability (CVE) and staleness audit | "Audit our dependencies for vulnerabilities" |
| `runbook.md` | runbook | Generates operational runbooks from actual project config | "Generate a runbook for deploying this service" |
| `grooming-agent.md` | grooming-agent | Autonomous pre-code ticket grooming — fetches ticket, validates against codebase, produces execution plan | "Groom PAY-1179 before I start coding" |
| `impact-analysis.md` | impact-analysis | Maps the blast radius of any change — callers, transitive dependents, shared state | "What breaks if I change getUserById?" |
| `data-flow.md` | data-flow | Maps how data moves through the system — entry points, transformations, storage, egress, PII tracking | "Map how customer data flows before we migrate the DB" |
| `synthesis.md` | synthesis | Consolidates findings from multiple specialist agents into one prioritized action plan | "We ran security, arch-review, and performance — synthesize the findings" |
| `tech-debt.md` | tech-debt | Business-prioritized tech debt register — what debt exists, what it costs to carry and fix | "Make the case for paying down tech debt this quarter" |
| `onboarding.md` | onboarding | Generates structured onboarding guides for new contributors to a specific module | "Generate an onboarding guide for the payments module" |

### opencode-Exclusive Agents

Live in `agents/opencode/` — written in opencode frontmatter format, installed as-is (no transformation).

| File | Agent Name | Purpose |
|------|-----------|---------|
| `opencode/orchestrator.md` | orchestrator | Swarm orchestrator: spawns specialist agents in parallel via Task tool (13 workflow presets) |

---

## Adding a New Agent

1. Copy `templates/agent-template.md` to `agents/<agent-name>.md`
2. Fill in frontmatter: `name`, `description` (with `<example>` blocks), `tools`, `color`
3. Write the system prompt body — follow style of existing agents
4. Run `./install.sh` to deploy to `~/.claude/agents/`
5. Restart Claude Code to activate

Full guide: [`docs/development/agent-authoring-guide.md`](../development/agent-authoring-guide.md)  
Structural conventions: [`docs/development/agent-architecture-reference.md`](../development/agent-architecture-reference.md)
