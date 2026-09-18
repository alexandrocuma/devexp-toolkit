/**
 * dangerous-cmd-guard.js — hard-blocks destructive shell commands
 *
 * Event: tool.execute.before (tool: bash)
 *
 * All guarded patterns are hard-blocked (throw). No prompts.
 *
 * A mention is not an invocation (#100). Before the patterns run, text that
 * provably never executes is blanked out (maskInert): arguments of
 * `echo`/`printf`, commit/tag messages, `gh` issue/PR/release bodies, heredoc
 * bodies fed to a plain text sink, and comments — only when that text cannot
 * reach a shell. Everything else is scanned as before. Anything the parser
 * cannot classify is scanned whole. Mirrors hooks/claude-code/dangerous-cmd-guard.sh.
 *
 * The scan runs under a wall-clock budget and refuses the call when it is
 * exceeded (#162) — see startScanBudget in utils.js.
 *
 * Tests: node hooks/opencode/dangerous-cmd-guard.test.js
 */

import { ScanBudgetError, startScanBudget } from './utils.js';

// Where a target ends (END). Whitespace, end of line, or a character that ends the word or
// changes what it expands to:
// - ' " ` ( ) ; & | < >  closes the word or starts a redirect, so `sh -c 'rm -rf /'`,
//   `$(rm -rf ~)` and `git push --force;` match;
// - $ { * ? [  and @( +( !(  start an expansion that can leave the target itself: a
//   parameter, command or ANSI-C expansion that may be empty or split the word, brace
//   expansion, a glob, a zsh subscript or qualifier, an extglob.
// Every other character continues the target, so `./build`, `~/projects/$x`,
// `/tmp/.deliver-$id-*` and `-v /tmp:/data` don't match. (`:` changes a word only right
// after an unbraced parameter name; rule 1 lists `$HOME:` itself.)
const END = String.raw`(?:\s|$|["'\x60();&|<>$*{?[]|[@+!]\()`;
// The home directory (HOME), spelled `$HOME` (zsh `$~HOME`, `$=HOME`, `$^HOME`), `${HOME}`
// with any operator, subscript, flags or modifier, `~` or `~name`.
const HOME = String.raw`(?:\$[~=^]*HOME|\$\{[~=^]*(?:\([^()$\{\}]*\))?HOME(?:[-=?+#%/^,@:[][^$\{\}]*)?\}|~(?:[A-Za-z_][A-Za-z0-9._-]*)?)`;
// `rm` as a word of its own: not part of a longer word or option (`--rm`, `terraform`). It
// may still be an argument (`xargs rm`, `find -exec rm`), or follow a positional parameter
// that may be empty (`$1rm`).
const RM_WORD = String.raw`(?:(?<![A-Za-z0-9_.-])|(?<=\$[0-9]))rm`; // grep: (^|[^A-Za-z0-9_.-]|\$[0-9])rm

// Every rule decides one line at a time, as the Claude Code hook's line-by-line grep does
// (blockReason splits the text; a backslash-continued command was joined by maskInert).
// Within a line, `\s` is whitespace, CR included, as it is for grep's `\s`.
//
// Each rule runs in time linear in the line (#146). JavaScript's regex engine backtracks,
// so the grep patterns are not copied as they are:
// - "rm, then -flags" names the first 'r' (or 'f') of the flags, so the letters are not
//   split every possible way;
// - "PREFIX, then any text without STOP, then SUFFIX" is decided by `sequence`: the first
//   PREFIX of each segment (text up to STOP, or up to where `commandEnd` says one simple
//   command ends) is the only one that matters, and SUFFIX is searched for once, left to
//   right, across all segments;
// - `\.claude\S*\/\*` is decided by `claudeGlob` from one right-to-left pass.

/** SUFFIX as a regex: does a match start in [from, to]? Calls must not move `from` back. */
function leftmost(source, flags = '') {
  const re = new RegExp(source, `g${flags}`);
  return (line) => {
    let searched = -1;
    let at = -1;
    return (from, to) => {
      if (searched < 0 || (at >= 0 && at < from)) {
        re.lastIndex = from;
        const m = re.exec(line);
        searched = from;
        at = m ? m.index : -1;
      }
      return at >= 0 && at <= to;
    };
  };
}

