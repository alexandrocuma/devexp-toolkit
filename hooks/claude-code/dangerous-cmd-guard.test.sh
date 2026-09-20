#!/usr/bin/env bash
# Tests for dangerous-cmd-guard.sh — exit 2 = blocked, exit 0 = allowed.
# Run: bash hooks/claude-code/dangerous-cmd-guard.test.sh
set -uo pipefail

HOOK="$(cd "$(dirname "$0")" && pwd)/dangerous-cmd-guard.sh"
pass=0; fail=0

# Feed a command to the hook as the real PreToolUse JSON envelope; return its exit code.
# An argument cannot hold a NUL byte, so "<NUL>" stands for one.
run() {
  python3 -c 'import json,sys; print(json.dumps({"tool_input":{"command":sys.argv[1].replace("<NUL>", "\0")}}))' "$1" \
    | bash "$HOOK" >/dev/null 2>&1
  echo $?
}

expect() { # $1=block|allow  $2=command
  local want="$1" cmd="$2" rc; rc=$(run "$cmd")
  if { [ "$want" = block ] && [ "$rc" = 2 ]; } || { [ "$want" = allow ] && [ "$rc" = 0 ]; }; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL [want %s, rc %s]: %s\n' "$want" "$rc" "$cmd"
  fi
}

# ── must BLOCK ──────────────────────────────────────────────────────────────
expect block 'rm -f /tmp/*'
expect block 'rm -rf /tmp'
expect block 'rm -rf /tmp/'
expect block 'rm -f /tmp/*"$ticket"*'                       # empty-var template collapses to /tmp/*
expect block 'rm -rf ~/.claude'
expect block 'rm -rf $HOME/.claude'
expect block 'rm -f ~/.claude/agent-memory/grooming-agent/sessions/*"$id"*'
expect block 'rm -f ~/.claude/agent-memory/*'
expect block 'rm -rf /'                                      # pre-existing rule still holds
expect block 'git push --force'
expect block 'git push origin main -f'
expect block 'git push --force-with-lease'
expect block "rm -rf '/tmp'/*"                              # quoted dir must not evade
expect block 'rm -rf "/tmp"/*'
expect block 'rm -rf "$HOME"/.claude'
expect block 'git commit -m x && git push --force'          # force push after separator still blocked

# ── must ALLOW ──────────────────────────────────────────────────────────────
expect allow 'rm -f /tmp/.deliver-PAY-123-*'                 # prefix-anchored toolkit scratch
expect allow 'rm -f /tmp/.recently_changed.txt'             # specific file (improve uses this)
expect allow 'rm -f ~/.claude/agent-memory/grooming-agent/plans/PAY-123.md'
expect allow 'rm -f ~/.claude/agent-memory/grooming-agent/sessions/PAY-123-*'
expect allow 'git push origin main'
expect allow 'git commit -m "fix rm -f false positive" ; git push'   # the bug this PR fixes
expect allow 'git push && rm -f /tmp/.deliver-X-1'
expect allow 'rm -rf node_modules'                          # common dev cleanup, not sensitive
expect allow 'rm -rf ./dist'

# ── must BLOCK: a real invocation, in every command position ────────────────
expect block 'git status && git reset --hard'
expect block 'git status; git reset --hard HEAD~1'
expect block 'false || git reset --hard'
expect block 'yes | git clean -fd'
expect block $'git status\ngit reset --hard'
expect block '(git reset --hard)'
expect block '{ git reset --hard; }'
expect block 'if true; then git reset --hard; fi'
expect block 'echo "$(git reset --hard)"'
expect block 'x=$(git reset --hard)'
expect block 'echo `git reset --hard`'
expect block '$(echo git reset --hard)'
expect block 'git commit -m "$(git reset --hard)"'
expect block 'sudo rm -rf /'
expect block 'sudo -u root git reset --hard'
expect block 'env GIT_DIR=.git git reset --hard'
expect block 'FOO=1 git push --force'
expect block 'nohup git push --force &'
expect block 'time git reset --hard'
expect block 'command git clean -fd'
expect block 'exec git reset --hard'
expect block 'find . -name x | xargs rm -rf /'
expect block 'timeout 5 git push origin main --force'
expect block "ssh host 'git reset --hard'"
expect block 'psql -c "DROP TABLE users"'
expect block '--help || git reset --hard'                    # command text starting with a dash
expect block 'echo a#b; git reset --hard'                    # mid-word # is not a comment
expect block 'echo ${#x}; git reset --hard'
expect block $'echo hi # note\ngit push --force'              # a comment ends at the newline

