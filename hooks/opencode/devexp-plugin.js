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
 *   test-on-save.js           runs the associated test file after source file edits
 *   graphify-read-guard.js    gates source reads behind a tapering graphify
 *                             query cadence — self-gated: no-op unless
 *                             graphify-out/graph.json exists
 *   graphify-session-sentinel.js  tracks graphify query usage toward the gate —
 *                             same self-gating
 *   graphify-grep-nudge.js    advisory nudge toward graphify on grep-like
 *                             commands — same self-gating, never blocks
 *
 * The three graphify modules ship `enabled: false` in the Claude Code registry
 * (registering the script would spawn python3 on every Read/Glob/Bash/Grep even
 * in projects without graphify). They're composed here because the JS versions
 * self-gate cheaply via existsSync and are inert without a graphify-out/ graph.
 *
 * @see https://opencode.ai/docs/plugins
 */

import { secretGuard }            from './secret-guard.js';
import { secretInWriteGuard }     from './secret-in-write-guard.js';
import { dangerousCmdGuard }      from './dangerous-cmd-guard.js';
import { largeFileGuard }         from './large-file-guard.js';
import { lintOnSave }             from './lint-on-save.js';
import { formatOnSave }           from './format-on-save.js';
import { testOnSave }             from './test-on-save.js';
import { graphifyReadGuard }      from './graphify-read-guard.js';
import { graphifySessionSentinel } from './graphify-session-sentinel.js';
import { graphifyGrepNudge }      from './graphify-grep-nudge.js';

export const DevExpPlugin = async (ctx) => {
  const modules = await Promise.all([
    secretGuard(ctx),
    secretInWriteGuard(ctx),
    dangerousCmdGuard(ctx),
    largeFileGuard(ctx),
    lintOnSave(ctx),
    formatOnSave(ctx),
    testOnSave(ctx),
    graphifyReadGuard(ctx),
    graphifySessionSentinel(ctx),
    graphifyGrepNudge(ctx),
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
