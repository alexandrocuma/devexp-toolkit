/**
 * format-on-save.js — runs the project formatter on edited source files
 *
 * Event: file.edited
 *
 * Formatter priority per language:
 *   JS/TS:  local biome --write (if biome.json) > local prettier > global biome > global prettier
 *   Python: ruff format > black
 *   Go:     gofmt -w
 *   Ruby:   rubocop --autocorrect-all
 *
 * Advisory only — cannot block. Modifies the file in-place. Silent if no formatter found.
 * The rewrite lands after opencode's edit tool computed the diff it reports, so
 * that diff can differ from the file on disk.
 */

import { existsSync, extname, join, findRoot, which, runCommand, editedPath } from './utils.js';

const FORMAT_EXTS = new Set(['.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs', '.py', '.go', '.rb']);

export const FORMAT_TIMEOUT_MS = 15000;

// Output is ignored, so a formatter that fails, times out or is missing is
// silent — as it was with execFileSync(…, { stdio: 'ignore' }).
async function runFormatter(cmd, args, cwd) {
  await runCommand(cmd, args, { cwd, timeout: FORMAT_TIMEOUT_MS, output: 'ignore' });
}

export async function formatOnSave(ctx) {
  return {
    'file.edited': async (event) => {
      const filePath = editedPath(event.file ?? event.path ?? '', ctx?.directory);
      if (!filePath || !existsSync(filePath)) return;

      const ext = extname(filePath).toLowerCase();
      if (!FORMAT_EXTS.has(ext)) return;

      const root = findRoot(filePath);

      if (['.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs'].includes(ext)) {
        const localBiome    = join(root, 'node_modules', '.bin', 'biome');
        const localPrettier = join(root, 'node_modules', '.bin', 'prettier');
        const biomeCfg = existsSync(join(root, 'biome.json')) || existsSync(join(root, 'biome.jsonc'));

        if (biomeCfg && existsSync(localBiome)) {
          await runFormatter(localBiome, ['format', '--write', filePath], root);
        } else if (existsSync(localPrettier)) {
          await runFormatter(localPrettier, ['--write', filePath], root);
        } else if (biomeCfg && await which('biome')) {
          await runFormatter('biome', ['format', '--write', filePath], root);
        } else if (await which('prettier')) {
          await runFormatter('prettier', ['--write', filePath], root);
        }
      } else if (ext === '.py') {
        if (await which('ruff')) {
          await runFormatter('ruff', ['format', filePath], root);
        } else if (await which('black')) {
          await runFormatter('black', ['--quiet', filePath], root);
        }
      } else if (ext === '.go') {
        if (await which('gofmt')) {
          await runFormatter('gofmt', ['-w', filePath], root);
        }
      } else if (ext === '.rb') {
        if (await which('rubocop')) {
          await runFormatter('rubocop', ['--autocorrect-all', '--no-color', '--format', 'quiet', filePath], root);
        }
      }
    },
  };
}
