#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

echo "  Installing ui-inspector npm dependencies..."
npm install --silent

echo "  Installing Playwright Chromium browser..."
npx playwright install chromium

echo "  Building TypeScript..."
npm run build

echo ""
echo "  ui-inspector ready. No background services required."
