#!/usr/bin/env bash
# Copies the tracked, embeddable subset of repo-root assets into
# cli/internal/assets/ so they can be baked into the devexp binary via go:embed.
# Regenerated on every build — the staged copy is gitignored, not source of truth.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$ROOT/cli/internal/assets"

# Only remove the staged copies — assets.go (the //go:embed source file) lives
# in this same directory and must survive re-staging.
rm -rf "$DEST/agents" "$DEST/skills" "$DEST/hooks" "$DEST/mcps" "$DEST/devexp.config.json" "$DEST/uninstall.sh"
mkdir -p "$DEST"

for dir in agents skills hooks; do
    rsync -a --exclude='.DS_Store' "$ROOT/$dir/" "$DEST/$dir/"
done

rsync -a --exclude='.DS_Store' --exclude='node_modules' --exclude='.env' "$ROOT/mcps/" "$DEST/mcps/"

cp "$ROOT/devexp.config.json" "$DEST/devexp.config.json"
cp "$ROOT/uninstall.sh" "$DEST/uninstall.sh"