# ── must BLOCK: a string handed to a shell or evaluator ─────────────────────
expect block 'bash -c "git reset --hard"'
expect block "sh -c 'git clean -fdx'"
expect block 'eval "git reset --hard"'
expect block $'bash <<\'EOF\'\ngit reset --hard\nEOF'
expect block $'cat <<\'EOF\' | sh\ngit reset --hard\nEOF'
expect block $'cat <<EOF\n$(git reset --hard)\nEOF'         # unquoted heredoc runs $(...)
expect block $'cat <<\'EOF\' > run.sh\ngit reset --hard\nEOF'
expect block 'echo "git reset --hard" | sh'
expect block $'echo "git reset --hard" |\n  bash'
expect block "printf 'git reset --hard\\n' | bash"
expect block 'echo "git reset --hard" |& sh'
expect block 'echo "git clean -fd" | xargs -I{} sh -c {}'
expect block "echo 'DROP DATABASE prod' | psql"
expect block 'echo "git reset --hard" > run.sh'
expect block 'echo "git reset --hard" >> run.sh'
expect block 'echo "git reset --hard" | tee run.sh'
expect block 'echo "git reset --hard" 2>err.log >&2'
expect block 'echo "git reset --hard" > >(sh)'
expect block 'bash <(echo "git reset --hard")'
expect block "printf -v c 'git reset --hard'; \$c"
expect block 'git commit -m "git reset --hard" | sh'
expect block '{ echo "git reset --hard"; } | sh'
expect block '(echo "git reset --hard") | sh'
expect block 'for i in 1; do echo "git reset --hard"; done | sh'
expect block '{ echo "git reset --hard"; } > run.sh'
expect block '(echo "git reset --hard") | tee run.sh'
expect block 'if true; then echo "git reset --hard"; fi >> run.sh'
expect block 'alias echo=sh; echo "git reset --hard"'
expect block 'f() { echo "git reset --hard"; }; f | sh'
expect block 'exec > run.sh; echo "git reset --hard"'
expect block 'echo "git reset --hard" | pbcopy'              # the clipboard is not a sink
expect block 'echo "git reset --hard" | uniq - run.sh'       # uniq writes its output file

# ── must BLOCK: masked text read back and run in the same command ───────────
expect block 'git commit -m "git reset --hard"; git log -1 --format=%B | sh'
expect block 'gh issue create --body "git push --force now"; gh issue view 1 --json body -q .body | bash'
expect block 'git commit -m "git reset --hard" && ./run.sh'
expect block 'echo "git reset --hard"; "$SHELL" run.sh'
expect block 'echo "git reset --hard"; . run.sh'
expect block 'echo "git reset --hard"; timeout 5 python3 run.py'

# ── must BLOCK: the target right before a closing quote, paren or operator ──
# (v0.9.0 required whitespace or end of line after the target, so these ran.)
expect block "sh -c 'rm -rf /'"
expect block 'bash -c "rm -rf ~"'
expect block 'bash -c "rm -fr $HOME"'
expect block "eval 'git push --force'"
expect block 'eval "git push origin main -f"'
expect block "ssh host 'rm -rf ~'"
expect block "su -c 'git push --force-with-lease' deploy"      # extra args after the string
expect block "echo x | xargs sh -c 'rm -rf /'"
expect block "sh -c 'rm -rf /' _ extra"
expect block 'bash -c "git push origin main --force" -- arg'
expect block 'echo "$(rm -rf ~)"'
expect block 'echo `rm -rf /`'
expect block '(rm -rf /tmp)'
expect block 'x=$(rm -rf ~/.claude)'
expect block 'git push --force;'
expect block 'git push -f&& echo done'
expect block 'rm -rf /|| true'
expect block 'rm -rf "/"'
expect block 'rm -rf "$HOME"'

