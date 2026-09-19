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
#   - the guard's body runs exactly once under the wrapper;
#   - every way the prologue can fail blocks, before any budget exists;
#   - the budget cannot be switched off from the environment;
#   - a budget above the ceiling is clamped, not honoured.
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
# U+0663 and U+00B2 are the ones that matter: Python's str.isdigit() accepts
# both, so before #167 one of them set a 3 ms budget that blocked every call and
# the other raised inside the watchdog. The JS twin's \d never did.
for bad in "abc" "-1" "1e3" "10s" "" " " "1.5" "0x10" "٣" "²" "12 34"; do
    got=$(run dangerous-cmd-guard "$bad" "$(allow_envelope dangerous-cmd-guard)")
    check "budget \"$bad\" falls back to the default (allows)" \
        "$([ "$got" = "0||" ] && echo 0 || echo 1)" "got $got"
done
got=$(run dangerous-cmd-guard "abc" "$(block_envelope dangerous-cmd-guard)")
check 'budget "abc" still blocks a destructive command' \
    "$([ "${got%%|*}" = 2 ] && echo 0 || echo 1)" "rc=${got%%|*}"

# Surrounding spaces are trimmed, as the JS twin trims them: "  0  " is a zero
# budget on both sides, not a typo that falls back to the default.
got=$(run dangerous-cmd-guard "  0  " "$(allow_envelope dangerous-cmd-guard)")
check 'budget "  0  " is trimmed and read as 0' \
    "$([ "${got%%|*}" = 2 ] && echo 0 || echo 1)" "rc=${got%%|*} ${got##*|}"

# ── The two twins carry the same default ────────────────────────────────────
sh_default=$(sed -n 's/^DEVEXP_SCAN_BUDGET_DEFAULT_MS=\([0-9]*\)$/\1/p' "$DIR/scan-budget.sh")
js_default=$(sed -n 's/^export const SCAN_BUDGET_DEFAULT_MS = \([0-9]*\);$/\1/p' "$DIR/../opencode/utils.js")
check "both twins default to the same budget" \
    "$([ -n "$sh_default" ] && [ "$sh_default" = "$js_default" ] && echo 0 || echo 1)" \
    "shell=$sh_default js=$js_default"

# ── A budget above the ceiling is clamped, out loud ──────────────────────────
# The one knob the docs advertise could otherwise reinstate the bug this guards
# against: above the registered hook timeout, Claude Code cancels the guard
# first, and a cancelled command hook does not block the tool call.
sh_max=$(sed -n 's/^DEVEXP_SCAN_BUDGET_MAX_MS=\([0-9]*\)$/\1/p' "$DIR/scan-budget.sh")
js_max=$(sed -n 's/^export const SCAN_BUDGET_MAX_MS = \([0-9]*\);$/\1/p' "$DIR/../opencode/utils.js")
check "both twins cap the budget at the same ceiling" \
    "$([ -n "$sh_max" ] && [ "$sh_max" = "$js_max" ] && echo 0 || echo 1)" "shell=$sh_max js=$js_max"
check "the default is within the ceiling" \
    "$([ "$sh_default" -le "$sh_max" ] && echo 0 || echo 1)" "default=$sh_default max=$sh_max"

got=$(run dangerous-cmd-guard 600000 "$(allow_envelope dangerous-cmd-guard)")
rc=${got%%|*}; err=${got##*|}
ok=1
case "$rc" in 0) case "$err" in *"DEVEXP_SCAN_BUDGET_MS of 600 s"*"Using 44 s"*) ok=0 ;; esac ;; esac
check "a budget of 600 s is clamped to the ceiling, with a notice" "$ok" "rc=$rc $(printf '%.130s' "$err")"

got=$(run dangerous-cmd-guard "$sh_max" "$(allow_envelope dangerous-cmd-guard)")
check "a budget at the ceiling passes without a notice" \
    "$([ "$got" = "0||" ] && echo 0 || echo 1)" "got $(printf '%.90s' "$got")"

# The cap is not just announced, it takes effect. Watching a 44 s budget bite
# would mean waiting 44 s, so the ceiling is lowered for this one case -- it may
# only ever be lowered, never raised, so the seam cannot undo the cap.
err="$TMP/clamp-err"
out=$(printf '%s' "$big" | DEVEXP_SCAN_BUDGET_CEILING_MS=300 DEVEXP_SCAN_BUDGET_MS=60000 \
    bash "$DIR/secret-in-write-guard.sh" 2>"$err"); rc=$?
