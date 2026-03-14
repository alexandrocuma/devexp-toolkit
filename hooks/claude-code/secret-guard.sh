#!/usr/bin/env bash
# devexp hook: secret-guard
# Event: PreToolUse | Matcher: Read
# Blocks accidental reads of .env and private key files.
#
# To block: print reason to stderr, exit 2
# To allow: exit 0 with no output

set -euo pipefail

input=$(cat)

file_path=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" \
    2>/dev/null || echo "")

basename_val=$(basename "$file_path")

case "$basename_val" in
    .env|.env.local|.env.production|.env.staging|.env.test|.env.secret|.env.*)
        echo "[devexp secret-guard] Blocked read of \"$basename_val\". This file may contain secrets. If intentional, confirm with the user first." >&2
        exit 2 ;;
    *.pem|*.key|*.p12|*.pfx|*_rsa|*_dsa|*_ecdsa|*_ed25519)
        echo "[devexp secret-guard] Blocked read of \"$basename_val\". This file may contain secrets. If intentional, confirm with the user first." >&2
        exit 2 ;;
esac

exit 0