# ── must BLOCK: a command split across backslash-continued lines ────────────
expect block $'rm -rf \\\n  /'
expect block $'git push origin \\\n  --force-with-lease'
expect block $'sh -c \'git push origin main \\\n  --force\''
expect block $'bash -c "rm -rf \\\n  ~"'

# ── must ALLOW: the wider boundary does not swallow benign targets ──────────
expect allow "sh -c 'rm -rf ./build'"
expect allow 'bash -c "git push origin main"'
expect allow "eval 'rm -f /tmp/.deliver-X-1'"
expect allow '(rm -rf node_modules)'
expect allow 'rm -rf ~/projects/old-build;'
expect allow 'bash -c "rm -rf /var/tmp/build"'
expect allow 'git push --follow-tags;'
expect allow "ssh host 'git push origin main'"
expect allow $'git push origin \\\n  main'

# ── must BLOCK: an expansion right after the target, or another home spelling ───────
# bash word-splits an unquoted expansion and zsh expands `~` after parameters, so
# a target followed by an expansion can still name the directory itself.
expect block 'rm -rf /$IFS'
expect block 'rm -rf /${IFS}'
expect block 'rm -rf ~$x'
expect block 'rm -rf ~/$x'
expect block 'rm -rf $HOME$IFS'
expect block 'rm -rf $HOME/'
expect block 'rm -rf /$(true)'
expect block "rm -rf /\$'\\x20'"
expect block 'rm -rf /$1'
expect block 'rm -rf /{,}'
expect block 'rm -rf ~{,}'
expect block 'rm -rf /*'
expect block 'rm -rf ~/*'
expect block 'rm -rf /tmp*'
expect block 'rm -rf /tmp/$x'
expect block 'rm -rf /tmp/{,x}'
expect block 'rm -rf /tmp/?(x)'
expect block 'rm -rf /tmp>x'
expect block 'rm -rf /tmp(@)'                                # zsh glob qualifier
expect block 'rm -rf ~(/)'
expect block 'rm -rf ~/.claude$IFS'
expect block 'rm -rf ~/.claude*'
expect block 'rm -rf ${HOME}'
expect block 'rm -rf "${HOME}"'
expect block 'rm -rf ${HOME:-/x}'
expect block 'rm -rf ${HOME:0:1}'
expect block 'rm -rf ${HOME}/.claude'
expect block 'rm -rf "${HOME}"/.claude'
expect block 'rm -rf $~HOME'
expect block 'rm -rf ${=HOME}'
expect block 'rm -rf ${(L)HOME}'
expect block 'rm -rf $HOME:h'
expect block 'rm -rf $HOME[1]'
expect block 'rm -rf ~alice'
expect block 'rm -rf ~alice/.claude'
expect block 'x=$(rm -rf ${HOME})'
expect block "sh -c 'rm -rf /\$IFS'"
expect block 'git push -f$x'
expect block 'git push --force{,}'
expect block 'git push --force-with-lease>log'

# ── must ALLOW: a literal component after the protected prefix ──────────────
expect allow 'rm -rf ~/x$y'
expect allow 'rm -rf ~/projects/$x'
expect allow 'rm -rf ~/work-$x'
expect allow 'rm -rf "$HOME/projects/$x"'
expect allow 'rm -rf ${HOME}/projects/old'
expect allow 'rm -rf ${HOME}x'
expect allow 'rm -rf ${HOME}_x'
expect allow 'rm -rf $HOMEDIR'
expect allow 'rm -rf ${HOMEDIR}'
expect allow 'rm -rf ${HOMER}'
expect allow 'rm -rf $HOME_BACKUP'
expect allow 'rm -rf ~+'
expect allow 'rm -rf /tmp/.deliver-$id-*'
expect allow 'rm -rf /tmp/.a-${id}'
expect allow 'rm -rf /tmpx'
expect allow 'rm -rf /tmp_old'
expect allow 'rm -rf /var/tmp/$x'
expect allow 'rm -rf ~/.claudex'
expect allow 'rm -rf ~/.claude-backup'
expect allow 'rm -f ~/.claude/agent-memory/x/$id.md'
expect allow 'rm -rf ./*'
expect allow 'git push --follow-tags$x'
expect allow 'echo rm -rf ${HOME}'
expect allow 'git commit -m "rm -rf /$IFS"'

