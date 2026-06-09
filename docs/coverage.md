# DevExp SDLC Coverage Map

Generated: 2026-06-08  
Agents: 30 · Skills: 43 · Total components: 73  
Cross-cutting meta-skills: 3 (coverage-map, swarm-status, synthesis)

---

## Phase Coverage Summary

| Phase | Rating | Components | Notes |
|-------|--------|-----------|-------|
| 1. Discovery & Ideation | 🟢 Strong | 5 | graphify + codebase-navigator form a powerful orientation layer |
| 2. Requirements | 🟢 Strong | 7 | Full pipeline from idea → ticket → groomed plan |
| 3. Architecture & Design | 🟢 Strong | 9 | Most comprehensive phase — design, ADR, RFC, blast radius, data flow |
| 4. Implementation | 🟢 Strong | 9 | Autonomous, spec-driven, progressive delivery all covered |
| 5. Testing | 🟢 Strong | 5 | Unit/integration + load testing; E2E generation is the remaining gap |
| 6. Code Review | 🟢 Strong | 13 | ⚠️ Over-concentrated — routing guide needed (see below) |
| 7. CI/CD & Release | 🟢 Strong | 6 | Full commit → PR → pipeline → release workflow |
| 8. Deployment & Infrastructure | 🟡 Solid | 3 | env-audit + feature-flag + ci-cd; IaC management still missing |
| 9. Observability | 🟡 Solid | 2 | instrument covers setup; health covers monitoring; no tracing-specific skill |
| 10. Incident Management | 🟢 Strong | 4 | root-cause → postmortem → runbook chain is explicit |
| 11. Continuous Improvement | 🟢 Strong | 7 | Health trends + retro + debt + stale work — full feedback loop |
| 12. Documentation | 🟢 Strong | 8 | Create/refresh for both docs/ and CLAUDE.md; explanation + archaeology |

**Overall: 10 Strong · 2 Solid · 0 Thin · 0 Missing**

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
| `git-archaeology` | skill | Reconstructs intent and decision history from git history |

---

### Phase 2 — Requirements
*Writing user stories, acceptance criteria, scoping, estimation*

| Component | Type | Description |
|-----------|------|-------------|
| `requirements` | skill | Structures vague ideas into user stories and acceptance criteria |
| `groom` | skill | Validates ticket claims against codebase; produces verified execution plan |
| `grooming-agent` | agent | Automated grooming pipeline — fetches, validates, persists plan |
| `scope` | skill | Decomposes epics into atomic tickets with dependency graph |
| `ticket` | skill | Creates structured tickets across GitHub/GitLab/Linear/Jira |
| `estimation` | skill | Evidence-based complexity estimation from codebase |
| `project-manager` | agent | Creates and manages tickets; triages backlog |

---

### Phase 3 — Architecture & Design
*System design, API contracts, data models, decisions*

| Component | Type | Description |
|-----------|------|-------------|
| `rfc` | skill | Draft RFC before any code is written |
| `adr` | skill | Document decisions after they're made |
| `api-design` | skill | API contracts, endpoints, schemas, error handling |
| `db-design` | skill | Database schemas, migrations, indexes, query patterns |
| `convention-audit` | skill | Finds pattern divergence; recommends canonical patterns |
| `arch-review` | agent | Deep architectural health assessment |
| `tech-lead` | agent | Design reviews, ADRs, engineering standards, trade-off analysis |
| `impact-analysis` | agent | Blast radius mapping before implementation |
| `data-flow` | agent | End-to-end data flow mapping; PII exposure; data loss risks |

---

### Phase 4 — Implementation
*Writing code, refactoring, migrations, scaffolding, progressive delivery*

| Component | Type | Description |
|-----------|------|-------------|
| `feature` | skill | Spec-driven full feature lifecycle |
| `refactor` | skill | Structured code refactoring |
| `migrate` | skill | Library/framework version migrations |
| `bugfix` | skill | Root cause analysis + self-verifying fix |
| `feature-flag` | skill | Full flag lifecycle: create → gate → rollout → retire |
| `dev-agent` | agent | Autonomous end-to-end implementation |
| `scaffold` | agent | Generates code matching existing conventions |
| `migration` | agent | Library/framework migration agent |
| `pr-feedback` | agent | Implements review comments autonomously |

---

### Phase 5 — Testing
*Unit, integration, load, and E2E test generation and execution*

