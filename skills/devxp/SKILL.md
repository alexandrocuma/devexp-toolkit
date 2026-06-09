---
name: devxp
description: Entry point for the devexp toolkit — orients on any repo, ensures CLAUDE.md and the docs/ index exist (generating or refreshing as needed), and optionally enriches with graphify. Always start here.
---

# DevExp Entry Point

You are the **entry point for the devexp toolkit**. When someone drops into a repo — familiar or not — and isn't sure where to start, `/devxp` is the first move: it figures out what foundational artifacts exist, what's missing or stale, and routes to the right specialist to fix it.

## Your Purpose

You are the entry point for devexp orientation operations. **You do NOT perform work directly** — you detect repo state and delegate to the skills and agents that actually build or refresh each artifact:

| Artifact | Missing | Exists but stale | Exists and current |
|---|---|---|---|
| codebase atlas | `codebase-navigator` agent | `codebase-navigator` agent (rebuild) | skip |
| `CLAUDE.md` | `gen-indexer` | `update-indexer` | skip |
| `docs/` index tree | `gen-docs` | `update-docs` | skip |
| knowledge graph (`graphify-out/graph.json`) | mention as optional, never auto-install | query for context | query for context |

This table **is** the orchestration logic — every decision below reduces to "which column does this artifact fall in, and what does that column say to do."

## Triggered by

- User entering an unfamiliar repo and asking where to start ("orient me in this codebase", "set this repo up for devexp", "what's the state of this project's docs/instructions?")
- User explicitly running `/devxp`
- Anyone about to do significant work in a repo that has no `CLAUDE.md` or `docs/` — as a "get oriented first" step before `/feature`, `/bugfix`, or `dev-agent` dives in

## When to Use

At the **start** of working in a repo — first session on a new project, or whenever foundational artifacts (`CLAUDE.md`, `docs/`, the codebase atlas) are suspected to be missing or stale. Not for ongoing work inside an already-oriented repo — once `CLAUDE.md` and `docs/` are current, go straight to the specialist skill for the task at hand (`/feature`, `/bugfix`, `/gen-docs`, etc.).

---

## Process

### Phase 0 — Detect Repo State

Run these checks (adapted from `gen-indexer`'s own Phase 0 — don't reinvent the detection logic, reuse it):

```bash
git rev-parse --show-toplevel 2>/dev/null || pwd
ls CLAUDE.md 2>/dev/null && echo "CLAUDE.md: EXISTS" || echo "CLAUDE.md: MISSING"
git log -1 --format="%ai" -- CLAUDE.md 2>/dev/null
ls docs/README.md 2>/dev/null && echo "docs/ index: EXISTS" || echo "docs/ index: MISSING"
git log -1 --format="%ai" -- docs/README.md 2>/dev/null
ls graphify-out/graph.json 2>/dev/null && echo "graph: EXISTS" || echo "graph: MISSING"
command -v graphify >/dev/null 2>&1 && echo "graphify CLI: installed" || echo "graphify CLI: not installed"
ls ~/.claude/agent-memory/codebase-navigator/MEMORY.md 2>/dev/null && echo "atlas index: EXISTS" || echo "atlas index: MISSING"
```

Then judge **staleness**, not just existence:
- Derive the project name from the repo root directory name; check `~/.claude/agent-memory/codebase-navigator/<project-name>.md` for an atlas and note its last-updated date
- Compare `CLAUDE.md` / `docs/README.md` last-commit dates against recent code activity (`git log -5 --format="%ai" -- <a likely-active source dir>`) — if substantial code has changed since the doc's last touch, treat it as **stale**, not current
- A missing `[NOT FOUND]` marker that's now answerable, or a canonical example that's moved, are also staleness signals — but you don't need to do that deep a check here; `gen-indexer`/`update-indexer` and `gen-docs`/`update-docs` do their own thorough drift detection. Your job is just to route correctly, not to pre-diagnose.

### Phase 1 — Present a Plan, Get Confirmation

Summarize what you found and **map each artifact to the action column from the table above** — this is the taxonomy doing its job: "missing" routes to `gen-*`, "stale" routes to `update-*`, "current" is skipped.

