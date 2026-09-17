/**
 * scan-budget.test.js — the budget every fail-closed security guard scans under (#162)
 *
 * Mirrors hooks/claude-code/scan-budget.test.sh, and checks the two twins
 * against each other: the same input has to get the same verdict from both,
 * a budget hit included.
 *
 * opencode puts no timeout on a plugin hook and runs it in its server process,
 * so a scan that drags on stalls the session. The budget turns that into a
 * refusal — a throw, which is how `tool.execute.before` refuses a call.
 *
 * Run: node hooks/opencode/scan-budget.test.js
 */

import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { SCAN_BUDGET_DEFAULT_MS, scanBudgetMs, startScanBudget } from './utils.js';
import { secretGuard } from './secret-guard.js';
import { dangerousCmdGuard } from './dangerous-cmd-guard.js';
import { secretInWriteGuard } from './secret-in-write-guard.js';

const HERE = dirname(fileURLToPath(import.meta.url));
const CC = join(HERE, '..', 'claude-code');

let pass = 0;
let fail = 0;
const check = (label, ok, detail = '') => {
  if (ok) pass++;
  else { fail++; console.log(`FAIL ${label}${detail ? `: ${detail}` : ''}`); }
};

const withBudget = async (ms, fn) => {
  const had = Object.prototype.hasOwnProperty.call(process.env, 'DEVEXP_SCAN_BUDGET_MS');
  const before = process.env.DEVEXP_SCAN_BUDGET_MS;
  if (ms === null) delete process.env.DEVEXP_SCAN_BUDGET_MS;
  else process.env.DEVEXP_SCAN_BUDGET_MS = ms;
  try {
    return await fn();
  } finally {
    if (had) process.env.DEVEXP_SCAN_BUDGET_MS = before;
    else delete process.env.DEVEXP_SCAN_BUDGET_MS;
  }
};

// ── The budget value ────────────────────────────────────────────────────────
// Only a plain non-negative integer counts. Everything else falls back to the
// default: a typo must neither widen the budget nor disable the guard.
check('an unset budget is the default', scanBudgetMs({}) === SCAN_BUDGET_DEFAULT_MS, String(scanBudgetMs({})));
check('the default is a whole number of seconds', SCAN_BUDGET_DEFAULT_MS % 1000 === 0, String(SCAN_BUDGET_DEFAULT_MS));
for (const [raw, want] of [
  ['0', 0], ['250', 250], ['  250  ', 250], ['15000', 15000],
  ['abc', SCAN_BUDGET_DEFAULT_MS], ['-1', SCAN_BUDGET_DEFAULT_MS], ['1e3', SCAN_BUDGET_DEFAULT_MS],
  ['10s', SCAN_BUDGET_DEFAULT_MS], ['', SCAN_BUDGET_DEFAULT_MS], [' ', SCAN_BUDGET_DEFAULT_MS],
  ['1.5', SCAN_BUDGET_DEFAULT_MS], ['0x10', SCAN_BUDGET_DEFAULT_MS],
]) {
  const got = scanBudgetMs({ DEVEXP_SCAN_BUDGET_MS: raw });
  check(`budget ${JSON.stringify(raw)} reads as ${want}`, got === want, `got ${got}`);
}

// ── The check throws once the budget is spent, and not before ───────────────
const threw = (fn) => { try { fn(); return null; } catch (e) { return e.message; } };
check('a zero budget is over at the first check',
  (threw(startScanBudget('g', { DEVEXP_SCAN_BUDGET_MS: '0' })) ?? '').includes('Blocked:'));
check('a real budget is not spent at the first check',
  threw(startScanBudget('g', { DEVEXP_SCAN_BUDGET_MS: '60000' })) === null);
check('the message names the guard',
  (threw(startScanBudget('secret-guard', { DEVEXP_SCAN_BUDGET_MS: '0' })) ?? '').startsWith('[devexp secret-guard] '));

// ── The two twins say the same thing about the same budget ──────────────────
// Seconds are spelled the same on both sides, so a user reading either message
// sees the same number.
// A guard that would never finish on its own, so the shell twin's deadline
// always fires and always prints its budget.
const SLOW_GUARD = join(mkdtempSync(join(tmpdir(), 'devexp-budget-')), 'slow-guard.sh');
writeFileSync(SLOW_GUARD, [
  '#!/usr/bin/env bash',
  'set -euo pipefail',
  `. ${JSON.stringify(join(CC, 'scan-budget.sh'))}`,
  'devexp_scan_budget slow-guard "$@"',
  'sleep 30',
  '',
].join('\n'));

