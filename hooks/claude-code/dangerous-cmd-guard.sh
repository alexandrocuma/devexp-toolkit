#!/usr/bin/env bash
# devexp hook: dangerous-cmd-guard
# Event: PreToolUse | Matcher: Bash
# Hard-blocks destructive shell commands. No prompts — all guarded patterns are blocked.
#
# A mention is not an invocation. Before the patterns run, text that
# provably never executes is blanked out: arguments of `echo`/`printf`,
# commit/tag messages, `gh` issue/PR/release bodies, heredoc bodies fed to a
# plain text sink, and comments — and only when that text cannot reach a shell
# (no pipe into anything but a text filter, no redirect into a file). Every
# other byte is scanned exactly as before, so `bash -c "…"`, `eval`, `ssh host
# "…"`, `psql -c "…"`, `$(…)` and heredocs fed to a shell still block.
#
# The parser fails closed: anything it cannot classify (unbalanced quotes,
# case statements, functions, aliases, `$((…))`, pipes or redirects on compound
# commands, …) leaves the whole command unmasked, which is how the guard
# behaved before any masking existed. Mirrored in hooks/opencode/dangerous-cmd-guard.js.
#
# Tests: bash hooks/claude-code/dangerous-cmd-guard.test.sh

set -euo pipefail

# The whole scan runs under a wall-clock budget and blocks when it is exceeded:
# a slow scan must never let a tool call through unchecked.
#
# Everything up to devexp_scan_budget runs with no floor under it, so this
# prologue installs one. It cannot be an `if ! . …`: under `set -e` bash leaves
# the script where the `.` failed and never reaches the body, and it cannot
# read $? either — for a missing, unreadable or unparsable file, and for an
# unset variable, bash 3.2 reports 0 to an EXIT trap, and 0 reads as "allow".
# So the trap asks no questions: reaching it at all means the budget never
# started. devexp_scan_budget clears it as soon as it takes over, and from
# there the watchdog's own 0-or-2 check is the floor.
devexp_budget_why="stopped before its scan budget could start"
devexp_budget_floor() {
    trap - EXIT
    echo "[devexp dangerous-cmd-guard] internal error -- the guard $devexp_budget_why, so it did not run. Blocking to be safe." >&2
    exit 2
}
trap devexp_budget_floor EXIT

devexp_dir="${BASH_SOURCE[0]%/*}"
if [ "$devexp_dir" = "${BASH_SOURCE[0]}" ]; then devexp_dir="."; fi
devexp_budget_why="could not read its scan budget helper"
[ -r "$devexp_dir/scan-budget.sh" ] || exit 1
devexp_budget_why="could not load its scan budget helper"
. "$devexp_dir/scan-budget.sh"
devexp_budget_why="loaded a scan budget helper that defines no entry point"
command -v devexp_scan_budget >/dev/null 2>&1 || exit 1
devexp_budget_why="could not run its scan budget"
devexp_scan_budget dangerous-cmd-guard "$@"

input=$(cat)

# The program is fixed text; the tool input reaches it only on stdin.
IFS= read -r -d '' GUARD_PY <<'PY' || true
import json, re, sys


class Raw(Exception):
    """The command cannot be classified: scan all of it."""


RESERVED = frozenset(['{', '!', 'if', 'then', 'else', 'elif', 'while', 'until', 'do', 'time'])
CLOSERS = frozenset(['}', 'fi', 'done'])
WRAPPERS = frozenset(['sudo', 'env', 'command', 'builtin', 'exec', 'time'])
DEFINERS = frozenset(['alias', 'unalias', 'function', 'hash', 'enable', 'coproc'])
SINKS = frozenset(['cat', 'head', 'tail', 'wc', 'grep', 'egrep', 'fgrep', 'tr', 'cut', 'nl'])
# Anything that can run text it reads (stdin, a file, the clipboard, git or
# GitHub). If one appears anywhere, masked text could be read back and run in
# the same command, so nothing is masked.
EXECUTORS = frozenset(['sh', 'bash', 'zsh', 'dash', 'ksh', 'mksh', 'fish', 'csh', 'tcsh', 'ash', 'busybox',
                       'eval', 'source', 'xargs', 'ssh', 'su', 'script', 'python', 'python2', 'python3',
                       'perl', 'ruby', 'node', 'deno', 'bun', 'php', 'awk', 'gawk', 'mawk', 'nawk', 'sed',
                       'osascript'])
