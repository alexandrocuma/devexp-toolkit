#!/usr/bin/env bash
# Tests the scan budget every fail-closed security guard runs under (#162).
#
# Claude Code does not block a tool call when a command hook times out, and its
# default timeout for one is 600 seconds, so a guard that is slow on some input
# used to fail OPEN. The budget makes the guard exit 2 first. What has to hold:
#
#   - an input forced over budget blocks, with exit status 2 and a message that
#     names the budget;
#   - the kill reaches the scan already running, not just the space around it;
#   - ordinary input is untouched, and a guard's own block still reads as its
#     own reason;
#   - a value that is not a non-negative integer is ignored, so a typo can
#     neither widen the budget nor disable the guard;
#   - a run that is neither allow (0) nor block (2) blocks;
#   - the guard's body runs exactly once under the wrapper.
#
# Run: bash hooks/claude-code/scan-budget.test.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

GUARDS="secret-guard dangerous-cmd-guard secret-in-write-guard"

# An envelope each guard reads and lets through, and one each guard blocks on
# its own merits. The secret-shaped one is assembled from pieces so this file
# holds nothing the installed guard would refuse to write.
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
        secret-in-write-guard)
            python3 -I -c '
import json
key = "AKIA" + "Q" * 8 + "7" * 8
print(json.dumps({"tool_name": "Write", "tool_input": {"file_path": "a.txt", "content": key}}))' ;;
    esac
}

# run <guard> <budget|-> <envelope> -> prints "rc|stdout|stderr"
run() {
    local guard="$1" budget="$2" envelope="$3" out err rc
    err="$TMP/err"
    if [ "$budget" = "-" ]; then
        out=$(printf '%s' "$envelope" | bash "$DIR/$guard.sh" 2>"$err"); rc=$?
    else
        out=$(printf '%s' "$envelope" | DEVEXP_SCAN_BUDGET_MS="$budget" bash "$DIR/$guard.sh" 2>"$err"); rc=$?
    fi
    printf '%s|%s|%s' "$rc" "$out" "$(cat "$err")"
}

check() { # $1=label  $2=condition-result(0/1)  $3=detail
    if [ "$2" = 0 ]; then
        pass=$((pass+1))
    else
        fail=$((fail+1)); printf 'FAIL %s: %s\n' "$1" "$3"
    fi
}

# ── A forced budget hit blocks, loudly, with nothing on stdout ───────────────
# 0 is "already over budget": the guard blocks before it scans at all. 1 ms
# goes the other way — the scan starts and the deadline kills it — so both the
# short circuit and the watchdog are covered.
for guard in $GUARDS; do
    for budget in 0 1; do
        got=$(run "$guard" "$budget" "$(allow_envelope "$guard")")
        rc=${got%%|*}; rest=${got#*|}; out=${rest%%|*}; err=${rest#*|}
        ok=1
        case "$rc|$out" in
            "2|") case "$err" in *"[devexp $guard] Blocked:"*"budget"*) ok=0 ;; esac ;;
        esac
        check "budget $budget blocks $guard" "$ok" "rc=$rc stdout=$(printf '%.40s' "$out") stderr=$(printf '%.90s' "$err")"
    done
done

# ── The deadline interrupts a scan already under way ─────────────────────────
# A write big enough to take about a second is cut off well inside a 300 ms
# budget, so the budget is enforced during the scan and not merely around it.
big=$(python3 -I -c '
import json
def rep(unit, n):
    return (unit * (n // len(unit) + 1))[:n]
head = "-" * 5 + "BEGIN RSA PRIVATE KEY" + "-" * 5 + "\n"
print(json.dumps({"tool_name": "Write", "tool_input": {"file_path": "big.txt",
      "content": rep(head + rep("A" * 31 + ".", 600), 2000000)}}))')
# time.monotonic() has no common reference across processes on every platform,
# so the two readings are wall clock.
start=$(python3 -I -c 'import time; print(time.time())')
got=$(run secret-in-write-guard 300 "$big")
elapsed=$(python3 -I -c 'import sys, time; print("%.2f" % (time.time() - float(sys.argv[1])))' "$start")
rc=${got%%|*}; err=${got##*|}
ok=1
case "$rc" in 2) case "$err" in *"did not finish within its 0.3 s budget"*)
    [ "$(python3 -I -c 'import sys; print(1 if float(sys.argv[1]) < 3 else 0)' "$elapsed")" = 1 ] && ok=0 ;; esac ;; esac
check "a 300 ms budget cuts off a scan of about a second" "$ok" "rc=$rc after ${elapsed}s: $(printf '%.90s' "$err")"

# The same write is allowed under the real budget, so the case above is the
# budget biting and not the content.
got=$(run secret-in-write-guard - "$big")
check "the same write passes under the default budget" \
    "$([ "${got%%|*}" = 0 ] && echo 0 || echo 1)" "rc=${got%%|*} ${got##*|}"

# ── Ordinary input is untouched ─────────────────────────────────────────────
for guard in $GUARDS; do
    got=$(run "$guard" - "$(allow_envelope "$guard")")
    check "$guard still allows ordinary input" \
        "$([ "$got" = "0||" ] && echo 0 || echo 1)" "got $got"

    got=$(run "$guard" - "$(block_envelope "$guard")")
    rc=${got%%|*}; err=${got##*|}
    ok=1
    case "$rc" in 2) case "$err" in
        *budget*) ;;                                   # must be the guard's reason, not the budget's
        *"[devexp $guard] Blocked"*) ok=0 ;;
    esac ;; esac
    check "$guard still blocks on its own reason" "$ok" "rc=$rc $(printf '%.90s' "$err")"
done

