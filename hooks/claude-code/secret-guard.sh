#!/usr/bin/env bash
# devexp hook: secret-guard
# Event: PreToolUse | Matcher: Read|Bash
# Blocks accidental reads of .env and private key files via the Read tool
# or Bash commands (cat, head, tail, etc.).
#
# To block: print reason to stderr, exit 2
# To allow: exit 0 with no output

set -euo pipefail

input=$(cat)

result=$(echo "$input" | python3 -c "
import sys, json, os, shlex

d = json.load(sys.stdin)
tool_name = d.get('tool_name', '')
tool_input = d.get('tool_input', {})

def is_secret(path):
    base = os.path.basename(path)
    if not base:
        return None
    if base in {'.env', '.env.local', '.env.production', '.env.staging', '.env.test', '.env.secret'}:
        return base
    if base.startswith('.env.'):
        return base
    lower = base.lower()
    for ext in ('.pem', '.key', '.p12', '.pfx'):
        if lower.endswith(ext):
            return base
    for sfx in ('_rsa', '_dsa', '_ecdsa', '_ed25519'):
        if lower.endswith(sfx):
            return base
    return None

if tool_name == 'Read':
    hit = is_secret(tool_input.get('file_path', ''))
    if hit:
        print(hit)
elif tool_name == 'Bash':
    cmd = tool_input.get('command', '')
    try:
        tokens = shlex.split(cmd)
    except Exception:
        tokens = cmd.split()
    for token in tokens:
        if token.startswith('-'):
            continue
        hit = is_secret(token)
        if hit:
            print(hit)
            break
" 2>/dev/null || echo "")

if [[ -n "$result" ]]; then
    echo "[devexp secret-guard] Blocked access to \"$result\". This file may contain secrets. If intentional, confirm with the user first." >&2
    exit 2
fi

exit 0
