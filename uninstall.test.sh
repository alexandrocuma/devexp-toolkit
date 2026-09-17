#!/usr/bin/env bash
# Tests uninstall.sh.
#
# 1. The Claude Code hook-removal logic. It used to match on `repo_dir in cmd`,
#    so it removed only hooks registered from the current clone. A registration
#    left by an earlier release-binary install lives elsewhere, so uninstalling
#    stripped the working entries and left the stale ones running — strictly
#    worse than doing nothing (issue #93). The block is embedded in uninstall.sh
#    as a heredoc, so it is extracted and run directly against fixtures.
#
# 2. The opencode wiring (issue #109): no top-level `local` (it aborted every
#    opencode uninstall), the MCP block tolerating a bad config.json, detection
#    of a plugin-only install, and delegation of plugin removal to
#    `devexp uninstall --target opencode`. These run uninstall.sh itself with
#    --yes, stdin closed, HOME in a temp dir, a minimal environment and stub
#    devexp binaries, so it never touches real files. The removal rules are
#    tested in Go (cli/internal/hooks, cli/cmd).
#
# 3. A round trip with the real binary, when `go` is on PATH and the CLI's
#    assets are staged (./scripts/stage-assets.sh); otherwise it prints SKIP.
#
# Run: bash uninstall.test.sh
set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

# ── Extract the embedded hook-removal program ───────────────────────────────
awk '/python3 - "\$REPO_DIR" "\$settings_path" <</{flag=1;next} /^PYEOF$/{flag=0} flag' \
    "$ROOT/uninstall.sh" > "$TMP/prune.py"
if [ ! -s "$TMP/prune.py" ]; then
    echo "FAIL: could not extract the hook-removal block from uninstall.sh"
    exit 1
fi

# ── A fake devexp repo, so the registry is the source of managed names ──────
REPO="$TMP/repo"
mkdir -p "$REPO/hooks/claude-code"
cat > "$REPO/hooks/registry.json" <<'JSON'
[
  {"name": "secret-guard", "enabled": true,
   "claude_code": {"event": "PreToolUse", "matcher": "Read|Bash",
                   "script": "hooks/claude-code/secret-guard.sh"}},
  {"name": "graphify-read-guard", "enabled": false,
   "claude_code": {"event": "PreToolUse", "matcher": "Read|Glob",
                   "script": "hooks/claude-code/graphify-read-guard.sh"}}
]
JSON

OTHER="$TMP/other-root"

run() { # $1 = settings json  -> prints resulting commands, one per line
    printf '%s' "$1" > "$TMP/settings.json"
    python3 "$TMP/prune.py" "$REPO" "$TMP/settings.json" >/dev/null 2>&1
    python3 - "$TMP/settings.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
for ev, arr in sorted((d.get('hooks') or {}).items()):
    for e in arr:
        for h in e.get('hooks', []):
            print(h.get('command', ''))
PY
}

expect() { # $1=label  $2=settings  $3=expected remaining commands (newline sep)
    local label="$1" got want
    got="$(run "$2")"
    want="$(printf '%s' "$3")"
    if [ "$got" = "$want" ]; then
        pass=$((pass+1))
    else
        fail=$((fail+1))
        printf 'FAIL %s\n  got:  %s\n  want: %s\n' "$label" "${got:-<empty>}" "${want:-<empty>}"
    fi
}

MINE="$REPO/hooks/claude-code/secret-guard.sh"
FOREIGN="$OTHER/hooks/claude-code/secret-guard.sh"
FOREIGN_DISABLED="$OTHER/hooks/claude-code/graphify-read-guard.sh"
USER_HOOK="$OTHER/my-hooks/secret-guard.sh"
USER_OTHER="$OTHER/my-hooks/format-check.sh"

# Removes our own registration (the normal uninstall case)
expect "removes the clone's own hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$MINE\"}]}]}}" \
  ""

# #93: also removes one registered from a different install root
expect "removes a hook from a foreign install root" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN\"}]}]}}" \
  ""

# Disabled hooks are devexp's too
expect "removes a foreign copy of a disabled hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Glob\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN_DISABLED\"}]}]}}" \
  ""

# A user hook sharing a basename must survive
expect "keeps a user hook that shares a basename" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$USER_HOOK\"}]}]}}" \
  "$USER_HOOK"

# An unrelated user hook must survive
expect "keeps an unrelated user hook" \
  "{\"hooks\":{\"PostToolUse\":[{\"matcher\":\"Write\",\"hooks\":[{\"type\":\"command\",\"command\":\"$USER_OTHER\"}]}]}}" \
  "$USER_OTHER"

# The bug this also fixes: entries were judged by hooks[0] alone, so a second
# command in the same entry was handled by accident rather than on its merits.
expect "an entry holding both keeps only the user command" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN\"},{\"type\":\"command\",\"command\":\"$USER_HOOK\"}]}]}}" \
  "$USER_HOOK"

expect "a user command listed first no longer shields a devexp one" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$USER_HOOK\"},{\"type\":\"command\",\"command\":\"$FOREIGN\"}]}]}}" \
  "$USER_HOOK"

# Both roots registered at once — the real-world state issue #93 describes
expect "removes both roots' copies of the same hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$MINE\"}]},{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN\"}]}]}}" \
  ""

# A relative registration (left by an install from a relative DEVEXP_DIR) is
# devexp's too; a relative user hook sharing a basename is not (#126).
expect "removes a relative devexp hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"hooks/claude-code/secret-guard.sh\"}]}]}}" \
  ""

expect "removes a ./-relative devexp hook, keeps a relative user hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Glob\",\"hooks\":[{\"type\":\"command\",\"command\":\"./hooks/claude-code/graphify-read-guard.sh\"},{\"type\":\"command\",\"command\":\"my-hooks/secret-guard.sh\"}]}]}}" \
  "my-hooks/secret-guard.sh"

