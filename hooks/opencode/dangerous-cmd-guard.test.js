/**
 * Tests for dangerous-cmd-guard.js — mirrors hooks/claude-code/dangerous-cmd-guard.test.sh.
 * Run: node hooks/opencode/dangerous-cmd-guard.test.js
 */
import { blockReason, dangerousCmdGuard } from './dangerous-cmd-guard.js';

const blocked = (cmd) => blockReason(cmd) !== null;

const BLOCK = [
  'rm -f /tmp/*',
  'rm -rf /tmp',
  'rm -rf /tmp/',
  'rm -f /tmp/*"$ticket"*',
  'rm -rf ~/.claude',
  'rm -rf $HOME/.claude',
  'rm -f ~/.claude/agent-memory/grooming-agent/sessions/*"$id"*',
  'rm -f ~/.claude/agent-memory/*',
  'rm -rf /',
  'git push --force',
  'git push origin main -f',
  'git push --force-with-lease',
  "rm -rf '/tmp'/*",
  'rm -rf "/tmp"/*',
  'rm -rf "$HOME"/.claude',
  'git commit -m x && git push --force',

  // #100: a real invocation, in every command position
  'git status && git reset --hard',
  'git status; git reset --hard HEAD~1',
  'false || git reset --hard',
  'yes | git clean -fd',
  'git status\ngit reset --hard',
  '(git reset --hard)',
  '{ git reset --hard; }',
  'if true; then git reset --hard; fi',
  'echo "$(git reset --hard)"',
  'x=$(git reset --hard)',
  'echo `git reset --hard`',
  '$(echo git reset --hard)',
  'git commit -m "$(git reset --hard)"',
  'sudo rm -rf /',
  'sudo -u root git reset --hard',
  'env GIT_DIR=.git git reset --hard',
  'FOO=1 git push --force',
  'nohup git push --force &',
  'time git reset --hard',
  'command git clean -fd',
  'exec git reset --hard',
  'find . -name x | xargs rm -rf /',
  'timeout 5 git push origin main --force',
  "ssh host 'git reset --hard'",
  'psql -c "DROP TABLE users"',
  '--help || git reset --hard', // command text starting with a dash
  'echo a#b; git reset --hard', // mid-word # is not a comment
  'echo ${#x}; git reset --hard',
  'echo hi # note\ngit push --force', // a comment ends at the newline

  // #100: a string handed to a shell or evaluator
  'bash -c "git reset --hard"',
  "sh -c 'git clean -fdx'",
  'eval "git reset --hard"',
  "bash <<'EOF'\ngit reset --hard\nEOF",
  "cat <<'EOF' | sh\ngit reset --hard\nEOF",
  'cat <<EOF\n$(git reset --hard)\nEOF', // unquoted heredoc runs $(...)
  "cat <<'EOF' > run.sh\ngit reset --hard\nEOF",
  'echo "git reset --hard" | sh',
  'echo "git reset --hard" |\n  bash',
  "printf 'git reset --hard\\n' | bash",
  'echo "git reset --hard" |& sh',
  'echo "git clean -fd" | xargs -I{} sh -c {}',
  "echo 'DROP DATABASE prod' | psql",
  'echo "git reset --hard" > run.sh',
  'echo "git reset --hard" >> run.sh',
  'echo "git reset --hard" | tee run.sh',
  'echo "git reset --hard" 2>err.log >&2',
  'echo "git reset --hard" > >(sh)',
  'bash <(echo "git reset --hard")',
  "printf -v c 'git reset --hard'; $c",
  'git commit -m "git reset --hard" | sh',
  '{ echo "git reset --hard"; } | sh',
  '(echo "git reset --hard") | sh',
  'for i in 1; do echo "git reset --hard"; done | sh',
  '{ echo "git reset --hard"; } > run.sh',
  '(echo "git reset --hard") | tee run.sh',
  'if true; then echo "git reset --hard"; fi >> run.sh',
  'alias echo=sh; echo "git reset --hard"',
  'f() { echo "git reset --hard"; }; f | sh',
  'exec > run.sh; echo "git reset --hard"',
  'echo "git reset --hard" | pbcopy', // the clipboard is not a sink
  'echo "git reset --hard" | uniq - run.sh', // uniq writes its output file

  // #100: masked text read back and run in the same command
  'git commit -m "git reset --hard"; git log -1 --format=%B | sh',
  'gh issue create --body "git push --force now"; gh issue view 1 --json body -q .body | bash',
  'git commit -m "git reset --hard" && ./run.sh',
  'echo "git reset --hard"; "$SHELL" run.sh',
  'echo "git reset --hard"; . run.sh',
  'echo "git reset --hard"; timeout 5 python3 run.py',

  // the target right before a closing quote, paren or operator
  // (v0.9.0 required whitespace or end of line after the target, so these ran)
  "sh -c 'rm -rf /'",
  'bash -c "rm -rf ~"',
  'bash -c "rm -fr $HOME"',
  "eval 'git push --force'",
  'eval "git push origin main -f"',
  "ssh host 'rm -rf ~'",
  "su -c 'git push --force-with-lease' deploy", // extra args after the string
  "echo x | xargs sh -c 'rm -rf /'",
  "sh -c 'rm -rf /' _ extra",
  'bash -c "git push origin main --force" -- arg',
  'echo "$(rm -rf ~)"',
  'echo `rm -rf /`',
  '(rm -rf /tmp)',
  'x=$(rm -rf ~/.claude)',
  'git push --force;',
  'git push -f&& echo done',
  'rm -rf /|| true',
  'rm -rf "/"',
  'rm -rf "$HOME"',

  // a command split across backslash-continued lines
  'rm -rf \\\n  /',
  'git push origin \\\n  --force-with-lease',
  "sh -c 'git push origin main \\\n  --force'",
  'bash -c "rm -rf \\\n  ~"',

  // #100: what the parser cannot classify is scanned whole
  'echo "git reset --hard', // unbalanced quote
  'echo "$(git reset --hard"', // unbalanced substitution
  "cat <<'EOF'\ngit reset --hard", // unterminated heredoc
  'echo "$(case x in a) git reset --hard;; esac)"',
  'echo `echo \\`git reset --hard\\``',
  'echo $((1)) "git reset --hard"',

  // #100: large input (many short lines)
  `git reset --hard; echo ${'a\n'.repeat(125000)}`,
  `echo "${'a\n'.repeat(125000)}" && git push --force`,
  `echo "git reset --hard ${'a\n'.repeat(125000)}`, // unbalanced: scanned whole
];

