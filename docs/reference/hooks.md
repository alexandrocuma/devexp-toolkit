# Hooks Reference

## How Hooks Work

Hooks intercept tool calls automatically — no user action required. Some are safety guards that block or ask; others (`lint-on-save`, `format-on-save`, `test-on-save`, `graphify-grep-nudge`) are advisory and never block. Each hook has an implementation per CLI, and the installer installs both: the Claude Code scripts (`cli/cmd/install_claude.go`) and the opencode plugin (`cli/cmd/install_opencode.go`).

**Claude Code** hooks are shell scripts registered in `~/.claude/settings.json` under `PreToolUse` or `PostToolUse` events. Claude Code calls the script with a JSON payload on stdin and reads the response:

- **Hard block** — print reason to stderr, `exit 2`. Claude stops the tool call entirely.
- **Soft block (ask)** — output `{"hookSpecificOutput": {"permissionDecision": "ask"}}` to stdout, `exit 0`. Claude pauses and asks the user.
- **Allow** — `exit 0` with no output.

**opencode** hooks are JS modules composed into a single plugin. The installer copies `hooks/opencode/devexp-plugin.js` to `~/.config/opencode/plugins/devexp.js`, the only file opencode loads, and puts the selected modules, `utils.js`, `package.json` and the `hooks.json` selection in `plugins/devexp/`, a subdirectory opencode's loader never scans (`cli/internal/hooks/opencode.go`). Handlers receive `(input, output)` and:

- **Block** — `throw new Error("reason")`. opencode stops the tool call.
- **Allow** — return without throwing.

---

## File Structure

```
hooks/
  registry.json               # Source of truth — one entry per hook
  claude-code/                # One .sh file per hook + tests
  └── secret-guard.sh
  └── secret-in-write-guard.sh
  └── dangerous-cmd-guard.sh
  └── large-file-guard.sh
  └── lint-on-save.sh
  └── format-on-save.sh
  └── test-on-save.sh
  └── graphify-read-guard.sh
  └── graphify-session-sentinel.sh
  └── graphify-grep-nudge.sh
  └── secret-guard.test.sh     # Hook tests (*.test.sh), run by CI
  └── dangerous-cmd-guard.test.sh
  └── fail-closed.test.sh      # Guards fail closed / advisory hooks fail open but loud
  opencode/                   # One .js module per hook + shared utils + entry point + tests
  └── utils.js                # Shared helpers: findRoot, which, runLinter, runCommand (async spawn), countLines
  └── secret-guard.js
  └── secret-in-write-guard.js
  └── dangerous-cmd-guard.js
  └── large-file-guard.js
  └── lint-on-save.js
  └── format-on-save.js
  └── test-on-save.js
  └── graphify-read-guard.js
  └── graphify-session-sentinel.js
  └── graphify-grep-nudge.js
  └── secret-guard.test.js     # Hook tests (*.test.js), run by CI
  └── dangerous-cmd-guard.test.js
  └── devexp-plugin.js        # Entry point — composes the modules listed in devexp/hooks.json
  └── devexp-plugin.test.js   # Entry tests: selection, failure isolation, fail-closed stubs, event adapter
  └── package.json            # { "type": "module" } — required for ESM
```

---

## Registry Format

`hooks/registry.json` is the source of truth. Each entry:

```json
{
  "name": "hook-name",
  "description": "What this hook does",
  "claude_code": {
    "event":   "PreToolUse",
    "matcher": "Bash",
    "script":  "hooks/claude-code/hook-name.sh"
  },
  "opencode": {
    "event":       "tool.execute.before",
    "module":      "hooks/opencode/hook-name.js",
    "export":      "hookName",
    "fail_closed": true
  },
  "enabled": true
}
```

