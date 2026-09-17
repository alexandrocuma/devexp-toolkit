/**
 * secret-guard.js — blocks accidental reads of .env and private key files
 *
 * Event: tool.execute.before (tool: read)
 *
 * Two false-positive classes are deliberately excluded, mirroring
 * hooks/claude-code/secret-guard.sh:
 *
 *   Templates (#87). A committed `.env.example` documents which keys exist;
 *   it never holds their values.
 *
 *   Mentions (#81). A shell token only counts as a path if it plausibly is
 *   one — heredoc bodies, program text and bare extensions are not reads.
 *
 * The scan runs under a wall-clock budget and refuses the call when it is
 * exceeded (#162) — see startScanBudget in utils.js.
 *
 * Tests: node hooks/opencode/secret-guard.test.js
 */

import { basename, startScanBudget } from './utils.js';

const SECRET_NAMES = new Set([
  '.env', '.env.local', '.env.production', '.env.staging',
  '.env.test', '.env.secret',
]);
const KEY_EXTS = ['.pem', '.key', '.p12', '.pfx'];
const KEY_SUFFIXES = ['_rsa', '_dsa', '_ecdsa', '_ed25519'];
// Committed templates name the keys; they never carry the values.
const TEMPLATE_SUFFIXES = ['.example', '.sample', '.template', '.dist'];

export function isSecretFile(filePath) {
  const base = basename(filePath ?? '');
  if (!base) return false;
  const lower = base.toLowerCase();

  // Safe even when the stem is a real secret name, e.g. .env.production.example
  if (TEMPLATE_SUFFIXES.some((s) => lower.endsWith(s))) return false;

  // Exact names stay blocked regardless of anything below.
  if (SECRET_NAMES.has(base)) return true;
  if (base.startsWith('.env.')) return true;

  // 'server.key' is a key file; a bare '.key' is an extension, which is what
  // a jq filter or a bare word looks like after tokenizing.
  const stem = lower.includes('.') ? lower.slice(0, lower.lastIndexOf('.')) : '';
  if (stem && KEY_EXTS.some((e) => lower.endsWith(e))) return true;
  if (KEY_SUFFIXES.some((s) => lower.endsWith(s))) return true;
  return false;
}

// Heredoc bodies are data, not paths.
const HEREDOC = /<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?[\s\S]*?^\s*\1\s*$/gm;
const PROGRAM_CHARS = /[|[\]{}()$`*?<>;&=\n\t]/;

export function plausiblePath(tok) {
  if (!tok || tok.startsWith('-')) return false;
  if (tok.includes(' ')) return false; // a quoted program, not a filename
  return !PROGRAM_CHARS.test(tok);
}

const SEARCHERS = new Set(['grep', 'egrep', 'fgrep', 'rg', 'ag', 'ack']);

/** `overBudget` is called once per token; it throws when the budget is spent. */
export function secretInCommand(cmd, overBudget = () => {}) {
  if (!cmd) return null;
  const stripped = cmd.replace(HEREDOC, ' ');
  const tokens = stripped.match(/(?:[^\s"']+|"[^"]*"|'[^']*')+/g) ?? [];
  let skipPattern = false;
  for (const token of tokens) {
    overBudget();
    const clean = token.replace(/^['"]|['"]$/g, '');
    // A searcher's first non-flag argument is a pattern, never a path.
    // Auditing for leaked key names is security work, not a secret read.
    if (SEARCHERS.has(basename(clean))) { skipPattern = true; continue; }
    if (skipPattern) {
      if (clean.startsWith('-')) continue;
      skipPattern = false;
      continue;
    }
    if (!plausiblePath(clean)) continue;
    if (isSecretFile(clean)) return basename(clean);
  }
  return null;
}

export async function secretGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'read' && input.tool !== 'bash') return;
      const overBudget = startScanBudget('secret-guard');
      overBudget();

      if (input.tool === 'read') {
        const filePath = output.args?.filePath ?? '';
        if (filePath && isSecretFile(filePath)) {
          throw new Error(
            `[devexp secret-guard] Blocked access to "${basename(filePath)}". ` +
            `This file may contain secrets. If intentional, confirm with the user first.`
          );
        }
      } else {
        const hit = secretInCommand(output.args?.command ?? '', overBudget);
        if (hit) {
          throw new Error(
            `[devexp secret-guard] Blocked access to "${hit}". ` +
            `This file may contain secrets. If intentional, confirm with the user first.`
          );
        }
      }
    },
  };
}
