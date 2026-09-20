#!/usr/bin/env bash
# Tests that a fail-closed guard allows only against proof that its own
# scanning code ran.
#
# The guards do their parsing and matching in a program they run through an
# interpreter found on PATH, and their pattern checks through `grep`. The shell
# can read only those programs' exit statuses. So anything on PATH that reports
# success without doing the work — a wrapper, a shim, a broken virtualenv, a
# stand-in that does nothing — used to read as "the scan found nothing", and
# the tool call went through unscanned. The scan budget widened it: the
# watchdog runs through the same interpreter, so a stand-in answered before the
# guard had read the envelope at all.
#
# What has to hold, for every guard the registry marks fail_closed:
#
#   - an interpreter that reports success without running the guard's program
#     blocks, however it reports it — silently, with plausible output, by
#     echoing its arguments or its input, or by running something else;
#   - an interpreter honest at one level and not the other blocks too, so
#     neither the watchdog nor the guard's own scan can vouch for the other;
#   - a marker that is not this invocation's own is no proof: neither a guess
#     at its shape, nor a real one captured from an earlier invocation;
#   - `grep`, whose status is the only thing dangerous-cmd-guard's `matches`
#     reads, is certified in the mode `matches` actually uses — `-q`, `-e`, a
#     here-string, `-E` and `-i` — because a grep honest everywhere else and
#     blind under `-q` would otherwise report every command clean;
#   - the proof descriptor reaches the watchdog and nobody else, so a process
#     the guarded run leaves behind cannot hold the guard open past its budget;
#   - a caller with no stdout still gets a decision, not a wedged guard;
#   - the healthy paths are untouched: ordinary input is allowed in silence and
#     a dangerous one is blocked with the guard's own reason.
#
# Run: bash hooks/claude-code/interpreter-proof.test.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
cleanup() {
    # Anything the timing case left running is swept here, by pid, before the
    # temporary tree goes.
    if [ -s "$TMP/sweep" ]; then
        while read -r pid; do [ -n "$pid" ] && kill "$pid" 2>/dev/null; done < "$TMP/sweep"
    fi
    rm -rf "$TMP"
}
trap cleanup EXIT
pass=0; fail=0

REAL_PYTHON="$(command -v python3)"
REAL_GREP="$(command -v grep)"

check() { # $1=label  $2=condition-result(0/1)  $3=detail
    if [ "$2" = 0 ]; then
        pass=$((pass+1))
    else
        fail=$((fail+1)); printf 'FAIL %s: %s\n' "$1" "$3"
    fi
}

# The guards under test are the ones the registry marks fail_closed, so a new
# one is covered the day it is registered rather than the day someone adds it
# here. Its envelopes still have to exist, and the check below says so by name.
GUARDS=$(python3 -I -c '
import json, sys
print(" ".join(h["name"] for h in json.load(open(sys.argv[1]))
                if h.get("opencode", {}).get("fail_closed")))' "$DIR/../registry.json")

# An envelope each guard lets through, and one each guard blocks on its own
# merits. The secret-shaped one is assembled from pieces, so this file holds
# nothing the installed guard would refuse to write.
allow_envelope() { # $1=guard
    case "$1" in
        secret-guard)          printf '{"tool_name":"Bash","tool_input":{"command":"cat README.md"}}' ;;
        dangerous-cmd-guard)   printf '{"tool_name":"Bash","tool_input":{"command":"ls -la"}}' ;;
        secret-in-write-guard) printf '{"tool_name":"Write","tool_input":{"file_path":"a.txt","content":"hello world"}}' ;;
    esac
}
block_envelope() { # $1=guard
    case "$1" in
        secret-guard)          printf '{"tool_name":"Read","tool_input":{"file_path":".env"}}' ;;
        dangerous-cmd-guard)   printf '{"tool_name":"Bash","tool_input":{"command":"git push --force"}}' ;;
        secret-in-write-guard) printf '{"tool_name":"Write","tool_input":{"file_path":"a.txt","content":"%s"}}' \
                                   "AKIA$(printf 'Q%.0s' 1 2 3 4 5 6 7 8)$(printf '7%.0s' 1 2 3 4 5 6 7 8)" ;;
    esac
}

