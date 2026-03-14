/**
 * utils.js — shared utilities for devexp opencode hook modules
 */

import { execFileSync } from 'child_process';
import { existsSync, readFileSync } from 'fs';
import { join, dirname, resolve, extname, basename } from 'path';

export const LINT_EXTS = new Set([
  '.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs',
  '.py',
  '.go',
  '.rb',
]);

export function findRoot(filePath) {
  const markers = ['package.json', 'pyproject.toml', 'go.mod', 'Cargo.toml', '.git'];
  let dir = dirname(resolve(filePath));
  while (true) {
    if (markers.some(m => existsSync(join(dir, m)))) return dir;
    const parent = dirname(dir);
    if (parent === dir) return dir;
    dir = parent;
  }
}

export function which(cmd) {
  try {
    return execFileSync('which', [cmd], {
      encoding: 'utf-8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim();
  } catch {
    return null;
  }
}

export function runLinter(cmd, args, cwd) {
  try {
    const result = execFileSync(cmd, args, {
      cwd,
      timeout: 10000,
      encoding: 'utf-8',
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    if (result.trim()) console.log(`[devexp lint-on-save] ${basename(cmd)}:\n${result.trim()}`);
  } catch (e) {
    // Linters exit 1 on issues — expected, still show output
    const output = ((e.stdout ?? '') + (e.stderr ?? '')).trim();
    if (output) console.log(`[devexp lint-on-save] ${basename(cmd)}:\n${output}`);
  }
}

export function countLines(filePath) {
  try {
    return readFileSync(filePath, 'utf-8').split('\n').length;
  } catch {
    return 0;
  }
}

export { existsSync, join, dirname, resolve, extname, basename };
