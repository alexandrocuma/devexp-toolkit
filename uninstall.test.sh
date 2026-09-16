#!/usr/bin/env bash
# Tests uninstall.sh.
#
# 1. The Claude Code hook-removal logic. It used to match on `repo_dir in cmd`,
#    so it removed only hooks registered from the current clone. A registration
#    left by an earlier release-binary install lives elsewhere, so uninstalling
#    stripped the working entries and left the stale ones running — strictly
#    worse than doing nothing (issue #93). The block is embedded in uninstall.sh
#    as a heredoc, so it is extracted and run directly against fixtures.
#
# 2. The opencode wiring (issue #109): no top-level `local` (it aborted every
#    opencode uninstall), the MCP block tolerating a bad config.json, detection
#    of a plugin-only install, and delegation of plugin removal to
#    `devexp uninstall --target opencode`. These run uninstall.sh itself with
#    --yes, stdin closed, HOME in a temp dir, a minimal environment and stub
#    devexp binaries, so it never touches real files. The removal rules are
#    tested in Go (cli/internal/hooks, cli/cmd).
#
# 3. A round trip with the real binary, when `go` is on PATH and the CLI's
#    assets are staged (./scripts/stage-assets.sh); otherwise it prints SKIP.
#
# Run: bash uninstall.test.sh
set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

# ── Extract the embedded hook-removal program ───────────────────────────────
awk '/python3 - "\$REPO_DIR" "\$settings_path" <</{flag=1;next} /^PYEOF$/{flag=0} flag' \
    "$ROOT/uninstall.sh" > "$TMP/prune.py"
if [ ! -s "$TMP/prune.py" ]; then
    echo "FAIL: could not extract the hook-removal block from uninstall.sh"
    exit 1
fi

# ── A fake devexp repo, so the registry is the source of managed names ──────
REPO="$TMP/repo"
mkdir -p "$REPO/hooks/claude-code"
cat > "$REPO/hooks/registry.json" <<'JSON'
[
  {"name": "secret-guard", "enabled": true,
   "claude_code": {"event": "PreToolUse", "matcher": "Read|Bash",
                   "script": "hooks/claude-code/secret-guard.sh"}},
  {"name": "graphify-read-guard", "enabled": false,
   "claude_code": {"event": "PreToolUse", "matcher": "Read|Glob",
                   "script": "hooks/claude-code/graphify-read-guard.sh"}}
]
JSON

OTHER="$TMP/other-root"

run() { # $1 = settings json  -> prints resulting commands, one per line
    printf '%s' "$1" > "$TMP/settings.json"
    python3 "$TMP/prune.py" "$REPO" "$TMP/settings.json" >/dev/null 2>&1
    python3 - "$TMP/settings.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
for ev, arr in sorted((d.get('hooks') or {}).items()):
    for e in arr:
        for h in e.get('hooks', []):
            print(h.get('command', ''))
PY
}

expect() { # $1=label  $2=settings  $3=expected remaining commands (newline sep)
    local label="$1" got want
    got="$(run "$2")"
    want="$(printf '%s' "$3")"
    if [ "$got" = "$want" ]; then
        pass=$((pass+1))
    else
        fail=$((fail+1))
        printf 'FAIL %s\n  got:  %s\n  want: %s\n' "$label" "${got:-<empty>}" "${want:-<empty>}"
    fi
}

MINE="$REPO/hooks/claude-code/secret-guard.sh"
FOREIGN="$OTHER/hooks/claude-code/secret-guard.sh"
FOREIGN_DISABLED="$OTHER/hooks/claude-code/graphify-read-guard.sh"
USER_HOOK="$OTHER/my-hooks/secret-guard.sh"
USER_OTHER="$OTHER/my-hooks/format-check.sh"

# Removes our own registration (the normal uninstall case)
expect "removes the clone's own hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$MINE\"}]}]}}" \
  ""

