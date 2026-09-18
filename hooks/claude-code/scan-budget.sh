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
# That same check is what floors the budgeted run, which is why the guard's
# prologue trap is cleared only where a decision is in hand, never on the way
# in — see devexp_scan_budget.
#
# The budget is DEVEXP_SCAN_BUDGET_MS milliseconds, defaulting to
# DEVEXP_SCAN_BUDGET_DEFAULT_MS and capped at DEVEXP_SCAN_BUDGET_MAX_MS. A
# value of 0 means "already over budget" and blocks without scanning; anything
# that is not a plain ASCII non-negative integer is ignored in favour of the
# default. hooks/opencode/utils.js reads the same variable and carries the same
# default, ceiling and spelling, so both twins decide alike.
#
# Usage — first thing in a guard, right after `set -euo pipefail`, under the
# prologue trap the guards install (copy it from any of them):
#
#   . "$devexp_dir/scan-budget.sh"
#   devexp_scan_budget <guard-name> "$@"
#
# Tests: bash hooks/claude-code/scan-budget.test.sh

# The default budget, in milliseconds. Sized from the worst cases measured on
# the crafted inputs the #146/#158 timing tests use: ~1.0 s for a 2 MB write
# through secret-in-write-guard, ~1.7 s for a 1 MB command through
# dangerous-cmd-guard, ~0.07 s for ordinary input. That is ~9x headroom, and it
# is above the ceilings those suites already accept as "not slow" (8 s for a
# 2 MB write, 15 s for a 400 KB pipeline), so a much slower machine still never
# trips it.
DEVEXP_SCAN_BUDGET_DEFAULT_MS=15000

# The ceiling, in milliseconds (DEVEXP_SCAN_BUDGET_CEILING_MS may lower it, and
# only lower it). A budget at or above the hook `timeout` in
# hooks/registry.json reinstates the bug this exists to prevent: Claude Code
# would cancel the guard first, and a cancelled command hook does not block the
# tool call. A larger DEVEXP_SCAN_BUDGET_MS is therefore clamped to this, with
# a notice, rather than honoured. It must stay below every fail-closed guard's
# registered timeout — scan-budget.test.sh and the Go registry test both check
# that against the registry, so raising one without the other fails.
DEVEXP_SCAN_BUDGET_MAX_MS=44000

# How the watchdog tells the budgeted run apart from the first one. It travels
# in argv, which only the watchdog can write, and Claude Code passes a command
# hook no arguments. An environment variable would be inherited from whatever
# started Claude Code — a settings `env` entry, a shell profile, a CI image —
# and would silently switch the budget off for every guard.
DEVEXP_SCAN_BUDGET_SENTINEL='--devexp-budgeted'

# A second marker, counted rather than trusted. If the sentinel above ever stops
# being recognised, each budgeted run starts a watchdog of its own and the guard
# forks without bound — the budget only kills its own child's process group, and
# every level makes a new session. This one is read from the environment on
# purpose, because the only thing it can do is make the guard BLOCK: it can
# switch nothing off. A value that is already at the limit stops the guard dead
# instead of letting it multiply.
DEVEXP_SCAN_BUDGET_DEPTH_VAR='DEVEXP_SCAN_BUDGET_DEPTH'

# The program is fixed text; nothing from the tool input reaches it.
IFS= read -r -d '' DEVEXP_SCAN_BUDGET_PY <<'PY' || true
import os, re, signal, subprocess, sys

guard, raw, default, ceiling, sentinel, depth_var = sys.argv[1:7]
cmd = sys.argv[7:]
ceiling = int(ceiling)

# DEVEXP_SCAN_BUDGET_CEILING_MS may only LOWER the ceiling, never raise it, so
# an ambient value cannot undo the cap -- the worst it can do is block sooner.
# It is what lets a test watch the cap take effect: with the real ceiling, the
# only way to see a clamped budget bite is to wait 44 seconds for it.
_env_ceiling = os.environ.get('DEVEXP_SCAN_BUDGET_CEILING_MS', '').strip()
if re.fullmatch(r'[0-9]+', _env_ceiling):
    ceiling = min(ceiling, int(_env_ceiling))
# One watchdog runs the guard; the guard does not run another. Anything past
# that is re-entry, which can only mean the sentinel stopped being recognised.
MAX_DEPTH = 1


def seconds(ms):
    return '%.4g' % (ms / 1000.0)


