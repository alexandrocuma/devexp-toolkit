# DevExp Framework — CLAUDE.md

> Index refreshed by devexp `update-indexer` on 2026-09-16. Knowledge lives in `docs/`.

A curated collection of Claude Code agents, skills, hooks, and MCP servers that brings a consistent, expert-level development experience to any project. Install once, distribute to your team.

**Components**: `agents/` (34 agents) · `skills/` (8 user commands) · `hooks/` (10 hooks — 7 enabled by default, 3 opt-in `graphify-*`) · `mcps/` (MCP registry) · `cli/` (Go installer CLI `devexp`)

**Stack:** Markdown assets · bash + python3 (Claude Code hooks) · ESM JS on Node builtins (opencode hooks) · Go 1.25 CLI (cobra, viper, promptui) · **Entry point:** `install.sh` → `cli/main.go`

> New to a repo? Run `/devxp` first — it orients on the codebase, ensures `CLAUDE.md` and `docs/` exist or are current, and routes you to the right skill next.

---

## Start Here

New to this repo? Read in order: [architecture overview](docs/architecture/overview.md) → [setup](docs/development/setup.md) → [conventions](docs/development/conventions.md) → [workflows](docs/guides/workflows.md)

---

## Dev Workflow

| Task | Command |
|------|---------|
| Install (interactive wizard) | `./install.sh` |
| Dry-run (preview, non-interactive) | `./install.sh --dry-run` |
| Uninstall | `./uninstall.sh` |
| Test — Go (as CI) | `./scripts/stage-assets.sh && (cd cli && go test ./... -race -cover)` |
| Test — Claude Code hooks (as CI) | `for f in hooks/claude-code/*.test.sh; do bash "$f" \|\| exit 1; done` |
| Test — Kimi hooks (as CI) | `for f in hooks/kimi/*.test.sh; do bash "$f" \|\| exit 1; done` |
| Rebuild local CLI after Go changes | `rm bin/devexp && ./install.sh --dry-run` |