| Field | Description |
|-------|-------------|
| `name` | Unique hook identifier — used in `devexp.config.json` `hooks.disabled` list |
| `claude_code.event` | `PreToolUse` or `PostToolUse` |
| `claude_code.matcher` | Tool-name matcher (e.g. `"Bash"`, `"Write\|Edit"`). A plain `\|`-separated list matches those exact names, so `Edit` doesn't match `MultiEdit` or `NotebookEdit`; a pattern with other regex characters is an unanchored regex |
| `claude_code.script` | Path to the shell script, relative to repo root |
| `opencode.event` | `tool.execute.before` or `file.edited` |
| `opencode.module` | Path to the JS module, relative to repo root (`hooks/opencode/<hook-name>.js`) |
| `opencode.export` | Name of the module's factory export — the lowerCamel hook name (`hookName`) |
| `opencode.fail_closed` | `true` for security guards: if the module fails to import or initialise, the plugin blocks every tool call instead of running without it. Omit for advisory hooks |
| `opencode.enabled` | Optional opencode-only override of `enabled`; absent (nil) = follow `enabled`. The `graphify-*` hooks set `true` |
| `enabled` | Set to `false` to skip this hook for all users (targets without their own `enabled` override) |

Every key other than `name`, `description` and `enabled` whose value is an object is an **install-target block**, keyed by target id: `claude_code` for Claude Code, `opencode` for opencode. The Go installer parses them all into `Hook.Targets` (`map[string]hooks.TargetSpec` in `cli/internal/hooks/installer.go`), so a new target is a new sibling block, not a new Go type. Target blocks share one field vocabulary: `event`, `matcher` (Claude Code), `script` (command-based targets), `module` + `export` (JS-plugin targets), `fail_closed`, and the optional per-target `enabled`. The Claude Code install reads `claude_code` and the top-level `enabled`.

---

## Hook Catalog

| Hook | Event | Matcher | What it does |
|------|-------|---------|--------------|
| `secret-guard` | PreToolUse | `Read\|Bash` | Hard-blocks reads of `.env*`, `.pem`, `.key`, private key files |
| `secret-in-write-guard` | PreToolUse | `Write\|Edit\|MultiEdit\|NotebookEdit` | Hard-blocks writing content that contains secret patterns (API keys, GitHub tokens, private key blocks); in opencode it scans `write`, `edit` and the lines `apply_patch` adds |
| `dangerous-cmd-guard` | PreToolUse | `Bash` | Hard-blocks `rm -rf /`, unanchored wildcard deletes in sensitive dirs (`/tmp/*`, `~/.claude/.../*`), fork bombs, `DROP DATABASE`, `git push --force`, `git reset --hard`, `git clean`, `DROP/TRUNCATE TABLE` |
| `large-file-guard` | PreToolUse | `Write` | Asks for confirmation before overwriting a file with >500 lines |
| `lint-on-save` | PostToolUse | `Write\|Edit` | Runs the project linter on edited source files (JS/TS → biome/eslint, Python → ruff/flake8, Go → go vet, Ruby → rubocop) |
| `format-on-save` | PostToolUse | `Write\|Edit` | Runs the project formatter in-place (JS/TS → biome/prettier, Python → ruff/black, Go → gofmt, Ruby → rubocop). In opencode it rewrites the file after the edit tool computed its diff, so the reported diff can differ from the file on disk |
| `test-on-save` | PostToolUse | `Write\|Edit` | Runs the associated test file after editing a source file — skips silently if no test file found |
| `graphify-read-guard` *(disabled)* | PreToolUse | `Read\|Glob` | Gates source reads/globs behind a tapering `graphify query` cadence (5 → 3 → 1 queries to unlock, ~6 reads per cycle) — pairs with the [`graphify`](../../skills/graphify/SKILL.md) skill |
| `graphify-session-sentinel` *(disabled)* | PostToolUse | `Bash` | Tracks `graphify query/path/explain` usage toward `graphify-read-guard`'s tapering gate |
| `graphify-grep-nudge` *(disabled)* | PreToolUse | `Bash\|Grep` | Soft-nudges toward `graphify query` (via `additionalContext`, never a block) when grep-like commands or the `Grep` tool run |

The three `graphify-*` hooks ship with `enabled: false` — they're an **optional set** for projects that adopt the `graphify` skill and maintain a `graphify-out/` knowledge graph. All three self-gate on `graphify-out/graph.json` existing, so flipping them on is harmless even if a project hasn't built a graph yet (they simply no-op). Enable them in a fork by setting `"enabled": true` in `hooks/registry.json`. `devexp.config.json` can't enable them: it supports only `hooks.disabled` (`cli/internal/config/config.go`), and the installer skips any hook with `enabled: false` before it looks at config (`cli/internal/hooks/installer.go`). The install wizard lists only enabled hooks (`listHookNames` in `cli/cmd/registry.go`).

