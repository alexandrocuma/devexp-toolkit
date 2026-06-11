# DevExp SDLC Coverage Map

Generated: 2026-06-09 (updated: slash commands reduced to 5 orchestrators)
Agents: 34 · Skills: 5 (user-facing orchestrators) · Total components: 39  
All specialist capabilities are now inline within orchestrators or invoked as agents — no standalone slash commands.

---

## Phase Coverage Summary

| Phase | Rating | Components | Notes |
|-------|--------|-----------|-------|
| 1. Discovery & Ideation | 🟢 Strong | 5 | graphify + codebase-navigator form a powerful orientation layer |
| 2. Requirements | 🟢 Strong | 7 | Full pipeline from idea → ticket → groomed plan |
| 3. Architecture & Design | 🟢 Strong | 9 | Most comprehensive phase — design, ADR, RFC, blast radius, data flow |
| 4. Implementation | 🟢 Strong | 9 | Autonomous, spec-driven, progressive delivery all covered; IaC handled inline in /deliver |
| 5. Testing | 🟢 Strong | 5 | Unit/integration + load testing + E2E (via /deliver Phase 4 when suite detected) |
| 6. Code Review | 🟢 Strong | 13 | ⚠️ Over-concentrated — routing guide needed (see below) |
| 7. CI/CD & Release | 🟢 Strong | 6 | Full commit → PR → pipeline → release workflow |
| 8. Deployment & Infrastructure | 🟢 Strong | 3 | IaC inline in /deliver Phase 2; Infrastructure Health dimension in /improve scorecard |
| 9. Observability | 🟢 Strong | 2 | SLO candidates surfaced by /deliver; Observability Maturity dimension in /improve scorecard |
| 10. Incident Management | 🟢 Strong | 4 | root-cause → postmortem → runbook chain is explicit |
| 11. Continuous Improvement | 🟢 Strong | 7 | Health trends + retro + debt + stale work — full feedback loop |
| 12. Documentation | 🟢 Strong | 8 | Create/refresh for both docs/ and CLAUDE.md; explanation + archaeology |

**Overall: 12 Strong · 0 Solid · 0 Thin · 0 Missing**

---

## Full Component Map

### Phase 1 — Discovery & Ideation
*Exploring what to build; orienting in unfamiliar code*

| Component | Type | Description |
|-----------|------|-------------|
| `graphify` | skill | Build a persistent knowledge graph from any codebase — queryable across sessions |
| `devxp` | skill | Entry point — orients on any repo, ensures CLAUDE.md and docs/ exist |
| `codebase-navigator` | agent | Maps codebase structure, conventions, entry points; maintains persistent atlas |
| `feature-path-tracer` | agent | Traces single execution paths end-to-end before implementing |
| `git-archaeology` | inline in `devxp` | Reconstructs intent and decision history from git history |

---

### Phase 2 — Requirements
*Writing user stories, acceptance criteria, scoping, estimation*

| Component | Type | Description |
|-----------|------|-------------|
| `requirements` | inline in `refine` | Structures vague ideas into user stories and acceptance criteria |
| `groom` | inline in `deliver` + `refine` | Validates ticket claims against codebase; produces verified execution plan |
| `grooming-agent` | agent | Automated grooming pipeline — fetches, validates, persists plan |
| `scope` | agent (`project-manager`) | Decomposes epics into atomic tickets with dependency graph |
| `ticket` | inline in `refine` | Creates structured tickets across GitHub/GitLab/Linear/Jira |
| `estimation` | inline in `refine` | Evidence-based complexity estimation from codebase |
| `project-manager` | agent | Creates and manages tickets; triages backlog |

---

### Phase 3 — Architecture & Design
*System design, API contracts, data models, decisions*

