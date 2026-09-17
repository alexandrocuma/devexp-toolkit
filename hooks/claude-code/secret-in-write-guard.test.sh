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
# The payload is the text the tool writes: Write content, Edit new_string, the
# second of two MultiEdit edits, or NotebookEdit new_source.
run() { # $1=Write|Edit|MultiEdit|NotebookEdit  $2=payload  $3=old_string (Edit, MultiEdit)
  printf '%s' "$2" | python3 -c '
import json, sys
tool, path, old = sys.argv[1], sys.argv[2], sys.argv[3]
payload = sys.stdin.read()
if tool == "Write":
    ti = {"file_path": path, "content": payload}
elif tool == "MultiEdit":
    ti = {"file_path": path, "edits": [
        {"old_string": "first", "new_string": "an unrelated first edit"},
        {"old_string": old, "new_string": payload},
    ]}
elif tool == "NotebookEdit":
    ti = {"notebook_path": path, "cell_id": "cell-1", "new_source": payload}
else:
    ti = {"file_path": path, "old_string": old, "new_string": payload}
print(json.dumps({"tool_name": tool, "tool_input": ti}))' "$1" "${FILE:-src/config.ts}" "${3:-TODO}" \
    | bash "$HOOK" >/dev/null 2>"$ERR"
  rc=$?; err=$(cat "$ERR")
}

show() { local s="${1:0:80}"; printf '%s' "${s//$'\n'/ }"; }

