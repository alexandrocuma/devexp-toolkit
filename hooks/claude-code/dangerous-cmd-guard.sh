#!/usr/bin/env bash
# devexp hook: dangerous-cmd-guard
# Event: PreToolUse | Matcher: Bash
# Blocks or prompts on destructive shell commands.
#
# Hard block (exit 2):  rm -rf /, fork bomb, DROP DATABASE
# Soft block (ask JSON): git push --force, git reset --hard, git clean, DROP/TRUNCATE TABLE

set -euo pipefail

input=$(cat)

command=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('command',''))" \
    2>/dev/null || echo "")

# ── Hard blocks ────────────────────────────────────────────────────────────────

# rm -rf targeting filesystem root or home directory
if echo "$command" | grep -qE 'rm\s+-[a-z]*r[a-z]*f\s+(\/\s*$|\/\s+|~\/?(\s|$)|\$HOME(\s|$))' || \
   echo "$command" | grep -qE 'rm\s+-[a-z]*f[a-z]*r\s+(\/\s*$|\/\s+|~\/?(\s|$)|\$HOME(\s|$))'; then
    echo "[devexp dangerous-cmd-guard] Blocked: 'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory." >&2
    exit 2
fi

# Fork bomb
if echo "$command" | grep -qE ':\s*\(\s*\)\s*\{.*\|.*:'; then
    echo "[devexp dangerous-cmd-guard] Blocked: fork bomb pattern detected." >&2
    exit 2
fi

# DROP DATABASE (immediate data loss)
if echo "$command" | grep -qiE 'DROP\s+DATABASE'; then
    echo "[devexp dangerous-cmd-guard] Blocked: DROP DATABASE would permanently destroy a database. Confirm with the user before proceeding." >&2
    exit 2
fi

# ── Soft blocks (ask) ─────────────────────────────────────────────────────────

ask_reason=""

if echo "$command" | grep -qE 'git\s+push\b' && echo "$command" | grep -qE '(--force|-f\b|--force-with-lease)'; then
    ask_reason="git push --force can overwrite remote history and affect other contributors."
fi

if [[ -z "$ask_reason" ]] && echo "$command" | grep -qE 'git\s+reset\b.*--hard'; then
    ask_reason="git reset --hard will permanently discard all uncommitted changes."
fi

if [[ -z "$ask_reason" ]] && echo "$command" | grep -qE 'git\s+clean\b.*-[a-z]*f'; then
    ask_reason="git clean -f will permanently delete untracked files."
fi

if [[ -z "$ask_reason" ]] && echo "$command" | grep -qiE '(DROP\s+TABLE|TRUNCATE\s+TABLE)'; then
    ask_reason="DROP TABLE or TRUNCATE TABLE will permanently destroy table data."
fi

if [[ -n "$ask_reason" ]]; then
    python3 -c "
import json
print(json.dumps({
    'hookSpecificOutput': {
        'permissionDecision': 'ask'
    },
    'systemMessage': '[devexp dangerous-cmd-guard] $ask_reason Confirm this is intentional.'
}))"
    exit 0
fi

exit 0
