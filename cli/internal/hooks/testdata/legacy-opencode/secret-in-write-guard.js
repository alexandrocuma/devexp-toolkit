/**
 * secret-in-write-guard.js — hard-blocks writing content that contains secret patterns
 *
 * Event: tool.execute.before (tool: write | edit)
 *
 * Scans the content being written for high-signal secret patterns.
 * Complements secret-guard which checks filenames on read.
 */

const SECRET_PATTERNS = [
  { re: /sk-ant-[A-Za-z0-9_\-]{40,}/m,          label: 'Anthropic API key (sk-ant-...)' },
  { re: /sk-[A-Za-z0-9]{32,}/m,                  label: 'OpenAI API key (sk-...)' },
  { re: /AKIA[0-9A-Z]{16}/m,                     label: 'AWS Access Key ID' },
  { re: /gh[posta]_[A-Za-z0-9_]{36,}/m,          label: 'GitHub token' },
  { re: /xox[baprs]-[0-9A-Za-z\-]{10,}/m,        label: 'Slack token' },
  { re: /-----BEGIN [A-Z ]*(PRIVATE|SECRET) KEY/m, label: 'private key block' },
];

export async function secretInWriteGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'write' && input.tool !== 'edit') return;

      const content = output.args?.content ?? output.args?.new_string ?? '';
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