const seconds = (message) => (/within its (\S+) s budget/.exec(message ?? '') ?? [])[1];
const shellSeconds = (ms) => seconds(spawnSync('bash', [SLOW_GUARD], {
  input: '', env: { ...process.env, DEVEXP_SCAN_BUDGET_MS: String(ms) }, encoding: 'utf8',
}).stderr);
// The JS check only throws once the clock has actually passed the deadline, so
// the budget is spent here before it is read.
const jsSeconds = (ms) => {
  const overBudget = startScanBudget('g', { DEVEXP_SCAN_BUDGET_MS: String(ms) });
  const until = performance.now() + ms + 20;
  while (performance.now() < until) { /* spend it */ }
  return seconds(threw(overBudget));
};
for (const ms of [0, 1, 250, 1250]) {
  const js = jsSeconds(ms);
  const sh = shellSeconds(ms);
  check(`${ms} ms reads as the same seconds in both twins`, js !== undefined && js === sh, `js=${js} shell=${sh}`);
}
check('both twins carry the same default budget',
  SCAN_BUDGET_DEFAULT_MS === Number(
    /^DEVEXP_SCAN_BUDGET_DEFAULT_MS=(\d+)$/m.exec(
      spawnSync('cat', [join(CC, 'scan-budget.sh')], { encoding: 'utf8' }).stdout)?.[1]),
  `js=${SCAN_BUDGET_DEFAULT_MS}`);

// ── The guards ──────────────────────────────────────────────────────────────
// Each case is one input in both spellings: the opencode tool args, and the
// Claude Code PreToolUse envelope for the same thing.
const KEY = 'AKIA' + 'Q'.repeat(8) + '7'.repeat(8);
const CASES = [
  {
    guard: 'secret-guard', factory: secretGuard, script: 'secret-guard.sh',
    handled: [
      { tool: 'read', args: { filePath: 'README.md' } },
      { tool: 'bash', args: { command: 'cat README.md' } },
    ],
    ignored: { tool: 'write', args: { content: 'hello world' } },
    allow: { oc: { tool: 'bash', args: { command: 'cat README.md' } },
             cc: { tool_name: 'Bash', tool_input: { command: 'cat README.md' } } },
    block: { oc: { tool: 'read', args: { filePath: '.env' } },
             cc: { tool_name: 'Read', tool_input: { file_path: '.env' } },
             reason: 'Blocked access' },
  },
  {
    guard: 'dangerous-cmd-guard', factory: dangerousCmdGuard, script: 'dangerous-cmd-guard.sh',
    handled: [{ tool: 'bash', args: { command: 'ls -la' } }],
    ignored: { tool: 'read', args: { filePath: 'README.md' } },
    allow: { oc: { tool: 'bash', args: { command: 'ls -la' } },
             cc: { tool_name: 'Bash', tool_input: { command: 'ls -la' } } },
    block: { oc: { tool: 'bash', args: { command: 'git push --force' } },
             cc: { tool_name: 'Bash', tool_input: { command: 'git push --force' } },
             reason: 'overwrite remote history' },
  },
  {
    guard: 'secret-in-write-guard', factory: secretInWriteGuard, script: 'secret-in-write-guard.sh',
    handled: [
      { tool: 'write', args: { filePath: 'a.txt', content: 'hello world' } },
      { tool: 'edit', args: { filePath: 'a.txt', newString: 'hello world' } },
      { tool: 'apply_patch', args: { patchText: '+hello world' } },
    ],
    ignored: { tool: 'bash', args: { command: 'ls -la' } },
    allow: { oc: { tool: 'write', args: { filePath: 'a.txt', content: 'hello world' } },
             cc: { tool_name: 'Write', tool_input: { file_path: 'a.txt', content: 'hello world' } } },
    block: { oc: { tool: 'write', args: { filePath: 'a.txt', content: KEY } },
             cc: { tool_name: 'Write', tool_input: { file_path: 'a.txt', content: KEY } },
             reason: 'AWS Access Key' },
  },
];