# Only a plain path is devexp's: a command that expands a variable, takes an
# argument or is wrapped is the user's, and so is one under a directory that
# merely ends in hooks/claude-code/ (#126).
expect "keeps a \$CLAUDE_PROJECT_DIR hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"\$CLAUDE_PROJECT_DIR/hooks/claude-code/secret-guard.sh\"}]}]}}" \
  "\$CLAUDE_PROJECT_DIR/hooks/claude-code/secret-guard.sh"

expect "keeps quoted, ~, \$HOME and wrapped hooks" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"\\\"\$CLAUDE_PROJECT_DIR\\\"/hooks/claude-code/secret-guard.sh\"},{\"type\":\"command\",\"command\":\"~/vendor/hooks/claude-code/secret-guard.sh\"},{\"type\":\"command\",\"command\":\"\$HOME/vendor/hooks/claude-code/secret-guard.sh\"},{\"type\":\"command\",\"command\":\"bash $FOREIGN\"},{\"type\":\"command\",\"command\":\"FOO=1 $FOREIGN\"}]}]}}" \
  "\"\$CLAUDE_PROJECT_DIR\"/hooks/claude-code/secret-guard.sh
~/vendor/hooks/claude-code/secret-guard.sh
\$HOME/vendor/hooks/claude-code/secret-guard.sh
bash $FOREIGN
FOO=1 $FOREIGN"

expect "keeps a my-hooks/claude-code/ hook, relative and absolute" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"my-hooks/claude-code/secret-guard.sh\"},{\"type\":\"command\",\"command\":\"$OTHER/my-hooks/claude-code/secret-guard.sh\"}]}]}}" \
  "my-hooks/claude-code/secret-guard.sh
$OTHER/my-hooks/claude-code/secret-guard.sh"

# ── Paths that need shell quoting (#135) ─────────────────────────────────────
# devexp registers a path with a space or shell syntax as one single-quoted
# word. Those are devexp's; so is the unquoted path an earlier install from
# this repo wrote. Other quoting, arguments or concatenation stay the user's.

q() { # POSIX single-quote $1, written independently of the code under test
    printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}
settings_with() { # commands... -> settings json with one entry holding them all
    python3 -c 'import json,sys; print(json.dumps({"hooks":{"PreToolUse":[{"matcher":"Read","hooks":[{"type":"command","command":c} for c in sys.argv[1:]]}]}}))' "$@"
}

SPACED="$TMP/My Proj/it's \$x;&clone"
SPACED_OTHER="$TMP/Other Root/cache"
mkdir -p "$SPACED/hooks/claude-code"
cp "$REPO/hooks/registry.json" "$SPACED/hooks/registry.json"
SP_MINE="$SPACED/hooks/claude-code/secret-guard.sh"
SP_FOREIGN="$SPACED_OTHER/hooks/claude-code/secret-guard.sh"
SP_FOREIGN_DISABLED="$SPACED_OTHER/hooks/claude-code/graphify-read-guard.sh"

# The helper really is shell quoting: sh reads each quoted path back unchanged.
for p in "$SP_MINE" "$SP_FOREIGN"; do
    if [ "$(sh -c "printf '%s' $(q "$p")")" = "$p" ]; then pass=$((pass+1)); else fail=$((fail+1)); printf 'FAIL q() does not round-trip %s\n' "$p"; fi
done

REPO_PLAIN="$REPO"
REPO="$SPACED"

expect "removes the repo's own quoted hook (space, ', \$, ;, &)" \
  "$(settings_with "$(q "$SP_MINE")")" \
  ""

expect "removes the repo's legacy unquoted hook" \
  "$(settings_with "$SP_MINE")" \
  ""

expect "removes quoted hooks from a foreign root, disabled ones too" \
  "$(settings_with "$(q "$SP_FOREIGN")" "$(q "$SP_FOREIGN_DISABLED")" "$(q "$FOREIGN")")" \
  ""

expect "keeps user commands that quote or wrap a devexp path another way" \
  "$(settings_with \
      "\"$SP_MINE\"" \
      "$(q "$SP_FOREIGN") --flag" \
      "$(q "$SP_FOREIGN"); true" \
      "$(q "$SPACED_OTHER/setup");$(q "$SP_FOREIGN")" \
      "'$TMP/it'\"'\"'s/hooks/claude-code/secret-guard.sh'" \
      "bash $(q "$SP_FOREIGN")" \
      "$(q "$SPACED_OTHER/hooks/claude-code/")secret-guard.sh" \
      "'$SPACED_OTHER/hooks/claude-code/secret-guard.sh" \
      "$(q "hooks/claude-code/secret-guard.sh")" \
      "$(q "$SPACED_OTHER/my-hooks/claude-code/secret-guard.sh")" \
      "$(q "$SPACED_OTHER/hooks/claude-code/format-check.sh")")" \
  "\"$SP_MINE\"
$(q "$SP_FOREIGN") --flag
$(q "$SP_FOREIGN"); true
$(q "$SPACED_OTHER/setup");$(q "$SP_FOREIGN")
'$TMP/it'\"'\"'s/hooks/claude-code/secret-guard.sh'
bash $(q "$SP_FOREIGN")
$(q "$SPACED_OTHER/hooks/claude-code/")secret-guard.sh
'$SPACED_OTHER/hooks/claude-code/secret-guard.sh
$(q "hooks/claude-code/secret-guard.sh")
$(q "$SPACED_OTHER/my-hooks/claude-code/secret-guard.sh")
$(q "$SPACED_OTHER/hooks/claude-code/format-check.sh")"

# Another root's legacy unquoted path can't be told apart from a user command
# that takes arguments, so it is left for that root's own install to fix.
expect "keeps another root's unquoted path that needs quoting" \
  "$(settings_with "$SP_FOREIGN")" \
  "$SP_FOREIGN"

REPO="$REPO_PLAIN"

# A hand fix wraps this repo's path in double quotes. When the path has no $,
# backquote, \ or ", those quotes are literal: it is devexp's (removed). With
# any of them, or with arguments, or for another root, it is the user's.
make_repo() { mkdir -p "$1/hooks/claude-code" && cp "$REPO_PLAIN/hooks/registry.json" "$1/hooks/registry.json"; }

