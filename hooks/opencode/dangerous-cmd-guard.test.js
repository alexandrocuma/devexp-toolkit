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

  // #151: an expansion right after the target, or another home spelling (bash word-splits
  // an unquoted expansion and zsh expands `~` after parameters)
  'rm -rf /$IFS',
  'rm -rf /${IFS}',
  'rm -rf ~$x',
  'rm -rf ~/$x',
  'rm -rf $HOME$IFS',
  'rm -rf $HOME/',
  'rm -rf /$(true)',
  "rm -rf /$'\\x20'",
  'rm -rf /$1',
  'rm -rf /{,}',
  'rm -rf ~{,}',
  'rm -rf /*',
  'rm -rf ~/*',
  'rm -rf /tmp*',
  'rm -rf /tmp/$x',
  'rm -rf /tmp/{,x}',
  'rm -rf /tmp/?(x)',
  'rm -rf /tmp>x',
  'rm -rf /tmp(@)', // zsh glob qualifier
  'rm -rf ~(/)',
  'rm -rf ~/.claude$IFS',
  'rm -rf ~/.claude*',
  'rm -rf ${HOME}',
  'rm -rf "${HOME}"',
  'rm -rf ${HOME:-/x}',
  'rm -rf ${HOME:0:1}',
  'rm -rf ${HOME}/.claude',
  'rm -rf "${HOME}"/.claude',
  'rm -rf $~HOME',
  'rm -rf ${=HOME}',
  'rm -rf ${(L)HOME}',
  'rm -rf $HOME:h',
  'rm -rf $HOME[1]',
  'rm -rf ~alice',
  'rm -rf ~alice/.claude',
  'x=$(rm -rf ${HOME})',
  "sh -c 'rm -rf /$IFS'",
  'git push -f$x',
  'git push --force{,}',
  'git push --force-with-lease>log',

  // #146: the same decisions, reached in linear time; nesting deeper than 100 levels is
  // scanned whole in both implementations
  ':(){ :|:& };:',
  'echo "`#`"; git reset --hard\necho done', // a comment inside backticks ends there
  `echo "${'$(echo '.repeat(100)}x${')'.repeat(100)}" "git reset --hard"`,
  `echo "${'$(echo '.repeat(101)}x${')'.repeat(101)}" "git reset --hard"`,
  `echo "${'$(echo '.repeat(150)}x${')'.repeat(150)}" "git reset --hard"`,
  // the rule applies again in every later stage or command, not only the first
  'rm -f a | rm -f /tmp/*',
  'rm -f a; rm -f ~/.claude/*',
  'git push origin | git push --force',
  'git push origin; git push -f',
  'git reset x | y --hard', // reset and clean scan past | ; &
  'git clean x; y -f',
  ':(){ :;:|:& };:', // the fork bomb scans past ;
  'git push --force-with-lease=main',

  // PR #160 review: more globs and home spellings end or name a target
  'rm -rf ~/?*', // a glob made of `?`
  'rm -rf /???',
  'rm -rf /tmp/?*',
  'rm -rf ~/.claude/?*',
  'rm -rf /tmp/@(x)',
  'rm -rf /tmp/!(x)',
  'rm -rf /tmp/+(x)',
  'rm -rf ${HOME[@]}', // subscripts inside the braces
  'rm -rf ${HOME[0]}',
  'rm -rf "${HOME[*]}"',
  'rm -rf ${HOME[1,-1]}',
  'rm -f ${HOME[@]}/.claude',
  'rm -rf $^HOME',
  'rm -rf ${^HOME}',
  'rm -rf ${HOME#x}',
  'rm -rf ${HOME/x/y}',
  'rm -rf ~a.b',
  'rm -rf $=HOME:h',

  // PR #160 review: a real rm after ; && || & |, as an argument, or with a redirect's &
  'docker run --rm img; rm -rf /tmp/*',
  'terraform init && rm -f ~/.claude/*',
  'make || rm -rf /tmp/$x',
  'sleep 1 & rm -f ~/.claude/x/*',
  'ls | xargs rm -f /tmp/*',
  'find . -exec rm -f ~/.claude/* +',
  '/bin/rm -rf /tmp/*',
  '\\rm -f ~/.claude/*',
  'sudo rm -rf $HOME/.claude',
  'rm -rf 2>&1 /tmp/*',
  'rm -rf &>/dev/null ~/.claude',
  'rm -f <&0 >&2 /tmp/$x',
  'x=$(rm -f /tmp/*)',
  'x=`rm -rf /tmp`', // a backtick ends the target
  '"rm" -f /tmp/*', // rm followed by a quote
  'rm -frvr /',
  'rm -f >&>& /tmp/*', // each & joins the > before it
  'rm -rf $=HOME', // the remaining home spellings
  'rm -rf ${~HOME}',
  'rm -rf ${HOME-x}',
  'rm -rf ${HOME=x}',
  'rm -rf ${HOME?x}',
  'rm -rf ${HOME+x}',
  'rm -rf ${HOME%x}',
  'rm -rf ${HOME^}',
  'rm -rf ${HOME,}',
  'rm -rf ${HOME@Q}',

  // PR #160 re-review: rm followed at once by an expansion, brace, glob or redirect
  'rm$IFS-f /tmp/*',
  'rm${IFS}-rf ~/.claude/*',
  'rm<<<x -f /tmp/*',
  'rm</dev/null -f /tmp/*',
  'rm>/dev/null -f /tmp/*',
  'rm&>/dev/null -f /tmp/*',
  'rm$(true) -f /tmp/*',
  'rm`true` -f /tmp/*',
  'rm$@ -f ~/.claude/*',
  "rm$'' -f /tmp/*",
  "rm'' -f /tmp/*",
  'rm{,} -f /tmp/*',
  '{sudo,rm} -f /tmp/*',
  'rm* -f /tmp/*',
  '$1rm -rf /', // a positional parameter before rm
  '$1rm -f /tmp/*',
  'rm /tmp/*', // no flags, one space
  'rm -fr $HOME:h',

  // PR #160 re-review: a ; or & that is data doesn't end the command
  'rm -rf "a;b" /tmp/*',
  "rm -rf 'a&b' ~/.claude/*",
  'rm -rf a\\;b /tmp/*',
  'rm -rf a\\&b /tmp/*',
  'rm -rf "$(a;b)" /tmp/*',
  'rm -rf $(a;b) /tmp/*',
  'rm -rf `a;b` /tmp/*',
  'rm -rf ${x:-a;b} /tmp/*',
  "rm -rf $'a;b' /tmp/*",
  'rm -rf "$(a && b)" ~/.claude/*',
  'rm -f "$f&" /tmp/*',
  'rm -f "a"; cp x /tmp/$y', // a quote before a real ; still scans on
  'rm$(a;b) /tmp/*', // a substitution right after rm
  'rm${x#;} -f /tmp/*',
  'rm>&2 -f /tmp/*',

  // line by line, like the Claude Code hook's grep: CR is an ordinary character inside
  // a line, and NUL is dropped
  'git reset x\r--hard',
  'git push origin\r--force',
  'rm -rf \0/',

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

  // #151: a literal component after the protected prefix
  'rm -rf ~/x$y',
  'rm -rf ~/projects/$x',
  'rm -rf ~/work-$x',
  'rm -rf "$HOME/projects/$x"',
  'rm -rf ${HOME}/projects/old',
  'rm -rf ${HOME}x',
  'rm -rf ${HOME}_x',
  'rm -rf $HOMEDIR',
  'rm -rf ${HOMEDIR}',
  'rm -rf ${HOMER}',
  'rm -rf $HOME_BACKUP',
  'rm -rf ~+',
  'rm -rf /tmp/.deliver-$id-*',
  'rm -rf /tmp/.a-${id}',
  'rm -rf /tmpx',
  'rm -rf /tmp_old',
  'rm -rf /var/tmp/$x',
  'rm -rf ~/.claudex',
  'rm -rf ~/.claude-backup',
  'rm -f ~/.claude/agent-memory/x/$id.md',
  'rm -rf ./*',
  'git push --follow-tags$x',
  'echo rm -rf ${HOME}',
  'git commit -m "rm -rf /$IFS"',

  // #146: the same decisions, reached in linear time — a protected target or force flag in
  // another pipeline stage than the command; nesting within the limit is still masked
  'rm -f a | cat ~/.claude | rm -f b',
  'rm -f a | ls ~/.claude/x/*',
  'rm -f ~/.claude/plans/x.md /tmp/.deliver-1/*',
  'git push origin | grep -f pats | git push origin',
  ':(){ :; } | cat',
  `echo "${'$(echo '.repeat(50)}x${')'.repeat(50)}" "git reset --hard"`,
  `echo "${'$(echo '.repeat(99)}x${')'.repeat(99)}" "git reset --hard"`,
  'ls ~/.claude/x/* | rm -f b', // .claude/* before the rm
  'ls ~/.claude/x/*; rm -f b',
  'rm -f x.claude\tb/*', // a tab ends the .claude…/* run

  // PR #160 review: a modifier needs the unbraced name; `:` continues a target
  'rm -rf ${HOME}:h',
  'rm -f /tmp/:x',

  // PR #160 review: `rm` must be its own word, and the target in its command
  'docker run --rm -v /tmp:/tmp alpine ls',
  'docker run --rm -v ~/.claude:/root/.claude img',
  'docker run --rm -v "$HOME/.claude":/home/node/.claude img',
  'docker run --rm -v /tmp/$x:/data img',
  'rm -rf dist && cp -r out /tmp/$(date +%s)',
  'rm -rf build; ls /tmp/{a,b}',
  'rm -f a.o && PATH=/tmp:$PATH make',
  'docker rm -f c1; docker run -v /tmp:/tmp img',
  'terraform init && cat ~/.claude/${f}.md',
  'rm -rf node_modules && mktemp -d /tmp/$USER.XXXX',
  'rm -f a && ls /tmp/*',
  'rm -f a & ls ~/.claude/*',
  'rm -f &>& /tmp/*', // the second & has no > of its own
  'rmdir ~/.claude/x/*', // rm must end its word
  'terraform plan -out /tmp/$plan', // nor start inside one
  './bin/v2rm /tmp/$x',
  './bin/safe_rm /tmp/$x',
  './bin/safe.rm /tmp/$x',
  './bin/Xrm /tmp/$x',
  'rm${HOME}/.claude', // not rm: the word is rm/…/.claude
  'rm -f a; cp "x" /tmp/$y', // a quote after the real ; doesn't scan on
  'rm -f a >&&2 /tmp/*', // the & after >& has no > of its own
  'rm -f $x && cp y /tmp/$z', // a bare $ is no quote
  'rm -f "a;b" | ls /tmp/*', // the scan still ends at the next |

  // line by line, like the Claude Code hook's grep: a pattern begun on one line and
  // completed on a later one is not a match (backslash continuations are joined first)
  'git push origin\ngit status --force',
  'git push\n--force',
  'rm -rf\n/',
  'rm\nfoo /tmp/*',
  'rm -rf /home/x\n/tmp/*',
  'git push origin \\\r\n  --force', // CRLF after a backslash is no continuation

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

