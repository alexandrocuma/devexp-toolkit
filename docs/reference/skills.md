# Skills Reference

## Start Here — Lifecycle Orchestrators

These four commands are the entry points for the entire development cycle. New users should start here; experienced users can invoke specialists directly.

| Command | When to use |
|---------|-------------|
| `/devxp` | First time on a repo — orient, set up CLAUDE.md and docs/ |
| `/refine` | You have an idea or feature request to turn into a groomed ticket |
| `/deliver <ticket>` | You have a groomed ticket and want to build and ship it |
| `/improve` | Sprint end or maintenance window — health, cleanup, retrospective |

```
/devxp  →  /refine  →  /deliver  →  /improve
  ↑                                      |
  └──────────── next sprint ─────────────┘
```

Everything else in this file is a **specialist** — invoked by the orchestrators, or directly when you need precision without the full sequence.

---

## How Skills Work

Skills live as Markdown files at `~/.claude/skills/<name>/SKILL.md`. Each skill is invoked using a slash command: `/<name>` in Claude Code.

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

---

## Skill Catalog

| Skill | Slash Command | Description |
|-------|---------------|-------------|
| bugfix | `/bugfix` | Root cause analysis and bug fixing with built-in verification |
| commit | `/commit` | Craft a conventional commit message and create the commit |
| explain | `/explain` | Explain code to a specific audience (junior, new-hire, non-technical) |
| devxp | `/devxp` | Entry point for the toolkit — orients on any repo, ensures CLAUDE.md and docs/ exist (generating or refreshing as needed), optionally enriches with graphify |
| gen-docs | `/gen-docs` | Write new project documentation from scratch and scaffold the standard docs/ folder tree |
| update-docs | `/update-docs` | Detect documentation that's drifted from the code and refresh it in place |
| api-design | `/api-design` | Designs API contracts, endpoints, schemas, and error handling |
| db-design | `/db-design` | Designs database schemas, migrations, and indexes |
| feature | `/feature` | Turn an idea into a feature — graphify discovery, an actionable plan, mandatory context7 verification for any external library, then implementation |
| logic-review | `/logic-review` | Reviews code logic for bugs, edge cases, and dysfunction |
| migrate | `/migrate` | Step-by-step migration guide for a library or framework upgrade |
| pr | `/pr` | Generate a PR/MR description and optionally open it via gh or glab |
| quality | `/quality` | Reviews code quality, style, complexity, and maintainability |
| refactor | `/refactor` | Code refactoring for improved structure and maintainability |
| regression | `/regression` | Ensures fixes don't introduce regressions |
| standup | `/standup` | Generate a daily standup update from recent git activity |
| test-gen | `/test-gen` | Generate tests for the current file or function |
| adr | `/adr` | Write an Architecture Decision Record saved to docs/architecture/adr/ |
| changelog | `/changelog` | Generate a changelog entry from git history using conventional commits |
| release | `/release` | Full release workflow: version bump, changelog, tag, and platform release |
| postmortem | `/postmortem` | Generate a structured blameless postmortem document |
| ticket | `/ticket` | Create a well-structured ticket — detects GitHub Issues, GitLab Issues, Linear, and Jira |
| scope | `/scope` | Break a large feature or epic into atomic tickets with dependencies |
| health | `/health` | Generate a codebase health scorecard with RAG status per dimension |
| gen-indexer | `/gen-indexer` | Crawl a project's docs and codebase to generate a directive CLAUDE.md from scratch |
| update-indexer | `/update-indexer` | Refresh an existing CLAUDE.md whose sections have drifted from the current codebase |
| review-pr | `/review-pr` | Surgical pre-merge code review using RISEN framework |
| groom | `/groom` | Pre-code grooming — fetches a ticket, validates against codebase, produces execution plan |
| rfc | `/rfc` | Draft a Request for Comments document for a proposed change |
| convention-audit | `/convention-audit` | Audit codebase for pattern divergence — finds all ways the same problem is solved |
| dead-code | `/dead-code` | Find unused exports, unreachable branches, zombie flags, orphaned files |
| estimation | `/estimation` | Evidence-based story point estimation from files to change, test coverage, risk |
| retrospective | `/retrospective` | Facilitate a blameless sprint retrospective — Start/Stop/Continue findings |
| git-archaeology | `/git-archaeology` | Reconstruct intent and decision history from messy git history |
| stale-work | `/stale-work` | Find orphaned branches, stale PRs, half-finished features, zombie flags |
| graphify | `/graphify` | Turn a codebase (or any input) into a persistent knowledge graph — query/path/explain tools, interactive HTML, GraphRAG-ready JSON. Pairs with the optional `graphify-read-guard`/`graphify-session-sentinel` [hooks](hooks.md) |
| requirements | `/requirements` | Structure a vague idea or stakeholder input into user stories, acceptance criteria, and a ticket-ready spec |
| instrument | `/instrument` | Add structured observability — logs, metrics, tracing — matching the project's existing conventions; audit-only mode available |
| feature-flag | `/feature-flag` | Full feature flag lifecycle: create, gate code, track rollout status, retire flags cleanly; detects zombie flags |
| env-audit | `/env-audit` | Audit env vars — detect undocumented reads, leaked secrets, config drift, and generate a complete `.env.example` |
| load-test | `/load-test` | Generate load test scenarios for critical endpoints using the project's existing tool; audit-only mode available |
| coverage-map | `/coverage-map` | Generate a live SDLC phase-coverage report for the devexp toolkit itself — shows strong/thin/missing phases |
| swarm-status | `/swarm-status` | Given the current project state, recommend which devexp specialists to activate today |
| refine | `/refine` | Lifecycle orchestrator — idea → requirements → estimation → groomed ticket |
| deliver | `/deliver` | Lifecycle orchestrator — groomed ticket → implement → test → review → release |
| improve | `/improve` | Lifecycle orchestrator — health check → cleanup → debt triage → retrospective |

---

## Choosing the Right Review Tool

Nine review surfaces exist in the toolkit. Here's how to pick the right one:

| When you want to… | Use |
|---|---|
| Review a PR before merging and post inline comments | `/review-pr` |
| Deep correctness audit — bugs, edge cases, logic errors | `/logic-review` |
| Style, complexity, and maintainability check | `/quality` |
| Full backend service review with architectural guidance | `backend-senior-dev` agent |
| Full frontend component or UI architecture review | `frontend-senior-dev` agent |
| Security audit — OWASP, auth, data exposure, secrets | `security` agent |
| Circular deps, unused imports, dependency graph | `dep-map` agent |
| CVE scan and dependency staleness | `dep-audit` agent |
| Prioritized tech debt register with ROI | `tech-debt` agent |

**TL;DR for the common case:** Merging? → `/review-pr`. Investigating a bug? → `/logic-review`. Auditing a service? → `backend-senior-dev` or `frontend-senior-dev`.

---

## Adding a New Skill

1. Create `skills/<skill-name>/` directory
2. Copy `templates/skill-template.md` to `skills/<skill-name>/SKILL.md`
3. Fill in frontmatter and write the skill body
4. Add a "Triggered by" section listing agents or skills that invoke it
5. Run `./install.sh` to deploy to `~/.claude/skills/`
6. The skill is immediately available as `/<skill-name>` in Claude Code

Full guide: [`docs/development/skill-authoring-guide.md`](../development/skill-authoring-guide.md)