REPO="$REPO_PLAIN"
expect "removes the repo's own double-quoted plain path" \
  "$(settings_with "\"$MINE\"" "\"$MINE\" --strict" "\"$FOREIGN\"")" \
  "\"$MINE\" --strict
\"$FOREIGN\""

DQ_REPO="$TMP/Dq Proj/it's (x) & y"
make_repo "$DQ_REPO"
REPO="$DQ_REPO"
DQ_MINE="$DQ_REPO/hooks/claude-code/secret-guard.sh"
DQ_DISABLED="$DQ_REPO/hooks/claude-code/graphify-read-guard.sh"
expect "removes the repo's own double-quoted path that needs quoting, disabled ones too" \
  "$(settings_with "\"$DQ_MINE\"" "\"$DQ_DISABLED\"" "\"$DQ_MINE\" --strict" "\"$DQ_REPO/hooks/claude-code/mine.sh\"" "\"$SP_FOREIGN\"")" \
  "\"$DQ_MINE\" --strict
\"$DQ_REPO/hooks/claude-code/mine.sh\"
\"$SP_FOREIGN\""

for special in '$' '`' '\'; do
    SPECIAL_REPO="$TMP/special/a${special}b"
    make_repo "$SPECIAL_REPO"
    REPO="$SPECIAL_REPO"
    expect "keeps a double-quoted own path holding $special (not literal in double quotes)" \
      "$(settings_with "\"$SPECIAL_REPO/hooks/claude-code/secret-guard.sh\"")" \
      "\"$SPECIAL_REPO/hooks/claude-code/secret-guard.sh\""
done
REPO="$REPO_PLAIN"

# Every character in SHELL_SYNTAX: a bare path from another root holding it is
# not a plain path, so it is the user's; its single-quoted form is devexp's.
# Dropping any character from SHELL_SYNTAX fails this. (Go's shellSyntax is
# held to the same set by TestShellSyntax_MatchesUninstallScript.)
syntax_chars=(' ' $'\t' $'\n' $'\r' '$' '~' "'" '"' '`' '\' ';' '&' '|' '<' '>' '(' ')' '*' '?' '[' ']' '{' '}' '!' '#')
plain_cmds=(); quoted_cmds=()
for c in "${syntax_chars[@]}"; do
    p="$OTHER/a${c}b/hooks/claude-code/secret-guard.sh"
    plain_cmds+=("$p"); quoted_cmds+=("$(q "$p")")
done
cat > "$TMP/syntax_check.py" <<'PY'
import json, subprocess, sys
prune, repo, settings, n = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4])
plain, quoted = sys.argv[5:5 + n], sys.argv[5 + n:]
with open(settings, 'w') as f:
    json.dump({"hooks": {"PreToolUse": [{"matcher": "Read", "hooks": [
        {"type": "command", "command": c} for c in plain + quoted]}]}}, f)
subprocess.run([sys.executable, prune, repo, settings], capture_output=True, check=True)
with open(settings) as f:
    kept = [h['command'] for e in json.load(f).get('hooks', {}).get('PreToolUse', []) for h in e['hooks']]
if kept == plain:
    print("ok")
else:
    print("wrongly removed:", [c for c in plain if c not in kept])
    print("wrongly kept:", [c for c in kept if c not in plain])
PY
syntax_result="$(python3 "$TMP/syntax_check.py" "$TMP/prune.py" "$REPO" "$TMP/settings.json" "${#plain_cmds[@]}" "${plain_cmds[@]}" "${quoted_cmds[@]}")"
if [ "$syntax_result" = "ok" ]; then pass=$((pass+1)); else fail=$((fail+1)); printf 'FAIL every SHELL_SYNTAX character\n%s\n' "$syntax_result"; fi


# ── Fields devexp doesn't own (#137) ─────────────────────────────────────────
# Only devexp's handlers are removed. Every other handler and entry keeps all
# its fields, an empty entry or event stays, and every byte outside the hooks
# value is as it was. A handler with args (exec form, no shell) or of another
# type is the user's, whatever path it names.
fill_paths() { # $1=template $2=output: @MINE@ and @FOREIGN@ replaced
    python3 - "$1" "$2" "$MINE" "$FOREIGN" <<'PY'
import sys
src, dst, mine, foreign = sys.argv[1:5]
text = open(src, encoding='utf-8').read().replace('@MINE@', mine).replace('@FOREIGN@', foreign)
open(dst, 'w', encoding='utf-8').write(text)
PY
}
expect_file() { # $1=label $2=input template $3=expected template
    fill_paths "$2" "$TMP/settings.json"
    fill_paths "$3" "$TMP/want.json"
    if python3 "$TMP/prune.py" "$REPO" "$TMP/settings.json" > "$TMP/prune.out" 2>&1 \
        && cmp -s "$TMP/settings.json" "$TMP/want.json"; then
        pass=$((pass+1))
    else
        fail=$((fail+1))
        printf 'FAIL %s\n' "$1"
        cat "$TMP/prune.out"
        diff "$TMP/want.json" "$TMP/settings.json" | sed 's/^/  | /'
    fi
}

cat > "$TMP/fields-in.json" <<'JSON'
{
  "env": {"GREETING": "caf\u00e9 & <tea>"},
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "cd /tmp && make lint",
            "timeout": 30,
            "async": true,
            "shell": "bash",
            "if": "Bash(git *)",
            "statusMessage": "Linting…",
            "x-future": {
              "nested": [
                1,
                null
              ]
            }
          },
          {
            "type": "command",
            "command": "@MINE@"
          }
        ]
      },
      {
        "matcher": "Read|Bash",
        "x-note": "mine",
        "hooks": [
          {
            "type": "command",
            "command": "@FOREIGN@",
            "args": [
              "--strict"
            ]
          },
          {
            "type": "prompt",
            "command": "@MINE@",
            "prompt": "Check $ARGUMENTS"
          },
          {
            "type": "http",
            "url": "http://localhost:8080/hooks",
            "headers": {
              "X-Trace": "on"
            }
          }
        ]
      },
      {
        "matcher": "Write",
        "hooks": []
      },
      {
        "matcher": "Read|Glob",
        "hooks": [
          {
            "type": "command",
            "command": "@FOREIGN@",
            "timeout": 9
          }
        ]
      }
    ],
    "Stop": []
  },
  "statusLine": {"type": "command", "command": "~/.claude/statusline.sh"},
  "cleanupPeriodDays": 30.0
}
JSON
cat > "$TMP/fields-want.json" <<'JSON'
{
  "env": {"GREETING": "caf\u00e9 & <tea>"},
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "cd /tmp && make lint",
            "timeout": 30,
            "async": true,
            "shell": "bash",
            "if": "Bash(git *)",
            "statusMessage": "Linting…",
            "x-future": {
              "nested": [
                1,
                null
              ]
            }
          }
        ]
      },
      {
        "matcher": "Read|Bash",
        "x-note": "mine",
        "hooks": [
          {
            "type": "command",
            "command": "@FOREIGN@",
            "args": [
              "--strict"
            ]
          },
          {
            "type": "prompt",
            "command": "@MINE@",
            "prompt": "Check $ARGUMENTS"
          },
          {
            "type": "http",
            "url": "http://localhost:8080/hooks",
            "headers": {
              "X-Trace": "on"
            }
          }
        ]
      },
      {
        "matcher": "Write",
        "hooks": []
      }
    ],
    "Stop": []
  },
  "statusLine": {"type": "command", "command": "~/.claude/statusline.sh"},
  "cleanupPeriodDays": 30.0
}
JSON
expect_file "keeps every field of user hooks and every byte outside hooks" "$TMP/fields-in.json" "$TMP/fields-want.json"

