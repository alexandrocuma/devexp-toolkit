#!/usr/bin/env bash
# Tests for hooks/kimi/adapter.sh — exit 2 = blocked, exit 0 = allowed.
# Run: bash hooks/kimi/adapter.test.sh
#
# Kimi reads exit 2 as a block (stderr is the reason) and EVERY other exit, a
# spawn failure and a timeout as an allow, so "the adapter fell over" and "the
# guard allowed" must never look alike. Each case below therefore pins the
# exact status, and the fail-closed ones pin that it is 2.
#
# No fixture here is a real credential: the secret-shaped and destructive ones
# are assembled at run time from pieces, so nothing in this file is a usable
# key or a runnable destructive command.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ADAPTER="$HERE/adapter.sh"
GUARDS="$(cd "$HERE/.." && pwd)/claude-code"
SECRET_GUARD="$GUARDS/secret-guard.sh"
DANGER_GUARD="$GUARDS/dangerous-cmd-guard.sh"
WRITE_GUARD="$GUARDS/secret-in-write-guard.sh"

TMP=$(mktemp -d "${TMPDIR:-/tmp}/devexp-kimi-adapter.XXXXXX") || exit 1
trap 'rm -rf "$TMP"' EXIT

pass=0; fail=0
OUT="$TMP/stdout"; ERR="$TMP/stderr"; RC=0

note() { # $1=label
  fail=$((fail+1)); printf 'FAIL %s\n' "$1"
}

# ── fixtures, assembled from pieces ─────────────────────────────────────────
DOTENV=$(printf '%s%s' '.' 'env')
PRIVATE_KEY_FILE=$(printf '%s%s' 'id_' 'rsa')
# An AWS-shaped access key id: the prefix, then 16 of [0-9A-Z]. Never issued.
AWS_KEY=$(printf '%s%s%s%s' 'AKI' 'A' 'IOSFODNN7' 'MKLPQ12')
# A destructive command, kept in pieces so this file never contains it whole.
DESTRUCTIVE=$(printf '%s%s' 'rm -r' 'f /')

# ── helpers ─────────────────────────────────────────────────────────────────

# A Kimi PreToolUse envelope: camelCase, tool arguments under toolInput.
# ── the envelope Kimi actually writes ────────────────────────────────────────
# matchHooks.ts builds the PreToolUse payload camelCase and then runs
# toHookInputData over it — Object.entries + camelToSnake, ONE level deep —
# before runHook ever spawns the command. So the top level arrives snake_case
# and `tool_input` arrives exactly as the tool named its arguments. Both
# helpers below start from the same pre-conversion object; `envelope` applies
# Kimi's conversion, `camel_envelope` does not, so the adapter is held to both
# spellings.
KIMI_ENVELOPE_PY='
import json, sys

def camel_to_snake(key):  # Kimi: value.replaceAll(/[A-Z]/g, ch => `_${ch.toLowerCase()}`)
    out = []
    for ch in key:
        if "A" <= ch <= "Z":
            out.append("_" + ch.lower())
        else:
            out.append(ch)
    return "".join(out)

args = {}
for pair in sys.argv[3:]:
    key, _, value = pair.partition("=")
    args[key] = value
raw = {
    "hookEventName": "PreToolUse",
    "sessionId": "session-abc",
    "clientType": "cli",
    "sessionTitle": "a session",
    "toolName": sys.argv[2],
    "toolInput": args,
    "toolCallId": "call-1",
}
if sys.argv[1] == "converted":
    raw = {camel_to_snake(k): v for k, v in raw.items()}
print(json.dumps(raw))
'

envelope() { # $1=toolName  $2..=key=value — what Kimi really sends
  python3 -c "$KIMI_ENVELOPE_PY" converted "$@"
}

camel_envelope() { # $1=toolName  $2..=key=value — the pre-conversion shape
  python3 -c "$KIMI_ENVELOPE_PY" raw "$@"
}

run() { # $1=guard-or-empty  $2=payload on stdin
  printf '%s' "$2" | bash "$ADAPTER" ${1:+"$1"} >"$OUT" 2>"$ERR"
  RC=$?
}

