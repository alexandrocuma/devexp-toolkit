#!/usr/bin/env bash
# Tests that the on-save hooks act on the exact path they were given.
#
# format-on-save, lint-on-save and test-on-save each hand the edited path to a
# project tool. A path must reach that decision unchanged: two paths that
# differ only by trailing characters are two different files, and a tool must
# never be pointed at the one that was not edited.
#
# The tools are stubs on PATH that only record how they were called.
#
# Run: bash hooks/claude-code/on-save-path.test.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

STUBS="$TMP/bin"; PROJECT="$TMP/project"
mkdir -p "$STUBS" "$PROJECT"
touch "$PROJECT/pyproject.toml"
export STUB_LOG="$TMP/calls.log"
for tool in ruff pytest; do
  printf '#!/bin/sh\nprintf "%%s\\n" "$0 $*" >> "$STUB_LOG"\n' > "$STUBS/$tool"
  chmod +x "$STUBS/$tool"
done
export PATH="$STUBS:$PATH"

# Feed the hook a PostToolUse Write envelope for $2; print how many tool calls it made.
calls() { # $1=hook  $2=path
  : > "$STUB_LOG"
  python3 -I -c 'import json,sys; print(json.dumps({"tool_name":"Write","tool_input":{"file_path":sys.argv[1]}}))' "$2" \
    | (cd "$PROJECT" && bash "$DIR/$1.sh" >/dev/null 2>&1)
  wc -l < "$STUB_LOG" | tr -d ' '
}

expect() { # $1=hook  $2=label  $3=path  $4=called|not-called
  local n; n=$(calls "$1" "$3")
  if { [ "$4" = called ] && [ "$n" -gt 0 ]; } || { [ "$4" = not-called ] && [ "$n" = 0 ]; }; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL %-16s %s: %s tool call(s), want %s\n' "$1" "$2" "$n" "$4"
  fi
}

SRC="$PROJECT/module.py"
printf 'x = 1\n' > "$SRC"
printf 'def test_x():\n    pass\n' > "$PROJECT/test_module.py"

for hook in format-on-save lint-on-save test-on-save; do
  # Control: the plain path does reach the tool, so "not-called" below is meaningful.
  expect "$hook" 'plain path'                         "$SRC"          called

  # A sibling whose name differs only by trailing characters is a different file.
  for suffix in $'\n' $'\n\n' $' \n'; do
    printf 'x = 1\n' > "$SRC$suffix"
    expect "$hook" 'path with trailing whitespace/newline' "$SRC$suffix" not-called
    rm -f "$SRC$suffix"
  done
done

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