printf '%s' '{"hooks":{"PreToolUse":["junk",{"hooks":[42,{"type":"command","command":"@MINE@"}]}],"Stop":[{"hooks":[{"type":"command","command":"@FOREIGN@"}]}]},"a":"\u0026"}' > "$TMP/compact-in.json"
printf '%s' '{"hooks":{"PreToolUse":["junk",{"hooks":[42]}]},"a":"\u0026"}' > "$TMP/compact-want.json"
expect_file "a compact file stays compact; entries and handlers that aren't objects stay" "$TMP/compact-in.json" "$TMP/compact-want.json"

printf '{\n\t"model": "opus",\n\t"hooks": {\n\t\t"Stop": [\n\t\t\t{\n\t\t\t\t"hooks": [\n\t\t\t\t\t{\n\t\t\t\t\t\t"type": "command",\n\t\t\t\t\t\t"command": "@MINE@"\n\t\t\t\t\t},\n\t\t\t\t\t{\n\t\t\t\t\t\t"type": "command",\n\t\t\t\t\t\t"command": "/usr/local/bin/notify"\n\t\t\t\t\t}\n\t\t\t\t]\n\t\t\t}\n\t\t]\n\t}\n}\n' > "$TMP/tabs-in.json"
printf '{\n\t"model": "opus",\n\t"hooks": {\n\t\t"Stop": [\n\t\t\t{\n\t\t\t\t"hooks": [\n\t\t\t\t\t{\n\t\t\t\t\t\t"type": "command",\n\t\t\t\t\t\t"command": "/usr/local/bin/notify"\n\t\t\t\t\t}\n\t\t\t\t]\n\t\t\t}\n\t\t]\n\t}\n}\n' > "$TMP/tabs-want.json"
expect_file "a tab-indented file keeps its indentation" "$TMP/tabs-in.json" "$TMP/tabs-want.json"

# ── opencode (#109) ──────────────────────────────────────────────────────────

ok() { pass=$((pass+1)); }
ko() { fail=$((fail+1)); printf 'FAIL %s\n' "$1"; [ -n "${2:-}" ] && printf '%s\n' "$2" | sed 's/^/  | /'; return 0; }
check() { # $1=label, rest = test command
    local label="$1"; shift
    if "$@"; then ok; else ko "$label" "$(cat "${E:-/nonexistent}/out" 2>/dev/null)"; fi
}

# ── Static: no top-level `local` ─────────────────────────────────────────────
if bash -n "$ROOT/uninstall.sh" && ! grep -nE '^[[:space:]]*local[[:space:]]' "$ROOT/uninstall.sh" >/dev/null; then
    ok
else
    ko "uninstall.sh parses and uses no \`local\` (it has no multi-line functions)" \
       "$(grep -nE '^[[:space:]]*local[[:space:]]' "$ROOT/uninstall.sh")"
fi

# ── The opencode MCP block tolerates bad input ───────────────────────────────
awk '/python3 - "\$REPO_DIR\/mcps\/registry.json" "\$config_path" <</{flag=1;next} /^PYEOF$/{flag=0} flag' \
    "$ROOT/uninstall.sh" > "$TMP/mcp.py"
if [ ! -s "$TMP/mcp.py" ]; then
    ko "could not extract the opencode MCP block from uninstall.sh"
