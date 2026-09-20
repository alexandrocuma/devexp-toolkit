/**
 * comment-refs-on-save.js — reports comments that cite something outside the file
 *
 * Event: file.edited
 * Advisory only — never throws, never edits.
 *
 * The rule: a comment is self-contained. A reader must not have to open a
 * tracker to understand the code in front of them, so where a reference
 * carries the reason, the reason is written inline and the history stays in
 * the commit body.
 *
 * Ships DISABLED. Unlike the guards beside it this enforces a house style, not
 * a safety property, and plenty of projects deliberately want an issue number
 * in a comment.
 *
 * Mirror of hooks/claude-code/comment-refs.sh — the syntax table, the findings
 * and the whole-line-comment rule must stay in lockstep with it. The shell
 * side is what the repo's lint job runs, so a disagreement between the two
 * shows up as an editor that is quiet about something CI then blocks on.
 */

import { readFileSync } from 'fs';
import { extname, editedPath } from './utils.js';

// ext -> { line, block }. A language with no entry is skipped rather than
// guessed at: a wrong marker reports findings in code, which is worse than
// reporting none. Only the C family gets block comments — the rest are not
// common enough to be worth the state a multi-line scan costs.
const LINE_FAMILIES = [
  ['//', '.go .js .jsx .ts .tsx .mjs .cjs .java .c .h .cc .cpp .hh .hpp .cs ' +
         '.rs .swift .kt .kts .scala .php .dart .zig .m .mm .proto .gradle'],
  ['#',  '.sh .bash .zsh .fish .py .rb .pl .pm .yaml .yml .toml .tf .hcl ' +
         '.nix .ex .exs .cr .jl .mk .cmake .dockerfile .gitignore'],
  ['--', '.sql .hs .lua .elm .adb .ads'],
  [';',  '.el .lisp .clj .cljs .cljc .scm .ini .asm'],
  ['%',  '.erl .tex .mat'],
];
const BLOCK = { '//': ['/*', '*/'] };

const SYNTAX = new Map();
for (const [marker, exts] of LINE_FAMILIES) {
  for (const ext of exts.split(/\s+/)) SYNTAX.set(ext, { line: marker, block: BLOCK[marker] });
}

// "..." and `...` only. Prose says devexp's, and treating that apostrophe as a
// quote would swallow the rest of the line — including a reference in it.
const QUOTED = /"[^"\n]*"|`[^`\n]*`/g;

const FINDINGS = [
  ['an issue number', /(?<![\w#])#\d{2,}\b/],
  ['a URL', /\bhttps?:\/\//],
  // Two or more digits, so SHA-1, UTF-8 and base-64 stay out of it; a prefix
  // that names a standard rather than a tracker is excluded by name.
  ['a tracker id',
   /\b(?!(?:SHA|UTF|RFC|ISO|UTC|AES|RSA|IEEE|ANSI|ASCII|CVE|GMT|EC|DSA)-)[A-Z][A-Z0-9]{1,9}-\d{2,}\b/],
];

/** commentLines yields every whole-line comment in source, as [lineNo, text]. */
export function commentLines(source, ext) {
  const syntax = SYNTAX.get(ext);
  if (!syntax) return [];
  const { line: marker, block } = syntax;
  const out = [];
  let inBlock = false;

  source.split('\n').forEach((text, i) => {
    const lineNo = i + 1;
    const stripped = text.replace(/^\s+/, '');
    if (inBlock) {
      out.push([lineNo, text]);
      if (block && text.includes(block[1])) inBlock = false;
      return;
    }
    if (block && stripped.startsWith(block[0])) {
      out.push([lineNo, text]);
      if (!stripped.slice(block[0].length).includes(block[1])) inBlock = true;
      return;
    }
    // A shebang is not a comment about the code; it is the code's interpreter.
    if (stripped.startsWith(marker) && !stripped.startsWith('#!')) out.push([lineNo, text]);
  });
  return out;
}

/** scanSource returns one finding per offending comment line. */
export function scanSource(source, ext) {
  const out = [];
  for (const [lineNo, text] of commentLines(source, ext)) {
    const bare = text.replace(QUOTED, ' ');
    for (const [label, pattern] of FINDINGS) {
      const m = bare.match(pattern);
      if (m) { out.push({ lineNo, label, token: m[0], text: text.trim() }); break; }
    }
  }
  return out;
}

export async function commentRefsOnSave(ctx) {
  return {
    'file.edited': async (event) => {
      const filePath = editedPath(event.file ?? '', ctx?.directory);
      if (!filePath) return;
      const ext = extname(filePath).toLowerCase();
      if (!SYNTAX.has(ext)) return;

      let source;
      // Advisory: an unreadable file is not this hook's problem to report.
      try { source = readFileSync(filePath, 'utf8'); } catch { return; }

      const findings = scanSource(source, ext);
      if (findings.length === 0) return;

      console.error('[devexp comment-refs-on-save] a comment here points outside the file:');
      for (const f of findings) {
        console.error(`  ${filePath}:${f.lineNo}: ${f.label} in a comment (${f.token}) -- ${f.text}`);
      }
      console.error('  Write the reason inline; history belongs in the commit body.');
    },
  };
}
