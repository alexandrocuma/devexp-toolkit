#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$REPO_DIR/bin/devexp"
[[ "${HOME:-}" == /* ]] || { echo "HOME is \"${HOME:-}\", not an absolute path — refusing to install anything; set HOME and re-run" >&2; exit 1; }
if [[ ! -x "$BIN" ]]; then
    echo ""
    echo "Building devexp CLI..."
    mkdir -p "$REPO_DIR/bin"
    "$REPO_DIR/scripts/stage-assets.sh"
    (cd "$REPO_DIR/cli" && go build -o "$BIN" .) || {
        echo "Build failed. Ensure Go is installed: https://go.dev/dl/"
        exit 1
    }
    echo "Done."
    echo ""
fi

exec "$BIN" install "$@"
