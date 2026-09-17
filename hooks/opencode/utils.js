/**
 * utils.js — shared utilities for devexp opencode hook modules
 */

import { spawn } from 'child_process';
import { existsSync, readFileSync } from 'fs';
import { join, dirname, resolve, extname, basename, isAbsolute } from 'path';

/**
 * The scan budget shared by the fail-closed security guards (#162).
 *
 * opencode loads plugins with a plain dynamic import inside its server process
 * and puts no timeout on a hook, so a slow scan stalls the session and a hook
 * that never returns hangs the tool call for good. The budget turns that into a
 * refusal: the deadline is checked at every unit of the scan, and the check
 * throws, which is how a `tool.execute.before` handler refuses a call.
 *
 * A single regex call is not interruptible in JavaScript, so the granularity is
 * one unit of work — one pattern, one rule against one line, one token. What
 * the budget rules out here is a scan that drags on, never a call that proceeds
 * unscanned: this side has no fail-open path to begin with.
 *
 * DEVEXP_SCAN_BUDGET_MS overrides the default, in milliseconds. 0 means
 * "already over budget" and blocks at the first check; anything that is not a
 * non-negative integer is ignored in favour of the default.
 * hooks/claude-code/scan-budget.sh reads the same variable and carries the same
 * default, so both twins decide alike.
 */
export const SCAN_BUDGET_DEFAULT_MS = 15000;

export function scanBudgetMs(env = process.env) {
  const raw = env?.DEVEXP_SCAN_BUDGET_MS;
  return typeof raw === 'string' && /^\d+$/.test(raw.trim()) ? Number(raw.trim()) : SCAN_BUDGET_DEFAULT_MS;
}

/** The budget in seconds, spelled as the Claude Code twin's `%.4g` spells it. */
function budgetSeconds(ms) {
  return String(Number((ms / 1000).toPrecision(4)));
}

/**
 * startScanBudget — begin a guard's budget and return the check to call at each
 * unit of the scan. The check throws once the budget is spent, with the message
 * the Claude Code twin prints.
 */
export function startScanBudget(guard, env = process.env) {
  const ms = scanBudgetMs(env);
  const deadline = performance.now() + ms;
  return () => {
    if (performance.now() >= deadline) {
      throw new Error(
        `[devexp ${guard}] Blocked: the scan did not finish within its ${budgetSeconds(ms)} s budget, ` +
        `so this input was not fully checked. Blocking to be safe.`
      );
    }
  };
}

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

/**
 * editedPath — the edited file as an absolute path.
 *
 * opencode sends an absolute path. A relative one is resolved the way
 * opencode's edit tool resolves it: against the session directory
 * (ctx.directory), else the process cwd, normalised by path.join as the
 * Claude Code hooks do. Tools run from the project root, so they get the
 * absolute path, which no tool reads as an option (#121). An
 * absolute path passes unchanged.
 */
export function editedPath(file, directory) {
  if (!file || isAbsolute(file)) return file;
  return join(typeof directory === 'string' && isAbsolute(directory) ? directory : process.cwd(), file);
}

/**
 * runCommand — run a command without blocking the event loop.
 *
 * opencode runs plugins inside its server process, so a synchronous spawn
 * freezes every session for as long as the tool runs. This is the async
 * equivalent of the execFileSync/spawnSync calls the on-save hooks used:
 * stdin is closed, stdout/stderr are collected as utf-8 (or ignored with
 * `output: 'ignore'`), and `timeout` kills the child with SIGTERM.
 *
 * Never rejects. Resolves { code, signal, stdout, stderr, error }:
 *   code    exit code, or null if the child was killed or never started
 *   signal  the killing signal (e.g. 'SIGTERM' on timeout), else null
 *   error   the spawn error (e.g. code 'ENOENT'), else null
 */
export function runCommand(cmd, args, { cwd, timeout, output = 'pipe' } = {}) {
  return new Promise((resolvePromise) => {
    let stdout = '';
    let stderr = '';
    let settled = false;
    const done = (result) => {
      if (settled) return;
      settled = true;
      resolvePromise({ stdout, stderr, error: null, ...result });
    };

    let child;
    try {
      child = spawn(cmd, args, {
        cwd,
        timeout,
        killSignal: 'SIGTERM',
        stdio: ['ignore', output, output],
      });
    } catch (error) {
      done({ code: null, signal: null, error });
      return;
    }
    child.stdout?.setEncoding('utf-8').on('data', (d) => { stdout += d; });
    child.stderr?.setEncoding('utf-8').on('data', (d) => { stderr += d; });
    child.on('error', (error) => done({ code: null, signal: null, error }));
    child.on('close', (code, signal) => done({ code, signal }));
  });
}

/** Resolves the absolute path of `cmd` on PATH, or null. */
export async function which(cmd) {
  const r = await runCommand('which', [cmd]);
  return r.error || r.code !== 0 ? null : r.stdout.trim();
}

export const LINT_TIMEOUT_MS = 10000;

export async function runLinter(cmd, args, cwd) {
  const r = await runCommand(cmd, args, { cwd, timeout: LINT_TIMEOUT_MS });
  if (!r.error && r.code === 0) {
    if (r.stdout.trim()) console.log(`[devexp lint-on-save] ${basename(cmd)}:\n${r.stdout.trim()}`);
    return;
  }
  // Linters exit 1 on issues — expected, still show output
  const output = (r.stdout + r.stderr).trim();
  if (output) console.log(`[devexp lint-on-save] ${basename(cmd)}:\n${output}`);
}

export function countLines(filePath) {
  try {
    return readFileSync(filePath, 'utf-8').split('\n').length;
  } catch {
    return 0;
  }
}

export { existsSync, join, dirname, resolve, extname, basename };
