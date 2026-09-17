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

> **How it finds its assets:** `devexp` never uses a directory it finds on disk — not the one the binary is in, and not the one you run it from. A release binary uses only `DEVEXP_DIR` or the assets bundled in it. A binary built from a clone (what `./install.sh` builds into `bin/devexp`, or `go run`) uses `DEVEXP_DIR` or the clone it was compiled from, wherever it is run or copied to, and reads agents/skills/hooks/MCPs live from there — so local edits never need a rebuild. A directory counts as a devexp-toolkit checkout only with the `.devexp-toolkit` marker file at its root plus `agents/`, `skills/` and `mcps/`; the marker identifies a checkout, while the rules above decide which one may be used. It uses that clone only while it can be verified as yours: on macOS and Linux the clone, and every directory and file under its `agents/`, `skills/`, `mcps/` and `hooks/` (plus `.devexp-toolkit`, `devexp.config.json` and `uninstall.sh`), must be owned by you, must not be symlinks, and must not be writable by others. Group write is accepted only when the group is your own private group (your primary group, named like your user), which covers clones made with the umask `002` default of distributions that give each user a private group. The directory holding the clone must be owned by you or root and follow the same write rule, unless it is sticky like `/tmp`. If the clone a binary was built from lacks the marker (pull to get it), has moved (rebuild with `rm bin/devexp && ./install.sh`), or can't be verified as yours (fix what the warning names — e.g. `chmod -R o-w` the clone, and `g-w` if its group is shared — or set `DEVEXP_DIR`), `devexp install` warns and uses the bundled assets. Before installing anything, it prints `Asset root: <dir> (<how it was chosen>)`. Agents and skills are *copied* into the CLI's config directory, so re-run the installer to pick up edits to them; Claude Code hooks are registered by absolute path into the repo, so edits to a registered hook script apply immediately. A standalone downloaded binary instead uses the copies baked in at release time, extracted to `<user cache dir>/devexp/assets` and re-extracted when the binary's version changes. A dev build that falls back to its bundled assets uses `<user cache dir>/devexp/assets-dev` instead and re-extracts on every run. Each extraction goes into a fresh directory that replaces the previous one atomically, so an interrupted or concurrent run never leaves hooks pointing at a partial copy. The previous copy is kept for an hour, so hooks that started just before the switch finish normally, and a later run removes it (`cli/internal/repo/repo.go`, `extractEmbedded`, `sweepStale`). With no absolute user cache dir (`HOME`, or `XDG_CACHE_HOME` on Linux, unset or relative) it refuses to extract rather than fall back to a temp directory. When `DEVEXP_DIR` or a dev build's own checkout qualifies, it wins over the embedded copies.

---

## install.sh (from a clone)

