#!/usr/bin/env bash
# devexp: the scan budget shared by the fail-closed security guards, and the
# proof of work each of them allows on.
#
# PROOF OF WORK. A guard must not exit 0 unless its OWN scanning code
# ran and decided to allow. The shell can only read an external program's exit
# status, so before this every program that reported success — a wrapper, a
# shim, a broken virtualenv, a stand-in that does nothing — was read as "the
# scan found nothing", and the tool call went through unscanned. The budget
# widened that: the wrapper below runs through the same interpreter, so a
# stand-in answers before the guard has even read the envelope.
#
# So a guard now allows only against positive proof, on a channel the shell
# reads, that the code which decides actually ran:
#
#   * the guard's own scanning step reports DEVEXP_SCAN_PROOF back, and
#     devexp_scan_result blocks when that token is missing, whatever the exit
#     status (see below);
#   * the watchdog reports a token of its own back on
#     DEVEXP_SCAN_BUDGET_PROOF_FD, and devexp_scan_budget honours exit 0 only
#     when it is there.
#
# Both tokens are minted per invocation, so nothing can carry one in advance,
# and neither is anything a program produces on the way past: silence, an empty
# line, a generic success message, an echo of the guard's input, a crash
# part-way through the scan and a wrapper that runs some other program all
# arrive without the token, and all of them block. What this does NOT stop is a
# program built to imitate this protocol on purpose — the program that produces
# the proof is the one under suspicion, so there is no channel it cannot see.
# The property is "the scanning code ran", not "the interpreter is honest".
#
# Claude Code runs a `PreToolUse` command hook with a timeout of 600 seconds by
# default, and a timed-out command hook does NOT block the tool call — the call
# continues through the normal permission flow. So a guard that is slow on some
# input fails OPEN: the tool call goes ahead unscanned after a long wait. No
# hook field expresses "block on timeout", so the only lever a guard has is to
# exit 2 on its own, before the timeout fires.
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
# the crafted inputs the timing cases in secret-in-write-guard.test.sh and
# dangerous-cmd-guard.test.sh use: ~1.0 s for a 2 MB write
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

# The descriptor the watchdog reports its own token back on. Not stdout: the
# guard's stdout has to reach Claude Code unchanged, and a channel the guarded
# run inherits is one a hung child could hold open past the deadline. This one
# is opened by devexp_scan_budget for the watchdog alone — subprocess closes
# everything above stderr in the child — so nothing but the watchdog can write
# to it, and nothing writes to it by accident.
#
# A redirection needs a literal number, so the `9>&1` in devexp_scan_budget
# spells it out; interpreter-proof.test.sh fails if the two ever disagree.
DEVEXP_SCAN_BUDGET_PROOF_FD=9

# The token a guard's own scanning step must report back before the guard may
# allow. Minted per process, so it cannot be baked into anything; it travels to
# the scanning step as an argument and comes back as the first line of the
# output the guard already captures. See devexp_scan_result.
DEVEXP_SCAN_PROOF="devexp-scanned-$$-${RANDOM}${RANDOM}${RANDOM}"

# Set by devexp_scan_result to the scanning step's own output, once the proof
# has been taken off the front of it.
devexp_scanned=''

# devexp_scan_result — check that the scanning step reported back, and leave
# what it actually said in devexp_scanned. A step that says nothing, says
# something else, or stops part-way through has not decided anything, so the
# guard blocks rather than reading its silence as "nothing found".
devexp_scan_result() { # $1=guard name  $2=the scanning step's raw output
    local guard="$1" out="$2"
    if [ "$out" = "$DEVEXP_SCAN_PROOF" ]; then
        devexp_scanned=''
    elif [ "${out%%$'\n'*}" = "$DEVEXP_SCAN_PROOF" ]; then
        devexp_scanned="${out#*$'\n'}"
    else
        echo "[devexp $guard] internal error -- the scan did not report back, so the input was not checked. Blocking to be safe." >&2
        exit 2
    fi
}

# The program is fixed text; nothing from the tool input reaches it.
IFS= read -r -d '' DEVEXP_SCAN_BUDGET_PY <<'PY' || true
import os, re, signal, subprocess, sys

