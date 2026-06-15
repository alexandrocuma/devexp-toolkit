#!/usr/bin/env bash
# devexp hook: dangerous-cmd-guard
# Event: PreToolUse | Matcher: Bash
# Hard-blocks destructive shell commands. No prompts — all guarded patterns are blocked.

set -euo pipefail

input=$(cat)

command=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('command',''))" \
    2>/dev/null || echo "")

# ── Blocked patterns ───────────────────────────────────────────────────────────

# rm -rf targeting filesystem root or home directory
if echo "$command" | grep -qE 'rm\s+-[a-z]*r[a-z]*f\s+(\/\s*$|\/\s+|~\/?(\s|$)|\$HOME(\s|$))' || \
   echo "$command" | grep -qE 'rm\s+-[a-z]*f[a-z]*r\s+(\/\s*$|\/\s+|~\/?(\s|$)|\$HOME(\s|$))'; then
    echo "[devexp dangerous-cmd-guard] Blocked: 'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory." >&2
    exit 2
fi

# Unanchored wildcard delete in a sensitive dir: a '*' immediately after the dir boundary
# (/tmp/* , ~/.claude/.../* ), or the dir wholesale (rm -rf /tmp , rm -rf ~/.claude).
# This is the blanket-wipe an empty variable produces — `rm -f /tmp/*"$id"*` with empty $id
# collapses to `/tmp/*`, and the template itself contains `/tmp/*`. Prefix-anchored globs like
# `/tmp/.deliver-PAY-123-*` are allowed (no '*' right after the '/').
if echo "$command" | grep -qE 'rm\b[^|]*(\s["'\'']?/tmp["'\'']?(/\*|/?(\s|$))|["'\'']?(\$HOME|~)["'\'']?/\.claude(\S*/\*|["'\'']?/?(\s|$))|\.claude\S*/\*)'; then
    echo "[devexp dangerous-cmd-guard] Blocked: unanchored wildcard delete in a sensitive directory (e.g. '/tmp/*' or '~/.claude/.../*'). Anchor the glob with a literal prefix (e.g. '/tmp/.deliver-<id>-*') so an empty variable cannot collapse it into a blanket wipe." >&2
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

# Force push — match the force flag only as an argument of the SAME push command (no intervening
# ; | & ), so an unrelated `-f` elsewhere (e.g. `rm -f` in a commit message) no longer false-positives.
if echo "$command" | grep -qE 'git\s+push\b[^|&;]*\s(--force-with-lease|--force|-f)(\s|=|$)'; then
    echo "[devexp dangerous-cmd-guard] Blocked: git push --force can overwrite remote history and affect other contributors." >&2
    exit 2
fi

if echo "$command" | grep -qE 'git\s+reset\b.*--hard'; then
    echo "[devexp dangerous-cmd-guard] Blocked: git reset --hard will permanently discard all uncommitted changes." >&2
    exit 2
fi

if echo "$command" | grep -qE 'git\s+clean\b.*-[a-z]*f'; then
    echo "[devexp dangerous-cmd-guard] Blocked: git clean -f will permanently delete untracked files." >&2
    exit 2
fi

if echo "$command" | grep -qiE '(DROP\s+TABLE|TRUNCATE\s+TABLE)'; then
    echo "[devexp dangerous-cmd-guard] Blocked: DROP TABLE or TRUNCATE TABLE will permanently destroy table data." >&2
    exit 2
fi

exit 0
