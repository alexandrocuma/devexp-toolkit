/**
 * Tests for devexp-plugin.js — the data-driven opencode entry.
 * Run: node hooks/opencode/devexp-plugin.test.js
 *
 * Each case builds the installed layout in a temp dir and imports the entry
 * from there, exactly as opencode would:
 *
 *   <tmp>/package.json            {"type":"module"} so the entry loads as ESM
 *   <tmp>/plugins/devexp.js       copy of devexp-plugin.js
 *   <tmp>/plugins/devexp/         selected modules, utils.js, package.json,
 *                                 probe modules and hooks.json
 */
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, unlinkSync, writeFileSync } from 'fs';
import { tmpdir } from 'os';
import { dirname, join } from 'path';
import { fileURLToPath, pathToFileURL } from 'url';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = join(HERE, '..', '..');
const DOTENV = '.env';

let pass = 0;
let fail = 0;
function check(name, ok, detail = '') {
  if (ok) { pass++; return; }
  fail++;
  console.log(`FAIL ${name}${detail ? ` — ${detail}` : ''}`);
}

async function rejection(promise) {
  try { await promise; return null; } catch (e) { return e; }
}

// Collects console.error output so skipped-module logs don't clutter the run.
async function quietly(fn) {
  const logged = [];
  const orig = console.error;
  console.error = (...args) => logged.push(args.join(' '));
  try { return { result: await fn(), logged }; } finally { console.error = orig; }
}

// ── Probe modules ────────────────────────────────────────────────────────────
// Each records on globalThis.__probe so a case can see what ran.

const PROBES = {
  'probe-count.js': `
    export async function probeCount(_ctx) {
      (globalThis.__probe.count ??= { init: 0, before: [], edited: [] }).init++;
      return {
        'tool.execute.before': async (input) => { globalThis.__probe.count.before.push(input.tool); },
        'file.edited': async (event) => { globalThis.__probe.count.edited.push(event); },
      };
    }`,
  'probe-unlisted.js': `
    globalThis.__probe.unlistedImported = true;
    export async function probeUnlisted(_ctx) {
      globalThis.__probe.unlistedCalled = true;
      return { 'tool.execute.before': async () => { globalThis.__probe.unlistedCalled = true; } };
    }`,
  'probe-throw-import.js': `
    throw new Error('probe exploded at import');
    export async function probeThrowImport(_ctx) { return {}; }`,
  'probe-throw-init.js': `
    export async function probeThrowInit(_ctx) { throw new Error('probe exploded at init'); }`,
  'probe-throw-edited.js': `
    export async function probeThrowEdited(_ctx) {
      return { 'file.edited': async () => { throw new Error('probe exploded on edit'); } };
    }`,
};

const probe = (file, exp, failClosed = false) => ({ name: file.replace(/\.js$/, ''), module: file, export: exp, failClosed });
const real = (name, exp, failClosed = false) => ({ name, module: `${name}.js`, export: exp, failClosed });

// ── Fixture ──────────────────────────────────────────────────────────────────

let seq = 0;
const trees = [];

// hooksJson: an array is written as hooks.json, a string is written verbatim,
// null leaves hooks.json out.
function buildTree(hooksJson) {
  const root = mkdtempSync(join(tmpdir(), 'devexp-plugin-test-'));
  trees.push(root);
  const plugins = join(root, 'plugins');
  const devexp = join(plugins, 'devexp');
  mkdirSync(devexp, { recursive: true });

  writeFileSync(join(root, 'package.json'), '{"type":"module"}\n');
  copyFileSync(join(HERE, 'devexp-plugin.js'), join(plugins, 'devexp.js'));
  for (const f of ['utils.js', 'package.json']) copyFileSync(join(HERE, f), join(devexp, f));

  const entries = Array.isArray(hooksJson) ? hooksJson : [];
  for (const e of entries) {
    const src = join(HERE, e.module ?? '');
    if (typeof e.module === 'string' && !e.module.includes('/') && existsSync(src) && !PROBES[e.module]) {
      copyFileSync(src, join(devexp, e.module));
    }
  }
  for (const [file, source] of Object.entries(PROBES)) writeFileSync(join(devexp, file), source);
  // Sits one level up; only a path escape could reach it.
  writeFileSync(join(plugins, 'evil.js'), `globalThis.__probe.evil = true;\nexport async function evil() { return {}; }\n`);

  if (Array.isArray(hooksJson)) writeFileSync(join(devexp, 'hooks.json'), JSON.stringify(hooksJson));
  else if (typeof hooksJson === 'string') writeFileSync(join(devexp, 'hooks.json'), hooksJson);

  return { root, devexp, entry: join(plugins, 'devexp.js') };
}

