#!/usr/bin/env bash
# start-services.sh — Start devexp MCP services without reinstalling or wiping data.
#
# Safe to run at any time:
#   - Skips already-running services
#   - Never rebuilds venvs
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

DEVEXP_DIR="$HOME/.devexp"
OBSCURA_PORT=9222
OBSCURA_PID_FILE="$DEVEXP_DIR/obscura.pid"
OBSCURA_LOG="$DEVEXP_DIR/obscura.log"

mkdir -p "$DEVEXP_DIR"

STATUS_ONLY=false
[[ "${1:-}" == "--status" ]] && STATUS_ONLY=true

info()    { echo -e "${BOLD}==>${RESET} $*"; }
ok()      { echo -e "  ${GREEN}✓${RESET} $*"; }
skip()    { echo -e "  ${YELLOW}~${RESET} $*"; }
warn()    { echo -e "  ${RED}!${RESET} $*"; }

# ── Obscura CDP server ────────────────────────────────────────────────────────

is_obscura_running() {
    if [[ -f "$OBSCURA_PID_FILE" ]]; then
        local pid; pid=$(cat "$OBSCURA_PID_FILE")
        kill -0 "$pid" 2>/dev/null && return 0
    fi
    lsof -ti:"$OBSCURA_PORT" >/dev/null 2>&1 && return 0
    return 1
}

start_obscura() {
    info "Obscura CDP server (port $OBSCURA_PORT)"

    if ! command -v obscura &>/dev/null; then
        skip "obscura not installed — skipping (run: cargo install obscura)"
        return 0
    fi

    if is_obscura_running; then
        skip "Already running (pid $(cat "$OBSCURA_PID_FILE" 2>/dev/null || echo '?'))"
        return 0
    fi

    if $STATUS_ONLY; then
        warn "Not running"
        return 1
    fi

    echo -e "  Starting Obscura..."
    nohup obscura serve --port "$OBSCURA_PORT" > "$OBSCURA_LOG" 2>&1 &
    echo $! > "$OBSCURA_PID_FILE"

    sleep 1
    if is_obscura_running; then
        ok "Started (pid $(cat "$OBSCURA_PID_FILE"), port $OBSCURA_PORT)"
    else
        warn "Failed to start — check $OBSCURA_LOG"
        tail -5 "$OBSCURA_LOG" 2>/dev/null | sed 's/^/    /'
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

start_obscura
echo ""

if ! $STATUS_ONLY; then
    echo -e "${BOLD}Done.${RESET} Reconnect MCP in your AI CLI to pick up any restarted services."
fi
