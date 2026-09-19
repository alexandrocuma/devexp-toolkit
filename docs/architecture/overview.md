# Architecture Overview

> Kit doc · Last verified: 2026-09-16 against commit `a86d2c3f6a41a6d033d31afd858ff723d5267dd7`

## What This System Is

devexp-toolkit is a collection of Claude Code and opencode **assets**: agents, skills (slash commands), safety hooks and an MCP server registry. A Go **installer CLI** (`devexp`) copies or registers these assets into a developer's `~/.claude/` or `~/.config/opencode/`. Developers install it once, and teams fork it and adjust `devexp.config.json` to distribute it. After installation, the assets run inside Claude Code or opencode. This repo has no server of its own and nothing runs continuously.

**Stack:**

- Assets: Markdown (agents, skills), bash with inline `python3` (Claude Code hooks), ESM JavaScript on Node builtins (opencode hooks), and JSON registries.
- CLI: Go (`cli/go.mod` module `devexp`, `go 1.25.11`) with cobra (commands), viper (reading `devexp.config.json`) and promptui (wizard).
- No database, network service or auth.

**Entry points:**

- `install.sh`: builds `bin/devexp` if it is missing, then runs `exec bin/devexp install "$@"`.
- `cli/main.go`: calls `cmd.Execute()`, the cobra root in `cli/cmd/root.go`.
- `scripts/remote-install.sh`: downloads a release binary and runs `devexp install` with no clone.
- `uninstall.sh`: removes installed assets. It's a bash script, also reachable from the wizard's "Remove" action (`runRemove` in `cli/cmd/wizard.go`, which passes its own path as `DEVEXP_BIN`). It delegates opencode hook plugin removal to the hidden `devexp uninstall --target opencode`.

## Layers

**Pattern:** two halves. The **asset tree** at the repo root is the product. The **installer CLI** in `cli/` has a thin command layer that resolves options and orchestrates, with one internal package per asset kind doing the file, JSON and exec work. Dependencies point one way, `cmd` → `internal/*`. Internal packages import only `internal/ui` (and `repo` imports `assets`). Everything is wired by hand, with no interfaces or DI.

