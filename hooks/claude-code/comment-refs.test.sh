#!/usr/bin/env bash
# Tests the language-agnostic comment-reference scanner and the advisory hook
# that reports its findings as a file is written.
#
# Two things are being pinned. The first is that the scanner reads comments in
# a language it has never been told about beyond one table row — the same
# reference is found in Go, shell, JS, SQL and Lisp, and the same non-reference
# is left alone in all of them. That is the whole reason this is not a linter
# plugin.
#
# The second is what it deliberately does NOT report: a trailing comment after
# code (telling it from a marker inside a string needs a parser per language),
# a shebang, a quoted example inside a comment, and anything in a file whose
# extension has no entry. A blocking check that cries wolf gets switched off,
# so under-reporting is the safe direction and is asserted, not assumed.
#
# Run: bash hooks/claude-code/comment-refs.test.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

check() { # $1=name  $2=ok  $3=detail
  if [ "$2" = 0 ]; then pass=$((pass+1)); return; fi
  fail=$((fail+1)); printf 'FAIL %s%s\n' "$1" "${3:+ -- $3}"
}

# scan writes $2 into a file with extension $1 and prints the scanner's output.
scan() { # $1=ext  $2=content
  local f="$TMP/case$1"
  printf '%s\n' "$2" > "$f"
  bash "$DIR/comment-refs.sh" "$f" 2>/dev/null
}

finds() { # $1=name  $2=ext  $3=content  $4=token it must name
  local out; out="$(scan "$2" "$3")"
  case "$out" in *"$4"*) check "$1" 0 ;; *) check "$1" 1 "got: ${out:-<nothing>}" ;; esac
}

quiet() { # $1=name  $2=ext  $3=content
  local out; out="$(scan "$2" "$3")"
  [ -z "$out" ] && check "$1" 0 || check "$1" 1 "got: $out"
}

# ── the same reference, found in every language in the table ────────────────
finds 'go: issue number'     .go  '// works around the bug (#412) that ate a byte'   '#412'
finds 'shell: issue number'  .sh  '# works around the bug (#412) that ate a byte'    '#412'
finds 'js: issue number'     .js  '// works around the bug (#412) that ate a byte'   '#412'
finds 'sql: issue number'    .sql '-- works around the bug (#412) that ate a byte'   '#412'
finds 'lisp: issue number'   .el  '; works around the bug (#412) that ate a byte'    '#412'
finds 'python: URL'          .py  '# the contract is at https://example.com/docs'    'https://'
finds 'rust: tracker id'     .rs  '// see PROJ-4421 for the original report'         'PROJ-4421'

# ── block comments, which is where a JSDoc @see hides ───────────────────────
finds 'js: inside a block comment' .js '/**
 * @see https://example.com/docs
 */' 'https://'
quiet 'js: a closed block leaves the next line alone' .js '/* nothing here */
const url = "https://example.com";'

# ── what it must NOT report ─────────────────────────────────────────────────
quiet 'a shebang is not a comment'        .sh  '#!/usr/bin/env bash'
quiet 'a quoted example is data'          .go  '// Hostname(), not Host: for "http://user@:80" the host is ":80"'
quiet 'a backtick example is data'        .sh  '# the placeholder is `https://host/x` until resolveStr runs'
quiet 'a trailing comment is out of scope' .go 'x := 1 // see #412'
quiet 'a URL in code is not a comment'    .go  'const docs = "https://example.com"'
quiet 'an unknown extension is skipped'   .zzz '// see #412'
quiet 'a short number is not an issue'    .go  '// the retry budget is #9 by design'
quiet 'SHA-256 is not a tracker id'       .go  '// the digest is SHA-256 over the body'
quiet 'UTF-8 is not a tracker id'         .go  '// the body must be valid UTF-8 to survive'
quiet 'an ordinary comment'               .go  '// resolveStr substitutes ${VAR} from the merged env'

# ── exit status: the scanner blocks, the hook never does ────────────────────
printf '// see #412\n' > "$TMP/dirty.go"
bash "$DIR/comment-refs.sh" "$TMP/dirty.go" >/dev/null 2>&1
check 'the scanner exits non-zero on a finding' "$([ $? -ne 0 ] && echo 0 || echo 1)"

printf '// nothing to see\n' > "$TMP/clean.go"
bash "$DIR/comment-refs.sh" "$TMP/clean.go" >/dev/null 2>&1
check 'the scanner exits 0 on a clean file' $?

envelope() { python3 -I -c 'import json,sys; print(json.dumps({"tool_name":"Write","tool_input":{"file_path":sys.argv[1]}}))' "$1"; }

out="$(envelope "$TMP/dirty.go" | bash "$DIR/comment-refs-on-save.sh" 2>&1 >/dev/null)"
rc=$?
check 'the hook exits 0 even with findings -- it is advisory' "$rc" "exit $rc"
case "$out" in *'#412'*) check 'the hook names the finding on stderr' 0 ;;
               *) check 'the hook names the finding on stderr' 1 "got: ${out:-<nothing>}" ;; esac

out="$(envelope "$TMP/clean.go" | bash "$DIR/comment-refs-on-save.sh" 2>&1 >/dev/null)"
[ -z "$out" ] && check 'the hook is silent on a clean file' 0 || check 'the hook is silent on a clean file' 1 "got: $out"

out="$(envelope "$TMP/does-not-exist.go" | bash "$DIR/comment-refs-on-save.sh" 2>&1 >/dev/null)"
rc=$?
check 'a missing file is not the hook to report it' "$rc" "exit $rc"

# ── the repo the hook ships from holds no references of its own ─────────────
# Through the same wrapper CI uses, so the two can't disagree about scope.
cd "$DIR/../.." || exit 1
bash scripts/check-comment-refs.sh >/dev/null 2>&1
check 'devexp itself passes the rule it ships' $?

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
