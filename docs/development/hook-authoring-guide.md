# Hook Authoring Guide

This guide covers everything you need to write a new devexp hook — from deciding what to guard to deploying it for both Claude Code and opencode.

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
  opencode/devexp-plugin.js       # entry point — composes all opencode modules
  opencode/utils.js               # shared helpers
```

The installer reads `registry.json` and:
- Registers each `.sh` script in `~/.claude/settings.json` with the correct event and matcher
- Copies `devexp-plugin.js` (and all imported modules) to `~/.config/opencode/plugins/`

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
    "event":  "tool.execute.before",
    "plugin": "hooks/opencode/devexp-plugin.js"
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

### Response types

**Hard block** — tool call is cancelled, reason shown to Claude:
```bash
echo "[devexp my-guard] Blocked: reason here." >&2
exit 2
```

**Soft block (ask)** — Claude pauses and shows a confirmation prompt to the user:
```bash
python3 -c "
import json
print(json.dumps({
    'hookSpecificOutput': {
        'permissionDecision': 'ask',
        'permissionDecisionReason': '[devexp my-guard] Reason. Confirm this is intentional.'
    }
}))"
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

command=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('command',''))" \
    2>/dev/null || echo "")

# Guard logic here...

exit 0
```

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
| `file.edited` | `async (event) => {}` | `event.path` = file path; must never throw |

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
  findRoot, which, runLinter, countLines,
  existsSync, join, dirname, resolve, extname, basename,
  LINT_EXTS,
} from './utils.js';

findRoot(filePath)         // walks up to find package.json / go.mod / .git
which('ruff')              // returns binary path or null
runLinter(cmd, args, cwd)  // runs linter, prints output, swallows non-zero exit, 10s timeout
countLines(filePath)       // returns line count, 0 on error
LINT_EXTS                  // Set of lintable extensions: .js, .ts, .py, .go, .rb, etc.
```

Path helpers (`join`, `dirname`, `resolve`, `extname`, `basename`) are re-exported from Node's `path` module for convenience. `existsSync` is re-exported from `fs`.

---

## Registering in the opencode Entry Point

After creating your module, add it to `hooks/opencode/devexp-plugin.js`:

```js
import { myGuard } from './my-guard.js';

export const DevExpPlugin = async (ctx) => {
  const modules = await Promise.all([
    secretGuard(ctx),
    secretInWriteGuard(ctx),
    dangerousCmdGuard(ctx),
    largeFileGuard(ctx),
    lintOnSave(ctx),
    formatOnSave(ctx),
    myGuard(ctx),           // ← add here
  ]);

  return {
    'tool.execute.before': async (input, output) => {
      for (const mod of modules) {
        if (mod['tool.execute.before']) {
          await mod['tool.execute.before'](input, output);
        }
      }
    },
    'file.edited': async (event) => {
      for (const mod of modules) {
        if (mod['file.edited']) {
          await mod['file.edited'](event);
        }
      }
    },
  };
};
```

---

## Deployment Checklist

- [ ] `hooks/claude-code/<hook-name>.sh` created with correct header comment
- [ ] `chmod +x hooks/claude-code/<hook-name>.sh`
- [ ] `hooks/opencode/<hook-name>.js` created
- [ ] Module imported and added to `devexp-plugin.js`
- [ ] Entry added to `hooks/registry.json` with correct event, matcher, and paths
- [ ] `bash -n hooks/claude-code/<hook-name>.sh` passes
- [ ] `node --input-type=module` import test passes
- [ ] `./install.sh` installs without errors
- [ ] Tested both the block path and the allow path

---

## Design Principles

**Be precise with matchers.** `matcher: "Bash"` runs only on Bash calls. `matcher: ".*"` runs on every tool call — avoid it unless truly necessary.

**Guards always hard-block.** All safety guards in this framework use hard blocks (`exit 2` / `throw`). This is intentional — soft blocks (ask) add friction without a real safety guarantee because the user can just approve them. The only legitimate use of soft block (ask) is for *reversible* operations where a brief confirmation is meaningful, like the large-file-guard which protects against accidentally targeting the wrong path.

**opencode has no "ask".** There is no soft-block mechanism in opencode — `throw` is always a hard block. Design guard logic so the error message is self-explanatory: tell the user what was blocked and what to do instead.

**PostToolUse / file.edited must never block.** These run after the tool completes — they are advisory only. In opencode `file.edited` handlers, wrap everything in try/catch and never throw.

**Silent on success.** Hooks that find nothing wrong should produce zero output. Don't print "all clear" messages — they create noise on every tool call.

**Keep it fast.** Hooks run synchronously before every tool call. Avoid network calls, heavy computation, or anything that could add noticeable latency. For PostToolUse linting/formatting, always set a timeout (10–15s).
