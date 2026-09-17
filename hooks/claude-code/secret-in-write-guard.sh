#!/usr/bin/env bash
# devexp hook: secret-in-write-guard
# Event: PreToolUse | Matcher: Write|Edit|MultiEdit|NotebookEdit
# Hard-blocks writing content that contains secret or token patterns.
#
# Scans the text being written for high-signal secret patterns: Write
# 'content', Edit 'new_string', MultiEdit 'edits[].new_string' (older Claude
# Code releases) and NotebookEdit 'new_source'. Text being replaced
# ('old_string') is never scanned, so an edit that removes a key is allowed.
# Complements secret-guard which checks filenames.
#
# Known limitation: only the new text of each edit is scanned, never the file
# text around it, so a secret completed across existing file text and an edit
# is not seen. The guard catches secrets written in one piece; it does not
# replace a secret scanner on the repository.
#
# Matching happens inside the Python step, which fails closed. An earlier
# shell pipeline read some of its own failures as "no match", so some kinds
# of secret and large writes went through unchecked (#101).
#
# Tests: bash hooks/claude-code/secret-in-write-guard.test.sh
# Mirror: hooks/opencode/secret-in-write-guard.js — keep the patterns in lockstep.

set -euo pipefail

input=$(cat)

label=$(echo "$input" | python3 -I -c "
import sys, json, re
d = json.load(sys.stdin)
ti = d.get('tool_input', {})
parts = [ti.get('content'), ti.get('new_string'), ti.get('new_source')]
edits = ti.get('edits')
if isinstance(edits, list):
    parts += [e.get('new_string') for e in edits if isinstance(e, dict)]
content = '\n'.join(str(p) for p in parts if p)

# Placeholders (#143). The (?!...) right after a prefix skips a body that is
# only a placeholder: one character repeated (separators aside), a your-...
# phrase made of letter words, or a key ID ending in EXAMPLE, as in AWS's
# documentation. A skipped body must run to where the key's characters end,
# so key material can't hide inside one, and re.search still tries every
# later start, so a real key next to or joined to a placeholder still blocks.
#
# Linear time. re.search retries a pattern at every start, so a repetition
# that can run over the next prefix costs quadratic time or worse on text
# that repeats a prefix, and a hook that runs out of time lets the write
# through. Every repetition that can span another start is bounded: phrase
# words, service-key name segments, GitHub id segments, and the private-key
# header and material search. The timing tests pin this.
PATTERNS = [
    ('an Anthropic API key (sk-ant-...)', r'sk-ant-(?!(?:(?:api|admin)[0-9]+-)?(?:([A-Za-z0-9])(?:\1|[_-])*|(?:your|YOUR)(?:[_-][A-Za-z]+){1,24})(?![A-Za-z0-9_-]))[A-Za-z0-9_-]{40,}'),
    # Legacy user keys (some issued as sk-None-...) are no longer issued, but
    # existing ones can still be live.
    ('an OpenAI API key (sk-...)',        r'sk-(?:None-)?(?!([A-Za-z0-9])\1*(?![A-Za-z0-9]))[A-Za-z0-9]{32,}'),
    # Project, service-account and admin keys: the body has _ and -, like a
    # kebab-case id, so its length carries the signal. Real bodies are well
    # over 100 characters; ids like desk-admin-... are far shorter. No left
    # boundary: a key can follow an escape, a %XX, a _, a - or a digit.
    ('an OpenAI API key (sk-...)',        r'sk-(?:proj|svcacct|admin)-(?!(?:([A-Za-z0-9])(?:\1|[_-])*|(?:your|YOUR)(?:[_-][A-Za-z]+){1,24})(?![A-Za-z0-9_-]))[A-Za-z0-9_-]{80,}'),
    # Older service keys (#152): sk-service-, a service-account name, -, then
    # 48 alphanumerics (20, the T3BlbkFJ watermark, 20), per Trivy's
    # openai-service-api-key rule (aquasecurity/trivy#10798) and TruffleHog's
    # OpenAI detector. The name is optional and the watermark isn't required
    # here, so no real key is missed; the 48-character run keeps kebab-case
    # ids like sk-service-account-... from matching. The name is read one
    # _/- separated segment at a time, so no segment is split two ways.
    ('an OpenAI API key (sk-...)',        r'sk-service-(?!([A-Za-z0-9])(?:\1|[_-])*(?![A-Za-z0-9_-]))(?:[A-Za-z0-9]{0,47}[_-]){0,20}[A-Za-z0-9]{48}'),
    ('an AWS Access Key ID (AKIA...)',    r'AKIA(?![0-9A-Z]{9}EXAMPLE|([0-9A-Z])\1{15})[0-9A-Z]{16}'),
    # Temporary (STS) key IDs are ASIA plus exactly 16 of [0-9A-Z], so a
    # boundary is any character outside that set: EURASIA... and
    # ASIAPACIFICDATACENTER01 don't match, while a key next to a lowercase
    # letter (an escape like \n) does. On the left, an encoded character
    # that ends in an uppercase hex digit (%2F, \u002F, \x2F) also counts;
    # \x5c is a backslash. A key run straight into more capitals or digits
    # reads as a longer word and is not matched.
    ('an AWS temporary Access Key ID (ASIA...)', r'(^|[^0-9A-Z]|%[0-9A-Fa-f]{2}|\x5cu[0-9A-Fa-f]{4}|\x5cx[0-9A-Fa-f]{2})ASIA(?![0-9A-Z]{9}EXAMPLE|([0-9A-Z])\2{15})[0-9A-Z]{16}([^0-9A-Z]|$)'),
    # Classic tokens: 36 or more alphanumerics, never _ (GitHub's token
    # formats docs; gitleaks and Trivy match the same class), so snake_case
    # names that contain a prefix like ght_ don't match.
    ('a GitHub token (ghp_, ghs_, etc.)', r'gh[postaur]_(?!([A-Za-z0-9])\1*(?![A-Za-z0-9]))[A-Za-z0-9]{36,}'),
    # Stateless tokens (installation tokens since 2026-04-27, GITHUB_TOKEN
    # included) are the prefix, an app id and _, then a JWT, and a JWT
    # starts eyJ: github.blog/changelog/2026-05-15-github-app-installation-
    # tokens-per-request-override-header. Any prefix, with a few id segments,
    # since GitHub says other token types may follow.
    ('a GitHub token (ghp_, ghs_, etc.)', r'gh[postaur]_(?:[A-Za-z0-9]+_){0,8}eyJ'),
    # Fine-grained tokens have a fixed shape: 22 alphanumerics, _, 59 more.
    # Anything looser blocks long snake_case names that start github_pat_.
    ('a GitHub token (ghp_, ghs_, etc.)', r'github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}'),
    ('a Slack token (xox...)',            r'xox[baprs]-(?!(?:([0-9A-Za-z])(?:\1|-)*|(?:your|YOUR)(?:-[A-Za-z]+){1,24})(?![0-9A-Za-z-]))[0-9A-Za-z-]{10,}'),
    # A private key block is its header followed by key material: a run of
    # 32 base64 characters (PEM wraps lines at 64, OpenSSH at 70) that starts
    # within a few hundred characters of the header, before any -----END or
    # -----BEGIN line. That window leaves room for encrypted-PEM and PGP armor
    # headers. A header quoted alone, or with an elided or placeholder body,
    # has no material and is allowed (#143); key-like text on the header's
    # line or just after it still blocks. A backslash (\x5c) counts as
    # material, so escaped slashes and newlines in JSON don't split a line.
    ('a private key block',               r'-----BEGIN [A-Z ]{0,40}(PRIVATE|SECRET) KEY(?:(?!-----(?:END|BEGIN))[\s\S]){0,500}?[A-Za-z0-9+/\x5c]{32}'),
]
for name, pattern in PATTERNS:
    if re.search(pattern, content, re.M):
        print(name)
        break
") || {
    echo "[devexp secret-in-write-guard] internal error -- the guard could not read its input, so it did not run. Blocking to be safe; the interpreter's error is above." >&2
    exit 2
}

if [[ -n "$label" ]]; then
    echo "[devexp secret-in-write-guard] Blocked: content appears to contain $label. Remove the secret before writing." >&2
    exit 2
fi

exit 0