SAFE_TARGETS = frozenset(['/dev/null', '/dev/stdout', '/dev/stderr', '/dev/tty'])
GIT_MSG_OBJ = frozenset(['commit', 'tag'])
GH_OBJ = frozenset(['issue', 'pr', 'release'])
GH_VERBS = frozenset(['create', 'comment', 'edit', 'review', 'close', 'merge'])
GIT_FLAGS = (frozenset(['-m', '-am', '--message']), ('-m',))
GH_FLAGS = (frozenset(['-b', '--body', '-t', '--title', '-n', '--notes', '-c', '--comment']), ())
GIT_FILE_FLAGS = frozenset(['-F', '--file'])
GH_FILE_FLAGS = frozenset(['-F', '--body-file', '--notes-file'])
ASSIGN = re.compile(r'[A-Za-z_][A-Za-z0-9_]*(\[[^\]]*\])?\+?=')
LITERAL = re.compile(r'[A-Za-z0-9_./:@%+,=-]+')
REDIR = re.compile(r'([0-9]*)(&>>|&>|>>|>\||>&|>|<<<|<<-|<<|<&|<>|<)')
CASE = re.compile(r'\besac\b', re.A)
# Deeper nesting of (…), $(…), backticks and <(…) is scanned whole. The same
# limit in both implementations keeps Python's recursion limit from deciding.
MAX_DEPTH = 100


class Word:
    def __init__(self, start, end, text, subs):
        self.start, self.end, self.text, self.subs = start, end, text, subs


class Simple:
    def __init__(self):
        self.words, self.redirs, self.heredocs = [], [], []


class Redir:
    def __init__(self, fd, op, target):
        self.fd, self.op, self.target = fd, op, target


class Heredoc:
    def __init__(self, delim, dash, quoted):
        self.delim, self.dash, self.quoted, self.body = delim, dash, quoted, None


class Subshell:
    def __init__(self, script):
        self.script = script


class Script:
    def __init__(self):
        self.pipelines, self.comments = [], []


