#!/usr/bin/env bash
# Tests install.sh's HOME check (#126), without building anything.
#
# In a clone without bin/devexp, install.sh stages the assets and runs
# `go build` before `devexp install` gets a chance to refuse a bad HOME, and a
# build with HOME unset, empty or relative writes Go's caches under the clone
# or the current directory. So install.sh refuses first.
#
# Each run uses a copy of install.sh in a temp repo with no bin/, a stub
# scripts/stage-assets.sh and a stub `go` first on PATH, both logging to an
# absolute file: "nothing logged" means no build was attempted.
#
# Run: bash install.test.sh
set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

ok() { pass=$((pass+1)); }
ko() { fail=$((fail+1)); printf 'FAIL %s\n' "$1"; [ -n "${2:-}" ] && printf '%s\n' "$2" | sed 's/^/  | /'; return 0; }
check() { # $1=label, rest = test command
    local label="$1"; shift
    if "$@"; then ok; else ko "$label" "$(cat "$E/out" 2>/dev/null)"; fi
}

tree_sum() { # $1=dir -> every path, then every file's checksum
    (cd "$1" && find . -print | LC_ALL=C sort && find . -type f -exec cksum {} + | LC_ALL=C sort)
}

# run_install [VAR=value ...]: install.sh --dry-run with only these variables,
# from $E/cwd (a dotfiles-style tree, with the same tree under home/).
envs=0
run_install() {
    envs=$((envs+1))
    E="$TMP/env$envs"
    mkdir -p "$E/r/scripts" "$E/r/cli" "$E/bin"
    cp "$ROOT/install.sh" "$E/r/install.sh"
    printf '#!/bin/sh\necho "stage-assets $*" >> "%s"\n' "$E/calls" > "$E/r/scripts/stage-assets.sh"
    printf '#!/bin/sh\necho "go $*" >> "%s"\n' "$E/calls" > "$E/bin/go"
    chmod +x "$E/r/scripts/stage-assets.sh" "$E/bin/go"
    for d in "$E/cwd" "$E/cwd/home"; do
        mkdir -p "$d/.claude/agents" "$d/Library/Caches/go-build"
        printf 'mine\n' > "$d/.claude/agents/mine.md"
        printf 'cache\n' > "$d/Library/Caches/go-build/entry"
    done
    REPO_BEFORE="$(tree_sum "$E/r")"
    CWD_BEFORE="$(tree_sum "$E/cwd")"
    (cd "$E/cwd" && env -i PATH="$E/bin:/usr/bin:/bin" "$@" \
        /bin/bash "$E/r/install.sh" --dry-run </dev/null > "$E/out" 2>&1; echo $? > "$E/rc")
}

for home_case in unset empty relative; do
    case "$home_case" in
        unset)    run_install ;;
        empty)    run_install HOME= ;;
        relative) run_install HOME=home ;;
    esac
    check "HOME $home_case: exits 1" test "$(cat "$E/rc")" = 1
    check "HOME $home_case: says why" grep -qF "not an absolute path — refusing to install anything; set HOME and re-run" "$E/out"
    check "HOME $home_case: no build attempted" test ! -e "$E/calls"
    check "HOME $home_case: the repo is unchanged" test "$(tree_sum "$E/r")" = "$REPO_BEFORE"
    check "HOME $home_case: the current directory is unchanged" test "$(tree_sum "$E/cwd")" = "$CWD_BEFORE"
done

# An absolute HOME passes the check and goes on to the build (the stubs make
# it a no-op; the exec of the missing binary then fails).
run_install HOME="$TMP/home"
check "absolute HOME: no refusal" sh -c "! grep -qF 'refusing to install anything' '$E/out'"
check "absolute HOME: goes on to stage and build" grep -qF "go build" "$E/calls"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
