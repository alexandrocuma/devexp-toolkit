---
name: improve
description: Continuous improvement cycle — health scorecard, stale work cleanup, dead code removal, tech debt triage, and retrospective. Self-contained: no other skills required.
---

# Improve: Post-Sprint Continuous Improvement

You are running the **continuous improvement cycle** — the closing ceremony of each sprint and the opening check of the next. You surface what's working, what's accumulating debt, and what the team should do differently. Every diagnostic step runs inline; the tech debt triage delegates to a specialist agent.

## Triggered by

- `/improve` — full cycle
- `/improve --health` — health scorecard only
- `/improve --cleanup` — stale work + dead code only
- `/improve --retro` — retrospective only

## When to Use

At sprint end, after a production incident, during a maintenance window, or whenever the system "feels slow or messy." Also run after `/deliver` to verify the new code didn't degrade health metrics.

---

## Process

### Phase 0 — Establish Context

```bash
git log --oneline -10 2>/dev/null
git tag --sort=-creatordate 2>/dev/null | head -3
ls .devexp/health-baseline.json 2>/dev/null && echo "baseline: EXISTS" || echo "baseline: MISSING"
gh run list --limit 3 2>/dev/null | head -5 || glab pipeline list --limit 3 2>/dev/null | head -5
```

Note: last release date, whether a baseline exists (enables trend tracking), CI status.

If CI is failing: surface it immediately — "CI is currently failing. Recommend addressing this before other improvement steps — a failing pipeline invalidates health dimension data." Offer to proceed anyway or pause for `ci-cd` agent.

---

### Phase 1 — Present the Plan

```
## Improvement cycle

Context:
  Last release:  <date or "none">
  Health baseline: <found — will show trends / first run — will establish baseline>
  CI status:     <passing / failing>

Steps:
  1. Health scorecard    [always runs]
  2. Stale work + dead code scan   [optional — yes/skip]
  3. Tech debt triage              [optional — yes/skip]
  4. Retrospective                 [optional — yes/skip]

Proceed with all steps? (yes / health only / choose)
```

If `--health`, `--cleanup`, or `--retro` flags were passed, skip this prompt and run only the flagged step(s).

---

### Phase 2 — Health Scorecard  *(always runs)*

Run all applicable checks. Note "N/A — not applicable" for checks that don't apply to the detected stack.

**Test Coverage:**
```bash
npx jest --coverage --coverageReporters=text-summary 2>/dev/null | tail -5
go test ./... -coverprofile=coverage.out 2>/dev/null && go tool cover -func=coverage.out | tail -3
python -m pytest --cov --cov-report=term-missing 2>/dev/null | tail -5
find . -name "*.test.*" -o -name "*_test.*" -o -name "*.spec.*" 2>/dev/null | grep -v node_modules | wc -l
```
Thresholds: 🟢 > 80% · 🟡 60-80% · 🔴 < 60% or no tests

**Security:**
```bash
npm audit --audit-level=moderate 2>/dev/null | tail -5
pip-audit 2>/dev/null | tail -5 || safety check 2>/dev/null | tail -5
govulncheck ./... 2>/dev/null | tail -5
grep -rE "(api[_-]?key|secret|password|token)\s*[=:]\s*['\"][a-zA-Z0-9]{16,}" . 2>/dev/null | grep -v node_modules | grep -v test | grep -v ".env.example" | head -5
```
Thresholds: 🟢 no high/critical · 🟡 moderate only · 🔴 any critical or hardcoded secret

**Dependencies:**
```bash
npm outdated 2>/dev/null | head -10
pip list --outdated 2>/dev/null | head -10
go list -m -u all 2>/dev/null | grep "\[" | head -10
```
Thresholds: 🟢 no packages > 2 major versions behind · 🟡 some behind · 🔴 critical packages (auth, crypto, web framework) multiple majors behind

**Code Quality:**
```bash
grep -rn "TODO\|FIXME\|HACK\|XXX" . 2>/dev/null | grep -v node_modules | grep -v ".git" | wc -l
find . \( -name "*.js" -o -name "*.ts" -o -name "*.py" -o -name "*.go" \) 2>/dev/null | grep -v node_modules | grep -v ".git" | xargs wc -l 2>/dev/null | sort -rn | head -5
npx eslint . --format=compact 2>/dev/null | tail -3
golangci-lint run 2>/dev/null | tail -5
```
Thresholds: 🟢 < 20 TODOs, no files > 500 lines · 🟡 20-50 TODOs, some large files · 🔴 > 50 TODOs, files > 1000 lines

**CI/CD:**
```bash
gh run list --limit 5 2>/dev/null || glab pipeline list --limit 5 2>/dev/null
git describe --tags --abbrev=0 2>/dev/null
git log $(git describe --tags --abbrev=0 2>/dev/null)..HEAD --oneline 2>/dev/null | wc -l
```
Thresholds: 🟢 CI passing, release < 30 days · 🟡 CI flaky or > 30 days · 🔴 CI failing or > 90 days

**Load previous baseline for trend comparison:**
```bash
cat .devexp/health-baseline.json 2>/dev/null
```

**Generate scorecard:**

```
# Health Scorecard — <project> — <date>

| Dimension       | Status | Trend | Summary |
|-----------------|--------|-------|---------|
| Test Coverage   | 🟢/🟡/🔴 | ↑/→/↓/— | <one-line> |
| Security        | 🟢/🟡/🔴 | ↑/→/↓/— | <one-line> |
| Dependencies    | 🟢/🟡/🔴 | ↑/→/↓/— | <one-line> |
| Code Quality    | 🟢/🟡/🔴 | ↑/→/↓/— | <one-line> |
| CI/CD           | 🟢/🟡/🔴 | ↑/→/↓/— | <one-line> |

Trend: ↑ improving · → stable · ↓ degrading · — no baseline
```

