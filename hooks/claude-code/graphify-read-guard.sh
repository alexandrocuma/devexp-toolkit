#!/usr/bin/env bash
# devexp hook: graphify-read-guard
# Event: PreToolUse | Matcher: Read|Glob
#
# Gates source-file reads/globs behind graphify usage with a tapering
# session cadence rather than a flat timer:
#
#   Phase 0 (fresh session): 5 graphify queries required to unlock
#   Phase 1: 3 queries required
#   Phase 2+ (steady state floor): 1 query required
#
# Each unlock grants a budget of 6 reads; exhausting the budget re-arms the
# gate at the next (tighter) phase — heavy grounding up front when context
# is thinnest, a lighter touch once the agent has shown sustained engagement
# with the graph. Requires the graphify skill and an existing
# graphify-out/graph.json — ships disabled by default.
#
# State: graphify-out/.graphify_session (JSON), shared with graphify-session-sentinel
#   {"session_id", "phase", "queries_done", "reads_done", "unlocked"}
#
# To block: print reason to stderr, exit 2
# To allow: exit 0 with no output (or hookSpecificOutput JSON for extra context)

set -euo pipefail

exec python3 -c "$(cat <<'PYEOF'
import json, sys, os, re

d = json.load(sys.stdin)
t = d.get("tool_input", d)
session_id = str(d.get("session_id") or "")
s = " ".join([
    str(t.get("file_path") or ""),
    str(t.get("pattern") or ""),
    str(t.get("path") or ""),
]).lower().replace("\\", "/")

SOURCE_EXTS = (
    ".py", ".js", ".ts", ".tsx", ".jsx", ".go", ".rs", ".java", ".rb",
    ".c", ".h", ".cpp", ".hpp", ".cc", ".cs", ".kt", ".swift", ".php",
    ".scala", ".lua", ".vue", ".svelte", ".ex", ".exs", ".clj", ".hs",
    ".sh", ".bash", ".zsh", ".fish",
)

is_source = any(
    re.search(re.escape(ext) + r"(?![a-z0-9])", s) for ext in SOURCE_EXTS
)
in_graphify = "graphify-out/" in s
graph_exists = os.path.exists("graphify-out/graph.json")

if not is_source or in_graphify or not graph_exists:
    sys.exit(0)

SENTINEL = "graphify-out/.graphify_session"
REQUIRED = (5, 3, 1)   # tapers down each re-arm cycle, floors at 1
READ_BUDGET = 6


def default_state():
    return {"session_id": session_id, "phase": 0, "queries_done": 0, "reads_done": 0, "unlocked": False}


def load_state():
    try:
        with open(SENTINEL) as f:
            data = json.load(f)
        if isinstance(data, dict) and "session_id" in data:
            return data
    except Exception:
        pass
    return default_state()


def save_state(state):
    try:
        with open(SENTINEL, "w") as f:
            json.dump(state, f)
    except Exception:
        pass


def block(message):
    sys.stderr.write(message)
    sys.exit(2)


def allow(context):
    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "additionalContext": context,
        }
    }))
    sys.exit(0)


state = load_state()
if state.get("session_id") != session_id:
    state = default_state()
    save_state(state)

phase = min(state.get("phase", 0), len(REQUIRED) - 1)
required = REQUIRED[phase]

if not state.get("unlocked"):
    queries_done = state.get("queries_done", 0)
    if queries_done < required:
        remaining = required - queries_done
        block(
            "[devexp graphify-read-guard] Blocked — graphify grounding required.\n\n"
            f"Run graphify query \"<your question>\" {remaining} more time(s) "
            f"before reading source files ({queries_done}/{required} done this cycle).\n\n"
            "The knowledge graph answers most codebase questions faster and cheaper than reading files.\n"
        )
    state["unlocked"] = True
    state["reads_done"] = 0
    save_state(state)
    allow(
        f"graphify: grounded for this cycle — up to {READ_BUDGET} reads allowed "
        "before re-querying is required again."
    )
else:
    reads_done = state.get("reads_done", 0)
    if reads_done < READ_BUDGET:
        state["reads_done"] = reads_done + 1
        save_state(state)
        remaining = READ_BUDGET - state["reads_done"]
        allow(
            f"graphify: read {state['reads_done']}/{READ_BUDGET} this cycle — "
            f"{remaining} more before re-grounding is required."
        )
    else:
        next_phase = min(phase + 1, len(REQUIRED) - 1)
        next_required = REQUIRED[next_phase]
        state["phase"] = next_phase
        state["queries_done"] = 0
        state["unlocked"] = False
        save_state(state)
        block(
            "[devexp graphify-read-guard] Blocked — re-grounding needed.\n\n"
            f"You've read {READ_BUDGET} files since the last graphify query. "
            f"Run graphify query \"<your question>\" {next_required} more time(s) "
            "to continue reading source files.\n"
        )
PYEOF
)"