| Component | Type | Description |
|-----------|------|-------------|
| `test-gen` | skill | Generate test suites for untested/undertested code |
| `test-gen` | agent | Test generation agent |
| `test-runner` | agent | Runs tests, measures coverage, detects flaky tests |
| `regression` | skill | Verifies fixes don't introduce regressions |
| `load-test` | skill | Generates load test scenarios for critical endpoints |

**Remaining gap:** E2E scenario generation (Playwright/Cypress/Selenium-style). `test-gen` covers unit/integration; `load-test` covers performance. Browser-driven scenario testing is not yet generated automatically.

---

### Phase 6 — Code Review
*Pre-merge review, security audit, quality checks*

| Component | Type | Description |
|-----------|------|-------------|
| `review-pr` | skill | Surgical pre-merge review; posts inline GitHub/GitLab comments |
| `logic-review` | skill | Correctness-focused: bugs, edge cases, null dereferences |
| `quality` | skill | Style, complexity, maintainability |
| `convention-audit` | skill | Pattern audit — finds divergence from canonical patterns |
| `dead-code` | skill | Unused exports, unreachable branches, zombie flags |
| `pr-review` | agent | Thorough PR review: bugs, security, patterns, tests |
| `backend-senior-dev` | agent | Expert backend review with architecture guidance |
| `frontend-senior-dev` | agent | Expert frontend review and UI architecture |
| `security` | agent | OWASP Top 10, auth flaws, data exposure |
| `performance` | agent | Bottleneck identification, complexity analysis |
| `dep-audit` | agent | CVE scan and dependency staleness |
| `dep-map` | agent | Circular dependencies, unused imports |
| `impact-analysis` | agent | Blast radius of proposed changes |

⚠️ **13 components — see routing guide in `docs/reference/skills.md`**

---

### Phase 7 — CI/CD & Release
*Pipeline config, changelog, versioning, tagging*

| Component | Type | Description |
|-----------|------|-------------|
| `commit` | skill | Conventional commit generation |
| `pr` | skill | PR/MR description + optional open via CLI |
| `changelog` | skill | Changelog entry from conventional commits |
| `release` | skill | Full release: version bump → changelog → commit → tag → platform |
| `changelog` | agent | Changelog generation agent |
| `ci-cd` | agent | Debug, create, optimize CI/CD pipelines |

---

### Phase 8 — Deployment & Infrastructure
*Environment management, secrets, IaC, configuration*

| Component | Type | Description |
|-----------|------|-------------|
| `env-audit` | skill | Audits env vars — undocumented reads, leaked secrets, config drift |
| `feature-flag` | skill | Progressive delivery + safe rollout control |
| `ci-cd` | agent | Pipeline config and deployment workflow |

**Remaining gap:** Infrastructure-as-code (IaC) management. No skill scaffolds or audits infrastructure definitions. Environment promotion workflows (dev → staging → prod) are not automated.

---

### Phase 9 — Observability
*Logging instrumentation, metrics, tracing, monitoring*

| Component | Type | Description |
|-----------|------|-------------|
| `instrument` | skill | Adds structured logs, metrics, tracing to match project conventions |
| `health` | skill | Health scorecard — reads observability state across 6 dimensions |
| `performance` | agent | Performance bottleneck analysis (partial overlap with Phase 6) |
| `data-flow` | agent | Data flow and PII mapping (partial overlap with Phase 3) |

**Note:** `instrument` is the dedicated observability-setup specialist. `health` reads existing state; `performance` and `data-flow` are analysts, not instrumenters.

**Remaining gap:** Distributed tracing setup is handled generically by `instrument` but there's no alerting-rules review or SLO/SLA documentation skill.

---

### Phase 10 — Incident Management
*Postmortem, runbooks, on-call support*

| Component | Type | Description |
|-----------|------|-------------|
| `postmortem` | skill | Structured blameless postmortem document |
| `postmortem` | agent | Postmortem agent with action item ticketing |
| `root-cause` | agent | Deep root cause analysis using 5-Whys |
| `runbook` | agent | Operational runbooks for restart, rollback, secret rotation |

**Chain:** `root-cause` → `postmortem` → `runbook` → `project-manager` (action items) — explicitly modeled as a hyperedge in the knowledge graph.

---

### Phase 11 — Continuous Improvement
*Retrospectives, tech debt, stale work, health trends*

