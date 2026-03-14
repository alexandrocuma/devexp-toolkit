/**
 * devexp-plugin.js — entry point for devexp opencode hooks
 *
 * Composes all hook modules into a single plugin export.
 * Each hook lives in its own file — edit them individually.
 *
 *   secret-guard.js           blocks reads of .env and key files
 *   secret-in-write-guard.js  blocks writes containing secret/token patterns
 *   dangerous-cmd-guard.js    hard-blocks destructive shell commands
 *   large-file-guard.js       blocks full overwrites of large files (>500 lines)
 *   lint-on-save.js           runs the project linter after source file edits
 *   format-on-save.js         runs the project formatter after source file edits
 *
 * @see https://opencode.ai/docs/plugins
 */

import { secretGuard }          from './secret-guard.js';
import { secretInWriteGuard }   from './secret-in-write-guard.js';
import { dangerousCmdGuard }    from './dangerous-cmd-guard.js';
import { largeFileGuard }       from './large-file-guard.js';
import { lintOnSave }           from './lint-on-save.js';
import { formatOnSave }         from './format-on-save.js';

export const DevExpPlugin = async (ctx) => {
  const modules = await Promise.all([
    secretGuard(ctx),
    secretInWriteGuard(ctx),
    dangerousCmdGuard(ctx),
    largeFileGuard(ctx),
    lintOnSave(ctx),
    formatOnSave(ctx),
  ]);

  return {
    // Run all tool.execute.before handlers in sequence — first throw wins
    'tool.execute.before': async (input, output) => {
      for (const mod of modules) {
        if (mod['tool.execute.before']) {
          await mod['tool.execute.before'](input, output);
        }
      }
    },

    // Run all file.edited handlers — errors are swallowed per-module
    'file.edited': async (event) => {
      for (const mod of modules) {
        if (mod['file.edited']) {
          await mod['file.edited'](event);
        }
      }
    },
  };
};