else
    printf '[{"name": "context7"}]' > "$TMP/mcp-registry.json"
    for cfg in '{not json' '[]' '{"mcp":"x"}' '{"mcp":[]}'; do
        printf '%s' "$cfg" > "$TMP/mcp-config.json"
        if python3 "$TMP/mcp.py" "$TMP/mcp-registry.json" "$TMP/mcp-config.json" >/dev/null 2>&1 \
            && [ "$(cat "$TMP/mcp-config.json")" = "$cfg" ]; then
            ok
        else
            ko "MCP block exits 0 and leaves config.json alone: $cfg"
        fi
    done
    printf '%s' '{"theme":"x","mcp":{"context7":{},"mine":{}}}' > "$TMP/mcp-config.json"
    python3 "$TMP/mcp.py" "$TMP/mcp-registry.json" "$TMP/mcp-config.json" >/dev/null 2>&1
    got="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(c["theme"], sorted(c["mcp"]))' "$TMP/mcp-config.json" 2>&1)"
    if [ "$got" = "x ['mine']" ]; then ok; else ko "MCP block removes only registry MCPs" "$got"; fi

    # mcp_run <dir>: runs the block on <dir>/config.json; output in <dir>.out,
    # exit code in <dir>.rc.
    mcp_run() {
        python3 "$TMP/mcp.py" "$TMP/mcp-registry.json" "$1/config.json" > "$1.out" 2>&1
        echo $? > "$1.rc"
    }
    mode_of() { python3 -c 'import os,sys; print(oct(os.stat(sys.argv[1]).st_mode & 0o777))' "$1"; }

    # The save is atomic: a replaced file (a hard link keeps the old bytes), the
    # old mode, and no temp file left behind.
    D="$TMP/mcp-atomic"; mkdir -p "$D"
    printf '%s' '{"theme":"x","mcp":{"context7":{},"mine":{}}}' > "$D/config.json"
    chmod 640 "$D/config.json"; ln "$D/config.json" "$D/hardlink"
    mcp_run "$D"
    got="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(c["theme"], sorted(c["mcp"]))' "$D/config.json" 2>&1)"
    if [ "$(cat "$D.rc")" = 0 ] && [ "$got" = "x ['mine']" ] \
        && [ "$(cat "$D/hardlink")" = '{"theme":"x","mcp":{"context7":{},"mine":{}}}' ] \
        && [ "$(mode_of "$D/config.json")" = 0o640 ] \
        && [ "$(ls -A "$D" | tr '\n' ' ')" = "config.json hardlink " ]; then
        ok
    else
        ko "MCP block saves by replacing the file atomically, keeping its mode, leaving no temp file" \
           "rc=$(cat "$D.rc") got=$got mode=$(mode_of "$D/config.json") files=$(ls -A "$D" | tr '\n' ' ')"
    fi

    # A symlinked config.json (dotfiles) is left alone, like the plugin step does.
    D="$TMP/mcp-symlink"; mkdir -p "$D" "$TMP/mcp-dotfiles"
    printf '%s' '{"mcp":{"context7":{}}}' > "$TMP/mcp-dotfiles/config.json"
    ln -s "$TMP/mcp-dotfiles/config.json" "$D/config.json"
    mcp_run "$D"
    if [ "$(cat "$D.rc")" = 0 ] && [ -L "$D/config.json" ] \
        && [ "$(cat "$TMP/mcp-dotfiles/config.json")" = '{"mcp":{"context7":{}}}' ] \
        && grep -qF "is a symlink, so it was left untouched" "$D.out" \
        && [ "$(ls -A "$TMP/mcp-dotfiles")" = "config.json" ]; then
        ok
    else
        ko "MCP block leaves a symlinked config.json and its target untouched, with a warning" "$(cat "$D.out")"
    fi

    # Read-only config.json, or a directory where no temp file can be made:
    # a warning, exit 0, the file unchanged, nothing left behind.
    for ro in file dir; do
        D="$TMP/mcp-ro-$ro"; mkdir -p "$D"
        printf '%s' '{"mcp":{"context7":{}}}' > "$D/config.json"
        if [ "$ro" = file ]; then chmod 444 "$D/config.json"; else chmod 555 "$D"; fi
        if [ "$ro" = dir ] && ( : > "$D/.probe" ) 2>/dev/null; then
            rm -f "$D/.probe"; chmod 755 "$D"
            echo "SKIP MCP block with a read-only $ro (permissions not enforced, running as root?)"
            continue
        fi
        mcp_run "$D"
        files="$(ls -A "$D" | tr '\n' ' ')"
        chmod 755 "$D"; chmod 644 "$D/config.json"
        if [ "$(cat "$D.rc")" = 0 ] && [ "$(cat "$D/config.json")" = '{"mcp":{"context7":{}}}' ] \
            && [ "$files" = "config.json " ] && grep -qF "[warn]" "$D.out" && ! grep -qF "Traceback" "$D.out"; then
            ok
        else
            ko "MCP block with a read-only $ro warns, exits 0 and leaves config.json alone" "rc=$(cat "$D.rc") files=$files $(cat "$D.out")"
        fi
    done
fi

# ── Harness: uninstall.sh in a temp HOME with stub devexp binaries ───────────
# Each case gets $E with a repo copy ($E/r), HOME ($E/h), a PATH dir ($E/bin)
# and a call log ($E/calls). Stubs log every real call (not the help probe)
# with the DEVEXP_DIR they were given.
envs=0
new_env() {
    envs=$((envs+1))
    E="$TMP/env$envs"
    mkdir -p "$E/r/agents" "$E/r/skills" "$E/r/mcps" "$E/h/.config/opencode/plugins" "$E/bin" "$E/stubs"
    cp "$ROOT/uninstall.sh" "$E/r/uninstall.sh"
    printf '[]' > "$E/r/mcps/registry.json"
    : > "$E/calls"
}

# make_stub <path> <tag>: a devexp that has `uninstall --target opencode`.
# Its help puts the target first and then more than a pipe buffer of text, so
# a probe piping it into `grep -q` sees it die of SIGPIPE (141 under pipefail).
make_stub() {
    mkdir -p "$(dirname "$1")"
    cat > "$1" <<STUB
#!/bin/bash
if [ "\$*" = "uninstall --help" ]; then
    echo "      --target string   CLI to remove devexp from (supported: opencode)"
    for i in \$(seq 4000); do printf '%s\\n' "help text help text help text help text help text help text help text help text"; done
    exit 0
fi
echo "STUB[$2] \$*"
echo "$2 \$* DEVEXP_DIR=\$DEVEXP_DIR" >> "\$CALLS"
case "\$*" in *--dry-run*) exit 0 ;; esac
exit "\${STUB_RC:-0}"
STUB
    chmod +x "$1"
}

# make_old_stub <path>: a devexp built before the command existed. Its output
# mentions opencode, but not as a supported uninstall target.
make_old_stub() {
    mkdir -p "$(dirname "$1")"
    cat > "$1" <<'STUB'
#!/bin/bash
echo "old $*" >> "$CALLS.old"
echo "DevExp Framework — agents, skills, hooks, and MCPs for Claude Code & opencode"
echo "Error: unknown command \"uninstall\" for \"devexp\"" >&2
exit 1
STUB
    chmod +x "$1"
}

