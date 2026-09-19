#!/usr/bin/env bash
# devexp: the Kimi acceptance criteria, proved against a faithful simulation of
# Kimi's own hook runner (#114).
#
# WHY A SIMULATION. The ticket asks that reading a .env, a destructive shell
# command and a write carrying a secret are each blocked in a Kimi session.
# Running `kimi` to show that would drive the developer's real credentials: the
# CLI refreshes its OAuth token on start-up, against the account the machine is
# logged in to. So instead this reproduces the ONE thing that stands between
# Kimi and a devexp guard — how runHook spawns the registered command — and
# drives the real adapter and the real guards through it.
#
# WHAT IT REPRODUCES, from agent-core-v2's externalHooks in Kimi Code CLI
# 2.0.1 — BOTH frames, because the payload is built in the outer one:
#
#   internal/matchHooks.ts
#     const inputData = toHookInputData({ hookEventName: event,
#                                         sessionId: args.sessionId ?? "",
#                                         ...args.inputData });
#     await Promise.all(matched.map((hook) => runHook(hostProcess, hook.command,
#                                                     inputData, {...})));
#     function toHookInputData(input) {
#       const result = {};
#       for (const [key, value] of Object.entries(input))
#         result[camelToSnake(key)] = value;
#       return result;
#     }
#     function camelToSnake(value) {
#       return value.replaceAll(/[A-Z]/g, (ch) => `_${ch.toLowerCase()}`);
#     }
#
#   internal/runHook.ts
#     proc = await hostProcess.spawn(command, [], { shell: true, cwd, env });
#     ...
#     proc.stdin.end(JSON.stringify(input));
#     ...
#     function resultFromExitCode(exitCode, stdout, stderr) {
#       if (exitCode === 2) { const message = stderr.trim();
#                             return { action: "block", message, reason: message, ... } }
#
#   * `spawn(command, [], { shell: true })` is Node's: on POSIX it execs
#     `/bin/sh -c <command>` with NO further arguments. So the command is run
#     here exactly as rendered, through `sh -c`, with nothing appended.
#   * the envelope is `JSON.stringify(inputData)` on the child's stdin, and
#     `inputData` has been through `toHookInputData`. That conversion is ONE
#     level deep (`Object.entries`), so the TOP LEVEL is snake_case —
#     `tool_name`, `tool_input`, `hook_event_name`, `session_id`,
#     `client_type`, `session_title`, `tool_call_id` — and everything inside
#     `tool_input` keeps the tool's own spelling, so a Read still names its
#     file in `path`. `kimi_envelope` below applies exactly that function, so
#     what goes down the pipe is the shape a session produces.
#     (An earlier version of this file wrote the pre-conversion camelCase
#     object straight to stdin. It passed against a shape Kimi never sends,
#     while the adapter refused every real tool call.)
#   * exit 2 is the block, and the trimmed stderr is the reason the user sees.
#     Every other exit — and a spawn failure, and the timeout kill — is an
#     ALLOW, which is why "did it exit 2?" is the whole test.
#
# The command line itself is rendered by the same rule as
# KimiCommand (cli/internal/hooks/kimi_config.go), and the layout under the
# hooks directory is the one InstallKimi writes. What is NOT simulated is
# Kimi's matcher, its config parsing and its UI; those have their own tests in
# cli/internal/hooks.
#
# Run: bash hooks/kimi/runner.test.sh

set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../.." && pwd)"

pass=0
fail=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# ── the install, as InstallKimi lays it out ──────────────────────────────────
# The adapter under kimi/, the guards and the scan budget they source together
# under claude-code/. Copied, not symlinked: that is what the installer does,
# and a guard resolves its scan budget from its own directory.
hooks_dir="$work/.kimi-code/hooks"
mkdir -p "$hooks_dir/kimi" "$hooks_dir/claude-code"
cp "$repo/hooks/kimi/adapter.sh" "$hooks_dir/kimi/adapter.sh"
for g in scan-budget.sh secret-guard.sh dangerous-cmd-guard.sh secret-in-write-guard.sh; do
    cp "$repo/hooks/claude-code/$g" "$hooks_dir/claude-code/$g"
done
chmod 0500 "$hooks_dir/kimi/adapter.sh" "$hooks_dir/claude-code/"*.sh