expect() { # $1=label  $2=want rc  $3=guard  $4=payload
  run "$3" "$4"
  if [ "$RC" = "$2" ]; then pass=$((pass+1)); else
    note "$1 [want rc $2, got $RC] $(head -c 200 "$ERR")"
  fi
}

expect_env() { # $1=label  $2=want rc  $3=guard  $4=payload  $5..=VAR=VALUE
  local label="$1" want="$2" guard="$3" payload="$4"; shift 4
  printf '%s' "$payload" | env "$@" bash "$ADAPTER" "$guard" >"$OUT" 2>"$ERR"
  RC=$?
  if [ "$RC" = "$want" ]; then pass=$((pass+1)); else
    note "$label [want rc $want, got $RC] $(head -c 200 "$ERR")"
  fi
}

check() { # $1=label  $2=condition result (0 ok)
  if [ "$2" = 0 ]; then pass=$((pass+1)); else note "$1"; fi
}

stub() { # $1=name — reads the script body on stdin, prints its path
  local path="$TMP/$1.sh"
  cat >"$path"
  chmod +x "$path"
  printf '%s' "$path"
}

# ── per guarded tool: one block, one allow ──────────────────────────────────

# secret-guard, via Read (Kimi's `path` -> Claude Code's `file_path`)
expect 'secret-guard Read blocks a dotenv'  2 "$SECRET_GUARD" "$(envelope Read "path=$DOTENV")"
expect 'secret-guard Read blocks a key'     2 "$SECRET_GUARD" "$(envelope Read "path=$HOME/.ssh/$PRIVATE_KEY_FILE")"
expect 'secret-guard Read allows a readme'  0 "$SECRET_GUARD" "$(envelope Read 'path=README.md')"

# secret-guard, via ReadMediaFile — mapped onto Read, or the registry's
# ^(Read|ReadMediaFile|Bash)$ matcher would run a scan with no case for it.
expect 'secret-guard ReadMediaFile blocks a dotenv' 2 "$SECRET_GUARD" "$(envelope ReadMediaFile "path=$DOTENV")"
expect 'secret-guard ReadMediaFile allows an image' 0 "$SECRET_GUARD" "$(envelope ReadMediaFile 'path=docs/diagram.png')"

# secret-guard, via Bash
expect 'secret-guard Bash blocks a dotenv read' 2 "$SECRET_GUARD" "$(envelope Bash "command=cat $DOTENV")"
expect 'secret-guard Bash allows a readme read' 0 "$SECRET_GUARD" "$(envelope Bash 'command=cat README.md')"

# dangerous-cmd-guard, via Bash
expect 'dangerous-cmd-guard blocks a destructive rm' 2 "$DANGER_GUARD" "$(envelope Bash "command=$DESTRUCTIVE")"
expect 'dangerous-cmd-guard blocks a force push'     2 "$DANGER_GUARD" "$(envelope Bash 'command=git push --force')"
expect 'dangerous-cmd-guard allows an ordinary push'  0 "$DANGER_GUARD" "$(envelope Bash 'command=git push origin main')"

# secret-in-write-guard, via Write (`path` + `content`)
expect 'secret-in-write-guard blocks a key in Write content' 2 "$WRITE_GUARD" \
  "$(envelope Write 'path=config.ts' "content=const id = \"$AWS_KEY\";")"
expect 'secret-in-write-guard allows ordinary Write content' 0 "$WRITE_GUARD" \
  "$(envelope Write 'path=README.md' 'content=# Title')"

# secret-in-write-guard, via Edit (`old_string` / `new_string` keep their names)
expect 'secret-in-write-guard blocks a key in Edit new_string' 2 "$WRITE_GUARD" \
  "$(envelope Edit 'path=config.ts' 'old_string=const id = "";' "new_string=const id = \"$AWS_KEY\";")"
expect 'secret-in-write-guard allows an ordinary Edit' 0 "$WRITE_GUARD" \
  "$(envelope Edit 'path=README.md' 'old_string=# Title' 'new_string=# Better title')"

