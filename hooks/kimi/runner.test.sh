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
# WHAT IT REPRODUCES, from agent-core-v2's
# features/externalHooks/internal/runHook.ts in Kimi Code CLI 2.0.1:
#
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
#   * the envelope is `JSON.stringify(input)` on the child's stdin — Kimi's own
#     camelCase shape, `toolName`/`toolInput`, not Claude Code's.
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

# ── acceptance criterion 1: reading a .env is blocked ────────────────────────
# Kimi's Read names the file in `path`; the adapter is what turns that into the
# `file_path` the guard reads.
check_blocked "a Read of .env is blocked" secret-guard.sh \
    "$(printf '{"hookEventName":"PreToolUse","sessionId":"s1","toolName":"Read","toolInput":{"path":"%s"}}' "$env_file")" \
    ".env"

# ReadMediaFile is the same read under another name, and the matcher lets it
# through to the guard, so the adapter has to rename it or the guard has no
# case for it and allows.
check_blocked "a ReadMediaFile of .env is blocked too" secret-guard.sh \
    "$(printf '{"hookEventName":"PreToolUse","sessionId":"s1","toolName":"ReadMediaFile","toolInput":{"path":"%s"}}' "$env_file")" \
    ".env"

# ── acceptance criterion 2: a destructive command is blocked ─────────────────
check_blocked "a destructive shell command is blocked" dangerous-cmd-guard.sh \
    "$(printf '{"hookEventName":"PreToolUse","sessionId":"s1","toolName":"Bash","toolInput":{"command":"%s"}}' "$destructive")" \
    "Blocked"

# ── acceptance criterion 3: a write carrying a secret is blocked ─────────────
check_blocked "a Write carrying a secret is blocked" secret-in-write-guard.sh \
    "$(printf '{"hookEventName":"PreToolUse","sessionId":"s1","toolName":"Write","toolInput":{"path":"%s/app.py","content":"AWS_ACCESS_KEY_ID = \\"%s\\"\\n"}}' "$work" "$aws_key")" \
    "AWS"

# ── and the other half: ordinary work still runs ─────────────────────────────
# A guard that blocked everything would pass all three criteria and be useless.
check_allowed "an ordinary Read is allowed" secret-guard.sh \
    "$(printf '{"hookEventName":"PreToolUse","sessionId":"s1","toolName":"Read","toolInput":{"path":"%s/project/README.md"}}' "$work")"
check_allowed "an ordinary command is allowed" dangerous-cmd-guard.sh \
    '{"hookEventName":"PreToolUse","sessionId":"s1","toolName":"Bash","toolInput":{"command":"ls -la"}}'
check_allowed "an ordinary Write is allowed" secret-in-write-guard.sh \
    "$(printf '{"hookEventName":"PreToolUse","sessionId":"s1","toolName":"Write","toolInput":{"path":"%s/app.py","content":"print(1)\\n"}}' "$work")"

# The command line rendered here is KimiCommand's rule spelled out; that the
# installer renders the same one, and registers it against scripts that exist,
# is asserted in cli/internal/hooks (TestKimiCommandQuoting,
# TestInstallKimiCopiesThenRegisters).

echo
echo "passed: $pass  failed: $fail"
[ "$fail" -eq 0 ]