check "the registry names at least one fail-closed guard" \
    "$([ -n "$GUARDS" ] && echo 0 || echo 1)" "the registry listed none"
for guard in $GUARDS; do
    check "$guard has both envelopes in this file" \
        "$([ -n "$(allow_envelope "$guard")" ] && [ -n "$(block_envelope "$guard")" ] && echo 0 || echo 1)" \
        "add an allow_envelope and a block_envelope case for $guard"
done

stub() { # $1=name  $2=program it stands in for  $3=body
    mkdir -p "$TMP/$1"
    printf '#!/bin/sh\n%s\n' "$3" > "$TMP/$1/$2"
    chmod +x "$TMP/$1/$2"
}

run() { # $1=stub dir  $2=guard  $3=envelope -> prints "rc|stdout|stderr"
    local out rc err="$TMP/err"
    out=$(printf '%s' "$3" | PATH="$TMP/$1:$PATH" bash "$DIR/$2.sh" 2>"$err"); rc=$?
    printf '%s|%s|%s' "$rc" "$out" "$(cat "$err")"
}

# A guard that cannot trust its scan must block and say why. Whatever the
# stand-in itself prints is its own noise, not the guard's — the guard's own
# silence on an allow is checked further down, on the healthy path.
blocks() { # $1=stub  $2=guard  $3=envelope  $4=label
    local got rc out err ok=1
    got=$(run "$1" "$2" "$3")
    rc=${got%%|*}; out=${got#*|}; err=${out#*|}; out=${out%%|*}
    if [ "$rc" = 2 ]; then
        case "$err" in *"[devexp $2]"*"Blocking to be safe"*) ok=0 ;; esac
    fi
    check "$4: $2 blocks" "$ok" "rc=$rc stdout=$(printf '%.40s' "$out") stderr=$(printf '%.110s' "$err")"
}

