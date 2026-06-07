/**
 * graphify-read-guard.js — gates source-file reads/globs behind graphify
 * usage with a tapering session cadence rather than a flat timer:
 *
 *   Phase 0 (fresh session): 5 graphify queries required to unlock
 *   Phase 1: 3 queries required
 *   Phase 2+ (steady state floor): 1 query required
 *
 * Each unlock grants a budget of 6 reads; exhausting the budget re-arms the
 * gate at the next (tighter) phase — heavy grounding up front when context
 * is thinnest, a lighter touch once the agent has shown sustained engagement
 * with the graph. Requires the graphify skill and an existing
 * graphify-out/graph.json — ships disabled by default.
 *
 * Session id: opencode doesn't pass a stable session identifier into
 * tool.execute.before, but the plugin factory runs once per session start —
 * so a per-process id (pid + load time) naturally resets the gate at the
 * start of each new session and stays stable for the life of this one.
 * Must match the id minted in graphify-session-sentinel.js (both derive it
 * the same way, independently — see LOCAL_SESSION_ID).
 *
 * Note: opencode's tool.execute.before is block-or-allow only (no
 * additionalContext equivalent), so the "allow" branches below are silent —
 * Claude Code users get cycle/budget hints via additionalContext; opencode
 * users won't.
 *
 * State: graphify-out/.graphify_session (JSON), shared with graphify-session-sentinel
 * Event: tool.execute.before (tools: read, glob)
 */

import { readFileSync, writeFileSync } from 'fs';
import { existsSync } from './utils.js';

const SOURCE_EXTS = [
  '.py', '.js', '.ts', '.tsx', '.jsx', '.go', '.rs', '.java', '.rb',
  '.c', '.h', '.cpp', '.hpp', '.cc', '.cs', '.kt', '.swift', '.php',
  '.scala', '.lua', '.vue', '.svelte', '.ex', '.exs', '.clj', '.hs',
  '.sh', '.bash', '.zsh', '.fish',
];

const SENTINEL = 'graphify-out/.graphify_session';
const REQUIRED = [5, 3, 1];
const READ_BUDGET = 6;

// Stable for this process's lifetime, distinct across separate opencode runs —
// the closest available proxy for a session id (see header note).
const LOCAL_SESSION_ID = `opencode-pid-${process.pid}`;

function defaultState() {
  return { session_id: LOCAL_SESSION_ID, phase: 0, queries_done: 0, reads_done: 0, unlocked: false };
}

function loadState() {
  try {
    const data = JSON.parse(readFileSync(SENTINEL, 'utf-8'));
    if (data && typeof data === 'object' && 'session_id' in data) return data;
  } catch {
    // missing or corrupt — fall through to default
  }
  return defaultState();
}

function saveState(state) {
  try {
    writeFileSync(SENTINEL, JSON.stringify(state));
  } catch {
    // best-effort
  }
}

export async function graphifyReadGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'read' && input.tool !== 'glob') return;

      const s = [output.args?.filePath, output.args?.pattern, output.args?.path]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .replace(/\\/g, '/');

      const isSource = SOURCE_EXTS.some(ext => s.includes(ext));
      const inGraphify = s.includes('graphify-out/');
      const graphExists = existsSync('graphify-out/graph.json');

      if (!isSource || inGraphify || !graphExists) return;

      let state = loadState();
      if (state.session_id !== LOCAL_SESSION_ID) {
        state = defaultState();
        saveState(state);
      }

      const phase = Math.min(state.phase ?? 0, REQUIRED.length - 1);
      const required = REQUIRED[phase];

      if (!state.unlocked) {
        const queriesDone = state.queries_done ?? 0;
        if (queriesDone < required) {
          const remaining = required - queriesDone;
          throw new Error(
            '[devexp graphify-read-guard] Blocked — graphify grounding required. ' +
            `Run graphify query "<your question>" ${remaining} more time(s) before reading ` +
            `source files (${queriesDone}/${required} done this cycle). The knowledge graph ` +
            'answers most codebase questions faster and cheaper than reading files.'
          );
        }
        state.unlocked = true;
        state.reads_done = 0;
        saveState(state);
        return;
      }

      const readsDone = state.reads_done ?? 0;
      if (readsDone < READ_BUDGET) {
        state.reads_done = readsDone + 1;
        saveState(state);
        return;
      }

      const nextPhase = Math.min(phase + 1, REQUIRED.length - 1);
      const nextRequired = REQUIRED[nextPhase];
      state.phase = nextPhase;
      state.queries_done = 0;
      state.unlocked = false;
      saveState(state);
      throw new Error(
        '[devexp graphify-read-guard] Blocked — re-grounding needed. ' +
        `You've read ${READ_BUDGET} files since the last graphify query. ` +
        `Run graphify query "<your question>" ${nextRequired} more time(s) to continue reading source files.`
      );
    },
  };
}
