# Skills Reference

## The Five Commands

The entire development lifecycle runs through five slash commands. Type `/` in Claude Code and you'll see exactly these:

| Command | When to use |
|---------|-------------|
| `/devxp` | First time on a repo — orient, ensure CLAUDE.md and docs/ exist, get routing |
| `/refine` | Turn an idea or request into a groomed, ready-to-build ticket |
| `/deliver <ticket>` | Implement, test, review, and release a ticket end-to-end |
| `/improve` | Sprint end or maintenance window — health, cleanup, debt, retro |
| `/graphify` | Build a persistent knowledge graph from this codebase |

```
/devxp  →  /refine  →  /deliver  →  /improve
  ↑                                      |
  └──────────── next sprint ─────────────┘
```

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
- Produces a verified execution plan attached to the ticket

### `/deliver <ticket>`
- Loads the groom plan (or runs grooming if missing)
- Implements changes — infrastructure files handled inline
- **Phase 3:** Adds observability (structured logs at entry/error points, SLO candidate notes)
- **Phase 4:** Fills test gaps (unit/integration via `test-gen` agent, E2E if suite exists), runs regression check, offers load test generation for new endpoints
- **Phase 5:** Correctness pass (null dereferences, error paths, race conditions — fix before review), quality pass (large functions, duplication — document for reviewer), then `pr-review` agent
- **Phase 6:** Changelog entry, version bump, git tag, platform release — **gated: requires explicit yes**

### `/improve`
- **Phase 2:** Health scorecard across 8 dimensions: test coverage, security, dependencies, code quality, CI/CD, infrastructure health, observability maturity, env var health
- **Phase 3:** Stale work (orphaned branches, old PRs, zombie flags), dead code (unused exports, orphaned files), convention audit (competing patterns)
- **Phase 4:** Tech debt triage via `tech-debt` agent — business-prioritized with ROI
- **Phase 5:** Sprint retrospective — evidence-grounded Start/Stop/Continue findings

### `/graphify`
- Builds a persistent knowledge graph from the codebase
- Queryable across sessions via `graphify query "<question>"`
- Referenced by other orchestrators (dev-agent, tech-lead) for prior bug root causes, conventions, and known debt

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

Only the 5 user-facing orchestrators are installed as skills. Everything else — the ~40 specialist capabilities — run as agents (read via `~/.claude/agents/<name>.md`) or inline within orchestrators.

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

New user-facing commands should be rare — they add to the discovery tax. Before adding a skill, consider whether the capability fits inside an existing orchestrator or as an agent.

If a new orchestrator-level command is genuinely needed:
1. Create `skills/<skill-name>/` directory
2. Copy `templates/skill-template.md` to `skills/<skill-name>/SKILL.md`
3. Fill in frontmatter and write the skill body
4. Run `./install.sh` to deploy

Full guide: [`docs/development/skill-authoring-guide.md`](../development/skill-authoring-guide.md)
