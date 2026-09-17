#!/usr/bin/env bash
# devexp hook: secret-in-write-guard
# Event: PreToolUse | Matcher: Write|Edit
# Hard-blocks writing content that contains secret or token patterns.
#
# Scans the content being written (Write: 'content', Edit: 'new_string') for
# high-signal secret patterns. Complements secret-guard which checks filenames.
#
# Matching happens inside the Python step, which fails closed. It used to be
# `echo "$content" | grep -qE "$pattern"`, and that read two failures as
# "no match": a pattern starting with dashes is parsed by grep as an option
# (so private keys were never caught), and under pipefail grep -q exiting on
# the first match SIGPIPEs echo on any write larger than the pipe buffer (so
# large writes were never caught).
#
# Tests: bash hooks/claude-code/secret-in-write-guard.test.sh
# Mirror: hooks/opencode/secret-in-write-guard.js — keep the patterns in lockstep.

set -euo pipefail

input=$(cat)

label=$(echo "$input" | python3 -I -c "
import sys, json, re
d = json.load(sys.stdin)
ti = d.get('tool_input', {})
content = str(ti.get('content', '') or ti.get('new_string', '') or '')

PATTERNS = [
    ('an Anthropic API key (sk-ant-...)', r'sk-ant-[A-Za-z0-9_-]{40,}'),
    ('an OpenAI API key (sk-...)',        r'sk-[A-Za-z0-9]{32,}'),
    # Project, service-account and admin keys: the body has _ and -, so the
    # prefix must start a word, or kebab-case ids like desk-admin-... match.
    ('an OpenAI API key (sk-...)',        r'(^|[^A-Za-z0-9_-])sk-(proj|svcacct|admin)-[A-Za-z0-9_-]{40,}'),
    ('an AWS Access Key ID (AKIA...)',    r'AKIA[0-9A-Z]{16}'),
    ('a GitHub token (ghp_, ghs_, etc.)', r'gh[postaur]_[A-Za-z0-9_]{36,}'),
    ('a GitHub token (ghp_, ghs_, etc.)', r'github_pat_[A-Za-z0-9_]{36,}'),
    ('a Slack token (xox...)',            r'xox[baprs]-[0-9A-Za-z-]{10,}'),
    ('a private key block',               r'-----BEGIN [A-Z ]*(PRIVATE|SECRET) KEY'),
]
for name, pattern in PATTERNS:
    if re.search(pattern, content, re.M):
        print(name)
        break
") || {
    echo "[devexp secret-in-write-guard] internal error -- the guard could not read its input, so it did not run. Blocking to be safe; the interpreter's error is above." >&2
    exit 2
}

if [[ -n "$label" ]]; then
    echo "[devexp secret-in-write-guard] Blocked: content appears to contain $label. Remove the secret before writing." >&2
    exit 2
fi

exit 0