block() { # $1=tool (see run)  $2=word the block message must name  $3=payload
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

allow() { # $1=tool (see run)  $2=payload  $3=old_string (Edit)
  run "$1" "$2" "${3:-}"
  if [ "$rc" = 0 ] && [ -z "$err" ]; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL [want quiet allow, rc %s] %s %s: %s\n  stderr: %s\n' \
      "$rc" "$1" "${FILE:-src/config.ts}" "$(show "$2")" "$(show "$err")"
  fi
}

rep() { python3 -c 'import sys; print(sys.argv[1] * int(sys.argv[2]), end="")' "$1" "$2"; }
# $1 repeated and cut to exactly $2 characters: a body of a given length.
body() { python3 -c 'import sys; u, n = sys.argv[1], int(sys.argv[2]); print((u * n)[:n], end="")' "$1" "$2"; }

# ── Fake secrets, one per shape the guard claims to detect ──────────────────
SK=sk; AK=AKIA; AS=ASIA; GH=gh; GHP=github; XOX=xox; D5=-----
ANTHROPIC="${SK}-ant-api03-$(rep FAKE_body- 9)AA"
OPENAI="${SK}-$(rep 0FAKE 10)"
AWS="${AK}$(rep FAKE 4)"
AWS_TMP="${AS}$(rep FAKE 4)"
GH_P="${GH}p_$(rep 0FAKE 8)"
GH_O="${GH}o_$(rep 0FAKE 8)"
GH_S="${GH}s_$(rep 0FAKE 8)"
OPENAI_NONE="${SK}-None-$(rep 0FAKE 10)"
OPENAI_PROJ="${SK}-proj-$(rep FAKE_proj-body 8)"
OPENAI_SVC="${SK}-svcacct-$(rep FAKE_svc-body 8)"
OPENAI_ADMIN="${SK}-admin-$(rep FAKE_admin-body 8)"
# An older service key: a service-account name, then 20 + watermark + 20.
OPENAI_SERVICE="${SK}-service-fake-svc-$(body FAKE0 20)T3BlbkFJ$(body 0FAKE 20)"
GH_U="${GH}u_$(rep 0FAKE 8)"
GH_R="${GH}r_$(rep 0FAKE 8)"
GH_PAT="${GHP}_pat_$(body 0FAKE 22)_$(body FAKE0 59)"
# A stateless installation token: prefix, app id, _, then a JWT.
GH_S_JWT="${GH}s_1536800_eyJ$(rep FAKE 9).eyJ$(rep FAKE_ 30).$(rep FAKE- 20)"
SLACK_B="${XOX}b-1234567890-1234567890123-$(rep FAKE 6)"
SLACK_P="${XOX}p-1234567890-1234567890-1234567890123-$(rep 0fake 6)"
# A block with one 64-character line of fake base64 material, as PEM wraps it.
pem() { printf '%sBEGIN %s%s\n%s\n%sEND %s%s\n' "$D5" "$1" "$D5" "$(body MIIEFAKE0+/fake 64)" "$D5" "$1" "$D5"; }
PK_RSA="$(pem 'RSA PRIVATE KEY')"
PK_OPENSSH="$(pem 'OPENSSH PRIVATE KEY')"
PK_EC="$(pem 'EC PRIVATE KEY')"
PK_PKCS8="$(pem 'PRIVATE KEY')"
PK_ENCRYPTED="$(pem 'ENCRYPTED PRIVATE KEY')"
PK_PGP="$(pem 'PGP PRIVATE KEY BLOCK')"
FILLER="$(rep $'an ordinary line of prose in a large generated file\n' 5000)"  # ~260 KB

# ── must BLOCK: each pattern, via every tool routed to the guard ────────────
for tool in Write Edit MultiEdit NotebookEdit; do
  block "$tool" Anthropic   "$ANTHROPIC"
  block "$tool" OpenAI      "$OPENAI"
  block "$tool" OpenAI      "$OPENAI_NONE"
  block "$tool" OpenAI      "$OPENAI_PROJ"
  block "$tool" OpenAI      "$OPENAI_SVC"
  block "$tool" OpenAI      "$OPENAI_ADMIN"
  block "$tool" OpenAI      "$OPENAI_SERVICE"
  block "$tool" AWS         "$AWS"
  block "$tool" AWS         "$AWS_TMP"
  block "$tool" GitHub      "$GH_P"
  block "$tool" GitHub      "$GH_O"
  block "$tool" GitHub      "$GH_S"
  block "$tool" GitHub      "$GH_U"
  block "$tool" GitHub      "$GH_R"
  block "$tool" GitHub      "$GH_PAT"
  block "$tool" GitHub      "$GH_S_JWT"
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
block Write AWS       "AWS_ACCESS_KEY_ID=$AWS_TMP"
block Edit  AWS       "credentials = {'AccessKeyId': '$AWS_TMP'}"
block Write OpenAI    "client = OpenAI(api_key='$OPENAI_PROJ')"
block Write OpenAI    "OPENAI_API_KEY=$OPENAI_SERVICE"
block Edit  OpenAI    "${SK}-service-$(body 0FAKE 48)"
block Write OpenAI    "${SK}-service-fake_svc_$(body 0FAKE 48)"
block Edit  OpenAI    "${SK}-service-your-service-key-$(body 0FAKE 48)"
# A template is exempt only for what it holds, not for its name: a real value
# in a committed .env.example is the likeliest way a secret reaches git.
FILE=.env.example block Write GitHub "GITHUB_TOKEN=$GH_P"
# Serialized text puts a letter, digit, _ or - right before a key: an escape
# in a quoted string, a percent-encoded character, a joined name.
block Write OpenAI '{"keys": "old\n'"$OPENAI_PROJ"'"}'
block Edit  OpenAI "https://example.com/callback?next=%2F&key%3D$OPENAI_SVC"
block Write OpenAI "OPENAI_KEY_$OPENAI_ADMIN"
block Write OpenAI "token-$OPENAI_PROJ"
block Edit  OpenAI "key1$OPENAI_SVC"
block Write AWS    '{"creds": "id\n'"$AWS_TMP"'"}'
block Edit  AWS    "https://sts.example.com/?AccessKeyId%3D$AWS_TMP"
block Write AWS    '{"id": "\u002F'"$AWS_TMP"'"}'
block Write AWS    "id = '\\x2F$AWS_TMP'"
block Edit  AWS    "${AWS_TMP}secretAccessKey"
block Write GitHub "GITHUB_TOKEN_${GH_P}_rotated_weekly"
block Edit  GitHub "export GITHUB_TOKEN=$GH_S_JWT"
block Write GitHub "${GH}u_eyJ$(rep FAKE 5)"
block Write GitHub "${GH}r_9_eyJ$(rep FAKE 5)"
block Write GitHub "${GH}s_12_34_eyJ$(rep FAKE 5)"
# A large write, secret first — once allowed silently (#101).
block Write OpenAI        "$OPENAI"$'\n'"$FILLER"
block Edit  'private key' "$PK_RSA$FILLER"

# ── must ALLOW: prose that mentions tokens is not a token ───────────────────
allow Write 'Set your API key and bearer token in the environment. Never commit a secret, a password or a private key.'
allow Write 'Anthropic keys start with sk-ant-, OpenAI keys with sk-, AWS key IDs with AKIA, GitHub tokens with ghp_ and Slack bot tokens with xoxb-.'
allow Edit  'Rotate the token if it leaks; see docs/security.md.'
allow Write 'OpenAI project keys start with sk-proj- and fine-grained GitHub tokens with github_pat_.'

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
allow Edit  'OPENAI_API_KEY = "sk-None-placeholder-for-local-dev"'
allow Write '<div class="desk-admin-navigation-sidebar-collapsed-state-controller">'
allow Edit  'const route = "/task-proj-onboarding-checklist-and-welcome-email-sequence";'
allow Write '<div class="sk-admin-navigation-sidebar-collapsed-state-controller">'
allow Edit  't("sk-proj-onboarding-checklist-welcome-email-sequence-step-three-title")'
allow Write 'sk-svcacct-rotation_reminder-banner-dismissed-at-timestamp-for-the-current-org: true'
allow Edit  'github_pat_rotation_reminder_days_for_organization_members_with_admin_access = 30'
allow Write 'sk-service-account-rotation-reminder-banner-dismissed-at-timestamp-for-the-current-org: true'
allow Edit  't("sk-service-worker-registration-failed-offline-fallback-page-title-for-every-locale")'
allow Write "OPENAI_API_KEY=${SK}-service-$(rep x 60)"
allow Edit  "OPENAI_API_KEY=${SK}-service-your-service-account-key-goes-here"
allow Write 'const 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 = true;'
allow Write 'const REGION = "ASIAPACIFICDATACENTER01";'
allow Edit  'EURASIAPACIFICREGION024 = load_regions()'
# Long snake_case names holding a GitHub prefix: right_ holds ght_, highs_ holds
# ghs_. Joined at runtime, since the guard blocked them before (#143).
allow Write "def test_blocks_ri${GH}t_token_when_a_write_holds_a_long_snake_case_name(): pass"
allow Edit  "hi${GH}s_and_lows_for_every_region_in_the_dataset_since_2000 = {}"
allow Write "const wei${GH}t_surveyJson_for_every_respondent_in_the_panel = load();"
allow Edit  "ri${GH}t_eye_contact_duration_for_the_whole_recorded_session = 3"
allow Write 'Installation tokens now look like ghs_APPID_JWT.'

# ── must ALLOW: placeholders shaped like a key (#143) ───────────────────────
# Documented example values, your-... phrases, one repeated character, <...>.
allow Write "SLACK_BOT_TOKEN=${XOX}b-your-token"
allow Edit  "slack_token: ${XOX}p-your-slack-user-token-here"
allow Write "SLACK_BOT_TOKEN=${XOX}b-$(rep x 10)-$(rep x 13)-$(rep x 24)"
allow Write "aws_access_key_id = ${AK}IOSFODNN7EXAMPLE"
allow Write "${AK}I44QH8DHBEXAMPLE"
allow Edit  "AccessKeyId: ${AS}IOSFODNN7EXAMPLE"
allow Write "AWS_ACCESS_KEY_ID=${AK}$(rep X 16)"
allow Edit  "AWS_ACCESS_KEY_ID=${AS}$(rep X 16)"
allow Write "GITHUB_TOKEN=${GH}p_$(rep x 40)"
allow Write "OPENAI_API_KEY=${SK}-$(rep x 48)"
allow Edit  "OPENAI_API_KEY=${SK}-None-$(rep X 48)"
allow Write "ANTHROPIC_API_KEY=${SK}-ant-api03-$(rep x 95)"
allow Write "ANTHROPIC_API_KEY=${SK}-ant-$(rep X_ 30)"
allow Edit  "ANTHROPIC_API_KEY=${SK}-ant-your-anthropic-api-key-goes-here-and-stays-out-of-git"
allow Write "ANTHROPIC_API_KEY=${SK}-ant-YOUR_ANTHROPIC_API_KEY_GOES_HERE_PLEASE_THANKS"
allow Write "SLACK_BOT_TOKEN=${XOX}b-YOUR-BOT-TOKEN"
allow Write "OPENAI_API_KEY=${SK}-proj-$(rep x- 60)"
allow Edit  "OPENAI_API_KEY=${SK}-svcacct-YOUR_SERVICE_ACCOUNT_KEY_GOES_HERE_AND_NEVER_INTO_A_COMMITTED_FILE_OR_ANY_BUILD_LOG"
allow Write "GITHUB_TOKEN=${GH}p_<your-token>"
allow Write "SLACK_BOT_TOKEN=${XOX}b-<your-bot-token>"
allow Edit  "ANTHROPIC_API_KEY=${SK}-ant-<your-key> OPENAI_API_KEY=${SK}-proj-<project-key>"
allow Write "AWS_ACCESS_KEY_ID=${AK}<ACCESS_KEY_ID>"

# ── must BLOCK: a real key is never hidden by a placeholder next to it ──────
block Write Slack     "${XOX}b-your-1234567890-$(rep FAKE 6)"
block Edit  Slack     "${XOX}b-$(rep x 10)-1234567890123-$(rep FAKE 6)"
block Write Slack     "${XOX}b-your-token-$SLACK_B"
block Write Slack     "${XOX}b-your-token $SLACK_B"
block Write AWS       "${AK}$(body FAKE 16)EXAMPLE"
block Edit  AWS       "${AK}$(rep X 15)F"
block Write AWS       "id = ${AS}$(rep X 15)F"
block Write AWS       "${AK}IOSFODNN7EXAMPLE $AWS"
block Write AWS       "${AK}EXAMPLE$(body FAKE 9)"
block Write GitHub    "${GH}p_$(rep x 35)F"
block Edit  GitHub    "${GH}p_$(rep x 36)F0FAKE"
block Write GitHub    "${GH}p_$(rep x 36)$GH_P"
block Write OpenAI    "${SK}-$(rep x 47)F"
block Edit  OpenAI    "${SK}-None-$(rep x 40) $OPENAI"
block Write Anthropic "${SK}-ant-api03-$(rep x 40)FAKE"
block Edit  Anthropic "${SK}-ant-your-key-FAKE0-$(body FAKE0 40)"
block Write Anthropic "${SK}-ant-your-key-here-$ANTHROPIC"
block Write OpenAI    "${SK}-proj-your-project-key-$(rep FAKE0_ 16)FAKE0"
block Write AWS       "id = ${AS}EXAMPLE$(body FAKE 9)"
# Only the whole body counts: "your" glued to more letters, or an unknown
# key-type segment before a repeated run, is not a placeholder.
block Edit  Anthropic "${SK}-ant-your$(rep FAKEfake 6)"
block Write OpenAI    "${SK}-proj-YOUR$(rep FAKEfake 12)"
block Write Slack     "${XOX}b-your$(rep FAKEfake 2)"
block Write Anthropic "${SK}-ant-fake0-$(rep x 60)"
block Edit  OpenAI    "${SK}-proj-$(rep x 80)FAKE"

# ── must ALLOW: a private-key header with no key material (#143) ────────────
allow Write "Keys in PKCS#1 form start with \"${D5}BEGIN RSA PRIVATE KEY${D5}\"; see https://docs.example.com/security/private-keys.html"
allow Edit  "if line.startswith('${D5}BEGIN OPENSSH PRIVATE KEY${D5}'):"$'\n'"    return True"
allow Write "${D5}BEGIN PRIVATE KEY${D5}"$'\n'"<your private key>"$'\n'"${D5}END PRIVATE KEY${D5}"
allow Write "${D5}BEGIN RSA PRIVATE KEY${D5}"$'\n'"MIIEpAIBAAKCAQEA..."$'\n'"${D5}END RSA PRIVATE KEY${D5}"
allow Edit  "${D5}BEGIN EC PRIVATE KEY${D5}"$'\n'"${D5}END EC PRIVATE KEY${D5}"
allow Write "PEM_HEADER = '${D5}BEGIN PRIVATE KEY${D5}'"$'\n'"$(pem CERTIFICATE)"

# ── must BLOCK: a header with key material, however it is written ───────────
block Write 'private key' "\"${D5}BEGIN PRIVATE KEY${D5}\\n$(body MIIEFAKE0+/ 64)\\n${D5}END PRIVATE KEY${D5}\""
block Edit  'private key' "${D5}BEGIN RSA PRIVATE KEY${D5}"$'\n'"Proc-Type: 4,ENCRYPTED"$'\n'"DEK-Info: AES-128-CBC,FAKE"$'\n\n'"$(body MIIEFAKE0+/ 64)"
block Write 'private key' "${D5}BEGIN PGP PRIVATE KEY BLOCK${D5}"$'\n'"Version: FAKE"$'\n'"Comment: exported by a fake key tool for the devexp guard tests, not a real key"$'\n\n'"$(body lQOYBFAKE0+/ 64)"
block Write 'private key' "key = \"${D5}BEGIN RSA PRIVATE KEY${D5}\\n\" +"$'\n'"  \"$(body MIIEFAKE0+/ 64)\\n\""
block Edit  'private key' "${D5}BEGIN PRIVATE KEY${D5}$(body MIIEFAKE0+/ 64)"
block Write 'private key' "${D5}BEGIN OPENSSH PRIVATE KEY${D5}"$'\n'"$(body b3BlbnNzaFAKE0 70)"
block Write 'private key' "Look for ${D5}BEGIN RSA PRIVATE KEY${D5} at the top."$'\n'"$PK_RSA"
block Edit  'private key' "${D5}BEGIN EC PRIVATE KEY${D5}"$'\n'"${D5}END EC PRIVATE KEY${D5}"$'\n'"$PK_EC"
# A dash rule between header and body doesn't end the search; only an END or
# BEGIN line does (#158).
for kind in 'RSA PRIVATE KEY' 'PRIVATE KEY' 'EC PRIVATE KEY' 'DSA PRIVATE KEY' 'ENCRYPTED PRIVATE KEY' 'OPENSSH PRIVATE KEY' 'PGP PRIVATE KEY BLOCK' 'PGP SECRET KEY BLOCK'; do
  block Write 'private key' "${D5}BEGIN $kind${D5}"$'\n'"${D5}"$'\n'"$(body MIIEFAKE0+/ 64)"$'\n'"${D5}END $kind${D5}"
done
# JSON that escapes every slash still carries the key material.
block Write 'private key' "{\"key\": \"${D5}BEGIN PRIVATE KEY${D5}\\n$(body 'MIIEFAKE0abcdefgh+\/' 64)\\n$(body 'fakeFAKE0123456+\/' 64)\\n${D5}END PRIVATE KEY${D5}\"}"
# Key-like text on the header's line, or just after it, still blocks.
block Edit  'private key' "if (pem.startsWith(\"${D5}BEGIN PRIVATE KEY${D5}\")) return parsePkcs8PrivateKeyFromPemEncodedString(pem);"

# ── must ALLOW: a quoted header, whatever the rest of the write holds (#158) ─
# Only text near the header counts as its key material, and the search ends at
# an END or BEGIN line, so a later identifier, fingerprint, path or hash in the
# same file doesn't block.
PROSE="$(rep $'This key is used by the deploy job to reach the staging hosts over SSH.\n' 8)"
CODE="$(rep $'  logger.debug(\'reading the key file for the deploy service\');\n' 10)"
allow Write "if (pem.startsWith(\"${D5}BEGIN PRIVATE KEY${D5}\")) {"$'\n'"$CODE"$'\n'"  return parsePkcs8PrivateKeyFromPemEncodedString(pem);"$'\n'"}"
allow Edit  "Paste the key (it starts with ${D5}BEGIN OPENSSH PRIVATE KEY${D5})."$'\n\n'"$PROSE"$'\n'"The host fingerprint is SHA256:$(body FAKEfake0+/ 43)."
allow Write "Store the ${D5}BEGIN EC PRIVATE KEY${D5} file on the host."$'\n'"$PROSE"$'\n'"Path: /home/deploy/configuration/secrets/keys/"
allow Write "${D5}BEGIN PRIVATE KEY${D5}"$'\n'"<paste your private key here>"$'\n'"${D5}END PRIVATE KEY${D5}"$'\n'"checksum: $(body 0123456789abcdef 64)"
allow Edit  "Header: ${D5}BEGIN RSA PRIVATE KEY${D5}"$'\n\n'"$PROSE$PROSE"$'\n'"Fixed in commit $(body 0123456789abcdef 40)."

# ── length thresholds, pinned at the edge: one short allows, exact blocks ───
allow Write "${D5}BEGIN RSA PRIVATE KEY${D5}"$'\n'"$(body FAKE0+/ 31)"$'\n'"${D5}END RSA PRIVATE KEY${D5}"
block Write 'private key' "${D5}BEGIN RSA PRIVATE KEY${D5}"$'\n'"$(body FAKE0+/ 32)"$'\n'"${D5}END RSA PRIVATE KEY${D5}"
# Bounded repetitions (#158), pinned at the edge: material that starts just
# inside the search window after a header, the header's words, the words of a
# your-... phrase, service-key name segments, and GitHub id segments.
block Write 'private key' "${D5}BEGIN RSA PRIVATE KEY${D5}"$'\n'"$(body ' ' 494)$(body FAKE0+/ 32)"
allow Write "${D5}BEGIN RSA PRIVATE KEY${D5}"$'\n'"$(body ' ' 495)$(body FAKE0+/ 32)"
block Write 'private key' "${D5}BEGIN $(body 'ABC ' 40)PRIVATE KEY${D5}"$'\n'"$(body MIIEFAKE0+/ 64)"
allow Write "${D5}BEGIN $(body 'ABC ' 41)PRIVATE KEY${D5}"$'\n'"$(body MIIEFAKE0+/ 64)"
allow Write "${SK}-ant-your$(rep -a 24)"
block Write Anthropic "${SK}-ant-your$(rep -a 25)"
allow Write "${SK}-proj-your$(rep -abc 24)"
block Write OpenAI    "${SK}-proj-your$(rep -abc 25)"
allow Write "${XOX}b-your$(rep -a 24)"
block Write Slack     "${XOX}b-your$(rep -a 25)"
block Write OpenAI    "${SK}-service-$(rep ab- 20)$(body 0FAKE 48)"
allow Write "${SK}-service-$(rep ab- 21)$(body 0FAKE 48)"
block Write OpenAI    "${SK}-service-$(body abcFAKE 47)-$(body 0FAKE 48)"
block Write GitHub    "${GH}s_$(rep 1_ 8)eyJ$(rep FAKE 5)"
allow Write "${GH}s_$(rep 1_ 9)eyJ$(rep FAKE 5)"
allow Write "${SK}-ant-$(body FAKE_ant- 39)"
block Write Anthropic "${SK}-ant-$(body FAKE_ant- 40)"
allow Write "${SK}-proj-$(body FAKE_proj- 79)"
block Write OpenAI    "${SK}-proj-$(body FAKE_proj- 80)"
allow Write "${SK}-service-fake-svc-$(body 0FAKE 47)"
block Write OpenAI    "${SK}-service-fake-svc-$(body 0FAKE 48)"
allow Write "${SK}-None-$(body 0FAKE 31)"
block Write OpenAI    "${SK}-None-$(body 0FAKE 32)"
allow Write "${AS}$(body FAKE 15)"
block Write AWS       "${AS}$(body FAKE 16)"
allow Write "${GH}u_$(body 0FAKE 35)"
block Write GitHub    "${GH}u_$(body 0FAKE 36)"
allow Write "${GH}s_$(body 0FAKE 35)"
block Write GitHub    "${GH}s_$(body 0FAKE 36)"
allow Write "${GH}p_$(body 0FAKE 35)_$(body FAKE0 40)"
allow Write "${GHP}_pat_$(body 0FAKE 21)_$(body FAKE0 59)"
allow Write "${GHP}_pat_$(body 0FAKE 23)_$(body FAKE0 59)"
allow Write "${GHP}_pat_$(body 0FAKE 22)_$(body FAKE0 58)"
allow Write "${GHP}_pat_$(body 0FAKE 81)"
block Write GitHub    "${GHP}_pat_$(body 0FAKE 22)_$(body FAKE0 59)"

# ── must ALLOW: removing a secret, and empty writes ─────────────────────────
# Only the new text is scanned; an Edit that takes a key out must not be refused.
allow Edit 'OPENAI_API_KEY=process.env.OPENAI_API_KEY' "OPENAI_API_KEY=$OPENAI"
allow MultiEdit 'OPENAI_API_KEY=process.env.OPENAI_API_KEY' "OPENAI_API_KEY=$OPENAI"
allow MultiEdit 'const token = process.env.GITHUB_TOKEN;'
allow NotebookEdit 'import os\nclient = OpenAI(api_key=os.environ["OPENAI_API_KEY"])'
allow Edit ''
allow Write ''
allow Write "$FILLER"

# ── routing: every content-writing tool reaches the guard ───────────────────
# Claude Code matches a plain "A|B" matcher by exact tool name, so "Write|Edit"
# never runs the guard for MultiEdit or NotebookEdit.
for tool in Write Edit MultiEdit NotebookEdit; do
  if python3 -c '
import json, sys
reg = json.load(open(sys.argv[1]))
m = next(h for h in reg if h["name"] == "secret-in-write-guard")["claude_code"]["matcher"]
sys.exit(0 if sys.argv[2] in [t.strip() for t in m.split("|")] else 1)' \
      "$(dirname "$HOOK")/../registry.json" "$tool"; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); printf 'FAIL [not routed] registry matcher for secret-in-write-guard omits %s\n' "$tool"
  fi
done

# ── timing: every pattern decides in linear time (#158) ─────────────────────
# A hook that runs past Claude Code's timeout doesn't block the call, so a slow
# pattern lets the write through. These writes repeat a prefix or a header so
# that an unbounded repetition would rescan from every start. The 100 KB tier
# stops at its first failure so a slow guard fails fast; the 2 MB tier runs
# only when the first tier passed. Budgets include starting the hook.
timing=$(python3 - "$HOOK" <<'PY'
import json, subprocess, sys, time
hook = sys.argv[1]
D5, SK, GH = '-' * 5, 's' + 'k', 'g' + 'h'
def rep(unit, n):
    return (unit * (n // len(unit) + 1))[:n]
SHAPES = [
    ('key-type words after a private-key header', lambda n: D5 + 'BEGIN ' + rep('PRIVATE KEY ', n)),
    ('private-key header, then spaced capitals', lambda n: D5 + 'BEGIN PRIVATE KEY' + rep(' A', n)),
    ('private-key headers between near-material runs', lambda n: rep(D5 + 'BEGIN RSA PRIVATE KEY' + D5 + '\n' + rep('A' * 31 + '.', 600), n)),
    ('repeated service-key prefix', lambda n: rep(SK + '-service-', n)),
    ('service-key prefixes with near-length name segments', lambda n: rep(SK + '-service-' + rep('a' * 47 + '-', 200), n)),
    ('repeated Anthropic prefix and your-', lambda n: rep(SK + '-ant-your-', n)),
    ('repeated project-key prefix and your-', lambda n: rep(SK + '-proj-your-', n)),
    ('repeated GitHub prefix', lambda n: rep(GH + 'p_', n)),
    ('repeated GitHub prefix and id segment', lambda n: rep(GH + 's_1_', n)),
]
ran = failed = 0
for tier, size, budget in (('100 KB', 100000, 2.0), ('2 MB', 2000000, 8.0)):
    for name, make in SHAPES:
        envelope = json.dumps({'tool_name': 'Write', 'tool_input': {'file_path': 'big.txt', 'content': make(size)}})
        start = time.monotonic()
        try:
            subprocess.run(['bash', hook], input=envelope, capture_output=True, text=True, timeout=budget)
            took = time.monotonic() - start
        except subprocess.TimeoutExpired:
            took = None
        ran += 1
        if took is None or took > budget:
            failed += 1
            print('FAIL [timing %s] %s: %s, budget %.0f s' % (tier, name, 'still running at the budget' if took is None else 'took %.1f s' % took, budget))
            if tier == '100 KB':
                break
    if failed:
        break
print('TIMING %d %d' % (ran, failed))
PY
)
case "$timing" in
  *TIMING*)
    printf '%s' "${timing%TIMING*}"
    read -r t_ran t_failed <<<"${timing##*TIMING }"
    pass=$((pass + t_ran - t_failed)); fail=$((fail + t_failed)) ;;
  *) fail=$((fail+1)); printf 'FAIL [timing] the timing check did not run: %s\n' "$timing" ;;
esac

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
