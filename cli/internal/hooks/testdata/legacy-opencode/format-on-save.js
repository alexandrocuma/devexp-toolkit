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
 */

import { execFileSync } from 'child_process';
import { existsSync, extname, join, findRoot, which } from './utils.js';

const FORMAT_EXTS = new Set(['.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs', '.py', '.go', '.rb']);

function runFormatter(cmd, args, cwd) {
  try {
    execFileSync(cmd, args, { cwd, timeout: 15000, stdio: 'ignore' });
  } catch (e) {
    if (e.code === 'ENOENT') return;
    const output = ((e.stdout ?? '') + (e.stderr ?? '')).trim();
    if (output) console.log(`[devexp format-on-save] ${cmd.split('/').pop()}:\n${output}`);
  }
}

export async function formatOnSave(_ctx) {
  return {
    'file.edited': async (event) => {
      const filePath = event.file ?? event.path ?? '';
      if (!filePath || !existsSync(filePath)) return;

      const ext = extname(filePath).toLowerCase();
      if (!FORMAT_EXTS.has(ext)) return;

      const root = findRoot(filePath);

      if (['.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs'].includes(ext)) {
        const localBiome    = join(root, 'node_modules', '.bin', 'biome');
        const localPrettier = join(root, 'node_modules', '.bin', 'prettier');
        const biomeCfg = existsSync(join(root, 'biome.json')) || existsSync(join(root, 'biome.jsonc'));

        if (biomeCfg && existsSync(localBiome)) {
          runFormatter(localBiome, ['format', '--write', filePath], root);
        } else if (existsSync(localPrettier)) {
          runFormatter(localPrettier, ['--write', filePath], root);
        } else if (biomeCfg && which('biome')) {
          runFormatter('biome', ['format', '--write', filePath], root);
        } else if (which('prettier')) {
          runFormatter('prettier', ['--write', filePath], root);
        }
      } else if (ext === '.py') {
        if (which('ruff')) {
          runFormatter('ruff', ['format', filePath], root);
        } else if (which('black')) {
          runFormatter('black', ['--quiet', filePath], root);
        }
      } else if (ext === '.go') {
        if (which('gofmt')) {
          runFormatter('gofmt', ['-w', filePath], root);
        }
      } else if (ext === '.rb') {
        if (which('rubocop')) {
          runFormatter('rubocop', ['--autocorrect-all', '--no-color', '--format', 'quiet', filePath], root);
        }
      }
    },
  };
}