# ── a malformed or incomplete envelope is a block, never an allow ───────────
expect 'malformed JSON blocks'            2 "$SECRET_GUARD" '{"toolName": "Read", '
expect 'empty stdin blocks'               2 "$SECRET_GUARD" ''
expect 'a JSON array blocks'              2 "$SECRET_GUARD" '[{"toolName": "Read"}]'
expect 'a JSON string blocks'             2 "$SECRET_GUARD" '"Read"'
expect 'a missing toolName blocks'        2 "$SECRET_GUARD" '{"toolInput": {"path": "README.md"}}'
expect 'an empty toolName blocks'         2 "$SECRET_GUARD" '{"toolName": "", "toolInput": {}}'
expect 'a non-string toolName blocks'     2 "$SECRET_GUARD" '{"toolName": 7, "toolInput": {}}'
expect 'a missing toolInput blocks'       2 "$SECRET_GUARD" '{"toolName": "Read"}'
expect 'a non-object toolInput blocks'    2 "$SECRET_GUARD" '{"toolName": "Read", "toolInput": "README.md"}'
# The same refusals against the spelling Kimi really sends. Both have to fail
# closed, or the adapter would block on one shape and wave through the other.
expect 'a missing tool_name blocks'       2 "$SECRET_GUARD" '{"tool_input": {"path": "README.md"}}'
expect 'an empty tool_name blocks'        2 "$SECRET_GUARD" '{"tool_name": "", "tool_input": {}}'
expect 'a non-string tool_name blocks'    2 "$SECRET_GUARD" '{"tool_name": 7, "tool_input": {}}'
expect 'a missing tool_input blocks'      2 "$SECRET_GUARD" '{"tool_name": "Read"}'
expect 'a non-object tool_input blocks'   2 "$SECRET_GUARD" '{"tool_name": "Read", "tool_input": "README.md"}'
# Neither spelling present at all — the shape a third Kimi rename would send.
expect 'neither spelling blocks'          2 "$SECRET_GUARD" '{"toolname": "Read", "toolinput": {}}'
# And the one that matters most: the real envelope must NOT be refused.
expect 'the real envelope is not refused' 0 "$SECRET_GUARD" "$(envelope Read 'path=README.md')"
run "$SECRET_GUARD" '{"toolName": "Read", '
check 'a malformed envelope says why on stderr' \
  "$(grep -q 'devexp kimi-adapter' "$ERR" && echo 0 || echo 1)"

# ── an unmapped tool passes through and is not blocked on its way ───────────
expect 'an unmapped tool is allowed through'  0 "$SECRET_GUARD" "$(envelope Glob 'pattern=**/*.ts')"
expect 'an unmapped MCP tool is allowed'      0 "$SECRET_GUARD" "$(envelope mcp__linear__issue 'id=ABC-1')"

# ── the adapter's own failures are blocks ───────────────────────────────────
expect 'no guard on the command line blocks' 2 '' "$(envelope Read 'path=README.md')"
expect 'a missing guard script blocks'       2 "$TMP/not-a-guard.sh" "$(envelope Read 'path=README.md')"
expect 'a directory as the guard blocks'     2 "$TMP" "$(envelope Read 'path=README.md')"

# ── a guard exit that is neither 0 nor 2 has no decision, so it blocks ──────
EXIT1=$(stub exit1 <<'SH'
#!/usr/bin/env bash
cat >/dev/null
echo "something went wrong" >&2
exit 1
SH
)
EXIT127=$(stub exit127 <<'SH'
#!/usr/bin/env bash
cat >/dev/null
exit 127
SH
)
SIGNALLED=$(stub signalled <<'SH'
#!/usr/bin/env bash
cat >/dev/null
kill -9 $$
SH
)
expect 'a guard that exits 1 blocks'          2 "$EXIT1" "$(envelope Read 'path=README.md')"
expect 'a guard that exits 127 blocks'        2 "$EXIT127" "$(envelope Read 'path=README.md')"
expect 'a guard killed by a signal blocks'    2 "$SIGNALLED" "$(envelope Read 'path=README.md')"
run "$EXIT1" "$(envelope Read 'path=README.md')"
check "an unknown guard exit says so on stderr" \
  "$(grep -q 'neither allow (0) nor block (2)' "$ERR" && echo 0 || echo 1)"

