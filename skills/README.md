# Skills

Each subdirectory here contains one skill. Skills are invoked in Claude Code or opencode via slash commands: `/skill-name`.

Install them by running `../install.sh` from the repo root. Installed skills land in `~/.claude/skills/<name>/SKILL.md`.

---

## Skill Index

### Implementation

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `bugfix/` | `/bugfix` | Root cause analysis and bug fixing with built-in verification |
| `feature/` | `/feature` | Spec-driven feature implementation with tests and documentation |
| `refactor/` | `/refactor` | Code refactoring for improved structure and maintainability |
| `api-design/` | `/api-design` | Design API contracts, endpoints, request/response schemas, and error handling |
| `db-design/` | `/db-design` | Design database schemas, migrations, indexes, and query patterns |
| `migrate/` | `/migrate` | Step-by-step migration guide for a library or framework upgrade |

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
| `docs/` | `/docs` | Documentation generation: API docs, code comments, usage examples, README |
| `explain/` | `/explain` | Explain code to a specific audience: junior, new-hire, or non-technical |
| `adr/` | `/adr` | Write an Architecture Decision Record saved to `docs/adr/` |
| `postmortem/` | `/postmortem` | Generate a structured blameless postmortem document |
| `gen-claude-md/` | `/gen-claude-md` | Crawl a project and generate a directive CLAUDE.md with architecture map and conventions |
