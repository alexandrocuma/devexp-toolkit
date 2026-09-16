#!/usr/bin/env bash
# Tests uninstall.sh's hook-removal logic.
#
# That logic used to match on `repo_dir in cmd`, so it removed only hooks
# registered from the current clone. A registration left by an earlier
# release-binary install lives elsewhere, so uninstalling stripped the working
# entries and left the stale ones running — strictly worse than doing nothing
# (issue #93).
#
# The block is embedded in uninstall.sh as a heredoc, so it is extracted here
# and exercised directly against fixtures. Running uninstall.sh itself would
# prompt and delete real files.
#
# Run: bash uninstall.test.sh
set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

# ── Extract the embedded hook-removal program ───────────────────────────────
awk '/python3 - "\$REPO_DIR" "\$settings_path" <</{flag=1;next} /^PYEOF$/{flag=0} flag' \
    "$ROOT/uninstall.sh" > "$TMP/prune.py"
if [ ! -s "$TMP/prune.py" ]; then
    echo "FAIL: could not extract the hook-removal block from uninstall.sh"
    exit 1
fi

# ── A fake devexp repo, so the registry is the source of managed names ──────
REPO="$TMP/repo"
mkdir -p "$REPO/hooks/claude-code"
cat > "$REPO/hooks/registry.json" <<'JSON'
[
  {"name": "secret-guard", "enabled": true,
   "claude_code": {"event": "PreToolUse", "matcher": "Read|Bash",
                   "script": "hooks/claude-code/secret-guard.sh"}},
  {"name": "graphify-read-guard", "enabled": false,
   "claude_code": {"event": "PreToolUse", "matcher": "Read|Glob",
                   "script": "hooks/claude-code/graphify-read-guard.sh"}}
]
JSON

OTHER="$TMP/other-root"

run() { # $1 = settings json  -> prints resulting commands, one per line
    printf '%s' "$1" > "$TMP/settings.json"
    python3 "$TMP/prune.py" "$REPO" "$TMP/settings.json" >/dev/null 2>&1
    python3 - "$TMP/settings.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
for ev, arr in sorted((d.get('hooks') or {}).items()):
    for e in arr:
        for h in e.get('hooks', []):
            print(h.get('command', ''))
PY
}

expect() { # $1=label  $2=settings  $3=expected remaining commands (newline sep)
    local label="$1" got want
    got="$(run "$2")"
    want="$(printf '%s' "$3")"
    if [ "$got" = "$want" ]; then
        pass=$((pass+1))
    else
        fail=$((fail+1))
        printf 'FAIL %s\n  got:  %s\n  want: %s\n' "$label" "${got:-<empty>}" "${want:-<empty>}"
    fi
}

MINE="$REPO/hooks/claude-code/secret-guard.sh"
FOREIGN="$OTHER/hooks/claude-code/secret-guard.sh"
FOREIGN_DISABLED="$OTHER/hooks/claude-code/graphify-read-guard.sh"
USER_HOOK="$OTHER/my-hooks/secret-guard.sh"
USER_OTHER="$OTHER/my-hooks/format-check.sh"

# Removes our own registration (the normal uninstall case)
expect "removes the clone's own hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$MINE\"}]}]}}" \
  ""

# #93: also removes one registered from a different install root
expect "removes a hook from a foreign install root" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN\"}]}]}}" \
  ""

# Disabled hooks are devexp's too
expect "removes a foreign copy of a disabled hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Glob\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN_DISABLED\"}]}]}}" \
  ""

# A user hook sharing a basename must survive
expect "keeps a user hook that shares a basename" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$USER_HOOK\"}]}]}}" \
  "$USER_HOOK"

# An unrelated user hook must survive
expect "keeps an unrelated user hook" \
  "{\"hooks\":{\"PostToolUse\":[{\"matcher\":\"Write\",\"hooks\":[{\"type\":\"command\",\"command\":\"$USER_OTHER\"}]}]}}" \
  "$USER_OTHER"

# The bug this also fixes: entries were judged by hooks[0] alone, so a second
# command in the same entry was handled by accident rather than on its merits.
expect "an entry holding both keeps only the user command" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN\"},{\"type\":\"command\",\"command\":\"$USER_HOOK\"}]}]}}" \
  "$USER_HOOK"

expect "a user command listed first no longer shields a devexp one" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$USER_HOOK\"},{\"type\":\"command\",\"command\":\"$FOREIGN\"}]}]}}" \
  "$USER_HOOK"

# Both roots registered at once — the real-world state issue #93 describes
expect "removes both roots' copies of the same hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$MINE\"}]},{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN\"}]}]}}" \
  ""

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
