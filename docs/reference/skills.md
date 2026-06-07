# Skills Reference

## How Skills Work

Skills live as Markdown files at `~/.claude/skills/<name>/SKILL.md`. Each skill is invoked using a slash command: `/<name>` in Claude Code.

When a skill is invoked, its `SKILL.md` content is injected into the conversation context, shaping Claude's behavior for that task.

### File Format

```markdown
---
name: my-skill
description: One-line description of what this skill does
---

# Skill Title

Body: instructions, process steps, output format...
```

---

## Skill Catalog

| Skill | Slash Command | Description |
|-------|---------------|-------------|
| bugfix | `/bugfix` | Root cause analysis and bug fixing with built-in verification |
| commit | `/commit` | Craft a conventional commit message and create the commit |
| explain | `/explain` | Explain code to a specific audience (junior, new-hire, non-technical) |
| docs | `/docs` | Documentation generation (API docs, guides, README, code comments) |
| api-design | `/api-design` | Designs API contracts, endpoints, schemas, and error handling |
| db-design | `/db-design` | Designs database schemas, migrations, and indexes |
| feature | `/feature` | Spec-driven feature implementation with tests and documentation |
| logic-review | `/logic-review` | Reviews code logic for bugs, edge cases, and dysfunction |
| migrate | `/migrate` | Step-by-step migration guide for a library or framework upgrade |
| pr | `/pr` | Generate a PR/MR description and optionally open it via gh or glab |
| quality | `/quality` | Reviews code quality, style, complexity, and maintainability |
| refactor | `/refactor` | Code refactoring for improved structure and maintainability |
| regression | `/regression` | Ensures fixes don't introduce regressions |
| standup | `/standup` | Generate a daily standup update from recent git activity |
| test-gen | `/test-gen` | Generate tests for the current file or function |
| adr | `/adr` | Write an Architecture Decision Record saved to docs/architecture/adr/ |
| changelog | `/changelog` | Generate a changelog entry from git history using conventional commits |
| release | `/release` | Full release workflow: version bump, changelog, tag, and platform release |
| postmortem | `/postmortem` | Generate a structured blameless postmortem document |
| ticket | `/ticket` | Create a well-structured ticket — detects GitHub Issues, GitLab Issues, Linear, and Jira |
| scope | `/scope` | Break a large feature or epic into atomic tickets with dependencies |
| health | `/health` | Generate a codebase health scorecard with RAG status per dimension |
| gen-claude-md | `/gen-claude-md` | Crawl a project's docs and codebase to generate a directive CLAUDE.md |
| review-pr | `/review-pr` | Surgical pre-merge code review using RISEN framework |
| groom | `/groom` | Pre-code grooming — fetches a ticket, validates against codebase, produces execution plan |
| rfc | `/rfc` | Draft a Request for Comments document for a proposed change |
| convention-audit | `/convention-audit` | Audit codebase for pattern divergence — finds all ways the same problem is solved |
| dead-code | `/dead-code` | Find unused exports, unreachable branches, zombie flags, orphaned files |
| estimation | `/estimation` | Evidence-based story point estimation from files to change, test coverage, risk |
| retrospective | `/retrospective` | Facilitate a blameless sprint retrospective — Start/Stop/Continue findings |
| git-archaeology | `/git-archaeology` | Reconstruct intent and decision history from messy git history |
| stale-work | `/stale-work` | Find orphaned branches, stale PRs, half-finished features, zombie flags |
| graphify | `/graphify` | Turn a codebase (or any input) into a persistent knowledge graph — query/path/explain tools, interactive HTML, GraphRAG-ready JSON. Pairs with the optional `graphify-read-guard`/`graphify-session-sentinel` [hooks](hooks.md) |

---

## Adding a New Skill

1. Create `skills/<skill-name>/` directory
2. Copy `templates/skill-template.md` to `skills/<skill-name>/SKILL.md`
3. Fill in frontmatter and write the skill body
4. Add a "Triggered by" section listing agents or skills that invoke it
5. Run `./install.sh` to deploy to `~/.claude/skills/`
6. The skill is immediately available as `/<skill-name>` in Claude Code

Full guide: [`docs/development/skill-authoring-guide.md`](../development/skill-authoring-guide.md)