/** `\.claude\S*\/\*`: does a match start in [from, to]? Calls must not move `from` back. */
function claudeGlob(line) {
  if (!line.includes('.claude')) return () => false;
  let lastGlob = null;
  const starts = [];
  let k = 0;
  return (from, to) => {
    if (lastGlob === null) {
      // lastGlob[i]: where the last '/*' begins that is reachable from i without whitespace, or -1.
      const n = line.length;
      lastGlob = new Int32Array(n + 1).fill(-1);
      for (let i = n - 1; i >= 0; i--) {
        if (/\s/.test(line[i])) continue;
        if (lastGlob[i + 1] >= 0) lastGlob[i] = lastGlob[i + 1];
        else if (line[i] === '/' && line[i + 1] === '*') lastGlob[i] = i;
      }
      for (let i = line.indexOf('.claude'); i >= 0; i = line.indexOf('.claude', i + 7)) starts.push(i);
    }
    while (k < starts.length && starts[k] < from) k++;
    for (; k < starts.length && starts[k] <= to; k++) if (lastGlob[starts[k] + 7] >= 0) return true;
    return false;
  };
}

/**
 * Where the text after `start` stops belonging to one simple command, for `([^|;&]|[<>]&|&>)*`:
 * `;` and `|` end it, and so does a `&` unless it is part of a redirect. A `&` joins the `<` or
 * `>` just before it when that character stands alone; otherwise it needs a `>` right after.
 * Joining backwards first leaves the most room, so this finds the end the pattern can reach.
 */
const PIPE = 124; // |
const SEMI = 59; // ;
const AMP = 38; // &
const LT = 60; // <
const GT = 62; // >

function commandEnd(line, start) {
  const n = line.length;
  let i = start;
  let single = false; // the previous character is a `<` or `>` not yet joined to a `&`
  while (i < n) {
    const c = line.charCodeAt(i);
    if (c === PIPE || c === SEMI) return i;
    if (c === AMP) {
      if (single) {
        single = false;
        i += 1;
      } else if (i + 1 < n && line.charCodeAt(i + 1) === GT) {
        single = false;
        i += 2;
      } else {
        return i;
      }
      continue;
    }
    single = c === LT || c === GT;
    i += 1;
  }
  return i;
}

/**
 * commandEnd, unless a quote, escape, backtick, `$(`, `${`, `$[` or a process substitution (`<(`, `>(`,
 * zsh `=(`) opens before that end (`$'…'` starts with a quote): it can hold a `;` or `&` that is
 * data, so the scan runs on to the next `|`. (The lookahead after `rm` never admits `=`.) The
 * grep pattern is `START_OPENER[^|]*` right after `rm`, or `SCAN(OPENER[^|]*)?` after the
 * character that follows it.
 */
function commandEndQuoted(line, start) {
  const end = commandEnd(line, start);
  for (let i = start; i < end; i++) {
    const c = line.charCodeAt(i);
    // " ' \ ` open at once; `${`, `$[`, and `$(` `<(` `>(` `=(`, open a substitution. Character codes keep
    // this loop fast on long lines; past the end of the line charCodeAt gives NaN.
    const next = line.charCodeAt(i + 1);
    const opens = c === 34 || c === 39 || c === 92 || c === 96
      || (c === 36 && (next === 123 || next === 91 || next === 40))
      || ((c === LT || c === GT || c === 61) && next === 40);
    if (opens) {
      const pipe = line.indexOf('|', end);
      return pipe < 0 ? line.length : pipe;
    }
  }
  return end;
}

/**
 * In the grep pattern `rm` is followed either by `\s/tmp…` at once, or by one more character
 * before any other target can start. So a finder is asked from `start + 1`, except for that one form.
 */
function afterRm(finder) {
  const tmpNow = new RegExp(String.raw`\s["']?\/tmp["']?\/?${END}`, 'y');
  return (line) => {
    const found = finder(line);
    return (from, to) => {
      tmpNow.lastIndex = from;
      return tmpNow.test(line) || found(from + 1, to);
    };
  };
}