/** Runs the opencode handler; returns the thrown message, or null when allowed. */
const runJs = async (factory, call) => {
  const handler = (await factory({}))['tool.execute.before'];
  try {
    await handler({ tool: call.tool }, { args: call.args });
    return null;
  } catch (e) {
    return e.message;
  }
};

/** Runs the Claude Code twin; returns { rc, stderr }. */
const runSh = (script, envelope, budget) => {
  const env = { ...process.env };
  if (budget === null) delete env.DEVEXP_SCAN_BUDGET_MS;
  else env.DEVEXP_SCAN_BUDGET_MS = String(budget);
  const r = spawnSync('bash', [join(CC, script)], { input: JSON.stringify(envelope), env, encoding: 'utf8' });
  return { rc: r.status, stderr: r.stderr ?? '' };
};

for (const c of CASES) {
  // Over budget: every tool the guard scans is refused, with the budget's own
  // message rather than the guard's finding.
  for (const call of c.handled) {
    const msg = await withBudget('0', () => runJs(c.factory, call));
    check(`${c.guard} blocks ${call.tool} when over budget`,
      msg !== null && msg.includes(`[devexp ${c.guard}] Blocked:`) && msg.includes('budget'), String(msg));
  }
  // A tool this guard does not scan is not its business, budget or no budget.
  const ignored = await withBudget('0', () => runJs(c.factory, c.ignored));
  check(`${c.guard} still ignores ${c.ignored.tool}`, ignored === null, String(ignored));

  // Under the real budget nothing changes: ordinary input passes, and a real
  // finding is reported as itself.
  const allowed = await withBudget(null, () => runJs(c.factory, c.allow.oc));
  check(`${c.guard} allows ordinary input under the default budget`, allowed === null, String(allowed));
  const blocked = await withBudget(null, () => runJs(c.factory, c.block.oc));
  check(`${c.guard} blocks on its own reason under the default budget`,
    blocked !== null && blocked.includes(c.block.reason) && !blocked.includes('budget'), String(blocked));

  // ── Parity: the same input, the same verdict from both twins ──────────────
  for (const [label, input, budget, wantBlock] of [
    ['over budget', c.allow, '0', true],
    ['ordinary input', c.allow, null, false],
    ['a real finding', c.block, null, true],
  ]) {
    const js = await withBudget(budget, () => runJs(c.factory, input.oc));
    const sh = runSh(c.script, input.cc, budget);
    const jsBlocked = js !== null;
    const shBlocked = sh.rc === 2;
    check(`${c.guard} twins agree on ${label}`,
      jsBlocked === shBlocked && jsBlocked === wantBlock,
      `js=${jsBlocked ? `block(${js})` : 'allow'} shell=${shBlocked ? `block(${sh.stderr.trim()})` : `allow(rc ${sh.rc})`}`);
  }
}

// ── The budget is checked inside the scan, not only around it ───────────────
// A 1 ms budget is not yet spent when the handler starts, so the check on the
// way in passes and only a check made during the scan can refuse the call. Each
// input is big enough that the first unit of work runs well past 1 ms. The same
// input under the real budget is allowed, so what refuses it is the clock.
const rep = (unit, n) => unit.repeat(Math.ceil(n / unit.length)).slice(0, n);
const HEAD = '-'.repeat(5) + 'BEGIN RSA PRIVATE KEY' + '-'.repeat(5) + '\n';
const INSIDE = [
  ['secret-in-write-guard', secretInWriteGuard,
    { tool: 'write', args: { filePath: 'big.txt', content: rep(HEAD + rep('A'.repeat(31) + '.', 600), 2_000_000) } }],
  ['dangerous-cmd-guard', dangerousCmdGuard,
    { tool: 'bash', args: { command: rep('echo a\n', 2_000_000) } }],
  ['secret-guard', secretGuard,
    { tool: 'bash', args: { command: rep('a ', 2_000_000) } }],
];
for (const [guard, factory, call] of INSIDE) {
  const t0 = performance.now();
  const msg = await withBudget('1', () => runJs(factory, call));
  const took = performance.now() - t0;
  check(`${guard} gives up from inside the scan`,
    msg !== null && msg.includes('budget'), `after ${took.toFixed(0)} ms: ${msg}`);
  check(`${guard} finishes the same input under the real budget`,
    (await withBudget(null, () => runJs(factory, call))) === null);
}

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