async function loadPlugin(tree) {
  const mod = await import(`${pathToFileURL(tree.entry).href}?n=${++seq}`);
  return { mod, hooks: await mod.DevExpPlugin({}) };
}

function reset() { globalThis.__probe = {}; }

const READ_ENV = [{ tool: 'read' }, { args: { filePath: `/p/${DOTENV}` } }];
const BENIGN = [{ tool: 'bash' }, { args: { command: 'ls' } }];

// ── 1. Loader shape ──────────────────────────────────────────────────────────
{
  reset();
  const tree = buildTree([probe('probe-count.js', 'probeCount')]);
  const { mod, hooks } = await loadPlugin(tree);
  const exports = Object.keys(mod);
  check('1 entry has exactly one export', exports.length === 1, `got ${exports}`);
  check('1 every export is a function', exports.every((k) => typeof mod[k] === 'function'));
  check('1 hook object keys are tool.execute.before + event',
    JSON.stringify(Object.keys(hooks).sort()) === JSON.stringify(['event', 'tool.execute.before']),
    `got ${Object.keys(hooks)}`);
}

// ── 2. Composition from the selection ────────────────────────────────────────
{
  reset();
  const tree = buildTree([probe('probe-count.js', 'probeCount')]);
  const { hooks } = await loadPlugin(tree);
  await hooks['tool.execute.before'](...BENIGN);
  check('2 listed probe is initialised once', globalThis.__probe.count?.init === 1);
  check('2 listed probe runs on tool.execute.before', globalThis.__probe.count?.before?.[0] === 'bash');
  check('2 unlisted probe in devexp/ is never imported', globalThis.__probe.unlistedImported === undefined);
  check('2 unlisted probe in devexp/ is never called', globalThis.__probe.unlistedCalled === undefined);
}

// ── 3. Parity with Claude Code's message (D3) ────────────────────────────────
{
  reset();
  const want = `[devexp secret-guard] Blocked access to "${DOTENV}". This file may contain secrets. If intentional, confirm with the user first.`;
  const tree = buildTree([real('secret-guard', 'secretGuard', true)]);
  const { hooks } = await loadPlugin(tree);
  const readErr = await rejection(hooks['tool.execute.before'](...READ_ENV));
  check('3 read of .env blocks with the Claude Code message', readErr?.message === want, `got ${readErr?.message}`);
  const bashErr = await rejection(hooks['tool.execute.before']({ tool: 'bash' }, { args: { command: `cat ${DOTENV}` } }));
  check('3 bash access to .env blocks with the same message', bashErr?.message === want, `got ${bashErr?.message}`);
}

// ── 4. Failure isolation ─────────────────────────────────────────────────────
{
  reset();
  const tree = buildTree([
    probe('probe-throw-import.js', 'probeThrowImport'),
    real('dangerous-cmd-guard', 'dangerousCmdGuard', true),
  ]);
  const { result, logged } = await quietly(async () => rejection(loadPlugin(tree)));
  check('4 factory resolves despite a module failing to import', result === null, `got ${result?.message}`);
  check('4 skipped module is logged', logged.some((l) => l.includes('[devexp probe-throw-import] failed to load; skipped: probe exploded at import')), `got ${logged}`);
  const { hooks } = (await quietly(() => loadPlugin(tree))).result;
  const err = await rejection(hooks['tool.execute.before']({ tool: 'bash' }, { args: { command: 'rm -rf /' } }));
  check('4 the other guard still blocks', err?.message?.startsWith('[devexp dangerous-cmd-guard] Blocked:'), `got ${err?.message}`);
  check('4 a failOpen load failure does not block benign calls', (await rejection(hooks['tool.execute.before'](...BENIGN))) === null);
}

