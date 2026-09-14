# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Epic #77 — promo-campaign: the toolkit's first domain playbook. Its capture and compose tooling (#76) ships separately, and the release waits for it.

### Added

- **`/promo-campaign` — app promo playbook (1st domain playbook).** A user-invoked command, with `disable-model-invocation: true` and an `argument-hint` of `[platform] [locale]`, that plans an app's promo reel and carousel from real footage. It is the first skill in a new domain-playbook category, whose entry test is: app-agnostic, user-invoked, interactive, and not absorbable by an orchestrator. (#75)
  - **Process:** feature extraction → exactly one hook → storyboard → claim and badge check → instance file → capture/compose handoff. The user confirms at three checkpoints.
  - **Claim rule:** discovery sources (changelog, README, code) find features, but only the claim authority (the current store listing or landing page) may back a caption or slide. Unclaimed features are reported, never captioned.
  - **Badge rule:** a store badge appears only for a listing publicly installable when checked signed-out, or when marked `launch_day: true` with a do-not-post-before date.
  - **Storyboard limits, measured on the render:**
    - ≤ 20.0s (shorter wins), and the render equals the plan within one frame
    - 1.0x, never sped up
    - captions: one idea each, ≥ 2.0s each, ≤ 8 per locale
    - hook lands ≤ 5.0s
    - end card ≤ 3.0s including its transition, with no caption over it
  - **Cut order:** compress pre-hook setup → drop beats from the end → drop repeated beats.
  - **Outputs:** a 9:16 reel (1080x1920) and a 4:5 carousel (1080x1350).
  - **Instance-file contract, inline in SKILL.md** (opencode installs SKILL.md only): `docs/marketing/campaign.md` in the consumer repo, with YAML front matter `schema: promo-campaign/v1`.
    - Keys: `app`, `platforms` (optional), `locales`, `max_duration_s`, `claim_sources`, `features`, `hook`, `beats`, `captions.<locale>`, `carousel.<locale>`, `end_card`, `outputs`, and reserved `seed`/`capture` blocks whose internals #76 defines.
    - Parser-proof rules: quoted strings and locale keys, decimal-second times, `true`/`false` only.
    - A dependency-free Ruby check in SKILL.md lints those rules before loading, type-checks, then enforces every limit, the single hook beat, verbatim claims and the badge rule, reporting every error by key.
    - The skill also maintains the consumer's `docs/marketing/README.md` index and `docs/README.md` entry, and git-ignores `outputs.dir`.
  - **Degrades without #76:** when the skill's `scripts/` directory is absent, it stops after writing the instance file and says so. `references/` ships a fictional, limit-passing EXAMPLE campaign and a blank template.

### Changed

