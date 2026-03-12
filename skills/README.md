# Skills

Each subdirectory here contains one skill. Skills are invoked in Claude Code via slash commands: `/skill-name`.

Install them by running `../install.sh` from the repo root. Installed skills land in `~/.claude/skills/<name>/skill.md`.

---

## Skill Index

### Orchestration

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `devexp/` | `/devexp` | Central orchestrator — routes development requests to the appropriate specialized skill |

---

### Analysis and Understanding

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `devexp-analyze/` | `/devexp-analyze` | Deep codebase analysis: project structure, tech stack, entry points, architecture |
| `devexp-arch-review/` | `/devexp-arch-review` | Reviews architecture patterns, design decisions, and structural quality |
| `devexp-dep-mapper/` | `/devexp-dep-mapper` | Maps module and package dependencies; finds circular dependencies and unused packages |
| `devexp-path-tracer/` | `/devexp-path-tracer` | Traces happy paths and execution flows through the codebase |
| `devexp-logic-review/` | `/devexp-logic-review` | Deep code logic review: control flow, edge cases, null dereferences, race conditions |
| `devexp-quality-review/` | `/devexp-quality-review` | Code quality assessment: style, complexity metrics, maintainability, SOLID principles |

---

### Implementation

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `devexp-feature/` | `/devexp-feature` | Spec-driven feature implementation: requirements analysis, design, code, tests, docs |
| `devexp-backend-dev/` | `/devexp-backend-dev` | Implements backend services, APIs, business logic, and data access layers |
| `devexp-frontend-dev/` | `/devexp-frontend-dev` | Implements frontend components, UI logic, forms, routing, and state management |
| `devexp-api-design/` | `/devexp-api-design` | Designs API contracts: endpoints, schemas, error handling, auth, versioning |
| `devexp-db-design/` | `/devexp-db-design` | Designs database schemas, migrations, indexes, and query optimization |
| `devexp-refactor/` | `/devexp-refactor` | Code refactoring with safety checks: extract, inline, rename, simplify, deduplicate |

---

### Bug Fixing

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `devexp-bugfix/` | `/devexp-bugfix` | Full bug fix workflow: reproduce, root cause analysis, minimal fix, regression test, verify |
| `devexp-root-cause/` | `/devexp-root-cause` | Deep root cause analysis using 5 Whys — finds the true cause, not just the symptom |
| `devexp-fix-verify/` | `/devexp-fix-verify` | Verifies that a bug fix works correctly and has no side effects |

---

### Testing

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `devexp-test/` | `/devexp-test` | Test discovery, execution, result analysis, and coverage reporting |
| `devexp-unit-test/` | `/devexp-unit-test` | Runs unit tests and reports results, failure details, and slow tests |
| `devexp-integration-test/` | `/devexp-integration-test` | Sets up environment and executes integration tests with external dependencies |
| `devexp-coverage/` | `/devexp-coverage` | Analyzes test coverage, finds critical uncovered code, prioritizes gaps |
| `devexp-regression/` | `/devexp-regression` | Runs regression suites to confirm bug fixes don't break existing behavior |
| `devexp-flaky-test/` | `/devexp-flaky-test` | Detects unreliable tests, identifies root causes, recommends fixes |

---

### Security and Performance

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `devexp-security-audit/` | `/devexp-security-audit` | Security audit: injection, auth flaws, data exposure, crypto issues, misconfigurations |
| `devexp-perf-profile/` | `/devexp-perf-profile` | Performance profiling: CPU, memory, I/O, database queries, algorithm efficiency |

---

### Documentation

| Directory | Slash Command | Description |
|-----------|---------------|-------------|
| `devexp-docs/` | `/devexp-docs` | Documentation generation, maintenance, and improvement (code docs, guides, READMEs) |
| `devexp-api-docs/` | `/devexp-api-docs` | Generates API documentation from code: endpoints, schemas, request/response examples |
| `devexp-code-comments/` | `/devexp-code-comments` | Adds inline documentation and docstrings to undocumented code |
| `devexp-examples/` | `/devexp-examples` | Creates usage examples and code samples for APIs and libraries |
| `devexp-readme/` | `/devexp-readme` | Audits and updates README files: outdated content, missing sections, broken links |
