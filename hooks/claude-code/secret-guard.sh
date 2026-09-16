#!/usr/bin/env bash
# devexp hook: secret-guard
# Event: PreToolUse | Matcher: Read|Bash
# Blocks accidental reads of .env and private key files via the Read tool
# or Bash commands (cat, head, tail, etc.).
#
# Two false-positive classes are deliberately excluded:
#
#   Templates (#87). A committed `.env.example` documents which keys exist;
#   it never holds their values. Blocking it blocks the one file a user is
#   meant to read in order to configure the others.
#
#   Mentions (#81). A shell token only counts as a path if it plausibly is
#   one. Heredoc bodies, program text (`jq -r '.key'`), quoted programs and
#   bare extensions are not file reads, and refusing them obstructs without
#   protecting anything.
#
# To block: print reason to stderr, exit 2
# To allow: exit 0 with no output
#
# Tests: bash hooks/claude-code/secret-guard.test.sh

set -euo pipefail

input=$(cat)

result=$(echo "$input" | python3 -c "
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

if tool_name == 'Read':
    hit = is_secret(tool_input.get('file_path', ''))
    if hit:
        print(hit)
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
            print(hit)
            break
") || {
    echo "[devexp secret-guard] internal error -- the guard could not read its input, so it did not run. Blocking to be safe; the interpreter's error is above." >&2
    exit 2
}

if [[ -n "$result" ]]; then
    echo "[devexp secret-guard] Blocked access to \"$result\". This file may contain secrets. If intentional, confirm with the user first." >&2
    exit 2
fi

exit 0
