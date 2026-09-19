#!/usr/bin/env bash
# devexp: the Kimi Code CLI adapter for the Claude Code guard scripts (#114).
#
# Usage — as the whole `command` of a Kimi [[hooks]] entry, with the guard to
# run as its one argument:
#
#   bash '<hooks-dir>/kimi/adapter.sh' '<hooks-dir>/claude-code/secret-guard.sh'
#
# Kimi spawns that command with `shell: true` and NO arguments of its own, and
# writes `JSON.stringify(input)` to its stdin, so the guard's path has to
# travel in the command line the installer renders.
#
# WHAT IT TRANSLATES. Kimi's PreToolUse envelope is camelCase and its tool
# arguments are its own; the guards read Claude Code's snake_case envelope:
#
#   toolName -> tool_name        toolInput -> tool_input
#   toolCallId -> tool_call_id   hookEventName -> hook_event_name
#   sessionId -> session_id      transcriptPath -> transcript_path
#
#   Read / ReadMediaFile / Write / Edit:  path -> file_path
#   Bash:                                 command, cwd, … unchanged
#
# ReadMediaFile becomes Read: it is the same read of the same `path`, and the
# guards match on the Claude Code tool name, so without this the registry's
# `^(Read|ReadMediaFile|Bash)$` matcher would run secret-guard on a tool name
# it has no case for — a scan that inspects nothing and allows. Every other
# tool name, and every argument not named above, is passed through unchanged
# rather than dropped: a guard that grows a new field must keep seeing it.
#
# WHY IT IS FAIL-CLOSED. Kimi reads exit 2 as a block with the trimmed stderr
# as the reason, and reads EVERY other exit, a spawn failure and a timeout as
# an ALLOW (see runHook in Kimi Code CLI 2.0.1 and cli/internal/hooks/kimi.go).
# So anything this adapter cannot carry out faithfully has to end at exit 2:
#
#   * bad JSON on stdin, a payload that is not an object, a missing or
#     unusable toolName/toolInput — the envelope cannot be translated, so no
#     guard can have scanned it;
#   * no guard argument, a guard that is not a readable file, a missing
#     python3 — the guard did not run;
#   * a guard exit that is neither allow (0) nor block (2). Claude Code reads
#     exit 1 as a non-blocking error and Kimi reads it as an allow, but a
#     fail-closed guard that neither allowed nor blocked has no decision, and
#     an unknown decision must not let the tool call through. This is the same
#     0-or-2 rule the guards' own scan budget applies to its child.
#
# It never manufactures an allow: the guards mint their scan proof and their
# budget proof themselves (hooks/claude-code/scan-budget.sh), and nothing here
# writes DEVEXP_SCAN_PROOF, DEVEXP_SCAN_BUDGET_PROOF_FD or the sentinel argv
# the budget recognises. The guard is invoked with no arguments at all, so its
# budget always sees a first run and re-runs it under the watchdog; a budget
# hit therefore arrives here as exit 2 and stays a block.
#
# ASK IS A BLOCK. Kimi's structuredOutput honours only
# `hookSpecificOutput.permissionDecision === "deny"`; an "ask" falls through to
# its allowResult, so a confirmation prompt would silently become consent.
# An ask verdict is therefore turned into exit 2, carrying the guard's own
# reason. (large-file-guard, whose only verdict is an ask, is off for Kimi in
# hooks/registry.json for the separate reason that it would then block every
# large overwrite instead of asking about it.)
#
# Tests: bash hooks/kimi/adapter.test.sh

set -euo pipefail

devexp_kimi_why="stopped before it could run the guard"

# The floor. Every path out of this script that is not an explicit decision
# lands here — `set -e` on an unexpected failure, `set -u` on an unset
# variable, a `.`/exec failure — and Kimi would read any of those as an allow.
# Cleared only where a decision is in hand.
devexp_kimi_floor() {
    trap - EXIT
    echo "[devexp kimi-adapter] internal error -- the adapter $devexp_kimi_why, so the tool call was not checked. Blocking to be safe." >&2
    exit 2
}
trap devexp_kimi_floor EXIT

