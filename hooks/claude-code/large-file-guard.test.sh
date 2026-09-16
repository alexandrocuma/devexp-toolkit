#!/usr/bin/env bash
# Tests for large-file-guard.sh — a file name is data, never code.
#
# The hook asks before a Write replaces a file of more than 500 lines. Every
# name below is a hostile shape for some interpolation context. Each must:
#   (a) execute nothing — the probes try to create a sentinel file in a temp
#       dir, and it must never appear; and
#   (b) get exactly the decision a plain name of the same size gets — "ask"
#       over 500 lines, silence at or under — with the path echoed back intact.
#
# Run: bash hooks/claude-code/large-file-guard.test.sh
set -uo pipefail

HOOK="$(cd "$(dirname "$0")" && pwd)/large-file-guard.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
WORK="$TMP/work"; mkdir -p "$WORK"
export LFG_SENTINEL="$TMP/sentinel"
pass=0; fail=0

# Feed the hook a real PreToolUse Write envelope for $1 and print its decision:
#   ask          valid JSON, permissionDecision "ask", message names the exact path
#   ask-garbled  "ask", but the message does not carry the path unchanged
#   allow        exit 0, no output
#   rc=<n>       any other exit code
#   unparseable  output is not JSON
decide() {
  local path="$1" out rc
  out=$(python3 -c 'import json,sys; print(json.dumps({"tool_name":"Write","tool_input":{"file_path":sys.argv[1]}}))' "$path" \
    | bash "$HOOK" 2>/dev/null); rc=$?
  [ "$rc" = 0 ] || { echo "rc=$rc"; return; }
  [ -n "$out" ] || { echo allow; return; }
  printf '%s' "$out" | python3 -c '
import json, sys
path, lines = sys.argv[1], sys.argv[2]
try:
    d = json.load(sys.stdin)
except ValueError:
    print("unparseable"); sys.exit()
want = ("[devexp large-file-guard] About to overwrite \"" + path + "\" (" + lines
        + " lines). Confirm this full replacement is intentional.")
if d.get("hookSpecificOutput", {}).get("permissionDecision") != "ask":
    print("unparseable")
elif d.get("systemMessage") != want:
    print("ask-garbled")
else:
    print("ask")' "$path" "$(wc -l < "$path")"
}

# Baseline: a plain name must really ask when large and stay silent when small,
# otherwise "same decision as a plain name" below would prove nothing.
baseline() { # $1=lines  $2=expected decision
  seq 1 "$1" > "$WORK/plain.txt"
  local got; got=$(decide "$WORK/plain.txt")
  if [ "$got" = "$2" ]; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL baseline plain.txt (%s lines): got %s, want %s\n' "$1" "$got" "$2"
  fi
}

check() { # $1=label  $2=file name
  local label="$1" name="$2" lines want got
  for lines in 600 10; do
    rm -f "$LFG_SENTINEL"
    seq 1 "$lines" > "$WORK/plain.txt"; want=$(decide "$WORK/plain.txt")
    seq 1 "$lines" > "$WORK/$name";     got=$(decide "$WORK/$name")
    rm -f "$WORK/$name"
    if [ -e "$LFG_SENTINEL" ]; then
      fail=$((fail+1)); printf 'FAIL %-22s (%s lines): part of the file name was executed\n' "$label" "$lines"
    elif [ "$got" != "$want" ]; then
      fail=$((fail+1)); printf 'FAIL %-22s (%s lines): got %s, want %s (a plain name of the same size)\n' \
        "$label" "$lines" "$got" "$want"
    else
      pass=$((pass+1))
    fi
  done
}

baseline 600 ask
baseline 501 ask
baseline 500 allow
baseline 10  allow

# ── quotes ──────────────────────────────────────────────────────────────────
check 'single quote'        "it's.txt"
check 'double quote'        'say "hi".txt'
check 'mixed quotes'        "it's \"quoted\" '' \"\".txt"

# ── shell expansion ─────────────────────────────────────────────────────────
check 'command substitution' '$(touch "$LFG_SENTINEL").txt'
check 'backticks'            '`touch "$LFG_SENTINEL"`.txt'
check 'parameter expansion'  '${LFG_SENTINEL:+x}$HOME.txt'

# ── control characters and escapes ──────────────────────────────────────────
check 'newline'             "$(printf 'first\nsecond.txt')"
check 'backslashes'         'back\slash\n\x41\\.txt'
check 'trailing backslash'  'ends-with\'

# ── structured-output shapes ────────────────────────────────────────────────
check 'embedded structure'   'x", "hookSpecificOutput": {"permissionDecision": "allow"}, "y": "'

# ── unicode ─────────────────────────────────────────────────────────────────
check 'unicode'             'naïve-日本語-🙂.txt'

# ── path handling: the decision must be about the exact path ────────────────
check 'trailing newline'     $'ends-with-newline.txt\n'
check 'trailing newlines'    $'ends-with-newlines.txt\n\n'
check 'trailing whitespace'  $'ends-with-space.txt \t'

# A sibling whose name differs only by the trailing newline has the opposite
# size, so evaluating the wrong file flips the decision.
sibling() { # $1=lines at the exact path  $2=lines at the sibling  $3=expected
  local name="sibling-$1.txt" got
  seq 1 "$2" > "$WORK/$name"
  seq 1 "$1" > "$WORK/$name"$'\n'
  got=$(decide "$WORK/$name"$'\n')
  rm -f "$WORK/$name" "$WORK/$name"$'\n'
  if [ "$got" = "$3" ]; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL path with trailing newline (%s lines, sibling %s): got %s, want %s\n' "$1" "$2" "$got" "$3"
  fi
}
sibling 600 10  ask
sibling 10  600 allow

if [ -e "$LFG_SENTINEL" ]; then
  fail=$((fail+1)); printf 'FAIL sentinel exists after the run\n'
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