- **Documentation surface reframed from seven commands to eight** across the skills catalog, READMEs, CLAUDE.md and coverage map: five lifecycle orchestrators, two utilities (`/graphify`, `/cleanup`) and one domain playbook (`/promo-campaign`). The skills reference gains a Domain Playbooks section with the entry test, plus notes on supporting files (Claude Code only, installed 0644) and on `disable-model-invocation`. (#75)

Epic #73 — on-demand cleanup, worktree access grants, and a closed manual-release gap.

### Added

- **`/cleanup` — on-demand artifact retirement (2nd utility).** A standalone command that retires finished or abandoned delivery artifacts: merged/superseded git worktrees, orphaned branches (never the default branch), stale persisted plans, groom-session leftovers, and stray `/tmp` scratch. Discovery → live/finished classification → always-on dry-run report → explicit confirmation (or `--cleanup` pre-confirmed mode with a line-by-line removal log) → scoped removal. Follows the cleanup-safety rules inline; explicitly out of scope: agent-memory pruning (stays in `/improve` C2, which owns codebase-navigator's drift classification), the main checkout, and release/changelog work. `/cleanup <ticket>` scopes the sweep to one ticket.
- **Worktree access grants at creation.** `/deliver` Phase 1.5 (and `/improve`'s parallel cleanup streams) now grant the runtime access to each new worktree immediately after `git worktree add` — the tree lives at `../<repo>-worktrees/<ticket>`, outside the project root, so without a grant every write prompts. Claude Code: the worktrees parent directory (absolute path) is merged into the project's `.claude/settings.json` `additionalDirectories` (idempotent, existing content preserved). opencode: no path-scoped grant exists in its tool-scoped permission model, so the user is told to approve the first write with "always allow" for the worktrees directory. The worktree-per-ticket lifecycle is now create → grant → work → merge → remove.

### Fixed

- **`/deliver` no longer orphans worktrees on a declined release gate.** When the user chooses "I'll release manually," Phase 6 now checks whether the ticket branch is already merged to the base: merged → the worktree and branch are removed immediately (same commands as the success path); not merged → the tree is kept and the user is told `/cleanup <ticket>` (or plain `/cleanup`) will retire it once the branch lands. The final report's worktree line reflects the deferred-release state. (#73)

### Changed

- Documentation surface reframed from six commands to seven (five lifecycle orchestrators — `/devxp`, `/refine`, `/deliver`, `/improve`, `/monitor` — plus the `/graphify` and `/cleanup` utilities) across the skills catalog, READMEs, CLAUDE.md, and coverage map. (#73)

## [0.5.0] - 2026-06-15

Epic #54 — `/monitor`, a new operate-phase orchestrator for reviewing the health of deployed systems.

### Added

- **`/monitor` — deployed-system health review (5th orchestrator).** A new operate-phase command that assesses the *running* system rather than the codebase. It detects stack surfaces by category (cloud/infra, dashboards, logging, alerting, tracing/metrics) from repo signals and already-authenticated connectors, reviews each surface live (read-only connector query) or via a config-as-code fallback, and produces a change-independent, equal-weighted composite health score with a ranked, actionable anomaly list. Vendor-agnostic (names no platform in its prompt) and credential-safe (never triggers auth or stores secrets). `/monitor <surface>` scopes the review to a single detected surface. (#55, #56, #57, #58)
- **Persisted health review + `/improve` reconciliation.** `/monitor` persists its scored report to `.devexp/system-health-review.md`; `/improve`'s Observability Maturity dimension now defers to that artifact when present — one home for "is the system healthy?" The existing `observability` baseline key is reused for trends with no schema migration. (#59)

### Changed

- Documentation surface reframed from five commands to six (five lifecycle orchestrators — `/devxp`, `/refine`, `/deliver`, `/improve`, `/monitor` — plus the `/graphify` utility) across the skills catalog, READMEs, CLAUDE.md, and quickstart. (#60)

## [0.4.0] - 2026-06-15

Epic #32 — fresh memory, worktree-per-ticket delivery, and completion cleanup.

### Added

- **Worktree-per-ticket delivery.** `/deliver` isolates each ticket in its own git worktree (create → work → merge-at-release-gate → remove), with a single-stream fallback; each epic sub-ticket gets its own worktree so independent work runs in parallel. `/improve` isolates parallel cleanup streams the same way. New `docs/guides/worktree-per-ticket.md` convention. (#36, #37, #38)
- **Memory freshness.** `codebase-navigator`'s coarse 30-day rebuild window is replaced by a canonical Drift Classification (CURRENT/SMALL/BIG keyed on *what changed*, not *how long ago*); `dev-agent` and `grooming-agent` run a cheap freshness gate before trusting the atlas; persisted plans are stamped with the commit they were validated against, and `/deliver` re-grooms on big drift. (#33, #34, #35)
- **Completion cleanup.** `/deliver` gains a final phase that retires the ticket's artifacts (worktree, persisted plan, groom session, `/tmp` scratch) on successful completion; `/improve` gains a repo-wide hygiene sweep for orphaned artifacts. Both follow the new `docs/guides/cleanup-safety.md` deletion-safety rules (dry-run, validated id guards, prefix-anchored globs). (#39, #40)
- Allow/deny test suites for the `dangerous-cmd-guard` hook (claude-code and opencode). (#50)

### Changed

- `dangerous-cmd-guard` now blocks unanchored wildcard deletes in sensitive directories (`/tmp/*`, `~/.claude/.../*`, and quoted variants) as an execution-time backstop for the cleanup phases. (#50)
- Toolkit-internal `docs/` references in the orchestrator skills are labeled as maintainer-only, since `docs/` is not installed into a user's environment. (#51)

### Fixed

- `dangerous-cmd-guard` no longer false-positives on a force flag that appears elsewhere in a command (e.g. a short flag inside a commit message); the force-push rule now matches only an argument of the same push command. (#50)

## [0.3.0] - 2026-06-13

### Added

- `devexp install` now performs manifest-based stale-file cleanup: each run records the agent/skill files it installs in `~/.claude/.devexp-manifest.json` (and `~/.config/opencode/.devexp-manifest.json` for opencode), and removes any from a prior run that are no longer shipped in the current version.
- Skill directories are now backed up to the timestamped backup folder before being overwritten, matching the existing behavior for agents.
- Stale hook pruning: hook entries whose backing script no longer exists on disk (because the hook was removed from `hooks/registry.json`) are automatically dropped from `settings.json`; user-authored hooks are left untouched.
- "Updating" section added to `docs/guides/install.md` documenting the update path, what's overwritten vs. preserved, stale-file/hook cleanup, and the one-time pre-manifest baseline caveat.

### Changed

- Disabling an agent or skill in `devexp.config.json` now removes it from disk on the next install (previously it only skipped updates, leaving the old copy in place).

### Fixed

- CI: bumped `actions/checkout` to v6 and `actions/setup-go` to v6 (with `cache-dependency-path: cli/go.sum`) in both workflows, and `goreleaser/goreleaser-action` to v7 — resolves the Node 20 deprecation warning and go.sum cache-restore failure seen on the v0.2.0 release run.

## [0.2.0] - 2026-06-13

### Added

- Canonical **Persistent Agent Memory** section rolled out to 25 agents (8 Category B, 9 analysis, 8 workflow) — every memory-enabled agent now follows a consistent format for storing and recalling project-specific context across sessions.
- "Persistent Agent Memory" checklist item added to the Agent File Checklist so new agents adopt the canonical pattern by default.
- Agent-memory duplication mapping convention: documents `~/.claude/agent-memory/graphify-out/` and wires a manual cross-agent duplication check into `/improve` Phase 3.
- Table-driven test coverage for the `cli/` Go module (previously 0%), covering `cli/cmd/install.go`, `cli/internal/config`, and `cli/internal/repo`.

### Changed

- Go table-driven tests now use `map[string]struct{}` keyed by test name across the `cli/` test suite.
- Removed 9 Python install scripts superseded by the Go CLI (`devexp install`); `install.sh` now defers entirely to `bin/devexp`.

### Fixed

- Atlas freshness checks: `/devxp` and codebase-navigator now validate dated Gotchas/Technical Debt entries against git history before trusting them, and feature-path-tracer's memory follows the index+topic-file convention instead of duplicating the atlas. codebase-navigator self-heals old-format atlases on the next run.
- CI: stage embedded assets before running `go test` so embed-dependent tests pass.
- Bumped Go toolchain to 1.25.11 and Cobra/Viper to v1.10.2/v1.21.0 to resolve stdlib and dependency CVEs.

## [0.1.0] - 2026-06-11

Initial release.

### Added

- `devexp` Go CLI (`cli/`) — installs agents, skills, hooks, and MCPs for Claude Code and opencode. No-clone installation via `curl | bash`, which downloads a release binary and runs `devexp install`.
- 34 specialist agents covering the full SDLC — implementation (`dev-agent`), code review (`backend-senior-dev`, `frontend-senior-dev`, `pr-review`), analysis (`root-cause`, `arch-review`, `impact-analysis`, `data-flow`), quality (`security`, `performance`, `test-gen`, `test-runner`, `dep-audit`), release (`changelog`, `ci-cd`, `postmortem`), and more — plus an opencode-exclusive swarm orchestrator.
- 5 user-facing slash commands covering the full development lifecycle: `/devxp` (orient), `/refine` (groom tickets), `/deliver` (implement, test, review, release), `/improve` (health check, cleanup, retro), and `/graphify` (build a persistent knowledge graph).
- 10 safety/quality hooks — secret protection, destructive command blocking, large-file confirmation, lint/format/test-on-save, plus an optional `graphify-*` set — implemented for both Claude Code (shell scripts) and opencode (JS plugin).
- Curated MCP server registry (`context7`, `ui-inspector`) with automatic install and Docker-backed service support.
- Team distribution via `devexp.config.json` — disable agents/hooks, set a default model, and register org-internal MCPs.
- GoReleaser-based release pipeline producing darwin/linux (amd64/arm64) binaries.
