# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **`/release` — the release phase, as its own command.** Release is a named phase of the development cycle, but it existed only as `/deliver` Phase 6, which made it unreachable on its own. The practical consequence: declining the release gate left **no way to resume** — the only options were re-running `/deliver` (redoing implementation) or releasing by hand, which is the main reason worktrees accumulated and why `/cleanup` had to exist. `/release <ticket>` now closes that loop.
  - **Process:** read-only preflight → gate → merge → changelog → version bump → tag and publish → retire artifacts → report.
  - **The gate is its own decision.** Consent is never inherited from `/deliver`'s Phase 1 "proceed" — release is irreversible and touches shared systems.
  - **Version bump is derived, not guessed:** breaking → major, `feat:` → minor, else patch. Non-conventional history is reported rather than invented around.
  - **Failure preserves, success retires.** Only a completed release removes the worktree, plan, groom session and scratch. Every failure path keeps them.
  - **Tag push is the point of no return** — after a partial push, `git ls-remote --tags origin` is checked before any retry.
  - Fixes a **dangling reference**: `agents/changelog.md` already chained to "invoke `/release` skill", which did not exist.
- **`/deliver` Phase 6 now delegates to `/release`** and reports one of three outcomes (released / deferred / failed). Former Phase 7 (retire artifacts) moved into `/release`, where it belongs — it was always gated on release success. Phase 8 became Phase 7. If `/release` is not installed, delivery reports it and stops rather than improvising a half-release.
- Documentation surface: **seven commands → eight** — six lifecycle orchestrators and two utilities.
- **The `postmortem` agent is reachable again.** It was a complete agent that no command ever invoked — `/improve` Phase 5 ran its own retro instead, and `/monitor` had no incident path at all. Both now hand off to it: `/monitor`'s report suggests it when an anomaly traces to an incident, and `/improve`'s retro suggests it before the timeline fades.
- **The planification phase is named.** The cycle includes it, but no document did. `/refine`'s verified execution plan *is* that phase; the skills reference now says so — and says why it has no command of its own: a plan with no ticket to attach to is just a document.

### Changed

