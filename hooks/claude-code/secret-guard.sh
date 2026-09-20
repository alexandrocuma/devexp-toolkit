#!/usr/bin/env bash
# devexp hook: secret-guard
# Event: PreToolUse | Matcher: Read|Bash
# Blocks accidental reads of .env and private key files via the Read tool
# or Bash commands (cat, head, tail, etc.).
#
# Two false-positive classes are deliberately excluded:
#
#   Templates. A committed `.env.example` documents which keys exist;
#   it never holds their values. Blocking it blocks the one file a user is
#   meant to read in order to configure the others.
#
#   Mentions. A shell token only counts as a path if it plausibly is
#   one. Heredoc bodies, program text (`jq -r '.key'`), quoted programs and
#   bare extensions are not file reads, and refusing them obstructs without
#   protecting anything.
#
# To block: print reason to stderr, exit 2
# To allow: exit 0 with no output
#
# Tests: bash hooks/claude-code/secret-guard.test.sh

set -euo pipefail

# The whole scan runs under a wall-clock budget and blocks when it is exceeded:
# a slow scan must never let a tool call through unchecked.
#
# Everything up to devexp_scan_budget runs with no floor under it, so this
# prologue installs one. It cannot be an `if ! . …`: under `set -e` bash leaves
# the script where the `.` failed and never reaches the body, and it cannot
# read $? either — for a missing, unreadable or unparsable file, and for an
# unset variable, bash 3.2 reports 0 to an EXIT trap, and 0 reads as "allow".
# So the trap asks no questions: reaching it at all means the budget never
# started. devexp_scan_budget clears it as soon as it takes over, and from
# there the watchdog's own 0-or-2 check is the floor.
devexp_budget_why="stopped before its scan budget could start"
devexp_budget_floor() {
    trap - EXIT
    echo "[devexp secret-guard] internal error -- the guard $devexp_budget_why, so it did not run. Blocking to be safe." >&2
    exit 2
}
trap devexp_budget_floor EXIT

devexp_dir="${BASH_SOURCE[0]%/*}"
if [ "$devexp_dir" = "${BASH_SOURCE[0]}" ]; then devexp_dir="."; fi
devexp_budget_why="could not read its scan budget helper"
[ -r "$devexp_dir/scan-budget.sh" ] || exit 1
devexp_budget_why="could not load its scan budget helper"
. "$devexp_dir/scan-budget.sh"
devexp_budget_why="loaded a scan budget helper that defines no entry point"
command -v devexp_scan_budget >/dev/null 2>&1 || exit 1
devexp_budget_why="could not run its scan budget"
devexp_scan_budget secret-guard "$@"

input=$(cat)

result=$(echo "$input" | python3 -I -c "
import sys, json, os, re, shlex

d = json.load(sys.stdin)
tool_name = d.get('tool_name', '')
tool_input = d.get('tool_input', {})

EXACT = {'.env', '.env.local', '.env.production', '.env.staging', '.env.test', '.env.secret'}
KEY_EXTS = ('.pem', '.key', '.p12', '.pfx')
KEY_SUFFIXES = ('_rsa', '_dsa', '_ecdsa', '_ed25519')
# Committed templates name the keys; they never carry the values.
TEMPLATE_SUFFIXES = ('.example', '.sample', '.template', '.dist')

def is_secret(path):
    base = os.path.basename(path)
    if not base:
        return None
    lower = base.lower()

    # A template is safe even when its stem is a real secret name,
    # e.g. .env.production.example
    if lower.endswith(TEMPLATE_SUFFIXES):
        return None

    # Exact names stay blocked regardless of anything below.
    if base in EXACT:
        return base
    if base.startswith('.env.'):
        return base

    # 'server.key' is a key file; a bare '.key' is an extension, which is
    # what a jq filter or a bare word looks like after tokenizing.
    stem = lower.rsplit('.', 1)[0] if '.' in lower else ''
    for ext in KEY_EXTS:
        if lower.endswith(ext) and stem:
            return base
    for sfx in KEY_SUFFIXES:
        if lower.endswith(sfx):
            return base
    return None

# Heredoc bodies are data, not paths: a PR description or commit message
# that merely names a file is not reading it.
HEREDOC = re.compile(r'<<-?[ \t]*[\x27\x22]?([A-Za-z_][A-Za-z0-9_]*)[\x27\x22]?.*?^[ \t]*\1[ \t]*\$', re.S | re.M)

PROGRAM_CHARS = set('|[]{}()\$\`*?<>;&=\n\t')
SEARCHERS = {'grep', 'egrep', 'fgrep', 'rg', 'ag', 'ack'}

def plausible_path(tok):
    if not tok or tok.startswith('-'):
        return False
    if ' ' in tok:          # a quoted program, not a filename
        return False
    return not any(c in tok for c in PROGRAM_CHARS)

found = ''
if tool_name == 'Read':
    found = is_secret(tool_input.get('file_path', '')) or ''
elif tool_name == 'Bash':
    cmd = HEREDOC.sub(' ', tool_input.get('command', ''))
    try:
        tokens = shlex.split(cmd)
    except Exception:
        tokens = cmd.split()
    skip_pattern = False
    for token in tokens:
        # A searcher's first non-flag argument is a pattern, never a path.
        # Auditing for leaked key names is security work, not a secret read.
        if os.path.basename(token) in SEARCHERS:
            skip_pattern = True
            continue
        if skip_pattern:
            if token.startswith('-'):
                continue
            skip_pattern = False
            continue
        if not plausible_path(token):
            continue
        hit = is_secret(token)
        if hit:
            found = hit
            break

# Proof that this scan ran. Written only here, once the scan is over and
# a verdict is in hand, so nothing that skipped the work can produce it; the
# shell blocks when it is missing, whatever this process's exit status.
sys.stdout.write(sys.argv[1] + '\n' + found)
" "$DEVEXP_SCAN_PROOF") || {
    echo "[devexp secret-guard] internal error -- the guard could not read its input, so it did not run. Blocking to be safe; the interpreter's error is above." >&2
    exit 2
}
devexp_scan_result secret-guard "$result"
result="$devexp_scanned"

if [[ -n "$result" ]]; then
    echo "[devexp secret-guard] Blocked access to \"$result\". This file may contain secrets. If intentional, confirm with the user first." >&2
    exit 2
fi

exit 0
