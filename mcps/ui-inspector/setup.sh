#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

# Install Obscura (shared by both obscura mcp and ui-inspector)
if ! command -v obscura &>/dev/null; then
  if command -v cargo &>/dev/null; then
    echo "  Installing Obscura via cargo (this takes a minute)..."
    cargo install obscura
  else
    echo "  WARNING: 'obscura' not found and 'cargo' is not installed."
    echo "  Install Rust: https://rustup.rs/ — then re-run setup.sh"
    echo "  Or grab a pre-built binary: https://github.com/h4ckf0r0day/obscura/releases"
    echo ""
  fi
else
  echo "  obscura: $(obscura --version 2>/dev/null || echo 'installed')"
fi

echo "  Installing ui-inspector npm dependencies..."
npm install --silent

echo "  Building TypeScript..."
npm run build

echo ""
echo "  ui-inspector ready."
echo "  Run './start-services.sh' to start the Obscura CDP server before use."