# ── A value that is not a non-negative integer is ignored ────────────────────
# Falling back to the default is the only safe reading: a typo must not widen
# the budget, and must not block every call either.
for bad in "abc" "-1" "1e3" "10s" "" " " "1.5" "0x10"; do
    got=$(run dangerous-cmd-guard "$bad" "$(allow_envelope dangerous-cmd-guard)")
    check "budget \"$bad\" falls back to the default (allows)" \
        "$([ "$got" = "0||" ] && echo 0 || echo 1)" "got $got"
done
got=$(run dangerous-cmd-guard "abc" "$(block_envelope dangerous-cmd-guard)")
check 'budget "abc" still blocks a destructive command' \
    "$([ "${got%%|*}" = 2 ] && echo 0 || echo 1)" "rc=${got%%|*}"

# ── The two twins carry the same default ────────────────────────────────────
sh_default=$(sed -n 's/^DEVEXP_SCAN_BUDGET_DEFAULT_MS=\([0-9]*\)$/\1/p' "$DIR/scan-budget.sh")
js_default=$(sed -n 's/^export const SCAN_BUDGET_DEFAULT_MS = \([0-9]*\);$/\1/p' "$DIR/../opencode/utils.js")
check "both twins default to the same budget" \
    "$([ -n "$sh_default" ] && [ "$sh_default" = "$js_default" ] && echo 0 || echo 1)" \
    "shell=$sh_default js=$js_default"

# ── Every fail-closed guard is registered with a timeout above the budget ────
# A timed-out command hook does not block the tool call, so Claude Code must
# never cancel a guard before the guard's own budget can.
registry_check=$(python3 -I -c '
import json, sys
registry, budget_ms = json.load(open(sys.argv[1])), int(sys.argv[2])
bad = [h["name"] for h in registry
       if h.get("opencode", {}).get("fail_closed")
       and h.get("claude_code", {}).get("timeout", 0) * 1000 <= budget_ms]
n = sum(1 for h in registry if h.get("opencode", {}).get("fail_closed"))
print("%d %s" % (n, ",".join(bad) or "-"))' "$DIR/../registry.json" "$sh_default")
check "every fail-closed guard has a timeout above the budget" \
    "$([ "$registry_check" = "3 -" ] && echo 0 || echo 1)" "got \"$registry_check\", want \"3 -\""

# ── A run that is neither allow nor block, blocks ────────────────────────────
# The wrapper is the only thing between a slow scan and an unscanned call, so a
# guard whose decision it cannot read must not be taken for "allowed".
cat > "$TMP/odd-guard.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
. "$DIR/scan-budget.sh"
devexp_scan_budget odd-guard "\$@"
cat >/dev/null
exit "\${ODD_RC:-7}"
EOF
for rc_in in 7 1 127; do
    err="$TMP/odd-err"
    ODD_RC="$rc_in" bash "$TMP/odd-guard.sh" </dev/null >/dev/null 2>"$err"; rc=$?
    ok=1
    [ "$rc" = 2 ] && case "$(cat "$err")" in *"neither allow (0) nor block (2)"*) ok=0 ;; esac
    check "a guard exiting $rc_in blocks" "$ok" "rc=$rc $(printf '%.90s' "$(cat "$err")")"
done

# ── A zero budget blocks without starting the scan at all ───────────────────
# Not merely "blocks": the guard is spent before it begins, so nothing it would
# have done gets a chance to happen first.
cat > "$TMP/marker-guard.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
. "$DIR/scan-budget.sh"
devexp_scan_budget marker-guard "\$@"
: > "$TMP/marker"
cat >/dev/null
exit 0
EOF
rm -f "$TMP/marker"
err="$TMP/marker-err"
DEVEXP_SCAN_BUDGET_MS=0 bash "$TMP/marker-guard.sh" </dev/null >/dev/null 2>"$err"; rc=$?
ok=1
[ "$rc" = 2 ] && [ ! -e "$TMP/marker" ] && case "$(cat "$err")" in *budget*) ok=0 ;; esac
check "a zero budget blocks before the guard body runs" "$ok" \
    "rc=$rc marker=$([ -e "$TMP/marker" ] && echo present || echo absent)"

# ── The watchdog's own failure blocks ───────────────────────────────────────
# It is the only thing between a slow scan and an unscanned tool call, so if it
# cannot even start — no python3 on PATH — the call is refused, not allowed.
mkdir -p "$TMP/nopython"
for guard in $GUARDS; do
    err="$TMP/nopython-err"
    # bash by its own path: PATH here holds nothing at all.
    printf '%s' "$(allow_envelope "$guard")" | PATH="$TMP/nopython" "${BASH:-/bin/bash}" "$DIR/$guard.sh" >/dev/null 2>"$err"; rc=$?
    ok=1
    [ "$rc" = 2 ] && case "$(cat "$err")" in *"scan budget could not run"*) ok=0 ;; esac
    check "$guard blocks when the budget itself cannot run" "$ok" "rc=$rc $(printf '%.90s' "$(cat "$err")")"
done

# ── The guard's body runs exactly once ──────────────────────────────────────
cat > "$TMP/once-guard.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
. "$DIR/scan-budget.sh"
devexp_scan_budget once-guard "\$@"
cat >/dev/null
echo ran >> "$TMP/ran"
exit 0
EOF
: > "$TMP/ran"
bash "$TMP/once-guard.sh" </dev/null >/dev/null 2>&1; rc=$?
runs=$(wc -l < "$TMP/ran" | tr -d ' ')
check "the guard body runs once and its status is passed through" \
    "$([ "$rc" = 0 ] && [ "$runs" = 1 ] && echo 0 || echo 1)" "rc=$rc runs=$runs"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