# #93: also removes one registered from a different install root
expect "removes a hook from a foreign install root" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN\"}]}]}}" \
  ""

# Disabled hooks are devexp's too
expect "removes a foreign copy of a disabled hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Glob\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN_DISABLED\"}]}]}}" \
  ""

# A user hook sharing a basename must survive
expect "keeps a user hook that shares a basename" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$USER_HOOK\"}]}]}}" \
  "$USER_HOOK"

# An unrelated user hook must survive
expect "keeps an unrelated user hook" \
  "{\"hooks\":{\"PostToolUse\":[{\"matcher\":\"Write\",\"hooks\":[{\"type\":\"command\",\"command\":\"$USER_OTHER\"}]}]}}" \
  "$USER_OTHER"

# The bug this also fixes: entries were judged by hooks[0] alone, so a second
# command in the same entry was handled by accident rather than on its merits.
expect "an entry holding both keeps only the user command" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN\"},{\"type\":\"command\",\"command\":\"$USER_HOOK\"}]}]}}" \
  "$USER_HOOK"

expect "a user command listed first no longer shields a devexp one" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$USER_HOOK\"},{\"type\":\"command\",\"command\":\"$FOREIGN\"}]}]}}" \
  "$USER_HOOK"

# Both roots registered at once — the real-world state issue #93 describes
expect "removes both roots' copies of the same hook" \
  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$MINE\"}]},{\"matcher\":\"Read|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$FOREIGN\"}]}]}}" \
  ""

# ── opencode (#109) ──────────────────────────────────────────────────────────

ok() { pass=$((pass+1)); }
ko() { fail=$((fail+1)); printf 'FAIL %s\n' "$1"; [ -n "${2:-}" ] && printf '%s\n' "$2" | sed 's/^/  | /'; return 0; }
check() { # $1=label, rest = test command
    local label="$1"; shift
    if "$@"; then ok; else ko "$label" "$(cat "${E:-/nonexistent}/out" 2>/dev/null)"; fi
}

# ── Static: no top-level `local` ─────────────────────────────────────────────
if bash -n "$ROOT/uninstall.sh" && ! grep -nE '^[[:space:]]*local[[:space:]]' "$ROOT/uninstall.sh" >/dev/null; then
    ok
else
    ko "uninstall.sh parses and uses no \`local\` (it has no multi-line functions)" \
       "$(grep -nE '^[[:space:]]*local[[:space:]]' "$ROOT/uninstall.sh")"
fi

# ── The opencode MCP block tolerates bad input ───────────────────────────────
awk '/python3 - "\$REPO_DIR\/mcps\/registry.json" "\$config_path" <</{flag=1;next} /^PYEOF$/{flag=0} flag' \
    "$ROOT/uninstall.sh" > "$TMP/mcp.py"
if [ ! -s "$TMP/mcp.py" ]; then
    ko "could not extract the opencode MCP block from uninstall.sh"
else
    printf '[{"name": "context7"}]' > "$TMP/mcp-registry.json"
    for cfg in '{not json' '[]' '{"mcp":"x"}' '{"mcp":[]}'; do
        printf '%s' "$cfg" > "$TMP/mcp-config.json"
        if python3 "$TMP/mcp.py" "$TMP/mcp-registry.json" "$TMP/mcp-config.json" >/dev/null 2>&1 \
            && [ "$(cat "$TMP/mcp-config.json")" = "$cfg" ]; then
            ok
        else
            ko "MCP block exits 0 and leaves config.json alone: $cfg"
        fi
    done
    printf '%s' '{"theme":"x","mcp":{"context7":{},"mine":{}}}' > "$TMP/mcp-config.json"
    python3 "$TMP/mcp.py" "$TMP/mcp-registry.json" "$TMP/mcp-config.json" >/dev/null 2>&1
    got="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(c["theme"], sorted(c["mcp"]))' "$TMP/mcp-config.json" 2>&1)"
    if [ "$got" = "x ['mine']" ]; then ok; else ko "MCP block removes only registry MCPs" "$got"; fi
