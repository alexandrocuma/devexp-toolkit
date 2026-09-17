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
// An older service key: a service-account name, then 20 + watermark + 20.
const OPENAI_SERVICE = `${SK}-service-fake-svc-${body('FAKE0', 20)}T3BlbkFJ${body('0FAKE', 20)}`;
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
// A few paragraphs of ordinary text, as the shell twin builds them (no final newline).
const PROSE = 'This key is used by the deploy job to reach the staging hosts over SSH.\n'.repeat(8).slice(0, -1);
const CODE = "  logger.debug('reading the key file for the deploy service');\n".repeat(10).slice(0, -1);

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
    [tool, 'OpenAI', OPENAI_SERVICE],
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
  ['write', 'OpenAI', `OPENAI_API_KEY=${OPENAI_SERVICE}`],
  ['edit', 'OpenAI', `${SK}-service-${body('0FAKE', 48)}`],
  ['write', 'OpenAI', `${SK}-service-fake_svc_${body('0FAKE', 48)}`],
  ['edit', 'OpenAI', `${SK}-service-your-service-key-${body('0FAKE', 48)}`],
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
  // A dash rule between header and body doesn't end the search; only an END or
  // BEGIN line does (#158).
  ...['RSA PRIVATE KEY', 'PRIVATE KEY', 'EC PRIVATE KEY', 'DSA PRIVATE KEY', 'ENCRYPTED PRIVATE KEY', 'OPENSSH PRIVATE KEY', 'PGP PRIVATE KEY BLOCK', 'PGP SECRET KEY BLOCK']
    .map((kind) => ['write', 'private key', `${D5}BEGIN ${kind}${D5}\n${D5}\n${body('MIIEFAKE0+/', 64)}\n${D5}END ${kind}${D5}`]),
  // JSON that escapes every slash still carries the key material.
  ['write', 'private key', `{"key": "${D5}BEGIN PRIVATE KEY${D5}\\n${body('MIIEFAKE0abcdefgh+\\/', 64)}\\n${body('fakeFAKE0123456+\\/', 64)}\\n${D5}END PRIVATE KEY${D5}"}`],
  // Key-like text on the header's line, or just after it, still blocks.
  ['edit', 'private key', `if (pem.startsWith("${D5}BEGIN PRIVATE KEY${D5}")) return parsePkcs8PrivateKeyFromPemEncodedString(pem);`],
  // Bounded repetitions (#158), pinned at the edge (the one-past twins are in ALLOW).
  ['write', 'private key', `${D5}BEGIN RSA PRIVATE KEY${D5}\n${' '.repeat(494)}${body('FAKE0+/', 32)}`],
  ['write', 'private key', `${D5}BEGIN ${body('ABC ', 40)}PRIVATE KEY${D5}\n${body('MIIEFAKE0+/', 64)}`],
  ['write', 'Anthropic', `${SK}-ant-your${'-a'.repeat(25)}`],
  ['write', 'OpenAI', `${SK}-proj-your${'-abc'.repeat(25)}`],
  ['write', 'Slack', `${XOX}b-your${'-a'.repeat(25)}`],
  ['write', 'OpenAI', `${SK}-service-${'ab-'.repeat(20)}${body('0FAKE', 48)}`],
  ['write', 'OpenAI', `${SK}-service-${body('abcFAKE', 47)}-${body('0FAKE', 48)}`],
  ['write', 'GitHub', `${GH}s_${'1_'.repeat(8)}eyJ${'FAKE'.repeat(5)}`],
  // Length thresholds, pinned at the edge (the one-short twins are in ALLOW).
  ['write', 'Anthropic', `${SK}-ant-${body('FAKE_ant-', 40)}`],
  ['write', 'OpenAI', `${SK}-proj-${body('FAKE_proj-', 80)}`],
  ['write', 'OpenAI', `${SK}-service-fake-svc-${body('0FAKE', 48)}`],
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
  ['write', 'sk-service-account-rotation-reminder-banner-dismissed-at-timestamp-for-the-current-org: true'],
  ['edit', 't("sk-service-worker-registration-failed-offline-fallback-page-title-for-every-locale")'],
  ['write', `OPENAI_API_KEY=${SK}-service-${'x'.repeat(60)}`],
  ['edit', `OPENAI_API_KEY=${SK}-service-your-service-account-key-goes-here`],
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
  // A quoted header, whatever the rest of the write holds (#158). Only text near
  // the header counts as its key material, and the search ends at an END or
  // BEGIN line, so a later identifier, fingerprint, path or hash doesn't block.
  ['write', `if (pem.startsWith("${D5}BEGIN PRIVATE KEY${D5}")) {\n${CODE}\n  return parsePkcs8PrivateKeyFromPemEncodedString(pem);\n}`],
  ['edit', `Paste the key (it starts with ${D5}BEGIN OPENSSH PRIVATE KEY${D5}).\n\n${PROSE}\nThe host fingerprint is SHA256:${body('FAKEfake0+/', 43)}.`],
  ['write', `Store the ${D5}BEGIN EC PRIVATE KEY${D5} file on the host.\n${PROSE}\nPath: /home/deploy/configuration/secrets/keys/`],
  ['write', `${D5}BEGIN PRIVATE KEY${D5}\n<paste your private key here>\n${D5}END PRIVATE KEY${D5}\nchecksum: ${body('0123456789abcdef', 64)}`],
  ['edit', `Header: ${D5}BEGIN RSA PRIVATE KEY${D5}\n\n${PROSE}${PROSE}\nFixed in commit ${body('0123456789abcdef', 40)}.`],
  // Bounded repetitions (#158), one past the edge.
  ['write', `${D5}BEGIN RSA PRIVATE KEY${D5}\n${' '.repeat(495)}${body('FAKE0+/', 32)}`],
  ['write', `${D5}BEGIN ${body('ABC ', 41)}PRIVATE KEY${D5}\n${body('MIIEFAKE0+/', 64)}`],
  ['write', `${SK}-ant-your${'-a'.repeat(24)}`],
  ['write', `${SK}-proj-your${'-abc'.repeat(24)}`],
  ['write', `${XOX}b-your${'-a'.repeat(24)}`],
  ['write', `${SK}-service-${'ab-'.repeat(21)}${body('0FAKE', 48)}`],
  ['write', `${GH}s_${'1_'.repeat(9)}eyJ${'FAKE'.repeat(5)}`],
  // Length thresholds, one character short of blocking.
  ['write', `${D5}BEGIN RSA PRIVATE KEY${D5}\n${body('FAKE0+/', 31)}\n${D5}END RSA PRIVATE KEY${D5}`],
  ['write', `${SK}-ant-${body('FAKE_ant-', 39)}`],
  ['write', `${SK}-proj-${body('FAKE_proj-', 79)}`],
  ['write', `${SK}-service-fake-svc-${body('0FAKE', 47)}`],
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

