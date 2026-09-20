/**
 * Measures what each fail-closed guard actually costs, against the budget that
 * would block it. The opencode twin of
 * hooks/claude-code/scan-latency.test.sh, on the same inputs and the same
 * margin, because both implementations run in front of real tool calls and a
 * regression in either one denies the user's call.
 *
 * Why this is a correctness test and not a comfort one: utils.js gives each
 * guard a wall-clock budget, and a guard that exceeds it throws rather than
 * returning. So a guard that grows past its budget does not make the user
 * wait — it starts DENYING legitimate tool calls. The failure mode of slowness
 * here is a refusal.
 *
 * scan-budget.test.js already proves the mechanism against a forced budget.
 * What was never asserted is that a real guard, on realistic input, finishes
 * inside the real one. The 15000 ms default was sized from worst cases
 * measured once and never re-checked, while the pattern sets these guards scan
 * keep being widened.
 *
 * Shape of the assertion — a fixed millisecond ceiling on shared CI hardware is
 * the wrong test, because a busy runner becomes a red build and a gate that
 * cries wolf gets deleted. So each guard is measured best-of-N (a scheduler can
 * only make a run slower, so the fastest observation is the least contaminated
 * one), the limit is a generous FRACTION of the budget rather than a figure in
 * milliseconds, and every measurement prints the percentage consumed, pass or
 * fail, so the trend is visible before it is a failure.
 *
 * These numbers come out far below the shell twin's, and that is not an error
 * in either. A Claude Code hook is a fresh process per tool call, so its
 * measurement necessarily includes bash and python3 startup, which it really
 * does pay every time. The opencode plugin is loaded once and its handlers are
 * called in-process thereafter, so measuring the handler is what that runtime
 * actually costs. Each side models its own runtime; do not "fix" the gap by
 * making them measure the same thing, because they do not run the same way.
 *
 * Run: node hooks/opencode/scan-latency.test.js
 */
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { SCAN_BUDGET_DEFAULT_MS } from './utils.js';
import { secretGuard } from './secret-guard.js';
import { secretInWriteGuard } from './secret-in-write-guard.js';
import { dangerousCmdGuard } from './dangerous-cmd-guard.js';

const here = dirname(fileURLToPath(import.meta.url));

// The two twins must agree on the budget, or one of them is measuring against
// a number the other does not enforce. Read the shell default out of the file
// rather than trusting that they still match.
const shellDefault = Number(
  /^DEVEXP_SCAN_BUDGET_DEFAULT_MS=(\d+)/m.exec(
    readFileSync(join(here, '..', 'claude-code', 'scan-budget.sh'), 'utf8'),
  )?.[1],
);

const BUDGET_MS = SCAN_BUDGET_DEFAULT_MS;

// See the shell twin for the full justification of this number. In short: the
// recorded worst cases sit near 10% of the budget, a CI runner is commonly 2-4x
// slower than a developer machine, and 50% is about one doubling of headroom
// over the slowest plausible honest run — crossed only by real growth or by a
// guard going non-linear, both of which deserve a red build while there is
// still 2x of real budget left to absorb them in production.
const MARGIN = 0.5;
const LIMIT_MS = BUDGET_MS * MARGIN;
const REPEATS = 3;

let pass = 0;
let fail = 0;

if (shellDefault !== BUDGET_MS) {
  fail++;
  console.log(
    `FAIL budget twins disagree: utils.js SCAN_BUDGET_DEFAULT_MS = ${BUDGET_MS}, ` +
      `scan-budget.sh DEVEXP_SCAN_BUDGET_DEFAULT_MS = ${shellDefault}.\n` +
      '       Both guards run in front of real tool calls; a budget enforced on one side only\n' +
      '       means one CLI blocks where the other allows, on the same input.',
  );
}

// Runs the handler REPEATS times on args and reports the fastest run. The guard
// throwing is the block path and is timed like any other — a block is not an
// error here, it is one of the two outcomes.
async function measure(label, guardFactory, tool, args) {
  const handler = (await guardFactory({}))['tool.execute.before'];
  let best = Infinity;
  for (let i = 0; i < REPEATS; i++) {
    const t0 = performance.now();
    try {
      await handler({ tool }, { args });
    } catch {
      /* a block is an outcome, not a failure of the measurement */
    }
    best = Math.min(best, performance.now() - t0);
  }

  const ms = Math.round(best);
  const pct = Math.round((best / BUDGET_MS) * 100);
  if (best <= LIMIT_MS) {
    pass++;
    console.log(`  ok   ${label.padEnd(46)} ${String(ms).padStart(6)} ms  ${String(pct).padStart(3)}% of budget`);
  } else {
    fail++;
    console.log(
      `FAIL   ${label.padEnd(46)} ${String(ms).padStart(6)} ms  ${String(pct).padStart(3)}% of budget ` +
        `(limit ${LIMIT_MS} ms, ${MARGIN * 100}%)\n` +
        '       The guard now consumes more of its scan budget than the margin allows.\n' +
        '       Exceeding the budget is a BLOCK, so this is a denied tool call in the making.\n' +
        '       Either the work it does per byte grew, or it went non-linear on this shape.',
    );
  }
}

console.log(
  `scan budget ${BUDGET_MS} ms, margin ${MARGIN * 100}%, limit ${LIMIT_MS} ms, best of ${REPEATS}\n`,
);

// ── The envelopes ───────────────────────────────────────────────────────────
//
// The same shapes the shell twin uses, so a divergence between the two
// implementations shows up as a difference in the printed percentages rather
// than as one suite testing something the other does not.

const bigFile =
  'def handler(request, response):\n' +
  '    value = compute(request.body, response.headers)\n' +
  '    return {"ok": True, "value": value}\n';

await measure('secret-in-write-guard: 2 MB write', secretInWriteGuard, 'write', {
  filePath: 'src/big_module.py',
  content: bigFile.repeat(24000),
});

await measure('dangerous-cmd-guard: 400 KB pipeline', dangerousCmdGuard, 'bash', {
  command: 'a|'.repeat(200000) + 'a',
});

await measure('dangerous-cmd-guard: 1 MB command', dangerousCmdGuard, 'bash', {
  command: 'echo ' + 'x'.repeat(1000000),
});

await measure('secret-guard: 1 MB command', secretGuard, 'bash', {
  command: Array.from({ length: 40000 }, (_, i) => `cat src/config/${i}.ts`).join(' '),
});

await measure('secret-guard: long Read path', secretGuard, 'read', {
  filePath: 'src/' + 'nested/'.repeat(2000) + 'file.ts',
});

// The ordinary case. Every tool call in a session pays this, so a regression
// here is felt on every call rather than on a rare large input.
await measure('secret-guard: ordinary Read', secretGuard, 'read', { filePath: 'src/index.ts' });

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