# ── more globs and home spellings end or name a target ──────────────────────
expect block 'rm -rf ~/?*'                                    # a glob made of `?`
expect block 'rm -rf /???'
expect block 'rm -rf /tmp/?*'
expect block 'rm -rf ~/.claude/?*'
expect block 'rm -rf /tmp/@(x)'
expect block 'rm -rf /tmp/!(x)'
expect block 'rm -rf /tmp/+(x)'
expect block 'rm -rf ${HOME[@]}'                              # subscripts inside the braces
expect block 'rm -rf ${HOME[0]}'
expect block 'rm -rf "${HOME[*]}"'
expect block 'rm -rf ${HOME[1,-1]}'
expect block 'rm -f ${HOME[@]}/.claude'
expect block 'rm -rf $^HOME'
expect block 'rm -rf ${^HOME}'
expect block 'rm -rf ${HOME#x}'
expect block 'rm -rf ${HOME/x/y}'
expect block 'rm -rf ~a.b'
expect block 'rm -rf $=HOME:h'
expect allow 'rm -rf ${HOME}:h'                               # a modifier needs the unbraced name
expect allow 'rm -f /tmp/:x'                                  # `:` continues a target

# ── `rm` must be its own word, and the target in its command ──────────────────
expect allow 'docker run --rm -v /tmp:/tmp alpine ls'
expect allow 'docker run --rm -v ~/.claude:/root/.claude img'
expect allow 'docker run --rm -v "$HOME/.claude":/home/node/.claude img'
expect allow 'docker run --rm -v /tmp/$x:/data img'
expect allow 'rm -rf dist && cp -r out /tmp/$(date +%s)'
expect allow 'rm -rf build; ls /tmp/{a,b}'
expect allow 'rm -f a.o && PATH=/tmp:$PATH make'
expect allow 'docker rm -f c1; docker run -v /tmp:/tmp img'
expect allow 'terraform init && cat ~/.claude/${f}.md'
expect allow 'rm -rf node_modules && mktemp -d /tmp/$USER.XXXX'
expect allow 'rm -f a && ls /tmp/*'
expect allow 'rm -f a & ls ~/.claude/*'
expect block 'docker run --rm img; rm -rf /tmp/*'              # a real rm after ; && || & |
expect block 'terraform init && rm -f ~/.claude/*'
expect block 'make || rm -rf /tmp/$x'
expect block 'sleep 1 & rm -f ~/.claude/x/*'
expect block 'ls | xargs rm -f /tmp/*'
expect block 'find . -exec rm -f ~/.claude/* +'
expect block '/bin/rm -rf /tmp/*'
expect block '\rm -f ~/.claude/*'
expect block 'sudo rm -rf $HOME/.claude'
expect block 'rm -rf 2>&1 /tmp/*'                             # a redirect's & doesn't end it
expect block 'rm -rf &>/dev/null ~/.claude'
expect block 'rm -f <&0 >&2 /tmp/$x'
expect block 'x=$(rm -f /tmp/*)'
expect block 'x=`rm -rf /tmp`'                                # a backtick ends the target
expect block '"rm" -f /tmp/*'                                 # rm followed by a quote
expect block 'rm -frvr /'
expect block 'rm -f >&>& /tmp/*'                              # each & joins the > before it
expect allow 'rm -f &>& /tmp/*'                               # the second & has no > of its own
expect allow 'rmdir ~/.claude/x/*'                            # rm must end its word
expect allow 'terraform plan -out /tmp/$plan'                 # nor start inside one
expect allow './bin/v2rm /tmp/$x'
expect allow './bin/safe_rm /tmp/$x'
expect allow './bin/safe.rm /tmp/$x'
expect block 'rm -rf $=HOME'                                  # the remaining home spellings
expect block 'rm -rf ${~HOME}'
expect block 'rm -rf ${HOME-x}'
expect block 'rm -rf ${HOME=x}'
expect block 'rm -rf ${HOME?x}'
expect block 'rm -rf ${HOME+x}'
expect block 'rm -rf ${HOME%x}'
expect block 'rm -rf ${HOME^}'
expect block 'rm -rf ${HOME,}'
expect block 'rm -rf ${HOME@Q}'