# run_uninstall [VAR=value ...]: uninstall.sh --yes with only these variables.
# Stdin is /dev/null unless STDIN_FILE names a file (menu answers).
run_uninstall() {
    env -i HOME="$E/h" PATH="$E/bin:/usr/bin:/bin" CALLS="$E/calls" "$@" \
        /bin/bash "$E/r/uninstall.sh" --yes <"${STDIN_FILE:-/dev/null}" > "$E/out" 2>&1
    echo $? > "$E/rc"
}

rc_is()        { [ "$(cat "$E/rc")" = "$1" ]; }
out_has()      { grep -qF -- "$1" "$E/out"; }
out_lacks()    { ! grep -qF -- "$1" "$E/out"; }
calls_are()    { [ "$(cat "$E/calls")" = "$(printf '%s' "$1")" ]; }
tagged_calls() { # $1=tag
    printf '%s uninstall --target opencode --dry-run DEVEXP_DIR=%s\n%s uninstall --target opencode --yes DEVEXP_DIR=%s' "$1" "$E/r" "$1" "$E/r"
}

# ── Detection ────────────────────────────────────────────────────────────────
for footprint in devexp.js devexp devexp-plugin.js dangling-devexp.js; do
    new_env
    P="$E/h/.config/opencode/plugins"
    case "$footprint" in
        devexp)             mkdir "$P/devexp" ;;
        dangling-devexp.js) ln -s "$E/nowhere" "$P/devexp.js" ;;
        *)                  printf '/**\n * x\n */\n' > "$P/$footprint" ;;
    esac
    make_stub "$E/stubs/a" A
    run_uninstall DEVEXP_BIN="$E/stubs/a"
    check "plugin-only install ($footprint) exits 0" rc_is 0
    check "plugin-only install ($footprint) is detected" out_has "devexp is installed for opencode"
    check "plugin-only install ($footprint) previews, then removes, through the binary with DEVEXP_DIR" calls_are "$(tagged_calls A)"
    check "plugin-only install ($footprint): the script itself removes no plugin file (the stub removes nothing)" \
        test -e "$P/${footprint#dangling-}" -o -L "$P/${footprint#dangling-}"
done

new_env
make_stub "$E/stubs/a" A
run_uninstall DEVEXP_BIN="$E/stubs/a"
check "no agents and no plugin: nothing to remove" out_has "Nothing to remove"
check "no agents and no plugin: the binary is not called" calls_are ""

# ── Binary lookup: DEVEXP_BIN, then bin/devexp, then PATH ────────────────────
new_env
touch "$E/h/.config/opencode/plugins/devexp.js"
make_stub "$E/stubs/a" A; make_stub "$E/r/bin/devexp" B; make_stub "$E/bin/devexp" C
run_uninstall DEVEXP_BIN="$E/stubs/a"
check "DEVEXP_BIN wins over bin/devexp and PATH" calls_are "$(tagged_calls A)"
: > "$E/calls"; run_uninstall
check "bin/devexp wins over PATH" calls_are "$(tagged_calls B)"
: > "$E/calls"; run_uninstall DEVEXP_BIN="$E/stubs/missing"
check "a DEVEXP_BIN that doesn't exist falls through" calls_are "$(tagged_calls B)"
mv "$E/r/bin/devexp" "$E/stubs/b-moved"
: > "$E/calls"; run_uninstall
check "devexp on PATH is used last" calls_are "$(tagged_calls C)"
make_old_stub "$E/stubs/old"
: > "$E/calls"; run_uninstall DEVEXP_BIN="$E/stubs/old"
check "a DEVEXP_BIN without the command falls through to the next binary" calls_are "$(tagged_calls C)"

# ── No usable binary ─────────────────────────────────────────────────────────
for setup in none old; do
    new_env
    printf 'devexp plugin\n' > "$E/h/.config/opencode/plugins/devexp.js"
    extra=()
    if [ "$setup" = old ]; then
        make_old_stub "$E/stubs/old"; make_old_stub "$E/r/bin/devexp"; make_old_stub "$E/bin/devexp"
        extra=(DEVEXP_BIN="$E/stubs/old")
    fi
    run_uninstall "${extra[@]+"${extra[@]}"}"
    check "no usable binary ($setup): exit 0" rc_is 0
    check "no usable binary ($setup): says so with the rebuild hint" out_has "no devexp binary with 'uninstall' found (set DEVEXP_BIN, or rebuild: rm bin/devexp && ./install.sh)"
    check "no usable binary ($setup): plugin left in place" test "$(cat "$E/h/.config/opencode/plugins/devexp.js")" = "devexp plugin"
    check "no usable binary ($setup): only help probes were made" \
        test -z "$(grep -v '^old uninstall --help$' "$E/calls.old" 2>/dev/null)"
done

# ── The real run fails ───────────────────────────────────────────────────────
new_env
touch "$E/h/.config/opencode/plugins/devexp.js"
make_stub "$E/stubs/a" A
run_uninstall DEVEXP_BIN="$E/stubs/a" STUB_RC=1
check "a failing plugin removal still exits 0" rc_is 0
check "a failing plugin removal is reported with a re-run hint" out_has "opencode plugin left in place (see above) — re-run: $E/stubs/a uninstall --target opencode"

# ── Order: plugin removal before the MCP block rewrites config.json ──────────
new_env
printf '# agent\n' > "$E/r/agents/some-agent.md"
mkdir -p "$E/h/.config/opencode/agents"
printf '# agent\n' > "$E/h/.config/opencode/agents/some-agent.md"
printf '{"mcp":{}}' > "$E/h/.config/opencode/config.json"
make_stub "$E/stubs/a" A
run_uninstall DEVEXP_BIN="$E/stubs/a"
plugin_line="$(grep -nF 'STUB[A] uninstall --target opencode --yes' "$E/out" | head -1 | cut -d: -f1)"
mcp_line="$(grep -nF 'Removing MCP servers (opencode)' "$E/out" | head -1 | cut -d: -f1)"
check "agents install: exit 0" rc_is 0
check "agents install: agents removed" test ! -e "$E/h/.config/opencode/agents/some-agent.md"
check "the plugin is removed before the MCP block runs" test -n "$plugin_line" -a -n "$mcp_line" -a "${plugin_line:-0}" -lt "${mcp_line:-0}"

