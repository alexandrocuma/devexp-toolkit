# Agents

Each file in this directory is a Claude Code sub-agent. Install them by running `../install.sh` from the repo root.

Agents are launched by Claude using the `Agent` tool. The `description` field in each agent's frontmatter is what Claude reads to decide when to invoke it — write descriptions carefully.

---

## Agents in This Directory

### dev-agent

**File:** `dev-agent.md`

The primary autonomous development agent. Handles the full spectrum of implementation work:

- **Bug fixes** — traces the execution path, identifies root cause, implements a minimal fix, adds a regression test
- **Feature implementation** — orients in the codebase, finds the canonical pattern, implements consistently with tests
- **Legacy rehabilitation** — triages broken/messy code, stabilizes, then modernizes
- **Refactoring** — restructures code incrementally without changing external behavior
- **Complex multi-step tasks** — breaks work into phases, uses task tracking, reports at phase boundaries

The dev-agent uses the codebase-navigator atlas when available, and delegates reviews to backend-senior-dev or frontend-senior-dev when appropriate.

---

### backend-senior-dev

**File:** `backend-senior-dev.md`

A senior backend engineer and architect with deep expertise across Python, Go, Java, TypeScript/Node.js, Rust, C#, Ruby, and PHP. Performs structured, high-signal code reviews.

Review output covers:
- Good patterns identified (not just criticism)
- Critical issues (must fix) with concrete remediation
- Significant improvements (should fix)
- Recommendations (consider)
- Algorithm complexity and performance analysis
- Honest verdict: Needs Major Rework / Needs Revision / Acceptable with Minor Changes / Good / Excellent

The agent can operate in **Review Mode** (report only) or **Fix Mode** (apply fixes directly for critical and mechanical issues).

---

### frontend-senior-dev

**File:** `frontend-senior-dev.md`

A senior frontend developer covering React, Vue, Angular, Svelte, Solid.js, TypeScript, JavaScript, HTML, CSS/SCSS/Tailwind, and build tooling. Philosophy: pragmatic excellence — ships, is maintainable, and is appropriate for the context.

Focuses on:
- Memory leaks, race conditions, and XSS vulnerabilities (always flags)
- Missing accessibility attributes
- Framework misuse and anti-patterns
- Good separation of concerns and composability

Like backend-senior-dev, it supports both Review Mode and Fix Mode.

---

### codebase-navigator

**File:** `codebase-navigator.md`

Builds and maintains a persistent "codebase atlas" — a structured map of any software project. The atlas is stored in `~/.claude/agent-memory/codebase-navigator/` and persists across sessions.

Other agents check the atlas before starting work so they can match existing conventions precisely.

The atlas covers:
- Tech stack (language, framework, ORM, auth, test framework, build tools)
- Architecture pattern and layer map
- Naming conventions (verified by triangulation across 3–5 examples)
- Entry points and key cross-cutting files
- Dependency injection approach
- Error handling pattern
- Test conventions
- Canonical example (the best-implemented feature in the codebase)
- Known technical debt and gotchas

---

### feature-path-tracer

**File:** `feature-path-tracer.md`

Traces exactly one execution path through code from entry point to terminal outcome. Strict single-path constraint: at every branch point, it identifies all alternatives, declares which one it is following, and continues exclusively down that path.

Useful for:
- Understanding an API endpoint end-to-end before modifying it
- Investigating a specific failure scenario without reading every possible branch
- Onboarding to complex logic by reading the happy path first
- Pre-modification understanding in unfamiliar code

Output: numbered execution chain, branch decisions documented, key findings, trace outcome, and an optional ASCII flow diagram.

---

### pr-feedback

**File:** `pr-feedback.md`

Reads all review comments on an existing GitHub PR and autonomously implements the actionable ones. Complements `pr-review` (which generates reviews) — this agent closes the loop on review cycles.

Triages comments into four categories:
- **Implement** — concrete, actionable code changes (renames, null checks, pattern fixes)
- **Ask** — ambiguous requests requiring a design decision before acting
- **Flag** — architectural or structural changes that belong in a separate PR
- **Skip** — already resolved or non-actionable (praise, questions already answered)

Reports a summary table of what was implemented, what was flagged, and what needs user input. Chains into `dev-agent` if tests fail post-implementation.

---

### dep-audit

**File:** `dep-audit.md`

Audits project dependencies for known vulnerabilities (CVEs) and staleness using ecosystem-native tools. Operates at the package manifest level — distinct from the `security` agent, which scans code patterns.

Supported ecosystems: Node.js (npm/yarn audit), Go (govulncheck), Python (pip-audit/safety), Rust (cargo audit), Ruby (bundle audit).

Produces a severity-ranked report (Critical/High/Medium/Low) covering:
- Known CVEs with CVE IDs, affected version ranges, and patched versions
- Major-version staleness gaps (packages that may be missing backported security patches)
- Specific upgrade commands for each finding

Chains into `security` for code-level impact assessment of Critical/High findings, and into `migration` for systematic upgrade planning.

---

### runbook

**File:** `runbook.md`

Generates operational runbooks — step-by-step procedures for restarting services, rolling back deployments, rotating secrets, scaling, and draining traffic. Discovers real commands from the project's actual configuration files (Makefile, docker-compose.yml, k8s manifests, CI workflows) — never fabricates commands.

Writes to `docs/runbooks/<service>-<operation>.md` using a standardized SRE format: purpose, when to run, prerequisites, numbered steps with expected output, rollback procedure, and escalation path.

Uses `[FILL IN]` placeholders honestly when a command cannot be sourced from the project's config files. Chains into `postmortem` when a runbook is written in response to an incident.
