#!/usr/bin/env bash
# Tests for secret-guard.sh — exit 2 = blocked, exit 0 = allowed.
# Run: bash hooks/claude-code/secret-guard.test.sh
set -uo pipefail

HOOK="$(cd "$(dirname "$0")" && pwd)/secret-guard.sh"
pass=0; fail=0

# Feed the hook the real PreToolUse JSON envelope; return its exit code.
run() { # $1=Read|Bash  $2=payload
  python3 -c '
import json, sys
tool, payload = sys.argv[1], sys.argv[2]
key = "file_path" if tool == "Read" else "command"
print(json.dumps({"tool_name": tool, "tool_input": {key: payload}}))' "$1" "$2" \
    | bash "$HOOK" >/dev/null 2>&1
  echo $?
}

expect() { # $1=block|allow  $2=Read|Bash  $3=payload
  local want="$1" tool="$2" payload="$3" rc; rc=$(run "$tool" "$payload")
  if { [ "$want" = block ] && [ "$rc" = 2 ]; } || { [ "$want" = allow ] && [ "$rc" = 0 ]; }; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL [want %s, rc %s] %s: %s\n' "$want" "$rc" "$tool" "$payload"
  fi
}

DOTENV=$(printf '.env')
TEMPLATE="${DOTENV}.example"

# ── must BLOCK: real secrets, via Read ──────────────────────────────────────
expect block Read "$DOTENV"
expect block Read "mcps/$DOTENV"
expect block Read "${DOTENV}.production"
expect block Read "${DOTENV}.local"
expect block Read "certs/server.pem"
expect block Read "certs/server.key"
expect block Read "bundle.p12"
expect block Read "$HOME/.ssh/id_rsa"
expect block Read "deploy_ed25519"

# ── must BLOCK: real secrets, via Bash ──────────────────────────────────────
expect block Bash "cat $DOTENV"
expect block Bash "head -5 mcps/$DOTENV"
expect block Bash "less ${DOTENV}.production"
expect block Bash "cat certs/server.key"
expect block Bash "cp mcps/$DOTENV /tmp/backup"
expect block Bash "cat $HOME/.ssh/id_rsa"
expect block Bash "openssl x509 -in certs/server.pem -text"

# ── must ALLOW: committed templates are not secrets ─────────────────────────
expect allow Read "$TEMPLATE"
expect allow Read "mcps/$TEMPLATE"
expect allow Read "${DOTENV}.production.example"
expect allow Read "config.sample"
expect allow Read "server.pem.template"
expect allow Bash "cat mcps/$TEMPLATE"
expect allow Bash "grep -c UI_INSPECTOR_DIR mcps/$TEMPLATE"

# ── must ALLOW: a mention is not a read ─────────────────────────────────────
expect allow Bash "jq -r '.key' entries.json"
expect allow Bash "jq '.entries[].key' seed.json"
expect allow Bash "echo .key"
expect allow Bash "python3 -c \"print('server.pem')\""
expect allow Bash "git commit -m 'document the key in the env template'"
expect allow Bash "grep -rn 'id_rsa' docs/"
expect allow Bash "grep -rn id_rsa docs/"
expect allow Bash "rg id_rsa ."

# ...but the same bare token in a path position is still a real read:
expect block Bash "cat id_rsa"
expect block Bash "grep secret id_rsa"
expect allow Bash "$(printf 'cat <<%sEOF%s\nmentions %s.production in prose\nEOF' "'" "'" "$DOTENV")"

# ── must ALLOW: ordinary work ───────────────────────────────────────────────
expect allow Bash "cat README.md"
expect allow Bash "ls mcps/"
expect allow Read "mcps/registry.json"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
