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

> **How it finds its assets:** when `devexp` runs from inside a cloned repo (or with `DEVEXP_DIR` set), it reads agents/skills/hooks/MCPs live from disk — so local edits take effect immediately without a rebuild. A standalone downloaded binary instead uses the copies baked in at release time (extracted on first run to a per-version cache directory). Filesystem always wins when both are available.

---

## install.sh (from a clone)

If you're contributing to the toolkit — editing agents, skills, or hooks — clone the repo and use `install.sh`. `install.sh` is now a thin wrapper: it builds the `devexp` Go CLI from `cli/` (requires a local Go toolchain) and execs `devexp install` with whatever flags you pass through. Because `devexp` prefers live files on disk over its embedded copies, your edits are picked up immediately without rebuilding the binary.

The installer is CLI-agnostic. It detects which AI coding CLI(s) are installed and asks which to target. Supported CLIs: **Claude Code** and **opencode**.

```bash
./install.sh                         # interactive
./install.sh --dry-run               # preview what would be installed, no changes made (-n)
./install.sh --model sonnet          # skip model prompt, use claude-sonnet-4-6
./install.sh --model opus            # skip model prompt, use claude-opus-4-6
./install.sh --reinstall-mcps        # remove registry MCPs then re-add them (forces a config refresh)
./install.sh --mcps-only             # only register MCP servers
./install.sh --agents-only           # only install agents
./install.sh --skills-only           # only install skills
```

`--model` accepts a short alias (`sonnet`, `opus`, `haiku`, `gpt4o`, `deepseek`, `kimi`, …) or a full model ID — see `cli/internal/agents/installer.go` for the alias table.

**Behavior:**
- Detects `claude` and/or `opencode` in PATH and prompts which to install for
- **Claude Code**: copies agents to `~/.claude/agents/`, skills to `~/.claude/skills/`, registers MCPs via `claude mcp add`
- **opencode**: transforms agent frontmatter (model aliases, tool mapping, adds `mode: subagent`) and installs to `~/.config/opencode/agents/`; skills go to `~/.claude/skills/`; MCPs are written to `~/.config/opencode/config.json`
- Backs up any conflicting files before overwriting
- The install script is **idempotent** — safe to run multiple times

---

## Updating

Re-running the installer is how you update devexp — there's no separate "upgrade" command.

- **Binary install**: re-run the `remote-install.sh` one-liner from [Quick install](#quick-install-no-clone). It downloads the latest release binary, overwrites `~/.local/bin/devexp`, and runs `devexp install` again.
- **Clone install**: `git pull && ./install.sh` rebuilds the CLI from the updated source and re-runs `devexp install`.

### What gets overwritten vs. preserved

- **Agents and skills** are overwritten in place with the versions shipped in the new release. Before overwriting, devexp backs up your existing `~/.claude/agents/*.md` and `~/.claude/skills/<name>/` directories (and the opencode equivalents) into a timestamped `~/.claude/.devexp-backup-<timestamp>/` folder.
- **MCP server registrations** are *not* refreshed automatically — pass `--reinstall-mcps` if an MCP's config (command, args, env) changed in the new release.
- **Hooks**: new hooks in `hooks/registry.json` are added to `settings.json`; hooks already registered are left as-is.

### Stale-file cleanup

`devexp install` removes files that a previous run installed but that are no longer part of the current version:

- **Agents and skills**: devexp tracks what it installed in `~/.claude/.devexp-manifest.json` (and `~/.config/opencode/.devexp-manifest.json` for opencode). On each run, anything from the previous manifest that isn't part of this run's install set is removed from disk and dropped from the manifest.
  - **One-time caveat**: if you're upgrading from a devexp version that predates manifests, the first run after upgrading has no prior manifest to diff against — it just records a baseline. Stale-file cleanup takes effect starting with the *second* run after upgrading.
- **Hooks**: devexp checks every registered hook command that points into the devexp repo/cache directory. If the backing script no longer exists (because the hook was removed from `hooks/registry.json`), the dangling entry is removed from `settings.json`. User-authored hooks pointing elsewhere are never touched.

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

## start-services.sh

Use this after a machine restart or when MCP services have stopped. Never wipes data or venvs.

```bash
./start-services.sh            # start anything that isn't running
./start-services.sh --status   # check service health without starting
```

**Behavior:**
- **ui-inspector** manages its own headless Chromium process — no daemons to start
- Safe to run at any time — skips services that are already running

After running, reconnect your MCP in Claude Code (`/mcp`) or opencode.

---

## uninstall.sh

```bash
./uninstall.sh          # interactive (prompts for confirmation)
./uninstall.sh --yes    # non-interactive
```

**Behavior:**
- Detects which CLIs have devexp installed and asks which to remove from
- Removes agents from the appropriate directory for each CLI
- Skills (`~/.claude/skills/`) are only removed if uninstalling from all CLIs that use them

---

## CLI Installation Paths

| Component | Claude Code | opencode |
|-----------|-------------|----------|
| Agents | `~/.claude/agents/` | `~/.config/opencode/agents/` (transformed) |
| Skills | `~/.claude/skills/` | `~/.config/opencode/commands/` (flat `.md`, `name:` stripped) |
| Hooks | `~/.claude/settings.json` (shell scripts) | `~/.config/opencode/plugins/devexp-plugin.js` (JS modules) |
| `CLAUDE.md` / `AGENTS.md` | `~/.claude/CLAUDE.md` | `~/.config/opencode/AGENTS.md` (or project root) |
| Agent tools | All Claude tools | `read/write/edit/bash/glob/grep/webfetch/websearch` only |
| `Agent`, `Skill`, `Task*` tools | Supported | No opencode equivalent — dropped at transform |

> **`.devexp-manifest.json`**: devexp writes `~/.claude/.devexp-manifest.json` and `~/.config/opencode/.devexp-manifest.json` to track which agent/skill files it installed, so future updates can detect and remove files no longer shipped by the toolkit (see [Updating](#updating)). These are managed automatically — don't hand-edit them.

> **opencode users — feature subset:**
> The following features are unavailable under opencode and are silently dropped at install time:
> multi-agent orchestration tools (`Agent`, `Skill`, `Task*`), persistent agent memory, and terminal colors.
> Skills that rely on agent spawning — including the `/deliver` and `/improve` orchestrators — will run in degraded mode.
> **Claude Code is recommended for the full experience.**
