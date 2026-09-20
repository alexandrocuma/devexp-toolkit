#!/usr/bin/env bash
# Mutation smoke test: prove the guards go red on demand.
#
# Every other suite in this repo answers "is the tree healthy?". This one
# answers a different question: "would we be told if it were not?" A green
# suite is exactly the evidence a broken guard also produces, and this repo has
# rediscovered "the guard doesn't actually guard" four separate times. So each
# check here breaks the thing a guard protects, asserts the guard fails, and
# puts it back.
#
# It mutates tracked files, so it refuses to start on a dirty tree and restores
# on ANY exit, including Ctrl-C. The clean-tree requirement is what makes the
# restore provable: everything is put back with `git checkout --`, not from a
# copy this script hopes it wrote correctly.
#
# Run: ./scripts/smoke.sh [--no-remote] [--quick]
#   --no-remote  skip the branch-protection section (works offline)
#   --quick      skip the full baseline suite (the mutations still run)
#
# NOT a CI job. It takes minutes and mutates the working tree; it is a manual
# check before a release and after touching a guard. See docs/development/testing.md.

# Deliberately NO `pipefail`. Every command below is EXPECTED to exit non-zero —
# that is what a caught mutation looks like — and its output is then grepped for
# the message. With pipefail the pipeline would report the command's failure
# even when grep matched, scoring every successful detection as a miss. Output
# is captured into a variable and matched separately so the two signals stay
# apart. This is not a style preference; getting it wrong once already produced
# a run that reported 11 failures against guards that were all working.
set -uo pipefail
set +o pipefail

cd "$(dirname "$0")/.."
REPO="$(pwd)"
GOLANGCI="$(go env GOPATH 2>/dev/null)/bin/golangci-lint"
NO_REMOTE=0
QUICK=0
for arg in "$@"; do
  case "$arg" in
    --no-remote) NO_REMOTE=1 ;;
    --quick)     QUICK=1 ;;
    -h|--help)   sed -n '2,20p' "$0"; exit 0 ;;
    *) printf 'unknown flag: %s\n' "$arg" >&2; exit 2 ;;
  esac
done

# Only TRACKED modifications matter. The restore path is `git checkout --`,
# which cannot reach untracked files and does not need to: an unrelated scratch
# file is no reason to refuse, and refusing on one would make this script
# unrunnable while it is itself still untracked.
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  cat >&2 <<'MSG'
smoke: refusing to run — the working tree is dirty.

This script mutates tracked files and restores them with `git checkout --`.
From a dirty tree that restore would also discard your uncommitted work, and
it could not tell your edits apart from its own. Commit or stash first.
MSG
  exit 2
fi

pass=0; fail=0
CREATED=()   # untracked files this script makes; removed on exit
TOUCHED=()   # tracked files it mutates; restored on exit

