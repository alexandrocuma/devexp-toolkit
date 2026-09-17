/**
 * Tests for secret-in-write-guard.js — mirrors hooks/claude-code/secret-in-write-guard.test.sh.
 * Run: node hooks/opencode/secret-in-write-guard.test.js
 *
 * Drives the real `tool.execute.before` handler with opencode-shaped args:
 * write → { filePath, content }, edit → { filePath, oldString, newString }.
 *
 * Every secret below is FAKE: a vendor prefix and a dummy body containing
 * "FAKE", joined at runtime. This file never holds a secret-shaped string
 * itself, so the installed guard lets it be written and secret scanners
 * have nothing to flag.
 */
import { secretInWriteGuard } from './secret-in-write-guard.js';

const handler = (await secretInWriteGuard({}))['tool.execute.before'];
const PREFIX = '[devexp secret-in-write-guard] Blocked: content appears to contain';

// Resolves to the thrown error, or null when the guard allowed the call.
async function run(tool, payload, { file = 'src/config.ts', old = 'TODO' } = {}) {
  const args = tool === 'write'
    ? { filePath: file, content: payload }
    : { filePath: file, oldString: old, newString: payload };
  try {
    await handler({ tool }, { args });
    return null;
  } catch (err) {
    return err;
  }
}

// ── Fake secrets, one per shape the guard claims to detect ──────────────────
const SK = 'sk', AK = 'AKIA', GH = 'gh', GHP = 'github', XOX = 'xox', D5 = '-----';
const ANTHROPIC = `${SK}-ant-api03-${'FAKE_body-'.repeat(9)}AA`;
const OPENAI = `${SK}-${'0FAKE'.repeat(10)}`;
const AWS = `${AK}${'FAKE'.repeat(4)}`;
const GH_P = `${GH}p_${'0FAKE'.repeat(8)}`;
const GH_O = `${GH}o_${'0FAKE'.repeat(8)}`;
const GH_S = `${GH}s_${'0FAKE'.repeat(8)}`;
const OPENAI_PROJ = `${SK}-proj-${'FAKE_proj-body'.repeat(8)}`;
const OPENAI_SVC = `${SK}-svcacct-${'FAKE_svc-body'.repeat(8)}`;
const OPENAI_ADMIN = `${SK}-admin-${'FAKE_admin-body'.repeat(8)}`;
const GH_U = `${GH}u_${'0FAKE'.repeat(8)}`;
const GH_R = `${GH}r_${'0FAKE'.repeat(8)}`;
const GH_PAT = `${GHP}_pat_${'0FAKE'.repeat(5)}_${'FAKE0'.repeat(12)}`;
const SLACK_B = `${XOX}b-1234567890-1234567890123-${'FAKE'.repeat(6)}`;
const SLACK_P = `${XOX}p-1234567890-1234567890-1234567890123-${'0fake'.repeat(6)}`;
const pem = (kind) => `${D5}BEGIN ${kind}${D5}\nMIIEFAKEFAKEFAKE\n${D5}END ${kind}${D5}`;
const PK_RSA = pem('RSA PRIVATE KEY');
const FILLER = 'an ordinary line of prose in a large generated file\n'.repeat(5000); // ~260 KB

// [tool, word the block message must name, payload, opts]
const BLOCK = [];

// Each pattern, via write content and edit newString.
for (const tool of ['write', 'edit']) {
  BLOCK.push(
    [tool, 'Anthropic', ANTHROPIC],
    [tool, 'OpenAI', OPENAI],
    [tool, 'OpenAI', OPENAI_PROJ],
    [tool, 'OpenAI', OPENAI_SVC],
    [tool, 'OpenAI', OPENAI_ADMIN],
    [tool, 'AWS', AWS],
    [tool, 'GitHub', GH_P],
    [tool, 'GitHub', GH_O],
    [tool, 'GitHub', GH_S],
    [tool, 'GitHub', GH_U],
    [tool, 'GitHub', GH_R],
    [tool, 'GitHub', GH_PAT],
    [tool, 'Slack', SLACK_B],
    [tool, 'Slack', SLACK_P],
    [tool, 'private key', PK_RSA],
    [tool, 'private key', pem('OPENSSH PRIVATE KEY')],
    [tool, 'private key', pem('EC PRIVATE KEY')],
    [tool, 'private key', pem('PRIVATE KEY')],
    [tool, 'private key', pem('ENCRYPTED PRIVATE KEY')],
    [tool, 'private key', pem('PGP PRIVATE KEY BLOCK')],
  );
}