**Save baseline:**
```bash
mkdir -p .devexp
```
Write `.devexp/health-baseline.json` with current scores, date, and git SHA. Append `.devexp/health-baseline.json` to `.gitignore` if not already there.

---

### Phase 3 — Stale Work + Dead Code  *(optional)*

**Stale work:**

```bash
# Orphaned branches (merged or > 30 days old, not the main branch)
git branch -r --merged 2>/dev/null | grep -v "main\|master\|HEAD" | head -10
git for-each-ref --sort=-committerdate refs/remotes --format='%(refname:short) %(committerdate:relative)' 2>/dev/null | tail -10

# Open PRs/MRs older than 14 days
gh pr list --state open --json number,title,createdAt 2>/dev/null | head -10
glab mr list --state opened 2>/dev/null | head -10

# Zombie feature flags (flags with no recent code changes in their surrounding code)
grep -rn "featureFlag\|isEnabled\|FLAG_\|FEATURE_\|getFlag" . 2>/dev/null | grep -v node_modules | grep -v test | head -15
```

**Dead code:**

```bash
# Exports that are never imported elsewhere
grep -rn "^export\|^module\.exports\|^pub fn\|^public func" . 2>/dev/null | grep -v node_modules | grep -v test | head -20

# TODO/FIXME comments referencing what might be stale work
grep -rn "TODO\|FIXME" . 2>/dev/null | grep -v node_modules | head -15

# Files with no git activity in > 90 days that aren't config files
git log --since="90 days ago" --name-only --format="" 2>/dev/null | sort -u > /tmp/.recently_changed.txt
find . -name "*.ts" -o -name "*.go" -o -name "*.py" 2>/dev/null | grep -v node_modules | while read f; do
  grep -qF "$f" /tmp/.recently_changed.txt 2>/dev/null || echo "STALE: $f"
done 2>/dev/null | head -10
rm -f /tmp/.recently_changed.txt
```

Present findings as a cleanup checklist:
```
Stale work found:
  Merged branches:     N (suggest: git branch -d <name>)
  Old open PRs:        N (suggest: close or merge)
  Zombie flags:        N (suggest: /feature-flag retire or run /deliver to remove)
  Orphaned exports:    N (suggest: remove if confirmed unused)
  Silent files:        N (suggest: verify still needed, archive if not)
```

Do not delete anything automatically — surface findings only.

---

### Phase 4 — Tech Debt Triage  *(optional)*

Invoke the `tech-debt` agent:

> "Produce a business-prioritized tech debt register for this codebase. Include: carrying cost estimate, ROI of fixing, and recommended sprint slot (now / next quarter / backlog). Include any 🔴 Red findings from this health scorecard as pre-qualified debt items: <paste Red scorecard items>."

Wait for the agent to complete. Present its prioritized list.

---

### Phase 5 — Retrospective  *(optional)*

Run a blameless retrospective synthesis from the evidence gathered.

**Gather the data:**
```bash
# Activity since last tag
git log $(git describe --tags --abbrev=0 2>/dev/null)..HEAD --oneline 2>/dev/null | head -30

# Files changed most frequently (hotspots)
git log --since="30 days ago" --name-only --format="" 2>/dev/null | sort | uniq -c | sort -rn | head -10

# PRs merged since last release
gh pr list --state merged --limit 20 --json number,title,mergedAt 2>/dev/null | head -20
```

Structure the retrospective findings:

```
# Sprint Retrospective — <date>

## Start Doing
- <Practice missing that the evidence suggests would help>
  Evidence: <specific signal from git log / health / incidents>

## Stop Doing
- <Practice that's causing friction or technical debt>
  Evidence: <specific signal>

## Continue Doing
- <Practice that's working — don't let it erode>
  Evidence: <specific signal>

## Commitments
- [ ] <Specific, measurable action with an owner and a date>
```

Rules for findings:
- Each item must be grounded in **evidence from the data** — no abstract "we should communicate better"
- Commitments must be specific: "add test coverage to payments module before next sprint" not "improve test coverage"
- Positive observations are not optional — Stop items without Continue items produce defensive teams

---

### Phase 6 — Report & Hand Off

```
## Improvement cycle complete

Health:      <N 🟢 / N 🟡 / N 🔴>  <trend vs last baseline>
Stale work:  <N items found> (if run)
Dead code:   <N items found> (if run)
Tech debt:   <top priority item> (if run)
Retro:       <N Start / N Stop / N Continue> (if run)

Critical actions before next sprint:
  <any 🔴 Red findings>
  <any ↓ degrading dimensions>

Next cycle:
  /refine "<next feature>"   — begin the next sprint
```

---

## Guidelines

- **Health always runs** — it's the minimum viable maintenance action; everything else is optional
- **Never auto-delete** — stale work and dead code scans produce findings for human review, not automated removal
- **CI failure pre-empts everything** — a failing pipeline invalidates the health scorecard's CI dimension; address it first
- **Retrospective items need evidence** — if you can't point to a git commit, health metric, or incident, the item is a feeling, not a finding
- **Trend direction matters more than absolute score** — a project with 65% coverage improving (↑) is in better shape than one at 80% degrading (↓)