class Parser:
    def __init__(self, s):
        self.s, self.n, self.pending_total, self.depth = s, len(s), 0, 0

    def at(self, i, tok):
        return self.s.startswith(tok, i, self.n)

    def find(self, ch, i):
        return self.s.find(ch, i, self.n)

    def blanks(self, i):
        while i < self.n:
            if self.s[i] in ' \t':
                i += 1
            elif self.at(i, '\\\n'):
                i += 2
            else:
                break
        return i

    def after_compound(self, i):
        # A compound command may only be followed by a list separator: a pipe or
        # redirect on it would carry its inner output somewhere we do not track.
        i = self.blanks(i)
        if i >= self.n or self.s[i] in '\n;)#':
            return i
        if self.at(i, '&&') or self.at(i, '||') or (self.s[i] == '&' and not self.at(i, '&>')):
            return i
        raise Raw()

    def script(self, i, closer):
        self.depth += 1
        if self.depth > MAX_DEPTH:
            raise Raw()
        try:
            return self.parse_script(i, closer)
        finally:
            self.depth -= 1

    def parse_script(self, i, closer):
        s = self.s
        sc = Script()
        pending, pipeline, cur, pipe_open = [], [], None, False
        while True:
            if i >= self.n:
                if closer or pending or pipe_open:
                    raise Raw()
                if cur is not None:
                    pipeline.append(cur)
                if pipeline:
                    sc.pipelines.append(pipeline)
                return sc, i
            c = s[i]
            if c in ' \t':
                i += 1
                continue
            if self.at(i, '\\\n'):
                i += 2
                continue
            if c == '\n':
                if self.pending_total > len(pending):
                    raise Raw()
                i = self.heredocs(i + 1, pending)
                pending = []
                if not pipe_open:
                    if cur is not None:
                        pipeline.append(cur)
                        cur = None
                    if pipeline:
                        sc.pipelines.append(pipeline)
                        pipeline = []
                continue
            if c == '#':
                j = self.find('\n', i)
                j = self.n if j < 0 else j
                sc.comments.append((i, j))
                i = j
                continue
            if c == ';' or (c == '&' and not self.at(i, '&>')) or self.at(i, '||'):
                if self.at(i, ';;') or self.at(i, ';&') or pipe_open:
                    raise Raw()
                if cur is not None:
                    pipeline.append(cur)
                    cur = None
                if pipeline:
                    sc.pipelines.append(pipeline)
                    pipeline = []
                i += 2 if (self.at(i, '&&') or self.at(i, '||')) else 1
                continue
            if c == '|':
                if cur is None:
                    raise Raw()
                pipeline.append(cur)
                cur, pipe_open = None, True
                i += 2 if self.at(i, '|&') else 1
                continue
            if c == '(':
                if cur is not None:
                    raise Raw()
                sub, j = self.script(i + 1, ')')
                pipeline.append(Subshell(sub))
                pipe_open = False
                i = self.after_compound(j + 1)
                continue
            if c == ')':
                if closer != ')' or pending or pipe_open:
                    raise Raw()
                if cur is not None:
                    pipeline.append(cur)
                if pipeline:
                    sc.pipelines.append(pipeline)
                return sc, i
            if cur is None:
                cur = Simple()
            pipe_open = False
            if c in '<>' and self.at(i + 1, '('):
                w, i = self.word(i)
                cur.words.append(w)
                continue
            m = REDIR.match(s, i, self.n)
            if m:
                fd = int(m.group(1)) if m.group(1) else None
                op = m.group(2)
                w, i = self.word(self.blanks(m.end()))
                if op in ('<<', '<<-'):
                    if w.subs or '$' in w.text or '`' in w.text:
                        raise Raw()
                    delim = w.text.replace("'", '').replace('"', '').replace('\\', '')
                    if not delim:
                        raise Raw()
                    hd = Heredoc(delim, op == '<<-', any(q in w.text for q in '\'"\\'))
                    pending.append(hd)
                    self.pending_total += 1
                    cur.heredocs.append(hd)
                else:
                    cur.redirs.append(Redir(fd, op, w))
                continue
            w, i = self.word(i)
            closes = w.text in CLOSERS and all(x.text in RESERVED for x in cur.words)
            cur.words.append(w)
            if closes:
                i = self.after_compound(i)

    def heredocs(self, i, pending):
        s = self.s
        for hd in pending:
            start = i
            while True:
                if i >= self.n:
                    raise Raw()
                j = self.find('\n', i)
                end = self.n if j < 0 else j
                line = s[i:end]
                if (line.lstrip('\t') if hd.dash else line) == hd.delim:
                    hd.body = (start, i)
                    i = self.n if j < 0 else j + 1
                    break
                if j < 0:
                    raise Raw()
                i = j + 1
        self.pending_total -= len(pending)
        return i

    def word(self, i):
        s = self.s
        start, subs = i, []
        while i < self.n:
            c = s[i]
            if c in '<>' and i == start and self.at(i + 1, '('):
                sub, j = self.script(i + 2, ')')
                subs.append(('proc', i + 2, j, sub))
                i = j + 1
                continue
            if c in ' \t\n;&|<>)':
                break
            if c == '(':
                raise Raw()
            if c == '\\':
                if i + 1 >= self.n:
                    raise Raw()
                i += 2
            elif c == "'":
                j = self.find("'", i + 1)
                if j < 0:
                    raise Raw()
                i = j + 1
            elif c == '"':
                i = self.dquote(i + 1, subs)
            elif c == '`':
                i = self.backtick(i, subs)
            elif c == '$' and self.at(i + 1, "'"):
                i = self.ansi(i + 2)
            elif c == '$' and self.at(i + 1, '"'):
                i = self.dquote(i + 2, subs)
            elif c == '$' and self.at(i + 1, '('):
                i = self.cmdsub(i, subs)
            elif c == '$' and self.at(i + 1, '{'):
                i = self.brace(i + 2)
            else:
                i += 1
        if i == start:
            raise Raw()
        return Word(start, i, s[start:i], subs), i

    def dquote(self, i, subs):
        s = self.s
        while i < self.n:
            c = s[i]
            if c == '\\':
                i += 2
            elif c == '"':
                return i + 1
            elif c == '`':
                i = self.backtick(i, subs)
            elif c == '$' and self.at(i + 1, '('):
                i = self.cmdsub(i, subs)
            elif c == '$' and self.at(i + 1, '{'):
                i = self.brace(i + 2)
            else:
                i += 1
        raise Raw()

    def ansi(self, i):
        while i < self.n:
            c = self.s[i]
            if c == '\\':
                i += 2
            elif c == "'":
                return i + 1
            else:
                i += 1
        raise Raw()

    def brace(self, i):
        j = self.find('}', i)
        if j < 0 or any(ch in self.s[i:j] for ch in '\'"`(){\\'):
            raise Raw()
        return j + 1

    def cmdsub(self, i, subs):
        if self.at(i, '$(('):
            raise Raw()
        sub, j = self.script(i + 2, ')')
        subs.append(('cmd', i + 2, j, sub))
        return j + 1

    def backtick(self, i, subs):
        j = self.find('`', i + 1)
        if j < 0 or '\\' in self.s[i + 1:j]:
            raise Raw()
        limit, self.n = self.n, j
        try:
            sub, _ = self.script(i + 1, None)
        finally:
            self.n = limit
        subs.append(('cmd', i + 1, j, sub))
        return j + 1