guard, raw, default, ceiling, sentinel, depth_var, proof, proof_fd = sys.argv[1:9]
cmd = sys.argv[9:]
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
    #
    # close_fds is spelled out because it is load-bearing, not
    # incidental. The proof descriptor is a pipe the caller reads to the end, so
    # anything that inherits it and outlives the guarded run keeps the guard
    # waiting — past the budget, past the hook timeout, which is the very
    # fail-open the budget exists to prevent. With it on, the guarded run and every
    # descendant get stdin, stdout and stderr and nothing else. Turning it off
    # took one guarded call from 0.2 s to over nine minutes in testing;
    # interpreter-proof.test.sh pins both halves.
    child = subprocess.Popen(cmd + [sentinel], start_new_session=True, close_fds=True,
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
    # Proof that this watchdog ran the guard and has its decision in hand
    # Written here and nowhere else: not on the way in, not on any
    # path that never started the guard. Every exit above is a block, which
    # needs no proof -- only an allow does.
    try:
        os.write(int(proof_fd), (proof + '\n').encode())
    except OSError as err:
        block('[devexp %s] internal error -- the scan budget could not report that the guard ran '
              '(%s), so its decision cannot be trusted. Blocking to be safe.\n' % (guard, err))
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

    # The watchdog's own token, minted here, for this invocation only — a
    # replay of an earlier invocation's token is a token for an invocation that
    # is over, and is refused. It comes back on DEVEXP_SCAN_BUDGET_PROOF_FD,
    # which devexp_scan_budget_run points at a command substitution while
    # sending the guarded run's stdout to fd 8, so the two never mix.
    local nonce="devexp-budget-$$-${RANDOM}${RANDOM}${RANDOM}"
    local rc=0 proof=''

    # The constants are read here, before the command substitution below and
    # not inside it: a helper that carries this function but not the constants
    # must end the guard under its prologue trap, as it did before the proof
    # channel existed. Inside a substitution `set -u` would only end the
    # subshell, and the guard would name the wrong thing.
    local program="$DEVEXP_SCAN_BUDGET_PY" \
          default="$DEVEXP_SCAN_BUDGET_DEFAULT_MS" \
          ceiling="$DEVEXP_SCAN_BUDGET_MAX_MS" \
          sentinel="$DEVEXP_SCAN_BUDGET_SENTINEL" \
          depth_var="$DEVEXP_SCAN_BUDGET_DEPTH_VAR" \
          proof_fd="$DEVEXP_SCAN_BUDGET_PROOF_FD"

    # fd 8 is where the guarded run's own stdout goes, and it is attached to
    # the call below rather than with `exec`: a failed `exec` redirection ends
    # a non-interactive shell on the spot, so a caller with no stdout — a
    # harness, a wrapper, a cron line — would have ended the guard mid-prologue
    # with a raw shell error and a reason naming the wrong cause. Claude Code
    # always gives a hook a stdout; when something else does not, the guarded
    # run writes to /dev/null and still decides, as it did before this channel
    # existed. Asked quietly, and stderr is silenced BEFORE the failing dup, or
    # the shell's own complaint is the noise this avoids.
    local dupable=0
    { : >&8; } 2>/dev/null 8>&1 || dupable=1
    if [ "$dupable" = 0 ]; then
        devexp_scan_budget_run ${1+"$@"} 8>&1
    else
        devexp_scan_budget_run ${1+"$@"} 8>/dev/null
    fi
    if [ "$rc" != 0 ] && [ "$rc" != 2 ]; then
        # The watchdog itself could not run (no python3, killed, …). It is the
        # only thing standing between a slow scan and an unscanned tool call,
        # so its own failure blocks too.
        echo "[devexp $guard] internal error -- the scan budget could not run (exit $rc), so the guard did not run. Blocking to be safe." >&2
        rc=2
    elif [ "$rc" = 0 ] && [ "$proof" != "$nonce" ]; then
        # Exit 0 with no proof: something answered for the watchdog without
        # running it, so no guard ran and nothing was scanned. An allow
        # here is the fail-open this whole file exists to prevent.
        echo "[devexp $guard] internal error -- the scan budget ended without proof that the guard ran, so its allow cannot be trusted. Blocking to be safe." >&2
        rc=2
    fi
    # After the lines above, not before them: the constants are dereferenced
    # inside this function, and the decision is only in hand once the status
    # and the proof have both been read, so clearing the trap any earlier
    # reopens the very window this guards.
    trap - EXIT
    exit "$rc"
}

# devexp_scan_budget_run — start the watchdog and collect its token. Called by
# devexp_scan_budget and nowhere else: it reads that function's locals and
# writes its `proof` and `rc` back, and it expects fd 8 to already point where
# the guarded run's stdout should go. It exists as a function so that fd 8 can
# be attached to a call rather than to the shell itself.
devexp_scan_budget_run() {
    proof=$(python3 -I -S -c "$program" \
        "$guard" "${DEVEXP_SCAN_BUDGET_MS-}" "$default" "$ceiling" "$sentinel" \
        "$depth_var" "$nonce" "$proof_fd" \
        "${BASH:-bash}" "$0" ${1+"$@"} 9>&1 1>&8) || rc=$?
}
