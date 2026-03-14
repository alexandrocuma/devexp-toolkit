#!/usr/bin/env bash
# devexp hook: large-file-guard
# Event: PreToolUse | Matcher: Write
# Asks for confirmation before overwriting an existing file with more than 500 lines.
#
# Write replaces the entire file — large overwrites risk data loss if the path is wrong.
# Edit is targeted and exempt from this check.

set -euo pipefail

input=$(cat)

file_path=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" \
    2>/dev/null || echo "")

if [[ -n "$file_path" && -f "$file_path" ]]; then
    line_count=$(wc -l < "$file_path" 2>/dev/null || echo 0)
    if [[ "$line_count" -gt 500 ]]; then
        python3 -c "
import json
print(json.dumps({
    'hookSpecificOutput': {
        'permissionDecision': 'ask'
    },
    'systemMessage': '[devexp large-file-guard] About to overwrite \"$file_path\" ($line_count lines). Confirm this full replacement is intentional.'
}))"
        exit 0
    fi
fi

exit 0