def effective(st):
    ws, k = st.words, 0
    while k < len(ws):
        t = ws[k].text
        if ASSIGN.match(t) or t in RESERVED:
            k += 1
        elif t in WRAPPERS and k + 1 < len(ws) and not ws[k + 1].text.startswith('-'):
            k += 1
        else:
            break
    if k >= len(ws):
        return k, None
    w = ws[k]
    return k, (w.text if LITERAL.fullmatch(w.text) and not w.subs else None)


def stdout_ok(st):
    for r in st.redirs:
        op, t = r.op, r.target.text
        if op in ('<', '<<<') or (op == '<&' and r.fd in (None, 0)):
            continue
        if op in ('>&', '<&') and t in ('1', '2'):
            continue
        if t in SAFE_TARGETS and not r.target.subs:
            continue
        return False
    return True


def body_reader(st, k, name):
    args = st.words[k + 1:]
    if name == 'git' and args and args[0].text in GIT_MSG_OBJ:
        flags, rest = GIT_FILE_FLAGS, args[1:]
    elif name == 'gh' and len(args) >= 2 and args[0].text in GH_OBJ and args[1].text in GH_VERBS:
        flags, rest = GH_FILE_FLAGS, args[2:]
    else:
        return False
    for p, w in enumerate(rest):
        if w.text in flags and p + 1 < len(rest) and rest[p + 1].text == '-':
            return True
        if any(w.text == f + '=-' for f in flags if f.startswith('--')):
            return True
    return False


def downstream_ok(pipeline):
    """For each stage: can its output only reach text filters and body readers?

    Computed once per pipeline, from the last stage back, so a long pipeline
    costs linear time."""
    ok, after = [True] * len(pipeline), True
    for idx in range(len(pipeline) - 1, -1, -1):
        ok[idx] = after
        st = pipeline[idx]
        if isinstance(st, Subshell) or not stdout_ok(st):
            after = False
        else:
            k, name = effective(st)
            after = after and (name in SINKS or body_reader(st, k, name))
    return ok


def flag_values(args, spec):
    flags, attached = spec
    inert = set()
    for p, w in enumerate(args):
        t = w.text
        if t in flags:
            if p + 1 < len(args):
                inert.add(id(args[p + 1]))
        elif any(t.startswith(f + '=') for f in flags if f.startswith('--')):
            inert.add(id(w))
        elif any(t.startswith(a) and len(t) > len(a) for a in attached):
            inert.add(id(w))
    return inert


def blank(out, s, a, b):
    for p in range(a, b):
        if s[p] != '\n':
            out[p] = ' '


def walk(sc, ok, s, out):
    if ok:
        for a, b in sc.comments:
            blank(out, s, a, b)
    for pipeline in sc.pipelines:
        downstream = downstream_ok(pipeline)
        for idx, st in enumerate(pipeline):
            if isinstance(st, Subshell):
                walk(st.script, ok, s, out)
            else:
                walk_simple(st, downstream[idx], ok, s, out)