# ── rm followed at once by an expansion, brace, glob or redirect ─────────────────────
expect block 'rm$IFS-f /tmp/*'
expect block 'rm${IFS}-rf ~/.claude/*'
expect block 'rm<<<x -f /tmp/*'
expect block 'rm</dev/null -f /tmp/*'
expect block 'rm>/dev/null -f /tmp/*'
expect block 'rm&>/dev/null -f /tmp/*'
expect block 'rm$(true) -f /tmp/*'
expect block 'rm`true` -f /tmp/*'
expect block 'rm$@ -f ~/.claude/*'
expect block "rm\$'' -f /tmp/*"
expect block "rm'' -f /tmp/*"
expect block 'rm{,} -f /tmp/*'
expect block '{sudo,rm} -f /tmp/*'
expect block 'rm* -f /tmp/*'
expect block '$1rm -rf /'                                     # a positional parameter before rm
expect block '$1rm -f /tmp/*'
expect block 'rm /tmp/*'                                      # no flags, one space
expect block 'rm -fr $HOME:h'
expect allow 'rm${HOME}/.claude'                              # not rm: the word is rm/…/.claude
expect allow './bin/Xrm /tmp/$x'

# ── a ; or & that is data doesn't end the command ────────────────────────────
expect block 'rm -rf "a;b" /tmp/*'
expect block "rm -rf 'a&b' ~/.claude/*"
expect block 'rm -rf a\;b /tmp/*'
expect block 'rm -rf a\&b /tmp/*'
expect block 'rm -rf "$(a;b)" /tmp/*'
expect block 'rm -rf $(a;b) /tmp/*'
expect block 'rm -rf `a;b` /tmp/*'
expect block 'rm -rf ${x:-a;b} /tmp/*'
expect block "rm -rf \$'a;b' /tmp/*"
expect block 'rm -rf "$(a && b)" ~/.claude/*'
expect block 'rm -f "$f&" /tmp/*'
expect block 'rm -f "a"; cp x /tmp/$y'                        # a quote before a real ; still scans on
expect allow 'rm -f a; cp "x" /tmp/$y'                        # a quote after it doesn't
expect allow 'rm -f a >&&2 /tmp/*'                            # the & after >& has no > of its own
expect block 'rm$(a;b) /tmp/*'                                # a substitution right after rm
expect block 'rm${x#;} -f /tmp/*'
expect block 'rm>&2 -f /tmp/*'
expect block 'rm ~/.claude'                                   # no flags, home target
expect block '{rm,echo} -f /tmp/*'                            # rm glued to , ? [
expect block 'rm? -f /tmp/*'
expect block 'rm[m] -f /tmp/*'
expect block 'rm -rf <(a;b) /tmp/*'                           # a process substitution holds ; or &
expect block 'rm -rf >(a&b) ~/.claude/*'
expect block 'rm -rf <(a && b) /tmp/*'
expect block 'rm -rf =(a;b) ~/.claude/*'
expect block 'rm -rf x<(a;b) /tmp/*'
expect block 'rm -rf x>(a&b) /tmp/*'
expect block 'rm -rf x=(a;b) ~/.claude/*'
expect block 'rm<(a;b) /tmp/*'
expect block 'rm>(a&b) ~/.claude/*'
expect block 'rm -f $[1&2] /tmp/*'                            # old-style arithmetic holds & too
expect block 'rm$[1&2] -f /tmp/*'
expect allow 'rm=(a;b) /tmp/*'                                # an array assignment, not rm
expect allow 'diff <(a) <(b) && ls /tmp/$x'
expect allow 'rm -f x && diff <(a) /tmp/$y'
expect allow 'x=(a;b) ls /tmp/*'
expect allow 'rm -f $x && cp y /tmp/$z'                       # a bare $ is no quote
expect allow 'rm -f "a;b" | ls /tmp/*'                        # the scan still ends at the next |