# Only a plain ASCII non-negative integer counts; "10s", "-1", "1e3", an empty
# or unset variable, a non-ASCII decimal digit such as U+0663 and a digit-like
# character such as U+00B2 all fall back to the default. str.isdigit() accepts
# the last two — one silently, the other by raising — and disagrees with the JS
# twin, which is ASCII-only. Both sides trim surrounding spaces.
ms = int(raw) if re.fullmatch(r'[0-9]+', raw.strip()) else int(default)

# A budget that outlives the hook timeout is a fail-open, so it is capped
# rather than honoured, and the cap is said out loud.
# The wording is the JS twin's, word for word, so the two say the same thing
# about the same input — and it names the ceiling, which is what actually bit.
# "at or above the registered hook timeout" would be false for anything between
# the ceiling and that timeout, and points at the wrong number.
#
# Frequency: the JS twin dedupes with a flag because its module outlives many
# tool calls. A Claude Code hook is a process per tool call, so printing once
# here already is once per process; there is nothing to dedupe against without
# state on disk, which is not worth an I/O on every guarded call.
if ms > ceiling:
    sys.stderr.write(
        '[devexp %s] Note: DEVEXP_SCAN_BUDGET_MS of %s s is above the %s s ceiling the guards '
        'share. Using %s s.\n' % (guard, seconds(ms), seconds(ceiling), seconds(ceiling)))
    ms = ceiling

over = ('[devexp %s] Blocked: the scan did not finish within its %s s budget, so this input '
        'was not fully checked. Blocking to be safe.\n' % (guard, seconds(ms)))


def block(message):
    sys.stderr.write(message)
    raise SystemExit(2)


# A zero budget is over before the scan starts. Tests use it to force a hit.
if ms <= 0:
    block(over)

try:
    depth = int(re.fullmatch(r'[0-9]+', os.environ.get(depth_var, '').strip() or 'x') and
                os.environ[depth_var].strip() or 0)
except Exception:
    depth = 0
if depth >= MAX_DEPTH:
    block('[devexp %s] internal error -- the scan budget re-entered itself, so the guard did not '
          'run. Blocking to be safe.\n' % guard)

try:
    # A session of its own, so the deadline can kill the guard's whole process
    # tree — its python and its greps — and never anything above it. The
    # sentinel tells that run it is the budgeted one; the depth says how many
    # watchdogs are already above it.
    child = subprocess.Popen(cmd + [sentinel], start_new_session=True,
                             env=dict(os.environ, **{depth_var: str(depth + 1)}))
except Exception as err:
    block('[devexp %s] internal error -- the guard could not be started (%s), so it did not run. '
          'Blocking to be safe.\n' % (guard, err))

try:
    rc = child.wait(timeout=ms / 1000.0)
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
# run, so the guard's body runs exactly once.
devexp_scan_budget() {
    local guard="$1"
    shift

    # The prologue trap stays installed until a decision is actually in hand.
    # Being defined is not the same as being able to run: a helper that carries
    # this function but not the constants above it passes the prologue's
    # `command -v`, and then `set -u` ends the guard at exit 1 — which Claude
    # Code reads as a non-blocking error, so the call would proceed unscanned.
    # Each `trap - EXIT` below therefore sits at a point where the status is
    # known, not on the way in.
    local arg
    for arg in ${1+"$@"}; do
        if [ "$arg" = "$DEVEXP_SCAN_BUDGET_SENTINEL" ]; then
            # The budgeted run is floored from here by the watchdog's 0-or-2
            # check on this process's own status; the trap must not fire on the
            # guard's clean exit.
            trap - EXIT
            return 0
        fi
    done

    local rc=0
    python3 -I -S -c "$DEVEXP_SCAN_BUDGET_PY" \
        "$guard" "${DEVEXP_SCAN_BUDGET_MS-}" "$DEVEXP_SCAN_BUDGET_DEFAULT_MS" \
        "$DEVEXP_SCAN_BUDGET_MAX_MS" "$DEVEXP_SCAN_BUDGET_SENTINEL" \
        "$DEVEXP_SCAN_BUDGET_DEPTH_VAR" \
        "${BASH:-bash}" "$0" ${1+"$@"} || rc=$?
    if [ "$rc" != 0 ] && [ "$rc" != 2 ]; then
        # The watchdog itself could not run (no python3, killed, …). It is the
        # only thing standing between a slow scan and an unscanned tool call,
        # so its own failure blocks too.
        echo "[devexp $guard] internal error -- the scan budget could not run (exit $rc), so the guard did not run. Blocking to be safe." >&2
        rc=2
    fi
    # After the line above, not before it: the constants are dereferenced on the
    # `python3` line itself, so clearing the trap any earlier reopens the very
    # window this guards.
    trap - EXIT
    exit "$rc"
}
