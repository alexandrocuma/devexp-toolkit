/**
 * graphify-grep-nudge.js — soft-nudge toward `graphify query` when grep-like
 * Bash commands or the grep tool run. Grepping raw files is often a fast,
 * legitimate choice — this just surfaces the cheaper alternative when a
 * knowledge graph is available. Ships disabled by default.
 *
 * Note: opencode's tool.execute.before is block-or-allow only — there's no
 * additionalContext / soft-ask equivalent (Claude Code can inject context
 * into the model's view; opencode can only throw or pass through silently).
 * So this logs an advisory to the console instead of nudging the agent
 * directly. It never blocks.
 *
 * Event: tool.execute.before (tools: bash, grep)
 */

import { existsSync } from './utils.js';

const GREP_LIKE = /(?:^|[\s;|&])(?:grep|rg|ripgrep|find|fd|ack|ag)\s/;

export async function graphifyGrepNudge(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'bash' && input.tool !== 'grep') return;
      if (!existsSync('graphify-out/graph.json')) return;

      const isGrepTool = input.tool === 'grep';
      const isGrepCmd = input.tool === 'bash' && GREP_LIKE.test(output.args?.command ?? '');
      if (!isGrepTool && !isGrepCmd) return;

      console.log(
        '[devexp graphify-grep-nudge] Tip: graphify query "<question>" often returns ' +
        'a smaller, more focused answer than grepping raw files (graphify-out/ has a graph).'
      );
    },
  };
}