ok=1
[ "$rc" = 2 ] && case "$(cat "$err")" in *"within its 0.3 s budget"*) ok=0 ;; esac
check "a clamped budget is the one actually enforced" "$ok" \
    "rc=$rc $(printf '%.110s' "$(cat "$err")")"

# And the seam is one-way: an ambient ceiling above the real one changes nothing.
out=$(printf '%s' "$(allow_envelope dangerous-cmd-guard)" \
    | DEVEXP_SCAN_BUDGET_CEILING_MS=600000 DEVEXP_SCAN_BUDGET_MS=600000 \
      bash "$DIR/dangerous-cmd-guard.sh" 2>"$err"); rc=$?
ok=1
[ "$rc" = 0 ] && case "$(cat "$err")" in *"Using 44 s"*) ok=0 ;; esac
check "the ceiling seam cannot raise the ceiling" "$ok" \
    "rc=$rc $(printf '%.110s' "$(cat "$err")")"

# ── Every fail-closed guard is registered with a timeout above the budget ────
# A timed-out command hook does not block the tool call, so Claude Code must
# never cancel a guard before the guard's own budget can.
# The ceiling, not just the default, has to clear every registered timeout:
# the ceiling is the largest budget a guard can actually run with.
registry_check=$(python3 -I -c '
import json, sys
registry, budget_ms = json.load(open(sys.argv[1])), int(sys.argv[2])
bad = [h["name"] for h in registry
       if h.get("opencode", {}).get("fail_closed")
       and h.get("claude_code", {}).get("timeout", 0) * 1000 <= budget_ms]
# Kimi runs the same guards under the same budget, and its own default hook
# timeout is 30 s -- below the ceiling -- so a kimi block that left the timeout
# out would have its guard killed mid-scan. Kimi reads a killed hook as an
# allow, so the timeout is spelled out in the block, never defaulted.
bad += ["kimi:" + h["name"] for h in registry
        if h.get("kimi", {}).get("fail_closed")
        and h.get("kimi", {}).get("timeout", 0) * 1000 <= budget_ms]
n = sum(1 for h in registry if h.get("opencode", {}).get("fail_closed"))
print("%d %s" % (n, ",".join(bad) or "-"))' "$DIR/../registry.json" "$sh_max")
check "every fail-closed guard has a timeout above the ceiling" \
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

# ── Every way the prologue can fail blocks ──────────────────────────────────
# The window before devexp_scan_budget exists has no watchdog under it, and an
# `if ! . …` cannot floor it: under `set -e` bash leaves the script where the
# `.` failed, and for several of these it then reports 0 to an EXIT trap --
# which reads as "allow". So each case is checked for exit 2 by itself.
SAND="$TMP/sandbox"
mkdir -p "$SAND"
cp "$DIR/secret-guard.sh" "$SAND/secret-guard.sh"

prologue_case() { # $1=label  $2=expected message fragment ("-" for any)  $3=noise that must not appear
    local label="$1" want="$2" noise="${3:-}" out rc
    out=$(printf '%s' "$(allow_envelope secret-guard)" | bash "$SAND/secret-guard.sh" 2>&1); rc=$?
    local ok=1
    if [ "$rc" = 2 ]; then
        case "$want" in
            -) ok=0 ;;
            *) case "$out" in *"$want"*) ok=0 ;; esac ;;
        esac
    fi
    # The guard has to say what happened itself. A shell error reaching the user
    # instead means the case was caught by the floor rather than named.
    if [ "$ok" = 0 ] && [ -n "$noise" ]; then
        case "$out" in *"$noise"*) ok=1 ;; esac
    fi
    check "prologue: $label blocks" "$ok" "rc=$rc $(printf '%.100s' "$out")"
    rm -rf "$SAND/scan-budget.sh"
}

# Sanity: with the helper in place the sandboxed copy behaves normally.
cp "$DIR/scan-budget.sh" "$SAND/scan-budget.sh"
out=$(printf '%s' "$(allow_envelope secret-guard)" | bash "$SAND/secret-guard.sh" 2>&1); rc=$?
check "prologue: the sandbox copy allows ordinary input" \
    "$([ "$rc" = 0 ] && [ -z "$out" ] && echo 0 || echo 1)" "rc=$rc $out"
rm -f "$SAND/scan-budget.sh"

