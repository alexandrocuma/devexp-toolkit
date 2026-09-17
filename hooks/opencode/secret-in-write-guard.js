/**
 * secret-in-write-guard.js — hard-blocks writing content that contains secret patterns
 *
 * Event: tool.execute.before (tool: write | edit)
 *
 * Scans the content being written for high-signal secret patterns.
 * Complements secret-guard which checks filenames on read.
 *
 * Tests: node hooks/opencode/secret-in-write-guard.test.js
 * Mirror: hooks/claude-code/secret-in-write-guard.sh — keep the patterns in lockstep.
 */

const SECRET_PATTERNS = [
  { re: /sk-ant-[A-Za-z0-9_\-]{40,}/m,          label: 'Anthropic API key (sk-ant-...)' },
  { re: /sk-[A-Za-z0-9]{32,}/m,                  label: 'OpenAI API key (sk-...)' },
  // Project, service-account and admin keys: the body has _ and -, so the
  // prefix must start a word, or kebab-case ids like desk-admin-... match.
  { re: /(^|[^A-Za-z0-9_-])sk-(proj|svcacct|admin)-[A-Za-z0-9_-]{40,}/m, label: 'OpenAI API key (sk-...)' },
  { re: /AKIA[0-9A-Z]{16}/m,                     label: 'AWS Access Key ID' },
  { re: /gh[postaur]_[A-Za-z0-9_]{36,}/m,        label: 'GitHub token' },
  { re: /github_pat_[A-Za-z0-9_]{36,}/m,         label: 'GitHub token' },
  { re: /xox[baprs]-[0-9A-Za-z\-]{10,}/m,        label: 'Slack token' },
  { re: /-----BEGIN [A-Z ]*(PRIVATE|SECRET) KEY/m, label: 'private key block' },
];

export async function secretInWriteGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'write' && input.tool !== 'edit') return;

      // opencode's edit tool passes camelCase `newString`; `new_string` is Claude Code's name.
      const content = output.args?.content ?? output.args?.newString ?? output.args?.new_string ?? '';
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