# ── the same decisions, reached in linear time ──────────────────────────────
# A protected target or force flag in another pipeline stage than the command.
expect allow 'rm -f a | cat ~/.claude | rm -f b'
expect allow 'rm -f a | ls ~/.claude/x/*'
expect allow 'rm -f ~/.claude/plans/x.md /tmp/.deliver-1/*'
expect allow 'git push origin | grep -f pats | git push origin'
expect block ':(){ :|:& };:'
expect block $'echo "`#`"; git reset --hard\necho done'     # a comment inside backticks ends there
expect allow ':(){ :; } | cat'
# The rule applies again in every later stage or command, not only the first.
expect block 'rm -f a | rm -f /tmp/*'
expect block 'rm -f a; rm -f ~/.claude/*'
expect block 'git push origin | git push --force'
expect block 'git push origin; git push -f'
expect allow 'ls ~/.claude/x/* | rm -f b'                     # .claude/* before the rm
expect allow 'ls ~/.claude/x/*; rm -f b'
expect allow $'rm -f x.claude\tb/*'                          # a tab ends the .claude…/* run
expect block 'git reset x | y --hard'                         # reset and clean scan past | ; &
expect block 'git clean x; y -f'
expect block ':(){ :;:|:& };:'                                # the fork bomb scans past ;
expect block 'git push --force-with-lease=main'
# Nesting deeper than 100 levels is scanned whole, at the same depth in both
# implementations: 99 nested substitutions are masked, 100 and 101 are not.
nest() { # $1=depth -> echo "$(echo …x…)" "git reset --hard"
  local open close; open=$(printf '$(echo %.0s' $(seq "$1")); close=$(printf ')%.0s' $(seq "$1"))
  printf 'echo "%sx%s" "git reset --hard"' "$open" "$close"
}
expect allow "$(nest 50)"
expect allow "$(nest 99)"
expect block "$(nest 100)"
expect block "$(nest 101)"
expect block "$(nest 150)"

# ── Both implementations decide line by line (the Claude Code hook's grep) ──
# A pattern begun on one line and completed on a later one is not a match; a
# backslash continuation (above) is joined first. CR is an ordinary character
# inside a line, and NUL is dropped. The opencode twin mirrors these cases.
expect allow $'git push origin\ngit status --force'
expect allow $'git push\n--force'
expect allow $'rm -rf\n/'
expect allow $'rm\nfoo /tmp/*'
expect allow $'rm -rf /home/x\n/tmp/*'
expect allow $'git push origin \\\r\n  --force'              # CRLF after a backslash is no continuation
expect block $'git reset x\r--hard'
expect block $'git push origin\r--force'
expect block 'rm -rf <NUL>/'

# ── must BLOCK: what the parser cannot classify is scanned whole ────────────
expect block 'echo "git reset --hard'                        # unbalanced quote
expect block 'echo "$(git reset --hard"'                     # unbalanced substitution
expect block $'cat <<\'EOF\'\ngit reset --hard'              # unterminated heredoc
expect block 'echo "$(case x in a) git reset --hard;; esac)"'
expect block 'echo `echo \`git reset --hard\``'
expect block 'echo $((1)) "git reset --hard"'

