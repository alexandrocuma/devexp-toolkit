#!/usr/bin/env bash
# Lints the Go CLI in cli/ with golangci-lint, using .golangci.yml at the repo
# root. Used by .github/workflows/ci.yml (job `lint`) and locally.
#
# Only the Go half of the house rules lives here. The comment standard spans
# Go, bash and JS, so it is checked by scripts/check-comment-refs.sh instead;
# a Go linter could only ever enforce it in a third of the repo.
#
# Exit status:
#   0  no findings
#   1  at least one finding: fix it
#   2  infrastructure failure, not a finding: staging assets, or installing or
#      running golangci-lint
#
# golangci-lint is installed into a temporary GOBIN that is removed on exit, so
# nothing lands on your PATH.
set -uo pipefail

# Bump here only. The major must match the `version:` key in .golangci.yml —
# v2 refuses a v1 config and vice versa, with a message about the schema
# rather than the version, which is a confusing way to find out.
GOLANGCI_VERSION=v2.13.2

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

infra() {
    echo "golangci-lint.sh: $* (infrastructure failure, not a finding)" >&2
    exit 2
}

# cli/internal/assets/ is gitignored and embedded at build time; without it the
# package does not compile, and a linter that cannot load a package reports
# nothing rather than reporting a pass.
"$ROOT/scripts/stage-assets.sh" || infra "staging assets failed"

bindir="$(mktemp -d)" || infra "mktemp failed"
trap 'rm -rf "$bindir"' EXIT
(cd "$ROOT/cli" && env -u GOOS -u GOARCH GOBIN="$bindir" \
    go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$GOLANGCI_VERSION") ||
    infra "installing golangci-lint $GOLANGCI_VERSION failed"

echo "== golangci-lint $GOLANGCI_VERSION"
(cd "$ROOT/cli" && "$bindir/golangci-lint" run --config "$ROOT/.golangci.yml")
rc=$?
case $rc in
    0) echo "golangci-lint.sh: no findings" ;;
    1) ;;  # findings, already printed
    *) infra "golangci-lint exited $rc" ;;
esac
exit $rc
