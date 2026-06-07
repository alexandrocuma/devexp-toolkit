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

If you're contributing to the toolkit — editing agents, skills, or hooks — clone the repo and use `install.sh`. Because `devexp` prefers live files on disk over its embedded copies, your edits are picked up immediately without rebuilding.

The installer is CLI-agnostic. It detects which AI coding CLI(s) are installed and asks which to target. Supported CLIs: **Claude Code** and **opencode**.

```bash
./install.sh                         # interactive
./install.sh --dry-run               # preview what would be installed, no changes made
./install.sh --model sonnet          # skip model prompt, use claude-sonnet-4-6
./install.sh --model opus            # skip model prompt, use claude-opus-4-6
./install.sh --reinstall-openviking  # wipe and reinstall the OpenViking MCP from scratch
./install.sh --reinstall-jina        # wipe and reinstall the Jina embeddings server from scratch
./install.sh --mcps-only             # only register MCP servers
./install.sh --agents-only           # only install agents
./install.sh --skills-only           # only install skills
```

**Behavior:**
- Detects `claude` and/or `opencode` in PATH and prompts which to install for
- **Claude Code**: copies agents to `~/.claude/agents/`, skills to `~/.claude/skills/`, registers MCPs via `claude mcp add`
- **opencode**: transforms agent frontmatter (model aliases, tool mapping, adds `mode: subagent`) and installs to `~/.config/opencode/agents/`; skills go to `~/.claude/skills/`; MCPs are written to `~/.config/opencode/config.json`
- Backs up any conflicting files before overwriting
- The install script is **idempotent** — safe to run multiple times

---

## start-services.sh

Use this after a machine restart or when MCP services have stopped. Never wipes data or venvs.

```bash
./start-services.sh            # start anything that isn't running
./start-services.sh --status   # check service health without starting
```

**Behavior:**
- **Jina** (Docker): health-checks via HTTP, restarts the container if needed — no model re-download
- **OpenViking** (Python): restarts the server process using the existing venv — no data wipe, no index rebuild
- Safe to run at any time — skips services that are already running

> Do **not** use `./install.sh --reinstall-openviking` to restart — it wipes the venv and all indexed knowledge. Use `start-services.sh` instead.

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