# ── verdicts carried on the guard's stdout ──────────────────────────────────
ASK=$(stub ask <<'SH'
#!/usr/bin/env bash
cat >/dev/null
echo '{"hookSpecificOutput": {"permissionDecision": "ask"}, "systemMessage": "[devexp large-file-guard] About to overwrite \"big.ts\" (900 lines)."}'
exit 0
SH
)
DENY=$(stub deny <<'SH'
#!/usr/bin/env bash
cat >/dev/null
echo '{"hookSpecificOutput": {"permissionDecision": "deny", "permissionDecisionReason": "no"}}'
exit 0
SH
)
DECISION_BLOCK=$(stub decision-block <<'SH'
#!/usr/bin/env bash
cat >/dev/null
echo '{"decision": "block", "reason": "legacy shape"}'
exit 0
SH
)
ALLOW_QUIET=$(stub allow-quiet <<'SH'
#!/usr/bin/env bash
cat >/dev/null
exit 0
SH
)
ALLOW_MESSAGE=$(stub allow-message <<'SH'
#!/usr/bin/env bash
cat >/dev/null
echo '{"hookSpecificOutput": {"permissionDecision": "allow"}, "systemMessage": "scanned, nothing found"}'
exit 0
SH
)
ALLOW_TEXT=$(stub allow-text <<'SH'
#!/usr/bin/env bash
cat >/dev/null
echo 'just talking to the user'
exit 0
SH
)
PAYLOAD=$(envelope Write 'path=big.ts' 'content=x')

expect 'an ask verdict becomes a block'        2 "$ASK" "$PAYLOAD"
run "$ASK" "$PAYLOAD"
check 'the ask block carries the guard reason' \
  "$(grep -q 'About to overwrite' "$ERR" && echo 0 || echo 1)"
check 'the ask block says Kimi runs an ask as an allow' \
  "$(grep -q 'runs an "ask" as an allow' "$ERR" && echo 0 || echo 1)"
check 'the ask verdict does not reach stdout' "$([ ! -s "$OUT" ] && echo 0 || echo 1)"

expect 'a deny verdict on stdout blocks'       2 "$DENY" "$PAYLOAD"
expect 'a legacy decision:block on stdout blocks' 2 "$DECISION_BLOCK" "$PAYLOAD"
expect 'a quiet allow allows'                  0 "$ALLOW_QUIET" "$PAYLOAD"
run "$ALLOW_QUIET" "$PAYLOAD"
check 'a quiet allow says nothing on stdout' "$([ ! -s "$OUT" ] && echo 0 || echo 1)"

expect 'an allow with a message allows'        0 "$ALLOW_MESSAGE" "$PAYLOAD"
run "$ALLOW_MESSAGE" "$PAYLOAD"
check "an allow's message reaches Kimi's schema" \
  "$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(0 if d.get("message")=="scanned, nothing found" else 1)' "$OUT" && echo 0 || echo 1)"

expect 'an allow with plain text allows'       0 "$ALLOW_TEXT" "$PAYLOAD"
run "$ALLOW_TEXT" "$PAYLOAD"
check 'plain stdout is handed over as a message' \
  "$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(0 if d.get("message")=="just talking to the user" else 1)' "$OUT" && echo 0 || echo 1)"

# ── the scan budget still bites, and a budget hit is still a block ──────────
# A guard that hangs past its budget: the watchdog in scan-budget.sh kills its
# process group and exits 2, and the adapter must carry that through as a
# block rather than let Kimi read a late or unknown status as an allow.
HANG=$(stub hang <<SH
#!/usr/bin/env bash
set -euo pipefail
. "$GUARDS/scan-budget.sh"
devexp_scan_budget hang-stub "\$@"
cat >/dev/null
sleep 30
exit 0
SH
)
expect_env 'a guard that hangs past its budget blocks' 2 "$HANG" "$PAYLOAD" \
  DEVEXP_SCAN_BUDGET_MS=400
