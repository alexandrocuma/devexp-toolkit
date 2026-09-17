#!/usr/bin/env bash
# Tests for secret-in-write-guard.sh — exit 2 = blocked, exit 0 = allowed.
# Run: bash hooks/claude-code/secret-in-write-guard.test.sh
#
# Every secret below is FAKE: a vendor prefix and a dummy body containing
# "FAKE", joined at runtime. This file never holds a secret-shaped string
# itself, so the installed guard lets it be written and secret scanners
# have nothing to flag.
set -uo pipefail

HOOK="$(cd "$(dirname "$0")" && pwd)/secret-in-write-guard.sh"
ERR="$(mktemp)"
trap 'rm -f "$ERR"' EXIT
pass=0; fail=0

# Feed the hook the real PreToolUse JSON envelope; sets rc and err.
# The payload travels on stdin, not argv: Linux refuses one argument over 128 KB.
run() { # $1=Write|Edit  $2=payload  $3=old_string (Edit)
  printf '%s' "$2" | python3 -c '
import json, sys
tool, path, old = sys.argv[1], sys.argv[2], sys.argv[3]
payload = sys.stdin.read()
if tool == "Write":
    ti = {"file_path": path, "content": payload}
else:
    ti = {"file_path": path, "old_string": old, "new_string": payload}
print(json.dumps({"tool_name": tool, "tool_input": ti}))' "$1" "${FILE:-src/config.ts}" "${3:-TODO}" \
    | bash "$HOOK" >/dev/null 2>"$ERR"
  rc=$?; err=$(cat "$ERR")
}

show() { local s="${1:0:80}"; printf '%s' "${s//$'\n'/ }"; }

block() { # $1=Write|Edit  $2=word the block message must name  $3=payload
  run "$1" "$3"
  if [ "$rc" != 2 ]; then
    fail=$((fail+1)); printf 'FAIL [want block, rc %s] %s %s: %s\n' "$rc" "$1" "${FILE:-src/config.ts}" "$(show "$3")"
  elif [[ "$err" != *"[devexp secret-in-write-guard] Blocked: content appears to contain"*"$2"* ]]; then
    fail=$((fail+1)); printf 'FAIL [message does not name %s] %s: %s\n' "$2" "$1" "$err"
  elif [[ "$err" == *FAKE* ]]; then
    fail=$((fail+1)); printf 'FAIL [message echoes the secret] %s: %s\n' "$1" "$err"
  else
    pass=$((pass+1))
  fi
}

allow() { # $1=Write|Edit  $2=payload  $3=old_string (Edit)
  run "$1" "$2" "${3:-}"
  if [ "$rc" = 0 ] && [ -z "$err" ]; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL [want quiet allow, rc %s] %s %s: %s\n  stderr: %s\n' \
      "$rc" "$1" "${FILE:-src/config.ts}" "$(show "$2")" "$(show "$err")"
  fi
}

rep() { python3 -c 'import sys; print(sys.argv[1] * int(sys.argv[2]), end="")' "$1" "$2"; }

# ── Fake secrets, one per shape the guard claims to detect ──────────────────
SK=sk; AK=AKIA; GH=gh; XOX=xox; D5=-----
ANTHROPIC="${SK}-ant-api03-$(rep FAKE_body- 9)AA"
OPENAI="${SK}-$(rep 0FAKE 10)"
AWS="${AK}$(rep FAKE 4)"
GH_P="${GH}p_$(rep 0FAKE 8)"
GH_O="${GH}o_$(rep 0FAKE 8)"
GH_S="${GH}s_$(rep 0FAKE 8)"
SLACK_B="${XOX}b-1234567890-1234567890123-$(rep FAKE 6)"
SLACK_P="${XOX}p-1234567890-1234567890-1234567890123-$(rep 0fake 6)"
pem() { printf '%sBEGIN %s%s\nMIIEFAKEFAKEFAKE\n%sEND %s%s\n' "$D5" "$1" "$D5" "$D5" "$1" "$D5"; }
PK_RSA="$(pem 'RSA PRIVATE KEY')"
PK_OPENSSH="$(pem 'OPENSSH PRIVATE KEY')"
PK_EC="$(pem 'EC PRIVATE KEY')"
PK_PKCS8="$(pem 'PRIVATE KEY')"
PK_ENCRYPTED="$(pem 'ENCRYPTED PRIVATE KEY')"
PK_PGP="$(pem 'PGP PRIVATE KEY BLOCK')"
FILLER="$(rep $'an ordinary line of prose in a large generated file\n' 5000)"  # ~260 KB

