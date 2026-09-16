# Development Workflows

> Kit doc · Last verified: 2026-09-16 against commit `a86d2c3f6a41a6d033d31afd858ff723d5267dd7`

Step-by-step recipes with real paths. Conventions: [conventions](../development/conventions.md) · Tests: [testing](../development/testing.md) · Structure: [overview](../architecture/overview.md) · Commands: [setup](../development/setup.md)

## Ground Rules (every change)

1. **Edit the source in this repo, never the installed copies.** `~/.claude/agents/`, `~/.claude/skills/` and `~/.config/opencode/` get overwritten on the next install (`cli/internal/agents/installer.go`, `cli/internal/skills/installer.go`).
2. **Re-run `./install.sh` to deploy** asset changes. It's idempotent: already-registered hooks and MCPs are skipped (`cli/internal/hooks/installer.go`, `cli/internal/mcp/claude.go`). Agents and skills you've removed or disabled are cleaned up through the manifest (`cli/cmd/install_claude.go`). A hook registration is pruned only when its script no longer exists or it came from another install root (`pruneStaleHooks`, `pruneForeignDevexpHooks`). Disabling a hook doesn't remove a registration that already exists. Agents and skills are copies, so they need a re-install. A hook script that's already registered runs from its absolute path in the repo, so edits to it take effect right away. Registry changes (event, matcher, new hook) still need a re-install. See [overview → runtime](../architecture/overview.md#a-tool-call-at-runtime-after-install).
3. **Preview first** with `./install.sh --dry-run`. Passing any flag skips the wizard (`cli/cmd/install.go`, `flagsProvided`).
4. **Add a `CHANGELOG.md` entry under `[Unreleased]`** (`### Added` / `### Changed` / `### Fixed` / `### Removed`) in the same commit. Recent PRs all do this (`f06b67e`, `2afc3af`, `06c45a1`, `bdf026f`). Commit and branch naming are covered in [conventions](../development/conventions.md#commits--branches).

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

Full guide: [hook-authoring-guide](../development/hook-authoring-guide.md#deployment-checklist). Every hook has three touch points, and **the `opencode` mapping in step 3 is the one people miss**:

1. `hooks/claude-code/<hook-name>.sh`: start with the `# devexp hook:` / `# Event: … | Matcher: …` header and `set -euo pipefail`, then `chmod +x`. A security guard must fail closed (`exit 2`) if it can't parse its input, and an advisory hook fails open but prints an internal error ([conventions](../development/conventions.md#error-handling)).
2. `hooks/opencode/<hook-name>.js`: export `async function <hookName>(_ctx)` that returns the event handlers, using `hooks/opencode/utils.js` for shared helpers.
3. Add an entry to `hooks/registry.json` with `name`, `description`, `claude_code{event,matcher,script}`, **`opencode{event,module,export,fail_closed?,enabled?}`** and `enabled`. `module` is `hooks/opencode/<hook-name>.js`, `export` is `<hookName>`, and security guards set `fail_closed: true`. Without the `opencode` mapping, opencode silently lacks the hook: `hooks/opencode/devexp-plugin.js` composes only the modules the installed selection lists, so it is never edited per hook. The Go installer parses every target block into `hooks.Hook.Targets`; the Claude Code install uses `claude_code` and the top-level `enabled` (`cli/internal/hooks/installer.go`). (Today the Go CLI doesn't deploy the opencode plugin at all; see [overview → Known gaps](../architecture/overview.md#known-gaps). Keep the mapping correct anyway.)
4. Write tests. Add a mirrored `<hook-name>.test.sh` and `<hook-name>.test.js` for the decision logic (pattern: `hooks/claude-code/secret-guard.test.sh` ↔ `hooks/opencode/secret-guard.test.js`). If the script extracts input with python3, add a `check <hook-name> 2 guard` or `check <hook-name> 0 advisory` line to `hooks/claude-code/fail-closed.test.sh`. CI runs every `*.test.sh` / `*.test.js` automatically (`.github/workflows/ci.yml`). How to write them: [testing](../development/testing.md).
5. Update the hook catalog and counts: `docs/reference/hooks.md` (catalog table and file tree), `hooks/README.md`, `README.md` ("10 hooks") and `CLAUDE.md` ("10 safety guards").
6. Run `./install.sh`, then check that the hook appears in `~/.claude/settings.json` and exercise both the block and allow paths.

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

## Fix a Bug

1. **Locate the layer.**
   - Install behavior (copy, transform, stale removal, settings.json): `cli/internal/{agents,skills,hooks,mcp,manifest}/`
   - Flags, wizard, target detection, paths, backups: `cli/cmd/`
   - Asset root resolution or embedded extraction: `cli/internal/repo/`
   - A hook blocking or allowing the wrong thing: **both** `hooks/claude-code/<name>.sh` and `hooks/opencode/<name>.js`
   - Agent or skill behavior: `agents/<name>.md` / `skills/<name>/SKILL.md`
   - Uninstall: `uninstall.sh` (its tests extract the embedded python, see `uninstall.test.sh`)
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
| Install manifest `~/.claude/.devexp-manifest.json` | `manifest.Manifest` in `cli/internal/manifest/manifest.go` | Nothing else. Keep `Load` tolerant, so a missing or old manifest loads as empty and the next install records a new baseline |
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

- **Go module:** add or bump it in `cli/go.mod` with the Go module tooling (`go get`, `go mod tidy` in `cli/`), and commit `cli/go.mod` and `cli/go.sum` together. There's no written approval rule. In practice, bumps land in their own `fix:` commit whose body says why (drift or CVE) and confirms the upgrade had no impact (`e6f65e3` cobra/viper, `e147e67` Go toolchain `1.25.11` for stdlib CVEs). CI pins Go `"1.25"` (`.github/workflows/ci.yml`, `.github/workflows/release.yml`), so keep the `go` directive in `cli/go.mod` inside that minor version.
- **Hooks:** don't add third-party packages. Shell hooks use only the python3 standard library, JS hooks use only Node builtins, `hooks/opencode/package.json` declares no dependencies, and the installer has no npm or pip step to install any ([conventions](../development/conventions.md#style)).
- **MCP servers** aren't repo dependencies. They're fetched at runtime by the command in `mcps/registry.json` (for example `npx -y @upstash/context7-mcp`) or live in their own repo, like `ui-inspector` ([Add an MCP server](#add-an-mcp-server)).

## Ship It

See the [release guide](release.md). The release commit (`chore: release vX.Y.Z`) moves `[Unreleased]` into a versioned section, and pushing the `v*` tag triggers goreleaser (`.github/workflows/release.yml`). Your commit prefix decides whether the change appears in GitHub release notes ([conventions](../development/conventions.md#commits--branches)).
