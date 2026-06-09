# Skills

Each subdirectory here contains one skill. Skills are invoked in Claude Code or opencode via slash commands: `/skill-name`.

Install them by running `../install.sh` from the repo root. Installed skills land in `~/.claude/skills/<name>/SKILL.md`.

---

## Start Here — Lifecycle Orchestrators

These four commands cover the full development cycle. Use them as your entry points; the specialists below are available when you need precision.

| Directory | Slash Command | When to use |
|-----------|---------------|------------|
| `devxp/` | `/devxp` | First time on a repo — orient, set up CLAUDE.md and docs/ |
| `refine/` | `/refine` | You have an idea or feature request to turn into a groomed ticket |
| `deliver/` | `/deliver <ticket>` | You have a groomed ticket and want to build and ship it |
| `improve/` | `/improve` | Sprint end or maintenance window — health, cleanup, retrospective |

```
/devxp  →  /refine  →  /deliver  →  /improve
  ↑                                      |
  └──────────── next sprint ─────────────┘
```

---

## Skill Index

### Entry Point

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `devxp/` | `/devxp` | Orients on any repo — ensures CLAUDE.md and the docs/ index exist (generating or refreshing as needed) and optionally enriches with graphify. Start here. |

---

### Discovery & Requirements

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `requirements/` | `/requirements` | Structure a vague idea or stakeholder input into user stories, acceptance criteria, and a ticket-ready spec |

---

### Implementation

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `bugfix/` | `/bugfix` | Root cause analysis and bug fixing with built-in verification |
| `feature/` | `/feature` | Spec-driven feature implementation with tests and documentation |
| `refactor/` | `/refactor` | Code refactoring for improved structure and maintainability |
| `api-design/` | `/api-design` | Design API contracts, endpoints, request/response schemas, and error handling |
| `db-design/` | `/db-design` | Design database schemas, migrations, indexes, and query patterns |
| `migrate/` | `/migrate` | Step-by-step migration guide for a library or framework upgrade |
| `instrument/` | `/instrument` | Add structured observability — logs, metrics, tracing — matching the project's existing conventions |
| `feature-flag/` | `/feature-flag` | Full feature flag lifecycle: create, gate code, track rollout status, retire flags cleanly |
| `env-audit/` | `/env-audit` | Audit env vars — detect undocumented reads, leaked secrets, and config drift |

---

### Review and Analysis

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `logic-review/` | `/logic-review` | Review code logic for bugs, edge cases, null dereferences, and race conditions |
| `quality/` | `/quality` | Code quality review: style, complexity, and SOLID principle adherence |
| `regression/` | `/regression` | Verify that fixes and changes don't introduce regressions |
| `convention-audit/` | `/convention-audit` | Audit for pattern divergence — finds all the ways the same problem is solved and which pattern won |
| `dead-code/` | `/dead-code` | Find unused exports, unreachable branches, zombie feature flags, and orphaned files |
| `estimation/` | `/estimation` | Evidence-based story point estimation — maps files, risk factors, and comparable past work |
| `health/` | `/health` | Generate a codebase health scorecard with RAG status per dimension |

---

### Testing

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `test-gen/` | `/test-gen` | Generate tests for the current file or function |
| `load-test/` | `/load-test` | Generate load test scenarios for critical endpoints using the project's existing tool |

---

### Git and Planning

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `commit/` | `/commit` | Craft a conventional commit message and create the commit |
| `pr/` | `/pr` | Generate a PR/MR description and optionally open it via gh or glab |
| `review-pr/` | `/review-pr` | Surgical pre-merge code review using the RISEN framework |
| `changelog/` | `/changelog` | Generate a changelog entry from git history |
| `release/` | `/release` | Full release workflow: version bump, changelog, tag, and platform release |
| `standup/` | `/standup` | Generate a daily standup update from recent git activity |
| `git-archaeology/` | `/git-archaeology` | Reconstruct intent, ownership, and decision history — answers "why does this code exist?" |
| `stale-work/` | `/stale-work` | Find orphaned branches, stale PRs, half-finished features, and zombie flags |

---

### Tickets and Planning

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `ticket/` | `/ticket` | Create a well-structured ticket for a bug, feature, or tech-debt item |
| `scope/` | `/scope` | Break a large feature or epic into atomic tickets with dependencies |
| `groom/` | `/groom` | Pre-code grooming — validates ticket claims against the codebase, produces a verified execution plan |
| `rfc/` | `/rfc` | Draft a Request for Comments document before any code is written |
| `retrospective/` | `/retrospective` | Facilitate a blameless sprint retrospective with Start/Stop/Continue findings |
| `estimation/` | `/estimation` | Evidence-based story point estimation |

---

### Documentation

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `gen-docs/` | `/gen-docs` | Write new project documentation from scratch and scaffold the standard docs/ folder tree |
| `update-docs/` | `/update-docs` | Detect documentation that's drifted from the code and refresh it in place |
| `explain/` | `/explain` | Explain code to a specific audience: junior, new-hire, or non-technical |
| `adr/` | `/adr` | Write an Architecture Decision Record saved to `docs/adr/` |
| `postmortem/` | `/postmortem` | Generate a structured blameless postmortem document |
| `gen-indexer/` | `/gen-indexer` | Crawl a project and generate a directive CLAUDE.md with architecture map and conventions, from scratch |
| `update-indexer/` | `/update-indexer` | Refresh an existing CLAUDE.md whose sections have drifted from the current codebase |

---

### Toolkit Meta

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `refine/` | `/refine` | Lifecycle orchestrator — idea → requirements → estimation → groomed ticket |
| `deliver/` | `/deliver` | Lifecycle orchestrator — groomed ticket → implement → test → review → release |
| `improve/` | `/improve` | Lifecycle orchestrator — health check → cleanup → debt triage → retrospective |
| `coverage-map/` | `/coverage-map` | Generate a live SDLC phase-coverage report for the devexp toolkit itself — shows which phases are strong, thin, or missing |
| `swarm-status/` | `/swarm-status` | Given the current project state, recommend which devexp specialists to activate today |
