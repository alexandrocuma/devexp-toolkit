#!/usr/bin/env bash
# devexp hook: graphify-session-sentinel
# Event: PostToolUse | Matcher: Bash
#
# Records each `graphify query/path/explain` toward graphify-read-guard's
# tapering gate by incrementing queries_done in the shared session state.
# Also seeds/resets that state on a new session_id, so a query run before
# any read still initializes the cycle correctly. Ships disabled by default
# — only meaningful alongside graphify-read-guard.
#
# State: graphify-out/.graphify_session (JSON) — shared with graphify-read-guard
#
# Advisory only: always exits 0.

set -euo pipefail

python3 -c "$(cat <<'PYEOF'
import json, sys, os

d = json.load(sys.stdin)
t = d.get("tool_input", d)
cmd = str(t.get("command") or "")
session_id = str(d.get("session_id") or "")

GRAPHIFY_CMDS = ("graphify query", "graphify path", "graphify explain")
if not any(c in cmd for c in GRAPHIFY_CMDS) or not os.path.exists("graphify-out"):
    sys.exit(0)

SENTINEL = "graphify-out/.graphify_session"
DEFAULT_STATE = {"session_id": session_id, "phase": 0, "queries_done": 0, "reads_done": 0, "unlocked": False}

try:
    with open(SENTINEL) as f:
        state = json.load(f)
    if not isinstance(state, dict) or "session_id" not in state:
        raise ValueError("not a session state object")
except Exception:
    state = dict(DEFAULT_STATE)

if state.get("session_id") != session_id:
    state = dict(DEFAULT_STATE)

state["queries_done"] = state.get("queries_done", 0) + 1

try:
    with open(SENTINEL, "w") as f:
        json.dump(state, f)
except Exception:
    pass
PYEOF
)" 2>/dev/null || true

exit 0