/**
 * PREFIX, then any run of characters not in `stop`, then SUFFIX. `suffix(line)` gives a finder.
 * `stop` is a string of characters, or a function (line, start) => end.
 */
function sequence(prefix, stop, suffix, flags = '') {
  const pre = new RegExp(prefix, `g${flags}`);
  return (line) => {
    const found = suffix(line);
    let pos = 0;
    for (;;) {
      pre.lastIndex = pos;
      const m = pre.exec(line);
      if (!m) return false;
      const start = m.index + m[0].length;
      let end = start;
      if (typeof stop === 'function') end = stop(line, start);
      else while (end < line.length && !stop.includes(line[end])) end += 1;
      if (found(start, end)) return true;
      if (end >= line.length) return false;
      pos = end + 1;
    }
  };
}

const regex = (source, flags = '') => {
  const re = new RegExp(source, flags);
  return (line) => re.test(line);
};
const either = (...finders) => (line) => {
  const fs = finders.map((f) => f(line));
  return (from, to) => fs.some((f) => f(from, to));
};

const WIPE = "'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory";
const TABLE = 'DROP TABLE will permanently destroy table data';

export const BLOCK_PATTERNS = [
  {
    // rm -rf targeting filesystem root or home directory (optionally quoted)
    test: regex(String.raw`${RM_WORD}\s+-[a-qs-z]*r[a-z]*f\s+["']?(?:(?:\/|${HOME}\/?)${END}|\$[~=^]*HOME:)`),
    label: WIPE,
  },
  {
    test: regex(String.raw`${RM_WORD}\s+-[a-eg-z]*f[a-z]*r\s+["']?(?:(?:\/|${HOME}\/?)${END}|\$[~=^]*HOME:)`),
    label: WIPE,
  },
  {
    // Unanchored wildcard delete in a sensitive dir (/tmp/* , ~/.claude/.../* , or the dir
    // wholesale) — the blanket wipe an empty variable produces. Prefix-anchored globs like
    // /tmp/.deliver-PAY-123-* are allowed (no '*' right after the '/'). The target must follow
    // `rm` in the same simple command: the scan stops at `;`, `|` and a `&` that isn't part of
    // a redirect (`2>&1`, `&>`), unless a quote or substitution opens first. `rm` may be followed
    // by whitespace, a quote, an expansion, a brace or glob character, or a redirect.
    test: sequence(
      String.raw`${RM_WORD}(?=[\s"'$\x60<>{},*?[]|&>)`,
      commandEndQuoted,
      afterRm(
        either(
          leftmost(String.raw`\s["']?\/tmp["']?\/?${END}|["']?${HOME}["']?\/\.claude["']?\/?${END}`),
          claudeGlob,
        ),
      ),
    ),
    label:
      "unanchored wildcard delete in a sensitive directory (e.g. '/tmp/*' or '~/.claude/.../*') — anchor the glob with a literal prefix like '/tmp/.deliver-<id>-*' so an empty variable cannot collapse it into a blanket wipe",
  },
  {
    test: sequence(String.raw`:\s*\(\s*\)\s*\{`, '', (line) => (from) => {
      const pipe = line.indexOf('|', from);
      return pipe >= 0 && line.indexOf(':', pipe + 1) >= 0;
    }),
    label: 'fork bomb pattern detected',
  },
  {
    test: regex(String.raw`DROP\s+DATABASE`, 'i'),
    label: 'DROP DATABASE would permanently destroy a database',
  },
  {
    // Force flag must be an argument of the same push command (no intervening ; | & ), so an
    // unrelated `-f` elsewhere (e.g. `rm -f` in a commit message) no longer false-positives.
    test: sequence(String.raw`git\s+push\b`, '|&;', leftmost(String.raw`\s(?:--force-with-lease|--force|-f)(?:=|${END})`)),
    label: 'git push --force can overwrite remote history and affect other contributors',
  },
  {
    test: sequence(String.raw`git\s+reset\b`, '', (line) => (from) => line.indexOf('--hard', from) >= 0),
    label: 'git reset --hard will permanently discard all uncommitted changes',
  },
  {
    test: sequence(String.raw`git\s+clean\b`, '', leftmost(String.raw`-[a-eg-z]*f`)),
    label: 'git clean -f will permanently delete untracked files',
  },
  { test: regex(String.raw`DROP\s+TABLE`, 'i'), label: TABLE },
  { test: regex(String.raw`TRUNCATE\s+TABLE`, 'i'), label: 'TRUNCATE TABLE will permanently destroy table data' },
];

