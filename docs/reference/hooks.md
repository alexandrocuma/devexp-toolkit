# Hooks Reference

## How Hooks Work

Hooks are safety and quality guards that intercept tool calls automatically — no user action required. They are implemented differently per CLI but behave identically from the user's perspective.

**Claude Code** hooks are shell scripts registered in `~/.claude/settings.json` under `PreToolUse` or `PostToolUse` events. Claude Code calls the script with a JSON payload on stdin and reads the response:

- **Hard block** — print reason to stderr, `exit 2`. Claude stops the tool call entirely.
- **Soft block (ask)** — output `{"hookSpecificOutput": {"permissionDecision": "ask"}}` to stdout, `exit 0`. Claude pauses and asks the user.
- **Allow** — `exit 0` with no output.

**opencode** hooks are JS modules composed into a single plugin (`devexp-plugin.js`) registered in `~/.config/opencode/config.json`. Handlers receive `(input, output)` and:

- **Block** — `throw new Error("reason")`. opencode stops the tool call.
- **Allow** — return without throwing.

---

## File Structure

```
hooks/
  registry.json               # Source of truth — one entry per hook
  claude-code/                # One .sh file per hook
  └── secret-guard.sh
  └── secret-in-write-guard.sh
  └── dangerous-cmd-guard.sh
  └── large-file-guard.sh
  └── lint-on-save.sh
  └── format-on-save.sh
  opencode/                   # One .js module per hook + shared utils + entry point
  └── utils.js                # Shared helpers: findRoot, which, runLinter, countLines
  └── secret-guard.js
  └── secret-in-write-guard.js
  └── dangerous-cmd-guard.js
  └── large-file-guard.js
  └── lint-on-save.js
  └── format-on-save.js
  └── devexp-plugin.js        # Composes all modules into a single plugin export
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
    "event":  "tool.execute.before",
    "plugin": "hooks/opencode/devexp-plugin.js"
  },
  "enabled": true
}
```

| Field | Description |
|-------|-------------|
| `name` | Unique hook identifier — used in `devexp.config.json` `hooks.disabled` list |
| `claude_code.event` | `PreToolUse` or `PostToolUse` |
| `claude_code.matcher` | Regex matched against tool name (e.g. `"Bash"`, `"Write\|Edit"`) |
| `claude_code.script` | Path to the shell script, relative to repo root |
| `opencode.event` | `tool.execute.before` or `file.edited` |
| `opencode.plugin` | Always `hooks/opencode/devexp-plugin.js` — the single entry point |
| `enabled` | Set to `false` to skip this hook for all users |

---

## Hook Catalog

| Hook | Event | Matcher | What it does |
|------|-------|---------|--------------|
| `secret-guard` | PreToolUse | `Read\|Bash` | Hard-blocks reads of `.env*`, `.pem`, `.key`, private key files |
| `secret-in-write-guard` | PreToolUse | `Write\|Edit` | Hard-blocks writing content that contains secret patterns (API keys, GitHub tokens, private key blocks) |
| `dangerous-cmd-guard` | PreToolUse | `Bash` | Hard-blocks `rm -rf /`, fork bombs, `DROP DATABASE`, `git push --force`, `git reset --hard`, `git clean`, `DROP/TRUNCATE TABLE` |
| `large-file-guard` | PreToolUse | `Write` | Asks for confirmation before overwriting a file with >500 lines |
| `lint-on-save` | PostToolUse | `Write\|Edit` | Runs the project linter on edited source files (JS/TS → biome/eslint, Python → ruff/flake8, Go → go vet, Ruby → rubocop) |
| `format-on-save` | PostToolUse | `Write\|Edit` | Runs the project formatter in-place (JS/TS → biome/prettier, Python → ruff/black, Go → gofmt, Ruby → rubocop) |
| `test-on-save` | PostToolUse | `Write\|Edit` | Runs the associated test file after editing a source file — skips silently if no test file found |
| `graphify-read-guard` *(disabled)* | PreToolUse | `Read\|Glob` | Gates source reads/globs behind a tapering `graphify query` cadence (5 → 3 → 1 queries to unlock, ~6 reads per cycle) — pairs with the [`graphify`](../../skills/graphify/SKILL.md) skill |
| `graphify-session-sentinel` *(disabled)* | PostToolUse | `Bash` | Tracks `graphify query/path/explain` usage toward `graphify-read-guard`'s tapering gate |
| `graphify-grep-nudge` *(disabled)* | PreToolUse | `Bash\|Grep` | Soft-nudges toward `graphify query` (via `additionalContext`, never a block) when grep-like commands or the `Grep` tool run |

The three `graphify-*` hooks ship with `enabled: false` — they're an **optional set** for projects that adopt the `graphify` skill and maintain a `graphify-out/` knowledge graph. All three self-gate on `graphify-out/graph.json` existing, so flipping them on is harmless even if a project hasn't built a graph yet (they simply no-op). Enable them in a fork by setting `"enabled": true` in `hooks/registry.json`, or override per-org via `devexp.config.json`.

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
| Entry point | Each script registered separately in `settings.json` | Single `devexp-plugin.js` registered in `config.json` |
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
   # soft block: python3 -c "print(json.dumps({'hookSpecificOutput': {'permissionDecision': 'ask'}}))"
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

3. Register the module in `hooks/opencode/devexp-plugin.js` — import and add to `Promise.all([...])`.

4. Add the entry to `hooks/registry.json`.

5. `chmod +x hooks/claude-code/<hook-name>.sh` and run `./install.sh`.

Full guide: [`docs/development/hook-authoring-guide.md`](../development/hook-authoring-guide.md)