printf '%s' "$PAYLOAD" | env DEVEXP_SCAN_BUDGET_MS=400 bash "$ADAPTER" "$HANG" >"$OUT" 2>"$ERR"
check 'the budget hit says the scan did not finish' \
  "$(grep -q 'budget' "$ERR" && echo 0 || echo 1)"

# A real guard with no budget left blocks without scanning — through the
# adapter exactly as it does on its own.
expect_env 'a real guard over budget blocks' 2 "$SECRET_GUARD" \
  "$(envelope Read 'path=README.md')" DEVEXP_SCAN_BUDGET_MS=0

# ── what the guard actually receives ────────────────────────────────────────
RECORD=$(stub record <<SH
#!/usr/bin/env bash
cat >"$TMP/seen.json"
printf '%s' "\$#" >"$TMP/argc"
{ printf 'proof=%s\n' "\${DEVEXP_SCAN_PROOF-unset}"
  printf 'fd=%s\n' "\${DEVEXP_SCAN_BUDGET_PROOF_FD-unset}"
  printf 'depth=%s\n' "\${DEVEXP_SCAN_BUDGET_DEPTH-unset}"; } >"$TMP/env"
exit 0
SH
)
# The real envelope first: snake_case at the top level, Kimi's own names
# inside tool_input. `somethingNew` stands for a key a later Kimi adds and
# converts; `unmappedArg` for a tool argument, which Kimi never converts.
REAL='{"hook_event_name": "PreToolUse", "session_id": "s-1", "tool_call_id": "call-9",
       "client_type": "cli", "session_title": "a session", "cwd": "/repo",
       "something_new": {"a": 1},
       "tool_name": "Read",
       "tool_input": {"path": "src/app.ts", "line_offset": 10, "n_lines": 20,
                      "unmappedArg": "kept"}}'
# The same facts unconverted. Kimi does not send this today; the adapter
# accepts it so a change on Kimi's side cannot turn every tool call into a
# block, which is what reading runHook without its caller once cost us.
CAMEL='{"hookEventName": "PreToolUse", "sessionId": "s-1", "toolCallId": "call-9",
        "clientType": "cli", "sessionTitle": "a session", "cwd": "/repo",
        "somethingNew": {"a": 1},
        "toolName": "Read",
        "toolInput": {"path": "src/app.ts", "line_offset": 10, "n_lines": 20,
                      "unmappedArg": "kept"}}'

TRANSLATION_PY='
import json, sys
d = json.load(open(sys.argv[1]))
problems = []
def want(key, value):
    if d.get(key) != value:
        problems.append("%s=%r" % (key, d.get(key)))
want("tool_name", "Read")
want("tool_call_id", "call-9")
want("session_id", "s-1")
want("hook_event_name", "PreToolUse")
want("client_type", "cli")
want("session_title", "a session")
want("cwd", "/repo")
# A top-level key with no Claude Code meaning still arrives snake_case: that
# is Kimi own rule, and the adapter applies the same one.
want("something_new", {"a": 1})
ti = d.get("tool_input", {})
if ti.get("file_path") != "src/app.ts": problems.append("file_path=%r" % ti.get("file_path"))
if "path" in ti: problems.append("path survived the rename")
if ti.get("line_offset") != 10: problems.append("line_offset dropped")
# Nothing inside tool_input is renamed, because Kimi renames nothing there.
if ti.get("unmappedArg") != "kept": problems.append("unmappedArg dropped")
for camel in ("toolName", "toolInput", "toolCallId", "sessionId", "somethingNew"):
    if camel in d: problems.append("%s not renamed" % camel)
if problems:
    sys.stderr.write("; ".join(problems) + "\n")
    sys.exit(1)
'

for spelling in real camel; do
  case "$spelling" in
    real)  PAYLOAD_IN="$REAL" ;;
    camel) PAYLOAD_IN="$CAMEL" ;;
  esac
  run "$RECORD" "$PAYLOAD_IN"
  check "a $spelling-spelled envelope is accepted" "$([ "$RC" = 0 ] && echo 0 || echo 1)"
  check "a $spelling-spelled envelope is translated faithfully" \
    "$(python3 -c "$TRANSLATION_PY" "$TMP/seen.json" && echo 0 || echo 1)"