// ── 5. Fail-closed stub ──────────────────────────────────────────────────────
{
  for (const [file, exp] of [['probe-throw-init.js', 'probeThrowInit'], ['probe-throw-import.js', 'probeThrowImport']]) {
    reset();
    const name = file.replace(/\.js$/, '');
    const tree = buildTree([probe(file, exp, true), probe('probe-count.js', 'probeCount')]);
    const { hooks } = await loadPlugin(tree);
    const err = await rejection(hooks['tool.execute.before'](...BENIGN));
    check(`5 ${name} with failClosed blocks every tool call`,
      err?.message?.startsWith(`[devexp ${name}] internal error — the guard failed to load`), `got ${err?.message}`);
    check(`5 ${name} stub message carries the load error`, err?.message?.includes('probe exploded at'));
  }
  reset();
  const tree = buildTree([{ name: 'bad-export', module: 'probe-count.js', export: 'noSuchExport', failClosed: true }]);
  const { hooks } = await loadPlugin(tree);
  const err = await rejection(hooks['tool.execute.before'](...BENIGN));
  check('5 a missing export with failClosed blocks', err?.message?.includes('[devexp bad-export] internal error') && err.message.includes('export noSuchExport is not a function'), `got ${err?.message}`);
}

// ── 6. Missing or malformed hooks.json ───────────────────────────────────────
{
  for (const [label, content] of [['missing', null], ['an object', '{}'], ['invalid JSON', 'not json']]) {
    reset();
    const tree = buildTree(content);
    if (content === null && existsSync(join(tree.devexp, 'hooks.json'))) unlinkSync(join(tree.devexp, 'hooks.json'));
    const { hooks } = await loadPlugin(tree);
    const err = await rejection(hooks['tool.execute.before'](...BENIGN));
    check(`6 hooks.json ${label} blocks tool calls`, err?.message?.includes('misconfigured'), `got ${err?.message}`);
    check(`6 hooks.json ${label} returns only tool.execute.before`,
      JSON.stringify(Object.keys(hooks)) === JSON.stringify(['tool.execute.before']));
  }
}

// ── 7. Path escape ───────────────────────────────────────────────────────────
{
  reset();
  let tree = buildTree([{ name: 'evil', module: '../evil.js', export: 'evil', failClosed: true }]);
  let { hooks } = await loadPlugin(tree);
  let err = await rejection(hooks['tool.execute.before'](...BENIGN));
  check('7 ../ module with failClosed blocks', err?.message?.startsWith('[devexp evil] internal error'), `got ${err?.message}`);

  tree = buildTree([
    { name: 'evil', module: '../evil.js', export: 'evil', failClosed: false },
    { name: 'evil-sub', module: 'sub/evil.js', export: 'evil', failClosed: false },
    probe('probe-count.js', 'probeCount'),
  ]);
  const { result, logged } = await quietly(() => loadPlugin(tree));
  hooks = result.hooks;
  err = await rejection(hooks['tool.execute.before'](...BENIGN));
  check('7 ../ module without failClosed is skipped', err === null, `got ${err?.message}`);
  check('7 skipped escapes are logged', logged.filter((l) => l.includes('is not a bare file name')).length === 2, `got ${logged}`);
  check('7 the rest of the selection still runs', globalThis.__probe.count?.before?.length === 1);
  check('7 evil.js is never imported', globalThis.__probe.evil === undefined);
}

