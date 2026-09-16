/**
 * Tests that the on-save modules hand the edited path to each tool as a path (#121).
 * Run: node hooks/opencode/on-save-path.test.js
 *
 * Mirrors the "Tool argv" part of hooks/claude-code/on-save-path.test.sh.
 * A relative path that starts with '-' must never reach a tool as an option.
 * For every tool the modules call, each case pins the exact argv for:
 *   abs   the file named by an absolute path: argv exactly as at v0.8.0
 *   dash  the file named by a relative path starting with '-' (cwd = project):
 *         the path must arrive './'-prefixed or absolute. jest gets its
 *         pattern bound with '=' instead.
 *
 * Each case is a fresh project laid out so the module takes one tool branch.
 * The tool is a stub that records its argv, and PATH holds only the stubs and
 * `which`.
 */
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'fs';
import { tmpdir } from 'os';
import { dirname, join } from 'path';
import { fileURLToPath, pathToFileURL } from 'url';

const HERE = dirname(fileURLToPath(import.meta.url));

let pass = 0;
let fail = 0;
function check(name, ok, detail = '') {
  if (ok) { pass++; return; }
  fail++;
  console.log(`FAIL ${name}${detail ? ` — ${detail}` : ''}`);
}

const TMP = realpathSync(mkdtempSync(join(tmpdir(), 'devexp-on-save-path-')));
const STUB_LOG = join(TMP, 'calls.log');

// The only non-stub command a module runs is `which`.
const BASE = join(TMP, 'base');
mkdirSync(BASE);
const whichBin = ['/usr/bin/which', '/bin/which'].find((p) => existsSync(p));
writeFileSync(join(BASE, 'which'), `#!/bin/sh\nexec '${whichBin}' "$@"\n`);
chmodSync(join(BASE, 'which'), 0o755);

// Records one line per call: the tool's name, then each argument in [brackets].
const ARGV_STUB = `#!/bin/sh
{ printf "%s" "\${0##*/}"; for a in "$@"; do printf " [%s]" "$a"; done; printf "\\n"; } >> "$STUB_LOG"
`;

const modules = {
  'format-on-save': (await import(pathToFileURL(join(HERE, 'format-on-save.js')).href)).formatOnSave,
  'lint-on-save': (await import(pathToFileURL(join(HERE, 'lint-on-save.js')).href)).lintOnSave,
  'test-on-save': (await import(pathToFileURL(join(HERE, 'test-on-save.js')).href)).testOnSave,
};

let nCase = 0;
let P;
let BIN;
// setup(...entries) — a new project P and tool dir BIN. An entry bin/<tool> or
// node_modules/.bin/<tool> is a stub; any other entry is an empty file.
function setup(...entries) {
  nCase++;
  P = join(TMP, `case${nCase}`, 'project');
  BIN = join(TMP, `case${nCase}`, 'bin');
  mkdirSync(P, { recursive: true });
  mkdirSync(BIN, { recursive: true });
  writeFileSync(join(P, 'pyproject.toml'), '');
  for (const e of entries) {
    const f = e.startsWith('bin/') ? join(BIN, e.slice(4)) : join(P, e);
    mkdirSync(dirname(f), { recursive: true });
    if (e.startsWith('bin/') || e.startsWith('node_modules/.bin/')) {
      writeFileSync(f, ARGV_STUB);
      chmodSync(f, 0o755);
    } else {
      writeFileSync(f, '');
    }
  }
}

// want(hook, label, path, argv) — run the module on path in P; the tool calls must be argv.
async function want(hook, label, file, argv) {
  writeFileSync(STUB_LOG, '');
  const saved = { path: process.env.PATH, log: process.env.STUB_LOG, cwd: process.cwd(), consoleLog: console.log };
  process.env.PATH = `${BIN}:${BASE}`;
  process.env.STUB_LOG = STUB_LOG;
  process.chdir(P);
  console.log = () => {};
  try {
    const handlers = await modules[hook]({});
    await handlers['file.edited']({ file });
  } finally {
    process.env.PATH = saved.path;
    if (saved.log === undefined) delete process.env.STUB_LOG; else process.env.STUB_LOG = saved.log;
    process.chdir(saved.cwd);
    console.log = saved.consoleLog;
  }
  const got = readFileSync(STUB_LOG, 'utf8').replace(/\n$/, '');
  check(`${hook} ${label}`, got === argv, `\n  want: ${argv}\n  got:  ${got}`);
}