const ALLOW = [
  'rm -f /tmp/.deliver-PAY-123-*',
  'rm -f /tmp/.recently_changed.txt',
  'rm -f ~/.claude/agent-memory/grooming-agent/plans/PAY-123.md',
  'rm -f ~/.claude/agent-memory/grooming-agent/sessions/PAY-123-*',
  'git push origin main',
  'git commit -m "fix -f false positive" ; git push',
  'git push && rm -f /tmp/.deliver-X-1',
  'rm -rf node_modules',
  'rm -rf ./dist',

  // the wider boundary does not swallow benign targets
  "sh -c 'rm -rf ./build'",
  'bash -c "git push origin main"',
  "eval 'rm -f /tmp/.deliver-X-1'",
  '(rm -rf node_modules)',
  'rm -rf ~/projects/old-build;',
  'bash -c "rm -rf /var/tmp/build"',
  'git push --follow-tags;',
  "ssh host 'git push origin main'",
  'git push origin \\\n  main',

  // #100: a mention is not an invocation
  'echo "  LOCAL ONLY — remote untouched. Tag still revertible:"\necho "    git reset --hard origin/main"     # <- text inside a quoted echo',
  "echo 'rm -rf / wipes everything'",
  'echo git push --force',
  `printf '%s\\n' "git clean -fd"`,
  'echo "DROP TABLE users" >&2',
  'echo "git reset --hard" > /dev/null 2>&1',
  'echo "rm -rf ~ later" | grep rm',
  'echo "$(echo git reset --hard)"',
  '(echo "git reset --hard")',
  'if true; then echo "git reset --hard"; fi',
  'sudo echo "git reset --hard"',
  'env FOO=1 echo "git push --force now"',
  `command printf '%s' "git reset --hard"`,
  'echo "git push --force origin" && git push',
  'git commit -m "docs: never run git reset --hard"',
  'git commit -am "note: git clean -fd removes untracked files"',
  'git tag -a v1 --message="undo with git reset --hard"',
  `git commit -m "$(cat <<'EOF'\nfix: guard no longer blocks git push --force mentions\nEOF\n)"`,
  'gh issue create --title "guard blocks git reset --hard" --body "e.g. rm -rf / first"',
  "gh pr create --title x --body-file - <<'EOF'\nRun git clean -fd first.\nEOF",
  "cat <<'EOF'\nrun git reset --hard to discard\nEOF",
  'cat <<-EOF\n\tDROP DATABASE is irreversible\n\tEOF',
  "cat <<'EOF' | grep reset\ngit reset --hard\nEOF",
  'git status  # never git reset --hard here',
  '# git push --force\ngit push',

  // #100: large input
  `echo "${'a\n'.repeat(125000)}"`,
];

const show = (c) => (c.length > 120 ? `${c.slice(0, 60)}…(${c.length} chars)` : c).replace(/\n/g, '\\n');

let fail = 0;
for (const c of BLOCK) if (!blocked(c)) { console.log('FAIL want block:', show(c)); fail++; }
for (const c of ALLOW) if (blocked(c)) { console.log('FAIL want allow:', show(c)); fail++; }

// The hook entry makes the same decision as the predicate.
const hooks = await dangerousCmdGuard({});
const run = async (command) => {
  try {
    await hooks['tool.execute.before']({ tool: 'bash' }, { args: { command } });
    return 'allow';
  } catch (e) {
    return e.message.startsWith('[devexp dangerous-cmd-guard] Blocked:') ? 'block' : `error ${e.message}`;
  }
};
const ENTRY = [
  ['block', 'git status && git reset --hard'],
  ['allow', 'echo "    git reset --hard origin/main"'],
];
for (const [want, c] of ENTRY) {
  const got = await run(c);
  if (got !== want) { console.log(`FAIL entry want ${want}, got ${got}:`, show(c)); fail++; }
}

console.log(`${BLOCK.length + ALLOW.length + ENTRY.length - fail} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