// ── 8. event → file.edited adaptation (D4) ───────────────────────────────────
{
  reset();
  const tree = buildTree([probe('probe-throw-edited.js', 'probeThrowEdited'), probe('probe-count.js', 'probeCount')]);
  const { hooks } = await loadPlugin(tree);

  const { result, logged } = await quietly(() => rejection(
    hooks.event({ event: { id: '1', type: 'file.edited', properties: { file: '/x/a.ts' } } })));
  check('8 a throwing file.edited handler does not reject event', result === null, `got ${result?.message}`);
  check('8 the throwing handler is logged', logged.some((l) => l.includes('[devexp probe-throw-edited] file.edited failed: probe exploded on edit')));
  check('8 file.edited delivers { file } to the module',
    JSON.stringify(globalThis.__probe.count?.edited) === JSON.stringify([{ file: '/x/a.ts' }]),
    `got ${JSON.stringify(globalThis.__probe.count?.edited)}`);

  await hooks.event({ event: { id: '2', type: 'session.idle', properties: {} } });
  check('8 other event types are ignored', globalThis.__probe.count.edited.length === 1);
  check('8 an event with no payload does not reject', (await rejection(hooks.event({}))) === null);
}

// ── 9. Real file.edited modules load and are wired ───────────────────────────
{
  reset();
  const onSave = [real('lint-on-save', 'lintOnSave'), real('format-on-save', 'formatOnSave'), real('test-on-save', 'testOnSave')];
  const tree = buildTree([...onSave, probe('probe-count.js', 'probeCount')]);
  const { result, logged } = await quietly(() => loadPlugin(tree));
  check('9 on-save modules load without errors', logged.length === 0, `got ${logged}`);

  for (const e of onSave) {
    const mod = await import(pathToFileURL(join(tree.devexp, e.module)).href);
    const keys = Object.keys(await mod[e.export]({}));
    check(`9 ${e.name} registers a file.edited handler`, keys.includes('file.edited'), `got ${keys}`);
  }

  // Unknown extension: every on-save module returns before running a tool.
  const scratch = mkdtempSync(join(tmpdir(), 'devexp-plugin-edit-'));
  trees.push(scratch);
  const file = join(scratch, 'notes.devexp-unknown');
  writeFileSync(file, 'x\n');
  const edited = await quietly(() => rejection(
    result.hooks.event({ event: { type: 'file.edited', properties: { file } } })));
  check('9 file.edited reaches the on-save modules without rejecting', edited.result === null && edited.logged.length === 0, `got ${edited.logged}`);
  check('9 dispatch continues past the on-save modules', globalThis.__probe.count?.edited?.[0]?.file === file);
}

// ── 10. Registry consistency ─────────────────────────────────────────────────
{
  const registry = JSON.parse(readFileSync(join(REPO, 'hooks', 'registry.json'), 'utf8'));
  const FAIL_CLOSED = ['dangerous-cmd-guard', 'secret-guard', 'secret-in-write-guard'];
  const GRAPHIFY = ['graphify-grep-nudge', 'graphify-read-guard', 'graphify-session-sentinel'];

  check('10 registry has 10 hooks', registry.length === 10, `got ${registry.length}`);
  for (const h of registry) {
    const oc = h.opencode ?? {};
    const mod = oc.module ?? '';
    const ok = mod.startsWith('hooks/opencode/') && mod.endsWith('.js') && !mod.endsWith('.test.js') &&
      !mod.slice('hooks/opencode/'.length).includes('/') && existsSync(join(REPO, mod));
    check(`10 ${h.name} opencode.module is an existing hooks/opencode module`, ok, `got ${mod}`);
    if (!ok) continue;

    const exported = (await import(pathToFileURL(join(REPO, mod)).href))[oc.export];
    check(`10 ${h.name} opencode.export is a function`, typeof exported === 'function', `export ${oc.export}`);

    const relImports = [...readFileSync(join(REPO, mod), 'utf8').matchAll(/from '(\.\/[^']+)'/g)].map((m) => m[1]);
    check(`10 ${h.name} imports only ./utils.js relatively`, relImports.every((p) => p === './utils.js'), `got ${relImports}`);
  }
  const failClosed = registry.filter((h) => h.opencode?.fail_closed === true).map((h) => h.name).sort();
  check('10 exactly the security guards are fail_closed', JSON.stringify(failClosed) === JSON.stringify(FAIL_CLOSED), `got ${failClosed}`);
  for (const name of GRAPHIFY) {
    const h = registry.find((x) => x.name === name);
    check(`10 ${name} is enabled for opencode`, h?.opencode?.enabled === true);
  }
}

for (const dir of trees) rmSync(dir, { recursive: true, force: true });

console.log(`${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