# ── the fixtures, assembled here rather than written down ────────────────────
# A secret-shaped string and a destructive command are built at run time from
# pieces that are harmless apart, so neither this file nor any command line in
# the log ever holds one whole.
aws_key="AK""IA""J7QK2N4PZVX3MTBD"
rm_flag="-r""f"
destructive="rm $rm_flag /"

env_file="$work/project/.env"
mkdir -p "$(dirname "$env_file")"
printf 'TOKEN=placeholder\n' > "$env_file"

# ── the envelope, built the way matchHooks.ts builds it ──────────────────────
# The pre-conversion object is assembled camelCase, exactly as
# runPreToolUse -> withSessionFacts -> triggerInner -> runMatchedHooks do, and
# then toHookInputData is applied over the top level. `spelling` is "converted"
# for what Kimi sends and "raw" for the shape before the conversion, which the
# adapter also has to accept.
KIMI_ENVELOPE_PY='
import json, sys

def camel_to_snake(key):  # value.replaceAll(/[A-Z]/g, ch => `_${ch.toLowerCase()}`)
    out = []
    for ch in key:
        out.append("_" + ch.lower() if "A" <= ch <= "Z" else ch)
    return "".join(out)

spelling, tool_name = sys.argv[1], sys.argv[2]
tool_input = json.loads(sys.argv[3])
raw = {
    "hookEventName": "PreToolUse",
    "sessionId": "s1",
    "clientType": "cli",
    "sessionTitle": "a session",
    "toolName": tool_name,
    "toolInput": tool_input,
    "toolCallId": "call-1",
}
# toHookInputData: one level deep, so tool_input is handed over untouched.
if spelling == "converted":
    raw = {camel_to_snake(k): v for k, v in raw.items()}
sys.stdout.write(json.dumps(raw))
'

kimi_envelope() { # $1=spelling  $2=toolName  $3=toolInput JSON
    python3 -c "$KIMI_ENVELOPE_PY" "$1" "$2" "$3"
}

# ── the runner ───────────────────────────────────────────────────────────────
# command: the rendered [[hooks]] command. envelope: what Kimi writes to stdin.
# Echoes the exit code and leaves the reason in $reason, as Kimi's
# resultFromExitCode does with the trimmed stderr.
run_hook() { # $1=command  $2=envelope
    local command="$1" envelope="$2" err_file status
    err_file="$work/stderr.$$"
    # `sh -c <command>` with no arguments, the envelope on stdin: Node's
    # spawn(command, [], {shell: true}) followed by stdin.end(JSON.stringify()).
    printf '%s' "$envelope" | sh -c "$command" >/dev/null 2>"$err_file"
    status=$?
    reason="$(tr -d '\r' < "$err_file" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
    rm -f "$err_file"
    return $status
}

# kimi_command renders the command line for a guard the way KimiCommand does:
# bash, the adapter, the guard, both paths absolute and single-quoted.
kimi_command() { # $1=guard basename
    printf "bash '%s' '%s'" "$hooks_dir/kimi/adapter.sh" "$hooks_dir/claude-code/$1"
}

check_blocked() { # $1=label  $2=guard  $3=envelope  $4=expected reason fragment
    local label="$1" status=0
    run_hook "$(kimi_command "$2")" "$3" || status=$?
    if [ "$status" != 2 ]; then
        echo "FAIL: $label — the registered command exited $status, and Kimi reads anything but 2 as an ALLOW"
        fail=$((fail + 1))
        return
    fi
    case "$reason" in
        *"$4"*) ;;
        *)
            echo "FAIL: $label — blocked, but the reason Kimi would show is not about it: $reason"
            fail=$((fail + 1))
            return
            ;;
    esac
    pass=$((pass + 1))
    echo "PASS: $label"
}

check_allowed() { # $1=label  $2=guard  $3=envelope
    local label="$1" status=0
    run_hook "$(kimi_command "$2")" "$3" || status=$?
    if [ "$status" != 0 ]; then
        echo "FAIL: $label — exited $status, want 0; an ordinary tool call must not be blocked ($reason)"
        fail=$((fail + 1))
        return
    fi
    pass=$((pass + 1))
    echo "PASS: $label"
}