fi

# ── Harness: uninstall.sh in a temp HOME with stub devexp binaries ───────────
# Each case gets $E with a repo copy ($E/r), HOME ($E/h), a PATH dir ($E/bin)
# and a call log ($E/calls). Stubs log every real call (not the help probe)
# with the DEVEXP_DIR they were given.
envs=0
new_env() {
    envs=$((envs+1))
    E="$TMP/env$envs"
    mkdir -p "$E/r/agents" "$E/r/skills" "$E/r/mcps" "$E/h/.config/opencode/plugins" "$E/bin" "$E/stubs"
    cp "$ROOT/uninstall.sh" "$E/r/uninstall.sh"
    printf '[]' > "$E/r/mcps/registry.json"
    : > "$E/calls"
}

# make_stub <path> <tag>: a devexp that has `uninstall --target opencode`.
# Its help puts the target first and then more than a pipe buffer of text, so
# a probe piping it into `grep -q` sees it die of SIGPIPE (141 under pipefail).
make_stub() {
    mkdir -p "$(dirname "$1")"
    cat > "$1" <<STUB
#!/bin/bash
if [ "\$*" = "uninstall --help" ]; then
    echo "      --target string   CLI to remove devexp from (supported: opencode)"
    for i in \$(seq 4000); do printf '%s\\n' "help text help text help text help text help text help text help text help text"; done
    exit 0
fi
echo "STUB[$2] \$*"
echo "$2 \$* DEVEXP_DIR=\$DEVEXP_DIR" >> "\$CALLS"
case "\$*" in *--dry-run*) exit 0 ;; esac
exit "\${STUB_RC:-0}"
STUB
    chmod +x "$1"
}

# make_old_stub <path>: a devexp built before the command existed. Its output
# mentions opencode, but not as a supported uninstall target.
make_old_stub() {
    mkdir -p "$(dirname "$1")"
    cat > "$1" <<'STUB'
#!/bin/bash
echo "old $*" >> "$CALLS.old"
echo "DevExp Framework — agents, skills, hooks, and MCPs for Claude Code & opencode"
echo "Error: unknown command \"uninstall\" for \"devexp\"" >&2
exit 1
STUB
    chmod +x "$1"
}

# run_uninstall [VAR=value ...]: uninstall.sh --yes with only these variables.
run_uninstall() {
    env -i HOME="$E/h" PATH="$E/bin:/usr/bin:/bin" CALLS="$E/calls" "$@" \
        /bin/bash "$E/r/uninstall.sh" --yes </dev/null > "$E/out" 2>&1
    echo $? > "$E/rc"
}

rc_is()        { [ "$(cat "$E/rc")" = "$1" ]; }
out_has()      { grep -qF -- "$1" "$E/out"; }
out_lacks()    { ! grep -qF -- "$1" "$E/out"; }
calls_are()    { [ "$(cat "$E/calls")" = "$(printf '%s' "$1")" ]; }
tagged_calls() { # $1=tag
    printf '%s uninstall --target opencode --dry-run DEVEXP_DIR=%s\n%s uninstall --target opencode --yes DEVEXP_DIR=%s' "$1" "$E/r" "$1" "$E/r"
}

# ── Detection ────────────────────────────────────────────────────────────────
for footprint in devexp.js devexp devexp-plugin.js dangling-devexp.js; do
    new_env
    P="$E/h/.config/opencode/plugins"
    case "$footprint" in
        devexp)             mkdir "$P/devexp" ;;
        dangling-devexp.js) ln -s "$E/nowhere" "$P/devexp.js" ;;
        *)                  printf '/**\n * x\n */\n' > "$P/$footprint" ;;
    esac
    make_stub "$E/stubs/a" A
    run_uninstall DEVEXP_BIN="$E/stubs/a"
    check "plugin-only install ($footprint) exits 0" rc_is 0
    check "plugin-only install ($footprint) is detected" out_has "devexp is installed for opencode"
    check "plugin-only install ($footprint) previews, then removes, through the binary with DEVEXP_DIR" calls_are "$(tagged_calls A)"
    check "plugin-only install ($footprint): the script itself removes no plugin file (the stub removes nothing)" \
        test -e "$P/${footprint#dangling-}" -o -L "$P/${footprint#dangling-}"