Sources: `install.sh`, `cli/cmd/install.go`, `.github/workflows/ci.yml`. Full list, opencode and installer-script suites, env vars: [setup](docs/development/setup.md#commands)

---

## Rules

- **Always** re-run `./install.sh` after editing `agents/`, `skills/` or `hooks/` — it is idempotent — see [workflows](docs/guides/workflows.md#ground-rules-every-change)
- **Never** edit deployed copies (`~/.claude/agents/`, `~/.claude/skills/`, `~/.config/opencode/`) — edit source here; the next install overwrites them — see [conventions](docs/development/conventions.md#module-structure)
- **Never** call the `Agent` tool with a custom agent name as `subagent_type` — read `agents/<name>.md` and follow it in the current context — see `agents/README.md`, [conventions](docs/development/conventions.md#module-structure)
- **Always** give a new hook an `opencode.module` + `opencode.export` (and `fail_closed` for security guards) in `hooks/registry.json` — see [workflows](docs/guides/workflows.md#add-a-hook)
- **Always** add a `CHANGELOG.md` entry under `[Unreleased]` in the same commit — see [conventions](docs/development/conventions.md#commits--branches)
- **Never** cite an issue number, PR link or URL in a code comment — write the reason inline; history goes in the commit body — see [conventions](docs/development/conventions.md#comments)
- **Never** put knowledge in `CLAUDE.md` — directives and pointers only; content goes in `docs/` — see [docs-architecture](docs/guides/docs-architecture.md)
- **Always** reach `main` through a pull request with green CI — `main` is protected; a direct push is rejected — see [workflows](docs/guides/workflows.md#branch-protection-main)
- **Before marking work done:** all five CI suites pass — see [testing](docs/development/testing.md#before-every-commit)

---

## Must-Know Gotchas

- **`go build`/`go test` fail in a fresh clone or worktree** (`pattern all:agents: no matching files found`) — staged assets under `cli/internal/assets/` are gitignored; run `./scripts/stage-assets.sh` first — see `.gitignore`, [setup](docs/development/setup.md#troubleshooting)
- **Go changes don't reach `bin/devexp`** — `install.sh` builds only when the binary is missing; `rm bin/devexp` first — see `install.sh`
- **`./install.sh` with no flags opens an interactive wizard — only when stdin is a TTY** (with no TTY it installs everything for every detected CLI instead); `--dry-run`, `--reinstall-mcps`, `--mcps-only`, `--agents-only`, `--skills-only` and `--target` take the non-interactive path, `--model` alone does not — see [setup](docs/development/setup.md#commands)
- **Two of the three graphify hooks are on for opencode** (`opencode.enabled` in the registry), all three off for Claude Code; `graphify-read-guard` is off everywhere because it can block and this repo commits the graph — turn the rest off with `hooks.disabled` — see [reference/hooks](docs/reference/hooks.md)
- **Adding or removing an agent, skill or hook leaves counts stale** in `CLAUDE.md`, `README.md`, `docs/README.md` and more — see [workflows](docs/guides/workflows.md#add-a-feature)

---

## Repo Structure

| Directory | What goes here |
|-----------|---------------|
| `agents/<name>.md` | One file per agent — frontmatter + system prompt (`agents/opencode/` for opencode-only) |
| `skills/<name>/SKILL.md` | One subdirectory per skill |
| `hooks/` | `registry.json` (source of truth) · `claude-code/<name>.sh` · `opencode/<name>.js` · `kimi/adapter.sh` |
| `mcps/registry.json` | Source of truth for all MCP servers |
| `templates/` | Starting points for new agents and skills |
| `cli/cmd/` · `cli/internal/` | Go CLI: commands + install orchestration · one package per asset kind or concern |
| `scripts/` | `stage-assets.sh` (stage embedded assets) · `remote-install.sh` · `govulncheck.sh` (vulnerability scan, CI + release) |
| `docs/` | All documentation — start at `docs/README.md` |

---

## Where Things Are

| I need to… | Go to |
|------------|-------|
| Set up, build from source, find a command or env var | [`docs/development/setup.md`](docs/development/setup.md) |
| Follow code conventions (Go CLI, cross-asset rules, commits) | [`docs/development/conventions.md`](docs/development/conventions.md) |
| Write or run tests (Go, hook, installer-script suites) | [`docs/development/testing.md`](docs/development/testing.md) |
| Understand structure, install flow, runtime hook flow, known gaps | [`docs/architecture/overview.md`](docs/architecture/overview.md) |
| Add an agent/skill/hook/MCP, change the Go CLI, fix a bug, change a schema | [`docs/guides/workflows.md`](docs/guides/workflows.md) |
| Cut a release, ship the `cli` / `toolkit-clone` targets | [`docs/guides/release.md`](docs/guides/release.md) |
| Understand why something was decided | [`docs/architecture/adr/README.md`](docs/architecture/adr/README.md) |

### Toolkit reference

Start at [`docs/README.md`](docs/README.md) for the full index.

| I need to know... | Go to |
|---|---|
| Full agent catalog + how agents work | [`docs/reference/agents.md`](docs/reference/agents.md) |
| Full skill catalog + how skills work | [`docs/reference/skills.md`](docs/reference/skills.md) |
| Hooks reference + registry format | [`docs/reference/hooks.md`](docs/reference/hooks.md) |
| MCP registry format + adding MCPs | [`docs/reference/mcps.md`](docs/reference/mcps.md) |
| Install / uninstall | [`docs/guides/install.md`](docs/guides/install.md) |
| Team distribution + devexp.config.json | [`docs/guides/team-distribution.md`](docs/guides/team-distribution.md) |
| Worktree-per-ticket delivery convention | [`docs/guides/worktree-per-ticket.md`](docs/guides/worktree-per-ticket.md) |
| On-demand cleanup of finished worktrees + delivery artifacts (`/cleanup`) | [`docs/reference/skills.md`](docs/reference/skills.md) |
| Cleanup safety rules (deletion guards) | [`docs/guides/cleanup-safety.md`](docs/guides/cleanup-safety.md) |
| CLAUDE.md-as-indexer pattern | [`docs/guides/docs-architecture.md`](docs/guides/docs-architecture.md) |
| How to write a new agent | [`docs/development/agent-authoring-guide.md`](docs/development/agent-authoring-guide.md) |
| How to write a new skill | [`docs/development/skill-authoring-guide.md`](docs/development/skill-authoring-guide.md) |
| How to write a new hook | [`docs/development/hook-authoring-guide.md`](docs/development/hook-authoring-guide.md) |
| Agent structural conventions | [`docs/development/agent-architecture-reference.md`](docs/development/agent-architecture-reference.md) |
