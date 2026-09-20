# Development Workflows

> Kit doc · Last verified: 2026-09-16 against commit `a86d2c3f6a41a6d033d31afd858ff723d5267dd7`

Step-by-step recipes with real paths. Conventions: [conventions](../development/conventions.md) · Tests: [testing](../development/testing.md) · Structure: [overview](../architecture/overview.md) · Commands: [setup](../development/setup.md)

## Ground Rules (every change)

1. **Edit the source in this repo, never the installed copies.** `~/.claude/agents/`, `~/.claude/skills/` and `~/.config/opencode/` get overwritten on the next install (`cli/internal/agents/installer.go`, `cli/internal/skills/installer.go`).
2. **Re-run `./install.sh` to deploy** asset changes. It's idempotent: already-registered hooks and MCPs are skipped (`cli/internal/hooks/installer.go`, `cli/internal/mcp/claude.go`). Agents and skills you've removed or disabled are cleaned up through the manifest (`cli/cmd/install_claude.go`). A hook registration is pruned only when its script no longer exists or it came from another install root (`pruneStaleHooks`, `pruneForeignDevexpHooks`). Disabling a hook doesn't remove a registration that already exists. Agents and skills are copies, so they need a re-install. A hook script that's already registered runs from its absolute path in the repo, so edits to it take effect right away. Registry changes (event, matcher, new hook) still need a re-install. See [overview → runtime](../architecture/overview.md#a-tool-call-at-runtime-after-install).
3. **Preview first** with `./install.sh --dry-run`. Passing any flag skips the wizard (`cli/cmd/install.go`, `flagsProvided`).
4. **Add a `CHANGELOG.md` entry under `[Unreleased]`** (`### Added` / `### Changed` / `### Fixed` / `### Removed`) in the same commit. Recent PRs all do this (`f06b67e`, `2afc3af`, `06c45a1`, `bdf026f`). Commit and branch naming are covered in [conventions](../development/conventions.md#commits--branches).
5. **Reach `main` through a pull request with green CI.** `main` is protected: a direct push is rejected, and `test`, `hooks` and `govulncheck` must pass on a branch that is up to date with `main` before merge. The settings and how to restore them are in [Branch Protection](#branch-protection-main).

## Branch Protection (`main`)

`main` is protected so that a red CI run stops a merge. Before this was in
place every gate in the repo was advisory: CI ran three blocking-by-design jobs
and nothing consumed the result, so `main` could sit broken until someone cut a
tag and `release.yml`'s `needs: ci` finally noticed.

| Setting | Value | Why |
|---|---|---|
| Pull request before merge | required | a direct push to `main` bypasses every check below |
| Required checks | `test`, `hooks`, `govulncheck` | the three jobs in `.github/workflows/ci.yml`; add `lint` when it lands |
| Strict (up to date with `main`) | on | a check that passed against a stale base proves nothing about the merge result |
| Required approving reviews | **0** | a PR is required, an approval is not — with one maintainer, requiring a review would deadlock the repo. Self-merge after green is the intended flow |
| Include administrators | **on** | a bypass for the only maintainer is not a gate. To unwedge a genuinely stuck check, toggle protection off deliberately rather than merging around it |
| Force push | blocked | [release](release.md) states rollback is "revert on `main`... history is never rewritten on `main`" — this makes that a fact rather than a promise |
| Branch deletion | blocked | same reason |

### Verify

```bash
gh api repos/:owner/:repo/branches/main/protection \
  -q '{pr_required: (.required_pull_request_reviews != null),
       approvals: .required_pull_request_reviews.required_approving_review_count,
       checks: .required_status_checks.contexts,
       strict: .required_status_checks.strict,
       enforce_admins: .enforce_admins.enabled,
       force_push: .allow_force_pushes.enabled,
       deletion: .allow_deletions.enabled}'
```

Expected:

```json
{"approvals":0,"checks":["test","hooks","govulncheck"],"deletion":false,"enforce_admins":true,"force_push":false,"pr_required":true,"strict":true}
```

A bare `gh api repos/:owner/:repo/branches/main/protection` returning
`{"message":"Branch not protected","status":"404"}` means the settings are gone
— restore them below.

### Restore (fresh fork, or re-created repo)

Needs repo-admin rights.

```bash
gh api -X PUT repos/:owner/:repo/branches/main/protection --input - <<'JSON'
{
  "required_status_checks": { "strict": true, "contexts": ["test", "hooks", "govulncheck"] },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "required_approving_review_count": 0,
    "dismiss_stale_reviews": false,
    "require_code_owner_reviews": false
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_linear_history": false,
  "required_conversation_resolution": false,
  "block_creations": false,
  "lock_branch": false,
  "allow_fork_syncing": false
}
JSON
```

`restrictions` is required by the API and must be `null` on a repo without
push-restriction lists. Adding a required check whose name never appears —
a typo, or a job renamed in `ci.yml` — blocks every merge permanently, because
a check that never reports can never pass. The names above are check-run names;
confirm them against a real run before changing them:

```bash
gh run list --workflow=ci.yml --limit 1 --json databaseId -q '.[0].databaseId' \
  | xargs -I{} gh api repos/:owner/:repo/actions/runs/{}/jobs -q '.jobs[].name'
```


## Add a Feature

Pick the recipe that matches what you're adding. Each one lists only the ordered steps; the linked guide covers how to write the asset.

### Add an agent

1. Copy `templates/agent-template.md` to `agents/<agent-name>.md`. The filename must match the `name:` frontmatter. Follow the [agent-authoring-guide](../development/agent-authoring-guide.md) and the [agent-architecture-reference](../development/agent-architecture-reference.md#agent-file-checklist) checklist (Phase 0, `## Chaining`, memory convention).
2. Write it in Claude Code format only. The opencode version is generated at install time (`transformForOpencode` in `cli/internal/agents/installer.go`). If it's opencode-only, put it in `agents/opencode/` instead.
3. If a skill or another agent should use it, have them **read** `~/.claude/agents/<agent-name>.md`. Never pass the name as a `subagent_type` ([conventions](../development/conventions.md#module-structure)).
4. Add a catalog row to `docs/reference/agents.md`, and update the agent count wherever it appears: `CLAUDE.md` ("Components"), `README.md` (the "34 agents" text and the repo tree), `agents/README.md`, `docs/README.md` (Reference table) and `docs/coverage.md`.
5. Run `./install.sh`, restart Claude Code, then trigger the agent with a real task ("Testing Your Agent" in the authoring guide).

### Add a skill

New slash commands should be rare. First check whether the capability fits inside an existing orchestrator or works as an agent (`skills/README.md`, "Adding a New Skill").

1. Create `skills/<skill-name>/SKILL.md` from `templates/skill-template.md`. The directory must match the `name:` frontmatter. Follow the [skill-authoring-guide](../development/skill-authoring-guide.md).
2. Put everything the skill needs to work in `SKILL.md`. opencode receives only that file, flattened to `commands/<name>.md`, while `references/` reaches Claude Code only (`cli/internal/skills/installer.go`, `docs/reference/skills.md`).
3. Update every place that lists or counts the commands. Between them, the last two skill additions touched `CLAUDE.md`, `README.md` (intro, orchestrator text, repo tree), `skills/README.md` (table and flow diagram), `docs/README.md`, `docs/reference/skills.md`, `docs/coverage.md` and `docs/guides/quickstart.md` (`2ae65db` /release, `27592b9` /cleanup).
4. Run `./install.sh` and invoke `/<skill-name>` in a new session ("Testing Your Skill" in the authoring guide).

### Add a hook

Full guide: [hook-authoring-guide](../development/hook-authoring-guide.md#deployment-checklist). Every hook has three touch points, and **the `opencode` and `kimi` mappings in step 3 are the ones people miss**.

Steps 3, 4 and 5 are now asserted by `cli/internal/repocheck/` — a registry entry naming a file that does not exist, a hook on disk with no registry entry, a `kimi` block switched off with no reason, a hook no test mentions, and a count in prose that no longer matches disk each fail `go test`. The prose below still explains *why* each step exists; the test is what notices when one is skipped.

1. `hooks/claude-code/<hook-name>.sh`: start with the `# devexp hook:` / `# Event: … | Matcher: …` header and `set -euo pipefail`, then `chmod +x`. A security guard must fail closed (`exit 2`) if it can't parse its input, and an advisory hook fails open but prints an internal error ([conventions](../development/conventions.md#error-handling)).
2. `hooks/opencode/<hook-name>.js`: export `async function <hookName>(_ctx)` that returns the event handlers, using `hooks/opencode/utils.js` for shared helpers.
3. Add an entry to `hooks/registry.json` with `name`, `description`, `claude_code{event,matcher,script}`, **`opencode{event,module,export,fail_closed?,enabled?}`** and `enabled`. `module` is `hooks/opencode/<hook-name>.js`, `export` is `<hookName>`, and security guards set `fail_closed: true`. Without the `opencode` mapping, opencode silently lacks the hook: `hooks/opencode/devexp-plugin.js` composes only the modules the installed selection lists, so it is never edited per hook. The Go installer parses every target block into `hooks.Hook.Targets`; the Claude Code install uses `claude_code` and the top-level `enabled` (`cli/internal/hooks/installer.go`). The opencode install copies the modules the registry maps into `~/.config/opencode/plugins/devexp/` and lists them in `devexp/hooks.json` (`cli/internal/hooks/opencode.go`).

   Decide the **`kimi`** block in the same commit: either `kimi{event,matcher,script,fail_closed?,timeout}` with an *anchored* matcher and a timeout above the scan budget, or `kimi{enabled:false,reason}` saying what Kimi does that makes the hook meaningless there — a `PostToolUse` result Kimi never reads, an `ask` Kimi runs as an allow. The Kimi install copies the adapter, the mapped guards and `scan-budget.sh` into `$KIMI_CODE_HOME/hooks/` and registers the commands in `config.toml` (`cli/internal/hooks/kimi_install.go`). A hook with no `kimi` block is simply absent there and says nothing; one with a reason says it on the run that would have installed it.
4. Write tests. Add a mirrored `<hook-name>.test.sh` and `<hook-name>.test.js` for the decision logic (pattern: `hooks/claude-code/secret-guard.test.sh` ↔ `hooks/opencode/secret-guard.test.js`). If the script extracts input with python3, add a `check <hook-name> 2 guard` or `check <hook-name> 0 advisory` line to `hooks/claude-code/fail-closed.test.sh`. If Kimi installs it, add a case to `hooks/kimi/runner.test.sh`, which drives the registered command the way Kimi's own runner does. CI runs every `*.test.sh` / `*.test.js` automatically (`.github/workflows/ci.yml`). How to write them: [testing](../development/testing.md).
5. Update the hook catalog and counts: `docs/reference/hooks.md` (catalog table and file tree), `hooks/README.md`, `README.md` and `CLAUDE.md`. The numbers are no longer written here, because a checklist that states a count goes stale the same way the prose it points at does — `cli/internal/repocheck/counts_test.go` derives them from disk and names every sentence to edit when one moves.
6. Run `./install.sh`, then check that the hook appears in `~/.claude/settings.json` (Claude Code), in `~/.config/opencode/plugins/devexp/hooks.json` (opencode) or in the `# devexp:hooks:begin` block of `$KIMI_CODE_HOME/config.toml` with its script under `$KIMI_CODE_HOME/hooks/` (Kimi), and exercise both the block and allow paths.

### Add an MCP server

Full guide: [mcp-guide](../development/mcp-guide.md#adding-a-new-mcp-to-the-framework).

1. Add an object to `mcps/registry.json`. The fields are defined by `mcp.MCP` in `cli/internal/mcp/types.go`.
2. For a secret or local path, put `${VAR}` in `args`/`headers`, list the variable in `required_env`, explain it in `setup_instructions`, and add a commented `VAR=` line to `mcps/.env.example`. Precedent: `UI_INSPECTOR_DIR` in `cb5f95b`.
3. Add a row to the MCP tables in `mcps/README.md` and `docs/reference/mcps.md`, and update the list in the `README.md` repo tree ("context7, ui-inspector").
4. Run `./install.sh --mcps-only`. For an MCP that's already installed, use `--reinstall-mcps`, because installed MCPs are skipped (`cli/internal/mcp/claude.go`). Verify with `claude mcp list`.

### Change the Go CLI

1. Find the layer in [overview → Layers](../architecture/overview.md#layers). Flag, wizard and target logic goes in `cli/cmd/`. File, JSON or exec work for one asset kind goes in `cli/internal/<kind>/`.
2. Put decisions in pure functions (inputs in, values out) and keep printing, prompting and exec in thin wrappers. Pass in what varies (`now`, `home`, a `removeFn`) so it can be tested. Follow `cli/cmd/targets.go` and `cli/cmd/paths.go` ([conventions](../development/conventions.md#module-structure)).
3. For a new flag: declare `flag<Name>` and register it in `init()` in `cli/cmd/install.go`. If it should skip the wizard, add it to `flagsProvided`. Carry it through `installOpts`. If the wizard needs it too, add it to `wizardResult` / `cli/cmd/wizard.go`.
4. Stage the embedded assets before any `go build`, `go test` or `go vet`. Otherwise `cli/internal/assets` doesn't compile (`cli/internal/assets/assets.go` doc comment; `scripts/stage-assets.sh`).
5. Write table tests next to the code, never touching the real `$HOME` or user cache (see [testing](../development/testing.md)). Then run the Go suite as CI does (`.github/workflows/ci.yml`; commands in [setup](../development/setup.md)).
6. **Rebuild the local binary on purpose.** `install.sh` builds only when `bin/devexp` is missing (`install.sh`, `[[ ! -x "$BIN" ]]`), so run `rm bin/devexp` and then `./install.sh --dry-run` to rebuild and preview. For a refactor, check that the dry-run output is byte-identical to a baseline taken before the change, as #97 did (`06c45a1`).

## Before You Change a Coupled Area

Some areas of this repo break things that are nowhere near the file you edited,
and the break is quiet. `/deliver` runs this as **Phase 1.7 — Blast Radius**,
invoking the `impact-analysis` agent on the paths the plan will touch *before
any code is written*. Working by hand, do the same walk yourself.

It is mandatory — not a judgement call — for a change to:

| Area here | What goes wrong quietly |
|---|---|
| `hooks/registry.json`, or a file it names | the hook exists and nothing loads it; the installer still reports success |
| `cli/internal/hooks/`, `cli/internal/mcp/`, `cli/cmd/install*.go` | the feature is built and its install path is never called |
| anything with an `opencode` or `kimi` counterpart | the twin drifts, and only one of the two is exercised |
| `hooks/claude-code/scan-budget.sh`, `hooks/opencode/utils.js` | every guard sources them; the consequence scales with callers |
| a guard, gate or check other code trusts | it keeps returning "fine" after it stops checking |

The reason those five rows are the list: every regression that prompted this
step came from one of them. A feature merged with a registry, new modules and
hundreds of lines of green tests, and its deploy was never called — *"opencode
users got no guards while the installer reported success."* A step that wrote a
config file, exited 0, and granted nothing. A guard that did not guard,
rediscovered four separate times, each by accident.

Two rules about the output, both learned the hard way:

- **An empty result is stated, not implied.** "Nothing depends on these paths,
  established by *X*" is a result. Silence is indistinguishable from never
  having looked.
- **The findings become tests.** A dependent that is identified and then not
  covered is a gap that was found, written down, and shipped anyway — which is
  worse than not looking, because the record shows someone knew.


## Fix a Bug

1. **Locate the layer.**
   - Install behavior (copy, transform, stale removal, settings.json): `cli/internal/{agents,skills,hooks,mcp,manifest}/`
   - Flags, wizard, target detection, paths, backups: `cli/cmd/`
   - Asset root resolution or embedded extraction: `cli/internal/repo/`
   - A hook blocking or allowing the wrong thing: **both** `hooks/claude-code/<name>.sh` and `hooks/opencode/<name>.js`
   - Agent or skill behavior: `agents/<name>.md` / `skills/<name>/SKILL.md`
   - Uninstall: `uninstall.sh` (its tests extract the embedded python and run the script in a temp `HOME` with a stub `devexp`, see `uninstall.test.sh`); opencode plugin removal rules: `cli/cmd/uninstall.go` → `hooks.UninstallOpencode`
2. **Reproduce with a failing test first.**
   - Go: add a case to the package's table test. `f06b67e` added `TestIsForeignDevexpHook` and `TestPruneForeignDevexpHooks` cases alongside the fix. `f41c79f` rewrote an assertion that "encoded the bug's premise" so each case could carry its own expectation.
   - Hooks: add the case to **both** mirrored suites. `f398a82` changed `secret-guard.test.sh` and `secret-guard.test.js` together, and `5e11596` did the same for `dangerous-cmd-guard`. If the hook fails open, add a case to `hooks/claude-code/fail-closed.test.sh` (`2afc3af`).
   - Agents and skills have no automated tests. Reproduce with a real task, as in the authoring guides' "Testing Your Agent/Skill" (`b7f81b9` touched only the `.md` files and `CHANGELOG.md`).
3. **Fix minimally.** Fix commits touch the source, its test and `CHANGELOG.md` (`f41c79f`, `f06b67e`). Keep refactors in their own change, as `06c45a1` did. For hooks, fix both implementations in the same commit.
4. **Remove workarounds the bug caused.** If docs or assets were written around the bug, remove that guidance in the same change (`f41c79f` removed the "keep `name:` out of SKILL.md bodies" advice).
5. **Run the affected suite**, then the full set CI runs ([testing](../development/testing.md)). Re-run `./install.sh` if an installed asset changed.

## Change the Data Model

There's no database, so there are no migrations. The "data model" is the JSON and frontmatter schemas the installer reads. Change a schema in every place that defines it:

| Schema | Go definition | Also update |
|--------|---------------|-------------|
| `hooks/registry.json` entries | `hooks.Hook` / `hooks.TargetSpec` (per-target map `Hook.Targets`, one sibling block per install target) in `cli/internal/hooks/installer.go` | Registry format in `docs/reference/hooks.md` and `docs/development/hook-authoring-guide.md` |
| MCP entries (`mcps/registry.json` and `mcps` in `devexp.config.json`) | `mcp.MCP` in `cli/internal/mcp/types.go` | `mcps.items` in `devexp.config.schema.json` (it has `additionalProperties: false`, so leaving it out rejects the new field), the field reference in `docs/development/mcp-guide.md`, `docs/reference/mcps.md` |
| `devexp.config.json` | `config.Config` + viper keys in `config.Load` (`cli/internal/config/config.go`) | `devexp.config.schema.json`, `devexp.config.json` (default value), the Fields table in `docs/guides/team-distribution.md` |
| Install manifest `~/.claude/.devexp-manifest.json` and `~/.config/opencode/.devexp-manifest.json` (its `plugins` key lists the opencode plugin files) | `manifest.Manifest` in `cli/internal/manifest/manifest.go` | Nothing else. Keep `Load` tolerant: a missing or old manifest loads as empty, an unreadable one loads as empty with an error the caller warns about, and the next install records a new baseline |
| Agent frontmatter keys | `skipFrontmatterKeys`, `claudeToOC`, `modelMap` in `cli/internal/agents/installer.go` | Frontmatter reference in `docs/development/agent-authoring-guide.md`, fixtures in `cli/internal/agents/testdata/` |
| Skill frontmatter | `stripFrontMatterName` in `cli/internal/skills/installer.go` | Frontmatter reference in `docs/development/skill-authoring-guide.md` |

Backward compatibility is handled by the loaders, not by migrations. Unknown or missing files load as defaults (`manifest.Load`, `hooks.InstallClaude` reading `settings.json`). Registry entries that have been removed are pruned on the next install (`pruneStaleHooks`, `manifest.Stale`). Follow the same approach so an upgraded binary never fails on an older user's files.

## Add Configuration

| Kind | Declare | Read | Document |
|------|---------|------|----------|
| Team setting (per fork) | `devexp.config.json` + `devexp.config.schema.json` | Add a field to `config.Config` and a `v.Get…("<key>")` call in `config.Load` (`cli/internal/config/config.go`), then pass it through `installOpts` (`cli/cmd/install.go`) into the installer as an argument | `docs/guides/team-distribution.md` (Fields, Precedence) |
| Environment variable read by the CLI | — | Only in `cli/cmd/` or `cli/internal/repo/` (today only `HOME` and `DEVEXP_DIR`); pass the value down as an argument | Environment variables table in [setup](../development/setup.md) |
| MCP secret or path | `${VAR}` + `required_env` + `setup_instructions` in `mcps/registry.json`, and a line in `mcps/.env.example` | `config.LoadDotenv` → `buildEnv` → `resolveStr` (`cli/cmd/registry.go`, `cli/internal/mcp/claude.go`) | `docs/development/mcp-guide.md`, [setup](../development/setup.md) |
| Install flag | `flag<Name>` + `init()` in `cli/cmd/install.go` | `installOpts`; add it to `flagsProvided` if setting it should skip the wizard | `docs/guides/install.md`, [setup](../development/setup.md) |

Values for `mcps/.env` go in the gitignored file, never in the registry or in commits (`.gitignore`, `mcps/.env.example`).

## Add a Dependency

- **Go module:** add or bump it in `cli/go.mod` with the Go module tooling (`go get`, `go mod tidy` in `cli/`), and commit `cli/go.mod` and `cli/go.sum` together. There's no written approval rule. In practice, bumps land in their own `fix:` commit whose body says why (drift or CVE) and confirms the upgrade had no impact (`e6f65e3` cobra/viper, `e147e67` Go toolchain `1.25.11` for stdlib CVEs). **Go toolchain:** the `toolchain` line in `cli/go.mod` sets the exact Go version for CI and release, and the minimum for local builds (an older Go switches up to it under `GOTOOLCHAIN=auto`; a newer Go, or `GOTOOLCHAIN=local`, builds with the installed Go). Both workflows install it through `go-version-file: cli/go.mod` (`.github/workflows/ci.yml`, `.github/workflows/release.yml`), so don't add a `go-version` there. setup-go ignores the `toolchain` line and falls back to the `go` directive when `GOTOOLCHAIN` is already `local` as it runs. So don't set `GOTOOLCHAIN` in workflow or job `env`, and don't add a second setup-go step to the same job, because the first step exports `GOTOOLCHAIN=local`. To pick up a stdlib fix, bump only the `toolchain` line to a supported, fixed patch and check with `./scripts/govulncheck.sh` (see [`../development/testing.md`](../development/testing.md#vulnerability-scan)). Leave the `go` directive (the minimum language version) alone unless code needs a newer one.
- **Hooks:** don't add third-party packages. Shell hooks use only the python3 standard library, JS hooks use only Node builtins, `hooks/opencode/package.json` declares no dependencies, and the installer has no npm or pip step to install any ([conventions](../development/conventions.md#style)).
- **MCP servers** aren't repo dependencies. They're fetched at runtime by the command in `mcps/registry.json` (for example `npx -y @upstash/context7-mcp`) or live in their own repo, like `ui-inspector` ([Add an MCP server](#add-an-mcp-server)).

## Ship It

See the [release guide](release.md). The release commit (`chore: release vX.Y.Z`) moves `[Unreleased]` into a versioned section, and pushing the `v*` tag triggers goreleaser (`.github/workflows/release.yml`). Your commit prefix decides whether the change appears in GitHub release notes ([conventions](../development/conventions.md#commits--branches)).