const JS = ['mod.js', '-mod.js'];
const PY = ['mod.py', '-mod.py'];
const GO = ['mod.go', '-mod.go'];
const RB = ['mod.rb', '-mod.rb'];

// ── format-on-save
setup('biome.json', 'node_modules/.bin/biome', ...JS);
await want('format-on-save', 'local biome abs', `${P}/mod.js`, `biome [format] [--write] [${P}/mod.js]`);
await want('format-on-save', 'local biome dash', '-mod.js', 'biome [format] [--write] [./-mod.js]');
setup('node_modules/.bin/prettier', ...JS);
await want('format-on-save', 'local prettier abs', `${P}/mod.js`, `prettier [--write] [${P}/mod.js]`);
await want('format-on-save', 'local prettier dash', '-mod.js', 'prettier [--write] [./-mod.js]');
setup('biome.json', 'bin/biome', ...JS);
await want('format-on-save', 'biome abs', `${P}/mod.js`, `biome [format] [--write] [${P}/mod.js]`);
await want('format-on-save', 'biome dash', '-mod.js', 'biome [format] [--write] [./-mod.js]');
setup('bin/prettier', ...JS);
await want('format-on-save', 'prettier abs', `${P}/mod.js`, `prettier [--write] [${P}/mod.js]`);
await want('format-on-save', 'prettier dash', '-mod.js', 'prettier [--write] [./-mod.js]');
setup('bin/ruff', ...PY);
await want('format-on-save', 'ruff abs', `${P}/mod.py`, `ruff [format] [${P}/mod.py]`);
await want('format-on-save', 'ruff dash', '-mod.py', 'ruff [format] [./-mod.py]');
setup('bin/black', ...PY);
await want('format-on-save', 'black abs', `${P}/mod.py`, `black [--quiet] [${P}/mod.py]`);
await want('format-on-save', 'black dash', '-mod.py', 'black [--quiet] [./-mod.py]');
setup('bin/gofmt', ...GO);
await want('format-on-save', 'gofmt abs', `${P}/mod.go`, `gofmt [-w] [${P}/mod.go]`);
await want('format-on-save', 'gofmt dash', '-mod.go', 'gofmt [-w] [./-mod.go]');
setup('bin/rubocop', ...RB);
await want('format-on-save', 'rubocop abs', `${P}/mod.rb`, `rubocop [--autocorrect-all] [--no-color] [--format] [quiet] [${P}/mod.rb]`);
await want('format-on-save', 'rubocop dash', '-mod.rb', 'rubocop [--autocorrect-all] [--no-color] [--format] [quiet] [./-mod.rb]');

