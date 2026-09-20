#!/usr/bin/env bash
# devexp: the language-agnostic comment-reference scanner.
#
# Not a hook. It is the one implementation behind comment-refs-on-save (which
# warns on a file as it is written) and the repo's own lint job (which blocks
# on the whole tree), so the two can never disagree about what a violation is.
#
#   comment-refs.sh <file>...   prints one line per finding; exit 1 if any
#
# WHAT IT LOOKS FOR: an external reference inside a comment -- an issue number
# `#1234`, a URL, or a tracker id `ABC-123` — quoted here because that is how
# this scanner tells an example from a citation. The rule it enforces is that a
# comment is self-contained: a reader must not have to leave the file, let
# alone the repo, to understand the code in front of them.
#
# WHY IT IS NOT A LINTER PLUGIN: the rule is about comments, and every language
# has those. Tying it to one language's parser would enforce it in that
# language and nowhere else, which is how a codebase ends up with the rule held
# in its Go and quietly abandoned in its shell and JS. The syntax table below
# is the whole language-specific part; adding a language is one line.
#
# WHAT IT DELIBERATELY MISSES. Only whole-line comments are scanned -- a marker
# that is the first non-whitespace on the line, or any line inside an open
# block comment. A trailing comment after code is skipped, because telling it
# from a `//` inside a string literal needs a real parser per language, and a
# blocking check that cries wolf gets switched off. Under-reporting is the
# safe direction.
#
# Quoted spans inside a comment are stripped before matching, so a comment
# showing `"http://user@:80"` as an example of what a parser does is data, not
# a citation, and is left alone. That is the general form of the exception for
# quoted third-party material.
#
# Tests: bash hooks/claude-code/comment-refs.test.sh

set -euo pipefail

exec python3 -I - "$@" <<'PYREFS'
import os
import re
import sys

# ext -> (line markers, (block open, block close) or None). A language devexp
# has no entry for is skipped rather than guessed at: a wrong comment marker
# reports findings in code, which is worse than reporting none.
_LINE = {
    '//': ('.go .js .jsx .ts .tsx .mjs .cjs .java .c .h .cc .cpp .hh .hpp .cs '
           '.rs .swift .kt .kts .scala .php .dart .zig .m .mm .proto .gradle'),
    '#':  ('.sh .bash .zsh .fish .py .rb .pl .pm .yaml .yml .toml .tf .hcl '
           '.nix .ex .exs .cr .jl .mk .cmake .dockerfile .gitignore'),
    '--': ('.sql .hs .lua .elm .adb .ads'),
    ';':  ('.el .lisp .clj .cljs .cljc .scm .ini .asm'),
    '%':  ('.erl .tex .mat'),
}
# Block comments, by line marker family. Only the C family is common enough to
# be worth the state a multi-line scan costs.
_BLOCK = {'//': ('/*', '*/')}

SYNTAX = {}
for marker, exts in _LINE.items():
    for ext in exts.split():
        SYNTAX[ext] = (marker, _BLOCK.get(marker))

# A shebang is not a comment about the code; it is the code's interpreter.
SHEBANG = re.compile(r'^#!')
# "..." and `...` only. Prose says devexp's, and treating that apostrophe as a
# quote would swallow the rest of the line -- including a reference in it.
QUOTED = re.compile(r'"[^"\n]*"|`[^`\n]*`')

FINDINGS = [
    ('an issue number', re.compile(r'(?<![\w#])#\d{2,}\b')),
    ('a URL', re.compile(r'\bhttps?://')),
    # Two or more digits, so SHA-1, UTF-8, RFC-1 and base-64 stay out of it.
    # A prefix that names a standard rather than a tracker is excluded by name.
    ('a tracker id', re.compile(
        r'\b(?!(?:SHA|UTF|RFC|ISO|UTC|AES|RSA|IEEE|ANSI|ASCII|CVE|GMT|EC|DSA)-)'
        r'[A-Z][A-Z0-9]{1,9}-\d{2,}\b')),
]


def comment_text(path):
    """Yield (lineno, text) for every whole-line comment in path."""
    ext = os.path.splitext(path)[1].lower()
    if ext not in SYNTAX:
        return
    marker, block = SYNTAX[ext]
    try:
        with open(path, 'r', encoding='utf-8', errors='replace') as fh:
            lines = fh.read().split('\n')
    except OSError:
        return

    in_block = False
    for i, line in enumerate(lines, 1):
        stripped = line.lstrip()
        if in_block:
            yield i, line
            if block and block[1] in line:
                in_block = False
            continue
        if block and stripped.startswith(block[0]):
            yield i, line
            # A block that opens and closes on one line is already handled.
            if block[1] not in stripped[len(block[0]):]:
                in_block = True
            continue
        if stripped.startswith(marker) and not SHEBANG.match(stripped):
            yield i, line


def scan(path):
    out = []
    for lineno, text in comment_text(path):
        bare = QUOTED.sub(' ', text)
        for label, pat in FINDINGS:
            m = pat.search(bare)
            if m:
                out.append((lineno, label, m.group(0), text.strip()))
                break
    return out


found = 0
for path in sys.argv[1:]:
    for lineno, label, token, text in scan(path):
        found += 1
        print('%s:%d: %s in a comment (%s) -- %s' % (path, lineno, label, token, text))

if found:
    print('', file=sys.stderr)
    print('%d comment%s cite%s something outside the file. Write the reason '
          'inline instead; history belongs in the commit body.'
          % (found, '' if found == 1 else 's', 's' if found == 1 else ''),
          file=sys.stderr)
    sys.exit(1)
PYREFS
