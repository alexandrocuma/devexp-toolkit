/**
 * devexp-plugin.js — devexp framework hooks for opencode
 *
 * Hooks provided:
 *  - secret-guard  (tool.execute.before): blocks accidental reads of .env and key files
 *  - lint-on-save  (file.edited):         advisory message after source file edits
 *
 * opencode plugin API:
 *  - tool.execute.before: throw to block, return to allow
 *  - file.edited: runs after a file is written
 *
 * @see https://opencode.ai/docs/plugins
 */

const SECRET_NAMES = new Set([
  '.env', '.env.local', '.env.production', '.env.staging',
  '.env.test', '.env.secret',
]);

const SOURCE_EXTENSIONS = new Set([
  '.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs',
  '.py',
  '.go',
  '.rs',
  '.rb',
  '.java', '.kt', '.scala',
  '.swift',
  '.c', '.cpp', '.cc', '.h', '.hpp',
]);

function basename(filePath) {
  return (filePath ?? '').split('/').pop() ?? '';
}

function extname(filePath) {
  const base = basename(filePath);
  const dot = base.lastIndexOf('.');
  return dot >= 0 ? base.slice(dot) : '';
}

function isSecretFile(filePath) {
  const base = basename(filePath);
  if (SECRET_NAMES.has(base)) return true;
  if (base.startsWith('.env.')) return true;
  const lower = base.toLowerCase();
  if (lower.endsWith('.pem') || lower.endsWith('.key') || lower.endsWith('.p12') || lower.endsWith('.pfx')) return true;
  if (lower.endsWith('_rsa') || lower.endsWith('_dsa') || lower.endsWith('_ecdsa') || lower.endsWith('_ed25519')) return true;
  return false;
}

export const DevExpPlugin = async ({ project, client, $, directory, worktree }) => {
  return {
    // secret-guard: block reads of .env and private key files
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'read') return;

      // opencode passes file path as filePath or file_path depending on version
      const filePath = output.args?.filePath ?? output.args?.file_path ?? '';
      if (!filePath) return;

      if (isSecretFile(filePath)) {
        throw new Error(
          `[devexp secret-guard] Blocked read of "${basename(filePath)}". ` +
          `This file may contain secrets. If intentional, confirm with the user first.`
        );
      }
    },

    // lint-on-save: advisory after source file edits
    'file.edited': async (event) => {
      const filePath = event.path ?? '';
      if (!filePath) return;

      if (SOURCE_EXTENSIONS.has(extname(filePath))) {
        console.log(`[devexp lint-on-save] ${filePath} edited — consider running the project linter`);
      }
    },
  };
};