# ── An interpreter that answers without running the guard's program ─────────
# Each of these exits 0, which without proof of work would be read as "the
# scan found nothing". None of them runs a line of the guard's code.
stub silent      python3 'exit 0'
stub chatty      python3 'echo
echo "ok"
exit 0'
stub echo-argv   python3 'echo "$@"
exit 0'
stub echo-stdin  python3 'cat
exit 0'
stub other       python3 "exec $REAL_PYTHON -c 'import sys; sys.exit(0)'"
# Honest for the watchdog (which passes -S) and a stand-in for the guard's own
# scan: proof at one level must not vouch for the other.
stub half        python3 "for a in \"\$@\"; do
  [ \"\$a\" = -S ] && exec $REAL_PYTHON \"\$@\"
done
exit 0"
# A marker that is not this invocation's own is no proof — neither a guess at
# the text, nor one built to the right shape, nor the arguments it was handed,
# on either channel the guards read.
stub guess       python3 'echo "devexp-scanned"
echo "devexp-scanned-1-234567"
echo "$@"
{ echo "devexp-scanned-1-234567"; echo "devexp-budget-1-234567"; echo "$@"; } >&9 2>/dev/null
exit 0'

for stub_name in silent chatty echo-argv echo-stdin other half guess; do
    for guard in $GUARDS; do
        blocks "$stub_name" "$guard" "$(block_envelope "$guard")" "an interpreter that $stub_name"
        # The allow envelope matters more than the block one: this is the call
        # that would have gone through unscanned.
        blocks "$stub_name" "$guard" "$(allow_envelope "$guard")" "an interpreter that $stub_name (ordinary input)"
    done
done

# The guard adds nothing of its own to stdout while refusing: the block is
# said on stderr, where Claude Code reads a guard's reason.
for guard in $GUARDS; do
    got=$(run silent "$guard" "$(allow_envelope "$guard")")
    rc=${got%%|*}; out=${got#*|}; out=${out%%|*}
    check "$guard says nothing on stdout when it cannot trust its scan" \
        "$([ "$rc" = 2 ] && [ -z "$out" ] && echo 0 || echo 1)" "rc=$rc stdout=$(printf '%.60s' "$out")"
done

# An interpreter that fails outright was already covered by the 0-or-2 rule;
# it stays covered.
stub quiet-fail python3 'exit 1'
stub says-block python3 'exit 2'
for stub_name in quiet-fail says-block; do
    for guard in $GUARDS; do
        got=$(run "$stub_name" "$guard" "$(allow_envelope "$guard")")
        check "an interpreter that $stub_name: $guard blocks" \
            "$([ "${got%%|*}" = 2 ] && echo 0 || echo 1)" "rc=${got%%|*} $(printf '%.90s' "${got##*|}")"
    done
done

# ── A token is this invocation's own, or it is nothing ──────────────────────
# "Minted per invocation" is what stops a stand-in carrying one in advance, so
# it is pinned twice over: the token differs between invocations, and a real
# one captured from an earlier invocation is refused by the next.
mint() { bash -c '. "$1"; printf "%s" "$DEVEXP_SCAN_PROOF"' _ "$DIR/scan-budget.sh"; }
first=$(mint); second=$(mint)
check "the scan token differs between invocations" \
    "$([ -n "$first" ] && [ "$first" != "$second" ] && echo 0 || echo 1)" "got \"$first\" twice"

# Real tokens, captured from a healthy run: this stand-in picks the tokens out
# of the arguments it is handed and then runs the interpreter it stands in for,
# so the run itself is an ordinary one. It matches by prefix and writes each to
# its own file — the arguments also carry the guards' program text, which is
# not all printable, and searching that with grep reports a binary match rather
# than the token.
mkdir -p "$TMP/capture"
cat > "$TMP/capture/python3" <<EOF
#!/bin/sh
for a in "\$@"; do
  case "\$a" in
    devexp-budget-*)  printf '%s' "\$a" > "$TMP/seen-budget" ;;
    devexp-scanned-*) printf '%s' "\$a" > "$TMP/seen-scan" ;;
  esac
done
exec $REAL_PYTHON "\$@"
EOF
chmod +x "$TMP/capture/python3"
rm -f "$TMP/seen-budget" "$TMP/seen-scan"
got=$(run capture dangerous-cmd-guard "$(allow_envelope dangerous-cmd-guard)")
old_nonce=$(cat "$TMP/seen-budget" 2>/dev/null)
old_token=$(cat "$TMP/seen-scan" 2>/dev/null)
check "a healthy run through the recording stand-in still allows" \
    "$([ "${got%%|*}" = 0 ] && echo 0 || echo 1)" "rc=${got%%|*} $(printf '%.80s' "${got##*|}")"
ok=1
case "$old_nonce" in devexp-budget-?*) case "$old_token" in devexp-scanned-?*) ok=0 ;; esac ;; esac
check "both tokens were observable in that run" "$ok" \
    "nonce=\"$old_nonce\" token=\"$old_token\" (a replay test with nothing to replay proves nothing)"

