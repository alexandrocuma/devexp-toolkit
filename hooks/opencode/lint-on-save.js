/**
 * lint-on-save.js — runs the project linter after source file edits
 *
 * Event: file.edited
 * Advisory only — never throws.
 *
 * Linter priority per language:
 *   JS/TS:  local biome (if biome.json) > local eslint > global biome > global eslint
 *   Python: ruff > flake8
 *   Go:     go vet
 *   Ruby:   rubocop
 */

import { existsSync, join, dirname, resolve, extname } from './utils.js';
import { LINT_EXTS, findRoot, which, runLinter } from './utils.js';

export async function lintOnSave(_ctx) {
  return {
    'file.edited': async (event) => {
      // opencode fires file.edited with { file: absolutePath }
      const filePath = event.file ?? '';
      if (!filePath) return;

      const ext = extname(filePath).toLowerCase();
      if (!LINT_EXTS.has(ext)) return;

      const root = findRoot(filePath);

      try {
        if (['.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs'].includes(ext)) {
          const localBiome  = join(root, 'node_modules', '.bin', 'biome');
          const localEslint = join(root, 'node_modules', '.bin', 'eslint');
          const biomeCfg    = existsSync(join(root, 'biome.json')) ||
                              existsSync(join(root, 'biome.jsonc'));

          if (biomeCfg && existsSync(localBiome)) {
            await runLinter(localBiome, ['lint', filePath], root);
          } else if (existsSync(localEslint)) {
            await runLinter(localEslint, ['--max-warnings=0', '--no-warn-ignored', filePath], root);
          } else if (biomeCfg && await which('biome')) {
            await runLinter('biome', ['lint', filePath], root);
          } else if (await which('eslint')) {
            await runLinter('eslint', ['--max-warnings=0', filePath], root);
          }

        } else if (ext === '.py') {
          const ruff   = await which('ruff');
          const flake8 = ruff ? null : await which('flake8');
          if (ruff)        await runLinter(ruff,   ['check', filePath], root);
          else if (flake8) await runLinter(flake8, [filePath], root);

        } else if (ext === '.go') {
          const go = await which('go');
          if (go) {
            const relDir = resolve(dirname(resolve(filePath))).replace(root, '').replace(/^\//, '');
            const pkg    = relDir === '' ? './...' : `./${relDir}`;
            await runLinter(go, ['vet', pkg], root);
          }

        } else if (ext === '.rb') {
          const rubocop = await which('rubocop');
          if (rubocop) await runLinter(rubocop, ['--no-color', '--format', 'simple', filePath], root);
        }
      } catch {
        // Advisory — never propagate errors from file.edited
      }
    },
  };
}
