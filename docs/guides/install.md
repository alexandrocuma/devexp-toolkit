# Installing and Managing devexp

## Quick install (no clone)

The `devexp` CLI binary bundles every agent, skill, hook, and the MCP registry, so you can install it directly from a [GitHub Release](https://github.com/alexandrocuma/devexp-toolkit/releases) — no `git clone`, no local Go toolchain:

```bash
curl -fsSL https://raw.githubusercontent.com/alexandrocuma/devexp-toolkit/main/scripts/remote-install.sh | bash
```

This detects your OS/architecture, downloads the matching release binary into `~/.local/bin/devexp`, and runs `devexp install`. Useful overrides:

```bash
curl -fsSL .../remote-install.sh | DEVEXP_VERSION=v1.2.3 bash   # install a specific tag
curl -fsSL .../remote-install.sh | DEVEXP_SKIP_RUN=1 bash       # download only, don't run install
curl -fsSL .../remote-install.sh | DEVEXP_INSTALL_DIR=/opt/bin bash  # put the binary somewhere else (absolute path)
```

The script refuses to run, before downloading anything, when `DEVEXP_INSTALL_DIR` is unset and `HOME` is unset, empty or not an absolute path (the default `~/.local/bin` would otherwise land in `/` or under the current directory), or when `DEVEXP_INSTALL_DIR` itself is relative. An absolute `DEVEXP_INSTALL_DIR` works without `HOME`, but `devexp install`, which runs next, still needs one — combine it with `DEVEXP_SKIP_RUN=1` in that case.

You can also grab a binary manually from the [Releases page](https://github.com/alexandrocuma/devexp-toolkit/releases) — pick the archive matching your OS/arch (`devexp-toolkit_<os>_<arch>.tar.gz`), extract it, and run `./devexp install`. Run `devexp --version` any time to confirm what's installed.

> **How it finds its assets:** a binary built from a clone (what `./install.sh` builds into `bin/devexp`), or any binary with `DEVEXP_DIR` set, reads agents/skills/hooks/MCPs live from disk — so local edits never need a rebuild. Only a devexp-toolkit checkout qualifies: its root has the `.devexp-toolkit` marker file, plus `agents/`, `skills/` and `mcps/`; a directory with just those names is never used. A downloaded release binary never picks up a checkout from where it is or where you run it — set `DEVEXP_DIR` to install from a clone with it. Before installing anything, `devexp install` prints `Asset root: <dir> (<how it was chosen>)`. Agents and skills are *copied* into the CLI's config directory, so re-run the installer to pick up edits to them; Claude Code hooks are registered by absolute path into the repo, so edits to a registered hook script apply immediately. A standalone downloaded binary instead uses the copies baked in at release time, extracted to `<user cache dir>/devexp/assets` and re-extracted when the binary's version changes (`cli/internal/repo/repo.go`, `extractEmbedded`). With no absolute user cache dir (`HOME`, or `XDG_CACHE_HOME` on Linux, unset or relative) it refuses to extract rather than fall back to a temp directory. When a checkout qualifies, it wins over the embedded copies.

---

## install.sh (from a clone)

If you're contributing to the toolkit — editing agents, skills, or hooks — clone the repo and use `install.sh`. `install.sh` is now a thin wrapper: if `bin/devexp` doesn't exist yet it stages the embedded assets and builds the `devexp` Go CLI from `cli/` (requires a local Go toolchain), then execs `devexp install` with whatever flags you pass through (`install.sh:7-20`). Because `devexp` prefers live files on disk over its embedded copies, asset edits never need a rebuild — only changes to the Go code under `cli/` do (see [Updating](#updating)).

The installer is CLI-agnostic. It detects which AI coding CLI(s) are installed and, only when both are present, asks which to target (`cli/cmd/targets.go`). Supported CLIs: **Claude Code** and **opencode**.

```bash
./install.sh                         # interactive wizard
./install.sh --dry-run               # preview what would be installed, no changes made (-n)
./install.sh --reinstall-mcps        # remove registry MCPs then re-add them (forces a config refresh)
./install.sh --mcps-only             # only register MCP servers
./install.sh --agents-only           # only install agents
./install.sh --skills-only           # only install skills
./install.sh --agents-only --model sonnet   # rewrite agents' model: lines (see below)
```

Passing any of `--dry-run`, `--reinstall-mcps`, `--mcps-only`, `--agents-only` or `--skills-only` skips the interactive wizard; with none of them, the wizard runs (`cli/cmd/install.go:115-119`).

`--model` overrides the `model` value from `devexp.config.json` (`cli/cmd/install.go:100-102`). It does **not** skip the wizard — the wizard has no model prompt (`cli/cmd/wizard.go`) — so combine it with one of the flags above for a non-interactive run. It accepts a short alias (`sonnet`, `opus`, `haiku`, `gpt4o`, `deepseek`, `kimi`, …), resolved to a provider-prefixed ID such as `anthropic/claude-sonnet-4-6`, or any other string used verbatim (`modelMap` / `resolveModel` in `cli/internal/agents/installer.go`). The value only **replaces an existing `model:` frontmatter line**; agents without one — most of them (only `dep-audit`, `docs-sync` and `runbook` declare `model:` today) — get no model line, for both CLIs.

**Behavior:**
- Refuses to run when `HOME` is unset, empty or not an absolute path, because every destination below is built from it and would otherwise land under the current directory. `install.sh` checks before it builds `bin/devexp` (a build would put Go's caches under the clone), and `devexp install` checks again as its first step, before it resolves assets, opens the wizard, registers MCPs or writes/backs up anything (`targetHome` in `cli/cmd/paths.go`, checked first in `runInstall`)
- Detects `claude` and/or `opencode` in PATH; prompts which to install for only when both are found, and stops with an error when neither is (`cli/cmd/targets.go`)
- **Claude Code**: copies agents to `~/.claude/agents/`, skill directories to `~/.claude/skills/`, registers MCPs via `claude mcp add`, and registers enabled hooks in `~/.claude/settings.json` (`cli/cmd/install_claude.go`)
- **opencode**: transforms agent frontmatter (model aliases, tool mapping, adds `mode: subagent`) and installs to `~/.config/opencode/agents/`; each skill's `SKILL.md` goes to `~/.config/opencode/commands/<name>.md`; MCPs are written to the `mcp` key of `~/.config/opencode/config.json`; the hook plugin goes to `~/.config/opencode/plugins/` — the entry `devexp.js` plus `devexp/` holding the selected modules, `utils.js`, `package.json` and the `hooks.json` selection (`cli/cmd/install_opencode.go`, `cli/internal/hooks/opencode.go`). With every hook disabled no plugin is installed
- Backs up existing agents and skills before overwriting — **Claude Code target only**; the opencode install has no backup step (`backupExisting` / `backupExistingDirs` are called only from `cli/cmd/install_claude.go`)
- The install script is **idempotent** — safe to run multiple times

---

## Updating

Re-running the installer is how you update devexp — there's no separate "upgrade" command.

- **Binary install**: re-run the `remote-install.sh` one-liner from [Quick install](#quick-install-no-clone). It downloads the latest release binary, overwrites `~/.local/bin/devexp`, and runs `devexp install` again.
- **Clone install**: `git pull && ./install.sh` re-runs `devexp install` against the updated assets, which are read live from the clone (`cli/internal/repo/repo.go`). It does **not** rebuild the CLI — `install.sh` builds only when `bin/devexp` is missing (`install.sh:7`). If the pull changed Go code under `cli/`, rebuild explicitly: `git pull && rm bin/devexp && ./install.sh`. Updating to the release that installs the opencode hook plugin (#108) needs this rebuild.

### What gets overwritten vs. preserved

- **Agents and skills** are overwritten in place with the versions shipped in the new release. Before overwriting, the Claude Code install backs up your existing `~/.claude/agents/*.md` and `~/.claude/skills/<name>/` directories into a timestamped `~/.claude/.devexp-backup-<timestamp>/` folder (`cli/cmd/backup.go`, `cli/cmd/paths.go`). The opencode install makes **no backup** of `~/.config/opencode/agents/` or `commands/` (`cli/cmd/install_opencode.go`).
- **MCP server registrations** are *not* refreshed automatically — pass `--reinstall-mcps` if an MCP's config (command, args, env) changed in the new release.
- **Hooks**:
  - Claude Code: new hooks in `hooks/registry.json` are added to `settings.json`; hooks already registered are left as-is, including ones disabled since.
  - opencode: the plugin files are copied again and rewritten only when their bytes changed. Only the selected modules are copied, and a module disabled since the last run is removed.

### Stale-file cleanup

`devexp install` removes files that a previous run installed but that are no longer part of the current version:

- **Agents and skills**: devexp tracks what it installed in `~/.claude/.devexp-manifest.json` (and `~/.config/opencode/.devexp-manifest.json` for opencode). On each run, anything from the previous manifest that isn't part of this run's install set is removed from disk and dropped from the manifest.
  - Only a bare name devexp installs is ever removed: no path separators, no `..`, no control characters, not empty or `.`. Agents are recorded as `<name>.md`; Claude Code skill directories and opencode commands as `<name>` (a command's file is `<name>.md`). Any other manifest entry is kept, with a warning naming it exactly as recorded, so a corrupted or hand-edited manifest can't delete anything outside `agents/`, `skills/` or `commands/`, or the directory itself.
  - An entry is removed only when it is still what devexp installs there: a regular file, or a real directory for a Claude Code skill. A symlink is never removed (nor what it points at), and neither is a file where a skill directory was, or a directory where an agent file was. Each is kept with a warning, as is an entry that can't be checked (for example, permission denied) (`removeStale` in `cli/cmd/backup.go`).
  - A stale entry that names something this run installed is never removed: the same name apart from case (for example `DEV-AGENT.md` after a rename to `dev-agent.md`, which a case-insensitive filesystem such as default APFS treats as the same file), or the same file on disk. It is kept with a warning. A variant that differs only in Unicode normalization is recognised by the file on disk, so `--dry-run`, which installs nothing, may still preview removing it.
  - Entry names and paths in these lines are printed quoted, so nothing from the manifest reaches the terminal raw. An entry already gone from disk is skipped silently.
  - **One-time caveat**: if you're upgrading from a devexp version that predates manifests, the first run after upgrading has no prior manifest to diff against — it just records a baseline. Stale-file cleanup takes effect starting with the *second* run after upgrading.
- **opencode plugin files**: the opencode manifest's `plugins` key lists every plugin file installed (`devexp.js` first, then `devexp/…`).
  - A plugin file this run doesn't install is removed, and `devexp/` is removed once empty. That covers files from the previous list, plus the devexp files recognised on disk if the manifest was lost.
  - Only `devexp.js` and file names devexp installs directly in `devexp/` are ever removed. Any other path in the manifest is kept with a warning.
  - **`plugins/` is a symlink to a directory** (for example managed from dotfiles):
    - devexp installs through it, writing files atomically inside the link target.
    - It never removes anything through it: not stale plugin files, not the plugin when every hook is disabled, not legacy flat files, not an empty `devexp/`.
    - The output says `plugins/ is a symlink — devexp never removes files through it; remove these by hand: …`. Those files stay recorded in the manifest, so a run after the link is replaced with a directory cleans them up.
  - **`plugins/devexp/` is a symlink**, or `plugins/` is a dangling link or doesn't point at a directory: the install stops with an error before writing or removing anything. A `devexp/` link may point at a source checkout. Replace the link with a real directory.
  - Removing the whole plugin is all-or-nothing. If `devexp.js` has to stay, `devexp/` stays too, and the output says why. It has to stay when it is a symlink, can't be deleted, or isn't recognised as devexp's: not recorded in the manifest and without the devexp header (for example after an editor added `// @ts-check` above it). Removing only `devexp/` would leave an entry that blocks every opencode tool call.
  - Files are written atomically. A `devexp.js` recorded in the manifest is repaired on re-install even if it was damaged; one that isn't recorded and isn't a devexp entry is never replaced.
- **Legacy opencode flat install** (clones from before v0.1.0 copied every hook file flat into `plugins/` and registered `plugins/devexp-plugin.js` in `config.json`). Cleaned up only after the new plugin installed successfully, so a refused install leaves the old one working. Through a symlinked `plugins/` the matching files are listed to remove by hand instead:
  - A file in `plugins/` is removed only when its name is one of the 9 legacy file names **and** its content starts with that file's devexp header. A same-named file without the header is kept, with a warning.
  - `plugins/package.json` is removed only alongside such a match and only if it is exactly `{ "type": "module" }`.
  - The `config.json` `plugin` entry is removed only when it is exactly `<HOME>/.config/opencode/plugins/devexp-plugin.js`. The key goes when the array ends up empty, and every other byte of `config.json` is kept. A symlinked or read-only `config.json` is left untouched, with a warning to remove the entry by hand (`CleanLegacyOpencode` in `cli/internal/hooks/opencode.go`).
- **Hooks (Claude Code)**: devexp checks every registered hook command that points into the devexp repo/cache directory. If the backing script no longer exists (because the hook was removed from `hooks/registry.json`), the dangling entry is removed from `settings.json`. A devexp hook registered from a *different* install root (e.g. a release-binary install later replaced by a clone install) is also removed as a duplicate, matched by registry script name directly under a `hooks/claude-code/` path segment (`pruneForeignDevexpHooks` in `cli/internal/hooks/installer.go`); so is a devexp entry registered as a relative path by an earlier install. Only plain paths count: a command with arguments, variables (`$CLAUDE_PROJECT_DIR/…`), `~`, quotes or other shell syntax, or one under a directory like `my-hooks/claude-code/`, is the user's (`isManagedScriptPath`; `uninstall.sh` uses the same rule). Other user-authored hooks are never touched.

### Behavior change: disabling now removes

Previously, disabling an agent or skill in `devexp.config.json` only skipped *updating* it — the old copy stayed on disk. Now a disabled agent/skill is excluded from the install set entirely, so it's treated as stale and **removed** on the next run. If you need a copy, recover it from the backup directory described above.

### Previewing an update

```bash
./install.sh --dry-run   # or: devexp install --dry-run
```

Shows every add, update, and removal devexp would make — including stale-file and stale-hook cleanup — without touching any files.

### Scoping an update

`--agents-only`, `--skills-only`, and `--mcps-only` limit *both* the install and the stale-file cleanup to that category — e.g. `--agents-only` won't touch your skills manifest or remove stale skills.

---

## uninstall.sh

```bash
./uninstall.sh          # interactive (prompts for confirmation)
./uninstall.sh --yes    # non-interactive
```

**Behavior:**
- Refuses to run (nothing is read or removed, exit 1) when `HOME` is unset, empty or not an absolute path — every path it removes from is built from `HOME`
- Detects which CLIs have devexp agents installed; asks which to remove from only when both are found
- Removes agents from the appropriate directory for each CLI
- Skills (`~/.claude/skills/`) are only removed if uninstalling from all CLIs that use them
- **opencode hook plugin**: an install with only the plugin (no agents) is detected too. The plugin (`plugins/devexp.js` + `plugins/devexp/`) and a legacy flat install are removed by the hidden `devexp uninstall --target opencode`, with exactly the rules of [Stale-file cleanup](#stale-file-cleanup): only devexp-owned files, nothing through a symlinked `plugins/`, a refusal (nothing removed) for a symlinked `devexp/` or a dangling `plugins/` link, all-or-nothing on `devexp.js` + `devexp/`, and the legacy `config.json` entry spliced out byte for byte. It runs before the MCP servers are removed from `config.json`.
  - The preview before the confirmation is that command's `--dry-run`.
  - It needs a `devexp` binary that has the command, looked up in this order: `DEVEXP_BIN` (set when you choose Remove in `devexp install`), `bin/devexp` in the clone, `devexp` on `PATH`. Without one, the plugin is left in place with a warning and the uninstall still completes. In a clone with an older binary, rebuild it: `rm bin/devexp && ./install.sh`.
  - Afterwards the opencode manifest's `plugins` key lists only the files that had to stay (for example behind a symlinked `plugins/`), so a later run can finish the job. A manifest that is missing, can't be read or is a symlink is left as it is.
  - The command refuses to run (nothing is removed) when `HOME` is unset, empty or not an absolute path.
- **opencode MCP servers** are removed from `config.json` by an embedded python step, which never fails the uninstall:
  - a malformed or unexpected `config.json` is skipped with a message;
  - a symlinked `config.json` is left untouched, with a warning to remove the servers by hand;
  - one that can't be written (a read-only file or directory) is left as it was, with a warning, and the remaining steps still run;
  - otherwise it is saved atomically (temp file, then rename), keeping its mode. The file is reformatted (2-space JSON), unlike the plugin step's byte-preserving edit.

> `uninstall.sh` predates the Go CLI and doesn't fully match it — e.g. it never removes opencode skills from `~/.config/opencode/commands/` or the `.devexp-manifest.json` files. See [Known gaps](../architecture/overview.md#known-gaps).

---

## CLI Installation Paths

| Component | Claude Code | opencode |
|-----------|-------------|----------|
| Agents | `~/.claude/agents/` | `~/.config/opencode/agents/` (transformed) |
| Skills | `~/.claude/skills/` | `~/.config/opencode/commands/` (flat `.md`, `name:` stripped) |
| Hooks | `~/.claude/settings.json` (shell scripts) | `~/.config/opencode/plugins/devexp.js` + `devexp/` (selected modules, `utils.js`, `package.json`, `hooks.json`) |
| MCPs | via `claude mcp add` | `mcp` key of `~/.config/opencode/config.json` |
| `CLAUDE.md` / `AGENTS.md` | `~/.claude/CLAUDE.md` | `~/.config/opencode/AGENTS.md` (or project root) |
| Agent tools | All Claude tools | `read/write/edit/bash/glob/grep/webfetch/websearch` only |
| `Agent`, `Skill`, `Task*` tools | Supported | No opencode equivalent — dropped at transform |

> **`.devexp-manifest.json`**: devexp writes `~/.claude/.devexp-manifest.json` and `~/.config/opencode/.devexp-manifest.json` to track which agent/skill files (and, for opencode, plugin files) it installed, so future updates can detect and remove files no longer shipped by the toolkit (see [Updating](#updating)). These are managed automatically — don't hand-edit them.

> **opencode users — feature subset:**
> The following features are unavailable under opencode and are dropped at install time (the installer prints a one-line warning, `cli/cmd/install_opencode.go`):
> multi-agent orchestration tools (`Agent`, `Skill`, `Task*`), persistent agent memory, and terminal colors. Hooks are installed for opencode, including the advisory lint/format/test-on-save hooks, and the three `graphify-*` hooks are on by default there (turn them off with `hooks.disabled`).
> Skills that rely on agent spawning — including the `/deliver` and `/improve` orchestrators — will run in degraded mode.
> **Claude Code is recommended for the full experience.**