**What `secret-in-write-guard` doesn't see** — it scans only the new text of a write, edit or patch, never the file text around it. A secret completed across text already in the file and an edit isn't seen. In opencode, `apply_patch` context lines are matched against the file loosely and written from the patch's own text, and those lines aren't scanned. The guard catches a secret written in one piece; it doesn't replace a secret scanner on the repository.

**How `graphify-read-guard` paces itself** — rather than a flat "queried in the last N hours" timer (which re-arms mid-session and creates friction, or "gate once" which under-uses the graph), it runs a tapering cadence sourced from a small JSON state file (`graphify-out/.graphify_session`, shared with `graphify-session-sentinel`):

1. **Fresh session** → blocks Read/Glob until `graphify query` has run **5** times
2. Unlocks a budget of **~6** reads
3. Budget exhausted → re-arms, but now needs only **3** queries to unlock
4. Subsequent re-arms taper to a steady-state floor of **1** query per ~6-read cycle

This front-loads grounding when the agent knows least about the codebase, and eases off once it's shown sustained engagement with the graph — without ever resetting on a wall-clock timer or permanently locking out repo reads. `graphify-grep-nudge` covers the gap for `grep`/`rg`/`find`/etc. (Bash) and the built-in `Grep` tool — since those are often legitimately faster for precise lookups, it only nudges via `additionalContext`, it never blocks.

---

## CLI Compatibility

| | Claude Code | opencode |
|---|---|---|
| Hook scripts | `hooks/claude-code/*.sh` (one per hook) | `hooks/opencode/*.js` (one module per hook) |
| Entry point | Each script registered separately in `settings.json` | `devexp-plugin.js` composes the modules listed in `devexp/hooks.json`; file events arrive through `event` → `file.edited` |
| Installed by `./install.sh` | Yes — enabled hooks, into `~/.claude/settings.json` | Yes — selected modules, into `~/.config/opencode/plugins/` (`devexp.js` + `devexp/`) |
| Selection | Top-level `enabled`, minus `hooks.disabled` / wizard deselection | `opencode.enabled` if set, else `enabled`, minus `hooks.disabled` / wizard deselection — so the `graphify-*` hooks are on (turn them off with `hooks.disabled`) |
| Hook disabled after install | Stays registered in `settings.json` | Removed on the next install |
| Block mechanism | `exit 2` + stderr | `throw new Error(...)` |
| Confirm/ask | `permissionDecision: "ask"` JSON output | Not supported — hard block instead |

---

## Adding a New Hook

1. Create `hooks/claude-code/<hook-name>.sh`:
   ```bash
   #!/usr/bin/env bash
   set -euo pipefail
   input=$(cat)
   # hard block: echo "reason" >&2; exit 2
   # soft block: python3 -I -c "import json; print(json.dumps({'hookSpecificOutput': {'permissionDecision': 'ask'}}))"
   # tool values go in as data, never into program text — see docs/development/hook-authoring-guide.md
   exit 0
   ```

2. Create `hooks/opencode/<hook-name>.js`:
   ```js
   export async function myHookName(_ctx) {
     return {
       'tool.execute.before': async (input, output) => {
         // throw new Error('reason') to block
       },
     };
   }
   ```

3. Add the entry to `hooks/registry.json`, including the opencode mapping — `opencode.module`, `opencode.export`, and `opencode.fail_closed: true` for security guards. `devexp-plugin.js` is never edited per hook.

4. Add mirrored tests (`<hook-name>.test.sh` / `<hook-name>.test.js`), a `check` line in `hooks/claude-code/fail-closed.test.sh`, and update this catalog, the file tree above and the hook counts — see [workflows → Add a hook](../guides/workflows.md#add-a-hook).

5. `chmod +x hooks/claude-code/<hook-name>.sh` and run `./install.sh`.

Full guide: [`docs/development/hook-authoring-guide.md`](../development/hook-authoring-guide.md)
