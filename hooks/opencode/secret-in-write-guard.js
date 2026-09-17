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
 * lands on disk for a context line isn't scanned either. A private-key body
 * wrapped far narrower than the usual PEM line width, written in pieces
 * across several edits, or starting far from its header, isn't recognized as
 * key material. The guard catches
 * secrets written in one piece; it does not replace a secret scanner on the
 * repository.
 *
 * The scan runs under a wall-clock budget and refuses the call when it is
 * exceeded (#162) — see startScanBudget in utils.js.
 *
 * Tests: node hooks/opencode/secret-in-write-guard.test.js
 * Mirror: hooks/claude-code/secret-in-write-guard.sh — keep the patterns in lockstep.
 */

import { startScanBudget } from './utils.js';

// Placeholders (#143). The (?!...) right after a prefix skips a body that is
// only a placeholder: one character repeated (separators aside), a your-...
// phrase made of letter words, or a key ID ending in EXAMPLE, as in AWS's
// documentation. A skipped body must run to where the key's characters end,
// so key material can't hide inside one, and RegExp.test still tries every
// later start, so a real key next to or joined to a placeholder still blocks.
//
// Linear time. RegExp.test retries a pattern at every start, so a repetition
// that can run over the next prefix costs quadratic time or worse on text
// that repeats a prefix, and this handler runs synchronously. Every
// repetition that can span another start is bounded: phrase words,
// service-key name segments, GitHub id segments, and the private-key header
// and material search. The timing tests pin this.
const SECRET_PATTERNS = [
  { re: /sk-ant-(?!(?:(?:api|admin)[0-9]+-)?(?:([A-Za-z0-9])(?:\1|[_-])*|(?:your|YOUR)(?:[_-][A-Za-z]+){1,24})(?![A-Za-z0-9_-]))[A-Za-z0-9_-]{40,}/m, label: 'Anthropic API key (sk-ant-...)' },
  // Legacy user keys (some issued as sk-None-...) are no longer issued, but
  // existing ones can still be live.
  { re: /sk-(?:None-)?(?!([A-Za-z0-9])\1*(?![A-Za-z0-9]))[A-Za-z0-9]{32,}/m, label: 'OpenAI API key (sk-...)' },
  // Project, service-account and admin keys: the body has _ and -, like a
  // kebab-case id, so its length carries the signal. Real bodies are well
  // over 100 characters; ids like desk-admin-... are far shorter. No left
  // boundary: a key can follow an escape, a %XX, a _, a - or a digit.
  { re: /sk-(?:proj|svcacct|admin)-(?!(?:([A-Za-z0-9])(?:\1|[_-])*|(?:your|YOUR)(?:[_-][A-Za-z]+){1,24})(?![A-Za-z0-9_-]))[A-Za-z0-9_-]{80,}/m, label: 'OpenAI API key (sk-...)' },
  // Older service keys (#152): sk-service-, a service-account name, -, then
  // 48 alphanumerics (20, the T3BlbkFJ watermark, 20), per Trivy's
  // openai-service-api-key rule (aquasecurity/trivy#10798) and TruffleHog's
  // OpenAI detector. The name is optional and the watermark isn't required
  // here, so no real key is missed; the 48-character run keeps kebab-case
  // ids like sk-service-account-... from matching. The name is read one
  // _/- separated segment at a time, so no segment is split two ways.
  { re: /sk-service-(?!([A-Za-z0-9])(?:\1|[_-])*(?![A-Za-z0-9_-]))(?:[A-Za-z0-9]{0,47}[_-]){0,20}[A-Za-z0-9]{48}/m, label: 'OpenAI API key (sk-...)' },
  { re: /AKIA(?![0-9A-Z]{9}EXAMPLE|([0-9A-Z])\1{15})[0-9A-Z]{16}/m, label: 'AWS Access Key ID' },
  // Temporary (STS) key IDs are ASIA plus exactly 16 of [0-9A-Z], so a
  // boundary is any character outside that set: EURASIA... and
  // ASIAPACIFICDATACENTER01 don't match, while a key next to a lowercase
  // letter (an escape like \n) does. On the left, an encoded character
  // that ends in an uppercase hex digit (%2F, \u002F, \x2F) also counts;
  // \x5c is a backslash. A key run straight into more capitals or digits
  // reads as a longer word and is not matched.
  { re: /(^|[^0-9A-Z]|%[0-9A-Fa-f]{2}|\x5cu[0-9A-Fa-f]{4}|\x5cx[0-9A-Fa-f]{2})ASIA(?![0-9A-Z]{9}EXAMPLE|([0-9A-Z])\2{15})[0-9A-Z]{16}([^0-9A-Z]|$)/m, label: 'AWS temporary Access Key ID' },
  // Classic tokens: 36 or more alphanumerics, never _ (GitHub's token
  // formats docs; gitleaks and Trivy match the same class), so snake_case
  // names that contain a prefix like ght_ don't match.
  { re: /gh[postaur]_(?!([A-Za-z0-9])\1*(?![A-Za-z0-9]))[A-Za-z0-9]{36,}/m, label: 'GitHub token' },
  // Stateless tokens (installation tokens since 2026-04-27, GITHUB_TOKEN
  // included) are the prefix, an app id and _, then a JWT, and a JWT
  // starts eyJ: github.blog/changelog/2026-05-15-github-app-installation-
  // tokens-per-request-override-header. Any prefix, with a few id segments,
  // since GitHub says other token types may follow.
  { re: /gh[postaur]_(?:[A-Za-z0-9]+_){0,8}eyJ/m, label: 'GitHub token' },
  // Fine-grained tokens have a fixed shape: 22 alphanumerics, _, 59 more.
  // Anything looser blocks long snake_case names that start github_pat_.
  { re: /github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}/m, label: 'GitHub token' },
  { re: /xox[baprs]-(?!(?:([0-9A-Za-z])(?:\1|-)*|(?:your|YOUR)(?:-[A-Za-z]+){1,24})(?![0-9A-Za-z-]))[0-9A-Za-z-]{10,}/m, label: 'Slack token' },
  // A private key block is its header followed by key material: a run of
  // 32 base64 characters (PEM wraps lines at 64, OpenSSH at 70) that starts
  // within a few hundred characters of the header, before any -----END or
  // -----BEGIN line. That window leaves room for encrypted-PEM and PGP armor
  // headers. A header quoted alone, or with an elided or placeholder body,
  // has no material and is allowed (#143); key-like text on the header's
  // line or just after it still blocks. Escaped bodies (JSON, code strings)
  // count too: an escaped slash (\x5c/) is part of a line, and an escaped
  // newline (\x5cn, \x5cr\x5cn) joins two lines of 16 or more characters.
  // Other backslashes, as in Windows paths or \x5cu escapes, split a run,
  // and an escape's letter never starts one.
  { re: /-----BEGIN [A-Z ]{0,40}(PRIVATE|SECRET) KEY(?:(?!-----(?:END|BEGIN))[\s\S]){0,500}?(?<!\x5c)(?:(?:[A-Za-z0-9+/]|\x5c\/){32}|(?:[A-Za-z0-9+/]|\x5c\/){16,31}(?:\x5cr)?\x5cn(?:[A-Za-z0-9+/]|\x5c\/){16})/m, label: 'private key block' },
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

      // The budget starts before the text is even assembled: reading a patch
      // apart is work too, and a guard that runs long must block, not linger.
      const overBudget = startScanBudget('secret-in-write-guard');
      overBudget();

      const content = writtenText(input.tool, output.args ?? {});
      if (!content) return;

      for (const { re, label } of SECRET_PATTERNS) {
        overBudget();
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
