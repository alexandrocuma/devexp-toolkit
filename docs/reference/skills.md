# Skills Reference

## The Eight Commands

The toolkit runs through eight slash commands — six lifecycle orchestrators and two utilities (`/graphify`, `/cleanup`). Type `/` in Claude Code and you'll see exactly these:

| Command | When to use |
|---------|-------------|
| `/devxp` | First time on a repo — orient, ensure CLAUDE.md and docs/ exist, get routing |
| `/refine` | Turn an idea or request into a groomed, ready-to-build ticket |
| `/deliver <ticket>` | Implement, test, review, and release a ticket end-to-end |
| `/release [<ticket>]` | Release phase — gated merge, changelog, version bump, tag, platform release; also finishes a deferred release |
| `/improve` | Sprint end or maintenance window — health, cleanup, debt, retro |
| `/monitor [<surface>]` | Operate phase — review the deployed system's health (telemetry/config), scored, anytime |
| `/graphify` | Build a persistent knowledge graph from this codebase |
| `/cleanup [<ticket>]` | On demand — retire finished/abandoned worktrees, orphaned branches, stale plans, groom sessions, and /tmp scratch |

```
/devxp  →  /refine  →  /deliver  →  /release  →  /improve
  ↑                                     │            │
  └──────────── next sprint ────────────┴────────────┘
                                        │
                                    /monitor   (operate: review the deployed system, anytime)
```

The first four orchestrators *build* software and `/release` *ships* it; `/monitor` *operates* what's shipped — a change-independent health read that does not diff commits.

---

## What Each Orchestrator Does

### `/devxp`
- Reads project structure, stack, and conventions
- Ensures CLAUDE.md exists (runs `gen-indexer` agent if missing, `update-indexer` if stale)
- Ensures `docs/` is scaffolded (runs `gen-docs` / `update-docs` agents)
- Handles inline: code explanation ("explain X to a junior"), git archaeology ("why does X exist"), routing recommendations ("what should I use for Y?")

### `/refine`
- Structures vague ideas into user stories + acceptance criteria
- Estimates complexity from the actual codebase
- Creates a structured ticket on GitHub Issues, GitLab, Linear, or Jira (auto-detected)
- Validates ticket claims against the codebase (via `grooming-agent`)
- **Planification** — produces a verified execution plan attached to the ticket. This is the cycle's planning phase; it has no command of its own because a plan with no ticket to attach to is just a document

### `/deliver <ticket>`
- Loads the groom plan (or runs grooming if missing)
- Implements changes — infrastructure files handled inline
- **Phase 3:** Adds observability (structured logs at entry/error points, SLO candidate notes)
- **Phase 4:** Fills test gaps (unit/integration via `test-gen` agent, E2E if suite exists), runs regression check, offers load test generation for new endpoints
- **Phase 5:** Correctness pass (null dereferences, error paths, race conditions — fix before review), quality pass (large functions, duplication — document for reviewer), then `pr-review` agent
- **Phase 6:** Hands off to `/release`, which runs its own gate. Delivery never releases on the "yes" given at Phase 1

### `/release [<ticket>]`
- **The release phase, as its own command.** Merge → changelog → version bump → tag → platform release → retire this delivery's artifacts
- **Gated:** asks for its own explicit confirmation. Consent is never inherited from `/deliver`'s Phase 1 "proceed"
- **Finishes a deferred release.** Previously, declining `/deliver`'s gate left no way to resume without re-running delivery or releasing by hand — the main source of accumulated worktrees. `/release <ticket>` now closes that loop
- **Preflight is read-only** and reports branch merge state, commits since the last tag, the detected version file and platform, and the *derived* version bump (breaking → major, feat → minor, else patch)
- **Failure preserves, success retires** — only a completed release removes the worktree, plan, groom session and scratch; every failure path keeps them for inspection
- **Never auto-resolves a merge conflict**, and never re-pushes a tag without first checking `git ls-remote --tags origin`
- Invoked by `/deliver` Phase 6 and chained to by the `changelog` agent when a version bump is needed

### `/improve`
- **Phase 2:** Health scorecard across 8 dimensions: test coverage, security, dependencies, code quality, CI/CD, infrastructure health, observability maturity, env var health
- **Phase 3:** Stale work (orphaned branches, old PRs, zombie flags), dead code (unused exports, orphaned files), convention audit (competing patterns)
- **Phase 4:** Tech debt triage via `tech-debt` agent — business-prioritized with ROI
- **Phase 5:** Sprint retrospective — evidence-grounded Start/Stop/Continue findings

