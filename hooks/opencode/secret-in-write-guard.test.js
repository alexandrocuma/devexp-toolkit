/**
 * Tests for secret-in-write-guard.js — mirrors hooks/claude-code/secret-in-write-guard.test.sh.
 * Run: node hooks/opencode/secret-in-write-guard.test.js
 *
 * Drives the real `tool.execute.before` handler with opencode-shaped args:
 * write → { filePath, content }, edit → { filePath, oldString, newString },
 * apply_patch → { patchText } (opencode's tool for GPT models).
 * MultiEdit and NotebookEdit are Claude Code tools, so only the shell twin
 * covers them.
 *
 * Every secret below is FAKE: a vendor prefix and a dummy body containing
 * "FAKE", joined at runtime. This file never holds a secret-shaped string
 * itself, so the installed guard lets it be written and secret scanners
 * have nothing to flag.
 */
import { secretInWriteGuard } from './secret-in-write-guard.js';

const handler = (await secretInWriteGuard({}))['tool.execute.before'];
const PREFIX = '[devexp secret-in-write-guard] Blocked: content appears to contain';

const patch = (...lines) => ['*** Begin Patch', ...lines, '*** End Patch'].join('\n');

// The args each tool gets. For apply_patch the payload becomes a new file's
// contents, unless opts.raw says it already is the whole patch.
function argsFor(tool, payload, { file = 'src/config.ts', old = 'TODO', raw = false } = {}) {
  if (tool === 'write') return { filePath: file, content: payload };
  if (tool === 'edit') return { filePath: file, oldString: old, newString: payload };
  if (raw) return { patchText: payload };
  return { patchText: patch(`*** Add File: ${file}`, ...payload.split('\n').map((l) => `+${l}`)) };
}

// Resolves to the thrown error, or null when the guard allowed the call.
async function run(tool, payload, opts) {
  const args = argsFor(tool, payload, opts);
  try {
    await handler({ tool }, { args });
    return null;
  } catch (err) {
    return err;
  }
}

// ── Fake secrets, one per shape the guard claims to detect ──────────────────
// unit repeated and cut to exactly n characters: a body of a given length.
const body = (unit, n) => unit.repeat(n).slice(0, n);
const SK = 'sk', AK = 'AKIA', AS = 'ASIA', GH = 'gh', GHP = 'github', XOX = 'xox', D5 = '-----';
const ANTHROPIC = `${SK}-ant-api03-${'FAKE_body-'.repeat(9)}AA`;
const OPENAI = `${SK}-${'0FAKE'.repeat(10)}`;
const AWS = `${AK}${'FAKE'.repeat(4)}`;
const AWS_TMP = `${AS}${'FAKE'.repeat(4)}`;
const GH_P = `${GH}p_${'0FAKE'.repeat(8)}`;
const GH_O = `${GH}o_${'0FAKE'.repeat(8)}`;
const GH_S = `${GH}s_${'0FAKE'.repeat(8)}`;
const OPENAI_NONE = `${SK}-None-${'0FAKE'.repeat(10)}`;
const OPENAI_PROJ = `${SK}-proj-${'FAKE_proj-body'.repeat(8)}`;
const OPENAI_SVC = `${SK}-svcacct-${'FAKE_svc-body'.repeat(8)}`;
const OPENAI_ADMIN = `${SK}-admin-${'FAKE_admin-body'.repeat(8)}`;
const GH_U = `${GH}u_${'0FAKE'.repeat(8)}`;
const GH_R = `${GH}r_${'0FAKE'.repeat(8)}`;
const GH_PAT = `${GHP}_pat_${body('0FAKE', 22)}_${body('FAKE0', 59)}`;
// A stateless installation token: prefix, app id, _, then a JWT.
const GH_S_JWT = `${GH}s_1536800_eyJ${'FAKE'.repeat(9)}.eyJ${'FAKE_'.repeat(30)}.${'FAKE-'.repeat(20)}`;
const SLACK_B = `${XOX}b-1234567890-1234567890123-${'FAKE'.repeat(6)}`;
const SLACK_P = `${XOX}p-1234567890-1234567890-1234567890123-${'0fake'.repeat(6)}`;
// A block with one 64-character line of fake base64 material, as PEM wraps it.
const pem = (kind) => `${D5}BEGIN ${kind}${D5}\n${body('MIIEFAKE0+/fake', 64)}\n${D5}END ${kind}${D5}`;
const PK_RSA = pem('RSA PRIVATE KEY');
const FILLER = 'an ordinary line of prose in a large generated file\n'.repeat(5000); // ~260 KB