| Component | Type | Description |
|-----------|------|-------------|
| `rfc` | agent (`tech-lead`) | Draft RFC before any code is written |
| `adr` | agent (`tech-lead`) | Document decisions after they're made |
| `api-design` | inline in `tech-lead` Phase 2e | API contracts, endpoints, schemas, error handling |
| `db-design` | inline in `tech-lead` Phase 2f | Database schemas, migrations, indexes, query patterns |
| `convention-audit` | inline in `improve` Phase 3 | Finds pattern divergence; recommends canonical patterns |
| `arch-review` | agent | Deep architectural health assessment |
| `tech-lead` | agent | Design reviews, ADRs, engineering standards, trade-off analysis, API + schema design |
| `impact-analysis` | agent | Blast radius mapping before implementation |
| `data-flow` | agent | End-to-end data flow mapping; PII exposure; data loss risks |

---

### Phase 4 — Implementation
*Writing code, refactoring, migrations, scaffolding, progressive delivery*

| Component | Type | Description |
|-----------|------|-------------|
| `feature` | agent (`dev-agent`) | Spec-driven full feature lifecycle |
| `refactor` | agent (`dev-agent`) | Structured code refactoring |
| `migrate` | agent (`migration`) | Library/framework version migrations |
| `bugfix` | agent (`dev-agent`) | Root cause analysis + self-verifying fix |
| `feature-flag` | inline in `dev-agent` | Full flag lifecycle: create → gate → rollout → retire |
| `dev-agent` | agent | Autonomous end-to-end implementation |
| `scaffold` | agent | Generates code matching existing conventions |
| `migration` | agent | Library/framework migration agent |
| `pr-feedback` | agent | Implements review comments autonomously |

---

### Phase 5 — Testing
*Unit, integration, load, and E2E test generation and execution*

| Component | Type | Description |
|-----------|------|-------------|
| `test-gen` | agent | Test generation agent — invoked by `deliver` Phase 4 |
| `test-runner` | agent | Runs tests, measures coverage, detects flaky tests |
| `regression` | inline in `deliver` Phase 4 | Verifies fixes don't introduce regressions |
| `load-test` | inline in `deliver` Phase 4 | Generates load test scenarios for critical endpoints |

**Remaining gap:** E2E scenario generation (Playwright/Cypress/Selenium-style). `test-gen` covers unit/integration; `load-test` covers performance. Browser-driven scenario testing is not yet generated automatically.

---

### Phase 6 — Code Review
*Pre-merge review, security audit, quality checks*

| Component | Type | Description |
|-----------|------|-------------|
| `logic-review` | inline in `deliver` Phase 5 | Correctness-focused: bugs, edge cases, null dereferences |
| `quality` | inline in `deliver` Phase 5 | Style, complexity, maintainability |
| `convention-audit` | inline in `improve` Phase 3 | Pattern audit — finds divergence from canonical patterns |
| `dead-code` | inline in `improve` Phase 3 | Unused exports, unreachable branches, zombie flags |
| `pr-review` | agent | Thorough PR review: bugs, security, patterns, tests — invoked by `deliver` Phase 5 |
| `backend-senior-dev` | agent | Expert backend review with architecture guidance |
| `frontend-senior-dev` | agent | Expert frontend review and UI architecture |
| `security` | agent | OWASP Top 10, auth flaws, data exposure |
| `performance` | agent | Bottleneck identification, complexity analysis |
| `dep-audit` | agent | CVE scan and dependency staleness |
| `dep-map` | agent | Circular dependencies, unused imports |
| `impact-analysis` | agent | Blast radius of proposed changes |

---

### Phase 7 — CI/CD & Release
*Pipeline config, changelog, versioning, tagging*

| Component | Type | Description |
|-----------|------|-------------|
| `commit` | inline in `deliver` Phase 6 | Conventional commit generation |
| `pr` | inline in `deliver` Phase 6 | PR/MR description + optional open via CLI |
| `changelog` | inline in `deliver` Phase 6 | Changelog entry from conventional commits |
| `release` | inline in `deliver` Phase 6 | Full release: version bump → changelog → commit → tag → platform |
| `changelog` | agent | Changelog generation agent |
| `ci-cd` | agent | Debug, create, optimize CI/CD pipelines |

---

### Phase 8 — Deployment & Infrastructure
*Environment management, secrets, IaC, configuration*

