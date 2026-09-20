#!/usr/bin/env bash
# Measures what each fail-closed guard actually costs, against the budget that
# would block it.
#
# Why this is a correctness test and not a comfort one: a guard that runs long
# does not merely make the user wait. scan-budget.sh gives each guard a
# wall-clock budget and a budget hit exits 2 — a block, never 0. So a guard
# that grows past its budget starts DENYING legitimate tool calls. The failure
# mode of slowness here is a refusal, not a delay.
#
# scan-budget.test.sh already proves the mechanism: a forced budget hit blocks,
# the budget cannot be switched off from the environment, an over-ceiling value
# is clamped. All of that uses a forced budget. What was never asserted is that
# a real guard, on realistic input, finishes inside the real one. The 15000 ms
# default was sized from worst cases measured once and never re-checked, while
# the pattern sets those guards scan keep being widened.
#
# Shape of the assertion. A fixed millisecond ceiling on shared CI hardware is
# the wrong test — it turns a busy runner into a red build, and a gate that
# cries wolf gets ignored or deleted, which costs more than it ever caught. So:
#
#   * each guard is measured best-of-N. The minimum is the right statistic for
#     "how long does this take" — a scheduler can only ever make a run slower,
#     so the fastest observation is the least contaminated one.
#   * the assertion is a FRACTION of the budget, not a millisecond figure, and
#     the fraction is generous. Failing at half the budget means a widening
#     surfaces here, in a test naming the guard, rather than in production as a
#     denied tool call with no explanation.
#   * every measurement prints the percentage consumed, pass or fail, so the
#     trend is visible before it is a failure.
#
# Run: bash hooks/claude-code/scan-latency.test.sh
set -euo pipefail

dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Kept in step with DEVEXP_SCAN_BUDGET_DEFAULT_MS in scan-budget.sh. Read from
# the file rather than copied, so the two cannot drift: a test that hardcodes
# the budget keeps passing after the budget is lowered underneath it.
BUDGET_MS=$(sed -n 's/^DEVEXP_SCAN_BUDGET_DEFAULT_MS=\([0-9]*\).*/\1/p' "$dir/scan-budget.sh")
[ -n "$BUDGET_MS" ] || { echo "cannot read DEVEXP_SCAN_BUDGET_DEFAULT_MS from scan-budget.sh"; exit 1; }

# The headroom margin. A guard may consume at most this fraction of the budget
# on its worst realistic input.
#
# Justification, so a future change to this number is a decision and not a
# nudge: the worst cases recorded against the budget are ~1.0 s for a 2 MB
# write and ~1.7 s for a 1 MB command, which is 7% and 11% of 15000 ms. A CI
# runner is commonly 2-4x slower than a developer machine, putting the worst
# case near 45% in the bad case — so 50% is about one doubling of headroom over
# the slowest plausible honest run, and roughly 4x the current worst case on
# this machine. Anything that crosses it has either grown a lot or gone
# non-linear, and both deserve a red build while there is still 2x of real
# budget left to absorb them in production.
MARGIN_NUM=1
MARGIN_DEN=2
LIMIT_MS=$(( BUDGET_MS * MARGIN_NUM / MARGIN_DEN ))

REPEATS=3

pass=0
fail=0

# measure <label> <guard-script> <envelope-producing python>
# Runs the guard REPEATS times on the envelope and reports the fastest run.
measure() {
  local label="$1" guard="$2" gen="$3"
  local best=""

  local envelope
  envelope=$(python3 -c "$gen")

  local i ms
  for ((i = 0; i < REPEATS; i++)); do
    ms=$(printf '%s' "$envelope" | python3 -c '
import subprocess, sys, time
payload = sys.stdin.buffer.read()
t0 = time.perf_counter()
subprocess.run(["bash", sys.argv[1]], input=payload,
               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
print(int((time.perf_counter() - t0) * 1000))
' "$guard")
    if [ -z "$best" ] || [ "$ms" -lt "$best" ]; then best="$ms"; fi
  done

  local pct=$(( best * 100 / BUDGET_MS ))
  if [ "$best" -le "$LIMIT_MS" ]; then
    pass=$((pass + 1))
    printf '  ok   %-46s %6s ms  %3s%% of budget\n' "$label" "$best" "$pct"
  else
    fail=$((fail + 1))
    printf 'FAIL   %-46s %6s ms  %3s%% of budget (limit %s ms, %d%%)\n' \
      "$label" "$best" "$pct" "$LIMIT_MS" "$(( 100 * MARGIN_NUM / MARGIN_DEN ))"
    printf '       The guard now consumes more of its scan budget than the margin allows.\n'
    printf '       A budget hit is a BLOCK, so this is a denied tool call in the making.\n'
    printf '       Either the work it does per byte grew, or it went non-linear on this shape.\n'
  fi
}

printf 'scan budget %s ms, margin %d%%, limit %s ms, best of %d\n\n' \
  "$BUDGET_MS" "$(( 100 * MARGIN_NUM / MARGIN_DEN ))" "$LIMIT_MS" "$REPEATS"

# ── The envelopes ───────────────────────────────────────────────────────────
#
# Each is the largest realistic input of its kind, not a pathological one: the
# point is what a real session can hand a guard, since that is what a block
# would be denying.

# secret-in-write-guard: a 2 MB file write. The worst case its own timing tests
# already use, and the shape most likely to grow as write patterns widen.
measure 'secret-in-write-guard: 2 MB write' "$dir/secret-in-write-guard.sh" '
import json
content = ("def handler(request, response):\n"
           "    value = compute(request.body, response.headers)\n"
           "    return {\"ok\": True, \"value\": value}\n") * 24000
print(json.dumps({"tool_name": "Write",
                  "tool_input": {"file_path": "src/big_module.py", "content": content}}))
'

# dangerous-cmd-guard: a long pipeline. Checking every stage against the rest
# of the pipeline was once quadratic here, so this is the shape whose cost is
# least obvious from reading the patterns.
measure 'dangerous-cmd-guard: 400 KB pipeline' "$dir/dangerous-cmd-guard.sh" '
import json
print(json.dumps({"tool_name": "Bash", "tool_input": {"command": "a|" * 200000 + "a"}}))
'

# dangerous-cmd-guard: one very long command line, no pipeline structure.
measure 'dangerous-cmd-guard: 1 MB command' "$dir/dangerous-cmd-guard.sh" '
import json
print(json.dumps({"tool_name": "Bash",
                  "tool_input": {"command": "echo " + "x" * 1000000}}))
'

# secret-guard on Bash: a long command line carrying many path-like tokens,
# which is what its patterns actually walk.
measure 'secret-guard: 1 MB command' "$dir/secret-guard.sh" '
import json
print(json.dumps({"tool_name": "Bash",
                  "tool_input": {"command": " ".join(["cat src/config/%d.ts" % i for i in range(40000)])}}))
'

# secret-guard on Read: a very long path. Small, and deliberately so — this is
# the call shape that happens constantly, and its cost is paid on every read.
measure 'secret-guard: long Read path' "$dir/secret-guard.sh" '
import json
print(json.dumps({"tool_name": "Read",
                  "tool_input": {"file_path": "src/" + "nested/" * 2000 + "file.ts"}}))
'

# The ordinary case. Every tool call in a session pays this, so a regression
# here is felt on every single call rather than on a rare large input.
measure 'secret-guard: ordinary Read' "$dir/secret-guard.sh" '
import json
print(json.dumps({"tool_name": "Read", "tool_input": {"file_path": "src/index.ts"}}))
'

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