done

new_env
make_stub "$E/stubs/a" A
run_uninstall DEVEXP_BIN="$E/stubs/a"
check "no agents and no plugin: nothing to remove" out_has "Nothing to remove"
check "no agents and no plugin: the binary is not called" calls_are ""

# ── Binary lookup: DEVEXP_BIN, then bin/devexp, then PATH ────────────────────
new_env
touch "$E/h/.config/opencode/plugins/devexp.js"
make_stub "$E/stubs/a" A; make_stub "$E/r/bin/devexp" B; make_stub "$E/bin/devexp" C
run_uninstall DEVEXP_BIN="$E/stubs/a"
check "DEVEXP_BIN wins over bin/devexp and PATH" calls_are "$(tagged_calls A)"
: > "$E/calls"; run_uninstall
check "bin/devexp wins over PATH" calls_are "$(tagged_calls B)"
: > "$E/calls"; run_uninstall DEVEXP_BIN="$E/stubs/missing"
check "a DEVEXP_BIN that doesn't exist falls through" calls_are "$(tagged_calls B)"
mv "$E/r/bin/devexp" "$E/stubs/b-moved"
: > "$E/calls"; run_uninstall
check "devexp on PATH is used last" calls_are "$(tagged_calls C)"
make_old_stub "$E/stubs/old"
: > "$E/calls"; run_uninstall DEVEXP_BIN="$E/stubs/old"
check "a DEVEXP_BIN without the command falls through to the next binary" calls_are "$(tagged_calls C)"

# ── No usable binary ─────────────────────────────────────────────────────────
for setup in none old; do
    new_env
    printf 'devexp plugin\n' > "$E/h/.config/opencode/plugins/devexp.js"
    extra=()
    if [ "$setup" = old ]; then
        make_old_stub "$E/stubs/old"; make_old_stub "$E/r/bin/devexp"; make_old_stub "$E/bin/devexp"
        extra=(DEVEXP_BIN="$E/stubs/old")
    fi
    run_uninstall "${extra[@]+"${extra[@]}"}"
    check "no usable binary ($setup): exit 0" rc_is 0
    check "no usable binary ($setup): says so with the rebuild hint" out_has "no devexp binary with 'uninstall' found (set DEVEXP_BIN, or rebuild: rm bin/devexp && ./install.sh)"
    check "no usable binary ($setup): plugin left in place" test "$(cat "$E/h/.config/opencode/plugins/devexp.js")" = "devexp plugin"
    check "no usable binary ($setup): only help probes were made" \
        test -z "$(grep -v '^old uninstall --help$' "$E/calls.old" 2>/dev/null)"
done

# ── The real run fails ───────────────────────────────────────────────────────
new_env
touch "$E/h/.config/opencode/plugins/devexp.js"
make_stub "$E/stubs/a" A
run_uninstall DEVEXP_BIN="$E/stubs/a" STUB_RC=1
check "a failing plugin removal still exits 0" rc_is 0
check "a failing plugin removal is reported with a re-run hint" out_has "opencode plugin left in place (see above) — re-run: $E/stubs/a uninstall --target opencode"

