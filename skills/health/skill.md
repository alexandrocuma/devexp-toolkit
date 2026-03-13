---
name: health
description: Generate a codebase health scorecard covering tests, security, dependencies, quality, and CI status
---

# Codebase Health Check

You are generating a **codebase health scorecard** — a structured assessment of the project's quality across multiple dimensions, each rated Red/Amber/Green with specific findings and recommended actions.

## Triggered by

- `/health` — direct invocation for a full health report

## When to Use

When the user wants a structured assessment of codebase quality across tests, security, dependencies, CI, and tech debt. Phrases: "check codebase health", "give me a health report", "how healthy is this project", "scorecard".

## Process

### 1. Establish baseline context

```bash
git rev-parse --show-toplevel 2>/dev/null || pwd  # project root
git log --oneline -10                              # recent activity
```

Check for codebase-navigator atlas:
```bash
cat ~/.claude/agent-memory/codebase-navigator/MEMORY.md 2>/dev/null
```

Read the project manifest to identify the stack:
```bash
cat package.json 2>/dev/null
cat go.mod 2>/dev/null
cat pyproject.toml 2>/dev/null
cat Cargo.toml 2>/dev/null
```

### 2. Run health checks by dimension

Run all checks that are applicable to the detected stack. Clearly note "N/A — not applicable to this stack" for checks that don't apply.

#### Dimension 1: Test Coverage

```bash
# Node.js / Jest
npx jest --coverage --coverageReporters=text-summary 2>/dev/null | tail -20

# Go
go test ./... -coverprofile=coverage.out 2>/dev/null && go tool cover -func=coverage.out | tail -5

# Python
python -m pytest --cov --cov-report=term-missing 2>/dev/null | tail -20

# Any
ls **/*.test.* **/*.spec.* **/*_test.* 2>/dev/null | wc -l  # count test files
```

Coverage thresholds:
- Green: > 80% overall, no critical path with < 60%
- Amber: 60-80% overall, or critical paths under-covered
- Red: < 60% overall, or no tests at all

#### Dimension 2: Security Vulnerabilities

```bash
# Node.js
npm audit --audit-level=moderate 2>/dev/null | tail -20

# Python
pip-audit 2>/dev/null || safety check 2>/dev/null

# Go
govulncheck ./... 2>/dev/null

# Any — check for hardcoded secrets
grep -r "password\s*=\s*['\"]" --include="*.js" --include="*.py" --include="*.go" --include="*.ts" . 2>/dev/null | grep -v test | grep -v example
grep -rE "(api_key|secret|token)\s*=\s*['\"][a-zA-Z0-9]{16,}" . 2>/dev/null | grep -v test
```

Security thresholds:
- Green: no high/critical vulnerabilities, no hardcoded secrets
- Amber: moderate vulnerabilities only, or 1-2 low-severity secrets in test code
- Red: any critical/high vulnerability, or hardcoded secrets in non-test code

#### Dimension 3: Dependency Health

```bash
# Node.js — outdated packages
npm outdated 2>/dev/null

# Python
pip list --outdated 2>/dev/null

# Go
go list -m -u all 2>/dev/null | grep "\["

# Check for circular dependencies (if dep-map tooling available)
# Check package count and last updated dates
```

Dependency thresholds:
- Green: no packages > 2 major versions behind, no circular dependencies
- Amber: some packages significantly behind, or minor circular dependencies in non-critical code
- Red: critical packages (auth, crypto, web framework) multiple major versions behind, or circular dependencies in core code

#### Dimension 4: Code Quality Metrics

```bash
# Count TODO/FIXME/HACK comments
grep -rn "TODO\|FIXME\|HACK\|XXX" --include="*.js" --include="*.ts" --include="*.py" --include="*.go" . 2>/dev/null | grep -v node_modules | grep -v ".git" | wc -l

# Find large files (potential god objects)
find . -name "*.js" -o -name "*.ts" -o -name "*.py" -o -name "*.go" 2>/dev/null | grep -v node_modules | grep -v ".git" | xargs wc -l 2>/dev/null | sort -rn | head -10

# Run linter if available
npx eslint . --format=compact 2>/dev/null | tail -5
golangci-lint run --out-format=line-number 2>/dev/null | tail -10
flake8 . 2>/dev/null | tail -10
```

Quality thresholds:
- Green: < 20 TODO/FIXME comments, no files > 500 lines, linter passes
- Amber: 20-50 TODO/FIXME, some large files (500-1000 lines), linter warnings
- Red: > 50 TODO/FIXME, files > 1000 lines, linter errors, or no linter configured