# Replay: the watchdog's token from the run above, reported for a later one.
stub replay-budget python3 "echo '$old_nonce' >&9 2>/dev/null
exit 0"
# Replay: the scan token from the run above, by a stand-in that is honest for
# the watchdog, so the guard's own scan check is what has to catch it.
stub replay-scan python3 "for a in \"\$@\"; do
  [ \"\$a\" = -S ] && exec $REAL_PYTHON \"\$@\"
done
echo '$old_token'
exit 0"
for guard in $GUARDS; do
    blocks replay-budget "$guard" "$(allow_envelope "$guard")" "a replayed watchdog token"
    blocks replay-scan   "$guard" "$(allow_envelope "$guard")" "a replayed scan token"
done

# ── grep is certified in the mode the checks actually use ───────────────────
# `matches` is `grep -q <flags> -e <pattern>` on a here-string. A stand-in that
# is honest in every other mode and blind under -q passed the first version of
# this check and then reported every blocked pattern as clean.
stub grep-never   grep 'exit 1'
stub grep-always  grep 'exit 0'
stub grep-error   grep 'exit 3'
stub grep-lies    grep 'echo 7
exit 0'
stub grep-q-blind grep "for a in \"\$@\"; do [ \"\$a\" = -q ] && exit 1; done
exec $REAL_GREP \"\$@\""
stub grep-q-always grep "for a in \"\$@\"; do [ \"\$a\" = -q ] && exit 0; done
exec $REAL_GREP \"\$@\""
# Drops -E, so every extended regular expression is read as a basic one. The
# argument list is rebuilt rather than re-split, so patterns with spaces and
# metacharacters reach grep intact. Where a BRE with `(` is an error the guard
# already blocked on grep's status; where it is a silent no-match (GNU grep) it
# would not have, which is why the first probe is ERE-only.
stub grep-no-E    grep "first=1
for a in \"\$@\"; do
  case \"\$a\" in
    -E) continue ;;
    -iE) a=-i ;;
    -qE) a=-q ;;
  esac
  if [ \"\$first\" = 1 ]; then set -- \"\$a\"; first=0; else set -- \"\$@\" \"\$a\"; fi
done
exec $REAL_GREP \"\$@\""

# Answers correctly except when asked case-insensitively, which is the mode
# half the checks below use — a DROP DATABASE would go unseen.
stub grep-i-blind grep "for a in \"\$@\"; do
  case \"\$a\" in -i|-iE) exit 1 ;; esac
done
exec $REAL_GREP \"\$@\""

# A grep can share the call shape and still not share the regular expressions.
# `\s`, `\b` and `\S` are GNU extensions, so a strict-POSIX or busybox-style
# grep reads them as literals and under-matches — by accident, on somebody's
# PATH, with nothing raised anywhere. Each of these refuses one construct the
# real patterns use; before the probes carried that vocabulary, the first of
# them allowed every dangerous command tested, in silence.
blind_to() { # $1=name  $2=the construct its patterns must not contain
    stub "$1" grep "for a in \"\$@\"; do
  case \"\$a\" in *'$2'*) exit 1 ;; esac
done
exec $REAL_GREP \"\$@\""
}
blind_to grep-s-blind     '\s'
blind_to grep-b-blind     '\b'
blind_to grep-brace-blind '{'
blind_to grep-class-blind '[a-z]'
# Reads only the first line of its input, so a probe whose answer is on line 1
# would certify it while a dangerous command further down went unseen.
stub grep-first-line grep "in=\$(cat)
printf '%s' \"\$in\" | head -1 | exec $REAL_GREP \"\$@\""

for stub_name in grep-never grep-always grep-error grep-lies grep-q-blind grep-q-always grep-no-E \
                 grep-i-blind grep-s-blind grep-b-blind grep-brace-blind grep-class-blind grep-first-line; do
    blocks "$stub_name" dangerous-cmd-guard "$(block_envelope dangerous-cmd-guard)" "a $stub_name"
    blocks "$stub_name" dangerous-cmd-guard "$(allow_envelope dangerous-cmd-guard)" "a $stub_name (ordinary input)"
done

# A command only a case-insensitive check blocks, against a grep that is blind
# in exactly that mode: without a case-insensitive probe this is allowed.
DROP_ENVELOPE=$(printf '{"tool_name":"Bash","tool_input":{"command":"psql -c \\"%s %s prod\\""}}' \
    "DR""OP" "DATA""BASE")
blocks grep-i-blind dangerous-cmd-guard "$DROP_ENVELOPE" "a grep-i-blind, on a case-insensitive check"
got=$(run clean dangerous-cmd-guard "$DROP_ENVELOPE")
check "that command is one the guard blocks on its own reason" \
    "$([ "${got%%|*}" = 2 ] && echo 0 || echo 1)" "rc=${got%%|*} $(printf '%.80s' "${got##*|}")"

# The reason has to name grep, not the interpreter: the two failures are told
# apart so the message points at what actually broke. For the -E-dropping
# stand-in the reason matters twice over — the probes have to be what catches
# it. Where a basic regular expression containing `(` is an error, grep's own
# status would block anyway; where it is a silent no-match (GNU grep) nothing
# downstream would, so a block attributed to the probes is the platform-
# independent statement of the property.
for stub_name in grep-never grep-q-blind grep-no-E grep-s-blind grep-first-line; do
    got=$(run "$stub_name" dangerous-cmd-guard "$(allow_envelope dangerous-cmd-guard)")
    ok=1; case "${got##*|}" in *"a question with a known answer"*) ok=0 ;; esac
    check "a $stub_name is caught by the probes, and named" "$ok" "$(printf '%.110s' "${got##*|}")"
done

# ── The probes speak the same regex dialect as the patterns ─────────────────
# Sharing the call shape is not sharing the vocabulary. This reads the real
# patterns and the probe patterns out of the guard and fails when a construct
# appears in the first and in none of the second — so adding a pattern that
# uses something the probes do not exercise is visible here rather than on
# somebody's PATH. The catalogue is of constructs, not spellings: a grep that
# handles `[0-9]` and refuses one particular range is the purpose-built class,
# not the accidental one.
drift=$(python3 -I -c '
import re, sys

CATALOGUE = [
    (r"\s",            r"\\s"),
    (r"\b",            r"\\b"),
    (r"\S",            r"\\S"),
    (r"\w",            r"\\w"),
    (r"\d",            r"\\d"),
    ("{m,n}",          r"\{[0-9]+,?[0-9]*\}"),
    ("[[:class:]]",    r"\[\[:"),
    ("[...]",          r"\[(?!\[:)"),
    ("literal brace",  r"[{}]"),
    ("alternation",    r"\|"),
    ("group",          r"\("),
    ("+",              r"\+"),
    ("*",              r"\*"),
    ("?",              r"\?"),
    ("^",              r"\^"),
    ("$",              r"\$"),
    ("backreference",  r"\\[1-9]"),
]

src = open(sys.argv[1]).read()
body = src[src.index("Blocked patterns"):]
real = "\n".join(s for s in (l.strip() for l in body.splitlines())
                 if s and not s.startswith("#")
                 and (re.match(r"^[A-Z_]+=", s) or re.match(r"^(if )?matches ", s)))
probes = "\n".join(l for l in src.splitlines() if "devexp_grep_probe -" in l)
gaps = [name for name, det in CATALOGUE
        if re.search(det, real) and not re.search(det, probes)]
print(",".join(gaps) if gaps else "-")
print(sum(1 for name, det in CATALOGUE if re.search(det, real)))
' "$DIR/dangerous-cmd-guard.sh")
gaps=$(printf '%s' "$drift" | head -1)
covered=$(printf '%s' "$drift" | tail -1)
check "every construct the real patterns use is exercised by a probe" \
    "$([ "$gaps" = "-" ] && echo 0 || echo 1)" "no probe exercises: $gaps"
check "the patterns use enough constructs for that to mean something" \
    "$([ "${covered:-0}" -ge 8 ] && echo 0 || echo 1)" "only $covered constructs found in the real patterns"

# ── The proof descriptor reaches the watchdog and nobody else ───────────────
# The guard waits for every holder of that pipe, so anything the guarded run
# leaves behind would hold the guard open past its budget — the same
# fail-open again. The only thing keeping descendants off it is close_fds on the
# watchdog's Popen, so that is pinned here rather than assumed.
cat > "$TMP/fd-guard.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
. "$DIR/scan-budget.sh"
devexp_scan_budget fd-guard "\$@"
cat >/dev/null
if { : >&9; } 2>/dev/null; then echo visible > "$TMP/fd-seen"; else echo hidden > "$TMP/fd-seen"; fi
exit 0
EOF
rm -f "$TMP/fd-seen"
bash "$TMP/fd-guard.sh" </dev/null >/dev/null 2>&1; rc=$?
check "the guarded run cannot see the proof descriptor" \
    "$([ "$rc" = 0 ] && [ "$(cat "$TMP/fd-seen" 2>/dev/null)" = hidden ] && echo 0 || echo 1)" \
    "rc=$rc saw \"$(cat "$TMP/fd-seen" 2>/dev/null)\""

# And the consequence, end to end: a process the guarded run leaves behind must
# not extend the guard's own wall time.
cat > "$TMP/linger-guard.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
. "$DIR/scan-budget.sh"
devexp_scan_budget linger-guard "\$@"
cat >/dev/null
( sleep 20 ) &
echo \$! >> "$TMP/sweep"
exit 0
EOF
: > "$TMP/sweep"
start=$(python3 -I -c 'import time; print(time.time())')
bash "$TMP/linger-guard.sh" </dev/null >/dev/null 2>&1; rc=$?
elapsed=$(python3 -I -c 'import sys, time; print("%.2f" % (time.time() - float(sys.argv[1])))' "$start")
quick=$(python3 -I -c 'import sys; print(1 if float(sys.argv[1]) < 8 else 0)' "$elapsed")
check "a process left behind by the guarded run does not extend the guard" \
    "$([ "$rc" = 0 ] && [ "$quick" = 1 ] && echo 0 || echo 1)" \
    "rc=$rc after ${elapsed}s (a 20 s sleeper was left running)"

# ── A caller with no stdout still gets a decision ───────────────────────────
# The proof channel is a duplicate of the guard's own stdout. Claude Code always
# gives a hook one; a harness or a wrapper may not, and that must not turn into
# "every tool call blocked", nor into a raw shell error from inside the guard.
for guard in $GUARDS; do
    err="$TMP/closed-err"
    printf '%s' "$(allow_envelope "$guard")" | bash "$DIR/$guard.sh" >&- 2>"$err"; rc=$?
    stderr=$(cat "$err")
    check "$guard still allows ordinary input with stdout closed" \
        "$([ "$rc" = 0 ] && [ -z "$stderr" ] && echo 0 || echo 1)" "rc=$rc $(printf '%.110s' "$stderr")"

    printf '%s' "$(block_envelope "$guard")" | bash "$DIR/$guard.sh" >&- 2>"$err"; rc=$?
    stderr=$(cat "$err")
    ok=1
    if [ "$rc" = 2 ]; then
        case "$stderr" in
            *"Bad file descriptor"*|*"internal error"*) ;;
            *"[devexp $guard] Blocked"*) ok=0 ;;
        esac
    fi
    check "$guard still blocks on its own reason with stdout closed" "$ok" "rc=$rc $(printf '%.110s' "$stderr")"
done

# ── The healthy paths are untouched ─────────────────────────────────────────
mkdir -p "$TMP/clean"
for guard in $GUARDS; do
    got=$(run clean "$guard" "$(allow_envelope "$guard")")
    check "$guard still allows ordinary input, in silence" \
        "$([ "$got" = "0||" ] && echo 0 || echo 1)" "got $(printf '%.90s' "$got")"

    got=$(run clean "$guard" "$(block_envelope "$guard")")
    rc=${got%%|*}; err=${got##*|}
    ok=1
    [ "$rc" = 2 ] && case "$err" in
        *"internal error"*) ;;                    # its own reason, not a proof failure
        *"[devexp $guard] Blocked"*) ok=0 ;;
    esac
    check "$guard still blocks on its own reason" "$ok" "rc=$rc $(printf '%.90s' "$err")"