# ── Order: plugin removal before the MCP block rewrites config.json ──────────
new_env
printf '# agent\n' > "$E/r/agents/some-agent.md"
mkdir -p "$E/h/.config/opencode/agents"
printf '# agent\n' > "$E/h/.config/opencode/agents/some-agent.md"
printf '{"mcp":{}}' > "$E/h/.config/opencode/config.json"
make_stub "$E/stubs/a" A
run_uninstall DEVEXP_BIN="$E/stubs/a"
plugin_line="$(grep -nF 'STUB[A] uninstall --target opencode --yes' "$E/out" | head -1 | cut -d: -f1)"
mcp_line="$(grep -nF 'Removing MCP servers (opencode)' "$E/out" | head -1 | cut -d: -f1)"
check "agents install: exit 0" rc_is 0
check "agents install: agents removed" test ! -e "$E/h/.config/opencode/agents/some-agent.md"
check "the plugin is removed before the MCP block runs" test -n "$plugin_line" -a -n "$mcp_line" -a "${plugin_line:-0}" -lt "${mcp_line:-0}"

# ── Crash regressions ────────────────────────────────────────────────────────
new_env
touch "$E/h/.config/opencode/plugins/devexp.js"
printf '{not json' > "$E/h/.config/opencode/config.json"
make_stub "$E/stubs/a" A
run_uninstall DEVEXP_BIN="$E/stubs/a"
check "malformed config.json: exit 0" rc_is 0
check "malformed config.json: no top-level local error" out_lacks "can only be used in a function"
check "malformed config.json: no python traceback" out_lacks "Traceback"
check "malformed config.json: left as it was" test "$(cat "$E/h/.config/opencode/config.json")" = "{not json"
check "malformed config.json: the run reaches the end" out_has "Uninstall complete."

# ── Round trip with the real binary ──────────────────────────────────────────
if command -v go >/dev/null 2>&1 && [ -d "$ROOT/cli/internal/assets/hooks" ]; then
    new_env
    R="$E/repo"; H="$E/h"; OC="$H/.config/opencode"; P="$OC/plugins"
    mkdir -p "$R"
    cp -R "$ROOT/agents" "$ROOT/skills" "$ROOT/hooks" "$ROOT/mcps" "$ROOT/devexp.config.json" "$ROOT/uninstall.sh" "$R/"
    printf '#!/bin/sh\nexit 0\n' > "$E/bin/opencode"; chmod +x "$E/bin/opencode"
    if (cd "$ROOT/cli" && go build -o "$E/devexp" .) > "$E/out" 2>&1 \
        && env -i HOME="$H" PATH="$E/bin:/usr/bin:/bin" DEVEXP_DIR="$R" "$E/devexp" install --reinstall-mcps > "$E/out" 2>&1; then
        cp "$ROOT/cli/internal/hooks/testdata/legacy-opencode/"* "$P/"
        printf 'export const mine = async () => ({})\n' > "$P/my-plugin.js"
        python3 - "$OC/config.json" "$P" <<'PY'
import json, sys
p = sys.argv[1]
c = json.load(open(p))
c["theme"] = "x"
c["plugin"] = [sys.argv[2] + "/devexp-plugin.js", "npm-plugin"]
json.dump(c, open(p, "w"))
PY
        env -i HOME="$H" PATH="$E/bin:/usr/bin:/bin" DEVEXP_BIN="$E/devexp" \
            /bin/bash "$R/uninstall.sh" --yes </dev/null > "$E/out" 2>&1
        echo $? > "$E/rc"
        check "round trip: exit 0" rc_is 0
        check "round trip: only the foreign plugin is left" test "$(ls -A "$P")" = "my-plugin.js"
        check "round trip: config.json keeps its own plugin and theme" test \
            "$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(c.get("theme"), c.get("plugin"))' "$OC/config.json")" = "x ['npm-plugin']"
        check "round trip: the manifest no longer records plugins" test \
            "$(python3 -c 'import json,sys; print("plugins" in json.load(open(sys.argv[1])))' "$OC/.devexp-manifest.json")" = "False"
    else
        ko "round trip: build or install failed" "$(cat "$E/out")"
    fi
else
    echo "SKIP round trip with the real binary (needs go and ./scripts/stage-assets.sh)"
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