// Wherever the secret sits.
BLOCK.push(
  ['write', 'Anthropic', `const client = new Anthropic({ apiKey: "${ANTHROPIC}" });`],
  ['write', 'OpenAI', `line one\nline two\nOPENAI_API_KEY=${OPENAI}\nline four`],
  ['edit', 'AWS', `aws_access_key_id = ${AWS}`],
  ['write', 'OpenAI', `client = OpenAI(api_key='${OPENAI_PROJ}')`],
  // A template is exempt only for what it holds, not for its name.
  ['write', 'GitHub', `GITHUB_TOKEN=${GH_P}`, { file: '.env.example' }],
  // Larger than a pipe buffer, secret first — the shell twin once allowed these (#101).
  ['write', 'OpenAI', `${OPENAI}\n${FILLER}`],
  ['edit', 'private key', `${PK_RSA}${FILLER}`],
);

const TEMPLATE = [
  '# Copy to .env and fill in real values.',
  'ANTHROPIC_API_KEY=',
  'OPENAI_API_KEY=sk-...',
  'GITHUB_TOKEN=<your-github-token>',
  'AWS_ACCESS_KEY_ID=your-access-key-id-here',
  'SLACK_BOT_TOKEN=xoxb-...',
].join('\n');

// [tool, payload, opts]
const ALLOW = [
  // Prose that mentions tokens is not a token.
  ['write', 'Set your API key and bearer token in the environment. Never commit a secret, a password or a private key.'],
  ['write', 'Anthropic keys start with sk-ant-, OpenAI keys with sk-, AWS key IDs with AKIA, GitHub tokens with ghp_ and Slack bot tokens with xoxb-.'],
  ['edit', 'Rotate the token if it leaks; see docs/security.md.'],
  ['write', 'OpenAI project keys start with sk-proj- and fine-grained GitHub tokens with github_pat_.'],
  // A variable named token holds no value.
  ['write', 'const token = process.env.GITHUB_TOKEN;'],
  ['write', 'api_key = os.environ["OPENAI_API_KEY"]'],
  ['edit', 'token: ${{ secrets.GITHUB_TOKEN }}'],
  // Committed templates document keys with placeholders.
  ['write', TEMPLATE, { file: '.env.example' }],
  ['write', TEMPLATE, { file: '.env.sample' }],
  ['write', TEMPLATE, { file: '.env.template' }],
  ['write', TEMPLATE, { file: '.env.dist' }],
  ['edit', 'OPENAI_API_KEY=<your-openai-key>', { file: '.env.example', old: 'OPENAI_API_KEY=' }],
  // Public material and look-alike words.
  ['write', pem('PUBLIC KEY')],
  ['write', pem('CERTIFICATE')],
  ['write', 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFAKE user@host'],
  ['write', 'import sklearn  # a.k.a. sk-learn; see task-runner and risk-score'],
  ['write', '<div class="desk-admin-navigation-sidebar-collapsed-state-controller">'],
  ['edit', 'const route = "/task-proj-onboarding-checklist-and-welcome-email-sequence";'],
  // Only the new text is scanned; an edit that takes a key out must not be refused.
  ['edit', 'OPENAI_API_KEY=process.env.OPENAI_API_KEY', { old: `OPENAI_API_KEY=${OPENAI}` }],
  ['edit', ''],
  ['write', ''],
  ['write', FILLER],
];

const show = (s) => s.slice(0, 80).replace(/\n/g, ' ');
let fail = 0;

for (const [tool, word, payload, opts] of BLOCK) {
  const err = await run(tool, payload, opts);
  if (!err) { console.log(`FAIL want block (${tool}):`, show(payload)); fail++; }
  else if (!(err instanceof Error) || !err.message.startsWith(PREFIX) || !err.message.includes(word)) {
    console.log(`FAIL message does not name ${word} (${tool}):`, err.message ?? err); fail++;
  } else if (err.message.includes('FAKE')) {
    console.log(`FAIL message echoes the secret (${tool}):`, err.message); fail++;
  }
}

for (const [tool, payload, opts] of ALLOW) {
  const err = await run(tool, payload, opts);
  if (err) { console.log(`FAIL want allow (${tool}):`, show(payload), '→', err.message); fail++; }
}

// Other tools are not this guard's business — secret-guard owns reads and shell.
const OTHER_TOOLS = [
  ['read', { filePath: 'notes.md', content: OPENAI }],
  ['bash', { command: `echo ${OPENAI}` }],
];
for (const [tool, args] of OTHER_TOOLS) {
  try { await handler({ tool }, { args }); } catch (err) {
    console.log(`FAIL ${tool} should be ignored:`, err.message); fail++;
  }
}

const total = BLOCK.length + ALLOW.length + OTHER_TOOLS.length;
console.log(`${total - fail} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