// ── Inert-text masking (#100) ─────────────────────────────────────────────────

/** The command cannot be classified: scan all of it. */
class Raw extends Error {}

const RESERVED = new Set(['{', '!', 'if', 'then', 'else', 'elif', 'while', 'until', 'do', 'time']);
const CLOSERS = new Set(['}', 'fi', 'done']);
const WRAPPERS = new Set(['sudo', 'env', 'command', 'builtin', 'exec', 'time']);
const DEFINERS = new Set(['alias', 'unalias', 'function', 'hash', 'enable', 'coproc']);
const SINKS = new Set(['cat', 'head', 'tail', 'wc', 'grep', 'egrep', 'fgrep', 'tr', 'cut', 'nl']);
// Anything that can run text it reads (stdin, a file, the clipboard, git or
// GitHub). If one appears anywhere, masked text could be read back and run in
// the same command, so nothing is masked.
const EXECUTORS = new Set(['sh', 'bash', 'zsh', 'dash', 'ksh', 'mksh', 'fish', 'csh', 'tcsh', 'ash', 'busybox',
  'eval', 'source', 'xargs', 'ssh', 'su', 'script', 'python', 'python2', 'python3',
  'perl', 'ruby', 'node', 'deno', 'bun', 'php', 'awk', 'gawk', 'mawk', 'nawk', 'sed',
  'osascript']);
const SAFE_TARGETS = new Set(['/dev/null', '/dev/stdout', '/dev/stderr', '/dev/tty']);
const GIT_MSG_OBJ = new Set(['commit', 'tag']);
const GH_OBJ = new Set(['issue', 'pr', 'release']);
const GH_VERBS = new Set(['create', 'comment', 'edit', 'review', 'close', 'merge']);
const GIT_FLAGS = [new Set(['-m', '-am', '--message']), ['-m']];
const GH_FLAGS = [new Set(['-b', '--body', '-t', '--title', '-n', '--notes', '-c', '--comment']), []];
const GIT_FILE_FLAGS = new Set(['-F', '--file']);
const GH_FILE_FLAGS = new Set(['-F', '--body-file', '--notes-file']);
const ASSIGN = /^[A-Za-z_][A-Za-z0-9_]*(\[[^\]]*\])?\+?=/;
const LITERAL = /^[A-Za-z0-9_./:@%+,=-]+$/;
const REDIR = /([0-9]*)(&>>|&>|>>|>\||>&|>|<<<|<<-|<<|<&|<>|<)/y;
const CASE = /\besac\b/;
// Deeper nesting of (…), $(…), backticks and <(…) is scanned whole. The same
// limit in both implementations keeps Python's recursion limit from deciding.
const MAX_DEPTH = 100;

// How far the masking pass may run between budget checks, in characters of the
// command or nodes of the parse. Below this the clock costs more than it saves;
// above it the overshoot stops being a rounding error on a command of tens of
// megabytes.
const BUDGET_STEP = 65536;

class Parser {
  constructor(s, overBudget = () => {}) {
    this.s = s;
    this.n = s.length;
    this.pendingTotal = 0;
    this.depth = 0;
    this.found = new Map(); // ch -> [from, index]: the last indexOf, reused while still valid
    this.overBudget = overBudget;
    this.nextCheck = 0;
  }

  // The scan budget, sampled by position: `i` only moves forward through the
  // command, so this fires about once every BUDGET_STEP characters however the
  // parse jumps between its loops. A single word, quoted string or run of
  // whitespace can be the whole command, so each of those loops ticks too.
  tick(i) {
    if (i >= this.nextCheck) {
      this.nextCheck = i + BUDGET_STEP;
      this.overBudget();
    }
  }