# ── Crash regressions ────────────────────────────────────────────────────────
# A config.json the MCP step can't save used to abort the script (exit 1)
# before the Claude Code hooks were removed. Choice [3] removes from both CLIs,
# so every later step must still run.
for ro in file dir; do
    new_env
    mkdir -p "$E/r/hooks/claude-code" "$E/h/.claude/agents" "$E/h/.config/opencode/agents"
    printf '[{"name": "secret-guard", "enabled": true, "claude_code": {"event": "PreToolUse", "matcher": "Read", "script": "hooks/claude-code/secret-guard.sh"}}]' > "$E/r/hooks/registry.json"
    printf '[{"name": "context7"}]' > "$E/r/mcps/registry.json"
    printf '# agent\n' > "$E/r/agents/some-agent.md"
    printf '# agent\n' > "$E/h/.claude/agents/some-agent.md"
    printf '# agent\n' > "$E/h/.config/opencode/agents/some-agent.md"
    printf '{"hooks":{"PreToolUse":[{"matcher":"Read","hooks":[{"type":"command","command":"%s/hooks/claude-code/secret-guard.sh"}]}]}}' "$E/r" > "$E/h/.claude/settings.json"
    cfg='{"mcp":{"context7":{}}}'
    printf '%s' "$cfg" > "$E/h/.config/opencode/config.json"
    if [ "$ro" = file ]; then chmod 444 "$E/h/.config/opencode/config.json"; else chmod 555 "$E/h/.config/opencode"; fi
    if [ "$ro" = dir ] && ( : > "$E/h/.config/opencode/.probe" ) 2>/dev/null; then
        rm -f "$E/h/.config/opencode/.probe"; chmod 755 "$E/h/.config/opencode"
        echo "SKIP uninstall.sh with a read-only config dir (permissions not enforced, running as root?)"
        continue
    fi
    make_stub "$E/stubs/a" A
    printf '3\n' > "$E/menu"
    STDIN_FILE="$E/menu" run_uninstall DEVEXP_BIN="$E/stubs/a"
    chmod 755 "$E/h/.config/opencode"; chmod 644 "$E/h/.config/opencode/config.json"
    check "read-only config ($ro): exit 0" rc_is 0
    check "read-only config ($ro): a warning, no traceback" out_lacks "Traceback"
    check "read-only config ($ro): says it was left untouched" out_has "left untouched"
    check "read-only config ($ro): config.json unchanged" test "$(cat "$E/h/.config/opencode/config.json")" = "$cfg"
    check "read-only config ($ro): the later Claude Code hook step still ran" \
        test "$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("hooks"))' "$E/h/.claude/settings.json")" = "{}"
    check "read-only config ($ro): both CLIs' agents removed" \
        test ! -e "$E/h/.claude/agents/some-agent.md" -a ! -e "$E/h/.config/opencode/agents/some-agent.md"
    check "read-only config ($ro): the run reaches the end" out_has "Uninstall complete."
done

new_env
touch "$E/h/.config/opencode/plugins/devexp.js"
printf '{not json' > "$E/h/.config/opencode/config.json"
make_stub "$E/stubs/a" A
run_uninstall DEVEXP_BIN="$E/stubs/a"
check "malformed config.json: exit 0" rc_is 0
check "malformed config.json: no top-level local error" out_lacks "can only be used in a function"
check "malformed config.json: no python traceback" out_lacks "Traceback"
check "malformed config.json: left as it was" test "$(cat "$E/h/.config/opencode/config.json")" = "{not json"
check "malformed config.json: the run reaches the end" out_has "Uninstall complete."

# ── HOME refusal (#126) ──────────────────────────────────────────────────────
# With HOME unset, empty or relative, every path would point at / or under the
# current directory. uninstall.sh refuses before it looks at anything: a
# dotfiles-style tree in the cwd — with a Claude Code devexp install at the top
# and under home/ — stays byte for byte as it was, and no binary is called.
# Claude Code only, so an unguarded run removes without stopping at the menu.
tree_sum() { # $1=dir -> every path, then every file's checksum
    (cd "$1" && find . -print | LC_ALL=C sort && find . -type f -exec cksum {} + | LC_ALL=C sort)
}
for home_case in unset empty relative; do
    new_env
    printf '# agent\n' > "$E/r/agents/some-agent.md"
    C="$E/cwd"
    for d in "$C" "$C/home"; do
        mkdir -p "$d/.claude/agents" "$d/.claude/skills/some-skill" "$d/.config/opencode"
        printf '# agent\n' > "$d/.claude/agents/some-agent.md"
        printf '# skill\n' > "$d/.claude/skills/some-skill/SKILL.md"
        printf '{"hooks":{}}' > "$d/.claude/settings.json"
        printf '{"mcp":{}}' > "$d/.config/opencode/config.json"
    done
    make_stub "$E/stubs/a" A
    case "$home_case" in
        unset)    home_env=() ;;
        empty)    home_env=(HOME=) ;;
        relative) home_env=(HOME=home) ;;
    esac
    before="$(tree_sum "$C")"
    (cd "$C" && env -i ${home_env[@]+"${home_env[@]}"} PATH="$E/bin:/usr/bin:/bin" CALLS="$E/calls" DEVEXP_BIN="$E/stubs/a" \
        /bin/bash "$E/r/uninstall.sh" --yes </dev/null > "$E/out" 2>&1; echo $? > "$E/rc")
    check "HOME $home_case: exits non-zero" test "$(cat "$E/rc")" != 0
    check "HOME $home_case: says why" out_has "not an absolute path — refusing to remove anything; set HOME and re-run"
    check "HOME $home_case: the current directory is unchanged" test "$(tree_sum "$C")" = "$before"
    check "HOME $home_case: the binary is not called" calls_are ""
done

