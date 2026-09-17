# Hooks Reference

## How Hooks Work

Hooks intercept tool calls automatically — no user action required. Some are safety guards that block or ask; others (`lint-on-save`, `format-on-save`, `test-on-save`, `graphify-grep-nudge`) are advisory and never block. Each hook has an implementation per CLI, and the installer installs both: the Claude Code scripts (`cli/cmd/install_claude.go`) and the opencode plugin (`cli/cmd/install_opencode.go`).

**Claude Code** hooks are shell scripts registered in `~/.claude/settings.json` under `PreToolUse` or `PostToolUse` events. devexp edits only its own handlers there; your hooks keep every field ([install guide](../guides/install.md#what-install-and-uninstall-change-in-settingsjson)). Claude Code calls the script with a JSON payload on stdin and reads the response:

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
| `secret-in-write-guard` | PreToolUse | `Write\|Edit\|MultiEdit\|NotebookEdit` | Hard-blocks writing content that contains secret patterns (Anthropic, OpenAI incl. older `sk-service-` keys, AWS, GitHub and Slack tokens, private key blocks), but not placeholders (see "lets through on purpose" below); in opencode it scans `write`, `edit` and the lines `apply_patch` adds |
| `dangerous-cmd-guard` | PreToolUse | `Bash` | Hard-blocks `rm -rf /`, unanchored wildcard deletes in sensitive dirs (`/tmp/*`, `~/.claude/.../*`), fork bombs, `DROP DATABASE`, `git push --force`, `git reset --hard`, `git clean`, `DROP/TRUNCATE TABLE`, but not a mention of one in text that never runs ([what it matches](#what-dangerous-cmd-guard-matches)) |
| `large-file-guard` | PreToolUse | `Write` | Asks for confirmation before overwriting a file with >500 lines |
| `lint-on-save` | PostToolUse | `Write\|Edit` | Runs the project linter on edited source files (JS/TS → biome/eslint, Python → ruff/flake8, Go → go vet, Ruby → rubocop) |
| `format-on-save` | PostToolUse | `Write\|Edit` | Runs the project formatter in-place (JS/TS → biome/prettier, Python → ruff/black, Go → gofmt, Ruby → rubocop). In opencode it rewrites the file after the edit tool computed its diff, so the reported diff can differ from the file on disk |
| `test-on-save` | PostToolUse | `Write\|Edit` | Runs the associated test file after editing a source file — skips silently if no test file found |
| `graphify-read-guard` *(disabled)* | PreToolUse | `Read\|Glob` | Gates source reads/globs behind a tapering `graphify query` cadence (5 → 3 → 1 queries to unlock, ~6 reads per cycle) — pairs with the [`graphify`](../../skills/graphify/SKILL.md) skill |
| `graphify-session-sentinel` *(disabled)* | PostToolUse | `Bash` | Tracks `graphify query/path/explain` usage toward `graphify-read-guard`'s tapering gate |
| `graphify-grep-nudge` *(disabled)* | PreToolUse | `Bash\|Grep` | Soft-nudges toward `graphify query` (via `additionalContext`, never a block) when grep-like commands or the `Grep` tool run |

The three `graphify-*` hooks ship with `enabled: false` — they're an **optional set** for projects that adopt the `graphify` skill and maintain a `graphify-out/` knowledge graph. All three self-gate on `graphify-out/graph.json` existing, so flipping them on is harmless even if a project hasn't built a graph yet (they simply no-op). Enable them in a fork by setting `"enabled": true` in `hooks/registry.json`. `devexp.config.json` can't enable them: it supports only `hooks.disabled` (`cli/internal/config/config.go`), and the installer skips any hook with `enabled: false` before it looks at config (`cli/internal/hooks/installer.go`). The install wizard lists only enabled hooks (`listHookNames` in `cli/cmd/registry.go`).

**What `secret-in-write-guard` lets through on purpose** (#143). A value that is only a placeholder is allowed even when it starts with a vendor prefix: a documented example key ID, a `your-…` phrase, or a body made of one repeated character. The whole matched value has to be the placeholder, and every position in the text is still checked, so a real key next to a placeholder or joined to one still blocks. A value wrapped in `<…>` never matched any pattern, and is now tested. Long snake_case names that happen to contain a GitHub prefix are allowed, because GitHub tokens are matched by their documented shapes. A private-key header is blocked only when key material follows it closely, before any `END` or `BEGIN` line, so a header quoted in documentation or code, or a template with an elided body, is allowed, whatever comes later in the file. Key-like text on the header's own line or just after it, such as a long identifier or a fingerprint, still blocks. A real PEM block still blocks, including escaped, concatenated, encrypted and PGP-armored ones.

**What `secret-in-write-guard` doesn't see** — it scans only the new text of a write, edit or patch, never the file text around it. A secret completed across text already in the file and an edit isn't seen. In opencode, `apply_patch` context lines are matched against the file loosely and written from the patch's own text, and those lines aren't scanned. A private-key body wrapped far narrower than the usual PEM line width, or written in pieces across several edits, isn't recognized as key material. Its patterns run in time linear in the length of the write; there is no separate time budget, and a Claude Code hook that runs past its timeout doesn't block the write. The guard catches a secret written in one piece; it doesn't replace a secret scanner on the repository.

**How `graphify-read-guard` paces itself** — rather than a flat "queried in the last N hours" timer (which re-arms mid-session and creates friction, or "gate once" which under-uses the graph), it runs a tapering cadence sourced from a small JSON state file (`graphify-out/.graphify_session`, shared with `graphify-session-sentinel`):

1. **Fresh session** → blocks Read/Glob until `graphify query` has run **5** times
2. Unlocks a budget of **~6** reads
3. Budget exhausted → re-arms, but now needs only **3** queries to unlock
4. Subsequent re-arms taper to a steady-state floor of **1** query per ~6-read cycle

This front-loads grounding when the agent knows least about the codebase, and eases off once it's shown sustained engagement with the graph — without ever resetting on a wall-clock timer or permanently locking out repo reads. `graphify-grep-nudge` covers the gap for `grep`/`rg`/`find`/etc. (Bash) and the built-in `Grep` tool — since those are often legitimately faster for precise lookups, it only nudges via `additionalContext`, it never blocks.

### What `dangerous-cmd-guard` matches

The patterns in the catalog row above run against the command after the guard blanks out text that can't run (#100). A runbook `echo`, a commit message or an issue body that names a destructive command is a mention, not an invocation. Both implementations share these rules: `maskInert` in `hooks/opencode/dangerous-cmd-guard.js` and `scan_text` in `hooks/claude-code/dangerous-cmd-guard.sh`.

**Blanked (never runs):**

- every argument of `echo`, and of `printf` without `-v`
- the message of `git commit`/`git tag` (`-m`, `-am`, `-m…`, `--message[=]`)
- the `--body`/`-b`, `--title`/`-t`, `--notes`/`-n` and `--comment`/`-c` value of `gh issue|pr|release create|comment|edit|review|close|merge`
- a heredoc body fed to `cat`, `head`, `tail`, `wc`, `grep`, `egrep`, `fgrep`, `tr`, `cut` or `nl`, or read as a body by `git commit|tag -F -` or `gh … --body-file -`, when the delimiter is quoted or the body has no `$(`/backtick
- a `#` comment

These apply only when the command's output can't reach anything that runs it. It must not be redirected anywhere but `/dev/null`, `/dev/stdout`, `/dev/stderr`, `/dev/tty`, `&1` or `&2`, and every later stage of its pipeline must be one of the text filters above or a body reader. Assignments, reserved words (`{`, `!`, `if`, `then`, `else`, `elif`, `while`, `until`, `do`, `time`) and option-free `sudo`, `env`, `command`, `builtin`, `exec` and `time` prefixes are skipped to find the command. A `$(…)` or backtick inside a blanked argument still runs, so it is checked like any other command.

**Always scanned:**

- every other command and argument, including the strings passed to `bash -c`, `sh -c`, `eval`, `ssh host "…"` and `psql -c "…"`
- a `$(…)` or backtick anywhere else (command position, assignment, another command's argument)
- process substitutions
- heredocs fed to anything else
- `echo …` piped to a shell or `tee`, or redirected into a file

**Scanned whole, like before #100:** if the guard can't be sure what runs, nothing is blanked. That happens when:

- a quote, `$(`, backtick, subshell or heredoc is unterminated
- the command has a `case` statement, a function (`f() {…}`), `alias`, `unalias`, `function`, `hash`, `enable`, `coproc`, a bare `exec` (fd redirection), `$((…))`, a `${…}` holding quotes, parentheses, braces or backslashes, or a backtick body with a backslash
- a `(` sits inside or right after a word (arrays, extglob, zsh `=(…)` and glob qualifiers)
- a subshell, `}`, `fi` or `done` is followed by a pipe or redirect
- a heredoc is still pending when a nested command ends
- any word names a shell or interpreter that could run text read back from a file, the clipboard, git or GitHub (`sh`, `bash`, `zsh`, …, `eval`, `source`, `xargs`, `ssh`, `su`, `script`, `python`, `perl`, `ruby`, `node`, `php`, `awk`, `sed`, `osascript`, …)
- the command word is `.`, a path, or not a plain literal (`$SHELL`, `"$x"`, `$(…)`)
- the parser raises for any other reason

**Where a target ends.** Before matching, a trailing backslash plus newline is joined, so a command continued across lines matches as one line. A target (`/`, `~`, `$HOME`, `/tmp`, `~/.claude`, and the `--force`, `--force-with-lease` and `-f` flags) ends at whitespace, end of line, or a character that closes a shell word: `'`, `"`, `)`, a backtick, `;`, `&` or `|`. The root and home targets may also start with a quote (`rm -rf "/"`). So `sh -c 'rm -rf /'`, `eval "git push --force"`, `$(rm -rf ~)` and `git push -f&& …` block. A letter, digit, `/`, `.`, `-` or `*` continues the target, so `rm -rf ./build`, `rm -rf ~/projects/x` and `git push --follow-tags` don't match.

**One line at a time.** Both implementations decide line by line. The Claude Code hook greps each line, and every opencode pattern is kept to a single line: its whitespace classes, negated classes and "any character" all stop at a newline. A pattern that starts on one line and ends on a later one is not a match, unless the lines were joined by a backslash continuation. Within a line, a carriage return counts as whitespace. NUL characters are dropped before matching.

In `dangerous-cmd-guard.sh`, all parsing happens in the `python3 -I` step, where the tool input is data on stdin. `grep` reads the result from a here-string, so no pipe writer is killed by SIGPIPE when `grep -q` exits early. Any interpreter or `grep` error blocks.

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
