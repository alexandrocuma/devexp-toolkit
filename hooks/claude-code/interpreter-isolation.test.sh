#!/usr/bin/env bash
# Tests that every Python-based hook ignores modules in the working directory.
#
# Hooks run with the project as their working directory, and a project can
# contain any file. The hook's interpreter must only ever import its own
# standard library, so whatever a project ships next to its code never runs
# as part of a hook.
#
# For each hook: a working directory holding a same-named stand-in for every
# top-level standard-library module, each of which records itself in a
# sentinel file on import. The sentinel must never appear, and the hook must
# decide exactly as it does from a clean working directory.
#
# Run: bash hooks/claude-code/interpreter-isolation.test.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
export ISO_SENTINEL="$TMP/sentinel"
pass=0; fail=0

CLEAN="$TMP/clean"; CROWDED="$TMP/crowded"
mkdir -p "$CLEAN" "$CROWDED"
for d in "$CLEAN" "$CROWDED"; do seq 1 600 > "$d/big.txt"; done

# One stand-in per importable top-level stdlib module. Stand-ins reach the
# sentinel through builtins only, so they work whatever else is shadowed.
( cd "$TMP" && python3 -I - "$CROWDED" <<'PY'
import os, pkgutil, sys, sysconfig
dest = sys.argv[1]
roots = {sysconfig.get_paths()["stdlib"], sysconfig.get_paths()["platstdlib"]}
roots |= {os.path.join(r, "lib-dynload") for r in list(roots)}
body = ("open(__import__('posix').environ[b'ISO_SENTINEL'].decode(), 'a')"
        ".write(__name__ + '\\n')\n")
names = {m.name for m in pkgutil.iter_modules(sorted(r for r in roots if os.path.isdir(r)))}
for name in names:
    with open(os.path.join(dest, name + ".py"), "w") as f:
        f.write(body)
if len(names) < 50:
    sys.exit("found only %d stdlib modules to stand in for" % len(names))
PY
) || { echo "FAIL could not build the crowded working directory"; exit 1; }

run() { # $1=working dir  $2=hook  $3=envelope -> prints "rc|stdout"
  local out rc
  out=$(cd "$1" && printf '%s' "$3" | bash "$DIR/$2.sh" 2>/dev/null); rc=$?
  printf '%s|%s' "$rc" "$out"
}

check() { # $1=hook  $2=envelope
  local hook="$1" env="$2" want got
  rm -f "$ISO_SENTINEL"
  want=$(run "$CLEAN" "$hook" "$env")
  got=$(run "$CROWDED" "$hook" "$env")
  if [ -e "$ISO_SENTINEL" ]; then
    fail=$((fail+1)); printf 'FAIL %-26s loaded %s module(s) from the working directory\n' \
      "$hook" "$(sort -u "$ISO_SENTINEL" | wc -l | tr -d ' ')"
  elif [ "$got" != "$want" ]; then
    fail=$((fail+1)); printf 'FAIL %-26s decided differently from a clean working directory\n' "$hook"
  else
    pass=$((pass+1))
  fi
}

WRITE='{"tool_name":"Write","tool_input":{"file_path":"big.txt","content":"hello"}}'

check dangerous-cmd-guard       '{"tool_name":"Bash","tool_input":{"command":"ls -la"}}'
check secret-guard              '{"tool_name":"Bash","tool_input":{"command":"cat README.md"}}'
check secret-guard              '{"tool_name":"Read","tool_input":{"file_path":"big.txt"}}'
check secret-in-write-guard     "$WRITE"
check large-file-guard          "$WRITE"
check format-on-save            "$WRITE"
check lint-on-save              "$WRITE"
check test-on-save              "$WRITE"
check graphify-read-guard       '{"tool_name":"Read","tool_input":{"file_path":"src/app.py"}}'
check graphify-session-sentinel '{"tool_name":"Bash","tool_input":{"command":"graphify query \"x\""}}'
check graphify-grep-nudge       '{"tool_name":"Grep","tool_input":{"pattern":"x"}}'

# Every file here that starts a Python interpreter must be covered above --
# every file, not just the ones hooks/registry.json registers, because the next
# shared helper that runs python3 is exactly what this is for.
#
# HELPERS are the exceptions: sourced by the guards, with no envelope of their
# own, so they are covered transitively -- every check above runs scan-budget.sh
# from the crowded working directory too, and a module it picked up there would
# show in the sentinel. Adding one is deliberate, and it must exist.
HELPERS="scan-budget"

for name in $HELPERS; do
  [ -f "$DIR/$name.sh" ] || { fail=$((fail+1)); printf 'FAIL %-26s is allowlisted but does not exist\n' "$name"; }
done

for f in "$DIR"/*.sh; do
  case "$f" in *.test.sh) continue ;; esac
  name=$(basename "$f" .sh)
  case " $HELPERS " in *" $name "*) continue ;; esac
  grep -q python3 "$f" || continue
  if ! grep -q "^check $name " "$0"; then
    fail=$((fail+1)); printf 'FAIL %-26s runs python3 but is not covered by this test\n' "$name"
  fi
done

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
