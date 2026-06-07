/**
 * graphify-session-sentinel.js — records each `graphify query/path/explain`
 * toward graphify-read-guard's tapering gate by incrementing queries_done in
 * the shared session state. Also seeds/resets that state on a new session,
 * so a query run before any read still initializes the cycle correctly.
 * Ships disabled by default — only meaningful alongside graphify-read-guard.
 *
 * Session id: derived the same independent way as graphify-read-guard.js
 * (pid + per-process constant) — see LOCAL_SESSION_ID there for why.
 *
 * Advisory only: never blocks, errors are swallowed.
 * State: graphify-out/.graphify_session (JSON) — shared with graphify-read-guard
 * Event: tool.execute.before (tool: bash)
 */

import { readFileSync, writeFileSync } from 'fs';
import { existsSync } from './utils.js';

const SENTINEL = 'graphify-out/.graphify_session';
const GRAPHIFY_CMDS = ['graphify query', 'graphify path', 'graphify explain'];

const LOCAL_SESSION_ID = `opencode-pid-${process.pid}`;

function defaultState() {
  return { session_id: LOCAL_SESSION_ID, phase: 0, queries_done: 0, reads_done: 0, unlocked: false };
}

export async function graphifySessionSentinel(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'bash') return;
      const cmd = output.args?.command ?? '';
      if (!cmd || !existsSync('graphify-out')) return;
      if (!GRAPHIFY_CMDS.some(c => cmd.includes(c))) return;

      try {
        let state;
        try {
          const data = JSON.parse(readFileSync(SENTINEL, 'utf-8'));
          state = (data && typeof data === 'object' && 'session_id' in data) ? data : defaultState();
        } catch {
          state = defaultState();
        }

        if (state.session_id !== LOCAL_SESSION_ID) {
          state = defaultState();
        }

        state.queries_done = (state.queries_done ?? 0) + 1;
        writeFileSync(SENTINEL, JSON.stringify(state));
      } catch {
        // advisory only — never propagate errors
      }
    },
  };
}
