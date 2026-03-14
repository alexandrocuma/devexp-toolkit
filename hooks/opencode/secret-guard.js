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
      if (input.tool !== 'read') return;

      // read tool uses filePath (camelCase) — confirmed from opencode source
      const filePath = output.args?.filePath ?? '';
      if (!filePath) return;

      if (isSecretFile(filePath)) {
        throw new Error(
          `[devexp secret-guard] Blocked read of "${basename(filePath)}". ` +
          `This file may contain secrets. If intentional, confirm with the user first.`
        );
      }
    },
  };
}
