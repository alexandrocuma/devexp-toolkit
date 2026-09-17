#!/usr/bin/env bash
# Scans the devexp CLI with govulncheck (source mode) for every platform the
# release ships: each GOOS x GOARCH in .goreleaser.yaml, with its CGO_ENABLED.
# Used by .github/workflows/ci.yml (job `govulncheck`), which release.yml calls
# on a tag, and locally.
#
# Exit status:
#   0  no platform has a vulnerability in code the CLI calls
#      (uncalled findings are still printed by -show verbose)
#   3  at least one platform has a called vulnerability: fix it
#   1  anything else is an infrastructure failure, not a finding: staging,
#      installing govulncheck, reading .goreleaser.yaml, or govulncheck itself
#      failing (e.g. vuln.go.dev or the module proxy unreachable)
#
# govulncheck is installed into a temporary GOBIN that is removed on exit, so
# nothing lands on your PATH.
set -uo pipefail

# Bump here only. CI runs with GOTOOLCHAIN=local, so the new version's `go`
# directive must not be newer than the `toolchain` line in cli/go.mod.
GOVULNCHECK_VERSION=v1.8.0

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG="$ROOT/.goreleaser.yaml"

infra() {
    echo "govulncheck.sh: $* (infrastructure failure, not a finding)" >&2
    exit 1
}

# Values of a YAML block list such as `goos:` followed by `- linux` lines.
list_values() {
    awk -v key="$1" '
        $0 ~ "^[[:space:]]*" key ":[[:space:]]*$" { inlist = 1; next }
        inlist && /^[[:space:]]*-[[:space:]]*[^[:space:]]/ {
            v = $0
            sub(/^[[:space:]]*-[[:space:]]*/, "", v)
            sub(/[[:space:]]*(#.*)?$/, "", v)
            gsub(/["\047]/, "", v)
            print v
            next
        }
        { inlist = 0 }
    ' "$CONFIG" | sort -u
}

[ -f "$CONFIG" ] || infra "$CONFIG not found"
goos_list="$(list_values goos)"
goarch_list="$(list_values goarch)"
cgo="$(sed -n 's/^[[:space:]]*-[[:space:]]*["'\'']\{0,1\}CGO_ENABLED=\([01]\).*/\1/p' "$CONFIG" | sort -u)"
[ -n "$goos_list" ] || infra "no goos list in .goreleaser.yaml"
[ -n "$goarch_list" ] || infra "no goarch list in .goreleaser.yaml"
case "$cgo" in
    0 | 1) ;;
    *) infra "expected exactly one CGO_ENABLED=0|1 in .goreleaser.yaml, got '${cgo//$'\n'/ }'" ;;
esac

"$ROOT/scripts/stage-assets.sh" || infra "staging assets failed"

bindir="$(mktemp -d)" || infra "mktemp failed"
trap 'rm -rf "$bindir"' EXIT
# Build govulncheck for the host, whatever GOOS/GOARCH the caller has set.
(cd "$ROOT/cli" && env -u GOOS -u GOARCH GOBIN="$bindir" \
    go install "golang.org/x/vuln/cmd/govulncheck@$GOVULNCHECK_VERSION") ||
    infra "installing govulncheck $GOVULNCHECK_VERSION failed"

found=""
failed=""
for goos in $goos_list; do
    for goarch in $goarch_list; do
        platform="$goos/$goarch"
        echo "== govulncheck $GOVULNCHECK_VERSION: GOOS=$goos GOARCH=$goarch CGO_ENABLED=$cgo"
        (cd "$ROOT/cli" && GOOS="$goos" GOARCH="$goarch" CGO_ENABLED="$cgo" \
            "$bindir/govulncheck" -show verbose ./...)
        rc=$?
        case $rc in
            0) ;;
            3) found="$found $platform" ;;
            *) failed="$failed $platform(exit $rc)" ;;
        esac
        echo
    done
done

if [ -n "$failed" ]; then
    [ -z "$found" ] || echo "govulncheck.sh: called vulnerabilities on:$found" >&2
    infra "govulncheck did not complete on:$failed"
fi
if [ -n "$found" ]; then
    echo "govulncheck.sh: called vulnerabilities on:$found" >&2
    exit 3
fi
echo "govulncheck.sh: no called vulnerabilities on any platform"
