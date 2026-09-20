#!/usr/bin/env bash
# devexp hook: comment-refs-on-save
# Event: PostToolUse | Matcher: Write|Edit
# Reports comments that cite an issue number, URL or tracker id in the file
# just written. Advisory only -- it cannot block, and never changes the file.
#
# The rule: a comment is self-contained. A reader must not have to open a
# tracker to understand the code in front of them, so where a reference
# carries the reason, the reason is written inline and the history stays in
# the commit body.
#
# Ships DISABLED. Unlike the guards beside it this enforces a house style, not
# a safety property, and plenty of projects deliberately want an issue number
# in a comment. Turn it on per project, in devexp.config.json or the registry.
#
# The scan itself is comment-refs.sh, shared with the repo's lint job so the
# editor and CI can never disagree. Language coverage is its syntax table.
#
# Tests: bash hooks/claude-code/comment-refs-on-save.test.sh
# Mirror: hooks/opencode/comment-refs-on-save.js -- keep the two in lockstep.

set -euo pipefail

input=$(cat)

# A relative path is resolved against the directory the CLI works in (the
# input's cwd, else the hook's own) and normalised, as opencode does. The
# trailing "x" keeps $(...) from trimming newlines that belong to the path.
file_path=$(echo "$input" | python3 -I -c '
import sys, json, os
d = json.load(sys.stdin)
p = str(d.get("tool_input", {}).get("file_path", ""))
c = d.get("cwd")
if p and not os.path.isabs(p):
    p = os.path.normpath(os.path.join(c if isinstance(c, str) and os.path.isabs(c) else os.getcwd(), p))
sys.stdout.write(p + "x")') || {
    echo "[devexp comment-refs-on-save] internal error -- could not read hook input, skipping. The interpreter's error is above." >&2
    exit 0
}
file_path=${file_path%x}

[ -n "$file_path" ] && [ -f "$file_path" ] || exit 0

scan="$(dirname "${BASH_SOURCE[0]}")/comment-refs.sh"
[ -f "$scan" ] || exit 0

# Advisory: the findings go to stderr and the exit stays 0 whatever the scan
# says. A style opinion must not be able to fail a write the user asked for.
if findings=$(bash "$scan" "$file_path" 2>/dev/null); then
    exit 0
fi

[ -n "$findings" ] || exit 0
echo "[devexp comment-refs-on-save] a comment here points outside the file:" >&2
echo "$findings" | sed 's/^/  /' >&2
echo "  Write the reason inline; history belongs in the commit body." >&2
exit 0
