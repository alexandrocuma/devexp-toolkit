#!/usr/bin/env bash
# devexp: PreToolUse hook
#
# Receives a JSON payload on stdin:
#   { "session_id": "...", "transcript_path": "...", "tool_name": "...", "tool_input": { ... } }
#
# To block: output {"decision":"block","reason":"..."} to stdout, exit 0
# To allow: exit 0 with no output (or output {"decision":"allow"})
#
# Hooks in this file:
#   - secret-guard: blocks reads of .env and private key files

set -euo pipefail

input=$(cat)

tool_name=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_name',''))" \
    2>/dev/null || echo "")

# ── secret-guard ──────────────────────────────────────────────────────────────
if [[ "$tool_name" == "Read" ]]; then
    file_path=$(echo "$input" | python3 -c \
        "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" \
        2>/dev/null || echo "")

    basename_val=$(basename "$file_path")

    block=false
    case "$basename_val" in
        .env|.env.local|.env.production|.env.staging|.env.test|.env.secret|.env.*)
            block=true ;;
        *.pem|*.key|*.p12|*.pfx|*_rsa|*_dsa|*_ecdsa|*_ed25519)
            block=true ;;
    esac

    if $block; then
        python3 -c "
import json, sys
print(json.dumps({
    'decision': 'block',
    'reason': '[devexp secret-guard] Blocked read of \"$basename_val\". This file may contain secrets. If intentional, confirm with the user first.'
}))
"
        exit 0
    fi
fi

exit 0
