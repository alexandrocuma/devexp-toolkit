#!/usr/bin/env bash
# Tests for dangerous-cmd-guard.sh — exit 2 = blocked, exit 0 = allowed.
# Run: bash hooks/claude-code/dangerous-cmd-guard.test.sh
set -uo pipefail

HOOK="$(cd "$(dirname "$0")" && pwd)/dangerous-cmd-guard.sh"
pass=0; fail=0

# Feed a command to the hook as the real PreToolUse JSON envelope; return its exit code.
run() {
  python3 -c 'import json,sys; print(json.dumps({"tool_input":{"command":sys.argv[1]}}))' "$1" \
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

# ── #100 must BLOCK: a real invocation, in every command position ───────────
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

# ── #100 must BLOCK: a string handed to a shell or evaluator ────────────────
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

# ── #100 must BLOCK: masked text read back and run in the same command ──────
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

# ── #100 must BLOCK: what the parser cannot classify is scanned whole ───────
expect block 'echo "git reset --hard'                        # unbalanced quote
expect block 'echo "$(git reset --hard"'                     # unbalanced substitution
expect block $'cat <<\'EOF\'\ngit reset --hard'              # unterminated heredoc
expect block 'echo "$(case x in a) git reset --hard;; esac)"'
expect block 'echo `echo \`git reset --hard\``'
expect block 'echo $((1)) "git reset --hard"'

# ── #100 must ALLOW: a mention is not an invocation ─────────────────────────
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

# ── #100 large input: grep must not lose a match to SIGPIPE ─────────────────
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

# ── A grep failure blocks; it is never read as "no match" ───────────────────
STUB=$(mktemp -d); trap 'rm -rf "$STUB"' EXIT
printf '#!/bin/sh\nexit 2\n' > "$STUB/grep"; chmod +x "$STUB/grep"
rc=$(python3 -c 'import json; print(json.dumps({"tool_input":{"command":"ls -la"}}))' \
  | PATH="$STUB:$PATH" bash "$HOOK" >/dev/null 2>&1; echo $?)
if [ "$rc" = 2 ]; then pass=$((pass+1)); else fail=$((fail+1)); printf 'FAIL [want block, rc %s]: grep failing\n' "$rc"; fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
