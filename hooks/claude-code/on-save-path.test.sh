#!/usr/bin/env bash
# Tests that the on-save hooks act on the exact path they were given.
#
# format-on-save, lint-on-save and test-on-save each hand the edited path to a
# project tool. A path must reach that decision unchanged: two paths that
# differ only by trailing characters are two different files, and a tool must
# never be pointed at the one that was not edited.
#
# The path must also reach the tool as the right file, and as a path (#121).
# For every tool the hooks call, the second part pins the exact argv and
# working directory for an absolute path (unchanged since v0.8.0) and for a
# leading-dash relative path, which must never be read as an option. It also
# covers a relative path from a hook cwd that is not the project root.
# hooks/opencode/on-save-path.test.js mirrors that part for opencode.
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

# ── Tool argv (#121) ─────────────────────────────────────────────────────────
# Each case is a fresh project laid out so the hook takes one tool branch, with
# that tool as a stub and a PATH that holds nothing else. Every tool gets the
# edited file as an absolute path, whatever form the input used:
#   abs   an absolute path: argv exactly as at v0.8.0 (jest aside: its pattern
#         is an exact path after
#         --runTestsByPath --, a form jest 29 and 30 both accept)
#   dash  a relative path starting with '-', hook cwd = project
#   the cases after the per-tool table cover a relative path when the hook's
#   cwd is not the project root: resolved against the input's cwd, else the
#   hook's own

BASH_BIN="$(command -v bash)"
BASE="$TMP/base"; mkdir -p "$BASE"   # the only non-stub commands a hook needs
printf '#!/bin/sh\nexec %q "$@"\n' "$(python3 -I -c 'import sys; print(sys.executable)')" > "$BASE/python3"
printf '#!/bin/sh\nexec %q "$@"\n' "$(command -v cat)" > "$BASE/cat"
chmod +x "$BASE/python3" "$BASE/cat"

# Records one line per call: the tool's name, {its working directory}, then
# each argument in [brackets].
ARGV_STUB='#!/bin/sh
{ printf "%s {%s}" "${0##*/}" "$(pwd -P)"; for a in "$@"; do printf " [%s]" "$a"; done; printf "\n"; } >> "$STUB_LOG"'