| Layer | Directory | Responsibility | Canonical example |
|-------|-----------|----------------|-------------------|
| Agents | `agents/` (+ `agents/opencode/` for opencode-only agents) | Role definitions: frontmatter plus a system prompt, written only in Claude Code format. They aren't executable code, and they're run by reading the file ([conventions](../development/conventions.md#module-structure)) | `templates/agent-template.md`; guide: [agent-authoring-guide](../development/agent-authoring-guide.md) |
| Skills | `skills/<name>/` | User-facing slash commands. Orchestrators pull in agents by reading `~/.claude/agents/<name>.md` | `skills/devxp/SKILL.md`; guide: [skill-authoring-guide](../development/skill-authoring-guide.md) |
| Hooks | `hooks/registry.json`, `hooks/claude-code/`, `hooks/opencode/` | Guards and advisory checks that run on every tool call. The registry is the source of truth. There is one `.sh` per hook for Claude Code, and one `.js` module per hook for opencode, composed in `hooks/opencode/devexp-plugin.js` | `hooks/claude-code/secret-guard.sh` + `hooks/opencode/secret-guard.js` (and their tests); guide: [hook-authoring-guide](../development/hook-authoring-guide.md) |
| MCP registry | `mcps/registry.json`, `mcps/.env.example` | Declares MCP servers. Secrets are `${VAR}` placeholders resolved from `mcps/.env` | `mcps/registry.json` (`ui-inspector` shows `required_env` + `setup_instructions`); guide: [mcp-guide](../development/mcp-guide.md) |
| Team config | `devexp.config.json`, `devexp.config.schema.json` | Per-fork settings: model, disabled agents/skills/hooks, extra MCPs | [team-distribution](../guides/team-distribution.md) |
| Shell wrappers | `install.sh`, `uninstall.sh`, `scripts/` | Build-if-missing and run the CLI; uninstall; remote install; stage the embedded assets | `install.sh` |
| CLI commands | `cli/main.go`, `cli/cmd/` | Parse flags or run the wizard, resolve `installOpts`, run each target's install in order. Holds pure decision functions and thin I/O wrappers. No file, JSON or exec logic for a particular asset kind belongs here | `cli/cmd/targets.go`, `cli/cmd/paths.go` |
| Asset installers | `cli/internal/{agents,skills,hooks,mcp}` | Install one asset kind into one target from explicit paths, returning what was installed. They don't read `$HOME` or config themselves | `cli/internal/hooks/installer.go` |
| Support packages | `cli/internal/{repo,assets,config,manifest,fsutil,removeguard}` | Find the asset root (clone or embedded), embed assets, load config/dotenv, record what was installed, save every file atomically and symlink-safely (`fsutil.WriteFileAtomic`, `fsutil.IsSymlink`), and decide whether a target directory is a symlink or behind one, so nothing is removed through it (`removeguard`) | `cli/internal/manifest/manifest.go` |
| Terminal UI | `cli/internal/ui` | Colored stdout output helpers and promptui prompts. The logic behind the prompts lives in pure helpers | `cli/internal/ui/output.go`, `cli/internal/ui/prompts.go` |

## Request / Job Flow

### `./install.sh` (Claude Code target)

```
./install.sh [flags]
  → install.sh: if bin/devexp is missing → scripts/stage-assets.sh + (cd cli && go build -o ../bin/devexp .)
  → exec bin/devexp install "$@"
  → cli/cmd/root.go Execute() → cli/cmd/install.go runInstall()
      → cli/internal/repo/repo.go Resolve(version, announceAssetRoot)
          locate(): no directory is ever searched for (not cwd, not next to the binary)
                         $DEVEXP_DIR (must be a checkout, else error — no fallback)
                         → dev builds only (version == "dev"): sourceCheckout(), the checkout
                           compiled in via runtime.Caller (none under -trimpath), used only if
                           ownedCheckout() verifies it as the user's: root, root files and every
                           entry under agents/ skills/ mcps/ hooks/ owned by the user, no
                           symlinks, no other-write, group-write only via the user's private
                           group; parent owned by user/root, same write rule unless sticky; if it
                           lacks the marker, is gone or can't be verified → Source.Warning,
                           fall through
                         a checkout = .devexp-toolkit marker (regular file, first line
                         "devexp-toolkit") + agents/ skills/ mcps/ (isRepoDir)
          else embeddedDir(version): os.UserCacheDir()/devexp/assets, or …/assets-dev for
                         dev builds (must be absolute)
          → announce(Source): install.go prints the warning, then
                         "Asset root: <dir> (<origin>)" — before any write
          → if embedded, extractEmbedded(): assets.FS (marker included) → a fresh sibling
                         .assets.<rand>/ (dirs made one level at a time, so a tree swept
                         mid-run fails), then a symlink renamed over that dir (atomic; a
                         failed swap puts a moved-aside in-place dir back); the retired tree
                         is kept (mtime refreshed) and sweepStale removes unlinked siblings
                         older than 1h, after an extraction or a reuse; reused while
                         .devexp-version == a tagged version (dev builds always re-extract);
                         *.sh written 0755
          → Source{RepoDir, Embedded, Origin, Warning}; every installer below — and the
                         wizard's Remove (runRemove) — reads RepoDir on disk
      → cli/internal/config/config.go Load(<repo>/devexp.config.json)   (missing → ui.Warn + defaults; --model overrides)
      → cli/internal/config/dotenv.go LoadDotenv(<repo>/mcps/.env)
      → cli/cmd/registry.go buildEnv(): OS env + DEVEXP_DIR + dotenv (dotenv wins)
      → any of --dry-run/--reinstall-mcps/--*-only set?
          yes → cli/cmd/targets.go detectTargets() (PATH lookup of claude/opencode/kimi; kimi is
                also version-probed) → announceTargets → resolveTargets(--target | checklist when
                >1 and a TTY | every detected CLI)
          no  → cli/cmd/wizard.go runWizard() (action → scope → detect/announce/resolveTargets
                → agent/MCP/hook multi-selects; "Remove" → runRemove → bash uninstall.sh)
      → installOpts{...}; selected* nil = "all minus config-disabled"
        (resolveAgentDisabled/resolveHookDisabled in cli/cmd/registry.go)
      → cli/cmd/install_claude.go doInstallClaude(opts)   paths: cli/cmd/paths.go claudeTargetPaths($HOME, now)
          1. MCPs      installMCPsClaude → loadFullRegistry (mcps/registry.json + cfg mcps) → filterMCPs
                       → cli/internal/mcp/claude.go InstallClaude → AddClaude:
                         skip if `claude mcp list` output contains the name; else `claude mcp add --scope user …`
                         (missing required_env → ui.Required + setup_instructions, not an error)
          2. manifest  cli/cmd/backup.go loadOldManifest → cli/internal/manifest Load(~/.claude/.devexp-manifest.json)
                       (unreadable/corrupt → warn, treat as empty, so nothing is stale this run)
          3. agents    cli/cmd/backup.go backupExisting(*.md → ~/.claude/.devexp-backup-<YYYYMMDDTHHMMSS>)
                       → cli/internal/agents/installer.go InstallClaude (copy, atomic; --model rewrites an existing model: line;
                         a symlinked agent file is kept as is, warned, still recorded)
                       → removeStale(old, new, staleFile, os.Remove): manifest.Stale(old, new), minus
                         case variants of / the same file as an installed name
                         (bare <name>.md regular files only; other entries and symlinks kept, warned;
                         exact listed name only; nothing removed through an agents/ that is a symlink
                         or behind one (removeguard.BehindSymlink): listed, and kept in the manifest;
                         removals go through os.Root)
          4. skills    backupExistingDirs → cli/internal/skills/installer.go InstallClaude (CopyDir whole skill dir;
                         a symlinked skill dir, or file/dir inside one, is kept as is, warned, dry run too)
                       → removeStale(..., staleDir, os.RemoveAll) (bare names, real directories only;
                         same symlinked-directory rule)
          5. hooks     cli/internal/hooks/installer.go LoadRegistry → InstallClaude(~/.claude/settings.json):
                       keep unknown keys; pruneStaleHooks (script gone: under repoDir, or under another
                         root holding a hooks/registry.json — isOrphanedDevexpHook)
                       + pruneForeignDevexpHooks (same registry script under another install root);
                       append {matcher, hooks:[{type: command, command: <repoDir>/hooks/claude-code/<name>.sh}]}
                       for each EnabledFor(claude_code), non-disabled hook not already registered;
                       write only if changed (fsutil.WriteFileAtomic: temp + fsync + rename of the symlink's target)
          6. manifest  manifest.Save (skipped in dry-run)
  → cobra prints any returned error to stderr, exit 1
```

`--mcps-only` stops after step 1. `--agents-only` and `--skills-only` skip MCPs and hooks, and only their own category's manifest entries are replaced (`cli/cmd/install_claude.go`).

### opencode target

`runInstall` → `cli/cmd/install_opencode.go` `doInstallOpencode(opts)`, with paths from `opencodeTargetPaths($HOME)`:

1. Warn that opencode gets a subset of features.
2. `installMCPsOpencode` → `cli/internal/mcp/opencode.go` `InstallOpencode` writes the `mcp` map in `~/.config/opencode/config.json` (`type: local|remote`, updating entries in place when they differ). A config that isn't strict JSON, or whose top level or `mcp` isn't an object, is left untouched (`loadOpencodeConfig` → `mcp.ConfigRefusedError`): `doInstallOpencode` warns with the servers to add by hand and skips only the MCP step, while `--mcps-only` returns the error. The file is re-encoded as 2-space JSON, and saved atomically through `fsutil.WriteFileAtomic`.
3. `agents.InstallOpencode` → `transformForOpencode` drops `name`/`color`/`memory`, turns `tools` into an explicit `false` deny list over the 8 opencode tools, resolves model aliases and appends `mode: subagent`. `agents.InstallOpencodeExclusive` then installs `agents/opencode/*.md`, changing only the model. Stale agents from the previous manifest are then removed (`removeStale` with `staleFile`: bare `<name>.md` regular files only; other entries and symlinks are kept, with a warning; nothing is removed through a symlinked `agents/`, and what is left there stays in the manifest). A symlinked agent file is never written; it is kept, with a warning. A symlinked agent file is never written; it is kept, with a warning.
4. `skills.InstallOpencode` writes `~/.config/opencode/commands/<name>.md` from `SKILL.md` alone, with the top-level `name:` stripped. Stale commands are then removed (`removeStale` with `staleCommand`: entries are recorded as bare `<name>`, and only a regular `<name>.md` file is removed; a warning names a rejected entry as recorded; same symlinked-directory rule). A symlinked `<name>.md` is never written; it is kept, with a warning. A symlinked `<name>.md` is never written; it is kept, with a warning.
5. Hooks (skipped by `--agents-only`/`--skills-only`), in `cli/internal/hooks/opencode.go`:
   - `InstallOpencode` selects hooks with an `opencode.module` that are `EnabledFor(opencode)` and not in `resolveHookDisabled`.
   - It validates before touching anything:
     - `plugins/devexp/` is refused if it is a symlink, and `plugins/` if it is a dangling link or doesn't point at a directory. A `plugins/` symlinked to a directory is written through, but every removal through it is skipped and listed in a warning;
     - every source must be readable;
     - no destination may be a symlink or a directory;
     - `devexp.js` must be a devexp entry or recorded in the manifest.
   - It then writes the selected modules, `utils.js`, `package.json` and `devexp/hooks.json` atomically, the entry `plugins/devexp.js` last.
   - It removes plugin files it no longer installs, entry first. Those are the previous manifest's `plugins` plus the devexp files it recognises on disk. Removal is all-or-nothing when the entry must stay (a symlink, undeletable, or not recognised as devexp's), and an empty real `devexp/` is removed. `devexp uninstall --target opencode` uses the same removal code. With nothing selected it installs nothing.
   - Only after that succeeds, `CleanLegacyOpencode` removes pre-v0.1.0 flat-install files (legacy name **and** header signature) and the exact legacy `config.json` `plugin` entry.
6. `manifest.Save` writes `~/.config/opencode/.devexp-manifest.json` (skipped in dry-run).

Unlike the Claude Code target, there's **no backup step**.

### Install: Kimi Code CLI

`runInstall` → `cli/cmd/install_kimi.go` `doInstallKimi(opts)`, with paths from `kimiTargetPaths($KIMI_CODE_HOME, $HOME, now)`. It installs MCP servers, then agents, then skills, then hooks, in `doInstallClaude`'s order — everything devexp ships. Each of `--mcps-only`/`--agents-only`/`--skills-only` installs exactly its own kind and skips hooks, so no flag combination leaves a Kimi run with nothing to do. The one thing still missing is the way out: `./uninstall.sh` cannot remove a Kimi install yet (#115), and every run says so.

1. `loadOldManifest` reads `$KIMI/.devexp-manifest.json`; the whole struct is carried forward, so nothing a later version adds is dropped.
2. `installMCPsKimi` → `cli/internal/mcp/kimi.go` `InstallKimi` merges the registry into the `mcpServers` object of `$KIMI/mcp.json`:
   - `resolveMCP` (`cli/internal/mcp/resolve.go`) expands every `${VAR}` first — Kimi expands nothing at read time — and an MCP whose `required_env` (or a placeholder) is unset is skipped with the usual `[REQUIRED]` notice rather than written half-configured.
   - Each entry is checked against Kimi's own schema (`validateKimiEntry`, a discriminated union on `transport`) before it goes in, and the whole rendered file is parsed and re-checked before it replaces anything: Kimi rejects the *entire file* if one entry is invalid, and only says so in `$KIMI/logs/kimi-code.log`.
   - `manifest.MCPs` holds a fingerprint per entry devexp wrote. An entry that is not devexp's, or is devexp's but has been edited, is skipped and reported; `--reinstall-mcps` replaces it anyway. An entry devexp wrote and no longer installs is removed.
   - Nothing is written when nothing changed. When something did, the document is re-rendered with the user's key order and untouched values intact, two-space indented with a trailing newline — the format Kimi's own writer produces — and saved through `fsutil.WriteFileAtomic` (0600 for a new file).
   - A file that is not strict JSON, whose top level is not an object, or whose `mcpServers` is not one, is left untouched (`mcp.ConfigRefusedError`): the MCP step is skipped with a warning and the run continues.
3. `hooks.InstallKimi` (`cli/internal/hooks/kimi_install.go`) copies `hooks/kimi/adapter.sh`, the selected guards and `hooks/claude-code/scan-budget.sh` into `$KIMI/hooks/`, keeping the executable bit, and only then calls `WriteKimiHooks` to register the commands:
   - Copy before register, always: Kimi reads a command it cannot run as an **allow**, so a registration pointing at a script that is not there yet is a disarmed guard. A copy that fails returns what it already wrote and registers nothing.
   - `SelectKimi` (`cli/internal/hooks/kimi.go`) picks only the hooks Kimi can honour and gives a reason for each of the others, printed on the run that would have installed it.
   - `WriteKimiHooks` rewrites only the lines between devexp's two marker comments, validates every entry against Kimi's `HookDefSchema` first, and re-parses the rendered file before it replaces anything. Kimi drops comments whenever it rewrites `config.toml` (on login), so when no markers are present an unmarked entry whose command runs devexp's adapter is recognised as devexp's and re-marked, rather than duplicated.
   - What it installed is recorded in the manifest's `hooks` list, and a script devexp no longer ships is removed along with its registration.
4. `manifest.Save` writes `$KIMI/.devexp-manifest.json`, including on `--mcps-only`, because for Kimi the manifest *is* the MCP ownership record (skipped in dry-run).

Every path this target prints is quoted: the root comes from `$KIMI_CODE_HOME`. Nothing is exec'd — Kimi has no `mcp add` command — and the MCP step has no backup, because it merges into `mcp.json` rather than replacing it; the agent and skill steps back up what is already there, as the Claude Code install does.

### A tool call at runtime (after install)

```
Claude Code PreToolUse/PostToolUse event matching a registered matcher
  → runs the absolute script path written into ~/.claude/settings.json (under the install root: clone or user cache)
  → hooks/claude-code/<name>.sh reads the JSON envelope on stdin, extracts fields with python3
  → exit 2 + stderr reason = block · exit 0 silent = allow · "ask" JSON on stdout = confirm (large-file-guard)

opencode tool.execute.before
  → hooks/opencode/devexp-plugin.js loads the modules listed in devexp/hooks.json and runs their handlers in sequence; the first throw blocks

opencode event (type file.edited)
  → devexp-plugin.js queues each module's file.edited handler with { file } and returns at once; the queue runs edits one at a time, commands spawn asynchronously, errors are logged, never rethrown
```

Hook commands point into the install root, so editing a registered script in the clone changes behavior immediately. Moving or deleting the clone breaks the registrations until the next install prunes them (`scriptAbs := filepath.Join(repoDir, cc.Script)` and `pruneStaleHooks` in `cli/internal/hooks/installer.go`). Agents and skills are copies, so changes to them need a re-install.

## Key Directories

| Directory | Responsibility |
|-----------|----------------|
| `agents/` | 34 agent definitions; `agents/opencode/` holds opencode-only agents (`orchestrator.md`) |
| `skills/` | 8 slash commands, one directory each; supporting files (e.g. `skills/graphify/references/`) reach Claude Code only |
| `hooks/claude-code/` | Shell hooks and their `*.test.sh` tests, including `fail-closed.test.sh` |
| `hooks/opencode/` | JS hook modules, `devexp-plugin.js` (composition), `utils.js` (shared helpers), `*.test.js` tests |
| `mcps/` | `registry.json`, `.env.example` (`.env` is gitignored) |
| `templates/` | Starting points for new agents and skills |
| `cli/cmd/` | cobra commands and install orchestration |
| `cli/internal/` | One package per asset kind or concern (see Layers) |
| `cli/internal/assets/` | `assets.go` (`//go:embed all:agents all:skills all:hooks all:mcps devexp.config.json uninstall.sh`). The staged copies next to it are gitignored and produced by `scripts/stage-assets.sh`, and the package doesn't compile until they exist |
| `cli/internal/agents/testdata/` | Agent fixtures for transformation tests |
| `scripts/` | `stage-assets.sh` (rsync of assets into `cli/internal/assets/`), `remote-install.sh`, `govulncheck.sh` (govulncheck for every release platform, run by `ci.yml`'s `govulncheck` job — which `release.yml` reaches by calling `ci.yml`) |
| `.github/workflows/` | `ci.yml` (Go tests, hook tests, installer script tests, govulncheck; on PRs, pushes to `main`, a weekly schedule, and `workflow_call`), `release.yml` (tag → calls `ci.yml` → goreleaser) |
| `docs/` | All documentation; start at [`docs/README.md`](../README.md) |

## External Dependencies

| Dependency | Used for | Client code |
|------------|----------|-------------|
| `claude` CLI | Detecting the install target; registering and removing MCPs (`claude mcp list/add/remove`) | `cli/cmd/targets.go` (`commandExists`), `cli/internal/mcp/claude.go` |
| `opencode` CLI | Detecting the install target only (on PATH). Its config file is edited directly | `cli/cmd/targets.go`, `cli/internal/mcp/opencode.go` |
| `kimi` CLI | Detecting the install target, and one bounded `kimi --version` probe to tell Kimi Code CLI from the unsupported legacy kimi-cli. Nothing is run to install: MCP servers are merged into `$KIMI_CODE_HOME/mcp.json` directly, because Kimi has no `mcp add` command; agents, skills and the hook scripts are written as files, and the hooks are registered as a marked `[[hooks]]` block in `config.toml` | `cli/cmd/kimi_detect.go`, `cli/cmd/paths.go` (`kimiTargetPaths`), `cli/cmd/install_kimi.go`, `cli/internal/mcp/kimi.go`, `cli/internal/hooks/kimi_install.go` |
| `~/.claude/settings.json` | Hook registration (only the `hooks` value is rewritten; other bytes and users' hook fields are kept) | `cli/internal/hooks/installer.go`, `cli/internal/hooks/settings.go` |
| User cache dir (`os.UserCacheDir()/devexp/assets`, `…/assets-dev` for dev builds) | Assets extracted from the embedded FS when no clone is found | `cli/internal/repo/repo.go` |
| cobra, viper, promptui | Commands; reading `devexp.config.json`; the interactive wizard (needs a TTY) | `cli/cmd/root.go`, `cli/internal/config/config.go`, `cli/internal/ui/prompts.go` |
| `python3` | Parsing hook input at runtime; `uninstall.sh` JSON edits | `hooks/claude-code/*.sh`, `uninstall.sh` |
| Node | Running opencode hook modules (inside opencode); hook tests in CI (Node 22) | `hooks/opencode/*.js`, `.github/workflows/ci.yml` |
| Go toolchain, `rsync` | Building from a clone | `install.sh`, `scripts/stage-assets.sh` |
| GitHub Releases | Release binaries (goreleaser), downloaded by the remote installer | `.goreleaser.yaml`, `.github/workflows/release.yml`, `scripts/remote-install.sh` |
| MCP servers | `context7` via `npx -y @upstash/context7-mcp`; `ui-inspector` from its own repo via `UI_INSPECTOR_DIR` | `mcps/registry.json` |

### Known gaps

- **Disabled hooks behave differently per CLI.** Re-installing opencode removes a hook disabled since the last run; Claude Code keeps it registered in `settings.json` (`hooks.InstallClaude` never removes a registry hook; it only adds, re-quotes and updates matchers of enabled ones).
- **`uninstall.sh` doesn't match the CLI.** It treats `~/.claude/skills` as "shared between both CLIs" (`uninstall.sh`, `SKILLS_DIR`), but the CLI writes opencode skills to `~/.config/opencode/commands/` (`cli/cmd/paths.go`). It doesn't remove `.devexp-manifest.json`. The opencode hook plugin is removed by the hidden `devexp uninstall --target opencode` (`cli/cmd/uninstall.go`), with the installer's own rules; without a `devexp` binary that has that command, `uninstall.sh` leaves the plugin in place (#109). Its opencode MCP removal is still python. It cuts only the removed servers out of `config.json`'s bytes (#124), and skips a symlinked, unwritable or too-deeply-nested `config.json`. Install's MCP merge, by contrast, re-encodes the whole `config.json` (key order, indentation).
- **Config schema is narrower than the loader.** `devexp.config.schema.json` `mcps.items` sets `additionalProperties: false` and requires `command`, but `mcp.MCP` also accepts `transport`, `url`, `headers` and `setup_instructions` (`cli/internal/mcp/types.go`). An HTTP/SSE MCP declared in config fails schema validation even though the installer supports it.

## Reference Implementation

- **Go package: `cli/internal/manifest`** (`manifest.go` + `manifest_test.go`). Read it before building anything new. It's small, with a package doc comment, doc comments that explain tolerance decisions, a pure `Stale` function, and table tests on `t.TempDir()`.
- For a richer example, see **`cli/internal/hooks`** (`installer.go` + `installer_test.go`), with why-comments, fixture helpers and prune logic tested case by case.
- **Command layer: `cli/cmd/targets.go`** and `cli/cmd/paths.go`, the post-#97 style: pure decision functions, I/O wrappers, and the clock and home dir passed in.
- **Hook: `secret-guard`**, the full set: `hooks/claude-code/secret-guard.sh` and `.test.sh`, `hooks/opencode/secret-guard.js` and `.test.js`, and a registry entry. The header comments record rationale with issue refs and a `Tests:` run line.

## Decisions

Architecture decisions that constrain implementation: [`adr/README.md`](adr/README.md). None have been recorded yet (the index table is empty). The rationale for past decisions lives in commit bodies, for example `06c45a1` (#97, splitting cmd into pure and I/O parts), `2afc3af` (#92, hooks fail closed) and `f06b67e` (#94, pruning hooks registered from another install root).
