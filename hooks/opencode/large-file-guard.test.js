/**
 * Tests for large-file-guard.js — the opencode twin of
 * hooks/claude-code/large-file-guard.test.sh.
 * Run: node hooks/opencode/large-file-guard.test.js
 *
 * The shell twin spends most of its cases proving that a file name is data and
 * never code, because a bash hook interpolates the name into a command line.
 * This guard reads `output.args.filePath` as a JavaScript string and passes it
 * to fs, so there is no interpolation context to escape — that axis does not
 * transfer. What does transfer is the decision table, and it is what this file
 * pins: the guard blocks a full overwrite of a file over the threshold, stays
 * silent at or under it, and never fires for a tool that is not `write`.
 *
 * The hostile names at the end are still exercised, for a different reason: the
 * thrown message embeds `basename(filePath)`, and a name must survive into it
 * unchanged rather than being mangled or swallowed.
 */
import { mkdtempSync, writeFileSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { largeFileGuard } from './large-file-guard.js';

const handler = (await largeFileGuard({}))['tool.execute.before'];
const THRESHOLD = 500;
const dir = mkdtempSync(join(tmpdir(), 'lfg-'));

let fail = 0;
let ran = 0;

const ok = (cond, label, detail = '') => {
  ran++;
  if (!cond) {
    fail++;
    console.log(`FAIL ${label}${detail ? `: ${detail}` : ''}`);
  }
};

// Writes a file of n lines and returns its path.
function fileOf(n, name = `${n}-lines.txt`) {
  const p = join(dir, name);
  writeFileSync(p, n === 0 ? '' : Array.from({ length: n }, (_, i) => `line ${i + 1}`).join('\n'));
  return p;
}

// Resolves to the thrown Error, or null when the guard allowed the call.
async function run(filePath, { tool = 'write' } = {}) {
  try {
    await handler({ tool }, { args: { filePath } });
    return null;
  } catch (err) {
    return err;
  }
}

// ── The decision table ──────────────────────────────────────────────────────
//
// The guard blocks strictly above the threshold, so 500 allows and 501 blocks.
// That boundary is the whole rule; an off-by-one here either nags on every
// medium file or lets the case the guard exists for through.

const over = await run(fileOf(THRESHOLD + 1));
ok(over !== null, `${THRESHOLD + 1} lines blocks`);
ok(over?.message.includes(String(THRESHOLD + 1)), 'message names the line count', over?.message);
ok(over?.message.includes('large-file-guard'), 'message names the guard', over?.message);

ok((await run(fileOf(THRESHOLD))) === null, `${THRESHOLD} lines allows (boundary is strict)`);
ok((await run(fileOf(10))) === null, '10 lines allows');
ok((await run(fileOf(0, 'empty.txt'))) === null, 'empty file allows');
ok((await run(fileOf(5000, 'huge.txt'))) !== null, '5000 lines blocks');

// ── What the guard must not touch ───────────────────────────────────────────
//
// Write replaces a whole file; edit is targeted. The guard's own header says
// edit is exempt, and a guard that also nagged on every edit to a long file
// would be turned off within a day — which is the failure mode that matters,
// because a disabled guard guards nothing.

const big = fileOf(THRESHOLD + 1, 'exempt-check.txt');
for (const tool of ['edit', 'read', 'bash', 'patch', 'apply_patch']) {
  ok((await run(big, { tool })) === null, `tool "${tool}" is untouched even over the threshold`);
}

// A path that does not exist yet is a new file, not an overwrite — there is
// nothing to lose, so it allows. Same for a call that carries no path at all.
ok((await run(join(dir, 'does-not-exist.txt'))) === null, 'non-existent path allows (a new file is not an overwrite)');
ok((await run('')) === null, 'empty filePath allows');
ok((await run(undefined)) === null, 'missing filePath allows');

// ── A file name is data ─────────────────────────────────────────────────────
//
// No shell is involved, so these cannot execute. What is asserted is that each
// name still reaches the message intact: the guard reports the file the user is
// about to lose, and a name it mangles is a name they cannot recognise.

// A bare name, with no separator: it has to survive being used as a file name
// in this temp dir, and a path would create directories that do not exist.
const SENTINEL = 'lfg-sentinel';
const hostile = [
  ['single quote', "it's.txt"],
  ['double quote', 'say "hi".txt'],
  ['command substitution', `$(touch ${SENTINEL}).txt`],
  ['backticks', `\`touch ${SENTINEL}\`.txt`],
  ['parameter expansion', '${HOME}.txt'],
  ['backslashes', 'back\\slash.txt'],
  ['leading dash', '-rf.txt'],
  ['semicolon', 'a;b.txt'],
  ['unicode', 'ファイル.txt'],
];

for (const [label, name] of hostile) {
  const err = await run(fileOf(THRESHOLD + 1, name));
  ok(err !== null, `hostile name blocks: ${label}`);
  // basename() is what the message carries, so compare against that, not the
  // full path — on a name containing a separator the two legitimately differ.
  const expected = name.split(/[\\/]/).pop();
  ok(err?.message.includes(expected), `hostile name survives into the message: ${label}`, err?.message);
}

// Checked in both plausible landing spots: the temp dir the names live in, and
// the process cwd a bare `touch` would have written to.
ok(!existsSync(join(dir, SENTINEL)), 'no probe executed — no sentinel in the temp dir');
ok(!existsSync(SENTINEL), 'no probe executed — no sentinel in the working directory');

rmSync(dir, { recursive: true, force: true });

console.log(`${ran - fail} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
