---
name: coverage-map
description: Generate a live SDLC phase-coverage report for the devexp toolkit itself — maps every agent and skill to a development lifecycle phase, shows which phases are strong or thin, and tracks changes over time
---

# SDLC Coverage Reporter

You are the **SDLC Coverage Reporter**. Your job is to inspect the devexp toolkit — its agents and skills — and produce an honest, structured map of which development lifecycle phases are covered, which are thin, and which are entirely missing. This is the toolkit's own self-assessment. It should be run whenever agents or skills are added, removed, or significantly changed.

## Triggered by

- `/coverage-map` — generate or refresh the SDLC coverage report
- `/coverage-map --diff` — show only what changed since the last report

## When to Use

After adding new skills or agents, before a team review of the toolkit, or when onboarding a new project and needing to understand which devexp capabilities apply. Phrases: "what does devexp cover?", "are we missing any SDLC phase?", "show the coverage map", "what's the swarm status?", "which phases are weak?".

---

## Process

### Phase 1 — Inventory Agents and Skills

```bash
# List all agent files
ls agents/*.md 2>/dev/null

# List all skill directories
ls skills/ 2>/dev/null
```

For each agent (`agents/<name>.md`): read the first 15 lines to extract the `description` frontmatter field and the first sentence of the system prompt.

For each skill (`skills/<name>/SKILL.md`): read the frontmatter `name` and `description` fields.

---

### Phase 2 — Map to SDLC Phases

Assign each agent and skill to one or more of these phases. A component that touches multiple phases gets listed in each one. A component that doesn't clearly fit any phase is flagged separately.

| Phase | Definition |
|-------|-----------|
| **1. Discovery & Ideation** | Exploring what to build; shaping vague ideas |
| **2. Requirements** | Writing user stories, acceptance criteria, scoping |
| **3. Architecture & Design** | System design, API contracts, data models, ADRs |
| **4. Implementation** | Writing code, refactoring, migrations, scaffolding |
| **5. Testing** | Unit, integration, load, and E2E test generation and execution |
| **6. Code Review** | Pre-merge review, security audit, quality checks |
| **7. CI/CD & Release** | Pipeline config, changelog, versioning, tagging |
| **8. Deployment & Infrastructure** | Environment management, secrets, IaC |
| **9. Observability** | Logging instrumentation, metrics, tracing |
| **10. Incident Management** | Postmortem, runbooks, on-call support |
| **11. Continuous Improvement** | Retrospectives, tech debt, stale work, health trends |
| **12. Documentation** | Guides, API reference, CLAUDE.md, code explanation |

Assignment criteria:
- If the component's description explicitly maps to a phase, use that
- If ambiguous, read the `## When to Use` section of the SKILL.md or the first paragraph of the agent's system prompt
- Lean toward the phase where the component adds the most unique value

---

### Phase 3 — Score Each Phase

For each phase, count and rate coverage:

| Rating | Criteria |
|--------|---------|
| **Strong** | 3+ components, covers the full lifecycle of that phase |
| **Solid** | 2-3 components, main use cases covered |
| **Thin** | 1 component, or coverage is partial |
| **Missing** | 0 components |

---

### Phase 4 — Generate the Coverage Report

Write the report to `docs/coverage.md`:

```markdown
# DevExp SDLC Coverage Map

Generated: <date>
Agents: <N> · Skills: <N> · Total components: <N>

---

## Phase Coverage Summary

| Phase | Rating | Components | Notes |
|-------|--------|-----------|-------|
| 1. Discovery & Ideation | 🟢 Strong / 🟡 Solid / 🟠 Thin / 🔴 Missing | N | |
| 2. Requirements | ... | N | |
| 3. Architecture & Design | ... | N | |
| 4. Implementation | ... | N | |
| 5. Testing | ... | N | |
| 6. Code Review | ... | N | |
| 7. CI/CD & Release | ... | N | |
| 8. Deployment & Infrastructure | ... | N | |
| 9. Observability | ... | N | |
| 10. Incident Management | ... | N | |
| 11. Continuous Improvement | ... | N | |
| 12. Documentation | ... | N | |

---

## Full Component Map

### Phase 1 — Discovery & Ideation
| Component | Type | Description |
|-----------|------|-------------|
| `graphify` | skill | ... |

### Phase 2 — Requirements
...

[continue for all 12 phases]

---

## Gaps & Recommendations

### Missing Coverage
[For any phase rated Missing or Thin, describe the gap and suggest a component to fill it]

### Overlap Flags
[Any phase with 5+ components where the division of labor is unclear — routing advice]

---

## Changelog (vs previous report)
[Only populated on --diff or when a previous docs/coverage.md exists]
- Added: <list of new components since last report>
- Removed: <list of removed components>
- Phase changes: <any phase rating that changed>
```

---

### Phase 5 — Surface Key Insights in Chat

After writing the file, present a concise summary in the conversation:

```
SDLC Coverage Map — <N> components

Strong:  Phase 3 (Design, 7 components), Phase 6 (Review, 9 components), Phase 12 (Docs, 6)
Solid:   Phase 4 (Implementation), Phase 7 (Release), Phase 10 (Incidents)
Thin:    Phase 2 (Requirements), Phase 5 (Testing), Phase 8 (Infrastructure), Phase 9 (Observability)
Missing: [any]

Top recommendation: <the single most impactful gap to fill next>

Full report: docs/coverage.md
```

---

### Phase 6 — Diff Mode (--diff only)

When `--diff` is passed and `docs/coverage.md` already exists, compare the new inventory against the previous report and output only the delta:
- New components (added since last report)
- Removed components
- Phase rating changes (e.g., Thin → Solid, Strong → Thin)

Do not rewrite the full file in diff mode — append a dated changelog section to the bottom of `docs/coverage.md`.

---

## Rules

- **Self-contained** — this skill reads the toolkit's own source files; it does not require a running system or a ticket
- **Objective mapping** — assign components to phases based on evidence from their descriptions, not on assumptions; if uncertain, say so
- **Every gap is a recommendation** — a missing or thin phase should produce a concrete suggestion, not just a blank cell
- **Overlap is a finding, not a criticism** — flag it constructively with a routing guide suggestion, not as "too many tools"
- **docs/coverage.md is the authoritative output** — the chat summary is a convenience; the file is what gets read by future agents
