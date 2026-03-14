/**
 * dangerous-cmd-guard.js — blocks or warns on destructive shell commands
 *
 * Event: tool.execute.before (tool: bash)
 *
 * Hard block (throw):  rm -rf /, fork bomb, DROP DATABASE
 * Soft block (throw):  git push --force, git reset --hard, git clean, DROP/TRUNCATE TABLE
 */

const HARD_BLOCK_PATTERNS = [
  {
    re: /rm\s+-[a-z]*r[a-z]*f\s+(\/\s*$|\/\s+|~\/?(\s|$)|\$HOME(\s|$))/m,
    label: "'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory",
  },
  {
    re: /rm\s+-[a-z]*f[a-z]*r\s+(\/\s*$|\/\s+|~\/?(\s|$)|\$HOME(\s|$))/m,
    label: "'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory",
  },
  {
    re: /:\s*\(\s*\)\s*\{.*\|.*:/m,
    label: 'fork bomb pattern detected',
  },
  {
    re: /DROP\s+DATABASE/im,
    label: 'DROP DATABASE would permanently destroy a database',
  },
];

const SOFT_BLOCK_PATTERNS = [
  {
    re: /git\s+push\b.*?(--force|-f\b|--force-with-lease)/m,
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

      for (const { re, label } of HARD_BLOCK_PATTERNS) {
        if (re.test(command)) {
          throw new Error(`[devexp dangerous-cmd-guard] Blocked: ${label}.`);
        }
      }

      for (const { re, label } of SOFT_BLOCK_PATTERNS) {
        if (re.test(command)) {
          throw new Error(
            `[devexp dangerous-cmd-guard] Blocked high-risk command: ${label}. ` +
            `Confirm with the user that this is intentional before proceeding.`
          );
        }
      }
    },
  };
}
