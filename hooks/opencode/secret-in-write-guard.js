/**
 * secret-in-write-guard.js — hard-blocks writing content that contains secret patterns
 *
 * Event: tool.execute.before (tool: write | edit | apply_patch)
 *
 * Scans the text being written for high-signal secret patterns: write
 * `content`, edit `newString`, and the lines apply_patch adds (opencode offers
 * apply_patch instead of write/edit to GPT models). Text being replaced or
 * removed is never scanned, so an edit that takes a key out is allowed.
 * Complements secret-guard which checks filenames on read.
 *
 * Known limitations: only the new text of an edit or patch is scanned, never
 * the file text around it, so a secret completed across existing file text
 * and an edit is not seen. For apply_patch, opencode matches context lines
 * against the file loosely and writes the patch's own text for them, so what
 * lands on disk for a context line isn't scanned either. The guard catches
 * secrets written in one piece; it does not replace a secret scanner on the
 * repository.
 *
 * Tests: node hooks/opencode/secret-in-write-guard.test.js
 * Mirror: hooks/claude-code/secret-in-write-guard.sh — keep the patterns in lockstep.
 */

const SECRET_PATTERNS = [
  { re: /sk-ant-[A-Za-z0-9_\-]{40,}/m,          label: 'Anthropic API key (sk-ant-...)' },
  // Legacy user keys (some issued as sk-None-...) are no longer issued, but
  // existing ones can still be live.
  { re: /sk-(None-)?[A-Za-z0-9]{32,}/m,          label: 'OpenAI API key (sk-...)' },
  // Project, service-account and admin keys: the body has _ and -, like a
  // kebab-case id, so its length carries the signal. Real bodies are well
  // over 100 characters; ids like desk-admin-... are far shorter. No left
  // boundary: a key can follow an escape, a %XX, a _, a - or a digit.
  { re: /sk-(proj|svcacct|admin)-[A-Za-z0-9_\-]{80,}/m, label: 'OpenAI API key (sk-...)' },
  { re: /AKIA[0-9A-Z]{16}/m,                     label: 'AWS Access Key ID' },
  // Temporary (STS) key IDs are ASIA plus exactly 16 of [0-9A-Z], so a
  // boundary is any character outside that set: EURASIA... and
  // ASIAPACIFICDATACENTER01 don't match, while a key next to a lowercase
  // letter (an escape like \n) does. On the left, an encoded character
  // that ends in an uppercase hex digit (%2F, \u002F, \x2F) also counts;
  // \x5c is a backslash. A key run straight into more capitals or digits
  // reads as a longer word and is not matched.
  { re: /(^|[^0-9A-Z]|%[0-9A-Fa-f]{2}|\x5cu[0-9A-Fa-f]{4}|\x5cx[0-9A-Fa-f]{2})ASIA[0-9A-Z]{16}([^0-9A-Z]|$)/m, label: 'AWS temporary Access Key ID' },
  // Classic tokens: 36 or more alphanumerics, never _ (GitHub's token
  // formats docs; gitleaks and Trivy match the same class), so snake_case
  // names that contain a prefix like ght_ don't match.
  { re: /gh[postaur]_[A-Za-z0-9]{36,}/m,         label: 'GitHub token' },
  // Stateless tokens (installation tokens since 2026-04-27, GITHUB_TOKEN
  // included) are the prefix, an app id and _, then a JWT, and a JWT
  // starts eyJ: github.blog/changelog/2026-05-15-github-app-installation-
  // tokens-per-request-override-header. Any prefix, with any number of id
  // segments, since GitHub says other token types may follow.
  { re: /gh[postaur]_(?:[A-Za-z0-9]+_)*eyJ/m,    label: 'GitHub token' },
  // Fine-grained tokens have a fixed shape: 22 alphanumerics, _, 59 more.
  // Anything looser blocks long snake_case names that start github_pat_.
  { re: /github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}/m, label: 'GitHub token' },
  { re: /xox[baprs]-[0-9A-Za-z\-]{10,}/m,        label: 'Slack token' },
  { re: /-----BEGIN [A-Z ]*(PRIVATE|SECRET) KEY/m, label: 'private key block' },
];

// The file content an apply_patch call writes: every line starting with "+",
// which is each "*** Add File" body line and each added line of an
// "*** Update File" hunk. Removed ("-") and context (" ") lines, "@@" anchors
// and "***" headers are not scanned (see the header on context lines).
export function patchAddedText(patchText) {
  if (typeof patchText !== 'string') return '';
  return patchText
    .split('\n')
    .filter((line) => line.startsWith('+'))
    .map((line) => line.slice(1))
    .join('\n');
}

function writtenText(tool, args = {}) {
  if (tool === 'apply_patch') return patchAddedText(args.patchText);
  // opencode's edit tool passes camelCase `newString`; `new_string` is Claude Code's name.
  return args.content ?? args.newString ?? args.new_string ?? '';
}

export async function secretInWriteGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'write' && input.tool !== 'edit' && input.tool !== 'apply_patch') return;

      const content = writtenText(input.tool, output.args ?? {});
      if (!content) return;

      for (const { re, label } of SECRET_PATTERNS) {
        if (re.test(content)) {
          throw new Error(
            `[devexp secret-in-write-guard] Blocked: content appears to contain ${label}. ` +
            `Remove the secret before writing.`
          );
        }
      }
    },
  };
}