def walk_simple(st, downstream, ok, s, out):
    k, name = effective(st)
    if name in DEFINERS or (name == 'exec' and k == len(st.words) - 1):
        raise Raw()
    if any(w.text.rsplit('/', 1)[-1] in EXECUTORS for w in st.words):
        raise Raw()
    if k < len(st.words) and (name is None or '/' in name or name == '.'):
        raise Raw()
    ctx = ok and name is not None and stdout_ok(st) and downstream
    inert = set()
    if ctx:
        args = st.words[k + 1:]
        if name == 'echo' or (name == 'printf' and not any(a.text.startswith('-v') for a in args)):
            inert = {id(a) for a in args}
        elif name == 'git' and args and args[0].text in GIT_MSG_OBJ:
            inert = flag_values(args[1:], GIT_FLAGS)
        elif name == 'gh' and len(args) >= 2 and args[0].text in GH_OBJ and args[1].text in GH_VERBS:
            inert = flag_values(args[2:], GH_FLAGS)
    for w in st.words:
        if id(w) in inert:
            pos = w.start
            for kind, a, b, sub in w.subs:
                blank(out, s, pos, a)
                pos = b
                walk(sub, kind == 'cmd', s, out)
            blank(out, s, pos, w.end)
        else:
            for kind, a, b, sub in w.subs:
                walk(sub, False, s, out)
    for r in st.redirs:
        for kind, a, b, sub in r.target.subs:
            walk(sub, False, s, out)
    reads = ctx and bool(st.heredocs) and (name in SINKS or body_reader(st, k, name))
    for hd in st.heredocs:
        a, b = hd.body
        expands = '$(' in s[a:b] or '`' in s[a:b]
        if reads and (hd.quoted or not expands):
            blank(out, s, a, b)


def scan_text(cmd):
    out = cmd
    if not CASE.search(cmd):
        try:
            sc, _ = Parser(cmd).script(0, None)
            chars = list(cmd)
            walk(sc, True, cmd, chars)
            out = ''.join(chars)
        except Exception:
            out = cmd
    # Continuations are joined so a continued command matches as one grep line.
    # NUL is dropped here rather than by bash's $(...), which the opencode twin mirrors.
    return out.replace('\\\n', '  ').replace('\0', '')


d = json.load(sys.stdin)
command = d.get('tool_input', {}).get('command', '')
scanned = scan_text(command if isinstance(command, str) else str(command))
# The first line is proof that this scan ran. Written only here, once
# scan_text has returned, so a run that skipped the masking pass -- or stopped
# part-way through it -- cannot produce it; the shell blocks when it is
# missing, whatever this process's exit status.
sys.stdout.write(sys.argv[1] + '\n' + scanned + 'x')
PY

# The trailing "x" keeps $(...) from trimming newlines that belong to the text.
scan=$(printf '%s' "$input" | python3 -I -c "$GUARD_PY" "$DEVEXP_SCAN_PROOF") || {
    echo "[devexp dangerous-cmd-guard] internal error -- the guard could not read its input, so it did not run. Blocking to be safe; the interpreter's error is above." >&2
    exit 2
}
devexp_scan_result dangerous-cmd-guard "$scan"
scan="$devexp_scanned"
scan=${scan%x}

# The one place a pattern is handed to grep. Every check below goes through it,
# and so do the probes that certify it — a question asked in some other
# mode certifies a code path the guard never takes.
#
# grep reads a here-string, not a pipe: with pipefail, a pipe writer killed by
# SIGPIPE when `grep -q` stops at its first match fails the pipeline and turns
# a match into "no match". `-e` keeps a pattern from being read as an option.
devexp_grep() { # $1=grep flags  $2=pattern  $3=subject -> 0 hit, 1 miss, else grep's own status
    local rc=0
    grep -q "$1" -e "$2" <<<"$3" || rc=$?
    return "$rc"
}

# Any grep error blocks.
matches() { # $1=grep flags  $2=pattern
    local rc=0
    devexp_grep "$1" "$2" "$scan" || rc=$?
    case "$rc" in
        0) return 0 ;;
        1) return 1 ;;
        *) echo "[devexp dangerous-cmd-guard] internal error -- grep exited $rc while checking the command. Blocking to be safe." >&2
           exit 2 ;;
    esac
}