restore() {
  local rc=$?
  [ ${#CREATED[@]} -gt 0 ] && rm -f "${CREATED[@]}" 2>/dev/null
  [ ${#TOUCHED[@]} -gt 0 ] && git checkout -- "${TOUCHED[@]}" 2>/dev/null
  # stage-assets output is gitignored, so git cannot restore it; section 0
  # deletes it on purpose and this puts it back.
  [ -d "$REPO/cli/internal/assets/agents" ] || ./scripts/stage-assets.sh >/dev/null 2>&1
  if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
    printf '\n\033[31msmoke: the tree is still dirty after restore — inspect before committing\033[0m\n' >&2
    git status --short --untracked-files=no >&2
  fi
  exit $rc
}
trap restore EXIT INT TERM

ok()  { pass=$((pass+1)); printf '  \033[32mPASS\033[0m  %s\n' "$1"; }
bad() { fail=$((fail+1)); printf '  \033[31mFAIL\033[0m  %s\n' "$1"; [ -n "${2:-}" ] && printf '        %s\n' "$2"; }
skip(){ printf '  \033[33mSKIP\033[0m  %s\n' "$1"; }

mutate() { TOUCHED+=("$1"); }                       # mark before editing
creates() { CREATED+=("$1"); }                      # mark before creating
undo()   { git checkout -- "$1" 2>/dev/null; }      # put one file back now
gotest() { (cd "$REPO/cli" && go test "$1" -count=1 2>&1); }
lint()   { (cd "$REPO/cli" && "$GOLANGCI" run --config ../.golangci.yml ./... 2>&1); }

# expect <label> <pattern> <output>
expect() { echo "$3" | grep -q "$2" && ok "$1" || bad "$1" "$(echo "$3" | grep -E '^(---|    |cli/)' | head -2)"; }

echo "=== 0. The fresh-clone gotcha ==="
rm -rf cli/internal/assets/agents cli/internal/assets/skills cli/internal/assets/hooks
out=$(cd "$REPO/cli" && go build ./... 2>&1)
expect "go build fails without staged assets, as the gotcha documents" "no matching files found" "$out"
./scripts/stage-assets.sh >/dev/null 2>&1 && ok "stage-assets.sh fixes it" || bad "stage-assets.sh failed"

echo
echo "=== 1. Baseline ==="
if [ "$QUICK" = 1 ]; then
  skip "full suite (--quick)"
else
  base=$(cd "$REPO/cli" && go test ./... -race -cover -count=1 2>&1)
  echo "$base" | grep -qE '^(FAIL|---)' \
    && bad "baseline suite is red" "$(echo "$base" | grep -E '^(FAIL|---)' | head -3)" \
    || ok "go test ./... -race -cover -count=1"
fi

echo
echo "=== 2. Repo consistency ==="

mutate hooks/registry.json
python3 - <<'PY'
import json; p='hooks/registry.json'; d=json.load(open(p))
d[0]['claude_code']['script']='hooks/claude-code/DOES-NOT-EXIST.sh'; json.dump(d,open(p,'w'),indent=2)
PY
expect "registry entry naming a missing file" "does not exist" "$(gotest ./internal/repocheck/)"
undo hooks/registry.json

python3 - <<'PY'
import json; p='hooks/registry.json'; d=json.load(open(p))
k=next(h for h in d if (h.get('kimi') or {}).get('enabled') is False); k['kimi'].pop('reason',None)
json.dump(d,open(p,'w'),indent=2)
PY
expect "kimi block disabled without a reason" "no reason" "$(gotest ./internal/repocheck/)"
undo hooks/registry.json

creates hooks/claude-code/smoke-orphan.sh
touch hooks/claude-code/smoke-orphan.sh
expect "hook on disk with no registry entry" "smoke-orphan.sh" "$(gotest ./internal/repocheck/)"
rm -f hooks/claude-code/smoke-orphan.sh

creates agents/smoke-ghost.md
touch agents/smoke-ghost.md
out=$(gotest ./internal/repocheck/)
echo "$out" | grep -q "smoke-ghost.md" && echo "$out" | grep -q "no longer says" \
  && ok "a new agent trips both the catalog and the counts" \
  || bad "a new agent should trip both the catalog and the counts"
rm -f agents/smoke-ghost.md

mutate docs/reference/agents.md
python3 - <<'PY'
p='docs/reference/agents.md'; s=open(p).read()
i=s.index('| `migration.md`'); j=s.index('\n',i); open(p,'w').write(s[:i]+s[j+1:])
PY
expect "agent catalog row removed" "migration.md" "$(gotest ./internal/repocheck/)"
undo docs/reference/agents.md

mutate docs/README.md
printf '\n99 hooks ship here.\n' >> docs/README.md
expect "a wrong count written in a new sentence" "99 hooks" "$(gotest ./internal/repocheck/)"
undo docs/README.md

echo
echo "=== 3. Procedures duplicated across skills ==="

mutate skills/cleanup/SKILL.md
python3 - <<'PY'
p='skills/cleanup/SKILL.md'; s=open(p).read()
s=s.replace('safe_id() { case "${1}" in ""|*[!A-Za-z0-9_-]*)','safe_id() { case "${1}" in *[!A-Za-z0-9_.-]*)',1)
open(p,'w').write(s)
PY
expect "safe_id() drifted between two skills" "has drifted" "$(gotest ./internal/skills/)"
undo skills/cleanup/SKILL.md

mutate skills/improve/SKILL.md
python3 -c "
p='skills/improve/SKILL.md';s=open(p).read();open(p,'w').write(s.replace('main checkout','checkout'))"
expect "a load-bearing grant claim dropped from one copy" "MAIN CHECKOUT" "$(gotest ./internal/skills/)"
undo skills/improve/SKILL.md

python3 -c "
import re;p='skills/improve/SKILL.md';s=open(p).read()
open(p,'w').write(re.sub(r'(?m)^safe_id\(\) \{ case.*\$','',s))"
expect "the guard refuses to pass vacuously when a copy disappears" "meaningless below two" "$(gotest ./internal/skills/)"
undo skills/improve/SKILL.md

echo
echo "=== 4. Lint: no silent success, no external comment refs ==="
if [ ! -x "$GOLANGCI" ]; then
  skip "golangci-lint not installed — the CI lint job covers it"
else
  expect "a clean tree lints clean" "0 issues" "$(lint)"

  mutate cli/internal/fsutil/atomic.go
  python3 -c "
p='cli/internal/fsutil/atomic.go';s=open(p).read()
i=s.index('//nolint:errcheck //'); j=s.index('\n',i)
open(p,'w').write(s[:i]+'//nolint:errcheck'+s[j:])"
  expect "a //nolint stripped of its reason" "should provide explanation" "$(lint)"
  undo cli/internal/fsutil/atomic.go

  python3 -c "
p='cli/internal/fsutil/atomic.go';s=open(p).read()
open(p,'w').write(s.replace('func syncDir(dir string) {','func syncDir(dir string) {\n\tos.MkdirAll(dir, 0o755)',1))"
  expect "a newly discarded error" "errcheck" "$(lint)"
  undo cli/internal/fsutil/atomic.go
fi

mutate cli/internal/fsutil/atomic.go
python3 -c "
p='cli/internal/fsutil/atomic.go';s=open(p).read()
open(p,'w').write(s.replace('func syncDir(dir string) {','// See #999 for why.\nfunc syncDir(dir string) {',1))"
expect "a comment carrying an external reference" "atomic.go" "$(./scripts/check-comment-refs.sh 2>&1)"
undo cli/internal/fsutil/atomic.go

echo
echo "=== 5. The test cache cannot hide an asset edit ==="
mutate hooks/registry.json
# Prime WITHOUT -count=1: that flag bypasses the cache, so priming with it
# stores nothing and the control below would prove nothing.
(cd "$REPO/cli" && go test ./internal/repocheck/ -race -cover >/dev/null 2>&1)
python3 - <<'PY'
import json; p='hooks/registry.json'; d=json.load(open(p))
d[0]['claude_code']['script']='hooks/claude-code/STALE-CHECK.sh'; json.dump(d,open(p,'w'),indent=2)
PY
expect "the documented command catches an edit after a warm cache" "STALE-CHECK.sh" \
  "$(cd "$REPO/cli" && go test ./internal/repocheck/ -race -cover -count=1 2>&1)"
expect "without the flag it is still cached, so the flag is what fixes it" "cached" \
  "$(cd "$REPO/cli" && go test ./internal/repocheck/ -race -cover 2>&1)"
undo hooks/registry.json

echo
echo "=== 6. Guard latency against its own budget ==="
lat=$(bash hooks/claude-code/scan-latency.test.sh 2>&1)
if echo "$lat" | grep -q "% of budget"; then
  ok "bash latency suite reports budget consumption"
  printf '        worst observed: %s%%\n' "$(echo "$lat" | grep -oE '[0-9]+% of budget' | grep -oE '^[0-9]+' | sort -rn | head -1)"
else
  bad "bash latency suite reported no percentages" "$(echo "$lat" | tail -2)"
fi
node hooks/opencode/scan-latency.test.js >/dev/null 2>&1 \
  && ok "opencode latency suite passes" || bad "opencode latency suite failed"

echo
echo "=== 7. Install ==="
inst=$(./install.sh --dry-run 2>&1)
expect "./install.sh --dry-run deploys the skills" "Installed 8 skill" "$inst"

echo
echo "=== 8. Branch protection ==="
if [ "$NO_REMOTE" = 1 ]; then
  skip "branch protection (--no-remote)"
elif ! command -v gh >/dev/null 2>&1; then
  skip "branch protection (gh not installed)"
else
  prot=$(gh api repos/:owner/:repo/branches/main/protection 2>/dev/null)
  if [ -z "$prot" ]; then
    bad "could not read branch protection (no access, or main is unprotected)"
  else
    for want in test hooks lint govulncheck; do
      echo "$prot" | grep -q "\"$want\"" && ok "required check: $want" || bad "required check missing: $want"
    done
    while IFS='|' read -r n v; do
      [ "$v" = "True" ] && ok "$n" || bad "$n"
    done < <(echo "$prot" | python3 -c "
import json,sys
d=json.load(sys.stdin)
for n,v in [('strict (branch up to date)',d['required_status_checks']['strict']),
            ('administrators included',d['enforce_admins']['enabled']),
            ('force-push blocked',not d['allow_force_pushes']['enabled']),
            ('deletion blocked',not d['allow_deletions']['enabled']),
            ('pull request required',d.get('required_pull_request_reviews') is not None)]:
    print(f'{n}|{v}')
" 2>/dev/null)
  fi
fi

echo
echo "────────────────────────────────────────"
printf 'smoke: %d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