#### Dimension 5: CI/CD Status

```bash
# Check if CI config exists
ls .github/workflows/*.yml 2>/dev/null
ls .gitlab-ci.yml 2>/dev/null

# GitHub Actions: recent run status
gh run list --limit 5 2>/dev/null

# Check for open PRs
gh pr list --limit 10 2>/dev/null | wc -l

# Days since last release
git describe --tags --abbrev=0 2>/dev/null
git log $(git describe --tags --abbrev=0 2>/dev/null)..HEAD --oneline | wc -l
```

CI/CD thresholds:
- Green: CI configured, last run passed, releases are recent (< 30 days)
- Amber: CI configured but flaky, or > 30 days since last release with significant unreleased changes
- Red: no CI configured, CI consistently failing, or > 90 days since last release

#### Dimension 6: Open Tech Debt

```bash
# Count open tech-debt issues
gh issue list --label tech-debt --state open 2>/dev/null | wc -l

# Check for stale issues
gh issue list --label bug --state open --limit 50 2>/dev/null
```

Tech debt thresholds:
- Green: < 5 open tech-debt issues, bugs addressed promptly (< 30 days old)
- Amber: 5-15 open tech-debt issues, or some bugs > 30 days old
- Red: > 15 open tech-debt issues, or critical bugs > 7 days old

### 3. Generate the scorecard

```markdown
# Codebase Health Report

**Project**: <name>
**Date**: YYYY-MM-DD
**Stack**: <detected stack>

---

## Scorecard

| Dimension | Status | Summary |
|-----------|--------|---------|
| Test Coverage | 🟢 Green / 🟡 Amber / 🔴 Red | [one-line summary, e.g., "83% overall coverage"] |
| Security | 🟢 Green / 🟡 Amber / 🔴 Red | [e.g., "2 moderate npm vulnerabilities"] |
| Dependencies | 🟢 Green / 🟡 Amber / 🔴 Red | [e.g., "express is 2 major versions behind"] |
| Code Quality | 🟢 Green / 🟡 Amber / 🔴 Red | [e.g., "47 TODO comments, 3 files > 800 lines"] |
| CI/CD | 🟢 Green / 🟡 Amber / 🔴 Red | [e.g., "CI passing, last release 12 days ago"] |
| Tech Debt | 🟢 Green / 🟡 Amber / 🔴 Red | [e.g., "8 open tech-debt issues"] |

---

## Details

### Test Coverage — 🟢/🟡/🔴
[Specific coverage numbers by module. Which areas are well-covered? Which are under-covered?]
**Action**: [Specific recommendation if Amber or Red]

### Security — 🟢/🟡/🔴
[Vulnerability findings with CVE numbers and severity. Hardcoded secret locations if found.]
**Action**: [Specific recommendation]

### Dependencies — 🟢/🟡/🔴
[List of significantly outdated packages. Any known breaking changes in the newer versions.]
**Action**: [Specific recommendation]

### Code Quality — 🟢/🟡/🔴
[Top 3 largest files with line counts. TODO/FIXME count. Key linter findings.]
**Action**: [Specific recommendation]

### CI/CD — 🟢/🟡/🔴
[Pipeline existence and recent status. Days since last release. Open PRs count.]
**Action**: [Specific recommendation]

### Tech Debt — 🟢/🟡/🔴
[Open tech-debt issue count. Oldest open bug age. Most critical items.]
**Action**: [Specific recommendation]

---

## Priority Actions

1. 🔴 [Most urgent — address this week]: [specific action]
2. 🟡 [Address this sprint]: [specific action]
3. 🟡 [Address this sprint]: [specific action]
4. 🟢 [Address this quarter]: [specific action]

---

## Checks Not Run

[List any checks that couldn't run due to missing tooling or N/A stack, and how to run them manually]
```

### 4. Offer next steps

Based on the findings, suggest:
- **Red security findings** → "Run the `security` agent for a full vulnerability audit"
- **Red test coverage** → "Run the `test-gen` agent to generate tests for uncovered modules"
- **Red CI/CD** → "Run the `ci-cd` agent to debug and fix the pipeline"
- **Many open tech-debt items** → "Run the `project-manager` agent to triage and schedule the backlog"
- **Significantly outdated dependencies** → "Run the `migration` agent to plan the upgrade"

## Notes

- Some checks require tools that may not be installed — always note what was checked vs skipped
- Focus on actionable findings — "coverage is 74%" is not actionable; "the payments module has 0% test coverage" is
- Green does not mean perfect — it means the dimension is in an acceptable state relative to industry norms
