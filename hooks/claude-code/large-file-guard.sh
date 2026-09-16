#!/usr/bin/env bash
# devexp hook: large-file-guard
# Event: PreToolUse | Matcher: Write
# Asks for confirmation before overwriting an existing file with more than 500 lines.
#
# Write replaces the entire file — large overwrites risk data loss if the path is wrong.
# Edit is targeted and exempt from this check.
#
# Tests: bash hooks/claude-code/large-file-guard.test.sh

set -euo pipefail

input=$(cat)

# The trailing "x" keeps $(...) from trimming newlines that belong to the path.
file_path=$(echo "$input" | python3 -I -c \
    "import sys,json; d=json.load(sys.stdin); sys.stdout.write(str(d.get('tool_input',{}).get('file_path','')) + 'x')") || {
    echo "[devexp large-file-guard] internal error -- could not read hook input, skipping. The interpreter's error is above." >&2
    exit 0
}
file_path=${file_path%x}

if [[ -n "$file_path" && -f "$file_path" ]]; then
    line_count=$(wc -l < "$file_path" 2>/dev/null || echo 0)
    if [[ "$line_count" -gt 500 ]]; then
        # Values go in as argv, never into the program text: the quoted heredoc
        # delimiter keeps bash from expanding anything inside the script.
        python3 -I - "$file_path" "$line_count" <<'PYASK' || { echo "[devexp large-file-guard] internal error -- could not build the confirmation prompt, skipping. The interpreter's error is above." >&2; exit 0; }
import json, sys
file_path, line_count = sys.argv[1], sys.argv[2]
print(json.dumps({
    'hookSpecificOutput': {
        'permissionDecision': 'ask'
    },
    'systemMessage': '[devexp large-file-guard] About to overwrite "' + file_path + '" (' + line_count + ' lines). Confirm this full replacement is intentional.'
}))
PYASK
        exit 0
    fi
fi

exit 0