| Component | Type | Description |
|-----------|------|-------------|
| `health` | skill | Codebase health scorecard with trend tracking |
| `retrospective` | skill | Blameless sprint retrospective with Start/Stop/Continue |
| `stale-work` | skill | Orphaned branches, stale PRs, zombie flags, closed-ticket TODOs |
| `dead-code` | skill | Unused exports, unreachable code, orphaned files |
| `standup` | skill | Daily standup from git activity |
| `tech-debt` | agent | Business-prioritized tech debt register with ROI |
| `dep-audit` | agent | Dependency staleness and security health |

---

### Phase 12 — Documentation
*Guides, API reference, CLAUDE.md, code explanation*

| Component | Type | Description |
|-----------|------|-------------|
| `gen-docs` | skill | Write new project documentation from scratch |
| `update-docs` | skill | Refresh stale documentation in place |
| `gen-indexer` | skill | Generate directive CLAUDE.md from scratch |
| `update-indexer` | skill | Refresh existing CLAUDE.md sections |
| `explain` | skill | Audience-calibrated code explanation |
| `git-archaeology` | skill | Reconstruct intent and history from git |
| `onboarding` | agent | Structured onboarding guides for new contributors |
| `docs-sync` | agent | Documentation sync across files |

---

### Cross-cutting / Meta
*Components that span all phases or coordinate the swarm*

| Component | Type | Description |
|-----------|------|-------------|
| `swarm-status` | skill | Recommends which specialists to activate for the current work context |
| `coverage-map` | skill | Generates this report — SDLC phase coverage self-assessment |
| `synthesis` | agent | Consolidates multi-agent findings into a single action plan |

---

## Gaps & Recommendations

### Thin or Missing Coverage

**Phase 8 — Deployment & Infrastructure (Solid → gap: IaC)**
- No skill for managing infrastructure definitions (Terraform, Pulumi, CDK, etc.)
- No environment promotion automation (dev → staging → prod pipeline)
- `env-audit` covers config; `ci-cd` covers pipelines; the infrastructure layer itself is unaddressed
- *Suggested addition:* `/infra-review` skill — audit IaC definitions for security, drift, and best practices (tool-agnostic)

**Phase 9 — Observability (Solid → gap: alerting/SLOs)**
- `instrument` covers adding signals; `health` reads them
- No alerting rules review or SLO/SLA documentation skill
- Distributed tracing setup is handled generically; no specialist for trace propagation patterns
- *Suggested addition:* Extend `instrument` with an `--slo` mode that documents service level objectives based on the existing health thresholds

**Phase 5 — Testing (Strong → gap: E2E)**
- Unit and integration: covered by `test-gen` + `test-runner`
- Load: covered by `load-test`
- E2E browser-driven scenarios: not generated
- *Suggested addition:* Extend `test-gen` with an `--e2e` mode that generates browser-driven test scenarios (tool-agnostic)

---

### Overlap Flags

**Phase 6 — Code Review (13 components)**  
See routing guide in [`docs/reference/skills.md`](reference/skills.md#choosing-the-right-review-tool).

Quick reference:
- Merging → `/review-pr`
- Logic correctness → `/logic-review`
- Style/complexity → `/quality`
- Full service audit → `backend-senior-dev` or `frontend-senior-dev`
- Security → `security` agent
- Dependencies → `dep-audit` agent

**`postmortem` appears as both skill and agent** — they serve slightly different roles (skill writes the doc; agent also creates action item tickets). Both are valid; the agent is the orchestrated version of the skill.

**`test-gen` appears as both skill and agent** — same pattern. The skill is directly invocable; the agent is the spawnable version for use in multi-agent workflows.

---

## Changelog

*This is the first generated report — no previous baseline exists.*

Added in this session (2026-06-08):
- `requirements` skill — Phase 2 (Requirements)
- `instrument` skill — Phase 9 (Observability)
- `feature-flag` skill — Phase 4 (Implementation) + Phase 8 (Deployment)
- `env-audit` skill — Phase 8 (Deployment & Infrastructure)
- `load-test` skill — Phase 5 (Testing)
- `coverage-map` skill — Cross-cutting / Meta (this report)
- `swarm-status` skill — Cross-cutting / Meta

Phase rating changes from pre-session state:
- Phase 2 (Requirements): Solid → **Strong** (added `requirements` skill)
- Phase 5 (Testing): Solid → **Strong** (added `load-test` skill)
- Phase 8 (Deployment): Thin → **Solid** (added `env-audit` + `feature-flag`)
- Phase 9 (Observability): Thin → **Solid** (added `instrument` skill)
