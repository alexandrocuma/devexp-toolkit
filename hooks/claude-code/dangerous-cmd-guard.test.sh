#!/usr/bin/env bash
# Tests for dangerous-cmd-guard.sh — exit 2 = blocked, exit 0 = allowed.
# Run: bash hooks/claude-code/dangerous-cmd-guard.test.sh
set -uo pipefail

HOOK="$(cd "$(dirname "$0")" && pwd)/dangerous-cmd-guard.sh"
pass=0; fail=0

# Feed a command to the hook as the real PreToolUse JSON envelope; return its exit code.
run() {
  python3 -c 'import json,sys; print(json.dumps({"tool_input":{"command":sys.argv[1]}}))' "$1" \
    | bash "$HOOK" >/dev/null 2>&1
  echo $?
}

expect() { # $1=block|allow  $2=command
  local want="$1" cmd="$2" rc; rc=$(run "$cmd")
  if { [ "$want" = block ] && [ "$rc" = 2 ]; } || { [ "$want" = allow ] && [ "$rc" = 0 ]; }; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL [want %s, rc %s]: %s\n' "$want" "$rc" "$cmd"
  fi
}

# ── must BLOCK ──────────────────────────────────────────────────────────────
expect block 'rm -f /tmp/*'
expect block 'rm -rf /tmp'
expect block 'rm -rf /tmp/'
expect block 'rm -f /tmp/*"$ticket"*'                       # empty-var template collapses to /tmp/*
expect block 'rm -rf ~/.claude'
expect block 'rm -rf $HOME/.claude'
expect block 'rm -f ~/.claude/agent-memory/grooming-agent/sessions/*"$id"*'
expect block 'rm -f ~/.claude/agent-memory/*'
expect block 'rm -rf /'                                      # pre-existing rule still holds
expect block 'git push --force'
expect block 'git push origin main -f'
expect block 'git push --force-with-lease'

# ── must ALLOW ──────────────────────────────────────────────────────────────
expect allow 'rm -f /tmp/.deliver-PAY-123-*'                 # prefix-anchored toolkit scratch
expect allow 'rm -f /tmp/.recently_changed.txt'             # specific file (improve uses this)
expect allow 'rm -f ~/.claude/agent-memory/grooming-agent/plans/PAY-123.md'
expect allow 'rm -f ~/.claude/agent-memory/grooming-agent/sessions/PAY-123-*'
expect allow 'git push origin main'
expect allow 'git commit -m "fix rm -f false positive" ; git push'   # the bug this PR fixes
expect allow 'git push && rm -f /tmp/.deliver-X-1'

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