devexp_kimi_block() { # $1=reason
    trap - EXIT
    echo "[devexp kimi-adapter] $1" >&2
    exit 2
}

# ── the guard to run ─────────────────────────────────────────────────────────
guard="${1-}"
[ -n "$guard" ] || devexp_kimi_block "internal error -- no guard script was named on the command line, so nothing scanned this tool call. Blocking to be safe."
[ -f "$guard" ] && [ -r "$guard" ] || devexp_kimi_block "internal error -- the guard script \"$guard\" is missing or unreadable, so it did not run. Blocking to be safe."
command -v python3 >/dev/null 2>&1 || devexp_kimi_block "internal error -- python3 is not available, so the hook envelope could not be translated and no guard ran. Blocking to be safe."

# ── the translation ──────────────────────────────────────────────────────────
# Fixed program text: the envelope arrives on stdin, never in the source.
IFS= read -r -d '' DEVEXP_KIMI_TRANSLATE_PY <<'PY' || true
import json, sys

# Kimi tool name -> Claude Code tool name. Everything absent from this map
# keeps its own name.
TOOL_NAMES = {'ReadMediaFile': 'Read'}

# Tools whose Kimi `path` is Claude Code's `file_path`.
PATH_TOOLS = {'Read', 'Write', 'Edit'}

# Top-level camelCase facts that have a Claude Code spelling. Any other key
# travels unchanged.
TOP_LEVEL = {
    'toolCallId': 'tool_call_id',
    'hookEventName': 'hook_event_name',
    'sessionId': 'session_id',
    'transcriptPath': 'transcript_path',
}

def fail(message):
    sys.stderr.write('[devexp kimi-adapter] %s\n' % message)
    raise SystemExit(2)

raw = sys.stdin.read()
try:
    envelope = json.loads(raw)
except Exception as err:
    fail('internal error -- the hook input is not valid JSON (%s), so it could not be '
         'translated and no guard ran. Blocking to be safe.' % err)
if not isinstance(envelope, dict):
    fail('internal error -- the hook input is not a JSON object, so it could not be '
         'translated and no guard ran. Blocking to be safe.')

tool_name = envelope.get('toolName')
if not isinstance(tool_name, str) or not tool_name:
    fail('internal error -- the hook input carries no usable "toolName", so the guard '
         'would have had nothing to scan. Blocking to be safe.')
tool_input = envelope.get('toolInput')
if not isinstance(tool_input, dict):
    fail('internal error -- the hook input carries no usable "toolInput" for %s, so the '
         'guard would have had nothing to scan. Blocking to be safe.' % tool_name)

out = {}
for key, value in envelope.items():
    if key in ('toolName', 'toolInput'):
        continue
    out[TOP_LEVEL.get(key, key)] = value

claude_name = TOOL_NAMES.get(tool_name, tool_name)
args = dict(tool_input)
# Only when the Claude Code spelling is not already there: an envelope that
# carries both keeps both, rather than having one overwrite the other.
if claude_name in PATH_TOOLS and 'path' in args and 'file_path' not in args:
    args['file_path'] = args.pop('path')

out['tool_name'] = claude_name
out['tool_input'] = args
sys.stdout.write(json.dumps(out))
PY

devexp_kimi_why="could not read the hook input"
input=$(cat) || devexp_kimi_block "internal error -- the hook input could not be read, so no guard ran. Blocking to be safe."

devexp_kimi_why="could not translate the hook input"
translated=$(printf '%s' "$input" | python3 -I -c "$DEVEXP_KIMI_TRANSLATE_PY") || {
    # The program above has already said why on stderr for every case it
    # recognises; this covers an interpreter that died some other way.
    trap - EXIT
    echo "[devexp kimi-adapter] internal error -- the hook input could not be translated, so no guard ran. Blocking to be safe." >&2
    exit 2
}

