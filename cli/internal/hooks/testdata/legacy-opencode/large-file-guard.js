/**
 * large-file-guard.js — blocks full overwrites of files with more than 500 lines
 *
 * Event: tool.execute.before (tool: write)
 *
 * Write replaces the entire file — large overwrites risk data loss if the path is wrong.
 * Edit is targeted and exempt from this check.
 */

import { existsSync, basename, countLines } from './utils.js';

const THRESHOLD = 500;

export async function largeFileGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'write') return;

      // write tool uses filePath (camelCase) — confirmed from opencode source
      const filePath = output.args?.filePath ?? '';
      if (!filePath || !existsSync(filePath)) return;

      const lineCount = countLines(filePath);
      if (lineCount > THRESHOLD) {
        throw new Error(
          `[devexp large-file-guard] About to overwrite "${basename(filePath)}" (${lineCount} lines). ` +
          `Confirm this full replacement is intentional.`
        );
      }
    },
  };
}