n_case=0
# setup ENTRY... — a new project $P and tool dir $BIN. An entry bin/<tool> or
# */node_modules/.bin/<tool> is a stub; any other entry is an empty file.
setup() {
  n_case=$((n_case+1))
  P="$TMP/case$n_case/project"; BIN="$TMP/case$n_case/bin"
  mkdir -p "$P" "$BIN"; P="$(cd "$P" && pwd -P)"
  touch "$P/pyproject.toml"
  local e f
  for e in "$@"; do
    case "$e" in
      bin/*) f="$BIN/${e#bin/}" ;;
      *)     f="$P/$e" ;;
    esac
    mkdir -p "$(dirname "$f")"
    case "$e" in
      bin/*|node_modules/.bin/*|*/node_modules/.bin/*) printf '%s\n' "$ARGV_STUB" > "$f"; chmod +x "$f" ;;
      *) : > "$f" ;;
    esac
  done
}

# want HOOK LABEL PATH ARGV [HOOK_CWD [INPUT_CWD]] — run HOOK on PATH from
# HOOK_CWD (default $P), with INPUT_CWD as the input's cwd if given; the tool
# calls must be ARGV.
want() {
  local got hook_cwd="${5:-$P}"
  : > "$STUB_LOG"
  python3 -I -c '
import json, sys
d = {"tool_name": "Write", "tool_input": {"file_path": sys.argv[1]}}
if len(sys.argv) > 2:
    d["cwd"] = sys.argv[2]
print(json.dumps(d))' "$3" ${6+"$6"} \
    | (cd "$hook_cwd" && PATH="$BIN:$BASE" "$BASH_BIN" "$DIR/$1.sh" >/dev/null 2>&1)
  got="$(cat "$STUB_LOG")"
  if [ "$got" = "$4" ]; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL %-16s %s\n  want: %s\n  got:  %s\n' "$1" "$2" "$4" "$got"
  fi
}

JS=(mod.js -mod.js); PY=(mod.py -mod.py); GO=(mod.go -mod.go); RB=(mod.rb -mod.rb)

# ── format-on-save
setup biome.json node_modules/.bin/biome "${JS[@]}"
want format-on-save 'local biome abs'   "$P/mod.js" "biome {$P} [format] [--write] [$P/mod.js]"
want format-on-save 'local biome dash'  -mod.js     "biome {$P} [format] [--write] [$P/-mod.js]"
setup node_modules/.bin/prettier "${JS[@]}"
want format-on-save 'local prettier abs'  "$P/mod.js" "prettier {$P} [--write] [$P/mod.js]"
want format-on-save 'local prettier dash' -mod.js     "prettier {$P} [--write] [$P/-mod.js]"
setup biome.json bin/biome "${JS[@]}"
want format-on-save 'biome abs'   "$P/mod.js" "biome {$P} [format] [--write] [$P/mod.js]"
want format-on-save 'biome dash'  -mod.js     "biome {$P} [format] [--write] [$P/-mod.js]"
setup bin/prettier "${JS[@]}"
want format-on-save 'prettier abs'  "$P/mod.js" "prettier {$P} [--write] [$P/mod.js]"
want format-on-save 'prettier dash' -mod.js     "prettier {$P} [--write] [$P/-mod.js]"
setup bin/ruff "${PY[@]}"
want format-on-save 'ruff abs'   "$P/mod.py" "ruff {$P} [format] [$P/mod.py]"
want format-on-save 'ruff dash'  -mod.py     "ruff {$P} [format] [$P/-mod.py]"
setup bin/black "${PY[@]}"
want format-on-save 'black abs'  "$P/mod.py" "black {$P} [--quiet] [$P/mod.py]"
want format-on-save 'black dash' -mod.py     "black {$P} [--quiet] [$P/-mod.py]"
setup bin/gofmt "${GO[@]}"
want format-on-save 'gofmt abs'  "$P/mod.go" "gofmt {$P} [-w] [$P/mod.go]"
want format-on-save 'gofmt dash' -mod.go     "gofmt {$P} [-w] [$P/-mod.go]"
setup bin/rubocop "${RB[@]}"
want format-on-save 'rubocop abs'  "$P/mod.rb" "rubocop {$P} [--autocorrect-all] [--no-color] [--format] [quiet] [$P/mod.rb]"
want format-on-save 'rubocop dash' -mod.rb     "rubocop {$P} [--autocorrect-all] [--no-color] [--format] [quiet] [$P/-mod.rb]"

# ── lint-on-save
setup biome.json node_modules/.bin/biome "${JS[@]}"
want lint-on-save 'local biome abs'  "$P/mod.js" "biome {$P} [lint] [$P/mod.js]"
want lint-on-save 'local biome dash' -mod.js     "biome {$P} [lint] [$P/-mod.js]"
setup node_modules/.bin/eslint "${JS[@]}"
want lint-on-save 'local eslint abs'  "$P/mod.js" "eslint {$P} [--max-warnings=0] [--no-warn-ignored] [$P/mod.js]"
want lint-on-save 'local eslint dash' -mod.js     "eslint {$P} [--max-warnings=0] [--no-warn-ignored] [$P/-mod.js]"
setup biome.json bin/biome "${JS[@]}"
want lint-on-save 'biome abs'  "$P/mod.js" "biome {$P} [lint] [$P/mod.js]"
want lint-on-save 'biome dash' -mod.js     "biome {$P} [lint] [$P/-mod.js]"
setup bin/eslint "${JS[@]}"
want lint-on-save 'eslint abs'  "$P/mod.js" "eslint {$P} [--max-warnings=0] [$P/mod.js]"
want lint-on-save 'eslint dash' -mod.js     "eslint {$P} [--max-warnings=0] [$P/-mod.js]"
setup bin/ruff "${PY[@]}"
want lint-on-save 'ruff abs'  "$P/mod.py" "ruff {$P} [check] [$P/mod.py]"
want lint-on-save 'ruff dash' -mod.py     "ruff {$P} [check] [$P/-mod.py]"
setup bin/flake8 "${PY[@]}"
want lint-on-save 'flake8 abs'  "$P/mod.py" "flake8 {$P} [$P/mod.py]"
want lint-on-save 'flake8 dash' -mod.py     "flake8 {$P} [$P/-mod.py]"
setup bin/go mod.go -pkg/mod.go
want lint-on-save 'go vet abs'  "$P/mod.go"  "go {$P} [vet] [./...]"
want lint-on-save 'go vet dash' -pkg/mod.go  "go {$P} [vet] [./-pkg]"
setup bin/rubocop "${RB[@]}"
want lint-on-save 'rubocop abs'  "$P/mod.rb" "rubocop {$P} [--no-color] [--format] [simple] [$P/mod.rb]"
want lint-on-save 'rubocop dash' -mod.rb     "rubocop {$P} [--no-color] [--format] [simple] [$P/-mod.rb]"

# ── test-on-save
JST=("${JS[@]}" mod.test.js -mod.test.js)
setup node_modules/.bin/vitest "${JST[@]}"
want test-on-save 'local vitest abs'  "$P/mod.js" "vitest {$P} [run] [$P/mod.test.js]"
want test-on-save 'local vitest dash' -mod.js     "vitest {$P} [run] [$P/-mod.test.js]"
setup node_modules/.bin/jest "${JST[@]}"
want test-on-save 'local jest abs'  "$P/mod.js" "jest {$P} [--passWithNoTests] [--no-coverage] [--runTestsByPath] [--] [mod.test.js]"
want test-on-save 'local jest dash' -mod.js     "jest {$P} [--passWithNoTests] [--no-coverage] [--runTestsByPath] [--] [-mod.test.js]"
setup bin/vitest "${JST[@]}"
want test-on-save 'vitest abs'  "$P/mod.js" "vitest {$P} [run] [$P/mod.test.js]"
want test-on-save 'vitest dash' -mod.js     "vitest {$P} [run] [$P/-mod.test.js]"
setup bin/jest "${JST[@]}"
want test-on-save 'jest abs'  "$P/mod.js" "jest {$P} [--passWithNoTests] [--no-coverage] [--runTestsByPath] [--] [mod.test.js]"
want test-on-save 'jest dash' -mod.js     "jest {$P} [--passWithNoTests] [--no-coverage] [--runTestsByPath] [--] [-mod.test.js]"
setup bin/go mod.go -pkg/mod.go
want test-on-save 'go test abs'  "$P/mod.go"  "go {$P} [test] [-timeout] [20s] [./...]"
want test-on-save 'go test dash' -pkg/mod.go  "go {$P} [test] [-timeout] [20s] [./-pkg]"
setup bin/pytest "${PY[@]}" test_mod.py test_-mod.py
want test-on-save 'pytest abs'  "$P/mod.py" "pytest {$P} [$P/test_mod.py] [-x] [-q]"
want test-on-save 'pytest dash' -mod.py     "pytest {$P} [$P/test_-mod.py] [-x] [-q]"
setup bin/rspec "${RB[@]}" mod_spec.rb -mod_spec.rb
want test-on-save 'rspec abs'  "$P/mod.rb" "rspec {$P} [$P/mod_spec.rb] [--format] [progress]"
want test-on-save 'rspec dash' -mod.rb     "rspec {$P} [$P/-mod_spec.rb] [--format] [progress]"

# ── Relative path, hook cwd is not the project root
# nested: the hook runs from the session directory $P, and the file sits in a
#   nested project $P/sub, so the tool runs from $P/sub. A relative path handed
#   on as given would name $P/sub/sub/... there.
# input cwd: the hook runs from elsewhere ($OUT); the input's cwd ($P) is the
#   directory the path is relative to, and wins over the hook's own cwd.
OUT="$TMP/elsewhere"; mkdir -p "$OUT"; OUT="$(cd "$OUT" && pwd -P)"

setup bin/ruff sub/pyproject.toml sub/-mod.py
want format-on-save 'nested ruff'     sub/-mod.py "ruff {$P/sub} [format] [$P/sub/-mod.py]"
want lint-on-save   'nested ruff'     sub/-mod.py "ruff {$P/sub} [check] [$P/sub/-mod.py]"
want format-on-save 'no input cwd: hook cwd' -mod.py "" "$OUT"   # $OUT/-mod.py does not exist
want format-on-save 'input cwd ruff'  sub/-mod.py "ruff {$P/sub} [format] [$P/sub/-mod.py]" "$OUT" "$P"
want lint-on-save   'input cwd ruff'  sub/-mod.py "ruff {$P/sub} [check] [$P/sub/-mod.py]"  "$OUT" "$P"
setup sub/package.json sub/node_modules/.bin/jest sub/-mod.js sub/-mod.test.js
want test-on-save 'nested jest'    sub/-mod.js "jest {$P/sub} [--passWithNoTests] [--no-coverage] [--runTestsByPath] [--] [-mod.test.js]"
want test-on-save 'input cwd jest' sub/-mod.js "jest {$P/sub} [--passWithNoTests] [--no-coverage] [--runTestsByPath] [--] [-mod.test.js]" "$OUT" "$P"
setup bin/rspec sub/pyproject.toml sub/lib/-mod.rb sub/spec/-mod_spec.rb
want test-on-save 'input cwd rspec' sub/lib/-mod.rb "rspec {$P/sub} [$P/sub/spec/-mod_spec.rb] [--format] [progress]" "$OUT" "$P"

# ── Relative path with '.' / '..' segments: normalised, same argv as opencode
# (a/ exists, so an unnormalised path would still reach the tool, as a/../-mod.py)
setup bin/ruff "${PY[@]}" a/keep test_-mod.py bin/pytest
want format-on-save 'normalised ./'  ./-mod.py     "ruff {$P} [format] [$P/-mod.py]"
want format-on-save 'normalised ..'  a/../-mod.py  "ruff {$P} [format] [$P/-mod.py]"
want lint-on-save   'normalised ./'  ./-mod.py     "ruff {$P} [check] [$P/-mod.py]"
want lint-on-save   'normalised ..'  a/../-mod.py  "ruff {$P} [check] [$P/-mod.py]"
want test-on-save   'normalised ..'  a/../-mod.py  "pytest {$P} [$P/test_-mod.py] [-x] [-q]"

# ── jest runs the exact test file: names with regex metacharacters
setup node_modules/.bin/jest a+b.js a+b.test.js ab.test.js 'c(1).js' 'c(1).test.js'
want test-on-save 'jest a+b'  "$P/a+b.js"  "jest {$P} [--passWithNoTests] [--no-coverage] [--runTestsByPath] [--] [a+b.test.js]"
want test-on-save 'jest c(1)' "$P/c(1).js" "jest {$P} [--passWithNoTests] [--no-coverage] [--runTestsByPath] [--] [c(1).test.js]"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