### `/monitor [<surface>]`
- **Operate phase** — assesses the *deployed* system, not the codebase; change-independent (never diffs commits), runnable anytime
- **Phase 1:** Detects deployed-system surfaces by category — cloud/infra, dashboards, logging, alerting, tracing/metrics — from repo signals and already-authenticated connectors; vendor-agnostic
- **Phase 2:** Reviews each surface live (read-only connector query) or via config-as-code fallback; labels findings `[live]`/`[config]`; cross-references observability coverage against critical paths (covered/partial/blind)
- **Phase 3:** Per-surface 🟢/🟡/🔴/N/A + an equal-weighted composite score, plus a ranked, actionable anomaly list with evidence
- **Phase 5:** Persists the scored report to `.devexp/system-health-review.md`. `/improve`'s Observability Maturity dimension defers to this artifact when present — one home for "is the system healthy?"
- `/monitor <surface>` scopes the review to a single detected surface

### `/graphify`
- Builds a persistent knowledge graph from the codebase
- Queryable across sessions via `graphify query "<question>"`
- Referenced by other orchestrators (dev-agent, tech-lead) for prior bug root causes, conventions, and known debt

### `/cleanup [<ticket>]`
- On-demand counterpart to `/improve`'s repo-wide hygiene sweep (C2): retires finished/abandoned git worktrees, orphaned merged branches, stale persisted plans, groom-session leftovers, and stray `/tmp` scratch — no memory pruning (that stays in `/improve`, where codebase-navigator's drift classification lives)
- Discovery → classification (live vs finished) → dry-run report → explicit confirmation (or `--cleanup` pre-confirmed mode with a line-by-line removal log) → scoped removal
- Safety rules are load-bearing and inlined: validated `[A-Za-z0-9_-]` ids, prefix-anchored globs, never the main checkout / default branch / shared memory, preserve on failure or doubt
- Primary sources of accumulated trees: failed deliveries, and releases deferred at `/release`'s gate whose branch has since landed

---

## How Skills Work (Technical)

Skills live as Markdown files at `~/.claude/skills/<name>/SKILL.md`. Each skill is auto-discovered by Claude Code as a `/<name>` slash command.

Frontmatter:
```markdown
---
name: my-skill
description: One-line description shown in the slash command picker
---
```

Only the 6 lifecycle orchestrators and the 2 utilities are installed as skills. Everything else — the ~30 specialist capabilities — run as agents (read via `~/.claude/agents/<name>.md`) or inline within orchestrators.

---

## Review Without `/deliver`

When you need a standalone review outside the delivery cycle:

| When you want to… | Use |
|---|---|
| Deep backend review with architecture guidance | `backend-senior-dev` agent |
| Frontend component or UI architecture review | `frontend-senior-dev` agent |
| Security audit — OWASP, auth, data exposure | `security` agent |
| PR review outside the `/deliver` flow | `pr-review` agent |
| Circular deps, unused imports, dependency graph | `dep-map` agent |
| CVE scan and dependency staleness | `dep-audit` agent |
| Prioritized tech debt register with ROI | `tech-debt` agent |

**During `/deliver`:** correctness + quality + `pr-review` run automatically in Phase 5. You don't invoke these separately.

---

## Adding a New Skill

New user-facing commands should be rare — they add to the discovery tax. Before adding a skill, consider whether the capability fits inside an existing orchestrator or as an agent. A capability outside the development lifecycle does not belong here at all.

If a new command is genuinely needed:
1. Create `skills/<skill-name>/` directory
2. Copy `templates/skill-template.md` to `skills/<skill-name>/SKILL.md`
3. Fill in frontmatter and write the skill body
4. Run `./install.sh` to deploy

Three install facts shape how a skill is written:
- **Supporting files reach Claude Code only.** `references/` and `scripts/` deploy with the whole skill directory to `~/.claude/skills/<name>/`. opencode installs SKILL.md alone, flattened to `~/.config/opencode/commands/<name>.md`, so anything load-bearing belongs in SKILL.md itself.
- **Installed files are written 0644**, so a bundled script is invoked as `bash <path>`, never `./script`.
- **`disable-model-invocation: true`** keeps a user-only skill's description out of every session's context while leaving `/<name>` invocable.

Full guide: [`docs/development/skill-authoring-guide.md`](../development/skill-authoring-guide.md)
