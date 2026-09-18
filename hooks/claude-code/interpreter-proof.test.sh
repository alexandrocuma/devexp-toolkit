#!/usr/bin/env bash
# Tests that a fail-closed guard allows only against proof that its own
# scanning code ran (#168).
#
# The guards do their parsing and matching in a program they run through an
# interpreter found on PATH, and the shell can read only that program's exit
# status. So anything on PATH that reports success without doing the work —
# a wrapper, a shim, a broken virtualenv, a stand-in that does nothing — used
# to read as "the scan found nothing", and the tool call went through
# unscanned. The scan budget (#162) widened it: the watchdog runs through the
# same interpreter, so a stand-in answers before the guard has read the
# envelope at all.
#
# What has to hold, for every guard the registry marks fail_closed:
#
#   - an interpreter that reports success without running the guard's program
#     blocks, however it reports it — silently, with plausible output, by
#     echoing its arguments or its input, or by running something else;
#   - an interpreter honest at one level and not the other blocks too, so
#     neither the watchdog nor the guard's own scan can vouch for the other;
#   - a marker that is not this invocation's own is no proof;
#   - `grep`, whose status is the only thing dangerous-cmd-guard's `matches`
#     reads, has to answer a question with a known answer before it is trusted;
#   - the healthy paths are untouched: ordinary input is allowed in silence and
#     a dangerous one is blocked with the guard's own reason.
#
# Run: bash hooks/claude-code/interpreter-proof.test.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

GUARDS="secret-guard dangerous-cmd-guard secret-in-write-guard"
REAL_PYTHON="$(command -v python3)"

check() { # $1=label  $2=condition-result(0/1)  $3=detail
    if [ "$2" = 0 ]; then
        pass=$((pass+1))
    else
        fail=$((fail+1)); printf 'FAIL %s: %s\n' "$1" "$3"
    fi
}

# An envelope each guard lets through, and one each guard blocks on its own
# merits. The secret-shaped one is assembled from pieces, so this file holds
# nothing the installed guard would refuse to write.
allow_envelope() { # $1=guard
    case "$1" in
        secret-guard)          printf '{"tool_name":"Bash","tool_input":{"command":"cat README.md"}}' ;;
        dangerous-cmd-guard)   printf '{"tool_name":"Bash","tool_input":{"command":"ls -la"}}' ;;
        secret-in-write-guard) printf '{"tool_name":"Write","tool_input":{"file_path":"a.txt","content":"hello world"}}' ;;
    esac
}
block_envelope() { # $1=guard
    case "$1" in
        secret-guard)          printf '{"tool_name":"Read","tool_input":{"file_path":".env"}}' ;;
        dangerous-cmd-guard)   printf '{"tool_name":"Bash","tool_input":{"command":"git push --force"}}' ;;
        secret-in-write-guard) printf '{"tool_name":"Write","tool_input":{"file_path":"a.txt","content":"%s"}}' \
                                   "AKIA$(printf 'Q%.0s' 1 2 3 4 5 6 7 8)$(printf '7%.0s' 1 2 3 4 5 6 7 8)" ;;
    esac
}

stub() { # $1=name  $2=program it stands in for  $3=body
    mkdir -p "$TMP/$1"
    printf '#!/bin/sh\n%s\n' "$3" > "$TMP/$1/$2"
    chmod +x "$TMP/$1/$2"
}

run() { # $1=stub dir  $2=guard  $3=envelope -> prints "rc|stdout|stderr"
    local out rc err="$TMP/err"
    out=$(printf '%s' "$3" | PATH="$TMP/$1:$PATH" bash "$DIR/$2.sh" 2>"$err"); rc=$?
    printf '%s|%s|%s' "$rc" "$out" "$(cat "$err")"
}

# A guard that cannot trust its scan must block and say why. Whatever the
# stand-in itself prints is its own noise, not the guard's — the guard's own
# silence on an allow is checked further down, on the healthy path.
blocks() { # $1=stub  $2=guard  $3=envelope  $4=label
    local got rc out err ok=1
    got=$(run "$1" "$2" "$3")
    rc=${got%%|*}; out=${got#*|}; err=${out#*|}; out=${out%%|*}
    if [ "$rc" = 2 ]; then
        case "$err" in *"[devexp $2]"*"Blocking to be safe"*) ok=0 ;; esac
    fi
    check "$4: $2 blocks" "$ok" "rc=$rc stdout=$(printf '%.40s' "$out") stderr=$(printf '%.110s' "$err")"
}