// ── Timing: every pattern decides in linear time (#158) ─────────────────────
// opencode runs this handler synchronously, so a slow pattern stalls the
// session. These writes repeat a prefix or a header so that an unbounded
// repetition would rescan from every start. Each runs in a child process that
// is killed at its budget, so a slow module fails instead of hanging the suite.
// The 100 KB tier stops at its first failure; the 2 MB tier runs only when the
// first tier passed. Budgets cover the handler only, not starting node.
const TIMING_SHAPES = `
  const rep = (unit, n) => unit.repeat(Math.ceil(n / unit.length)).slice(0, n);
  const D5 = '-'.repeat(5), SK = 's' + 'k', GH = 'g' + 'h';
  export const SHAPES = {
    'key-type words after a private-key header': (n) => D5 + 'BEGIN ' + rep('PRIVATE KEY ', n),
    'private-key header, then spaced capitals': (n) => D5 + 'BEGIN PRIVATE KEY' + rep(' A', n),
    'private-key headers between near-material runs': (n) => rep(D5 + 'BEGIN RSA PRIVATE KEY' + D5 + '\\n' + rep('A'.repeat(31) + '.', 600), n),
    'repeated service-key prefix': (n) => rep(SK + '-service-', n),
    'service-key prefixes with near-length name segments': (n) => rep(SK + '-service-' + rep('a'.repeat(47) + '-', 200), n),
    'repeated Anthropic prefix and your-': (n) => rep(SK + '-ant-your-', n),
    'repeated project-key prefix and your-': (n) => rep(SK + '-proj-your-', n),
    'repeated GitHub prefix': (n) => rep(GH + 'p_', n),
    'repeated GitHub prefix and id segment': (n) => rep(GH + 's_1_', n),
  };`;
const { spawnSync } = await import('node:child_process');
const moduleUrl = new URL('./secret-in-write-guard.js', import.meta.url).href;
const timeOne = (name, size) => {
  const child = `
    const { SHAPES } = await import('data:text/javascript,' + encodeURIComponent(${JSON.stringify(TIMING_SHAPES)}));
    const { secretInWriteGuard } = await import(${JSON.stringify(moduleUrl)});
    const h = (await secretInWriteGuard({}))['tool.execute.before'];
    const content = SHAPES[${JSON.stringify(name)}](${size});
    const t0 = performance.now();
    try { await h({ tool: 'write' }, { args: { filePath: 'big.txt', content } }); } catch {}
    process.stdout.write(String(performance.now() - t0));`;
  return child;
};
const { SHAPES: TIMED } = await import('data:text/javascript,' + encodeURIComponent(TIMING_SHAPES));
let timed = 0;
let timingFail = 0;
for (const [tier, size, budgetMs] of [['100 KB', 100_000, 250], ['2 MB', 2_000_000, 2000]]) {
  for (const name of Object.keys(TIMED)) {
    const p = spawnSync(process.execPath, ['--input-type=module', '-e', timeOne(name, size)], { timeout: budgetMs + 5000, encoding: 'utf8' });
    const ms = Number(p.stdout);
    timed++;
    if (p.error || p.status !== 0 || !(ms <= budgetMs)) {
      const why = p.error ? 'still running at the budget' : p.status !== 0 ? `exited ${p.status}: ${p.stderr.trim().slice(0, 200)}` : `took ${ms.toFixed(0)} ms`;
      console.log(`FAIL timing ${tier}: ${name}: ${why}, budget ${budgetMs} ms`);
      fail++;
      timingFail++;
      if (tier === '100 KB') break;
    }
  }
  if (timingFail) break;
}

const total = BLOCK.length + ALLOW.length + OTHER_TOOLS.length + timed;
console.log(`${total - fail} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