If you're contributing to the toolkit — editing agents, skills, or hooks — clone the repo and use `install.sh`. `install.sh` is now a thin wrapper: if `bin/devexp` doesn't exist yet it stages the embedded assets and builds the `devexp` Go CLI from `cli/` (requires a local Go toolchain), then execs `devexp install` with whatever flags you pass through (`install.sh:7-22`). Because `devexp` prefers live files on disk over its embedded copies, asset edits never need a rebuild — only changes to the Go code under `cli/` do (see [Updating](#updating)).

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
- **opencode**: transforms agent frontmatter (model aliases, tool mapping, adds `mode: subagent`) and installs to `~/.config/opencode/agents/`; each skill's `SKILL.md` goes to `~/.config/opencode/commands/<name>.md`; MCPs are written to the `mcp` key of `~/.config/opencode/config.json` (a `config.json` that isn't strict JSON — including one with comments or trailing commas, which opencode itself accepts — or whose top level or `mcp` isn't an object is left untouched: the MCP step is skipped with a warning naming the file and the servers to add by hand, and agents, skills and hooks still install. With `--mcps-only` it is an error); the hook plugin goes to `~/.config/opencode/plugins/` — the entry `devexp.js` plus `devexp/` holding the selected modules, `utils.js`, `package.json` and the `hooks.json` selection (`cli/cmd/install_opencode.go`, `cli/internal/hooks/opencode.go`). With every hook disabled no plugin is installed
- Backs up existing agents and skills before overwriting — **Claude Code target only**; the opencode install has no backup step (`backupExisting` / `backupExistingDirs` are called only from `cli/cmd/install_claude.go`)
- The install script is **idempotent** — safe to run multiple times

---

## Updating

Re-running the installer is how you update devexp — there's no separate "upgrade" command.

- **Binary install**: re-run the `remote-install.sh` one-liner from [Quick install](#quick-install-no-clone). It downloads the latest release binary, overwrites `~/.local/bin/devexp`, and runs `devexp install` again.
- **Clone install**: `git pull && ./install.sh` re-runs `devexp install` against the updated assets, which are read live from the clone (`cli/internal/repo/repo.go`). It does **not** rebuild the CLI — `install.sh` builds only when `bin/devexp` is missing (`install.sh:7`). If the pull changed Go code under `cli/`, rebuild explicitly: `git pull && rm bin/devexp && ./install.sh`. Updating to the release that installs the opencode hook plugin (#108) needs this rebuild.

### What gets overwritten vs. preserved

- **Agents and skills** are overwritten with the versions shipped in the new release, each file atomically. An installed agent file, opencode command, skill directory, or file or directory inside a skill that is a **symlink** (for example to your own customised copy in a dotfiles repo) is left untouched, with a warning: devexp never writes through it or replaces it. Its name stays in the manifest. Replace the link with a regular file to get the release's copy (#124). Before overwriting, the Claude Code install backs up your existing `~/.claude/agents/*.md` and `~/.claude/skills/<name>/` directories into a timestamped `~/.claude/.devexp-backup-<timestamp>/` folder (`cli/cmd/backup.go`, `cli/cmd/paths.go`). The opencode install makes **no backup** of `~/.config/opencode/agents/` or `commands/` (`cli/cmd/install_opencode.go`).
- **MCP server registrations** are *not* refreshed automatically — pass `--reinstall-mcps` if an MCP's config (command, args, env) changed in the new release.
- **Hooks**:
  - Claude Code: new hooks in `hooks/registry.json` are added to `settings.json`; hooks already registered are left as-is, including ones disabled since. There are two exceptions, the matcher and the command string. Install enforces the registry's matcher on devexp's own handlers (`registerHook`). An enabled hook registered under any other matcher is brought to the registry's, whether the registry changed it since (as when `secret-guard` went from `Read` to `Read|Bash`), you changed it by hand, or its entry has no `matcher` at all (which matches every tool). An entry holding only that hook's command takes the new matcher in place and keeps its other fields; a `matcher` key it didn't have is written before `hooks`. From an entry it shares with other commands, the handler moves out, fields and all, into an entry of its own, so the other commands keep their matcher. A hook already registered under the registry's matcher is left alone, even if its command also appears in another entry, and a disabled hook keeps whatever matcher it has. **To run a devexp hook under a matcher of your own**, disable it (`hooks.disabled` in `devexp.config.json`), remove devexp's entry for it from `settings.json` (install leaves a disabled hook's entry in place), and register your own command for it, for example a wrapper script or the script with `"args": []`: devexp treats either as yours. Another spelling of one of this repo/cache dir's own scripts is rewritten in place to the form devexp writes, which is quoted only when the path needs it. That covers the bare path, the path in double quotes and a single-quoted path that needs no quoting. If the event already holds the registered command, the other spelling is removed instead, so the script doesn't run twice (see [Hook commands](#hook-commands-claude-code)). Nothing else in `settings.json` changes (see [What install and uninstall change in settings.json](#what-install-and-uninstall-change-in-settingsjson)).
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
- **Hooks (Claude Code)**: devexp checks every registered hook command that runs a script directly in the devexp repo/cache directory's `hooks/claude-code/`, in a form devexp writes: the plain path, the path as one single-quoted word, or the bare path an earlier install from that directory wrote before quoting (#135). If that script no longer exists (because the hook was removed from `hooks/registry.json`), the dangling entry is removed from `settings.json` (`isStaleDevexpHook`). The same goes for a command in that form naming a missing script directly under `<root>/hooks/claude-code/` of **another** devexp install root — a checkout or asset cache you installed from earlier — recognised by the root's own `hooks/registry.json` (a non-empty JSON array of hooks, each with a `name`), since a removed script's name is in no registry (`isOrphanedDevexpHook`, #150). A directory without such a registry is the user's, so a registration whose whole root has been deleted can't be recognised and is left alone. Any other command under that directory is the user's and stays even when what it names doesn't exist: one with arguments (`<dir>/hooks/x.sh --flag`), in another directory (`<dir>/bin/tool`, `<dir>/hooks/claude-code/sub/x.sh`), double-quoted, chained, or using a variable (#138). A devexp hook registered from a *different* install root (e.g. a release-binary install later replaced by a clone install) is also removed as a duplicate, matched by registry script name directly under a `hooks/claude-code/` path segment (`pruneForeignDevexpHooks` in `cli/internal/hooks/installer.go`); so is a devexp entry registered as a relative path by an earlier install. Only the two command forms devexp writes count (see [Hook commands](#hook-commands-claude-code)): a plain path, or exactly one single-quoted absolute path. A command with arguments, variables (`$CLAUDE_PROJECT_DIR/…`), `~`, double quotes, other quoting or other shell syntax, or one under a directory like `my-hooks/claude-code/`, is the user's (`commandPath` and `isManagedScriptPath`; `uninstall.sh` uses the same rules). Other user-authored hooks are never touched.

### Behavior change: disabling now removes

Previously, disabling an agent or skill in `devexp.config.json` only skipped *updating* it — the old copy stayed on disk. Now a disabled agent/skill is excluded from the install set entirely, so it's treated as stale and **removed** on the next run. If you need a copy, recover it from the backup directory described above.

### Hook commands (Claude Code)

Claude Code runs a hook's `command` through a shell (`sh -c` on macOS and Linux). devexp registers each hook as the absolute path of its script in the repo or asset-cache dir (`hookCommand` in `cli/internal/hooks/installer.go`):

- A path with no shell syntax is registered as-is, exactly as earlier releases wrote it.
- A path with whitespace or any of `` $ ~ ' " ` \ ; & | < > ( ) * ? [ ] { } ! # `` (a clone under `~/My Projects/`, say) is registered as one POSIX single-quoted word, each `'` written as `'\''`. The shell runs that path and nothing else, with no expansion.

Before #135 such a path was registered unquoted. The shell split or expanded it, so the hook didn't run (Claude Code treats that as a non-blocking error, so a guard failed open) or a different program ran. On the next `devexp install` from the same repo/cache dir, each registration of one of that dir's registry scripts is brought to the registered form, disabled hooks included (`requoteDevexpHooks`). Only these exact spellings of the script's path count:

- the bare path, as earlier releases wrote it;
- the path in double quotes (`"<dir>/hooks/claude-code/<script>"`, the natural hand fix), only when the path has no `$`, backquote, `\` or `"`, so the double quotes are literal;
- a single-quoted path that needs no quoting, which becomes the plain path.

Each is rewritten in place (event, matcher and position kept). If the event already holds the registered command, for example after an older devexp re-added the bare path, the spelling is removed instead, and an entry left with no commands goes too, so the script never runs twice. `uninstall.sh` from that dir removes the bare and double-quoted spellings along with the registered forms.

Everything else stays the user's. That includes a double-quoted path holding `$`, backquote or `\` (the shell expands those inside double quotes), arguments, other directories and other scripts. An unquoted entry from a *different* root can't be told apart from a user command that takes arguments, so it is left alone. An install or uninstall from its own root fixes it.

### What install and uninstall change in settings.json

devexp owns only the hook handlers it registers: `{"type": "command", "command": "<script path>"}`, in an entry with the hook's `matcher`. Install and `uninstall.sh` add, re-quote or remove those and nothing else (`cli/internal/hooks/settings.go`, the python step in `uninstall.sh`):

- **Other handlers and entries keep every field** Claude Code reads, whether devexp knows it or not: `timeout`, `async`, `asyncRewake`, `shell`, `if`, `statusMessage`, `once`, an `http` hook's `url`, `headers` and `allowedEnvVars`, an `mcp_tool` or `prompt` hook's own fields, and any field a later Claude Code adds. Their fields, key order and values are kept, and a user's entry without a `matcher` doesn't gain one (devexp's own entries do: see [What gets overwritten vs. preserved](#what-gets-overwritten-vs-preserved)). When devexp re-quotes one of its own commands, only that `command` value changes; the handler's other fields stay.
- **A handler with `args`, or of a type other than `command`, is the user's**, whatever path it names. With `args`, Claude Code spawns `command` directly without a shell, so devexp never matches, re-quotes, prunes or removes it, and doesn't count it as the hook's registration.
- **Removing devexp's handlers** drops an entry only when that leaves it with no handlers, and an event only when that leaves it with no entries. An entry with `"hooks": []` or an event with `[]` that was already empty stays.
- **Only the value of the top-level `hooks` key is rewritten.** Outside it, every byte of the file is kept: other keys, their formatting and order, line endings (LF or CRLF) and the trailing newline. Inside it, what is kept is content, not bytes: every handler's and entry's fields, their order and their values. When devexp writes, the whole `hooks` value is written anew:
  - it is re-indented to the indentation of the line its key is on (two spaces, four, tabs), or compact in a compact file, so a handler you wrote on one line is spread over several;
  - its lines end like the file's first line (CRLF or LF);
  - keys are re-encoded, so an escaped key such as `"comm\u0061nd"` becomes `"command"`;
  - install keeps string values' escapes and number spellings as they were (including numbers a float can't hold, such as `1e400`); `uninstall.sh` writes them as python's `json` does (`\u00e9` becomes `é`, `2.50` becomes `2.5`).

  A file without `hooks` gains the key after its last member; a new file is written as earlier releases wrote it. Events devexp adds go after the existing ones, sorted. Nothing is written when there's nothing to change. The file is saved atomically, and a symlinked `settings.json` keeps its link (see [How devexp saves files](#how-devexp-saves-files)).
- **A `settings.json` devexp can't edit without losing something is left untouched.** That's invalid JSON, a top level that isn't an object, a `hooks` that isn't an object, an event that isn't an array, or an entry or handler that isn't an object. `devexp install` stops with an error naming the file, and no hooks are registered, where earlier releases replaced the whole file with just the hooks. `uninstall.sh` skips the step with a message, and also for a file holding `NaN`, `Infinity` or a number a float can't hold (python would write `1e400` back as `Infinity`, which isn't JSON), or nested more deeply than python's `json` can read (around 1,000 levels; #150). Both also check that the rewritten file decodes to the original with only `hooks` changed, and write nothing otherwise.

### How devexp saves files

Every file devexp edits or installs is saved the same way (`WriteFileAtomic` in `cli/internal/fsutil/atomic.go`; `write_atomic` in both python steps of `uninstall.sh`, #124): `settings.json`, the opencode `config.json`, both `.devexp-manifest.json` files, the opencode plugin files, and agent, skill and command files.

- **Atomic.** The new contents go to a temp file in the same directory as the file being replaced, which is fsynced, given that file's permission bits and renamed over it. A new file gets its default mode (0644) minus your umask, as a plain write would, so under `umask 077` a new `config.json` holding MCP tokens is 0600. An interrupted or failed save leaves the old file or the new one, never a partial one, and removes the temp file.
- **Symlinks.** A symlinked `settings.json`, `config.json` or manifest (dotfiles) is followed to the file it finally points at, and that file is replaced; the link, and every link in a chain, stays a link. Earlier releases wrote through the link without the temp file, and a plain rename would have turned the link into a regular file.
- **Refused, file left as it is, with an error or warning naming it:**
  - a dangling link (nothing is created where it points);
  - a link loop;
  - a path, or link target, that is a directory or another non-regular file;
  - a file you can't write (a rename would still replace a read-only file in a writable directory);
  - a directory where no temp file can be created;
  - a rename the file system refuses, such as a bind-mounted file (`EXDEV`/`EBUSY`).
- **Where the symlink rule differs.** Some paths keep their earlier, stricter rules:
  - The steps that *remove* devexp's entries from opencode's `config.json` leave a symlinked `config.json` untouched, with a warning to edit it by hand. That covers the legacy plugin entry and `uninstall.sh`'s MCP step.
  - `devexp uninstall` does the same for a symlinked manifest.
  - An installed agent, command or skill entry that is itself a symlink is never written (see [What gets overwritten vs. preserved](#what-gets-overwritten-vs-preserved)).
  - Plugin files must not be symlinks at all (see [Stale-file cleanup](#stale-file-cleanup)).
- **Replacing makes a new file**, so what belonged to the old one doesn't carry over:
  - ownership: the saved file belongs to the user running devexp;
  - a hard link: the other name keeps the old contents;
  - extended attributes and ACLs;
  - the setuid, setgid and sticky bits: only the permission bits are kept.
- The symlink checks and the write are separate steps, so a link created at the path between them is followed like any other. Only a program running as you can do that.

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
- **Claude Code hooks** are removed from `settings.json` by an embedded python step, with the install's rules: a registry script under a `hooks/claude-code/` path segment, as a plain path or one single-quoted absolute path, from any install root; a script in that form that is gone from `<root>/hooks/claude-code/` of any devexp install root (this repo or another, recognised by its `hooks/registry.json`, #150); plus this repo's own scripts spelled as the bare path or, when the path has no `$`, backquote, `\` or `"`, the path in double quotes ([Hook commands](#hook-commands-claude-code)). The file is saved atomically and through a symlink ([How devexp saves files](#how-devexp-saves-files)); a file it can't save, or one nested too deeply to read, is skipped with a message and the uninstall carries on. Everything else is left in place, with every field and every byte outside `hooks` ([What install and uninstall change in settings.json](#what-install-and-uninstall-change-in-settingsjson)).
- **opencode hook plugin**: an install with only the plugin (no agents) is detected too. The plugin (`plugins/devexp.js` + `plugins/devexp/`) and a legacy flat install are removed by the hidden `devexp uninstall --target opencode`, with exactly the rules of [Stale-file cleanup](#stale-file-cleanup): only devexp-owned files, nothing through a symlinked `plugins/`, a refusal (nothing removed) for a symlinked `devexp/` or a dangling `plugins/` link, all-or-nothing on `devexp.js` + `devexp/`, and the legacy `config.json` entry spliced out byte for byte. It runs before the MCP servers are removed from `config.json`.
  - The preview before the confirmation is that command's `--dry-run`.
  - It needs a `devexp` binary that has the command, looked up in this order: `DEVEXP_BIN` (set when you choose Remove in `devexp install`), `bin/devexp` in the clone, `devexp` on `PATH`. Without one, the plugin is left in place with a warning and the uninstall still completes. In a clone with an older binary, rebuild it: `rm bin/devexp && ./install.sh`.
  - Afterwards the opencode manifest's `plugins` key lists only the files that had to stay (for example behind a symlinked `plugins/`), so a later run can finish the job. A manifest that is missing, can't be read or is a symlink is left as it is.
  - The command refuses to run (nothing is removed) when `HOME` is unset, empty or not an absolute path.
- **opencode MCP servers** are removed from `config.json` by an embedded python step, which never fails the uninstall:
  - a malformed or unexpected `config.json` is skipped with a message;
  - a symlinked `config.json` is left untouched, with a warning to remove the servers by hand;
  - one nested too deeply for python's `json` to read is skipped with a message (#150);
  - one that can't be written (a read-only file or directory) is left as it was, with a warning, and the remaining steps still run;
  - otherwise only the removed servers' members are cut out of the `mcp` object, with the comma and whitespace that joined each one to a neighbour. Every other byte stays, including key order, indentation, CRLF line endings, escapes, number spellings and the other servers (#124). The edit is saved atomically, keeping the file's mode, and only when the result reads as the original minus those servers.
- If either python step fails in some other way, `uninstall.sh` prints a warning and carries on instead of stopping.

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