  at(i, tok) {
    return i + tok.length <= this.n && this.s.startsWith(tok, i);
  }

  // Python's bounded str.find. indexOf cannot stop at this.n, so its answer is
  // cached: a search from a later position before that answer returns it again.
  find(ch, i) {
    const hit = this.found.get(ch);
    let j;
    if (hit && hit[0] <= i && (hit[1] < 0 || i <= hit[1])) {
      j = hit[1];
    } else {
      j = this.s.indexOf(ch, i);
      this.found.set(ch, [i, j]);
    }
    return j < 0 || j >= this.n ? -1 : j;
  }

  blanks(i) {
    while (i < this.n) {
      this.tick(i);
      if (this.s[i] === ' ' || this.s[i] === '\t') i += 1;
      else if (this.at(i, '\\\n')) i += 2;
      else break;
    }
    return i;
  }

  // A compound command may only be followed by a list separator: a pipe or
  // redirect on it would carry its inner output somewhere we do not track.
  afterCompound(i) {
    i = this.blanks(i);
    if (i >= this.n || '\n;)#'.includes(this.s[i])) return i;
    if (this.at(i, '&&') || this.at(i, '||') || (this.s[i] === '&' && !this.at(i, '&>'))) return i;
    throw new Raw();
  }

  script(i, closer) {
    this.depth += 1;
    try {
      if (this.depth > MAX_DEPTH) throw new Raw();
      return this.parseScript(i, closer);
    } finally {
      this.depth -= 1;
    }
  }

