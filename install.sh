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
        toolchain="$(sed -n 's/^toolchain //p' "$REPO_DIR/cli/go.mod" 2>/dev/null || true)"
        echo "Build failed. Ensure Go is installed: https://go.dev/dl/" >&2
        echo "If your Go is older than ${toolchain:-the toolchain line in cli/go.mod}, the first build downloads that toolchain, which needs network access. Offline: install ${toolchain:-that version} or newer, or set GOTOOLCHAIN=local to build with the Go you have (at least the go line in cli/go.mod)." >&2
        exit 1
    }
    echo "Done."
    echo ""
fi

exec "$BIN" install "$@"
