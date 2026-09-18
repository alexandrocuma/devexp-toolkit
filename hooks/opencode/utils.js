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
 * refusal: the deadline is checked as the scan runs, and the check throws, which
 * is how a `tool.execute.before` handler refuses a call.
 *
 * A single regex call is not interruptible in JavaScript, so the granularity is
 * one unit of work. What the budget rules out here is a scan that drags on,
 * never a call that proceeds unscanned: this side has no fail-open path.
 *
 * **How often to check.** Reading the clock is not free — on an 800k-token
 * command, checking per token costs about 9% over sampling. So the rule across
 * the guards is one rule, not two: check every unit when units are few and each
 * is expensive (one regex over the whole write: 11 of them), and sample every
 * 1024 when units are many and each is cheap (one token, one line of a command
 * that may hold hundreds of thousands). Both keep the overshoot to one unit of
 * work; the sampled ones keep the clock off the hot path.
 *
 * DEVEXP_SCAN_BUDGET_MS overrides the default, in milliseconds, up to
 * SCAN_BUDGET_MAX_MS. 0 means "already over budget" and blocks at the first
 * check; anything that is not a plain ASCII non-negative integer is ignored in
 * favour of the default. hooks/claude-code/scan-budget.sh reads the same
 * variable and carries the same default, ceiling and spelling, so both twins
 * decide alike.
 */
export const SCAN_BUDGET_DEFAULT_MS = 15000;

/**
 * The ceiling, in milliseconds — the same one the Claude Code twin enforces,
 * where a budget above the registered hook timeout would let Claude Code cancel
 * the guard before it could block. opencode has no such timeout, so here the
 * ceiling is about the session: a ten-minute budget means a tool call that can
 * stall for ten minutes. Keeping one number for both twins also keeps them
 * deciding alike on the same input.
 */
export const SCAN_BUDGET_MAX_MS = 44000;

/** The refusal a spent budget throws — distinguishable from a guard's own findings. */
export class ScanBudgetError extends Error {
  constructor(message) {
    super(message);
    this.name = 'ScanBudgetError';
  }
}

/** `\d` is ASCII-only in JavaScript; the Python twin spells it `[0-9]` for the same reason. */
export function scanBudgetMs(env = process.env) {
  const raw = env?.DEVEXP_SCAN_BUDGET_MS;
  const ms = typeof raw === 'string' && /^\d+$/.test(raw.trim()) ? Number(raw.trim()) : SCAN_BUDGET_DEFAULT_MS;
  return Math.min(ms, SCAN_BUDGET_MAX_MS);
}

/** The budget in seconds, spelled as the Claude Code twin's `%.4g` spells it. */
function budgetSeconds(ms) {
  return String(Number((ms / 1000).toPrecision(4)));
}

// The clamp is said out loud, but once per process: this runs on every tool
// call, and a notice per call would bury the guards' own messages.
let clampNoticed = false;

/**
 * startScanBudget — begin a guard's budget and return the check to call as the
 * scan runs. The check throws once the budget is spent, with the message the
 * Claude Code twin prints.
 */
export function startScanBudget(guard, env = process.env) {
  const ms = scanBudgetMs(env);
  const raw = env?.DEVEXP_SCAN_BUDGET_MS;
  if (!clampNoticed && typeof raw === 'string' && /^\d+$/.test(raw.trim()) && Number(raw.trim()) > SCAN_BUDGET_MAX_MS) {
    clampNoticed = true;
    console.error(
      `[devexp ${guard}] Note: DEVEXP_SCAN_BUDGET_MS of ${budgetSeconds(Number(raw.trim()))} s is above the ` +
      `${budgetSeconds(SCAN_BUDGET_MAX_MS)} s ceiling the guards share. Using ${budgetSeconds(ms)} s.`
    );
  }
  const deadline = performance.now() + ms;
  return () => {
    if (performance.now() >= deadline) {
      throw new ScanBudgetError(
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
