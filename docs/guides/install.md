# Installing and Managing devexp

## Quick install (no clone)

The `devexp` CLI binary bundles every agent, skill, hook, and the MCP registry, so you can install it directly from a [GitHub Release](https://github.com/alexandrocuma/devexp-toolkit/releases) — no `git clone`, no local Go toolchain:

```bash
curl -fsSL https://raw.githubusercontent.com/alexandrocuma/devexp-toolkit/main/scripts/remote-install.sh | bash
```

This detects your OS/architecture, downloads the matching release binary into `~/.local/bin/devexp`, and runs `devexp install`. Useful overrides:

```bash
DEVEXP_VERSION=v1.2.3 curl -fsSL .../remote-install.sh | bash   # install a specific tag
DEVEXP_SKIP_RUN=1 curl -fsSL .../remote-install.sh | bash       # download only, don't run install
```

You can also grab a binary manually from the [Releases page](https://github.com/alexandrocuma/devexp-toolkit/releases) — pick the archive matching your OS/arch (`devexp-toolkit_<os>_<arch>.tar.gz`), extract it, and run `./devexp install`. Run `devexp --version` any time to confirm what's installed.

> **How it finds its assets:** when `devexp` runs from inside a cloned repo (or with `DEVEXP_DIR` set), it reads agents/skills/hooks/MCPs live from disk — so local edits never need a rebuild. Agents and skills are *copied* into the CLI's config directory, so re-run the installer to pick up edits to them; Claude Code hooks are registered by absolute path into the repo, so edits to a registered hook script apply immediately. A standalone downloaded binary instead uses the copies baked in at release time, extracted to `<user cache dir>/devexp/assets` and re-extracted when the binary's version changes (`cli/internal/repo/repo.go`, `extractEmbedded`). Filesystem always wins when both are available.

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

Passing any of `--dry-run`, `--reinstall-mcps`, `--mcps-only`, `--agents-only` or `--skills-only` skips the interactive wizard; with none of them, the wizard runs (`cli/cmd/install.go:108-112`).

`--model` overrides the `model` value from `devexp.config.json` (`cli/cmd/install.go:93-95`). It does **not** skip the wizard — the wizard has no model prompt (`cli/cmd/wizard.go`) — so combine it with one of the flags above for a non-interactive run. It accepts a short alias (`sonnet`, `opus`, `haiku`, `gpt4o`, `deepseek`, `kimi`, …), resolved to a provider-prefixed ID such as `anthropic/claude-sonnet-4-6`, or any other string used verbatim (`modelMap` / `resolveModel` in `cli/internal/agents/installer.go`). The value only **replaces an existing `model:` frontmatter line**; agents without one — most of them (only `dep-audit`, `docs-sync` and `runbook` declare `model:` today) — get no model line, for both CLIs.

**Behavior:**
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
  - **One-time caveat**: if you're upgrading from a devexp version that predates manifests, the first run after upgrading has no prior manifest to diff against — it just records a baseline. Stale-file cleanup takes effect starting with the *second* run after upgrading.
- **opencode plugin files**: the opencode manifest's `plugins` key lists every plugin file installed (`devexp.js` first, then `devexp/…`). A file from the previous list that this run doesn't install is removed, and `devexp/` is removed once empty. Only `devexp.js` (while it is still a devexp entry) and file names devexp installs directly in `devexp/` are ever removed this way; any other path in the manifest is kept with a warning.
- **Legacy opencode flat install** (clones from before v0.1.0 copied every hook file flat into `plugins/` and registered `plugins/devexp-plugin.js` in `config.json`):
  - A file in `plugins/` is removed only when its name is one of the 9 legacy file names **and** its content starts with that file's devexp header. A same-named file without the header is kept, with a warning.
  - `plugins/package.json` is removed only alongside such a match and only if it is exactly `{ "type": "module" }`.
  - The `config.json` `plugin` entry is removed only when it is exactly `<HOME>/.config/opencode/plugins/devexp-plugin.js`. The key goes when the array ends up empty, and every other byte of `config.json` is kept (`CleanLegacyOpencode` in `cli/internal/hooks/opencode.go`).
- **Hooks (Claude Code)**: devexp checks every registered hook command that points into the devexp repo/cache directory. If the backing script no longer exists (because the hook was removed from `hooks/registry.json`), the dangling entry is removed from `settings.json`. A devexp hook registered from a *different* install root (e.g. a release-binary install later replaced by a clone install) is also removed as a duplicate, matched by registry script name under `hooks/claude-code/` (`pruneForeignDevexpHooks` in `cli/internal/hooks/installer.go`). Other user-authored hooks are never touched.

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
- Detects which CLIs have devexp agents installed; asks which to remove from only when both are found
- Removes agents from the appropriate directory for each CLI
- Skills (`~/.claude/skills/`) are only removed if uninstalling from all CLIs that use them

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