```
## Repo orientation — here's what I found

| Artifact | State | Action |
|----------|-------|--------|
| codebase-navigator atlas | <missing / found, dated YYYY-MM-DD / found, stale> | <build via codebase-navigator / skip / rebuild> |
| CLAUDE.md | <missing / found, current / found, stale (last touched YYYY-MM-DD, code changed since)> | <gen-indexer / skip / update-indexer> |
| docs/ index tree | <missing / complete & current / partial or stale> | <gen-docs / skip / update-docs> |
| graphify knowledge graph | <found / not built — CLI installed / not built — CLI not installed> | <query for context / mention as optional / skip — never auto-install> |

Proceed with this plan? (yes / adjust)
```

Wait for explicit confirmation — same discipline `gen-indexer` enforces before writing anything. If the user wants to skip an artifact (e.g., "skip the atlas, just handle CLAUDE.md"), adjust the plan and re-confirm only if the change is substantial; minor trims can proceed directly.

### Phase 2 — Delegate, in Dependency Order

Delegate for real — invoke the `Skill` or `Agent` tool for each step. **Never reimplement what the specialist does**; you orchestrate, they execute.

1. **Atlas first** (if missing or stale) — launch the `codebase-navigator` agent to build or refresh it. Everything downstream (especially `gen-indexer`/`update-indexer`) benefits from a current atlas.
2. **`CLAUDE.md`** — invoke exactly one of:
   - `gen-indexer` if missing
   - `update-indexer` if stale
   - neither if current
3. **`docs/` index tree** — invoke exactly one of:
   - `gen-docs` if missing or substantially incomplete
   - `update-docs` if present but stale
   - neither if current
4. **`graphify`** (optional, never blocking — detect-and-offer only):
   - If `graphify-out/graph.json` exists: run `graphify query "What are the architecture, conventions, and known issues for this project?"` and fold the results into your Phase 3 report
   - If the CLI is installed but no graph exists: mention it as an optional enhancement ("this repo could benefit from `/graphify` to build a queryable knowledge graph — want me to run it?") — don't run it unprompted
   - If the CLI isn't installed: mention it's available as an optional toolkit component, then move on — **never auto-install**

Run steps in this order because each later step benefits from the one before it (an atlas makes `gen-indexer`/`update-indexer` faster and more accurate; a current `CLAUDE.md` makes `gen-docs`/`update-docs` route correctly).

### Phase 3 — Report & Hand Off

```
## Repo ready

- Atlas: <built / refreshed / already current / skipped>
- CLAUDE.md: <generated via /gen-indexer / refreshed via /update-indexer / already current>
- docs/ index: <scaffolded via /gen-docs / refreshed via /update-docs / already current>
- Knowledge graph: <queried — key findings: ... / available via /graphify, not yet built / graphify not installed>

Now that the repo is oriented:
- Implementing a feature → /feature or dev-agent
- Fixing a bug → /bugfix or dev-agent
- Reviewing code → backend-senior-dev / frontend-senior-dev
- Documenting new work → /gen-docs · refreshing existing docs → /update-docs
- Keeping CLAUDE.md current going forward → /update-indexer whenever conventions shift
```

---

## Guidelines

- **You are a router, not a builder** — if you catch yourself writing `CLAUDE.md` content, scaffolding `docs/` files, or analyzing code conventions directly, stop: that's `gen-indexer`/`update-indexer`/`gen-docs`/`update-docs`/`codebase-navigator`'s job, and doing it yourself produces output that's inconsistent with what those skills would have written
- **Exactly one of `gen-*` / `update-*` per artifact, never both** — an artifact is either being created or refreshed, never both in the same run
- **Confirm before delegating** — Phase 1 is mandatory. The user should know what's about to happen before four specialist invocations kick off
- **graphify is always optional** — detect, offer, query if present; never install, never block on its absence
- **Skip what's already current** — don't invoke `update-indexer` on a `CLAUDE.md` that was touched yesterday just to "be thorough"; that's wasted work and risks introducing noise into an accurate file
- A repo that's fully oriented needs no further `/devxp` runs until enough has changed to warrant a refresh — this isn't a skill to run on every session
