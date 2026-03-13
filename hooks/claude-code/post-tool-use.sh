#!/usr/bin/env bash
# devexp: PostToolUse hook
#
# Receives a JSON payload on stdin:
#   { "session_id": "...", "tool_name": "...", "tool_input": { ... }, "tool_response": { ... } }
#
# PostToolUse hooks run after a tool completes. They cannot block — they are advisory only.
# Output to stderr is shown to the user as a notification.
#
# Hooks in this file:
#   - lint-on-save: advisory after source file edits

set -euo pipefail

input=$(cat)

tool_name=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_name',''))" \
    2>/dev/null || echo "")

# ── lint-on-save ──────────────────────────────────────────────────────────────
if [[ "$tool_name" == "Write" || "$tool_name" == "Edit" ]]; then
    file_path=$(echo "$input" | python3 -c \
        "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" \
        2>/dev/null || echo "")

    case "$file_path" in
        *.js|*.jsx|*.ts|*.tsx|*.mjs|*.cjs|\
        *.py|\
        *.go|\
        *.rs|\
        *.rb|\
        *.java|*.kt|*.scala|\
        *.swift|\
        *.c|*.cpp|*.cc|*.h|*.hpp)
            echo "[devexp lint-on-save] $file_path edited — consider running the project linter" >&2
            ;;
    esac
fi

exit 0