  parseScript(i, closer) {
    const s = this.s;
    const sc = { pipelines: [], comments: [] };
    let pending = [];
    let pipeline = [];
    let cur = null;
    let pipeOpen = false;
    const endPipeline = () => {
      if (cur !== null) pipeline.push(cur);
      cur = null;
      if (pipeline.length) sc.pipelines.push(pipeline);
      pipeline = [];
    };
    for (;;) {
      this.tick(i);
      if (i >= this.n) {
        if (closer || pending.length || pipeOpen) throw new Raw();
        endPipeline();
        return [sc, i];
      }
      const c = s[i];
      if (c === ' ' || c === '\t') { i += 1; continue; }
      if (this.at(i, '\\\n')) { i += 2; continue; }
      if (c === '\n') {
        if (this.pendingTotal > pending.length) throw new Raw();
        i = this.heredocs(i + 1, pending);
        pending = [];
        if (!pipeOpen) endPipeline();
        continue;
      }
      if (c === '#') {
        let j = this.find('\n', i);
        if (j < 0) j = this.n;
        sc.comments.push([i, j]);
        i = j;
        continue;
      }
      if (c === ';' || (c === '&' && !this.at(i, '&>')) || this.at(i, '||')) {
        if (this.at(i, ';;') || this.at(i, ';&') || pipeOpen) throw new Raw();
        endPipeline();
        i += this.at(i, '&&') || this.at(i, '||') ? 2 : 1;
        continue;
      }
      if (c === '|') {
        if (cur === null) throw new Raw();
        pipeline.push(cur);
        cur = null;
        pipeOpen = true;
        i += this.at(i, '|&') ? 2 : 1;
        continue;
      }
      if (c === '(') {
        if (cur !== null) throw new Raw();
        const [sub, j] = this.script(i + 1, ')');
        pipeline.push({ subshell: sub });
        pipeOpen = false;
        i = this.afterCompound(j + 1);
        continue;
      }
      if (c === ')') {
        if (closer !== ')' || pending.length || pipeOpen) throw new Raw();
        endPipeline();
        return [sc, i];
      }
      if (cur === null) cur = { words: [], redirs: [], heredocs: [] };
      pipeOpen = false;
      if ((c === '<' || c === '>') && this.at(i + 1, '(')) {
        const [w, j] = this.word(i);
        cur.words.push(w);
        i = j;
        continue;
      }
      REDIR.lastIndex = i;
      const m = REDIR.exec(s);
      if (m && REDIR.lastIndex <= this.n) {
        const fd = m[1] ? Number(m[1]) : null;
        const op = m[2];
        const [w, j] = this.word(this.blanks(REDIR.lastIndex));
        i = j;
        if (op === '<<' || op === '<<-') {
          if (w.subs.length || w.text.includes('$') || w.text.includes('`')) throw new Raw();
          const delim = w.text.replace(/['"\\]/g, '');
          if (!delim) throw new Raw();
          const hd = { delim, dash: op === '<<-', quoted: /['"\\]/.test(w.text), body: null };
          pending.push(hd);
          this.pendingTotal += 1;
          cur.heredocs.push(hd);
        } else {
          cur.redirs.push({ fd, op, target: w });
        }
        continue;
      }
      const [w, j] = this.word(i);
      i = j;
      const closes = CLOSERS.has(w.text) && cur.words.every((x) => RESERVED.has(x.text));
      cur.words.push(w);
      if (closes) i = this.afterCompound(i);
    }
  }

  heredocs(i, pending) {
    const s = this.s;
    for (const hd of pending) {
      const start = i;
      for (;;) {
        if (i >= this.n) throw new Raw();
        const j = this.find('\n', i);
        const end = j < 0 ? this.n : j;
        const line = s.slice(i, end);
        if ((hd.dash ? line.replace(/^\t+/, '') : line) === hd.delim) {
          hd.body = [start, i];
          i = j < 0 ? this.n : j + 1;
          break;
        }
        if (j < 0) throw new Raw();
        i = j + 1;
      }
    }
    this.pendingTotal -= pending.length;
    return i;
  }

  word(i) {
    const s = this.s;
    const start = i;
    const subs = [];
    while (i < this.n) {
      this.tick(i);
      const c = s[i];
      if ((c === '<' || c === '>') && i === start && this.at(i + 1, '(')) {
        const [sub, j] = this.script(i + 2, ')');
        subs.push(['proc', i + 2, j, sub]);
        i = j + 1;
        continue;
      }
      if (' \t\n;&|<>)'.includes(c)) break;
      if (c === '(') throw new Raw();
      if (c === '\\') {
        if (i + 1 >= this.n) throw new Raw();
        i += 2;
      } else if (c === "'") {
        const j = this.find("'", i + 1);
        if (j < 0) throw new Raw();
        i = j + 1;
      } else if (c === '"') {
        i = this.dquote(i + 1, subs);
      } else if (c === '`') {
        i = this.backtick(i, subs);
      } else if (c === '$' && this.at(i + 1, "'")) {
        i = this.ansi(i + 2);
      } else if (c === '$' && this.at(i + 1, '"')) {
        i = this.dquote(i + 2, subs);
      } else if (c === '$' && this.at(i + 1, '(')) {
        i = this.cmdsub(i, subs);
      } else if (c === '$' && this.at(i + 1, '{')) {
        i = this.brace(i + 2);
      } else {
        i += 1;
      }
    }
    if (i === start) throw new Raw();
    return [{ start, end: i, text: s.slice(start, i), subs }, i];
  }

  dquote(i, subs) {
    const s = this.s;
    while (i < this.n) {
      this.tick(i);
      const c = s[i];
      if (c === '\\') i += 2;
      else if (c === '"') return i + 1;
      else if (c === '`') i = this.backtick(i, subs);
      else if (c === '$' && this.at(i + 1, '(')) i = this.cmdsub(i, subs);
      else if (c === '$' && this.at(i + 1, '{')) i = this.brace(i + 2);
      else i += 1;
    }
    throw new Raw();
  }

  ansi(i) {
    while (i < this.n) {
      this.tick(i);
      const c = this.s[i];
      if (c === '\\') i += 2;
      else if (c === "'") return i + 1;
      else i += 1;
    }
    throw new Raw();
  }

  brace(i) {
    const j = this.find('}', i);
    if (j < 0 || /['"`(){\\]/.test(this.s.slice(i, j))) throw new Raw();
    return j + 1;
  }

  cmdsub(i, subs) {
    if (this.at(i, '$((')) throw new Raw();
    const [sub, j] = this.script(i + 2, ')');
    subs.push(['cmd', i + 2, j, sub]);
    return j + 1;
  }

  backtick(i, subs) {
    const j = this.find('`', i + 1);
    if (j < 0 || this.s.slice(i + 1, j).includes('\\')) throw new Raw();
    const limit = this.n;
    this.n = j;
    let sub;
    try {
      [sub] = this.script(i + 1, null);
    } finally {
      this.n = limit;
    }
    subs.push(['cmd', i + 1, j, sub]);
    return j + 1;
  }
}

function effective(st) {
  const ws = st.words;
  let k = 0;
  while (k < ws.length) {
    const t = ws[k].text;
    if (ASSIGN.test(t) || RESERVED.has(t)) k += 1;
    else if (WRAPPERS.has(t) && k + 1 < ws.length && !ws[k + 1].text.startsWith('-')) k += 1;
    else break;
  }
  if (k >= ws.length) return [k, null];
  const w = ws[k];
  return [k, LITERAL.test(w.text) && !w.subs.length ? w.text : null];
}

function stdoutOk(st) {
  for (const r of st.redirs) {
    const { op } = r;
    const t = r.target.text;
    if (op === '<' || op === '<<<' || (op === '<&' && (r.fd === null || r.fd === 0))) continue;
    if ((op === '>&' || op === '<&') && (t === '1' || t === '2')) continue;
    if (SAFE_TARGETS.has(t) && !r.target.subs.length) continue;
    return false;
  }
  return true;
}

function bodyReader(st, k, name) {
  const args = st.words.slice(k + 1);
  let flags;
  let rest;
  if (name === 'git' && args.length && GIT_MSG_OBJ.has(args[0].text)) {
    flags = GIT_FILE_FLAGS;
    rest = args.slice(1);
  } else if (name === 'gh' && args.length >= 2 && GH_OBJ.has(args[0].text) && GH_VERBS.has(args[1].text)) {
    flags = GH_FILE_FLAGS;
    rest = args.slice(2);
  } else {
    return false;
  }
  for (let p = 0; p < rest.length; p++) {
    const t = rest[p].text;
    if (flags.has(t) && p + 1 < rest.length && rest[p + 1].text === '-') return true;
    if ([...flags].some((f) => f.startsWith('--') && t === `${f}=-`)) return true;
  }
  return false;
}

/**
 * For each stage: can its output only reach text filters and body readers?
 * Computed once per pipeline, from the last stage back, so a long pipeline
 * costs linear time.
 */
function downstreamOk(pipeline) {
  const ok = new Array(pipeline.length).fill(true);
  let after = true;
  for (let idx = pipeline.length - 1; idx >= 0; idx--) {
    ok[idx] = after;
    const st = pipeline[idx];
    if (st.subshell || !stdoutOk(st)) {
      after = false;
    } else {
      const [k, name] = effective(st);
      after = after && (SINKS.has(name) || bodyReader(st, k, name));
    }
  }
  return ok;
}

function flagValues(args, [flags, attached]) {
  const inert = new Set();
  args.forEach((w, p) => {
    const t = w.text;
    if (flags.has(t)) {
      if (p + 1 < args.length) inert.add(args[p + 1]);
    } else if ([...flags].some((f) => f.startsWith('--') && t.startsWith(`${f}=`))) {
      inert.add(w);
    } else if (attached.some((a) => t.startsWith(a) && t.length > a.length)) {
      inert.add(w);
    }
  });
  return inert;
}

function blank(out, s, a, b) {
  for (let p = a; p < b; p++) if (s[p] !== '\n') out[p] = ' ';
}

function walk(sc, ok, s, out, sample = () => {}) {
  if (ok) for (const [a, b] of sc.comments) blank(out, s, a, b);
  for (const pipeline of sc.pipelines) {
    sample();
    const downstream = downstreamOk(pipeline);
    pipeline.forEach((st, idx) => {
      if (st.subshell) walk(st.subshell, ok, s, out, sample);
      else walkSimple(st, downstream[idx], ok, s, out, sample);
    });
  }
}

function walkSimple(st, downstream, ok, s, out, sample = () => {}) {
  sample();
  const [k, name] = effective(st);
  if (DEFINERS.has(name) || (name === 'exec' && k === st.words.length - 1)) throw new Raw();
  if (st.words.some((w) => EXECUTORS.has(w.text.split('/').pop()))) throw new Raw();
  if (k < st.words.length && (name === null || name.includes('/') || name === '.')) throw new Raw();
  const ctx = ok && name !== null && stdoutOk(st) && downstream;
  let inert = new Set();
  if (ctx) {
    const args = st.words.slice(k + 1);
    if (name === 'echo' || (name === 'printf' && !args.some((a) => a.text.startsWith('-v')))) {
      inert = new Set(args);
    } else if (name === 'git' && args.length && GIT_MSG_OBJ.has(args[0].text)) {
      inert = flagValues(args.slice(1), GIT_FLAGS);
    } else if (name === 'gh' && args.length >= 2 && GH_OBJ.has(args[0].text) && GH_VERBS.has(args[1].text)) {
      inert = flagValues(args.slice(2), GH_FLAGS);
    }
  }
  for (const w of st.words) {
    if (inert.has(w)) {
      let pos = w.start;
      for (const [kind, a, b, sub] of w.subs) {
        blank(out, s, pos, a);
        pos = b;
        walk(sub, kind === 'cmd', s, out, sample);
      }
      blank(out, s, pos, w.end);
    } else {
      for (const [, , , sub] of w.subs) walk(sub, false, s, out, sample);
    }
  }
  for (const r of st.redirs) for (const [, , , sub] of r.target.subs) walk(sub, false, s, out, sample);
  const reads = ctx && st.heredocs.length > 0 && (SINKS.has(name) || bodyReader(st, k, name));
  for (const hd of st.heredocs) {
    const [a, b] = hd.body;
    const body = s.slice(a, b);
    const expands = body.includes('$(') || body.includes('`');
    if (reads && (hd.quoted || !expands)) blank(out, s, a, b);
  }
}

/**
 * maskInert — the command with every provably non-executing region replaced
 * by spaces (newlines kept), line continuations joined and NUL dropped (as
 * the Claude Code hook's text reaches grep). Anything the parser cannot
 * classify comes back unmasked.
 *
 * This pass is often the largest single piece of work here, so it carries the
 * budget too, sampled every BUDGET_STEP characters of the parse and nodes of
 * the walk.
 *
 * The catch below turns anything the parser refuses into "scan it all", so a
 * spent budget has to be let through it rather than read as an unclassifiable
 * command.
 */
export function maskInert(command, overBudget = () => {}) {
  let out = command;
  if (!CASE.test(command)) {
    let nodes = 0;
    const sampleWalk = () => {
      if ((nodes++ & (BUDGET_STEP - 1)) === 0) overBudget();
    };
    try {
      const [sc] = new Parser(command, overBudget).script(0, null);
      const chars = command.split('');
      walk(sc, true, command, chars, sampleWalk);
      out = chars.join('');
    } catch (err) {
      if (err instanceof ScanBudgetError) throw err;
      out = command;
    }
  }
  return out.split('\\\n').join('  ').split('\0').join('');
}

/**
 * blockReason — the label of the first pattern the command really invokes, or null.
 *
 * `overBudget` throws once the scan budget is spent. It is sampled here and
 * inside the masking pass, at the shared cadence `startScanBudget` explains:
 * a command can hold hundreds of thousands of lines, and reading the clock on
 * each one costs more than the sampling loses.
 */
export function blockReason(command, overBudget = () => {}) {
  overBudget();
  const lines = maskInert(command, overBudget).split('\n');
  for (const { test, label } of BLOCK_PATTERNS) {
    for (let i = 0; i < lines.length; i++) {
      if ((i & 1023) === 0) overBudget();
      if (test(lines[i])) return label;
    }
  }
  return null;
}

export async function dangerousCmdGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'bash') return;
      const overBudget = startScanBudget('dangerous-cmd-guard');
      overBudget();

      const command = output.args?.command ?? '';
      if (!command) return;

      const reason = blockReason(String(command), overBudget);
      if (reason) throw new Error(`[devexp dangerous-cmd-guard] Blocked: ${reason}.`);
    },
  };
}