// ── lint-on-save
setup('biome.json', 'node_modules/.bin/biome', ...JS);
await want('lint-on-save', 'local biome abs', `${P}/mod.js`, `biome [lint] [${P}/mod.js]`);
await want('lint-on-save', 'local biome dash', '-mod.js', 'biome [lint] [./-mod.js]');
setup('node_modules/.bin/eslint', ...JS);
await want('lint-on-save', 'local eslint abs', `${P}/mod.js`, `eslint [--max-warnings=0] [--no-warn-ignored] [${P}/mod.js]`);
await want('lint-on-save', 'local eslint dash', '-mod.js', 'eslint [--max-warnings=0] [--no-warn-ignored] [./-mod.js]');
setup('biome.json', 'bin/biome', ...JS);
await want('lint-on-save', 'biome abs', `${P}/mod.js`, `biome [lint] [${P}/mod.js]`);
await want('lint-on-save', 'biome dash', '-mod.js', 'biome [lint] [./-mod.js]');
setup('bin/eslint', ...JS);
await want('lint-on-save', 'eslint abs', `${P}/mod.js`, `eslint [--max-warnings=0] [${P}/mod.js]`);
await want('lint-on-save', 'eslint dash', '-mod.js', 'eslint [--max-warnings=0] [./-mod.js]');
setup('bin/ruff', ...PY);
await want('lint-on-save', 'ruff abs', `${P}/mod.py`, `ruff [check] [${P}/mod.py]`);
await want('lint-on-save', 'ruff dash', '-mod.py', 'ruff [check] [./-mod.py]');
setup('bin/flake8', ...PY);
await want('lint-on-save', 'flake8 abs', `${P}/mod.py`, `flake8 [${P}/mod.py]`);
await want('lint-on-save', 'flake8 dash', '-mod.py', 'flake8 [./-mod.py]');
setup('bin/go', 'mod.go', '-pkg/mod.go');
await want('lint-on-save', 'go vet abs', `${P}/mod.go`, 'go [vet] [./...]');
await want('lint-on-save', 'go vet dash', '-pkg/mod.go', 'go [vet] [./-pkg]');
setup('bin/rubocop', ...RB);
await want('lint-on-save', 'rubocop abs', `${P}/mod.rb`, `rubocop [--no-color] [--format] [simple] [${P}/mod.rb]`);
await want('lint-on-save', 'rubocop dash', '-mod.rb', 'rubocop [--no-color] [--format] [simple] [./-mod.rb]');

// ── test-on-save
const JST = [...JS, 'mod.test.js', '-mod.test.js'];
setup('node_modules/.bin/vitest', ...JST);
await want('test-on-save', 'local vitest abs', `${P}/mod.js`, `vitest [run] [${P}/mod.test.js]`);
await want('test-on-save', 'local vitest dash', '-mod.js', `vitest [run] [${P}/-mod.test.js]`);
setup('node_modules/.bin/jest', ...JST);
await want('test-on-save', 'local jest abs', `${P}/mod.js`, 'jest [--testPathPattern=mod.test.js] [--passWithNoTests] [--no-coverage]');
await want('test-on-save', 'local jest dash', '-mod.js', 'jest [--testPathPattern=-mod.test.js] [--passWithNoTests] [--no-coverage]');
setup('bin/vitest', ...JST);
await want('test-on-save', 'vitest abs', `${P}/mod.js`, `vitest [run] [${P}/mod.test.js]`);
await want('test-on-save', 'vitest dash', '-mod.js', `vitest [run] [${P}/-mod.test.js]`);
setup('bin/jest', ...JST);
await want('test-on-save', 'jest abs', `${P}/mod.js`, 'jest [--testPathPattern=mod.test.js] [--passWithNoTests] [--no-coverage]');
await want('test-on-save', 'jest dash', '-mod.js', 'jest [--testPathPattern=-mod.test.js] [--passWithNoTests] [--no-coverage]');
setup('bin/go', 'mod.go', '-pkg/mod.go');
await want('test-on-save', 'go test abs', `${P}/mod.go`, 'go [test] [-timeout] [20s] [./...]');
await want('test-on-save', 'go test dash', '-pkg/mod.go', 'go [test] [-timeout] [20s] [./-pkg]');
setup('bin/pytest', ...PY, 'test_mod.py', 'test_-mod.py');
await want('test-on-save', 'pytest abs', `${P}/mod.py`, `pytest [${P}/test_mod.py] [-x] [-q]`);
await want('test-on-save', 'pytest dash', '-mod.py', `pytest [${P}/test_-mod.py] [-x] [-q]`);
setup('bin/rspec', ...RB, 'mod_spec.rb', '-mod_spec.rb');
await want('test-on-save', 'rspec abs', `${P}/mod.rb`, `rspec [${P}/mod_spec.rb] [--format] [progress]`);
await want('test-on-save', 'rspec dash', '-mod.rb', `rspec [${P}/-mod_spec.rb] [--format] [progress]`);

rmSync(TMP, { recursive: true, force: true });

console.log(`${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