# ── the acceptance criteria, once per spelling ───────────────────────────────
# "converted" is what a real Kimi session sends. "raw" is the same facts
# before toHookInputData, which the adapter also accepts so a change on Kimi's
# side cannot turn every guarded tool call into a block.
for spelling in converted raw; do
    echo "--- envelope spelling: $spelling ---"

    # ── criterion 1: reading a .env is blocked ───────────────────────────────
    # Kimi's Read names the file in `path`, INSIDE tool_input, which Kimi never
    # converts; the adapter is what turns that into the `file_path` the guard
    # reads.
    check_blocked "a Read of .env is blocked" secret-guard.sh \
        "$(kimi_envelope "$spelling" Read "$(printf '{"path": "%s"}' "$env_file")")" \
        ".env"

    # ReadMediaFile is the same read under another name, and the matcher lets
    # it through to the guard, so the adapter has to rename it or the guard has
    # no case for it and allows.
    check_blocked "a ReadMediaFile of .env is blocked too" secret-guard.sh \
        "$(kimi_envelope "$spelling" ReadMediaFile "$(printf '{"path": "%s"}' "$env_file")")" \
        ".env"

    # ── criterion 2: a destructive command is blocked ────────────────────────
    check_blocked "a destructive shell command is blocked" dangerous-cmd-guard.sh \
        "$(kimi_envelope "$spelling" Bash "$(printf '{"command": "%s"}' "$destructive")")" \
        "Blocked"

    # ── criterion 3: a write carrying a secret is blocked ────────────────────
    check_blocked "a Write carrying a secret is blocked" secret-in-write-guard.sh \
        "$(kimi_envelope "$spelling" Write "$(printf '{"path": "%s/app.py", "content": "AWS_ACCESS_KEY_ID = \\"%s\\"\\n"}' "$work" "$aws_key")")" \
        "AWS"

    # ── and the other half: ordinary work still runs ─────────────────────────
    # A guard that blocked everything would pass all three criteria and be
    # useless — and that is exactly what an adapter reading the wrong spelling
    # did, so these cases are the regression, not a nicety.
    check_allowed "an ordinary Read is allowed" secret-guard.sh \
        "$(kimi_envelope "$spelling" Read "$(printf '{"path": "%s/project/README.md"}' "$work")")"
    check_allowed "an ordinary command is allowed" dangerous-cmd-guard.sh \
        "$(kimi_envelope "$spelling" Bash '{"command": "ls -la"}')"
    check_allowed "an ordinary Write is allowed" secret-in-write-guard.sh \
        "$(kimi_envelope "$spelling" Write "$(printf '{"path": "%s/app.py", "content": "print(1)\\n"}' "$work")")"
done

# The shape itself, asserted rather than assumed: what goes down the pipe for
# the "converted" spelling must be snake_case at the top level and untouched
# inside tool_input. If this drifts, every PASS above is about a shape Kimi
# does not send.
echo "--- the envelope shape ---"
shape_rc=0
kimi_envelope converted Read '{"path": "/x/.env", "lineOffset": 3}' | python3 -c '
import json, sys
d = json.load(sys.stdin)
problems = []
for key in ("hook_event_name", "session_id", "client_type", "session_title",
            "tool_name", "tool_input", "tool_call_id"):
    if key not in d:
        problems.append("missing %s" % key)
for key in ("hookEventName", "sessionId", "toolName", "toolInput", "toolCallId"):
    if key in d:
        problems.append("%s was not converted" % key)
ti = d.get("tool_input", {})
if ti.get("path") != "/x/.env":
    problems.append("tool_input.path was renamed to %r" % list(ti))
if ti.get("lineOffset") != 3:
    problems.append("a tool argument was converted, and Kimi converts none")
if problems:
    sys.stderr.write("; ".join(problems) + "\n")
    sys.exit(1)
' || shape_rc=$?
if [ "$shape_rc" = 0 ]; then
    pass=$((pass + 1)); echo "PASS: the envelope is snake_case at the top level only"
else
    fail=$((fail + 1)); echo "FAIL: the envelope is not the shape Kimi sends"
fi

# The command line rendered here is KimiCommand's rule spelled out; that the
# installer renders the same one, and registers it against scripts that exist,
# is asserted in cli/internal/hooks (TestKimiCommandQuoting,
# TestInstallKimiCopiesThenRegisters).

echo
echo "passed: $pass  failed: $fail"
[ "$fail" -eq 0 ]
