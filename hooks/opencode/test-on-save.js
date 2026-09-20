/**
 * test-on-save.js — runs the associated test file after source file edits
 *
 * Event: file.edited
 * Advisory only — never throws.
 *
 * Test discovery per language:
 *   JS/TS:  <name>.test.{ts,js,tsx,jsx} or <name>.spec.{ts,js} in same dir or __tests__/
 *   Go:     go test ./<package> (package containing the edited file)
 *   Python: test_<name>.py or <name>_test.py in same dir or tests/
 *   Ruby:   spec/<path>/<name>_spec.rb
 */

import { existsSync, join, dirname, resolve, extname, basename } from './utils.js';
import { findRoot, which, runCommand, editedPath } from './utils.js';
import { relative } from 'path';

const SOURCE_EXTS = new Set(['.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs', '.py', '.go', '.rb']);
const TEST_MARKERS = ['.test.', '.spec.', '_test.', 'test_'];

function isTestFile(filePath) {
  const name = basename(filePath);
  return TEST_MARKERS.some(m => name.includes(m));
}

export const TEST_TIMEOUT_MS = 20000;

async function runTests(cmd, args, cwd) {
  const r = await runCommand(cmd, args, { cwd, timeout: TEST_TIMEOUT_MS });
  const output = (r.stdout + r.stderr).trim();
  const status = r.code === 0 ? 'PASS' : 'FAIL';
  console.log(`[devexp test-on-save] ${basename(cmd)} [${status}]`);
  if (output) console.log(output);
}

export async function testOnSave(ctx) {
  return {
    'file.edited': async (event) => {
      const filePath = editedPath(event.file ?? '', ctx?.directory);
      if (!filePath) return;

      const ext = extname(filePath).toLowerCase();
      if (!SOURCE_EXTS.has(ext)) return;
      if (isTestFile(filePath)) return;

      const root    = findRoot(filePath);
      const fileDir = dirname(resolve(filePath));
      const base    = basename(filePath, ext);

      try {
        // ── JS / TS ──────────────────────────────────────────────────────────
        if (['.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs'].includes(ext)) {
          const testExts = ['.test.ts', '.test.tsx', '.test.js', '.test.jsx',
                            '.spec.ts', '.spec.tsx', '.spec.js', '.spec.jsx'];
          const candidates = testExts.flatMap(te => [
            join(fileDir, base + te),
            join(fileDir, '__tests__', base + te),
          ]);
          const testFile = candidates.find(c => existsSync(c));
          if (!testFile) return;

          const localVitest = join(root, 'node_modules', '.bin', 'vitest');
          const localJest   = join(root, 'node_modules', '.bin', 'jest');
          // --runTestsByPath runs exactly this file: a plain pattern is a regex that
          // can match other tests or none. It works on jest 29 and 30 (30 renamed
          // --testPathPattern). After '--' a path starting with '-' is never read as
          // options, and it stays relative to root: an absolute path through a
          // symlinked root finds no tests. vitest gets no '--': it would drop
          // the file filter.
          const jestArgs    = ['--passWithNoTests', '--no-coverage', '--runTestsByPath', '--', relative(root, testFile)];

          if (existsSync(localVitest)) {
            await runTests(localVitest, ['run', testFile], root);
          } else if (existsSync(localJest)) {
            await runTests(localJest, jestArgs, root);
          } else if (await which('vitest')) {
            await runTests('vitest', ['run', testFile], root);
          } else if (await which('jest')) {
            await runTests('jest', jestArgs, root);
          }

        // ── Go ───────────────────────────────────────────────────────────────
        } else if (ext === '.go') {
          const go = await which('go');
          if (!go) return;
          const relDir = relative(root, fileDir);
          const pkg    = relDir === '' ? './...' : `./${relDir}`;
          await runTests(go, ['test', '-timeout', '20s', pkg], root);

        // ── Python ───────────────────────────────────────────────────────────
        } else if (ext === '.py') {
          const pytest = await which('pytest');
          if (!pytest) return;
          const candidates = [
            join(fileDir, `test_${base}.py`),
            join(fileDir, `${base}_test.py`),
            join(root, 'tests', `test_${base}.py`),
            join(root, 'tests', `${base}_test.py`),
          ];
          const testFile = candidates.find(c => existsSync(c));
          if (!testFile) return;
          await runTests(pytest, [testFile, '-x', '-q'], root);

        // ── Ruby ─────────────────────────────────────────────────────────────
        } else if (ext === '.rb') {
          const rspec = await which('rspec');
          if (!rspec) return;
          const rel      = relative(root, filePath);
          const specRel  = rel.replace(/^lib\//, 'spec/').replace(/\.rb$/, '_spec.rb');
          const specPath = join(root, specRel);
          if (!existsSync(specPath)) return;
          await runTests(rspec, [specPath, '--format', 'progress'], root);
        }
      } catch {
        // Advisory — never propagate errors from file.edited
      }
    },
  };
}
