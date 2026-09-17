#!/usr/bin/env bash
# devexp hook: secret-in-write-guard
# Event: PreToolUse | Matcher: Write|Edit|MultiEdit|NotebookEdit
# Hard-blocks writing content that contains secret or token patterns.
#
# Scans the text being written for high-signal secret patterns: Write
# 'content', Edit 'new_string', MultiEdit 'edits[].new_string' (older Claude
# Code releases) and NotebookEdit 'new_source'. Text being replaced
# ('old_string') is never scanned, so an edit that removes a key is allowed.
# Complements secret-guard which checks filenames.
#
# Matching happens inside the Python step, which fails closed. An earlier
# shell pipeline read some of its own failures as "no match", so some kinds
# of secret and large writes went through unchecked (#101).
#
# Tests: bash hooks/claude-code/secret-in-write-guard.test.sh
# Mirror: hooks/opencode/secret-in-write-guard.js — keep the patterns in lockstep.

set -euo pipefail

input=$(cat)

label=$(echo "$input" | python3 -I -c "
import sys, json, re
d = json.load(sys.stdin)
ti = d.get('tool_input', {})
parts = [ti.get('content'), ti.get('new_string'), ti.get('new_source')]
edits = ti.get('edits')
if isinstance(edits, list):
    parts += [e.get('new_string') for e in edits if isinstance(e, dict)]
content = '\n'.join(str(p) for p in parts if p)

PATTERNS = [
    ('an Anthropic API key (sk-ant-...)', r'sk-ant-[A-Za-z0-9_-]{40,}'),
    ('an OpenAI API key (sk-...)',        r'sk-[A-Za-z0-9]{32,}'),
    # Project, service-account and admin keys: the body has _ and -, so the
    # prefix must start a word, or kebab-case ids like desk-admin-... match.
    ('an OpenAI API key (sk-...)',        r'(^|[^A-Za-z0-9_-])sk-(proj|svcacct|admin)-[A-Za-z0-9_-]{40,}'),
    ('an AWS Access Key ID (AKIA...)',    r'AKIA[0-9A-Z]{16}'),
    # Temporary (STS) key IDs are exactly 20 characters; bounding both ends
    # keeps words like ASIAPACIFICDATACENTER01 from matching.
    ('an AWS temporary Access Key ID (ASIA...)', r'(^|[^A-Za-z0-9])ASIA[0-9A-Z]{16}([^A-Za-z0-9]|$)'),
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
