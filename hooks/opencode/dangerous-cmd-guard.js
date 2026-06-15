/**
 * dangerous-cmd-guard.js — hard-blocks destructive shell commands
 *
 * Event: tool.execute.before (tool: bash)
 *
 * All guarded patterns are hard-blocked (throw). No prompts.
 */

export const BLOCK_PATTERNS = [
  {
    re: /rm\s+-[a-z]*r[a-z]*f\s+(\/\s*$|\/\s+|~\/?(\s|$)|\$HOME(\s|$))/m,
    label: "'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory",
  },
  {
    re: /rm\s+-[a-z]*f[a-z]*r\s+(\/\s*$|\/\s+|~\/?(\s|$)|\$HOME(\s|$))/m,
    label: "'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory",
  },
  {
    // Unanchored wildcard delete in a sensitive dir (/tmp/* , ~/.claude/.../* , or the dir
    // wholesale) — the blanket wipe an empty variable produces. Prefix-anchored globs like
    // /tmp/.deliver-PAY-123-* are allowed (no '*' right after the '/').
    re: /rm\b[^|]*(\s\/tmp\/\*|\s\/tmp\/?(\s|$)|\.claude\S*\/\*|(\$HOME|~)\/\.claude\/?(\s|$))/m,
    label:
      "unanchored wildcard delete in a sensitive directory (e.g. '/tmp/*' or '~/.claude/.../*') — anchor the glob with a literal prefix like '/tmp/.deliver-<id>-*' so an empty variable cannot collapse it into a blanket wipe",
  },
  {
    re: /:\s*\(\s*\)\s*\{.*\|.*:/m,
    label: 'fork bomb pattern detected',
  },
  {
    re: /DROP\s+DATABASE/im,
    label: 'DROP DATABASE would permanently destroy a database',
  },
  {
    // Force flag must be an argument of the same push command (no intervening ; | & ), so an
    // unrelated `-f` elsewhere (e.g. `rm -f` in a commit message) no longer false-positives.
    re: /git\s+push\b[^|&;]*\s(--force-with-lease|--force|-f)(\s|=|$)/m,
    label: 'git push --force can overwrite remote history and affect other contributors',
  },
  {
    re: /git\s+reset\b.*?--hard/m,
    label: 'git reset --hard will permanently discard all uncommitted changes',
  },
  {
    re: /git\s+clean\b.*?-[a-z]*f/m,
    label: 'git clean -f will permanently delete untracked files',
  },
  {
    re: /DROP\s+TABLE/im,
    label: 'DROP TABLE will permanently destroy table data',
  },
  {
    re: /TRUNCATE\s+TABLE/im,
    label: 'TRUNCATE TABLE will permanently destroy table data',
  },
];

export async function dangerousCmdGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'bash') return;

      const command = output.args?.command ?? '';
      if (!command) return;

      for (const { re, label } of BLOCK_PATTERNS) {
        if (re.test(command)) {
          throw new Error(`[devexp dangerous-cmd-guard] Blocked: ${label}.`);
        }
      }
    },
  };
}
