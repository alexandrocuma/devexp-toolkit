#!/usr/bin/env bash
# devexp hook: graphify-grep-nudge
# Event: PreToolUse | Matcher: Bash|Grep
#
# Soft-nudges toward `graphify query` when a grep-like Bash command or the
# built-in Grep tool runs — injects additionalContext, never blocks.
# Grepping raw files is often a legitimate, fast choice; this just surfaces
# the cheaper alternative when a knowledge graph is available. Ships
# disabled by default — pairs with the graphify skill and graphify-read-guard.
#
# Always exits 0 — advisory only.

set -euo pipefail

exec python3 -c "$(cat <<'PYEOF'
import json, sys, os, re

d = json.load(sys.stdin)
tool_name = str(d.get("tool_name") or "")
t = d.get("tool_input", d)

if not os.path.exists("graphify-out/graph.json"):
    sys.exit(0)

GREP_LIKE = re.compile(r"(?:^|[\s;|&])(?:grep|rg|ripgrep|find|fd|ack|ag)\s")

is_grep_tool = tool_name == "Grep"
is_grep_cmd = tool_name == "Bash" and bool(GREP_LIKE.search(str(t.get("command") or "")))

if not (is_grep_tool or is_grep_cmd):
    sys.exit(0)

print(json.dumps({
    "hookSpecificOutput": {
        "hookEventName": "PreToolUse",
        "additionalContext": (
            "graphify: knowledge graph at graphify-out/. For focused questions, run "
            "graphify query \"<question>\" (scoped subgraph, usually much smaller "
            "than grep output) instead of grepping raw files. Read GRAPH_REPORT.md "
            "only for broad architecture context."
        ),
    }
}))
PYEOF
)"
