/**
 * secret-guard.js — blocks accidental reads of .env and private key files
 *
 * Event: tool.execute.before (tool: read)
 */

import { basename } from './utils.js';

const SECRET_NAMES = new Set([
  '.env', '.env.local', '.env.production', '.env.staging',
  '.env.test', '.env.secret',
]);

function isSecretFile(filePath) {
  const base = basename(filePath ?? '');
  if (SECRET_NAMES.has(base)) return true;
  if (base.startsWith('.env.')) return true;
  const lower = base.toLowerCase();
  if (lower.endsWith('.pem') || lower.endsWith('.key') || lower.endsWith('.p12') || lower.endsWith('.pfx')) return true;
  if (lower.endsWith('_rsa') || lower.endsWith('_dsa') || lower.endsWith('_ecdsa') || lower.endsWith('_ed25519')) return true;
  return false;
}

export async function secretGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool === 'read') {
        const filePath = output.args?.filePath ?? '';
        if (filePath && isSecretFile(filePath)) {
          throw new Error(
            `[devexp secret-guard] Blocked read of "${basename(filePath)}". ` +
            `This file may contain secrets. If intentional, confirm with the user first.`
          );
        }
      } else if (input.tool === 'bash') {
        const cmd = output.args?.command ?? '';
        if (!cmd) return;
        // Tokenize and check each non-flag argument for secret file paths
        const tokens = cmd.match(/(?:[^\s"']+|"[^"]*"|'[^']*')+/g) ?? [];
        for (const token of tokens) {
          if (token.startsWith('-')) continue;
          const clean = token.replace(/^['"]|['"]$/g, '');
          if (isSecretFile(clean)) {
            throw new Error(
              `[devexp secret-guard] Blocked bash access to "${basename(clean)}". ` +
              `This file may contain secrets. If intentional, confirm with the user first.`
            );
          }
        }
      }
    },
  };
}