# `matches` reads grep's exit status and nothing else, so every check below is
# only as good as that status. These probes ask questions whose answers
# are known, through devexp_grep — the same call shape, `-q` and `-e` and a
# here-string included — because a grep honest in some other mode and blind
# under `-q` would otherwise pass and then report every command clean.
#
# A shared call shape is not enough on its own: a grep can also differ in the
# regular expressions it understands. `\s` is a GNU extension, so a strict-POSIX
# or busybox-style grep reads it as a literal `s` and quietly under-matches
# every pattern below that uses it — no error, no match, every command clean.
# That is an accident waiting on someone's PATH, not an attack, so each probe
# pattern carries the same vocabulary the real patterns do: `\s`, `\b`, `\S`, a
# POSIX class, a bracket range, a literal brace, alternation, a group, `+`, `*`,
# `?` and both anchors. interpreter-proof.test.sh reads the patterns below and
# fails if one of them uses a construct no probe exercises, so the two cannot
# drift apart.
#
# The subject is two lines and the answer is on the second, because a grep that
# only ever sees the first line would otherwise pass every probe and then miss
# any dangerous command that is not on line 1.
#
# Three probes, and each is the only one that catches something:
#   * a hit, so a grep that never matches — or refuses one of the constructs,
#     or drops -E, or reads only the first line — cannot pass;
#   * a miss with the same vocabulary, so one that matches everything cannot;
#   * a case-insensitive hit, because half the checks below use -iE.
devexp_grep_canary="devexp-grep-canary-$$-${RANDOM}-${RANDOM}"
devexp_grep_subject="not-the-canary
$devexp_grep_canary {end}"
devexp_grep_probe() { # $1=flags  $2=pattern  $3=expected status (0 hit, 1 miss)
    local rc=0
    devexp_grep "$1" "$2" "$devexp_grep_subject" || rc=$?
    [ "$rc" = "$3" ]
}
if ! devexp_grep_probe -E  '^(zz|devexp)\b-grep-canary-[0-9]+-[0-9]*[0-9]-[0-9]+\s+[{][a-z]\S*[[:alpha:]]}?$' 0 ||
   ! devexp_grep_probe -E  '^(zz|qq)\b-grep-canary-[0-9]+-[0-9]*[0-9]-[0-9]+\s+[{][a-z]\S*[[:alpha:]]}?$' 1 ||
   ! devexp_grep_probe -iE '^(ZZ|DEVEXP)\b-GREP-CANARY-[0-9]+-[0-9]*[0-9]-[0-9]+\s+[{][A-Z]\S*[[:alpha:]]}?$' 0; then
    echo "[devexp dangerous-cmd-guard] internal error -- grep answered a question with a known answer wrongly, so the command was not checked. Blocking to be safe." >&2
    exit 2
fi

# ── Blocked patterns ───────────────────────────────────────────────────────────