# ── An interpreter that answers without running the guard's program ─────────
# Each of these exits 0, which before #168 was read as "the scan found
# nothing". None of them runs a line of the guard's code.
stub silent      python3 'exit 0'
stub chatty      python3 'echo
echo "ok"
exit 0'
stub echo-argv   python3 'echo "$@"
exit 0'
stub echo-stdin  python3 'cat
exit 0'
stub other       python3 "exec $REAL_PYTHON -c 'import sys; sys.exit(0)'"
# Honest for the watchdog (which passes -S) and a stand-in for the guard's own
# scan: proof at one level must not vouch for the other.
stub half        python3 "for a in \"\$@\"; do
  [ \"\$a\" = -S ] && exec $REAL_PYTHON \"\$@\"
done
exit 0"
# A marker that is not this invocation's own is no proof: plausible text, and
# the arguments it was handed, on both channels the guards read.
stub guess       python3 'echo "devexp-scanned"
echo "$@"
{ echo "devexp-scanned"; echo "$@"; } >&9 2>/dev/null
exit 0'

for stub_name in silent chatty echo-argv echo-stdin other half guess; do
    for guard in $GUARDS; do
        blocks "$stub_name" "$guard" "$(block_envelope "$guard")" "an interpreter that $stub_name"
        # The allow envelope matters more than the block one: this is the call
        # that would have gone through unscanned.
        blocks "$stub_name" "$guard" "$(allow_envelope "$guard")" "an interpreter that $stub_name (ordinary input)"
    done
done

# The guard adds nothing of its own to stdout while refusing: the block is
# said on stderr, where Claude Code reads a guard's reason.
for guard in $GUARDS; do
    got=$(run silent "$guard" "$(allow_envelope "$guard")")
    rc=${got%%|*}; out=${got#*|}; out=${out%%|*}
    check "$guard says nothing on stdout when it cannot trust its scan" \
        "$([ "$rc" = 2 ] && [ -z "$out" ] && echo 0 || echo 1)" "rc=$rc stdout=$(printf '%.60s' "$out")"
done

# An interpreter that fails outright was already covered by the 0-or-2 rule;
# it stays covered.
stub quiet-fail python3 'exit 1'
stub says-block python3 'exit 2'
for stub_name in quiet-fail says-block; do
    for guard in $GUARDS; do
        got=$(run "$stub_name" "$guard" "$(allow_envelope "$guard")")
        check "an interpreter that $stub_name: $guard blocks" \
            "$([ "${got%%|*}" = 2 ] && echo 0 || echo 1)" "rc=${got%%|*} $(printf '%.90s' "${got##*|}")"
    done
done

# ── grep's answer is proved, not assumed ────────────────────────────────────
# `matches` reads grep's exit status and nothing else, so a grep that reports
# "no match" for everything would report every command as clean. v0.9.1 made a
# grep *error* block; "no match" was still believed.
stub grep-never  grep 'exit 1'
stub grep-always grep 'exit 0'
stub grep-error  grep 'exit 3'
stub grep-lies   grep 'echo 7
exit 0'
for stub_name in grep-never grep-always grep-error grep-lies; do
    blocks "$stub_name" dangerous-cmd-guard "$(block_envelope dangerous-cmd-guard)" "a $stub_name"
    blocks "$stub_name" dangerous-cmd-guard "$(allow_envelope dangerous-cmd-guard)" "a $stub_name (ordinary input)"
done
# The reason has to name grep, not the interpreter: the two failures are told
# apart so the message points at what actually broke.
got=$(run grep-never dangerous-cmd-guard "$(allow_envelope dangerous-cmd-guard)")
ok=1; case "${got##*|}" in *grep*) ok=0 ;; esac
check "a lying grep is named as the reason" "$ok" "$(printf '%.110s' "${got##*|}")"

# ── The healthy paths are untouched ─────────────────────────────────────────
mkdir -p "$TMP/clean"
for guard in $GUARDS; do
    got=$(run clean "$guard" "$(allow_envelope "$guard")")
    check "$guard still allows ordinary input, in silence" \
        "$([ "$got" = "0||" ] && echo 0 || echo 1)" "got $(printf '%.90s' "$got")"

    got=$(run clean "$guard" "$(block_envelope "$guard")")
    rc=${got%%|*}; err=${got##*|}
    ok=1
    [ "$rc" = 2 ] && case "$err" in
        *"internal error"*) ;;                    # its own reason, not a proof failure
        *"[devexp $guard] Blocked"*) ok=0 ;;
    esac
    check "$guard still blocks on its own reason" "$ok" "rc=$rc $(printf '%.90s' "$err")"
done

# ── Every fail-closed guard asks for the proof ──────────────────────────────
# A new guard that forgets to is exactly the hole this closes, and it would
# pass every other suite.
registered=$(python3 -I -c '
import json, sys
print(" ".join(h["name"] for h in json.load(open(sys.argv[1]))
                if h.get("opencode", {}).get("fail_closed")))' "$DIR/../registry.json")
check "the fail-closed guards are the ones tested here" \
    "$([ "$registered" = "$GUARDS" ] && echo 0 || echo 1)" "registry says \"$registered\", this file tests \"$GUARDS\""
for guard in $registered; do
    ok=1
    grep -q 'devexp_scan_result' "$DIR/$guard.sh" &&
        grep -q 'DEVEXP_SCAN_PROOF' "$DIR/$guard.sh" && ok=0
    check "$guard requires proof from its own scan" "$ok" "no devexp_scan_result/DEVEXP_SCAN_PROOF in $guard.sh"
done

# The proof channel is spelled twice — a redirection needs a literal number —
# so the two spellings have to agree or the watchdog's proof goes nowhere.
fd_const=$(sed -n 's/^DEVEXP_SCAN_BUDGET_PROOF_FD=\([0-9]*\)$/\1/p' "$DIR/scan-budget.sh")
fd_redir=$(sed -n 's/.*[^0-9]\([0-9]\)>&1 1>&8.*/\1/p' "$DIR/scan-budget.sh")
check "the proof descriptor and its redirection agree" \
    "$([ -n "$fd_const" ] && [ "$fd_const" = "$fd_redir" ] && echo 0 || echo 1)" \
    "constant=$fd_const redirection=$fd_redir"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