| Component | Type | Description |
|-----------|------|-------------|
| `env-audit` | inline in `improve` Phase 2 | Audits env vars — undocumented reads, leaked secrets, config drift |
| `feature-flag` | inline in `dev-agent` | Progressive delivery + safe rollout control |
| `ci-cd` | agent | Pipeline config and deployment workflow |

**Remaining gap:** Environment promotion workflows (dev → staging → prod) are not automated.

---

### Phase 9 — Observability
*Logging instrumentation, metrics, tracing, monitoring*

| Component | Type | Description |
|-----------|------|-------------|
| `instrument` | inline in `deliver` Phase 3 | Adds structured logs at entry/error points; surfaces SLO candidates |
| `health` | inline in `improve` Phase 2 | Health scorecard — 8 dimensions with trend tracking |
| `performance` | agent | Performance bottleneck analysis (partial overlap with Phase 6) |
| `data-flow` | agent | Data flow and PII mapping (partial overlap with Phase 3) |

**Note:** Instrumentation runs automatically as part of every `/deliver` cycle. Health scoring runs automatically as part of every `/improve` cycle.

**Remaining gap:** No alerting-rules code generation; coverage is detection and gap reporting only.

---

### Phase 10 — Incident Management
*Postmortem, runbooks, on-call support*

| Component | Type | Description |
|-----------|------|-------------|
| `postmortem` | agent | Structured blameless postmortem document + action item ticketing |
| `root-cause` | agent | Deep root cause analysis using 5-Whys |
| `runbook` | agent | Operational runbooks for restart, rollback, secret rotation |

**Chain:** `root-cause` → `postmortem` → `runbook` → `project-manager` (action items) — explicitly modeled as a hyperedge in the knowledge graph.

---

### Phase 11 — Continuous Improvement
*Retrospectives, tech debt, stale work, health trends*

| Component | Type | Description |
|-----------|------|-------------|
| `health` | inline in `improve` Phase 2 | Codebase health scorecard with 8 dimensions and trend tracking |
| `retrospective` | inline in `improve` Phase 5 | Blameless sprint retrospective with Start/Stop/Continue |
| `stale-work` | inline in `improve` Phase 3 | Orphaned branches, stale PRs, zombie flags, closed-ticket TODOs |
| `dead-code` | inline in `improve` Phase 3 | Unused exports, unreachable code, orphaned files |
| `standup` | *(removed — covered by `improve` Phase 5 git activity summary)* | |
| `tech-debt` | agent | Business-prioritized tech debt register with ROI |
| `dep-audit` | agent | Dependency staleness and security health |

---

### Phase 12 — Documentation
*Guides, API reference, CLAUDE.md, code explanation*

| Component | Type | Description |
|-----------|------|-------------|
| `gen-docs` | agent | Write new project documentation from scratch — invoked by `devxp` |
| `update-docs` | agent | Refresh stale documentation in place — invoked by `devxp` |
| `gen-indexer` | agent | Generate directive CLAUDE.md from scratch — invoked by `devxp` |
| `update-indexer` | agent | Refresh existing CLAUDE.md sections — invoked by `devxp` |
| `explain` | inline in `devxp` | Audience-calibrated code explanation |
| `git-archaeology` | inline in `devxp` | Reconstruct intent and history from git |
| `onboarding` | agent | Structured onboarding guides for new contributors |
| `docs-sync` | agent | Documentation sync across files |

---

### Cross-cutting / Meta
*Components that span all phases or coordinate the swarm*

| Component | Type | Description |
|-----------|------|-------------|
| `swarm-status` | inline in `devxp` Phase 3 | Recommends which specialists to activate for the current work context |
| `synthesis` | agent | Consolidates multi-agent findings into a single action plan |

---

## Gaps & Recommendations

### Thin or Missing Coverage

All previously identified gaps have been closed via orchestrator enhancements rather than adding new slash commands.

**Phase 8 — Deployment & Infrastructure (now Strong)**
- IaC changes are handled inline in `/deliver` Phase 2 — detect, read conventions, apply same discipline as application code
- `/improve` Phase 2 now includes an Infrastructure Health dimension in the health scorecard
- Remaining gap: environment promotion automation (dev → staging → prod) — not yet covered

