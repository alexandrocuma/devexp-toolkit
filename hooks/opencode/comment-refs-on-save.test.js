/**
 * Tests the opencode twin of the comment-reference scanner.
 * Run: node hooks/opencode/comment-refs-on-save.test.js
 *
 * The shell side is what CI blocks on, so the two have to agree about what a
 * violation is. Every case here is also a case in
 * hooks/claude-code/comment-refs.test.sh; a disagreement shows up as an editor
 * that stays quiet about something CI then rejects.
 *
 * The cases split three ways: the same reference found across languages that
 * share nothing but a table row, the things deliberately not reported (a
 * trailing comment, a shebang, a quoted example, an unknown extension), and
 * the hook's own contract — stderr only, never a throw.
 */
import { commentRefsOnSave, scanSource } from './comment-refs-on-save.js';

let pass = 0;
let fail = 0;
function check(name, ok, detail = '') {
  if (ok) { pass++; return; }
  fail++;
  console.log(`FAIL ${name}${detail ? ` — ${detail}` : ''}`);
}

function finds(name, ext, source, token) {
  const out = scanSource(source, ext);
  check(name, out.length === 1 && out[0].token === token,
    `got ${JSON.stringify(out.map((f) => f.token))}`);
}
function quiet(name, ext, source) {
  const out = scanSource(source, ext);
  check(name, out.length === 0, `got ${JSON.stringify(out.map((f) => f.token))}`);
}

// ── the same reference, found in every language in the table ────────────────
finds('go: issue number', '.go', '// works around the bug (#412) that ate a byte', '#412');
finds('shell: issue number', '.sh', '# works around the bug (#412) that ate a byte', '#412');
finds('js: issue number', '.js', '// works around the bug (#412) that ate a byte', '#412');
finds('sql: issue number', '.sql', '-- works around the bug (#412) that ate a byte', '#412');
finds('lisp: issue number', '.el', '; works around the bug (#412) that ate a byte', '#412');
finds('python: URL', '.py', '# the contract is at https://example.com/docs', 'https://');
finds('rust: tracker id', '.rs', '// see PROJ-4421 for the original report', 'PROJ-4421');

// ── block comments, which is where a JSDoc @see hides ───────────────────────
finds('js: inside a block comment', '.js', '/**\n * @see https://example.com/docs\n */', 'https://');
quiet('js: a closed block leaves the next line alone', '.js',
  '/* nothing here */\nconst url = "https://example.com";');

// ── what it must NOT report ─────────────────────────────────────────────────
quiet('a shebang is not a comment', '.sh', '#!/usr/bin/env bash');
quiet('a quoted example is data', '.go',
  '// Hostname(), not Host: for "http://user@:80" the host is ":80"');
quiet('a backtick example is data', '.sh',
  '# the placeholder is `https://host/x` until resolveStr runs');
quiet('a trailing comment is out of scope', '.go', 'x := 1 // see #412');
quiet('a URL in code is not a comment', '.go', 'const docs = "https://example.com"');
quiet('an unknown extension is skipped', '.zzz', '// see #412');
quiet('a short number is not an issue', '.go', '// the retry budget is #9 by design');
quiet('SHA-256 is not a tracker id', '.go', '// the digest is SHA-256 over the body');
quiet('UTF-8 is not a tracker id', '.go', '// the body must be valid UTF-8 to survive');
quiet('an ordinary comment', '.go', '// resolveStr substitutes ${VAR} from the merged env');

// ── the hook: stderr only, never a throw, never a non-zero exit ─────────────
const { mkdtempSync, writeFileSync, rmSync } = await import('fs');
const { tmpdir } = await import('os');
const { join } = await import('path');

const TMP = mkdtempSync(join(tmpdir(), 'devexp-comment-refs-'));
const dirty = join(TMP, 'dirty.go');
const clean = join(TMP, 'clean.go');
writeFileSync(dirty, '// works around the bug (#412) that ate a byte\n');
writeFileSync(clean, '// resolveStr substitutes ${VAR} from the merged env\n');

async function runHook(file) {
  const handlers = await commentRefsOnSave({ directory: TMP });
  const lines = [];
  const realError = console.error;
  console.error = (...args) => lines.push(args.join(' '));
  try {
    await handlers['file.edited']({ file });
  } finally {
    console.error = realError;
  }
  return lines.join('\n');
}

let out = await runHook(dirty);
check('the hook names the finding on stderr', out.includes('#412'), `got: ${out || '<nothing>'}`);

out = await runHook(clean);
check('the hook is silent on a clean file', out === '', `got: ${out}`);

out = await runHook(join(TMP, 'does-not-exist.go'));
check('a missing file is not the hook to report it', out === '', `got: ${out}`);

out = await runHook(join(TMP, 'notes.zzz'));
check('an unknown extension is skipped', out === '', `got: ${out}`);

rmSync(TMP, { recursive: true, force: true });

console.log(`${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