# ── must ALLOW: a mention is not an invocation ──────────────────────────────
expect allow $'echo "  LOCAL ONLY — remote untouched. Tag still revertible:"\necho "    git reset --hard origin/main"     # <- text inside a quoted echo'
expect allow "echo 'rm -rf / wipes everything'"
expect allow 'echo git push --force'
expect allow "printf '%s\\n' \"git clean -fd\""
expect allow 'echo "DROP TABLE users" >&2'
expect allow 'echo "git reset --hard" > /dev/null 2>&1'
expect allow 'echo "rm -rf ~ later" | grep rm'
expect allow 'echo "$(echo git reset --hard)"'
expect allow '(echo "git reset --hard")'
expect allow 'if true; then echo "git reset --hard"; fi'
expect allow 'sudo echo "git reset --hard"'
expect allow 'env FOO=1 echo "git push --force now"'
expect allow "command printf '%s' \"git reset --hard\""
expect allow 'echo "git push --force origin" && git push'
expect allow 'git commit -m "docs: never run git reset --hard"'
expect allow 'git commit -am "note: git clean -fd removes untracked files"'
expect allow 'git tag -a v1 --message="undo with git reset --hard"'
expect allow $'git commit -m "$(cat <<\'EOF\'\nfix: guard no longer blocks git push --force mentions\nEOF\n)"'
expect allow 'gh issue create --title "guard blocks git reset --hard" --body "e.g. rm -rf / first"'
expect allow $'gh pr create --title x --body-file - <<\'EOF\'\nRun git clean -fd first.\nEOF'
expect allow $'cat <<\'EOF\'\nrun git reset --hard to discard\nEOF'
expect allow $'cat <<-EOF\n\tDROP DATABASE is irreversible\n\tEOF'
expect allow $'cat <<\'EOF\' | grep reset\ngit reset --hard\nEOF'
expect allow 'git status  # never git reset --hard here'
expect allow $'# git push --force\ngit push'

# ── large input: grep must not lose a match to SIGPIPE ──────────────────────
# Built inside Python: an argv string this long exceeds Linux's per-argument limit.
# The filler is many short lines: grep -q stops reading after an early match,
# which is what used to SIGPIPE the writer.
big() { # $1=prefix  $2=suffix  -> rc of the hook on prefix + 250 KB of lines + suffix
  python3 -c 'import json,sys; print(json.dumps({"tool_input":{"command":sys.argv[1] + "a\n" * 125000 + sys.argv[2]}}))' "$1" "$2" \
    | bash "$HOOK" >/dev/null 2>&1
  echo $?
}
expect_big() { # $1=block|allow  $2=prefix  $3=suffix
  local want="$1" rc; rc=$(big "$2" "$3")
  if { [ "$want" = block ] && [ "$rc" = 2 ]; } || { [ "$want" = allow ] && [ "$rc" = 0 ]; }; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL [want %s, rc %s]: big command %s…%s\n' "$want" "$rc" "$2" "$3"
  fi
}
expect_big block 'git reset --hard; echo ' ''
expect_big block 'echo "' '" && git push --force'
expect_big block 'echo "git reset --hard ' ''                # unbalanced: scanned whole, still blocks
expect_big allow 'echo "' '"'

# ── a long pipeline is checked in linear time ───────────────────────────────
# 400 KB of `a|a|…` takes about a second; checking every stage against the rest
# of the pipeline took over a minute. The run is cut off at the budget.
rc=$(python3 - "$HOOK" <<'PY'
import json, subprocess, sys
cmd = 'a|' * 200000 + 'a'
try:
    r = subprocess.run(['bash', sys.argv[1]], input=json.dumps({'tool_input': {'command': cmd}}).encode(),
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15)
    print(r.returncode)
except subprocess.TimeoutExpired:
    print('timeout')
PY
)
if [ "$rc" = 0 ]; then pass=$((pass+1)); else fail=$((fail+1)); printf 'FAIL [want allow within 15 s, got %s]: 400 KB pipeline\n' "$rc"; fi

# ── A grep failure blocks; it is never read as "no match" ───────────────────
STUB=$(mktemp -d); trap 'rm -rf "$STUB"' EXIT
printf '#!/bin/sh\nexit 2\n' > "$STUB/grep"; chmod +x "$STUB/grep"
rc=$(python3 -c 'import json; print(json.dumps({"tool_input":{"command":"ls -la"}}))' \
  | PATH="$STUB:$PATH" bash "$HOOK" >/dev/null 2>&1; echo $?)
if [ "$rc" = 2 ]; then pass=$((pass+1)); else fail=$((fail+1)); printf 'FAIL [want block, rc %s]: grep failing\n' "$rc"; fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
