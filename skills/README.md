# Skills

Each subdirectory here contains one skill. Skills are invoked in Claude Code or opencode via slash commands: `/skill-name`.

Install them by running `../install.sh` from the repo root. Installed skills land in `~/.claude/skills/<name>/SKILL.md`.

---

## The Eight Commands

These eight commands are the entire user-facing surface — six lifecycle orchestrators and two utilities (`/graphify`, `/cleanup`). Everything else — ~30 specialist capabilities — runs as agents (`agents/<name>.md`) or inline within these orchestrators.

| Directory | Slash Command | When to use |
|-----------|---------------|------------|
| `devxp/` | `/devxp` | First time on a repo — orient, set up CLAUDE.md and docs/ |
| `refine/` | `/refine` | You have an idea or feature request to turn into a groomed ticket |
| `deliver/` | `/deliver <ticket>` | You have a groomed ticket and want to build and ship it |
| `release/` | `/release [<ticket>]` | Work is reviewed and approved — merge, changelog, version, tag, publish (gated) |
| `improve/` | `/improve` | Sprint end or maintenance window — health, cleanup, retrospective |
| `monitor/` | `/monitor [<surface>]` | Operate phase — review the deployed system's health via telemetry/config, anytime |
| `graphify/` | `/graphify` | Build a persistent, queryable knowledge graph from this codebase |
| `cleanup/` | `/cleanup [<ticket>]` | On demand — retire finished/abandoned worktrees, orphaned branches, stale plans, and scratch |

```
/devxp  →  /refine  →  /deliver  →  /release  →  /improve
  ↑                                     │            │
  └──────────── next sprint ────────────┴────────────┘
                                        │
                                    /monitor   (operate: review the deployed system, anytime)

  /graphify   (utility: knowledge graph)   /cleanup   (utility: retire finished artifacts)
```

---

## What Each Orchestrator Does

Full breakdown of each orchestrator's phases, the agents it invokes, and where former standalone skills (ADR, test-gen, retrospective, etc.) now live: [`docs/reference/skills.md`](../docs/reference/skills.md)

---

## Adding a New Skill

New user-facing commands should be rare — they add to the discovery tax. Before adding one, consider whether the capability fits inside an existing orchestrator or as an agent.

Full guide: [`docs/development/skill-authoring-guide.md`](../docs/development/skill-authoring-guide.md)
