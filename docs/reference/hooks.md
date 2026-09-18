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
  └── scan-budget.sh           # Shared: the wall-clock scan budget the fail-closed guards run under
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
  └── scan-budget.test.sh      # The scan budget: forced hits block, ordinary input is untouched
  opencode/                   # One .js module per hook + shared utils + entry point + tests
  └── utils.js                # Shared helpers: scanBudgetMs/startScanBudget, findRoot, which, runLinter, runCommand (async spawn), countLines
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
  └── scan-budget.test.js     # The scan budget, and both twins agreeing on the same input
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
    "script":  "hooks/claude-code/hook-name.sh",
    "timeout": 45
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
| `claude_code.timeout` | Optional hook timeout **in seconds**, written into the registration. Set on the guards that enforce a [scan budget](#the-scan-budget), above that budget. Omit for everything else, and an install leaves whatever timeout is registered alone |
| `opencode.event` | `tool.execute.before` or `file.edited` |
| `opencode.module` | Path to the JS module, relative to repo root (`hooks/opencode/<hook-name>.js`) |
| `opencode.export` | Name of the module's factory export — the lowerCamel hook name (`hookName`) |
| `opencode.fail_closed` | `true` for security guards: if the module fails to import or initialise, the plugin blocks every tool call instead of running without it. Omit for advisory hooks |
| `claude_code.enabled` | Optional Claude Code-only override of `enabled`; absent (nil) = follow `enabled` (#150) |
| `opencode.enabled` | Optional opencode-only override of `enabled`; absent (nil) = follow `enabled`. The `graphify-*` hooks set `true` |
| `enabled` | Set to `false` to skip this hook for all users (targets without their own `enabled` override) |

Every key other than `name`, `description` and `enabled` whose value is an object is an **install-target block**, keyed by target id: `claude_code` for Claude Code, `opencode` for opencode. The Go installer parses them all into `Hook.Targets` (`map[string]hooks.TargetSpec` in `cli/internal/hooks/installer.go`), so a new target is a new sibling block, not a new Go type. Target blocks share one field vocabulary: `event`, `matcher` and `timeout` (Claude Code), `script` (command-based targets), `module` + `export` (JS-plugin targets), `fail_closed`, and the optional per-target `enabled`. Each install asks `EnabledFor(<target>)`: the target block's own `enabled` when set, else the top-level `enabled` (Claude Code too, since #150; before it, Claude Code read only the top-level `enabled`).

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

The three `graphify-*` hooks ship with `enabled: false` — they're an **optional set** for projects that adopt the `graphify` skill and maintain a `graphify-out/` knowledge graph. All three self-gate on `graphify-out/graph.json` existing, so flipping them on is harmless even if a project hasn't built a graph yet (they simply no-op). Enable them in a fork by setting `"enabled": true` in `hooks/registry.json`. `devexp.config.json` can't enable them: it supports only `hooks.disabled` (`cli/internal/config/config.go`), and the installer skips any hook not enabled for its target (`EnabledFor`: the target block's `enabled`, else the top-level one) before it looks at config (`cli/internal/hooks/installer.go`). The install wizard lists only enabled hooks (`listHookNames` in `cli/cmd/registry.go`).

**What `secret-in-write-guard` lets through on purpose** (#143). A value that is only a placeholder is allowed even when it starts with a vendor prefix: a documented example key ID, a `your-…` phrase, or a body made of one repeated character. The whole matched value has to be the placeholder, and every position in the text is still checked, so a real key next to a placeholder or joined to one still blocks. A value wrapped in `<…>` never matched any pattern, and is now tested. Long snake_case names that happen to contain a GitHub prefix are allowed, because GitHub tokens are matched by their documented shapes. A private-key header is blocked only when key material follows it closely, before any `END` or `BEGIN` line, so a header quoted in documentation or code, or a template with an elided body, is allowed, whatever comes later in the file. Key-like text on the header's own line or just after it, such as a long identifier or a fingerprint, still blocks. A real PEM block still blocks, including escaped, concatenated, encrypted and PGP-armored ones.

**What `secret-in-write-guard` doesn't see** — it scans only the new text of a write, edit or patch, never the file text around it. A secret completed across text already in the file and an edit isn't seen. In opencode, `apply_patch` context lines are matched against the file loosely and written from the patch's own text, and those lines aren't scanned. A private-key body wrapped far narrower than the usual PEM line width, written in pieces across several edits, or starting far from its header, isn't recognized as key material. Its patterns run in time linear in the length of the write, and the scan runs under a [budget](#the-scan-budget) that blocks the write if it is exceeded. The guard catches a secret written in one piece; it doesn't replace a secret scanner on the repository.

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
- subshells, `$(…)`, backticks or process substitutions are nested more than 100 levels deep
- the parser raises for any other reason

**Where a target ends.** Before matching, a trailing backslash plus newline is joined, so a command continued across lines matches as one line. The targets are `/`, the home directory, `/tmp` and `~/.claude`, plus the `--force`, `--force-with-lease` and `-f` flags.

A target ends at whitespace or end of line. It also ends at a character that ends the word or can change what it expands to:

- a character that closes the word or starts a redirect: `'`, `"`, a backtick, `(`, `)`, `;`, `&`, `|`, `<` or `>`
- the start of an expansion: `$`, `{`, `*`, `?` or `[`, or an extglob `@(`, `+(` or `!(`

An expansion right after a target can leave the target itself. It may be empty, or split the word apart (bash splits an unquoted expansion; zsh expands `~` after parameters). It may also be a glob that matches the directory or everything in it, a brace expansion with an empty alternative, or a zsh subscript or glob qualifier. So `rm -rf ~/$dir`, `rm -f /tmp/$name`, `rm -rf /*` and `rm -rf ~/?*` block, just as `rm -rf "/tmp/"$name` already did.

A `:` after a target is a plain character in both shells, so `-v /tmp:/data` doesn't match. It changes a word only right after an unbraced parameter name, as a zsh modifier, so `rm -rf $HOME:h` blocks.

**The trade-off.** A protected directory followed directly by *any* expansion blocks, even when a literal follows the expansion (`rm -f /tmp/$name.log`, `rm -f /tmp/{a,b}.log`). To delete something under a protected directory by variable, put a literal component right after the directory: `rm -f /tmp/.myapp-$name.log`.

The root and home targets may also start with a quote (`rm -rf "/"`). So `sh -c 'rm -rf /'`, `eval "git push --force"`, `$(rm -rf ~)` and `git push -f&& …` block.

The home directory is recognised as `$HOME` (also zsh `$~HOME`, `$=HOME` and `$^HOME`), `${HOME}` with any operator, subscript, flags or modifier (`${HOME:-…}`, `${HOME[@]}`, `${=HOME}`, `${(L)HOME}`), `~`, or `~name` (any user's home). A trailing `/` is allowed after every spelling.

Every character not listed above continues the target: letters, digits, `/`, `.`, `-`, `_`, `:`, `=`, `,`, `%`, `^`, `#`, `~`, `]`, `}` and `\`; `@`, `+` and `!` when no `(` follows; and any non-ASCII character (except that the opencode module counts non-ASCII spaces as whitespace). So `rm -rf ./build`, `rm -rf ~/projects/$x`, `rm -f /tmp/.deliver-$id-*`, `rm -rf $HOMEDIR`, `rm -rf ${HOME}x` and `git push --follow-tags` don't match.

**Which `rm`.** The `rm -rf` and wildcard-delete rules need `rm` as a word of its own. It must not end a longer word or option (`--rm`, `terraform`), except after a positional parameter that may be empty (`$1rm`). It may still be an argument of another command (`xargs rm`, `find … -exec rm`, `sudo rm`, `/bin/rm`, `\rm`), so a wrapper can't hide it. In the wildcard-delete rule, `rm` may be followed by whitespace, a quote, an expansion (`$…`, a backtick), a brace or glob character, or a redirect (`<`, `>`, `&>`), because the shell still runs `rm` with what comes after. So `rm$IFS-f /tmp/*`, `rm>/dev/null -f /tmp/*` and `{sudo,rm} -f /tmp/*` block, while `rm.sh /tmp/$x` doesn't. (The `rm -rf` rule needs whitespace after `rm`, as before.)

**Which command.** The wildcard-delete rule also needs the target in the same simple command as that `rm`. Its scan stops at `;`, `|`, and a `&` that isn't part of a redirect (`2>&1` and `&>` don't stop it). So `docker run --rm -v /tmp:/tmp img`, `rm -rf dist && cp -r out /tmp/$(date +%s)` and `rm -f a && ls /tmp/*` don't match, while `make || rm -rf /tmp/$x` does.

A `;` or `&` can also be data: inside quotes, after a backslash, or inside `$(…)`, backticks, `${…}`, `$[…]`, `$'…'` or a process substitution (`<(…)`, `>(…)`, zsh `=(…)`). So once a quote, backslash, backtick, `$(`, `${`, `$[`, `<(`, `>(` or `=(` appears between `rm` and the first `;` or `&`, the scan runs on to the next `|`, as it did before this change. `rm -rf "a;b" /tmp/*` blocks. The cost is an over-block when quoting comes before a real separator (`rm -f "$x" && cp out /tmp/$y` blocks). A `|` still ends the scan even inside quotes, as it always has; that limit is tracked separately.

**One line at a time.** Both implementations decide line by line: the Claude Code hook greps each line, and the opencode module splits the text at newlines before it tests its rules. A pattern that starts on one line and ends on a later one is not a match, unless the lines were joined by a backslash continuation. Within a line, a carriage return counts as whitespace. NUL characters are dropped before matching.

**Time on long commands (#146).** Both implementations take time linear in the command's length, so a crafted command can't stall a guarded tool call. A crafted 1 MB line takes at most about 300 ms in the opencode module and a few seconds in the Claude Code hook, most of it in the Python step (about 2.5 s for a 1 MB pipeline with BSD `grep`). Before, a 0.5 MB pipeline took over a minute there.

The opencode rules are not copies of the grep patterns, because JavaScript's regex engine backtracks. Each rule is written in an equivalent form that doesn't backtrack:

- `rm` flags are split at their first `r` or `f`.
- A "prefix, then any text, then suffix" rule tries only the first prefix in each segment and searches for the suffix once, left to right. For the wildcard-delete rule a segment is one simple command, found by the same redirect-aware scan as the grep pattern `([^|;&]|[<>]&|&>)*`, or the text up to the next `|` once a quote or substitution opens before that end (`commandEndQuoted`).
- The `.claude…/*` scan reads from a single table built right to left.

The test suites run the same cases against both. The opencode suite also times crafted 100 KB and 1 MB lines, and the Claude Code suite times a 400 KB pipeline.

In `dangerous-cmd-guard.sh`, all parsing happens in the `python3 -I` step, where the tool input is data on stdin. `grep` reads the result from a here-string, so no pipe writer is killed by SIGPIPE when `grep -q` exits early. Any interpreter or `grep` error blocks.

---

## The Scan Budget

Every guard marked `fail_closed` — `secret-guard`, `secret-in-write-guard`,
`dangerous-cmd-guard` — scans under a wall-clock budget and **blocks** when it is
exceeded (#162).

**Why.** Claude Code does not block a tool call when a **command** hook times
out: *"A timed-out `command`, `http`, or `mcp_tool` hook doesn't block the tool
call. The call continues through the normal permission flow, so don't count on a
stalled hook to act as a gate."* Its default timeout for such a hook is **600
seconds**, and no hook field expresses "block on timeout". So without a budget a
guard that is slow on some input fails **open**: the call goes ahead unscanned
after a long wait. (The Agent SDK's in-process *callback* hooks do block on
timeout — a different mechanism, and not what devexp registers.) opencode is the
other way round: it puts no timeout on a plugin hook and runs it in its server
process, so a slow scan stalls the session and a hook that never returns hangs
the call for good.

**The budget.** `DEVEXP_SCAN_BUDGET_MS`, default **15000 ms**, ceiling
**44000 ms**, per guard invocation. Both twins read the variable and carry the
same default and ceiling (`DEVEXP_SCAN_BUDGET_DEFAULT_MS` and
`DEVEXP_SCAN_BUDGET_MAX_MS` in `hooks/claude-code/scan-budget.sh`,
`SCAN_BUDGET_DEFAULT_MS` and `SCAN_BUDGET_MAX_MS` in `hooks/opencode/utils.js`);
the test suites pin them to each other and to the registry.

Only a plain **ASCII** non-negative integer counts, with surrounding spaces
trimmed — anything else falls back to the default, so a typo can neither widen
the budget nor disable the guard. (Python's `str.isdigit()` is not that test: it
accepts a non-ASCII decimal digit such as `U+0663` and a digit-like character
such as `U+00B2`, which is why the shell side spells it `[0-9]` like the JS
side's `\d`.) `0` means "already over budget" and blocks immediately; it is the
test seam, not a setting to use.

**A value above the ceiling is clamped, not honoured**, with a one-line notice
on stderr (once per process). `DEVEXP_SCAN_BUDGET_CEILING_MS` may *lower* the
ceiling and never raise it — the worst an ambient value can do is make a guard
block sooner — which is what lets the suites watch a clamped budget bite without
waiting 44 seconds for one. A budget at or above the registered hook timeout
would reinstate the bug this exists to prevent: Claude Code would cancel the
guard first, and a cancelled command hook does not block the tool call. The
ceiling sits just under the registered 45 s, and `scan-budget.test.sh` and the Go
registry test both fail if a timeout ever drops to or below it. opencode has no
such timeout, but shares the ceiling: a ten-minute budget there is a tool call
that can stall for ten minutes.

15 s is sized from the worst cases measured on the crafted inputs the timing
tests use — about 1 s for a 2 MB write through `secret-in-write-guard`, about
1.7 s for a 1 MB command through `dangerous-cmd-guard`, under 0.1 s for ordinary
input — and sits above the ceilings those suites already accept as "not slow"
(8 s for a 2 MB write, 15 s for a 400 KB pipeline), so a much slower machine
still never trips it.

**The hook timeout.** Each of these guards sets `claude_code.timeout: 45` in the
registry — three times the default budget, and just above the ceiling — so the
guard's own exit 2 always lands before Claude Code cancels it. The installer writes the field into the registration and
brings an existing one to the registry's value, so a machine installed before
this existed stops running on the 600 s default at its next install.

**How it is enforced.**

| | Claude Code | opencode |
|---|---|---|
| Mechanism | `scan-budget.sh` re-runs the guard under a `python3` watchdog in a session of its own and `SIGKILL`s the process group at the deadline | a deadline started at handler entry, checked as the scan runs; the check throws a `ScanBudgetError`, which is how `tool.execute.before` refuses a call |
| Covers | the whole hook run — reading the envelope, `json.load`, the regex work and every `grep` | the scan, at the granularity of one unit of work: one pattern, one rule against one line, one token, and the masking pass `dangerous-cmd-guard` runs first |
| Cost | one extra `python3` **and** one extra `bash` — the guard is re-run as a child, not `exec`'d — so roughly **+60 to +85 ms** per guarded tool call on current hardware, about double a guard's run (median of 30 warmed runs against `origin/main`, same machine: `dangerous-cmd-guard` 66 → 129 ms, `secret-guard` 46 → 130 ms, `secret-in-write-guard` 46 → 130 ms; one Bash tool call, two guards in parallel, 66 → 130 ms. An independent run on the same head measured 69 → 144, 50 → 135 and 50 → 137 ms) | none measurable — the masking pass's budget checks are sampled by position and came out within noise (−6% to +2%) |
| On a hit | exit 2 with a message naming the budget | a thrown block with the same message |

**How often the opencode side looks at the clock.** Reading it is not free — on
an 800k-token command, checking per token costs about 9% over sampling — so
there is one rule rather than a choice per guard: check every unit where units
are few and each is expensive (the 11 regexes of a write), and sample every 1024
where units are many and each is cheap (a token, a line of a command that may
hold hundreds of thousands). The masking pass is sampled by position, every
65536 characters of the parse and nodes of the walk. Either way the overshoot is
one unit of work.

A single regex call cannot be interrupted from inside JavaScript, so the opencode
side gives up at the first check after the budget rather than mid-pattern. That
side has no fail-open path to begin with: the outcome there is a refused call,
never one that proceeds unscanned. A spent budget throws a distinct
`ScanBudgetError` so that `maskInert`'s catch — which turns anything the parser
refuses into "scan the whole command" — lets it past instead of swallowing it.

The shell twin also blocks on any child status that is neither allow (0) nor
block (2), and when the watchdog itself cannot run — a guard whose decision
cannot be read must not be taken for "allowed". The same goes for the window
before the budget exists: each guard opens with a prologue trap, because an
`if ! . …` cannot floor it (under `set -e` bash leaves the script where the `.`
failed, and for a missing, unreadable or unparsable file it then reports 0 to an
EXIT trap, which reads as *allow*). Reaching that trap at all is a block. The
marker that tells the budgeted run apart travels in **argv**, not the
environment, so an ambient variable — a settings `env` entry, a shell profile, a
CI image — cannot switch the budget off. A second marker, a depth counter, *is*
read from the environment, because the only thing it can do is block: if the
argv marker ever stopped being recognised, each budgeted run would start a
watchdog of its own and the guard would fork without bound (a watchdog kills
only its own child's process group, and every level makes a new session). At the
limit the guard stops with an internal error instead of multiplying.

**Where the twins differ.** The Claude Code budget starts before the guard can
tell which tool it was handed and covers process startup; the opencode budget
starts once the module has seen a tool it scans. With a real budget that makes no
difference; with a budget of a millisecond or two it does, which is why the
parity tests use a zero budget or the real one.

---

## CLI Compatibility

| | Claude Code | opencode |
|---|---|---|
| Hook scripts | `hooks/claude-code/*.sh` (one per hook) | `hooks/opencode/*.js` (one module per hook) |
| Entry point | Each script registered separately in `settings.json` | `devexp-plugin.js` composes the modules listed in `devexp/hooks.json`; file events arrive through `event` → `file.edited` |
| Installed by `./install.sh` | Yes — enabled hooks, into `~/.claude/settings.json` | Yes — selected modules, into `~/.config/opencode/plugins/` (`devexp.js` + `devexp/`) |
| Selection | `claude_code.enabled` if set, else `enabled`, minus `hooks.disabled` / wizard deselection | `opencode.enabled` if set, else `enabled`, minus `hooks.disabled` / wizard deselection — so the `graphify-*` hooks are on (turn them off with `hooks.disabled`) |
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

   A **security guard** also takes a [scan budget](#the-scan-budget): source `scan-budget.sh` and call `devexp_scan_budget <hook-name> "$@"` at the top of the `.sh` (copy the block from an existing guard, including its fail-closed load), start a budget in the `.js` handler and check it at every unit of the scan, and give the registry entry a `claude_code.timeout` above the budget. `scan-budget.test.sh` fails if a `fail_closed` guard has no such timeout.

4. Add mirrored tests (`<hook-name>.test.sh` / `<hook-name>.test.js`), a `check` line in `hooks/claude-code/fail-closed.test.sh`, and update this catalog, the file tree above and the hook counts — see [workflows → Add a hook](../guides/workflows.md#add-a-hook).

5. `chmod +x hooks/claude-code/<hook-name>.sh` and run `./install.sh`.

Full guide: [`docs/development/hook-authoring-guide.md`](../development/hook-authoring-guide.md)