- **`ui-inspector` now ships as its own repo** — [mcp-ui-inspector](https://github.com/alexandrocuma/mcp-ui-inspector). It was vendored here as a Node project with its own `setup.sh` and a committed `dist/`, which quietly turned a distribution repo into a monorepo. The registry locates it via `UI_INSPECTOR_DIR`, documented in the MCP env template, reusing the installer's existing `[REQUIRED]` warning and `setup_instructions` path — **no Go changes were needed**, because the CLI never special-cased it (it generically injects `DEVEXP_DIR` and renders `setup_instructions`). The extracted repo gitignores `dist/` instead of committing it; the vendored copy had gone stale, with `src/tools/interact.ts` having no built counterpart.

### Removed

- **`start-services.sh`.** It had become a documented no-op, printing "No background services required" — `ui-inspector` was the only service it ever managed, and that launches its own Chromium on demand and shuts it down on SIGTERM. Removed with its references in `README.md`, `CLAUDE.md`, `docs/README.md`, `docs/guides/README.md` and `docs/guides/install.md`.
- **`/promo-campaign` and the domain-playbook category.** Marketing is not a phase of the development lifecycle. This toolkit covers idea → refinement → grooming → planification → delivery → release → cleanup → improvements/postmortem; promoting an app that already ships sits outside that loop. The skill was also the repo's largest single component (~2,700 lines), the only one requiring ffmpeg, ImageMagick, Maestro and Xcode, and iOS-Simulator-only — so it did not work on the majority of its own author's projects. It moves to **`marketing-toolkit`**, a sibling project, where it gains Android capture, an audio path and a platform-plural contract.
  - **The category goes with it.** It had exactly one member, and its four-part entry test (app-agnostic, user-invoked, interactive, not absorbable by an orchestrator) admitted a capability that the lifecycle test rejects.
  - **Documentation surface: eight commands → seven** — five lifecycle orchestrators and two utilities (`/graphify`, `/cleanup`) — across `README.md`, `skills/README.md`, `docs/reference/skills.md`, `docs/coverage.md`, `docs/README.md` and `CLAUDE.md`.
  - **Corrected a stale count** found while rewriting those lines: the skills reference claimed "~40 specialist capabilities" where every other document says ~30.
  - **Installed copies retire themselves** — `manifest.Stale()` removes vanished skills on the next `install.sh`.

## [0.6.0] - 2026-09-14

Epic #77 — promo-campaign: the toolkit's first domain playbook, with its iOS Simulator capture and segment reel composer (#76). Android capture and the carousel composer (#78) follow.

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
- **promo-campaign iOS capture and segment reel composer.** Bash scripts bundled in `skills/promo-campaign/scripts/`. They target `/bin/bash` 3.2, run as `bash <path>` because skill files install 0644, and work in Claude Code only (opencode installs SKILL.md alone). (#76)
  - **Contract:** defines the `seed` and `capture` blocks: `seed.backend` + `entries[]`; `capture.ios`; `takes[].steps[]` (Maestro flows interleaved with `hold_s` and `appearance`); `stills[]`; `cut[]` segments; `compose` styling. SKILL.md has a compact key table; `references/capture-and-compose.md` has the full schema, cue sheets, traps and the mutation record.
  - **`lib/contract.sh`:** converts front matter with ruby, then python3 + PyYAML, then yq v4. Each path applies SKILL.md's parser-proof lint first, naming the key path and line. A jq pass then type-checks every key the scripts read.
  - **`capture-ios.sh`:** one booted Simulator, pinned by UDID for every simctl and Maestro call. Each take: fresh install → `rn-asyncstorage` seed → demo status bar → starting appearance → `recordVideo` (h264), with the steps driven only after `Recording started`. Writes a cue sheet from Maestro's `commands.json`, a lossless still pass with `simctl io screenshot`, and a restore-on-exit trap.
  - **`seed-rn-asyncstorage-ios.sh`:** writes AsyncStorage's `manifest.json` before the first launch, spilling any value longer than 1024 UTF-16 units to a file named the MD5 of its key.
  - **`compose-reel.sh`:** a 1.0x reel from `capture.cut[]`: `trim` + `setpts=PTS-STARTPTS`, `concat` or `xfade`. Each take is padded with `tpad` to its logged length, and `settb=AVTB,fps=30` is applied before every join. Adds the phone window derived from the source aspect, PNG32 frame and caption layers with `shortest=1`, and a `-respect-parentheses` end card. Output: 1080x1920 H.264.
  - **Self-checks that fail the run:** frame alpha, caption band, out-points within the logged take, no re-timing, segments − fades = beats, render = plan within one frame and ≤ `max_duration_s`, size.
  - **Proven by mutation:** guards for frame alpha, `shortest=1`, take `tpad`, per-join normalisation, Σ segments ≠ Σ beats, PTS scaling, out-of-take out-points and caption band all go red. Single-segment and 1080x2220-source controls stay green.
  - **SKILL.md Phase 7** names the entry points, passes absolute paths, and reports each missing component separately: Android capture and the carousel composer are #78. Step 1 runs `contract.sh validate … capture`, and `all` runs once the cue sheets exist.
  - **Review fixes (#82):**
    - A caption ending after the end-card start, starting below 0, or empty is refused.
    - `--launch-day` writes `reel-<locale>.launch-day.mp4` and never replaces the postable reel.
    - Capture:
      - refuses an already-installed app unless `--replace-installed`
      - saves, clears and restores the host clipboard (text only; `--keep-clipboard` skips it) and empties the Simulator pasteboard
      - removes a take's previous files before recording, so a failed recapture cannot leave a stale take
      - keeps a cue when Maestro gives no timestamp
      - discards an unfinalised recording with a reason
    - The parser lint, in all three script paths and SKILL.md's check, refuses anchors, aliases and duplicate keys.
    - Stricter formats for locales, `outputs.dir`, `bundle_id` and `PROMO_RENDER_TIMEOUT_S`.

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
