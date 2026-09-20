#!/usr/bin/env bash
# Tests scripts/remote-install.sh's install-directory check, offline.
#
# The binary goes to DEVEXP_INSTALL_DIR, else $HOME/.local/bin. With HOME
# unset, empty or relative that default is /.local/bin or a path under the
# current directory, so the script refuses before any download; a relative
# DEVEXP_INSTALL_DIR is refused too, and an absolute one works without HOME.
#
# Every run has a stub `curl` first on PATH that logs its call and fails, so
# nothing touches the network: "curl was called" is how a test sees that the
# check passed, and "curl was not called" that it refused before downloading.
#
# Run: bash remote-install.test.sh
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

# run_remote [VAR=value ...]: the script with only these variables, from $E/cwd,
# which holds a dotfiles-style tree (.local/bin and home/.local/bin).
envs=0
run_remote() {
    envs=$((envs+1))
    E="$TMP/env$envs"
    mkdir -p "$E/bin" "$E/cwd/.local/bin" "$E/cwd/home/.local/bin"
    printf 'old devexp\n' > "$E/cwd/.local/bin/devexp"
    printf 'old devexp\n' > "$E/cwd/home/.local/bin/devexp"
    printf '#!/bin/sh\necho "curl $*" >> "$CALLS"\nexit 22\n' > "$E/bin/curl"
    chmod +x "$E/bin/curl"
    : > "$E/calls"
    BEFORE="$(tree_sum "$E/cwd")"
    (cd "$E/cwd" && env -i PATH="$E/bin:/usr/bin:/bin" CALLS="$E/calls" "$@" \
        /bin/bash "$ROOT/scripts/remote-install.sh" </dev/null > "$E/out" 2>&1; echo $? > "$E/rc")
}

rc_nonzero()     { [ "$(cat "$E/rc")" != 0 ]; }
out_has()        { grep -qF -- "$1" "$E/out"; }
out_lacks()      { ! grep -qF -- "$1" "$E/out"; }
curl_called()    { [ -s "$E/calls" ]; }
curl_not_called() { [ ! -s "$E/calls" ]; }
cwd_unchanged()  { [ "$(tree_sum "$E/cwd")" = "$BEFORE" ]; }

# ── HOME unset, empty or relative, no DEVEXP_INSTALL_DIR: refused ────────────
for home_case in unset empty relative; do
    case "$home_case" in
        unset)    run_remote ;;
        empty)    run_remote HOME= ;;
        relative) run_remote HOME=home ;;
    esac
    check "HOME $home_case: exits non-zero" rc_nonzero
    check "HOME $home_case: says why" out_has "not an absolute path — refusing to install; set HOME (or an absolute DEVEXP_INSTALL_DIR) and re-run"
    check "HOME $home_case: refuses before any download" curl_not_called
    check "HOME $home_case: the current directory is unchanged" cwd_unchanged
done

# An empty DEVEXP_INSTALL_DIR counts as unset, so HOME is still checked.
run_remote HOME= DEVEXP_INSTALL_DIR=
check "empty DEVEXP_INSTALL_DIR, empty HOME: refused before any download" curl_not_called
check "empty DEVEXP_INSTALL_DIR, empty HOME: says why" out_has "HOME is \"\", not an absolute path"

# ── A relative DEVEXP_INSTALL_DIR: refused, even with a good HOME ────────────
run_remote HOME="$TMP/home" DEVEXP_INSTALL_DIR=.local/bin
check "relative DEVEXP_INSTALL_DIR: exits non-zero" rc_nonzero
check "relative DEVEXP_INSTALL_DIR: says why" out_has "DEVEXP_INSTALL_DIR is \".local/bin\", not an absolute path — refusing to install"
check "relative DEVEXP_INSTALL_DIR: refuses before any download" curl_not_called
check "relative DEVEXP_INSTALL_DIR: the current directory is unchanged" cwd_unchanged

# ── Accepted: the check passes and the script goes on to download ────────────
run_remote DEVEXP_INSTALL_DIR="$TMP/abs-bin"
check "absolute DEVEXP_INSTALL_DIR, HOME unset: no refusal" out_lacks "refusing to install"
check "absolute DEVEXP_INSTALL_DIR, HOME unset: goes on to download" curl_called

run_remote HOME="$TMP/home"
check "absolute HOME, no DEVEXP_INSTALL_DIR: no refusal" out_lacks "refusing to install"
check "absolute HOME, no DEVEXP_INSTALL_DIR: goes on to download" curl_called

run_remote HOME=home DEVEXP_VERSION=v0.0.0 DEVEXP_INSTALL_DIR="$TMP/abs-bin"
check "absolute DEVEXP_INSTALL_DIR, relative HOME: goes on to download" curl_called
check "absolute DEVEXP_INSTALL_DIR, relative HOME: the failed download is reported" out_has "failed to download"

# ── The default path: no DEVEXP_VERSION, so the latest release is looked up ──
#
# Every case above pins DEVEXP_VERSION, so none of them enters this branch --
# and the release guide's post-release check pins it too. The documented
# one-liner is the one path nothing exercised, and it was broken: the lookup
# piped curl into `grep -m1`, which closes the pipe at the first match while
# curl is still writing, so curl died of SIGPIPE and `pipefail` aborted the
# script before it printed anything useful.
#
# The stub below reproduces that deterministically. "tag_name" is on the first
# line and ~2MB of filler follows it, so a matcher that stops early is
# guaranteed to close the pipe mid-write -- with a short body curl can finish
# before the matcher exits, and the bug hides.
run_remote_lookup() {
    envs=$((envs+1))
    E="$TMP/env$envs"
    mkdir -p "$E/bin" "$E/cwd"
    cat > "$E/bin/curl" <<'STUB'
#!/bin/sh
echo "curl $*" >> "$CALLS"
case "$*" in
  *releases/latest*)
    echo '{"tag_name": "v9.9.9", "name": "v9.9.9",'
    # filler: enough to outlast any pipe buffer
    i=0
    while [ $i -lt 40000 ]; do
      echo '  "note": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",'
      i=$((i+1))
    done
    echo '  "end": true}'
    exit 0 ;;
  *) exit 22 ;;
esac
STUB
    chmod +x "$E/bin/curl"
    : > "$E/calls"
    (cd "$E/cwd" && env -i PATH="$E/bin:/usr/bin:/bin" CALLS="$E/calls" "$@" \
        /bin/bash "$ROOT/scripts/remote-install.sh" </dev/null > "$E/out" 2>&1; echo $? > "$E/rc")
}

run_remote_lookup HOME="$TMP/home" DEVEXP_INSTALL_DIR="$TMP/lookup-bin"
check "no DEVEXP_VERSION: the lookup survives a large response" out_lacks "Failure writing output"
check "no DEVEXP_VERSION: the tag is extracted from the response" out_has "Installing devexp v9.9.9"
# It then fails at the download, because the stub only answers the API call.
# That is the point: the run has to get *past* the lookup to reach it.
check "no DEVEXP_VERSION: it proceeds to the download" out_has "failed to download"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