**Phase 9 — Observability (now Strong)**
- `/deliver` Phase 3 now surfaces SLO candidates for each new critical path (latency, error rate, throughput)
- `/improve` Phase 2 now includes an Observability Maturity dimension (SLIs documented, alerts exist, runbooks linked)
- Remaining gap: no alerting-rules code generation; coverage is detection and gap reporting only

**Phase 5 — Testing (gap closed)**
- `/deliver` Phase 4 now checks for E2E coverage after unit/integration tests, generating scenarios if the project already has a suite
- No new E2E framework is scaffolded unilaterally — the check is conditional on the suite's existence

---

### Overlap Flags

**Phase 6 — Code Review**  
`/deliver` Phase 5 runs correctness + quality inline, then invokes `pr-review` automatically. For a standalone deep review outside the delivery cycle, invoke `backend-senior-dev`, `frontend-senior-dev`, or `security` agents directly.

---

## Changelog

Added in session 2026-06-08:
- `requirements` skill — Phase 2 (Requirements)
- `instrument` skill — Phase 9 (Observability)
- `feature-flag` skill — Phase 4 (Implementation) + Phase 8 (Deployment)
- `env-audit` skill — Phase 8 (Deployment & Infrastructure)
- `load-test` skill — Phase 5 (Testing)
- `coverage-map` skill — Cross-cutting / Meta (this report)
- `swarm-status` skill — Cross-cutting / Meta

Phase rating changes 2026-06-08:
- Phase 2 (Requirements): Solid → **Strong** (added `requirements` skill)
- Phase 5 (Testing): Solid → **Strong** (added `load-test` skill)
- Phase 8 (Deployment): Thin → **Solid** (added `env-audit` + `feature-flag`)
- Phase 9 (Observability): Thin → **Solid** (added `instrument` skill)

Added in session 2026-06-09 (slash command surface reduced to 5):
- 42 standalone skills removed as slash commands — all capabilities preserved inline in orchestrators or as agents
- 4 doc skills (`gen-indexer`, `update-indexer`, `gen-docs`, `update-docs`) converted to agents; `devxp` updated to invoke them via agent reads
- `tech-lead` agent: added Phase 2e (API design) and Phase 2f (database schema design)
- `dev-agent`: added feature-flag lifecycle section
- `/deliver`: added regression check, load test offer (Phase 4), correctness + quality pre-review (Phase 5)
- `/improve`: added Env Var Health dimension (Phase 2), convention audit (Phase 3)
- `devxp`: added inline modes for explain, git archaeology, and routing recommendations
- `docs/guides/quickstart.md`: updated "Going deeper" section; added orchestrator examples table
- `docs/coverage.md`: component map updated to reflect new model (types: inline/agent)
- `README.md`: skills table replaced with 5-command summary

Added in session 2026-06-09 (orchestrator enhancements — no new slash commands):
- `/deliver` Phase 2: IaC awareness (scan + inline handling)
- `/deliver` Phase 3: SLO candidate surfacing for new critical paths
- `/deliver` Phase 4: E2E coverage check after unit/integration tests
- `/deliver` Phase 7: updated report to include infrastructure, SLO, and E2E status
- `/improve` Phase 2: Infrastructure Health dimension added to health scorecard
- `/improve` Phase 2: Observability Maturity dimension added to health scorecard
- `docs/guides/quickstart.md`: new zero-to-shipped guide using only the 4 orchestrators
- `docs/guides/install.md`: opencode feature parity callout added
- CLI installer: opencode feature parity warning added before install proceeds

Phase rating changes 2026-06-09:
- Phase 5 (Testing): gap closed via `/deliver` Phase 4 E2E check → **Strong** (no gap)
- Phase 8 (Deployment): Solid → **Strong** (IaC inline in `/deliver` + Infrastructure Health in `/improve`)
- Phase 9 (Observability): Solid → **Strong** (SLO surface in `/deliver` + Observability Maturity in `/improve`)
