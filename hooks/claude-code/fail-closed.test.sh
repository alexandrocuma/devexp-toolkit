#!/usr/bin/env bash
# Tests that no hook fails OPEN silently when its interpreter errors.
#
# Every hook extracts its decision input by piping the tool envelope through
# python3. If that call fails, the old code collapsed the result to an empty
# string, which is indistinguishable from "ran fine, nothing to block" — so the
# hook exited 0 and allowed the operation, saying nothing.
#
# Security guards must now fail CLOSED (exit 2). Advisory hooks may fail open,
# but must say so. Either way the failure must be visible.
#
# Run: bash hooks/claude-code/fail-closed.test.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
pass=0; fail=0

# Feed the hook something json.load cannot parse, so the extractor genuinely
# crashes — the same shape as a syntax error or a missing interpreter.
BROKEN='this is not json'

check() { # $1=hook name  $2=expected exit code  $3=guard|advisory
    local hook="$1" want="$2" kind="$3" out rc
    out=$(printf '%s' "$BROKEN" | bash "$DIR/$hook.sh" 2>&1); rc=$?

    if [ "$rc" != "$want" ]; then
        fail=$((fail+1))
        printf 'FAIL %-26s exit %s, want %s (%s must fail %s)\n' \
            "$hook" "$rc" "$want" "$kind" \
            "$([ "$want" = 2 ] && echo closed || echo 'open but loud')"
        return
    fi
    if ! printf '%s' "$out" | grep -q 'internal error'; then
        fail=$((fail+1))
        printf 'FAIL %-26s exit %s but said nothing — a silent failure\n' "$hook" "$rc"
        return
    fi
    pass=$((pass+1))
}

# ── Security guards: must fail CLOSED ───────────────────────────────────────
# An empty extraction makes every pattern check trivially pass, so allowing
# here would permit exactly what the guard exists to stop.
check secret-guard            2 guard   # reading dotenv and private-key files
check secret-in-write-guard   2 guard   # writing API keys and tokens
check dangerous-cmd-guard     2 guard   # rm -rf, git push --force

# ── Advisory hooks: may fail open, but must say so ──────────────────────────
# Blocking every Write because a formatter's parser broke would wedge the user,
# and the downside is only that formatting/linting/testing did not happen.
check large-file-guard        0 advisory
check format-on-save          0 advisory
check lint-on-save            0 advisory
check test-on-save            0 advisory
check comment-refs-on-save    0 advisory

# ── The happy path must be untouched ────────────────────────────────────────
# A well-formed envelope with nothing to block still exits 0 and stays silent.
quiet() { # $1=hook  $2=envelope
    local out rc
    out=$(printf '%s' "$2" | bash "$DIR/$1.sh" 2>&1); rc=$?
    if [ "$rc" = 0 ] && ! printf '%s' "$out" | grep -q 'internal error'; then
        pass=$((pass+1))
    else
        fail=$((fail+1)); printf 'FAIL %-26s clean input should exit 0 quietly (exit %s)\n' "$1" "$rc"
    fi
}
quiet dangerous-cmd-guard   '{"tool_name":"Bash","tool_input":{"command":"ls -la"}}'
quiet secret-guard          '{"tool_name":"Bash","tool_input":{"command":"cat README.md"}}'
quiet secret-in-write-guard '{"tool_name":"Write","tool_input":{"content":"hello world"}}'

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
