#!/usr/bin/env bash
# Downloads and installs the devexp CLI from GitHub Releases — no git clone,
# no local Go toolchain required. The binary bundles all agents, skills,
# hooks, and the MCP registry, so `devexp install` works standalone.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/alexandrocuma/devexp-toolkit/main/scripts/remote-install.sh | bash
#
# Env overrides:
#   DEVEXP_VERSION   install a specific tag (e.g. v1.2.3) instead of latest
#   DEVEXP_INSTALL_DIR  directory to place the binary in (default: ~/.local/bin)
#   DEVEXP_SKIP_RUN  set to skip running `devexp install` after download
set -euo pipefail

REPO="alexandrocuma/devexp-toolkit"
INSTALL_DIR="${DEVEXP_INSTALL_DIR:-$HOME/.local/bin}"

say() { echo "[devexp] $*"; }
die() { echo "[devexp] error: $*" >&2; exit 1; }

# ── Detect platform ───────────────────────────────────────────────────────────

os="$(uname -s)"
case "$os" in
    Darwin) os="darwin" ;;
    Linux)  os="linux" ;;
    *) die "unsupported OS: $os (devexp ships darwin/linux binaries — see https://github.com/$REPO/releases)" ;;
esac

arch="$(uname -m)"
case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) die "unsupported architecture: $arch" ;;
esac

# ── Resolve version ───────────────────────────────────────────────────────────

version="${DEVEXP_VERSION:-}"
if [[ -z "$version" ]]; then
    say "Looking up latest release..."
    version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/')"
    [[ -n "$version" ]] || die "could not determine latest release — set DEVEXP_VERSION to install a specific tag"
fi
say "Installing devexp $version ($os/$arch)..."

# ── Download and extract ──────────────────────────────────────────────────────

asset="devexp-toolkit_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$version/$asset"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL "$url" -o "$tmpdir/$asset" || die "failed to download $url — check that $version has a release for $os/$arch"
tar -xzf "$tmpdir/$asset" -C "$tmpdir"

[[ -x "$tmpdir/devexp" ]] || die "extracted archive did not contain a devexp binary"

mkdir -p "$INSTALL_DIR"
mv "$tmpdir/devexp" "$INSTALL_DIR/devexp"
chmod +x "$INSTALL_DIR/devexp"
say "Installed to $INSTALL_DIR/devexp"

case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) say "Note: $INSTALL_DIR is not on your PATH — add it to your shell profile, e.g.:"
       echo "         export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac

echo
"$INSTALL_DIR/devexp" --version

if [[ -n "${DEVEXP_SKIP_RUN:-}" ]]; then
    say "Run \"devexp install\" to set up agents, skills, hooks, and MCPs."
else
    echo
    "$INSTALL_DIR/devexp" install
fi
