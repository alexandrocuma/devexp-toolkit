#!/usr/bin/env bash
# start-services.sh — Start devexp MCP background services.
#
# ui-inspector now manages its own headless Chromium process (launched on demand,
# shut down on SIGTERM) so there are no daemons to start here.
#
# Usage:
#   ./start-services.sh            # no-op, prints status
#   ./start-services.sh --status   # same
set -euo pipefail

BOLD='\033[1m'
RESET='\033[0m'

echo ""
echo -e "${BOLD}devexp services${RESET}"
echo ""
echo "  No background services required."
echo "  ui-inspector manages its own browser process."
echo ""