done

# Both spellings in one envelope. It cannot happen today, but the order the
# adapter reads them in is a stated invariant — snake_case first, because that
# is what Kimi sends — and swapping it must not pass unnoticed.
run "$RECORD" '{"hook_event_name": "PreToolUse",
                "tool_name": "Read",  "toolName": "Bash",
                "tool_input": {"path": "snake.ts"},
                "toolInput": {"command": "echo camel"}}'
check 'with both spellings present, snake_case wins' \
  "$(python3 -c '
import json, sys
d = json.load(open(sys.argv[1]))
problems = []
if d.get("tool_name") != "Read":
    problems.append("tool_name=%r, want the snake_case value" % d.get("tool_name"))
ti = d.get("tool_input", {})
if ti.get("file_path") != "snake.ts":
    problems.append("tool_input came from the camelCase key: %r" % ti)
if "command" in ti:
    problems.append("the camelCase toolInput won")
if problems:
    sys.stderr.write("; ".join(problems) + "\n")
    sys.exit(1)
' "$TMP/seen.json" && echo 0 || echo 1)"

# The regression itself: the exact shape Kimi sends must not be refused.
run "$RECORD" "$(envelope Read 'path=src/app.ts')"
check 'the envelope helper produces what Kimi sends, and it is accepted' \
  "$([ "$RC" = 0 ] && echo 0 || echo 1)"
check 'that envelope reaches the guard as tool_name/file_path' \
  "$(python3 -c '
import json, sys
d = json.load(open(sys.argv[1]))
sys.exit(0 if d.get("tool_name") == "Read"
         and d.get("tool_input", {}).get("file_path") == "src/app.ts" else 1)' \
    "$TMP/seen.json" && echo 0 || echo 1)"
check 'the guard is run with no arguments' \
  "$([ "$(cat "$TMP/argc")" = 0 ] && echo 0 || echo 1)"
check 'the adapter does not mint the scan proof' \
  "$(grep -qx 'proof=unset' "$TMP/env" && grep -qx 'fd=unset' "$TMP/env" && echo 0 || echo 1)"
check 'the adapter does not pre-set the budget depth' \
  "$(grep -qx 'depth=unset' "$TMP/env" && echo 0 || echo 1)"

# A Bash envelope keeps its own argument names.
run "$RECORD" "$(envelope Bash 'command=ls -la' 'cwd=/repo' 'run_in_background=no')"
check 'Bash arguments keep their names' \
  "$(python3 -c '
import json, sys
ti = json.load(open(sys.argv[1]))["tool_input"]
sys.exit(0 if ti.get("command") == "ls -la" and ti.get("cwd") == "/repo"
         and ti.get("run_in_background") == "no" else 1)' "$TMP/seen.json" && echo 0 || echo 1)"

# An unmapped tool keeps its name and its arguments untouched.
run "$RECORD" "$(envelope Glob 'pattern=**/*.ts' 'path=src')"
check 'an unmapped tool keeps its name and path argument' \
  "$(python3 -c '
import json, sys
d = json.load(open(sys.argv[1]))
ti = d["tool_input"]
sys.exit(0 if d["tool_name"] == "Glob" and ti.get("path") == "src"
         and "file_path" not in ti else 1)' "$TMP/seen.json" && echo 0 || echo 1)"

# An envelope that already carries file_path keeps both, rather than one
# overwriting the other.
run "$RECORD" '{"toolName": "Write", "toolInput": {"path": "a.ts", "file_path": "b.ts"}}'
check 'an existing file_path is not overwritten' \
  "$(python3 -c '
import json, sys
ti = json.load(open(sys.argv[1]))["tool_input"]
sys.exit(0 if ti.get("file_path") == "b.ts" and ti.get("path") == "a.ts" else 1)' \
    "$TMP/seen.json" && echo 0 || echo 1)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
