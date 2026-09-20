/**
 * Tests for secret-guard.js — mirrors hooks/claude-code/secret-guard.test.sh.
 * Run: node hooks/opencode/secret-guard.test.js
 */
import { isSecretFile, secretInCommand } from './secret-guard.js';

const DOTENV = '.env';
const TEMPLATE = `${DOTENV}.example`;

// Read-tool paths that must be blocked.
const BLOCK_PATHS = [
  DOTENV,
  `mcps/${DOTENV}`,
  `${DOTENV}.production`,
  `${DOTENV}.local`,
  'certs/server.pem',
  'certs/server.key',
  'bundle.p12',
  '~/.ssh/id_rsa',
  'deploy_ed25519',
];

// Committed templates are not secrets.
const ALLOW_PATHS = [
  TEMPLATE,
  `mcps/${TEMPLATE}`,
  `${DOTENV}.production.example`,
  'config.sample',
  'server.pem.template',
  'mcps/registry.json',
  'README.md',
];

// Commands that genuinely read a secret.
const BLOCK_CMDS = [
  `cat ${DOTENV}`,
  `head -5 mcps/${DOTENV}`,
  `less ${DOTENV}.production`,
  'cat certs/server.key',
  `cp mcps/${DOTENV} /tmp/backup`,
  'cat ~/.ssh/id_rsa',
  'openssl x509 -in certs/server.pem -text',
  'cat id_rsa',          // bare token in a path position is still a read
  'grep secret id_rsa',  // pattern is 'secret'; id_rsa is the file argument
];

// A mention is not a read, and templates stay readable.
const ALLOW_CMDS = [
  `cat mcps/${TEMPLATE}`,
  `grep -c UI_INSPECTOR_DIR mcps/${TEMPLATE}`,
  "jq -r '.key' entries.json",
  "jq '.entries[].key' seed.json",
  'echo .key',
  `python3 -c "print('server.pem')"`,
  "git commit -m 'document the key in the env template'",
  // Searching for key names is how you audit for leaks — the first non-flag
  // argument of a searcher is a pattern, never a path.
  "grep -rn 'id_rsa' docs/",
  'grep -rn id_rsa docs/',
  'rg id_rsa .',
  `cat <<'EOF'\nmentions ${DOTENV}.production in prose\nEOF`,
  'cat README.md',
  'ls mcps/',
];

let fail = 0;
for (const p of BLOCK_PATHS) if (!isSecretFile(p)) { console.log('FAIL want block (path):', p); fail++; }
for (const p of ALLOW_PATHS) if (isSecretFile(p)) { console.log('FAIL want allow (path):', p); fail++; }
for (const c of BLOCK_CMDS) if (!secretInCommand(c)) { console.log('FAIL want block (cmd):', c); fail++; }
for (const c of ALLOW_CMDS) if (secretInCommand(c)) { console.log('FAIL want allow (cmd):', c); fail++; }

const total = BLOCK_PATHS.length + ALLOW_PATHS.length + BLOCK_CMDS.length + ALLOW_CMDS.length;
console.log(`${total - fail} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
