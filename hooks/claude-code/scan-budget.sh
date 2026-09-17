#!/usr/bin/env bash
# devexp: the scan budget shared by the fail-closed security guards.
#
# Claude Code runs a `PreToolUse` command hook with a timeout of 600 seconds by
# default, and a timed-out command hook does NOT block the tool call — the call
# continues through the normal permission flow. So a guard that is slow on some
# input fails OPEN: the tool call goes ahead unscanned after a long wait. No
# hook field expresses "block on timeout", so the only lever a guard has is to
# exit 2 on its own, before the timeout fires (#162).
#
# This wrapper gives each guard a wall-clock budget covering the WHOLE scan —
# reading the envelope, json.load, the regex work and every grep. It re-runs
# the guard under a python3 watchdog in a session of its own and SIGKILLs the
# process group at the deadline. The kill has to reach the group: bash defers
# trap handlers while a foreground child runs, so a trap would not interrupt a
# slow `grep` or `python3`, and neither `timeout(1)` (absent on macOS) nor
# `EPOCHREALTIME` (bash 5, while macOS ships 3.2) is available here.
#
# A budget hit exits 2 — a block — never 0. So does a child status that is
# neither allow (0) nor block (2): a guard that dies on a signal or cannot
# start has no decision, and an unknown decision must not let the call through.
#
# The budget is DEVEXP_SCAN_BUDGET_MS milliseconds, defaulting to
# DEVEXP_SCAN_BUDGET_DEFAULT_MS. A value of 0 means "already over budget" and
# blocks without scanning; anything that is not a non-negative integer is
# ignored in favour of the default. hooks/opencode/utils.js reads the same
# variable and carries the same default, so both twins decide alike.
#
# Usage — first thing in a guard, right after `set -euo pipefail`:
#
#   . "$(dirname "${BASH_SOURCE[0]}")/scan-budget.sh"
#   devexp_scan_budget <guard-name> "$@"
#
# Tests: bash hooks/claude-code/scan-budget.test.sh

# The default budget, in milliseconds. Sized from the worst cases measured on
# the crafted inputs the #146/#158 timing tests use: ~1.0 s for a 2 MB write
# through secret-in-write-guard, ~1.7 s for a 1 MB command through
# dangerous-cmd-guard, ~0.07 s for ordinary input. That is ~9x headroom, and it
# is above the ceilings those suites already accept as "not slow" (8 s for a
# 2 MB write, 15 s for a 400 KB pipeline), so a much slower machine still never
# trips it. hooks/registry.json sets each guard's hook `timeout` well above it,
# so the guard's own block always lands first.
DEVEXP_SCAN_BUDGET_DEFAULT_MS=15000

# The program is fixed text; nothing from the tool input reaches it.
IFS= read -r -d '' DEVEXP_SCAN_BUDGET_PY <<'PY' || true
import os, signal, subprocess, sys

guard, raw, default = sys.argv[1], sys.argv[2], sys.argv[3]
cmd = sys.argv[4:]

# Only a plain non-negative integer counts; "10s", "-1", "1e3" and an empty or
# unset variable all fall back to the default, so a typo cannot silently widen
# or disable the budget.
ms = int(raw) if raw.isdigit() else int(default)
secs = ms / 1000.0

over = ('[devexp %s] Blocked: the scan did not finish within its %.4g s budget, so this input '
        'was not fully checked. Blocking to be safe.\n' % (guard, secs))


def block(message):
    sys.stderr.write(message)
    raise SystemExit(2)


# A zero budget is over before the scan starts. Tests use it to force a hit.
if ms <= 0:
    block(over)

try:
    # A session of its own, so the deadline can kill the guard's whole process
    # tree — its python and its greps — and never anything above it.
    child = subprocess.Popen(cmd, start_new_session=True)
except Exception as err:
    block('[devexp %s] internal error -- the guard could not be started (%s), so it did not run. '
          'Blocking to be safe.\n' % (guard, err))

try:
    rc = child.wait(timeout=secs)
except subprocess.TimeoutExpired:
    try:
        os.killpg(child.pid, signal.SIGKILL)
    except OSError:
        child.kill()
    child.wait()
    block(over)

if rc in (0, 2):
    raise SystemExit(rc)
block('[devexp %s] internal error -- the guard exited %d, which is neither allow (0) nor '
      'block (2), so its decision is unknown. Blocking to be safe.\n' % (guard, rc))
PY

# devexp_scan_budget — run the calling guard under its budget, then exit with
# its status. Returns without doing anything when it is already the budgeted
# run, so the guard's own body runs exactly once.
devexp_scan_budget() {
    if [ "${DEVEXP_SCAN_BUDGET_RUNNING:-}" = "1" ]; then
        return 0
    fi
    local guard="$1"
    shift
    local rc=0
    DEVEXP_SCAN_BUDGET_RUNNING=1 python3 -I -S -c "$DEVEXP_SCAN_BUDGET_PY" \
        "$guard" "${DEVEXP_SCAN_BUDGET_MS-}" "$DEVEXP_SCAN_BUDGET_DEFAULT_MS" \
        "${BASH:-bash}" "$0" "$@" || rc=$?
    if [ "$rc" != 0 ] && [ "$rc" != 2 ]; then
        # The watchdog itself could not run (no python3, killed, …). It is the
        # only thing standing between a slow scan and an unscanned tool call,
        # so its own failure blocks too.
        echo "[devexp $guard] internal error -- the scan budget could not run (exit $rc), so the guard did not run. Blocking to be safe." >&2
        rc=2
    fi
    exit "$rc"
}