prologue_case "a missing helper" "could not read its scan budget helper"
ln -s /no/such/target "$SAND/scan-budget.sh"
prologue_case "an unreadable helper" "could not read its scan budget helper"
printf 'x=1\n' > "$SAND/scan-budget.sh"
prologue_case "a helper with no entry point" "defines no entry point" "command not found"
printf 'if then fi\n' > "$SAND/scan-budget.sh"
prologue_case "a helper bash cannot parse" -
mkdir "$SAND/scan-budget.sh"
prologue_case "a helper that is a directory" -
# The entry point without the constants above it: `command -v` is satisfied, so
# a floor lifted on the way into the function would already be gone, and `set -u`
# would end the guard at exit 1 -- which Claude Code reads as non-blocking.
sed -n '/^devexp_scan_budget()/,$p' "$DIR/scan-budget.sh" > "$SAND/scan-budget.sh"
prologue_case "a helper with the entry point but not its constants" "could not run its scan budget"

# ── The budget cannot be switched off from the environment ──────────────────
# The marker that tells the budgeted run apart travels in argv, not the
# environment: a settings `env` entry, a shell profile or a CI image would
# otherwise silently disable every guard's budget.
cp "$DIR/scan-budget.sh" "$SAND/scan-budget.sh"
for var in DEVEXP_SCAN_BUDGET_RUNNING DEVEXP_SCAN_BUDGET_SENTINEL DEVEXP_BUDGET_CHILD; do
    for value in 1 --devexp-budgeted; do
        out=$(printf '%s' "$(allow_envelope secret-guard)" \
            | env "$var=$value" DEVEXP_SCAN_BUDGET_MS=0 bash "$SAND/secret-guard.sh" 2>&1); rc=$?
        ok=1
        [ "$rc" = 2 ] && case "$out" in *budget*) ok=0 ;; esac
        check "an ambient $var=$value does not disable the budget" "$ok" "rc=$rc $(printf '%.80s' "$out")"
    done
done

# ── Re-entry is bounded, and bounded by blocking ────────────────────────────
# The sentinel is the only thing that stops a budgeted run from starting a
# watchdog of its own, and each watchdog kills only its own child's process
# group, in a session of its own -- so if the sentinel ever stopped being
# recognised the guard would fork without bound. The depth counter is read from
# the environment on purpose: the only thing it can do is block.
for depth in 1 2 9; do
    out=$(printf '%s' "$(allow_envelope secret-guard)" \
        | DEVEXP_SCAN_BUDGET_DEPTH="$depth" bash "$SAND/secret-guard.sh" 2>&1); rc=$?
    ok=1
    [ "$rc" = 2 ] && case "$out" in *"re-entered itself"*) ok=0 ;; esac
    check "an ambient depth of $depth blocks" "$ok" "rc=$rc $(printf '%.80s' "$out")"
done
for depth in 0 "" "abc" "-1"; do
    out=$(printf '%s' "$(allow_envelope secret-guard)" \
        | DEVEXP_SCAN_BUDGET_DEPTH="$depth" bash "$SAND/secret-guard.sh" 2>&1); rc=$?
    check "an ambient depth of \"$depth\" reads as none" \
        "$([ "$rc" = 0 ] && [ -z "$out" ] && echo 0 || echo 1)" "rc=$rc $(printf '%.80s' "$out")"
done

# The guard itself, with the sentinel deliberately broken: it has to stop, and
# stop quickly, rather than multiply.
BROKEN="$TMP/broken"
mkdir -p "$BROKEN"
cp "$DIR/secret-guard.sh" "$DIR/scan-budget.sh" "$BROKEN/"
python3 -I - "$BROKEN/scan-budget.sh" <<'MUTPY' || fail=$((fail+1))
import sys
path = sys.argv[1]
text = open(path).read()
anchor = 'if [ "$arg" = "$DEVEXP_SCAN_BUDGET_SENTINEL" ]; then'
assert text.count(anchor) == 1, "the sentinel check moved; update this test"
open(path, "w").write(text.replace(anchor, "if false; then", 1))
MUTPY
start=$(python3 -I -c 'import time; print(time.time())')
out=$(printf '%s' "$(allow_envelope secret-guard)" | bash "$BROKEN/secret-guard.sh" 2>&1); rc=$?
elapsed=$(python3 -I -c 'import sys, time; print("%.2f" % (time.time() - float(sys.argv[1])))' "$start")
ok=1
[ "$rc" = 2 ] && case "$out" in *"re-entered itself"*)
    [ "$(python3 -I -c 'import sys; print(1 if float(sys.argv[1]) < 10 else 0)' "$elapsed")" = 1 ] && ok=0 ;;
esac
check "a guard that cannot recognise the sentinel stops instead of forking" "$ok" \
    "rc=$rc after ${elapsed}s: $(printf '%.80s' "$out")"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
