#!/usr/bin/env bash
# start-services.sh — Start devexp MCP services without reinstalling or wiping data.
#
# Safe to run at any time:
#   - Skips already-running services
#   - Never touches ~/.openviking/data (no memory loss)
#   - Never rebuilds venvs
#   - Checks Jina Docker container health separately
#
# Usage:
#   ./start-services.sh            # start everything that isn't running
#   ./start-services.sh --status   # show service status only
set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
RESET='\033[0m'

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OV_DIR="$HOME/.openviking"
OV_VENV="$OV_DIR/venv"
OV_CONF="$OV_DIR/ov.conf"
OV_DATA="$OV_DIR/data"
OV_PID="$OV_DIR/mcp.pid"
OV_LOG="$OV_DIR/mcp.log"
OV_PORT=2033
JINA_PORT=8081
JINA_PID_FILE="$OV_DIR/jina.pid"

STATUS_ONLY=false
[[ "${1:-}" == "--status" ]] && STATUS_ONLY=true

info()    { echo -e "${BOLD}==>${RESET} $*"; }
ok()      { echo -e "  ${GREEN}✓${RESET} $*"; }
skip()    { echo -e "  ${YELLOW}~${RESET} $*"; }
warn()    { echo -e "  ${RED}!${RESET} $*"; }

# ── Jina status ───────────────────────────────────────────────────────────────

jina_is_up() {
    # Docker ports aren't visible to lsof — use curl to check HTTP health
    curl -sf --max-time 3 "http://localhost:$JINA_PORT/health" >/dev/null 2>&1 && return 0
    # Fallback: check local process (pip-based Jina)
    lsof -ti:"$JINA_PORT" >/dev/null 2>&1 && return 0
    return 1
}

check_jina() {
    info "Jina embeddings server (port $JINA_PORT)"

    if jina_is_up; then
        # Clarify whether it's Docker or pip
        if [[ -f "$JINA_PID_FILE" ]] && [[ "$(cat "$JINA_PID_FILE")" == docker:* ]]; then
            ok "Running via Docker on port $JINA_PORT"
        else
            ok "Running on port $JINA_PORT"
        fi
        return 0
    fi

    if [[ -f "$JINA_PID_FILE" ]]; then
        local entry; entry=$(cat "$JINA_PID_FILE")
        if [[ "$entry" == docker:* ]]; then
            local container="${entry#docker:}"
            if $STATUS_ONLY; then
                warn "Docker container '$container' not responding on port $JINA_PORT"
                return 1
            fi
            echo -e "  Restarting Docker container '$container'..."
            docker start "$container" 2>/dev/null \
                && ok "Restarted '$container'" \
                || warn "Could not restart — run: docker start $container"
            return
        fi
        # pip-based: try to restart the process
        local pid="$entry"
        if ! kill -0 "$pid" 2>/dev/null; then
            if $STATUS_ONLY; then warn "Not running (stale pid $pid)"; return 1; fi
            # Restart via pip venv
            local jina_venv="$OV_DIR/jina-venv"
            if [[ -x "$jina_venv/bin/infinity_emb" ]]; then
                local jina_log="$OV_DIR/jina.log"
                nohup "$jina_venv/bin/infinity_emb" v2 \
                    --model-id jinaai/jina-embeddings-v2-base-en \
                    --engine torch --no-bettertransformer \
                    --port "$JINA_PORT" > "$jina_log" 2>&1 &
                echo $! > "$JINA_PID_FILE"
                sleep 3
                jina_is_up && ok "Restarted Jina (pid $!)" || warn "Failed to restart Jina — check $jina_log"
                return
            fi
        fi
    fi

    warn "Jina is not running and no known restart method found"
    echo -e "  Run ./install.sh to set it up"
}

# ── OpenViking status/start ───────────────────────────────────────────────────

is_ov_running() {
    if [[ -f "$OV_PID" ]]; then
        local pid; pid=$(cat "$OV_PID")
        kill -0 "$pid" 2>/dev/null && return 0
    fi
    # Also check by port
    lsof -ti:"$OV_PORT" >/dev/null 2>&1 && return 0
    return 1
}

start_openviking() {
    info "OpenViking MCP server (port $OV_PORT)"

    if is_ov_running; then
        skip "Already running (pid $(cat "$OV_PID" 2>/dev/null || echo '?'))"
        return 0
    fi

    if $STATUS_ONLY; then
        warn "Not running"
        return 1
    fi

    # Guard: venv must exist (do not rebuild — that's install.sh's job)
    if [[ ! -x "$OV_VENV/bin/python" ]]; then
        warn "OpenViking venv not found at $OV_VENV"
        echo -e "  Run ./install.sh to install OpenViking first"
        return 1
    fi

    if [[ ! -f "$OV_CONF" ]]; then
        warn "Config not found at $OV_CONF"
        echo -e "  Run ./install.sh to generate ov.conf"
        return 1
    fi

    if [[ ! -f "$REPO_DIR/mcps/openviking/server.py" ]]; then
        warn "server.py not found — is REPO_DIR correct? ($REPO_DIR)"
        return 1
    fi

    echo -e "  Starting OpenViking..."
    nohup "$OV_VENV/bin/python" "$REPO_DIR/mcps/openviking/server.py" \
        --config "$OV_CONF" \
        --data   "$OV_DATA" \
        > "$OV_LOG" 2>&1 &
    echo $! > "$OV_PID"

    # Wait briefly and verify it came up
    sleep 2
    if is_ov_running; then
        ok "Started (pid $(cat "$OV_PID"), port $OV_PORT)"
        ok "Log: $OV_LOG"
    else
        warn "Failed to start — check $OV_LOG"
        tail -10 "$OV_LOG" 2>/dev/null | sed 's/^/    /'
        return 1
    fi
}

# ── Main ──────────────────────────────────────────────────────────────────────

echo ""
if $STATUS_ONLY; then
    echo -e "${BOLD}devexp service status${RESET}"
else
    echo -e "${BOLD}Starting devexp services${RESET}"
fi
echo ""

check_jina
echo ""
start_openviking
echo ""

if ! $STATUS_ONLY; then
    echo -e "${BOLD}Done.${RESET} Reconnect MCP in your AI CLI to pick up any restarted services."
fi