// #146: deciding stays fast on crafted long lines. Each input used to make a pattern (or the
// masking pass) backtrack or rescan: at 100 KB the old module took seconds on the first ones
// and far longer on the last; this module takes milliseconds. The first tier stops at its
// first failure so an old module fails fast; the 1 MB tier runs only when the first passed.
const rep = (unit, len) => unit.repeat(Math.ceil(len / unit.length)).slice(0, len);
const RM = ['r', 'm'].join('');
const crafted = (len) => [
  ['rm -r…', `${RM} -${rep('r', len)}`],
  ['rm -f…', `${RM} -${rep('f', len)}`],
  ['fork opener then pipes', `:(){ ${rep('|', len)}`],
  ['repeated rm', rep(`${RM} `, len)],
  ['repeated rm, then a pipe', `${rep(`${RM} `, len)}|`],
  ['repeated git push, then ;', `${rep('git push ', len)};`],
  ['repeated git push', rep('git push ', len)],
  ['long pipeline', rep('a|', len * 2)],
  ['backtick comments', rep('`#` ', len)],
  ['repeated rm.claude', rep(`${RM}.claude`, len)],
  ['rm then .claude runs', `${RM} ${rep('.claude', len)}`],
  ['repeated rm .claude|', rep(`${RM} .claude|`, len)],
  ['git push then dashes', `git push ${rep('-', len)}`],
  ['git clean -rrr', `git clean -${rep('r', len)}`],
  ['repeated ${HOME:', `${RM} -rf ${rep('${HOME:', len)}`],
  ['repeated ${(', `${RM} ${rep('${(', len)}`],
  ['long ~name', `${RM} -rf ${rep('~a', 2)}${rep('a', len)}`],
  ['repeated $~', `${RM} ${rep('$~', len)}`],
  ['rm then redirects', `${RM} ${rep('>&', len)}`],
  ['repeated rm x&', rep(`${RM} x&`, len)],
  ['repeated ;rm', rep(`;${RM} `, len)],
  ['rm then ?', `${RM} -rf ${rep('?', len)}`],
  ['open ${HOME[', `${RM} -rf \${HOME[${rep('a', len)}`],
  ['rm " then ; run', `${RM} "${rep(';', len)}`],
  ['repeated rm ";', rep(`${RM} ";`, len)],
  ['repeated rm "|', rep(`${RM} "|`, len)],
  ['repeated rm$(&', rep(`${RM}$(&`, len)],
];
let perfFail = 0;
const timed = (tier, len, budgetMs, stopEarly) => {
  let n = 0;
  for (const [name, cmd] of crafted(len)) {
    const t0 = performance.now();
    blockReason(cmd);
    const ms = performance.now() - t0;
    n++;
    if (ms > budgetMs) {
      console.log(`FAIL ${tier}: ${name} (${cmd.length} chars) took ${ms.toFixed(0)} ms, budget ${budgetMs} ms`);
      perfFail++;
      fail++;
      if (stopEarly) break;
    }
  }
  return n;
};
let perfCases = timed('100 KB line', 100_000, 500, true);
if (perfFail === 0) perfCases += timed('1 MB line', 1_000_000, 2000, false);

console.log(`${BLOCK.length + ALLOW.length + ENTRY.length + perfCases - fail} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
