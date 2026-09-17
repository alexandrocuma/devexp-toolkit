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
 * Tests: node hooks/opencode/dangerous-cmd-guard.test.js
 */

// A target ends at whitespace, end of line, or a character that closes the word in shell
// syntax: ' " ) ` ; & | — so `sh -c 'rm -rf /'`, `$(rm -rf ~)` and `git push --force;` match.
// A letter, digit, '/', '.', '-' or '*' continues it.
export const BLOCK_PATTERNS = [
  {
    re: /rm\s+-[a-z]*r[a-z]*f\s+["']?(\/(\s|$|['"`);&|])|~\/?(\s|$|['"`);&|])|\$HOME(\s|$|['"`);&|]))/m,
    label: "'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory",
  },
  {
    re: /rm\s+-[a-z]*f[a-z]*r\s+["']?(\/(\s|$|['"`);&|])|~\/?(\s|$|['"`);&|])|\$HOME(\s|$|['"`);&|]))/m,
    label: "'rm -rf /' or 'rm -rf ~' would wipe your filesystem or home directory",
  },
  {
    // Unanchored wildcard delete in a sensitive dir (/tmp/* , ~/.claude/.../* , or the dir
    // wholesale) — the blanket wipe an empty variable produces. Prefix-anchored globs like
    // /tmp/.deliver-PAY-123-* are allowed (no '*' right after the '/').
    re: /rm\b[^|]*(\s["']?\/tmp["']?(\/\*|\/?(\s|$|['"`);&|]))|["']?(\$HOME|~)["']?\/\.claude(\S*\/\*|["']?\/?(\s|$|['"`);&|]))|\.claude\S*\/\*)/m,
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
    re: /git\s+push\b[^|&;]*\s(--force-with-lease|--force|-f)(\s|=|$|['"`);&|])/m,
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

class Parser {
  constructor(s) {
    this.s = s;
    this.n = s.length;
    this.pendingTotal = 0;
  }

  at(i, tok) {
    return i + tok.length <= this.n && this.s.startsWith(tok, i);
  }

  find(ch, i) {
    const j = this.s.indexOf(ch, i);
    return j >= this.n ? -1 : j;
  }

  blanks(i) {
    while (i < this.n) {
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

function downstreamOk(pipeline, idx) {
  for (const st of pipeline.slice(idx + 1)) {
    if (st.subshell || !stdoutOk(st)) return false;
    const [k, name] = effective(st);
    if (!SINKS.has(name) && !bodyReader(st, k, name)) return false;
  }
  return true;
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

function walk(sc, ok, s, out) {
  if (ok) for (const [a, b] of sc.comments) blank(out, s, a, b);
  for (const pipeline of sc.pipelines) {
    pipeline.forEach((st, idx) => {
      if (st.subshell) walk(st.subshell, ok, s, out);
      else walkSimple(st, pipeline, idx, ok, s, out);
    });
  }
}

function walkSimple(st, pipeline, idx, ok, s, out) {
  const [k, name] = effective(st);
  if (DEFINERS.has(name) || (name === 'exec' && k === st.words.length - 1)) throw new Raw();
  if (st.words.some((w) => EXECUTORS.has(w.text.split('/').pop()))) throw new Raw();
  if (k < st.words.length && (name === null || name.includes('/') || name === '.')) throw new Raw();
  const ctx = ok && name !== null && stdoutOk(st) && downstreamOk(pipeline, idx);
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
        walk(sub, kind === 'cmd', s, out);
      }
      blank(out, s, pos, w.end);
    } else {
      for (const [, , , sub] of w.subs) walk(sub, false, s, out);
    }
  }
  for (const r of st.redirs) for (const [, , , sub] of r.target.subs) walk(sub, false, s, out);
  for (const hd of st.heredocs) {
    const [a, b] = hd.body;
    const body = s.slice(a, b);
    const expands = body.includes('$(') || body.includes('`');
    if (ctx && (SINKS.has(name) || bodyReader(st, k, name)) && (hd.quoted || !expands)) blank(out, s, a, b);
  }
}

/**
 * maskInert — the command with every provably non-executing region replaced
 * by spaces (newlines kept), and line continuations joined. Anything the
 * parser cannot classify comes back unmasked.
 */
export function maskInert(command) {
  let out = command;
  if (!CASE.test(command)) {
    try {
      const [sc] = new Parser(command).script(0, null);
      const chars = command.split('');
      walk(sc, true, command, chars);
      out = chars.join('');
    } catch {
      out = command;
    }
  }
  return out.split('\\\n').join('  ');
}

/** blockReason — the label of the first pattern the command really invokes, or null. */
export function blockReason(command) {
  const text = maskInert(command);
  for (const { re, label } of BLOCK_PATTERNS) if (re.test(text)) return label;
  return null;
}

export async function dangerousCmdGuard(_ctx) {
  return {
    'tool.execute.before': async (input, output) => {
      if (input.tool !== 'bash') return;

      const command = output.args?.command ?? '';
      if (!command) return;

      const reason = blockReason(String(command));
      if (reason) throw new Error(`[devexp dangerous-cmd-guard] Blocked: ${reason}.`);
    },
  };
}