# ── must BLOCK: each pattern, via Write content and Edit new_string ─────────
for tool in Write Edit; do
  block "$tool" Anthropic   "$ANTHROPIC"
  block "$tool" OpenAI      "$OPENAI"
  block "$tool" AWS         "$AWS"
  block "$tool" GitHub      "$GH_P"
  block "$tool" GitHub      "$GH_O"
  block "$tool" GitHub      "$GH_S"
  block "$tool" Slack       "$SLACK_B"
  block "$tool" Slack       "$SLACK_P"
  block "$tool" 'private key' "$PK_RSA"
  block "$tool" 'private key' "$PK_OPENSSH"
  block "$tool" 'private key' "$PK_EC"
  block "$tool" 'private key' "$PK_PKCS8"
  block "$tool" 'private key' "$PK_ENCRYPTED"
  block "$tool" 'private key' "$PK_PGP"
done

# ── must BLOCK: wherever the secret sits ────────────────────────────────────
block Write Anthropic "const client = new Anthropic({ apiKey: \"$ANTHROPIC\" });"
block Write OpenAI    $'line one\nline two\nOPENAI_API_KEY='"$OPENAI"$'\nline four'
block Edit  AWS       "aws_access_key_id = $AWS"
# A template is exempt only for what it holds, not for its name: a real value
# in a committed .env.example is the likeliest way a secret reaches git.
FILE=.env.example block Write GitHub "GITHUB_TOKEN=$GH_P"
# Larger than the pipe buffer, secret first — once allowed silently (#101).
block Write OpenAI        "$OPENAI"$'\n'"$FILLER"
block Edit  'private key' "$PK_RSA$FILLER"

# ── must ALLOW: prose that mentions tokens is not a token ───────────────────
allow Write 'Set your API key and bearer token in the environment. Never commit a secret, a password or a private key.'
allow Write 'Anthropic keys start with sk-ant-, OpenAI keys with sk-, AWS key IDs with AKIA, GitHub tokens with ghp_ and Slack bot tokens with xoxb-.'
allow Edit  'Rotate the token if it leaks; see docs/security.md.'

# ── must ALLOW: a variable named token holds no value ───────────────────────
allow Write 'const token = process.env.GITHUB_TOKEN;'
allow Write 'api_key = os.environ["OPENAI_API_KEY"]'
allow Edit  'token: ${{ secrets.GITHUB_TOKEN }}'

# ── must ALLOW: committed templates document keys with placeholders ─────────
TEMPLATE='# Copy to .env and fill in real values.
ANTHROPIC_API_KEY=
OPENAI_API_KEY=sk-...
GITHUB_TOKEN=<your-github-token>
AWS_ACCESS_KEY_ID=your-access-key-id-here
SLACK_BOT_TOKEN=xoxb-...'
FILE=.env.example  allow Write "$TEMPLATE"
FILE=.env.sample   allow Write "$TEMPLATE"
FILE=.env.template allow Write "$TEMPLATE"
FILE=.env.dist     allow Write "$TEMPLATE"
FILE=.env.example  allow Edit  'OPENAI_API_KEY=<your-openai-key>' 'OPENAI_API_KEY='

# ── must ALLOW: public material and look-alike words ────────────────────────
allow Write "$(pem 'PUBLIC KEY')"
allow Write "$(pem 'CERTIFICATE')"
allow Write 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFAKE user@host'
allow Write 'import sklearn  # a.k.a. sk-learn; see task-runner and risk-score'

# ── must ALLOW: removing a secret, and empty writes ─────────────────────────
# Only the new text is scanned; an Edit that takes a key out must not be refused.
allow Edit 'OPENAI_API_KEY=process.env.OPENAI_API_KEY' "OPENAI_API_KEY=$OPENAI"
allow Edit ''
allow Write ''
allow Write "$FILLER"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