# ── Round trip with the real binary ──────────────────────────────────────────
if command -v go >/dev/null 2>&1 && [ -d "$ROOT/cli/internal/assets/hooks" ]; then
    new_env
    R="$E/repo"; H="$E/h"; OC="$H/.config/opencode"; P="$OC/plugins"
    mkdir -p "$R"
    cp -R "$ROOT/.devexp-toolkit" "$ROOT/agents" "$ROOT/skills" "$ROOT/hooks" "$ROOT/mcps" "$ROOT/devexp.config.json" "$ROOT/uninstall.sh" "$R/"
    printf '#!/bin/sh\nexit 0\n' > "$E/bin/opencode"; chmod +x "$E/bin/opencode"
    if (cd "$ROOT/cli" && go build -o "$E/devexp" .) > "$E/out" 2>&1 \
        && env -i HOME="$H" PATH="$E/bin:/usr/bin:/bin" DEVEXP_DIR="$R" "$E/devexp" install --reinstall-mcps > "$E/out" 2>&1; then
        cp "$ROOT/cli/internal/hooks/testdata/legacy-opencode/"* "$P/"
        printf 'export const mine = async () => ({})\n' > "$P/my-plugin.js"
        python3 - "$OC/config.json" "$P" <<'PY'
import json, sys
p = sys.argv[1]
c = json.load(open(p))
c["theme"] = "x"
c["plugin"] = [sys.argv[2] + "/devexp-plugin.js", "npm-plugin"]
json.dump(c, open(p, "w"))
PY
        env -i HOME="$H" PATH="$E/bin:/usr/bin:/bin" DEVEXP_BIN="$E/devexp" \
            /bin/bash "$R/uninstall.sh" --yes </dev/null > "$E/out" 2>&1
        echo $? > "$E/rc"
        check "round trip: exit 0" rc_is 0
        check "round trip: only the foreign plugin is left" test "$(ls -A "$P")" = "my-plugin.js"
        check "round trip: config.json keeps its own plugin and theme" test \
            "$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(c.get("theme"), c.get("plugin"))' "$OC/config.json")" = "x ['npm-plugin']"
        check "round trip: the manifest no longer records plugins" test \
            "$(python3 -c 'import json,sys; print("plugins" in json.load(open(sys.argv[1])))' "$OC/.devexp-manifest.json")" = "False"
    else
        ko "round trip: build or install failed" "$(cat "$E/out")"
    fi
else
    echo "SKIP round trip with the real binary (needs go and ./scripts/stage-assets.sh)"
fi

# ── Claude Code round trip with the real binary (#137) ───────────────────────
# User hooks carrying timeout, args, shell, async and an unknown field, plus an
# exec-form handler naming a devexp script: `devexp install` adds devexp's
# hooks and keeps them, a re-install changes nothing, and uninstall.sh takes
# settings.json back to the exact bytes it started as.
if command -v go >/dev/null 2>&1 && [ -d "$ROOT/cli/internal/assets/hooks" ]; then
    new_env
    R="$E/repo"; H="$E/h"; S="$H/.claude/settings.json"
    mkdir -p "$R" "$H/.claude"
    cp -R "$ROOT/.devexp-toolkit" "$ROOT/agents" "$ROOT/skills" "$ROOT/hooks" "$ROOT/mcps" "$ROOT/devexp.config.json" "$ROOT/uninstall.sh" "$R/"
    printf '#!/bin/sh\nexit 0\n' > "$E/bin/claude"; chmod +x "$E/bin/claude"
    python3 - "$S" "$R" <<'PY'
import sys
settings, repo = sys.argv[1:3]
open(settings, 'w', encoding='utf-8').write('''{
  "model": "opus",
  "env": {"NOTE": "caf\\u00e9 & <tea>"},
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "REPO/hooks/claude-code/dangerous-cmd-guard.sh",
            "args": [
              "--strict"
            ],
            "timeout": 5
          },
          {
            "type": "command",
            "command": "/usr/local/bin/audit-log",
            "timeout": 30,
            "async": true,
            "shell": "bash",
            "x-future": {
              "level": 2
            }
          }
        ]
      }
    ],
    "Stop": []
  }
}
'''.replace('REPO', repo))
PY
    cp "$S" "$E/before.json"
    cc_install() { env -i HOME="$H" PATH="$E/bin:/usr/bin:/bin" DEVEXP_DIR="$R" "$E/devexp" install --reinstall-mcps > "$E/out" 2>&1; }
    if (cd "$ROOT/cli" && go build -o "$E/devexp" .) > "$E/out" 2>&1 && cc_install; then
        check "cc round trip: install keeps the user's handlers and adds devexp's" python3 - "$E/before.json" "$S" "$R" <<'PY'
import json, sys
before, after, repo = (json.load(open(sys.argv[1])), json.load(open(sys.argv[2])), sys.argv[3])
user = before['hooks']['PreToolUse'][0]
got = after['hooks']['PreToolUse']
mine = [h['command'] for e in got[1:] for h in e['hooks']]
sys.exit(0 if got[0] == user and after['hooks']['Stop'] == []
         and repo + '/hooks/claude-code/dangerous-cmd-guard.sh' in mine
         and {k: v for k, v in after.items() if k != 'hooks'} == {k: v for k, v in before.items() if k != 'hooks'} else 1)
PY
        cp "$S" "$E/installed.json"
        if cc_install; then ok; else ko "cc round trip: re-install" "$(cat "$E/out")"; fi
        check "cc round trip: re-install leaves settings.json byte for byte" cmp -s "$S" "$E/installed.json"
        env -i HOME="$H" PATH="$E/bin:/usr/bin:/bin" /bin/bash "$R/uninstall.sh" --yes </dev/null > "$E/out" 2>&1
        echo $? > "$E/rc"
        check "cc round trip: uninstall exit 0" rc_is 0
        check "cc round trip: uninstall restores settings.json byte for byte" cmp -s "$S" "$E/before.json"
    else
        ko "cc round trip: build or install failed" "$(cat "$E/out")"
    fi
else
    echo "SKIP Claude Code round trip with the real binary (needs go and ./scripts/stage-assets.sh)"
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