# Where a target ends (END). Whitespace, end of line, or a character that ends
# the word or changes what it expands to:
# - ' " ` ( ) ; & | < >  closes the word or starts a redirect, so
#   `sh -c 'rm -rf /'`, `$(rm -rf ~)` and `git push --force;` match;
# - $ { * ? [  and @( +( !(  start an expansion that can leave the target
#   itself: a parameter, command or ANSI-C expansion that may be empty or split
#   the word, brace expansion, a glob, a zsh subscript or qualifier, an extglob.
# Every other character continues the target, so `./build`, `~/projects/$x`,
# `/tmp/.deliver-$id-*` and `-v /tmp:/data` don't match. (`:` changes a word
# only right after an unbraced parameter name; rule 1 lists `$HOME:` itself.)
END='(\s|$|["'\''`();&|<>$*{?[]|[@+!]\()'
# The home directory (HOME), spelled `$HOME` (zsh `$~HOME`, `$=HOME`, `$^HOME`),
# `${HOME}` with any operator, subscript, flags or modifier, `~` or `~name`.
HOME_RE='(\$[~=^]*HOME|\$[{][~=^]*(\([^()${}]*\))?HOME([-=?+#%/^,@:[][^${}]*)?[}]|~([A-Za-z_][A-Za-z0-9._-]*)?)'
# `rm` as a word of its own: not part of a longer word or option (`--rm`,
# `terraform`). It may still be an argument (`xargs rm`, `find -exec rm`), or
# follow a positional parameter that may be empty (`$1rm`).
RM_WORD='(^|[^A-Za-z0-9_.-]|\$[0-9])rm'

# rm -rf targeting filesystem root or home directory (optionally quoted)
if matches -E "$RM_WORD"'\s+-[a-z]*r[a-z]*f\s+["'\'']?((/|'"$HOME_RE"'/?)'"$END"'|\$[~=^]*HOME:)' || \
   matches -E "$RM_WORD"'\s+-[a-z]*f[a-z]*r\s+["'\'']?((/|'"$HOME_RE"'/?)'"$END"'|\$[~=^]*HOME:)'; then
    echo "[devexp dangerous-cmd-guard] Blocked: 'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory." >&2
    exit 2
fi

# Unanchored wildcard delete in a sensitive dir: a '*' immediately after the dir boundary
# (/tmp/* , ~/.claude/.../* ), or the dir wholesale (rm -rf /tmp , rm -rf ~/.claude).
# This is the blanket-wipe an empty variable produces — `rm -f /tmp/*"$id"*` with empty $id
# collapses to `/tmp/*`, and the template itself contains `/tmp/*`. Prefix-anchored globs like
# `/tmp/.deliver-PAY-123-*` are allowed (no '*' right after the '/').
# The target must follow `rm` in the same simple command: the scan (SCAN) stops
# at `;`, `|` and a `&` that isn't part of a redirect (`2>&1`, `&>`), so
# `rm -rf dist && cp out /tmp/$x` doesn't match. But a quote, an escape, a
# backtick, `$(`, `${`, `$[` or a process substitution (`<(`, `>(`, zsh `=(`) can hold
# a `;` or `&` that is data (`$'…'` starts with a quote), so once one of those
# opens (OPENER) the scan runs on to the next `|`. Right after `rm` a `=(` is an
# array assignment, not rm, so START_OPENER leaves it out.
# `rm` itself may be followed by whitespace, a quote, an expansion, a brace or
# glob character, or a redirect: the shell still runs `rm` with what follows.
SENSITIVE='(\s["'\'']?/tmp["'\'']?/?'"$END"'|["'\'']?'"$HOME_RE"'["'\'']?/\.claude["'\'']?/?'"$END"'|\.claude\S*/\*)'
SCAN='([^|;&]|[<>]&|&>)*'
OPENER='(["'\''\\`]|[$<>=][(]|\$[{[])'
START_OPENER='(["'\''`]|[$<>][(]|\$[{[])'
AFTER_RM='([[:space:]<>${},*?[]|&>|[<>]&)'
if matches -E "$RM_WORD"'(\s["'\'']?/tmp["'\'']?/?'"$END"'|('"$START_OPENER"'[^|]*|'"$AFTER_RM$SCAN"'('"$OPENER"'[^|]*)?)'"$SENSITIVE"')'; then
    echo "[devexp dangerous-cmd-guard] Blocked: unanchored wildcard delete in a sensitive directory (e.g. '/tmp/*' or '~/.claude/.../*'). Anchor the glob with a literal prefix (e.g. '/tmp/.deliver-<id>-*') so an empty variable cannot collapse it into a blanket wipe." >&2
    exit 2
fi

# Fork bomb
if matches -E ':\s*\(\s*\)\s*\{.*\|.*:'; then
    echo "[devexp dangerous-cmd-guard] Blocked: fork bomb pattern detected." >&2
    exit 2
fi

# DROP DATABASE (immediate data loss)
if matches -iE 'DROP\s+DATABASE'; then
    echo "[devexp dangerous-cmd-guard] Blocked: DROP DATABASE would permanently destroy a database. Confirm with the user before proceeding." >&2
    exit 2
fi

# Force push — match the force flag only as an argument of the SAME push command (no intervening
# ; | & ), so an unrelated `-f` elsewhere (e.g. `rm -f` in a commit message) no longer false-positives.
if matches -E 'git\s+push\b[^|&;]*\s(--force-with-lease|--force|-f)(=|'"$END"')'; then
    echo "[devexp dangerous-cmd-guard] Blocked: git push --force can overwrite remote history and affect other contributors." >&2
    exit 2
fi

if matches -E 'git\s+reset\b.*--hard'; then
    echo "[devexp dangerous-cmd-guard] Blocked: git reset --hard will permanently discard all uncommitted changes." >&2
    exit 2
fi

if matches -E 'git\s+clean\b.*-[a-z]*f'; then
    echo "[devexp dangerous-cmd-guard] Blocked: git clean -f will permanently delete untracked files." >&2
    exit 2
fi

if matches -iE '(DROP\s+TABLE|TRUNCATE\s+TABLE)'; then
    echo "[devexp dangerous-cmd-guard] Blocked: DROP TABLE or TRUNCATE TABLE will permanently destroy table data." >&2
    exit 2
fi

exit 0