done

# ── Every fail-closed guard asks for the proof ──────────────────────────────
# A new guard that forgets to is exactly the hole this closes, and it would
# pass every other suite. Both spellings are matched as code — at the start of
# a line, or as an argument — so a mention in a comment does not satisfy them.
for guard in $GUARDS; do
    ok=1
    grep -qE "^devexp_scan_result +$guard( |\$)" "$DIR/$guard.sh" &&
        grep -qE '^[^#]*"\$DEVEXP_SCAN_PROOF"' "$DIR/$guard.sh" && ok=0
    check "$guard requires proof from its own scan" "$ok" \
        "$guard.sh must call devexp_scan_result and pass \$DEVEXP_SCAN_PROOF to its scanning step"
done

# ── One place hands a pattern to grep ──────────────────────────────────────
# The probes certify the call the checks make, which only means something while
# there is exactly one such call. Re-inlining it in `matches` is behaviour-
# preserving today and takes the basis of that away, so it is structural here.
funnel=$(python3 -I -c '
import re, sys
calls, heredoc = [], None
for path in sys.argv[1:]:
    name = path.rsplit("/", 1)[-1]
    for n, line in enumerate(open(path), 1):
        # Program text embedded in a quoted heredoc is data, not a command.
        if heredoc is None:
            m = re.match(r"^\s*\S*\s*<<[-]?[\x27\x22]?([A-Za-z_][A-Za-z0-9_]*)", line)
            if m:
                heredoc = m.group(1)
                continue
        else:
            if line.strip() == heredoc:
                heredoc = None
            continue
        code = line.split("#", 1)[0]
        if re.search(r"(?:^|[;&|(]|\$\()\s*grep\s", code):
            calls.append("%s:%d" % (name, n))
print(" ".join(calls) if calls else "-")
' "$DIR/dangerous-cmd-guard.sh" "$DIR/secret-guard.sh" "$DIR/secret-in-write-guard.sh" "$DIR/scan-budget.sh")
inside=$(awk '/^devexp_grep\(\) \{/{s=NR} /^\}/{if (s && !e) {e=NR}} END{print s "-" e}' "$DIR/dangerous-cmd-guard.sh")
call_line=${funnel#*:}
check "exactly one grep invocation across the guards and the helper" \
    "$([ "$(printf '%s' "$funnel" | wc -w | tr -d ' ')" = 1 ] && echo 0 || echo 1)" "found: $funnel"
check "that invocation is inside devexp_grep" \
    "$(python3 -I -c '
import sys
span, call = sys.argv[1], sys.argv[2]
start, end = (int(x) for x in span.split("-"))
sys.exit(0 if start < int(call) < end else 1)' "$inside" "${call_line:-0}" && echo 0 || echo 1)" \
    "devexp_grep spans lines $inside, the call is at line ${call_line:-none}"

# ── The guarded run's stdout still reaches the caller ───────────────────────
# fd 8 exists so a guard can write where it always did while the proof travels
# separately. Forcing the "no stdout" branch sends that output to /dev/null
# instead, which nothing else would notice.
cat > "$TMP/speak-guard.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
. "$DIR/scan-budget.sh"
devexp_scan_budget speak-guard "\$@"
cat >/dev/null
echo "devexp-passthrough-marker"
exit 0
EOF
spoke=$(bash "$TMP/speak-guard.sh" </dev/null 2>/dev/null); rc=$?
check "the guarded run's stdout reaches the caller" \
    "$([ "$rc" = 0 ] && [ "$spoke" = "devexp-passthrough-marker" ] && echo 0 || echo 1)" \
    "rc=$rc stdout=\"$spoke\""

# The proof channel is spelled twice — a redirection needs a literal number —
# so the two spellings have to agree or the watchdog's proof goes nowhere.
fd_const=$(sed -n 's/^DEVEXP_SCAN_BUDGET_PROOF_FD=\([0-9]*\)$/\1/p' "$DIR/scan-budget.sh")
fd_redir=$(sed -n 's/.*[^0-9]\([0-9]\)>&1 1>&8.*/\1/p' "$DIR/scan-budget.sh")
check "the proof descriptor and its redirection agree" \
    "$([ -n "$fd_const" ] && [ "$fd_const" = "$fd_redir" ] && echo 0 || echo 1)" \
    "constant=$fd_const redirection=$fd_redir"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