# ── the guard ────────────────────────────────────────────────────────────────
# Its stderr goes straight through to ours, which is what Kimi quotes as the
# block reason. Its stdout is captured because a Claude Code guard can carry a
# verdict there, and Claude Code's JSON is not Kimi's.
#
# No arguments: the scan budget forwards this script's argv to its watchdog and
# recognises its own sentinel in it, so anything added here would travel into
# that check.
#
# The input arrives through a process substitution rather than a pipe, so the
# status read below is the guard's own — a guard that blocks before reading
# stdin leaves the writer with a SIGPIPE, and in a pipeline that could be read
# as the guard's status — and rather than a temporary file, so a payload that
# may hold a secret is not written to disk to be scanned.
devexp_kimi_why="could not run the guard"
guard_rc=0
guard_stdout=$(bash "$guard" < <(printf '%s' "$translated")) || guard_rc=$?

if [ "$guard_rc" = 2 ]; then
    # A block, with the guard's own reason already on stderr. Kimi trims that
    # stderr and shows it as the reason.
    trap - EXIT
    exit 2
fi

if [ "$guard_rc" != 0 ]; then
    devexp_kimi_block "internal error -- the guard \"${guard##*/}\" exited $guard_rc, which is neither allow (0) nor block (2), so its decision is unknown. Blocking to be safe."
fi

# ── exit 0: the verdict may still be in the guard's stdout ───────────────────
IFS= read -r -d '' DEVEXP_KIMI_VERDICT_PY <<'PY' || true
import json, sys

# Claude Code's structured hook output, as the guards write it:
#   {"hookSpecificOutput": {"permissionDecision": "ask"|"deny"|"allow",
#                           "permissionDecisionReason": "..."},
#    "systemMessage": "...", "decision": "block", "reason": "..."}
#
# Kimi blocks only on permissionDecision "deny", so "ask" and the older
# top-level "block" have to be turned into an exit 2 here or they would pass
# as consent.
BLOCKING = {'deny', 'ask', 'block'}

text = sys.stdin.read().strip()
if not text:
    raise SystemExit(0)

try:
    parsed = json.loads(text)
except Exception:
    parsed = None
if not isinstance(parsed, dict):
    # Not a verdict — a guard talking to the user. Kimi's JSON.parse would
    # drop it, so it is handed over in the one shape Kimi does read.
    json.dump({'message': text}, sys.stdout)
    raise SystemExit(0)

specific = parsed.get('hookSpecificOutput')
if not isinstance(specific, dict):
    specific = {}

def text_of(*values):
    for value in values:
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ''

decision = parsed.get('decision')
decision = decision.strip().lower() if isinstance(decision, str) else ''
permission = specific.get('permissionDecision')
permission = permission.strip().lower() if isinstance(permission, str) else ''

reason = text_of(specific.get('permissionDecisionReason'), parsed.get('reason'),
                 parsed.get('systemMessage'), specific.get('message'), parsed.get('message'))

if decision == 'block' or permission in BLOCKING:
    if permission == 'ask':
        note = ('the guard asked for confirmation, and Kimi runs an "ask" as an allow, so '
                'it is a block here. Re-run with the guard satisfied, or confirm with the '
                'user first.')
        sys.stderr.write('[devexp kimi-adapter] Blocked: %s %s\n' % (reason or 'No reason given.', note))
    else:
        sys.stderr.write('[devexp kimi-adapter] Blocked: %s\n' % (reason or 'the guard returned a block with no reason.'))
    raise SystemExit(2)

# An allow. Anything the guard wanted to say travels on as Kimi's `message`,
# which its HookJsonOutputSchema reads and its allowResult carries.
if reason:
    json.dump({'message': reason}, sys.stdout)
raise SystemExit(0)
PY

devexp_kimi_why="could not read the guard's verdict"
verdict_rc=0
printf '%s' "$guard_stdout" | python3 -I -c "$DEVEXP_KIMI_VERDICT_PY" || verdict_rc=$?

if [ "$verdict_rc" = 0 ]; then
    trap - EXIT
    exit 0
fi
if [ "$verdict_rc" = 2 ]; then
    trap - EXIT
    exit 2
fi
devexp_kimi_block "internal error -- the guard's verdict could not be read (exit $verdict_rc), so it is unknown. Blocking to be safe."