// [tool, word the block message must name, payload, opts]
const BLOCK = [];

// Each pattern, via every tool the guard inspects.
for (const tool of ['write', 'edit', 'apply_patch']) {
  BLOCK.push(
    [tool, 'Anthropic', ANTHROPIC],
    [tool, 'OpenAI', OPENAI],
    [tool, 'OpenAI', OPENAI_NONE],
    [tool, 'OpenAI', OPENAI_PROJ],
    [tool, 'OpenAI', OPENAI_SVC],
    [tool, 'OpenAI', OPENAI_ADMIN],
    [tool, 'AWS', AWS],
    [tool, 'AWS', AWS_TMP],
    [tool, 'GitHub', GH_P],
    [tool, 'GitHub', GH_O],
    [tool, 'GitHub', GH_S],
    [tool, 'GitHub', GH_U],
    [tool, 'GitHub', GH_R],
    [tool, 'GitHub', GH_PAT],
    [tool, 'GitHub', GH_S_JWT],
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
  ['write', 'AWS', `AWS_ACCESS_KEY_ID=${AWS_TMP}`],
  ['edit', 'AWS', `credentials = {'AccessKeyId': '${AWS_TMP}'}`],
  // apply_patch: an added line in an update hunk, a heredoc-wrapped patch, CRLF line endings.
  ['apply_patch', 'Anthropic', patch('*** Update File: src/config.ts', '@@ export const config = {',
    '-  apiKey: process.env.ANTHROPIC_API_KEY,', `+  apiKey: "${ANTHROPIC}",`, ' };'), { raw: true }],
  ['apply_patch', 'AWS', `cat <<'EOF'\n${patch('*** Add File: creds.ini', `+aws_access_key_id = ${AWS_TMP}`)}\nEOF`, { raw: true }],
  ['apply_patch', 'private key', patch('*** Add File: id_rsa', ...PK_RSA.split('\n').map((l) => `+${l}`)).replace(/\n/g, '\r\n'), { raw: true }],
  ['apply_patch', 'GitHub', patch('*** Update File: a.txt', '*** Move to: b.txt', '@@', '-old', `+${GH_PAT}`), { raw: true }],
  // A template is exempt only for what it holds, not for its name.
  ['write', 'GitHub', `GITHUB_TOKEN=${GH_P}`, { file: '.env.example' }],
  // Serialized text puts a letter, digit, _ or - right before a key: an escape
  // in a quoted string, a percent-encoded character, a joined name.
  ['write', 'OpenAI', `{"keys": "old\\n${OPENAI_PROJ}"}`],
  ['edit', 'OpenAI', `https://example.com/callback?next=%2F&key%3D${OPENAI_SVC}`],
  ['write', 'OpenAI', `OPENAI_KEY_${OPENAI_ADMIN}`],
  ['write', 'OpenAI', `token-${OPENAI_PROJ}`],
  ['edit', 'OpenAI', `key1${OPENAI_SVC}`],
  ['write', 'AWS', `{"creds": "id\\n${AWS_TMP}"}`],
  ['edit', 'AWS', `https://sts.example.com/?AccessKeyId%3D${AWS_TMP}`],
  ['write', 'AWS', `{"id": "\\u002F${AWS_TMP}"}`],
  ['write', 'AWS', `id = '\\x2F${AWS_TMP}'`],
  ['edit', 'AWS', `${AWS_TMP}secretAccessKey`],
  ['write', 'GitHub', `GITHUB_TOKEN_${GH_P}_rotated_weekly`],
  ['edit', 'GitHub', `export GITHUB_TOKEN=${GH_S_JWT}`],
  ['write', 'GitHub', `${GH}u_eyJ${'FAKE'.repeat(5)}`],
  ['write', 'GitHub', `${GH}r_9_eyJ${'FAKE'.repeat(5)}`],
  ['write', 'GitHub', `${GH}s_12_34_eyJ${'FAKE'.repeat(5)}`],
  // A large write, secret first — the shell twin once allowed these (#101).
  ['write', 'OpenAI', `${OPENAI}\n${FILLER}`],
  ['edit', 'private key', `${PK_RSA}${FILLER}`],
  // A real key is never hidden by a placeholder next to it (#143).
  ['write', 'Slack', `${XOX}b-your-1234567890-${'FAKE'.repeat(6)}`],
  ['edit', 'Slack', `${XOX}b-${'x'.repeat(10)}-1234567890123-${'FAKE'.repeat(6)}`],
  ['write', 'Slack', `${XOX}b-your-token-${SLACK_B}`],
  ['write', 'Slack', `${XOX}b-your-token ${SLACK_B}`],
  ['write', 'AWS', `${AK}${body('FAKE', 16)}EXAMPLE`],
  ['edit', 'AWS', `${AK}${'X'.repeat(15)}F`],
  ['write', 'AWS', `id = ${AS}${'X'.repeat(15)}F`],
  ['write', 'AWS', `${AK}IOSFODNN7EXAMPLE ${AWS}`],
  ['write', 'AWS', `${AK}EXAMPLE${body('FAKE', 9)}`],
  ['write', 'GitHub', `${GH}p_${'x'.repeat(35)}F`],
  ['edit', 'GitHub', `${GH}p_${'x'.repeat(36)}F0FAKE`],
  ['write', 'GitHub', `${GH}p_${'x'.repeat(36)}${GH_P}`],
  ['write', 'OpenAI', `${SK}-${'x'.repeat(47)}F`],
  ['edit', 'OpenAI', `${SK}-None-${'x'.repeat(40)} ${OPENAI}`],
  ['write', 'Anthropic', `${SK}-ant-api03-${'x'.repeat(40)}FAKE`],
  ['edit', 'Anthropic', `${SK}-ant-your-key-FAKE0-${body('FAKE0', 40)}`],
  ['write', 'Anthropic', `${SK}-ant-your-key-here-${ANTHROPIC}`],
  ['write', 'OpenAI', `${SK}-proj-your-project-key-${'FAKE0_'.repeat(16)}FAKE0`],
  ['write', 'AWS', `id = ${AS}EXAMPLE${body('FAKE', 9)}`],
  // Only the whole body counts: "your" glued to more letters, or an unknown
  // key-type segment before a repeated run, is not a placeholder.
  ['edit', 'Anthropic', `${SK}-ant-your${'FAKEfake'.repeat(6)}`],
  ['write', 'OpenAI', `${SK}-proj-YOUR${'FAKEfake'.repeat(12)}`],
  ['write', 'Slack', `${XOX}b-your${'FAKEfake'.repeat(2)}`],
  ['write', 'Anthropic', `${SK}-ant-fake0-${'x'.repeat(60)}`],
  ['edit', 'OpenAI', `${SK}-proj-${'x'.repeat(80)}FAKE`],
  // A private-key header with key material, however it is written (#143).
  ['write', 'private key', `"${D5}BEGIN PRIVATE KEY${D5}\\n${body('MIIEFAKE0+/', 64)}\\n${D5}END PRIVATE KEY${D5}"`],
  ['edit', 'private key', `${D5}BEGIN RSA PRIVATE KEY${D5}\nProc-Type: 4,ENCRYPTED\nDEK-Info: AES-128-CBC,FAKE\n\n${body('MIIEFAKE0+/', 64)}`],
  ['write', 'private key', `${D5}BEGIN PGP PRIVATE KEY BLOCK${D5}\nVersion: FAKE\nComment: exported by a fake key tool for the devexp guard tests, not a real key\n\n${body('lQOYBFAKE0+/', 64)}`],
  ['write', 'private key', `key = "${D5}BEGIN RSA PRIVATE KEY${D5}\\n" +\n  "${body('MIIEFAKE0+/', 64)}\\n"`],
  ['edit', 'private key', `${D5}BEGIN PRIVATE KEY${D5}${body('MIIEFAKE0+/', 64)}`],
  ['write', 'private key', `${D5}BEGIN OPENSSH PRIVATE KEY${D5}\n${body('b3BlbnNzaFAKE0', 70)}`],
  ['write', 'private key', `Look for ${D5}BEGIN RSA PRIVATE KEY${D5} at the top.\n${PK_RSA}`],
  ['edit', 'private key', `${D5}BEGIN EC PRIVATE KEY${D5}\n${D5}END EC PRIVATE KEY${D5}\n${pem('EC PRIVATE KEY')}`],
  ['write', 'private key', `${D5}BEGIN RSA PRIVATE KEY${D5}\n${body('FAKE0+/', 32)}\n${D5}END RSA PRIVATE KEY${D5}`],
  // Length thresholds, pinned at the edge (the one-short twins are in ALLOW).
  ['write', 'Anthropic', `${SK}-ant-${body('FAKE_ant-', 40)}`],
  ['write', 'OpenAI', `${SK}-proj-${body('FAKE_proj-', 80)}`],
  ['write', 'OpenAI', `${SK}-None-${body('0FAKE', 32)}`],
  ['write', 'AWS', `${AS}${body('FAKE', 16)}`],
  ['write', 'GitHub', `${GH}u_${body('0FAKE', 36)}`],
  ['write', 'GitHub', `${GH}s_${body('0FAKE', 36)}`],
  ['write', 'GitHub', `${GHP}_pat_${body('0FAKE', 22)}_${body('FAKE0', 59)}`],
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
  ['edit', 'OPENAI_API_KEY = "sk-None-placeholder-for-local-dev"'],
  ['write', '<div class="desk-admin-navigation-sidebar-collapsed-state-controller">'],
  ['edit', 'const route = "/task-proj-onboarding-checklist-and-welcome-email-sequence";'],
  ['write', '<div class="sk-admin-navigation-sidebar-collapsed-state-controller">'],
  ['edit', 't("sk-proj-onboarding-checklist-welcome-email-sequence-step-three-title")'],
  ['write', 'sk-svcacct-rotation_reminder-banner-dismissed-at-timestamp-for-the-current-org: true'],
  ['edit', 'github_pat_rotation_reminder_days_for_organization_members_with_admin_access = 30'],
  ['write', 'const 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 = true;'],
  ['write', 'const REGION = "ASIAPACIFICDATACENTER01";'],
  ['edit', 'EURASIAPACIFICREGION024 = load_regions()'],
  // Long snake_case names holding a GitHub prefix: right_ holds ght_, highs_ holds
  // ghs_. Joined at runtime, since the guard blocked them before (#143).
  ['write', `def test_blocks_ri${GH}t_token_when_a_write_holds_a_long_snake_case_name(): pass`],
  ['edit', `hi${GH}s_and_lows_for_every_region_in_the_dataset_since_2000 = {}`],
  ['write', `const wei${GH}t_surveyJson_for_every_respondent_in_the_panel = load();`],
  ['edit', `ri${GH}t_eye_contact_duration_for_the_whole_recorded_session = 3`],
  ['write', 'Installation tokens now look like ghs_APPID_JWT.'],
  // Placeholders shaped like a key (#143): documented example values,
  // your-... phrases, one repeated character, <...>.
  ['write', `SLACK_BOT_TOKEN=${XOX}b-your-token`],
  ['edit', `slack_token: ${XOX}p-your-slack-user-token-here`],
  ['write', `SLACK_BOT_TOKEN=${XOX}b-${'x'.repeat(10)}-${'x'.repeat(13)}-${'x'.repeat(24)}`],
  ['write', `aws_access_key_id = ${AK}IOSFODNN7EXAMPLE`],
  ['write', `${AK}I44QH8DHBEXAMPLE`],
  ['edit', `AccessKeyId: ${AS}IOSFODNN7EXAMPLE`],
  ['write', `AWS_ACCESS_KEY_ID=${AK}${'X'.repeat(16)}`],
  ['edit', `AWS_ACCESS_KEY_ID=${AS}${'X'.repeat(16)}`],
  ['write', `GITHUB_TOKEN=${GH}p_${'x'.repeat(40)}`],
  ['write', `OPENAI_API_KEY=${SK}-${'x'.repeat(48)}`],
  ['edit', `OPENAI_API_KEY=${SK}-None-${'X'.repeat(48)}`],
  ['write', `ANTHROPIC_API_KEY=${SK}-ant-api03-${'x'.repeat(95)}`],
  ['write', `ANTHROPIC_API_KEY=${SK}-ant-${'X_'.repeat(30)}`],
  ['edit', `ANTHROPIC_API_KEY=${SK}-ant-your-anthropic-api-key-goes-here-and-stays-out-of-git`],
  ['write', `ANTHROPIC_API_KEY=${SK}-ant-YOUR_ANTHROPIC_API_KEY_GOES_HERE_PLEASE_THANKS`],
  ['write', `SLACK_BOT_TOKEN=${XOX}b-YOUR-BOT-TOKEN`],
  ['write', `OPENAI_API_KEY=${SK}-proj-${'x-'.repeat(60)}`],
  ['edit', `OPENAI_API_KEY=${SK}-svcacct-YOUR_SERVICE_ACCOUNT_KEY_GOES_HERE_AND_NEVER_INTO_A_COMMITTED_FILE_OR_ANY_BUILD_LOG`],
  ['write', `GITHUB_TOKEN=${GH}p_<your-token>`],
  ['write', `SLACK_BOT_TOKEN=${XOX}b-<your-bot-token>`],
  ['edit', `ANTHROPIC_API_KEY=${SK}-ant-<your-key> OPENAI_API_KEY=${SK}-proj-<project-key>`],
  ['write', `AWS_ACCESS_KEY_ID=${AK}<ACCESS_KEY_ID>`],
  // A private-key header with no key material (#143).
  ['write', `Keys in PKCS#1 form start with "${D5}BEGIN RSA PRIVATE KEY${D5}"; see https://docs.example.com/security/private-keys.html`],
  ['edit', `if line.startswith('${D5}BEGIN OPENSSH PRIVATE KEY${D5}'):\n    return True`],
  ['write', `${D5}BEGIN PRIVATE KEY${D5}\n<your private key>\n${D5}END PRIVATE KEY${D5}`],
  ['write', `${D5}BEGIN RSA PRIVATE KEY${D5}\nMIIEpAIBAAKCAQEA...\n${D5}END RSA PRIVATE KEY${D5}`],
  ['edit', `${D5}BEGIN EC PRIVATE KEY${D5}\n${D5}END EC PRIVATE KEY${D5}`],
  ['write', `PEM_HEADER = '${D5}BEGIN PRIVATE KEY${D5}'\n${pem('CERTIFICATE')}`],
  // Length thresholds, one character short of blocking.
  ['write', `${D5}BEGIN RSA PRIVATE KEY${D5}\n${body('FAKE0+/', 31)}\n${D5}END RSA PRIVATE KEY${D5}`],
  ['write', `${SK}-ant-${body('FAKE_ant-', 39)}`],
  ['write', `${SK}-proj-${body('FAKE_proj-', 79)}`],
  ['write', `${SK}-None-${body('0FAKE', 31)}`],
  ['write', `${AS}${body('FAKE', 15)}`],
  ['write', `${GH}u_${body('0FAKE', 35)}`],
  ['write', `${GH}s_${body('0FAKE', 35)}`],
  ['write', `${GH}p_${body('0FAKE', 35)}_${body('FAKE0', 40)}`],
  ['write', `${GHP}_pat_${body('0FAKE', 21)}_${body('FAKE0', 59)}`],
  ['write', `${GHP}_pat_${body('0FAKE', 23)}_${body('FAKE0', 59)}`],
  ['write', `${GHP}_pat_${body('0FAKE', 22)}_${body('FAKE0', 58)}`],
  ['write', `${GHP}_pat_${body('0FAKE', 81)}`],
  // Only the new text is scanned; an edit that takes a key out must not be refused.
  ['edit', 'OPENAI_API_KEY=process.env.OPENAI_API_KEY', { old: `OPENAI_API_KEY=${OPENAI}` }],
  // apply_patch writes only its "+" lines: removing a key, a key in unchanged
  // context, a deleted file and headers are not written content.
  ['apply_patch', patch('*** Update File: .env', '@@', `-OPENAI_API_KEY=${OPENAI}`, '+OPENAI_API_KEY='), { raw: true }],
  ['apply_patch', patch('*** Update File: .env', '@@', ` GITHUB_TOKEN=${GH_P}`, '+# rotated weekly'), { raw: true }],
  ['apply_patch', patch('*** Delete File: secrets.pem'), { raw: true }],
  ['apply_patch', TEMPLATE, { file: '.env.example' }],
  ['apply_patch', 'const token = process.env.GITHUB_TOKEN;'],
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
