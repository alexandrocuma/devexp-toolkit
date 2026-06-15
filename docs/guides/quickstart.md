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

The installer detects Claude Code and opencode automatically. Restart your CLI after installation.

---

## 2. Orient on a repo: `/devxp`

Run this the first time you open a repo — or any time you return to one you haven't touched in a while.

```
/devxp
```

What it does:
- Reads the project's structure, stack, and conventions
- Ensures a `CLAUDE.md` exists (creates or refreshes it)
- Ensures a `docs/` folder exists (scaffolds it if missing)
- Hands off to `/refine` when you're ready to start work

Run it once per repo. Future sessions pick up where it left off.

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
3. Creates a well-formed ticket on your issue tracker (GitHub Issues, GitLab, Linear, or Jira — auto-detected)
4. Validates the ticket's claims against the codebase before saving

Output: a groomed ticket with an attached execution plan — ready for `/deliver`.

---

## 4. Build and ship: `/deliver`

Point `/deliver` at a ticket and it handles the rest.

```
/deliver PAY-42
```

What it does, in order:
1. Loads the groom plan (or runs grooming if it's missing)
2. Implements the changes — infrastructure files included if the ticket touches them
3. Adds observability: log calls at entry/error points, SLO candidate notes
4. Fills test gaps: unit/integration tests first, then E2E scenarios if the project has a suite
5. Opens a PR and runs a code review
6. Releases: changelog entry, version bump, git tag, GitHub/GitLab release — **gated, requires your explicit yes**

The only decision you make is whether to release. Everything else runs automatically.

---

## 5. Keep it healthy: `/improve`

Run at sprint end, after a production incident, or any time things "feel messy."

```
/improve
```

What it does:
1. Health scorecard across 8 dimensions: test coverage, security, dependencies, code quality, CI/CD, infrastructure health, observability maturity, and env var health
2. Stale work scan: orphaned branches, old PRs, zombie flags, dead code
3. Tech debt triage: business-prioritized list with carrying cost and fix ROI
4. Sprint retrospective: evidence-grounded Start/Stop/Continue findings

All diagnostic steps run inline — nothing is deleted automatically. You get findings and a checklist.

---

## The loop

```
/devxp  ──→  /refine  ──→  /deliver  ──→  /improve
  ↑                              │             │
  └─────────── next sprint ──────┴─────────────┘
                                 │
                            /monitor   (operate: review the deployed system, anytime)
```

This is the full development cycle. The four build commands carry most work; `/monitor` is the operate phase — run it anytime to review the health of what's deployed, independent of any code change.

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
