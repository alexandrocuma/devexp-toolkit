# Quick Start

Everything you need to go from zero to shipping — plus `/monitor` to review what's deployed.

---

## 1. Install

**No clone needed:**

```bash
curl -fsSL https://raw.githubusercontent.com/alexandrocuma/devexp-toolkit/main/scripts/remote-install.sh | bash
```

Or from a clone:

```bash
./install.sh
```

The installer detects Claude Code, opencode and Kimi Code CLI automatically, and installs for every one it finds unless you pass `--target`. Restart your CLI after installation. (For Kimi Code, MCP servers, agents, skills and hooks all install, and `./uninstall.sh` removes them again — see [install.md](install.md#kimi-code-cli) for what Kimi honours less of.)

---

## 2. Orient on a repo: `/devxp`

Run this the first time you open a repo — or any time you return to one you haven't touched in a while.

```
/devxp
```

What it does:
- Reads the project's structure, stack, and conventions
- Writes the **development kit** in `docs/` — setup, conventions, testing, architecture overview, workflows (add a feature, fix a bug, change the data model) — every claim cited from the code
- Writes a `CLAUDE.md` that is only an **index** into those docs: rules, gotchas, a few commands, and "I need to… → go to docs/…" pointers
- Detects what this repo ships — a service, a web app, an iOS/Android app, a library — and writes `docs/guides/release.md`, the release guide `/release` follows. Anything it can't prove from the repo (usually rollback and store gates) is marked `[CONFIRM]` for you to fill in
- Hands off to `/refine` when you're ready to start work

Run it once per repo, and again whenever the docs drift — a re-run refreshes only what changed.

---

## 3. Turn an idea into a ticket: `/refine`

Have a feature idea, a bug report, or a vague stakeholder request? Feed it to `/refine`.

```
/refine

We need to let users export their data as CSV. The settings page should have an export button.
```

What it does:
1. Turns your input into structured user stories and acceptance criteria
2. Estimates complexity from the actual codebase (files to change, test coverage, risk)
3. Creates a well-formed ticket on your issue tracker (GitHub Issues, GitLab, Linear, or Jira — auto-detected), recording which release targets it affects
4. Grooms the new ticket against the codebase: validates its claims and writes a verified execution plan back to it

Output: a groomed ticket with an attached execution plan — ready for `/deliver`.

---

## 4. Build and ship: `/deliver`

Point `/deliver` at a ticket and it handles the rest.

```
/deliver PAY-42
```

What it does, in order:
1. Loads the groom plan (or runs grooming if it's missing)
2. Implements the changes in the ticket's own git worktree — infrastructure files included if the ticket touches them
3. Adds observability: log calls at entry/error points, SLO candidate notes
4. Fills test gaps: unit/integration tests first, then E2E scenarios if the project has a suite
5. Checks release readiness for each affected target (build numbers, deploy ordering, feature flags when rollback is flag-only)
6. Opens a PR and runs a code review
7. Hands off to `/release` for the gated release

You confirm the delivery plan once up front; after that, the only hard gate is whether to release.

---

## 5. Ship it: `/release`

`/deliver` hands off here once review passes — but `/release` also stands on its own.

```
/release PAY-42
```

What it does:
1. Preflight (read-only): branch merge state, commits since the last tag, version file, platform, the **derived** version bump, and a per-target inventory from the release guide
2. Asks for your explicit yes to **cut** — merge, changelog, version bump, tag
3. **Ships each affected target** exactly as the release guide says — a deploy, a beta upload then store submission, a package publish — with its own yes per target, the rollback plan shown first, and a separate yes for every step that reaches production
4. Retires the worktree, plan and scratch — **only once every target has shipped** (or been explicitly skipped)

Said no to the gate last week and never finished? Waiting on App Store review or halfway through a staged rollout? `/release PAY-42` picks it up where it stopped. Before this command existed, the only options were re-running `/deliver` or releasing by hand — which is how stray worktrees pile up.

---

## 6. Keep it healthy: `/improve`

Run at sprint end, after a production incident, or any time things "feel messy."

```
/improve
```

What it does:
1. Health scorecard across 8 dimensions: test coverage, security, dependencies, code quality, CI/CD, infrastructure health, observability maturity, and env var health
2. Stale work scan: orphaned branches, old PRs, zombie flags, dead code
3. Toolkit hygiene sweep: orphaned worktrees, stale plans, old groom sessions, stray scratch
4. Tech debt triage: business-prioritized list with carrying cost and fix ROI
5. Sprint retrospective: evidence-grounded Start/Stop/Continue findings

Only the scorecard always runs; the rest are opt-in. Nothing is deleted automatically — scans produce findings, and the hygiene sweep removes only the candidates you confirm.

---

## The loop

```
/devxp  ──→  /refine  ──→  /deliver  ──→  /release  ──→  /improve
  ↑                                           │             │
  └──────────────── next sprint ──────────────┴─────────────┘
                                              │
                                         /monitor   (operate: review the deployed system, anytime)
```

This is the full development cycle. The five build-and-ship commands carry most work; `/monitor` is the operate phase — run it anytime to review the health of what's deployed, independent of any code change.

---

## Examples: orchestrators handle the specialist work

You don't need to learn sub-commands. The orchestrators absorb them:

| Old way | New way |
|---------|---------|
| `/logic-review` then `/quality` then `/review-pr` | `/deliver` — Phase 5 does correctness + quality + PR review in sequence |
| `/health` | `/improve --health` |
| `/convention-audit` + `/stale-work` + `/dead-code` | `/improve` — Phase 3 runs all three |
| `/api-design` then `/adr` | `/refine "design the notifications API"` → groomed ticket → `/deliver` |
| `/instrument` | `/deliver` — Phase 3 adds log calls inline |
| `/explain <function> to junior` | `/devxp` — handles code explanation requests inline |

When you need specialist depth beyond what the orchestrators provide (security audit, architecture review, migration planning), the full agent catalog is available. The orchestrators invoke these agents automatically when the task calls for it.

---

## Going deeper

- **Agents reference** (`docs/reference/agents.md`) — all specialist agents and when they're invoked
- **Team distribution** (`docs/guides/team-distribution.md`) — configure devexp for your org via `devexp.config.json`
