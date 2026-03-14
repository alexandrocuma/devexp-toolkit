#!/usr/bin/env bash
# devexp hook: secret-in-write-guard
# Event: PreToolUse | Matcher: Write|Edit
# Hard-blocks writing content that contains secret or token patterns.
#
# Scans the content being written (Write: 'content', Edit: 'new_string') for
# high-signal secret patterns. Complements secret-guard which checks filenames.

set -euo pipefail

input=$(cat)

content=$(echo "$input" | python3 -c "
import sys, json
d = json.load(sys.stdin)
ti = d.get('tool_input', {})
print(ti.get('content', '') or ti.get('new_string', ''))
" 2>/dev/null || echo "")

if [[ -z "$content" ]]; then
    exit 0
fi

check() {
    local label="$1"
    local pattern="$2"
    if echo "$content" | grep -qE "$pattern"; then
        echo "[devexp secret-in-write-guard] Blocked: content appears to contain $label. Remove the secret before writing." >&2
        exit 2
    fi
}

check "an Anthropic API key (sk-ant-...)"  'sk-ant-[A-Za-z0-9_\-]{40,}'
check "an OpenAI API key (sk-...)"          'sk-[A-Za-z0-9]{32,}'
check "an AWS Access Key ID (AKIA...)"      'AKIA[0-9A-Z]{16}'
check "a GitHub token (ghp_, ghs_, etc.)"  'gh[posta]_[A-Za-z0-9_]{36,}'
check "a Slack token (xox...)"             'xox[baprs]-[0-9A-Za-z\-]{10,}'
check "a private key block"                '-----BEGIN [A-Z ]*(PRIVATE|SECRET) KEY'

exit 0
