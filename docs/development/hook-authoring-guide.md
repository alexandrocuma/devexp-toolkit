# Hook Authoring Guide

This guide covers everything you need to write a new devexp hook — from deciding what to guard to writing both the Claude Code script and the opencode module, and deploying it. The installer deploys both — see [How the System Is Structured](#how-the-system-is-structured).

---

## What Are Hooks?

Hooks intercept tool calls before or after they execute. They are the right tool for:

- **Safety guards** — hard-blocking irreversible or dangerous operations
- **Quality enforcement** — running linters, formatters, or validators automatically
- **Audit trails** — logging what commands were run

Hooks are **not** the right tool for: adding AI reasoning, calling external APIs, or anything that should be an agent or skill.

---

## How the System Is Structured

Each hook has two implementations — one per CLI — and a registry entry that ties them together:

```
hooks/
  registry.json                   # source of truth
  claude-code/<hook-name>.sh      # Claude Code implementation
  opencode/<hook-name>.js         # opencode implementation
  opencode/devexp-plugin.js       # entry point — composes the modules listed in devexp/hooks.json
  opencode/utils.js               # shared helpers
```

The installer reads `registry.json` and:
- Registers each enabled `.sh` script in `~/.claude/settings.json` with the correct event and matcher, by absolute path into the install root (`cli/internal/hooks/installer.go`)
- Installs the opencode plugin: copies `opencode/devexp-plugin.js` to `~/.config/opencode/plugins/devexp.js`, copies each selected hook's `opencode.module` plus `utils.js` and `package.json` into `plugins/devexp/`, and writes `plugins/devexp/hooks.json` (`cli/internal/hooks/opencode.go`). It copies by the registry list, so a module without an `opencode` mapping is never installed and `*.test.js` never ships.

---

## Registry Entry

Every hook must have an entry in `hooks/registry.json`:

```json
{
  "name": "my-guard",
  "description": "One-line description of what this hook guards",
  "claude_code": {
    "event":   "PreToolUse",
    "matcher": "Bash",
    "script":  "hooks/claude-code/my-guard.sh"
  },
  "opencode": {
    "event":       "tool.execute.before",
    "module":      "hooks/opencode/my-guard.js",
    "export":      "myGuard",
    "fail_closed": true
  },
  "enabled": true
}
```

### Event and matcher reference

| Use case | Claude Code event | Claude Code matcher | opencode event |
|----------|-------------------|---------------------|----------------|
| Before a shell command | `PreToolUse` | `Bash` | `tool.execute.before` |
| Before reading a file | `PreToolUse` | `Read` | `tool.execute.before` |
| Before writing a file | `PreToolUse` | `Write` | `tool.execute.before` |
| Before writing or editing | `PreToolUse` | `Write\|Edit` | `tool.execute.before` |
| After writing or editing | `PostToolUse` | `Write\|Edit` | `file.edited` |
| Before any tool | `PreToolUse` | `.*` | `tool.execute.before` |

The `matcher` is a regex matched against the tool name. Use precise matchers — they prevent your script from running on every single tool call.

---

## Writing the Claude Code Shell Script

Shell scripts receive a JSON payload on stdin and communicate back via exit code and stdout/stderr.

### Payload structure

```json
{
  "session_id": "...",
  "tool_name":  "Bash",
  "tool_input": { "command": "rm -rf node_modules" }
}
```

For `Read`: `tool_input.file_path`
For `Write`: `tool_input.file_path`, `tool_input.content`
For `Edit`: `tool_input.file_path`, `tool_input.old_string`, `tool_input.new_string`
For `Bash`: `tool_input.command`

### Treat tool input as data, never as code

Every value in `tool_input` — paths, commands, content — is untrusted. Never expand one into program text: not inside a `python3 -c "..."` string, not through `eval`, `bash -c` or an unquoted expansion. Pass it as data instead — on stdin, as an argument (`python3 -I - "$value" <<'PY'` with a **quoted** heredoc delimiter, then `sys.argv[1]`), or through the environment — and always quote shell expansions (`"$value"`). In opencode modules, spawn with `await runCommand(cmd, [args…], { cwd, timeout })` from `utils.js`: an argument array, no shell, never a synchronous spawn (see [Shared utilities](#shared-utilities-utilsjs)).

**Hand a tool the edited file as an absolute path.** Resolve a relative path first, against the input's `cwd` (Claude Code) or `ctx.directory` (opencode, `editedPath` in `utils.js`), else the process cwd. Tools run from the project root, and an absolute path is never read as an option. When a tool needs a relative value, pass it after `--`, and check how the tool reads it: jest treats a bare path as a regex, so the hook adds `--runTestsByPath` to match the exact file; ruff still expands `@argfile` after `--`; vitest drops file filters after it (#121).

Two more rules for the Python calls:

- **Always run the interpreter isolated: `python3 -I`.** Hooks run with the project as their working directory; isolated mode keeps the interpreter from importing anything from the project. It also ignores `PYTHON*` environment variables and the user's site-packages; hooks use only the standard library, so they lose nothing. Child processes still inherit the full environment, so project tools a hook launches (formatters, linters, test runners) find their configuration as before.
- **Keep paths byte-exact.** `$(...)` trims trailing newlines, which are legal in file names. When extracting a path, have Python write it with a one-character suffix and strip that suffix in bash (see the boilerplate below).

`hooks/claude-code/interpreter-isolation.test.sh` fails for any hook that starts `python3` without isolation, or that is not listed in it — add new hooks there.

### Response types

**Hard block** — tool call is cancelled, reason shown to Claude:
```bash
echo "[devexp my-guard] Blocked: reason here." >&2
exit 2
```

**Soft block (ask)** — Claude pauses and shows a confirmation prompt to the user. Values go in as arguments; the quoted `<<'PY'` delimiter stops bash expanding anything inside the script:
```bash
python3 -I - "$file_path" <<'PY' || { echo "[devexp my-guard] internal error -- could not build the prompt, skipping. The interpreter's error is above." >&2; exit 0; }
import json, sys
file_path = sys.argv[1]
print(json.dumps({
    'hookSpecificOutput': {
        'permissionDecision': 'ask',
        'permissionDecisionReason': '[devexp my-guard] About to change "' + file_path + '". Confirm this is intentional.'
    }
}))
PY
exit 0
```

**Allow** — proceed silently:
```bash
exit 0
```

**Advisory** (PostToolUse only — cannot block):
```bash
echo "[devexp my-guard] Note: something worth knowing." >&2
exit 0
```

### Boilerplate

```bash
#!/usr/bin/env bash
# devexp hook: my-guard
# Event: PreToolUse | Matcher: Bash
# Short description of what this hook guards.

set -euo pipefail

input=$(cat)

# The trailing "x" keeps $(...) from trimming newlines that belong to the value.
command=$(echo "$input" | python3 -I -c \
    "import sys,json; d=json.load(sys.stdin); sys.stdout.write(str(d.get('tool_input',{}).get('command','')) + 'x')") || {
    # A guard must fail CLOSED: an empty value would make every check pass.
    echo "[devexp my-guard] internal error -- the guard could not read its input, so it did not run. Blocking to be safe; the interpreter's error is above." >&2
    exit 2
}
command=${command%x}

# Guard logic here...

exit 0
```

If the interpreter fails, never swallow it (`2>/dev/null || echo ""` turns a crash into "nothing to block"). Follow the contract `hooks/claude-code/fail-closed.test.sh` enforces:

| Hook kind | On internal error |
|-----------|-------------------|
| Guard (blocks something) | `exit 2` and print `internal error` to stderr — fail closed |
| Advisory (asks, formats, lints, tests) | `exit 0` and print `internal error` to stderr — fail open, but loudly |

Add every new hook to `fail-closed.test.sh` with its kind.

Examples: `hooks/claude-code/secret-guard.sh` (guard) and `hooks/claude-code/lint-on-save.sh` (advisory); rationale in [conventions → Error Handling](conventions.md#error-handling).

---

## Writing the opencode JS Module

Each hook is an async function that returns an object of event handlers.

### Module structure

```js
/**
 * my-guard.js — short description
 *
 * Event: tool.execute.before (tool: bash)
 */

import { someHelper } from './utils.js';

export async function myGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'bash') return;

      const command = output.args?.command ?? '';
      if (!command) return;

      // Block:
      if (/dangerous-pattern/.test(command)) {
        throw new Error('[devexp my-guard] Blocked: reason.');
      }
    },
  };
}
```

### Event handler signatures

| Event | Signature | Notes |
|-------|-----------|-------|
| `tool.execute.before` | `async (input, output) => {}` | `input.tool` = tool name (lowercase), `output.args` = mutable args |
| `file.edited` | `async (event) => {}` | `event.file` = the edited path (delivered by the entry's `event` adapter; opencode sends it absolute, resolve it with `editedPath(event.file, ctx.directory)`); must never throw |

### Tool names in opencode (lowercase)

| Claude Code tool | opencode tool name |
|------------------|--------------------|
| `Bash` | `bash` |
| `Read` | `read` |
| `Write` | `write` |
| `Edit` | `edit` |

### Shared utilities (`utils.js`)

```js
import {
  findRoot, editedPath, which, runLinter, runCommand, countLines,
  existsSync, join, dirname, resolve, extname, basename,
  LINT_EXTS,
} from './utils.js';

findRoot(filePath)         // walks up to find package.json / go.mod / .git
editedPath(file, ctx.directory)  // absolute path of the edited file (relative → against ctx.directory)
await which('ruff')                     // resolves binary path or null
await runLinter(cmd, args, cwd)         // runs linter, prints output, swallows non-zero exit, 10s timeout
await runCommand(cmd, args, { cwd, timeout, output })
                                        // async spawn; never rejects; resolves { code, signal, stdout, stderr, error }
countLines(filePath)       // returns line count, 0 on error
LINT_EXTS                  // Set of lintable extensions: .js, .ts, .py, .go, .rb, etc.
```

**Never spawn synchronously** (`execFileSync`, `spawnSync`, `execSync`) in an opencode module. opencode runs plugins inside its server process, so a synchronous spawn freezes every session until the tool exits. Use `runCommand` (or `which` / `runLinter`) and `await` it.

Path helpers (`join`, `dirname`, `resolve`, `extname`, `basename`) are re-exported from Node's `path` module for convenience. `existsSync` is re-exported from `fs`.

---

## Registering in the opencode Entry Point

You never edit `hooks/opencode/devexp-plugin.js` to add a hook. The entry composes only the modules listed in the installed selection file, and that selection comes from the registry. Register the module by giving the hook its `opencode` mapping in `hooks/registry.json`:

```json
"opencode": {
  "event":       "tool.execute.before",
  "module":      "hooks/opencode/my-guard.js",
  "export":      "myGuard",
  "fail_closed": true
}
```

| Field | Meaning |
|-------|---------|
| `module` | The module file, relative to repo root, directly under `hooks/opencode/` |
| `export` | The factory's export name — the lowerCamel hook name |
| `fail_closed` | `true` for security guards. If the module fails to import or initialise, every tool call is blocked instead of running without the guard. Omit it for advisory hooks |
| `enabled` | Optional opencode-only override of the top-level `enabled` (the `graphify-*` hooks set `true`). Absent = follow `enabled` |

A module may import only `./utils.js` relatively — the installed plugin directory holds the hook modules, `utils.js` and `package.json`, nothing else (`hooks/opencode/devexp-plugin.test.js` checks this for every registry entry).

### The `devexp/hooks.json` contract

The installed plugin is `<plugins>/devexp.js` (the entry) next to `<plugins>/devexp/`, which holds `hooks.json`, `utils.js`, `package.json` and the selected modules. `hooks.json` is a **non-empty** JSON array in registry order, one object per selected hook with exactly the keys `name`, `module`, `export` and `failClosed` (note: camelCase here, `fail_closed` in the registry):

```json
[
  { "name": "secret-guard", "module": "secret-guard.js", "export": "secretGuard", "failClosed": true },
  { "name": "lint-on-save", "module": "lint-on-save.js", "export": "lintOnSave", "failClosed": false }
]
```

- `module` must match `^[A-Za-z0-9][A-Za-z0-9._-]*\.js$` and resolve to a `file:` URL inside `devexp/`. Anything else — a path, a `node:` builtin, encoded dot segments — is refused as a load failure.
- An empty array is **not** a valid selection. With every hook disabled the installer installs no plugin at all, so `[]` can only be an installer bug and is treated like an unreadable file.
- `hooks/opencode/devexp-plugin.test.js` parses the example above and asserts these exact key names, so it is the target the installer must write.

Writing this file is the installer's job (`opencodeSelectionJSON` in `cli/internal/hooks/opencode.go`); `TestOpencodeSelectionJSON_Contract` checks its output against the example above.

### Failure behaviour

- **`hooks.json` missing, invalid, not an array or empty** → every tool call is blocked with `[devexp] opencode plugin is misconfigured (devexp/hooks.json unreadable) — re-run devexp install. Blocking to be safe.`
- **An entry that is not an object with a non-empty string `name`** → every tool call is blocked (`… (devexp/hooks.json entry <i> is not a hook entry) …`); it is never skipped.
- **A module fails to import or initialise** (or its export is not a function, or `module` is refused):
  - if the entry is fail-closed → a stub blocks every tool call with `[devexp <name>] internal error — the guard failed to load, so it did not run. Blocking to be safe: <error>`. An entry is fail-closed when `failClosed: true`, when it carries the registry spelling `fail_closed: true`, **or** when its name is one of the security guards (`secret-guard`, `secret-in-write-guard`, `dangerous-cmd-guard`) — the entry hard-codes that set so a guard can't fail open because one flag was dropped or misspelled
  - without → `[devexp <name>] failed to load; skipped: <error>` is logged and the other hooks keep running
- **At runtime** `tool.execute.before` handlers run one after another in selection order; the first throw blocks the call.

### File events

opencode has no `file.edited` plugin hook: file events reach plugins only through the `event` hook. The entry adapts every `event` of type `file.edited` into a call to each module's `file.edited` handler with `{ file }` (the absolute path).

- The handlers are **queued, not awaited**: `event` returns at once, and the queue runs one edit at a time, calling the modules in selection order. opencode's event publisher is never held while a linter, formatter or test runner works.
- Errors from those handlers are logged, never rethrown.
- `format-on-save` rewrites the file in place **after** opencode's edit tool has already computed the diff it reports (and after opencode's own formatter ran), so the diff shown for that edit can differ from what ends up on disk.

---

## Deployment Checklist

- [ ] `hooks/claude-code/<hook-name>.sh` created with correct header comment
- [ ] `chmod +x hooks/claude-code/<hook-name>.sh`
- [ ] `hooks/opencode/<hook-name>.js` created
- [ ] Registry `opencode.module`/`export` set (`fail_closed` for guards)
- [ ] Entry added to `hooks/registry.json` with correct event, matcher, and paths
- [ ] Extraction fails closed (guard) or open-but-loud (advisory) — see [Failing on bad input](#boilerplate)
- [ ] Mirrored tests added: `hooks/claude-code/<hook-name>.test.sh` and `hooks/opencode/<hook-name>.test.js` (pattern: `secret-guard.test.sh` ↔ `secret-guard.test.js`)
- [ ] `check <hook-name> 2 guard` or `check <hook-name> 0 advisory` line added to `hooks/claude-code/fail-closed.test.sh`
- [ ] Hook catalog, file tree and counts updated (`docs/reference/hooks.md`, `hooks/README.md`, `README.md`, `CLAUDE.md`)
- [ ] `bash -n hooks/claude-code/<hook-name>.sh` passes
- [ ] `node --input-type=module` import test passes
- [ ] `./install.sh` installs without errors, and the hook appears in `~/.claude/settings.json` and in `~/.config/opencode/plugins/devexp/hooks.json`
- [ ] Tested both the block path and the allow path

Exact steps and the files each touches: [workflows → Add a hook](../guides/workflows.md#add-a-hook).

---

## Design Principles

**Be precise with matchers.** `matcher: "Bash"` runs only on Bash calls. `matcher: ".*"` runs on every tool call — avoid it unless truly necessary.

**Guards always hard-block.** All safety guards in this framework use hard blocks (`exit 2` / `throw`). This is intentional — soft blocks (ask) add friction without a real safety guarantee because the user can just approve them. The only legitimate use of soft block (ask) is for *reversible* operations where a brief confirmation is meaningful, like the large-file-guard which protects against accidentally targeting the wrong path.

**opencode has no "ask".** There is no soft-block mechanism in opencode — `throw` is always a hard block. Design guard logic so the error message is self-explanatory: tell the user what was blocked and what to do instead.

**PostToolUse / file.edited must never block.** These run after the tool completes — they are advisory only. In opencode `file.edited` handlers, wrap everything in try/catch and never throw.

**Silent on success.** Hooks that find nothing wrong should produce zero output. Don't print "all clear" messages — they create noise on every tool call.

**Keep it fast.** Hooks run synchronously before every tool call. Avoid network calls, heavy computation, or anything that could add noticeable latency. For PostToolUse linting/formatting, always set a timeout (10–15s).
