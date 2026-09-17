#!/usr/bin/env bash
set -euo pipefail

# ── Colors ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
RESET='\033[0m'

# ── Helpers ───────────────────────────────────────────────────────────────────
info()    { echo -e "${BLUE}[devexp]${RESET} $*"; }
success() { echo -e "${GREEN}[devexp]${RESET} $*"; }
warn()    { echo -e "${YELLOW}[devexp]${RESET} $*"; }
error()   { echo -e "${RED}[devexp] ERROR:${RESET} $*" >&2; }
die()     { error "$*"; exit 1; }
# Prints the first devexp binary whose `uninstall --help` lists target $1:
# DEVEXP_BIN (set by `devexp install` → Remove), then bin/devexp, then PATH.
# The help text is captured, not piped into `grep -q`: under pipefail an early
# grep exit can SIGPIPE the binary and reject a good one. A binary without the
# command (built before it existed) fails the probe and the next one is tried.
find_devexp_bin() { for c in "${DEVEXP_BIN:-}" "$REPO_DIR/bin/devexp" "$(command -v devexp 2>/dev/null || true)"; do [[ -n "$c" && -x "$c" && "$("$c" uninstall --help 2>/dev/null </dev/null || true)" == *"supported: "*"$1"* ]] && { echo "$c"; return 0; }; done; return 1; }

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── HOME ──────────────────────────────────────────────────────────────────────
# Every path below is built from HOME. Unset, empty or relative, it would point
# at / or at the current directory (a dotfiles checkout, say), so refuse before
# looking at anything — the same rule `devexp install` / `devexp uninstall`
# apply (#126).
[[ "${HOME:-}" == /* ]] || die "HOME is \"${HOME:-}\", not an absolute path — refusing to remove anything; set HOME and re-run"

[[ -d "$REPO_DIR/agents" ]] || die "agents/ directory not found. Is this the devexp repo?"
[[ -d "$REPO_DIR/skills" ]] || die "skills/ directory not found. Is this the devexp repo?"

# ── Paths ─────────────────────────────────────────────────────────────────────
CLAUDE_AGENTS="$HOME/.claude/agents"
OPENCODE_AGENTS="$HOME/.config/opencode/agents"
OPENCODE_PLUGINS="$HOME/.config/opencode/plugins"
SKILLS_DIR="$HOME/.claude/skills"   # shared between both CLIs

# ── Detect what's installed ───────────────────────────────────────────────────
HAS_CLAUDE_INSTALL=false
HAS_OPENCODE_INSTALL=false

for f in "$REPO_DIR/agents/"*.md; do
    [[ -f "$f" ]] || continue
    [[ -f "$CLAUDE_AGENTS/$(basename "$f")"   ]] && HAS_CLAUDE_INSTALL=true
    [[ -f "$OPENCODE_AGENTS/$(basename "$f")" ]] && HAS_OPENCODE_INSTALL=true
done
# The hook plugin alone is an install too: every agent can be deselected.
# devexp-plugin.js is the legacy flat entry.
for p in "$OPENCODE_PLUGINS/devexp.js" "$OPENCODE_PLUGINS/devexp" "$OPENCODE_PLUGINS/devexp-plugin.js"; do
    [[ -e "$p" || -L "$p" ]] && HAS_OPENCODE_INSTALL=true
done

echo ""
echo -e "${BOLD}devexp Framework Uninstaller${RESET}"
echo "────────────────────────────────────────"
echo ""

if ! $HAS_CLAUDE_INSTALL && ! $HAS_OPENCODE_INSTALL; then
    info "Nothing to remove — no devexp agents or hook plugin found in Claude Code or opencode directories."
    exit 0
fi

# ── Determine what to remove ──────────────────────────────────────────────────
REMOVE_CLAUDE=false
REMOVE_OPENCODE=false

if $HAS_CLAUDE_INSTALL && $HAS_OPENCODE_INSTALL; then
    warn "devexp is installed for both Claude Code and opencode."
    echo ""
    echo "  [1] Claude Code only"
    echo "  [2] opencode only"
    echo "  [3] Both"
    echo ""
    read -r -p "Remove from which CLI? [1/2/3]: " choice
    case "$choice" in
        1) REMOVE_CLAUDE=true ;;
        2) REMOVE_OPENCODE=true ;;
        3) REMOVE_CLAUDE=true; REMOVE_OPENCODE=true ;;
        *) die "Invalid choice." ;;
    esac
elif $HAS_CLAUDE_INSTALL; then
    warn "devexp is installed for Claude Code."
    REMOVE_CLAUDE=true
elif $HAS_OPENCODE_INSTALL; then
    warn "devexp is installed for opencode."
    REMOVE_OPENCODE=true
fi

echo ""

# ── Collect what will be removed ──────────────────────────────────────────────
AGENT_FILES_CLAUDE=()
AGENT_FILES_OPENCODE=()
SKILL_DIRS=()

if $REMOVE_CLAUDE; then
    for f in "$REPO_DIR/agents/"*.md; do
        [[ -f "$f" ]] || continue
        [[ "$(basename "$f")" == "README.md" ]] && continue
        t="$CLAUDE_AGENTS/$(basename "$f")"
        [[ -f "$t" ]] && AGENT_FILES_CLAUDE+=("$t")
    done
fi

if $REMOVE_OPENCODE; then
    # Shared agents (transformed)
    for f in "$REPO_DIR/agents/"*.md; do
        [[ -f "$f" ]] || continue
        [[ "$(basename "$f")" == "README.md" ]] && continue
        t="$OPENCODE_AGENTS/$(basename "$f")"
        [[ -f "$t" ]] && AGENT_FILES_OPENCODE+=("$t")
    done
    # opencode-exclusive agents
    for f in "$REPO_DIR/agents/opencode/"*.md; do
        [[ -f "$f" ]] || continue
        t="$OPENCODE_AGENTS/$(basename "$f")"
        [[ -f "$t" ]] && AGENT_FILES_OPENCODE+=("$t")
    done
fi

# Skills are shared — only remove if uninstalling from all installed CLIs
REMOVE_SKILLS=false
if $REMOVE_CLAUDE && $REMOVE_OPENCODE; then
    REMOVE_SKILLS=true
elif $REMOVE_CLAUDE && ! $HAS_OPENCODE_INSTALL; then
    REMOVE_SKILLS=true
elif $REMOVE_OPENCODE && ! $HAS_CLAUDE_INSTALL; then
    REMOVE_SKILLS=true
fi

if $REMOVE_SKILLS; then
    for d in "$REPO_DIR/skills/"/*/; do
        [[ -d "$d" ]] || continue
        t="$SKILLS_DIR/$(basename "$d")"
        [[ -d "$t" ]] && SKILL_DIRS+=("$t")
    done
fi

# ── Preview ───────────────────────────────────────────────────────────────────
if [[ ${#AGENT_FILES_CLAUDE[@]} -gt 0 ]]; then
    info "Claude Code agents to remove (${#AGENT_FILES_CLAUDE[@]}):"
    for f in "${AGENT_FILES_CLAUDE[@]}"; do
        echo -e "  ${RED}-${RESET} $(basename "$f")"
    done
    echo ""
fi

if [[ ${#AGENT_FILES_OPENCODE[@]} -gt 0 ]]; then
    info "opencode agents to remove (${#AGENT_FILES_OPENCODE[@]}):"
    for f in "${AGENT_FILES_OPENCODE[@]}"; do
        echo -e "  ${RED}-${RESET} $(basename "$f")"
    done
    echo ""
fi

if [[ ${#SKILL_DIRS[@]} -gt 0 ]]; then
    info "Skill directories to remove (${#SKILL_DIRS[@]}) from $SKILLS_DIR:"
    for d in "${SKILL_DIRS[@]}"; do
        echo -e "  ${RED}-${RESET} $(basename "$d")/"
    done
    echo ""
elif $REMOVE_CLAUDE || $REMOVE_OPENCODE; then
    info "Skills will be kept (still in use by other installed CLI)."
    echo ""
fi

# The opencode hook plugin is removed by the devexp binary, by the installer's
# own rules; this script never deletes plugin files itself.
OPENCODE_BIN=""
if $REMOVE_OPENCODE; then
    if OPENCODE_BIN="$(find_devexp_bin opencode)"; then
        info "opencode hook plugin (preview):"
        DEVEXP_DIR="$REPO_DIR" "$OPENCODE_BIN" uninstall --target opencode --dry-run </dev/null \
            || warn "could not preview the opencode plugin removal"
        echo ""
    else
        warn "opencode hook plugin: no devexp binary with 'uninstall' found (set DEVEXP_BIN, or rebuild: rm bin/devexp && ./install.sh). The plugin will be left in place."
        echo ""
    fi
fi

# ── Confirm ───────────────────────────────────────────────────────────────────
if [[ "${1:-}" != "--yes" && "${1:-}" != "-y" ]]; then
    read -r -p "Proceed with removal? [y/N] " confirm
    case "$confirm" in
        [yY][eE][sS]|[yY]) ;;
        *) info "Aborted."; exit 0 ;;
    esac
    echo ""
fi

# ── Remove ────────────────────────────────────────────────────────────────────
removed=0

if [[ ${#AGENT_FILES_CLAUDE[@]} -gt 0 ]]; then
    info "Removing Claude Code agents..."
    for f in "${AGENT_FILES_CLAUDE[@]}"; do
        rm -f "$f" && echo -e "  ${RED}-${RESET} $(basename "$f")"
        (( removed++ )) || true
    done
    echo ""
fi

if [[ ${#AGENT_FILES_OPENCODE[@]} -gt 0 ]]; then
    info "Removing opencode agents..."
    for f in "${AGENT_FILES_OPENCODE[@]}"; do
        rm -f "$f" && echo -e "  ${RED}-${RESET} $(basename "$f")"
        (( removed++ )) || true
    done
    echo ""
fi

if [[ ${#SKILL_DIRS[@]} -gt 0 ]]; then
    info "Removing skills..."
    for d in "${SKILL_DIRS[@]}"; do
        rm -rf "$d" && echo -e "  ${RED}-${RESET} $(basename "$d")/"
        (( removed++ )) || true
    done
    echo ""
fi

# ── Remove hooks (opencode plugin) ────────────────────────────────────────────
# Before the MCP block below: it rewrites config.json, and the legacy plugin
# entry is spliced out of config.json's original bytes.
if $REMOVE_OPENCODE && [[ -n "$OPENCODE_BIN" ]]; then
    DEVEXP_DIR="$REPO_DIR" "$OPENCODE_BIN" uninstall --target opencode --yes </dev/null \
        || warn "opencode plugin left in place (see above) — re-run: $OPENCODE_BIN uninstall --target opencode"
    echo ""
fi

# ── Remove MCPs ───────────────────────────────────────────────────────────────
if $REMOVE_CLAUDE && command -v claude &>/dev/null && [[ -f "$REPO_DIR/mcps/registry.json" ]]; then
    info "Removing MCP servers (Claude Code)..."
    python3 - "$REPO_DIR/mcps/registry.json" <<'PYEOF'
import json, sys, subprocess
with open(sys.argv[1]) as f:
    mcps = json.load(f)
for mcp in mcps:
    name = mcp['name']
    result = subprocess.run(['claude', 'mcp', 'list'], capture_output=True, text=True)
    if name not in result.stdout:
        print(f"  [skip] {name} — not installed")
        continue
    r = subprocess.run(['claude', 'mcp', 'remove', name], capture_output=True, text=True)
    if r.returncode == 0:
        print(f"  \033[0;31m-\033[0m {name}")
    else:
        print(f"  [warn] {name} — {r.stderr.strip()}")
PYEOF
    echo ""
fi

if $REMOVE_OPENCODE && [[ -f "$REPO_DIR/mcps/registry.json" ]]; then
    config_path="$HOME/.config/opencode/config.json"
    if [[ -f "$config_path" ]]; then
        info "Removing MCP servers (opencode)..."
        python3 - "$REPO_DIR/mcps/registry.json" "$config_path" <<'PYEOF' || warn "opencode MCP servers: the config.json step failed (see above) — remove the devexp MCP servers from $config_path by hand"
import json, os, stat, sys, tempfile
config_path = sys.argv[2]

class WriteRefused(Exception):
    pass

def write_atomic(path, data):
    # Replace the file at path with the bytes data, as fsutil.WriteFileAtomic
    # does (cli/internal/fsutil/atomic.go; uninstall.test.sh keeps every copy of
    # this function identical). A symlink is followed to the file it points at,
    # which is replaced while the link stays. A dangling link, a target that
    # isn't a regular file or isn't writable is refused. The new bytes go to a
    # temp file next to the target, fsync'd and given its mode (a new file gets
    # 0644 minus the umask), then renamed over it: an interrupted save leaves
    # the old file or the new one, never a partial one, and no temp file.
    # Replacing makes a new inode: hard links, xattrs, ACLs and setuid, setgid
    # and sticky bits don't carry over. Raises WriteRefused with a message.
    target = path
    if os.path.islink(path):
        try:
            os.stat(path)
        except FileNotFoundError:
            raise WriteRefused(f"{path} is a symlink to a file that does not exist, so it was left untouched")
        except OSError as e:
            raise WriteRefused(f"{path} is a symlink that can't be resolved, so it was left untouched: {e}")
        target = os.path.realpath(path)
    try:
        st = os.stat(target)
    except FileNotFoundError:
        st = None
    except OSError as e:
        raise WriteRefused(f"could not save {target}, so it was left untouched: {e}")
    mode = None
    if st is not None:
        if not stat.S_ISREG(st.st_mode):
            raise WriteRefused(f"{target} is not a regular file, so it was left untouched")
        if not os.access(target, os.W_OK):
            raise WriteRefused(f"{target} is not writable, so it was left untouched")
        mode = st.st_mode & 0o777
    directory = os.path.dirname(target) or '.'
    tmp = None
    try:
        if mode is None:
            # A new file: created with 0644 and never chmodded, so the umask
            # applies (0600 under umask 077), as with a plain open().
            while True:
                tmp = os.path.join(directory, '.' + os.path.basename(target) + '.tmp-' + os.urandom(8).hex())
                try:
                    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o644)
                    break
                except FileExistsError:
                    tmp = None
        else:
            fd, tmp = tempfile.mkstemp(prefix='.' + os.path.basename(target) + '.tmp-', dir=directory)
        with os.fdopen(fd, 'wb') as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        if mode is not None:
            os.chmod(tmp, mode)
        os.replace(tmp, target)
        tmp = None
    except OSError as e:
        raise WriteRefused(f"could not save {target}, so it was left untouched: {e}")
    finally:
        if tmp is not None:
            try:
                os.unlink(tmp)
            except OSError:
                pass
    try:
        dfd = os.open(directory, os.O_RDONLY)
        try:
            os.fsync(dfd)
        finally:
            os.close(dfd)
    except OSError:
        pass

# Like the plugin step (removeLegacyConfigEntry), a symlinked config.json is
# someone's dotfiles setup: never written through, never replaced.
if os.path.islink(config_path):
    print(f"  [skip] {config_path} is a symlink, so it was left untouched — remove the devexp MCP servers from it by hand")
    sys.exit(0)

def parse(text):
    # NaN and Infinity compare unequal to themselves; read them as markers so
    # the check below compares like with like. Nothing is ever re-encoded.
    return json.loads(text, parse_constant=lambda name: ('constant', name))

try:
    with open(sys.argv[1]) as f:
        mcps = json.load(f)
    # newline='' keeps line endings exactly as they are.
    with open(config_path, encoding='utf-8', newline='') as f:
        text = f.read()
    config = parse(text)
except RecursionError:
    print(f"  [skip] {config_path} is nested too deeply to edit, so it was left untouched — remove the devexp MCP servers from it by hand")
    sys.exit(0)
except (OSError, ValueError) as e:
    print(f"  [skip] {config_path}: {e}")
    sys.exit(0)
if not isinstance(config, dict) or not isinstance(config.get('mcp', {}), dict) or not isinstance(mcps, list):
    print(f"  [skip] {config_path}: unexpected shape")
    sys.exit(0)

names = []
for mcp in mcps:
    name = mcp.get('name') if isinstance(mcp, dict) else None
    if not isinstance(name, str) or name in names:
        continue
    if name in config.get('mcp', {}):
        names.append(name)
    else:
        print(f"  [skip] {name} — not configured")
if not names:
    sys.exit(0)

def ws(text, i):
    while i < len(text) and text[i] in ' \t\n\r':
        i += 1
    return i

def object_members(text, i):
    # The members of the object whose '{' is at text[i], as (key, key start,
    # value start, value end), and the index of its closing '}'.
    decoder = json.JSONDecoder(parse_constant=lambda name: None)
    members = []
    i = ws(text, i + 1)
    if text[i] == '}':
        return members, i
    while True:
        key_start = i
        key, i = json.decoder.scanstring(text, i + 1)
        i = ws(text, ws(text, i) + 1)  # past ':'
        value_start = i
        _, end = decoder.raw_decode(text, i)
        members.append((key, key_start, value_start, end))
        i = ws(text, end)
        if text[i] != ',':
            return members, i
        i = ws(text, i + 1)

def splice_out(text, name):
    # text with every member called name removed from the "mcp" object that
    # json.loads reads (the last "mcp" key), cutting only that member and the
    # comma and whitespace that joined it to a neighbour: every other byte of
    # config.json stays as it was.
    while True:
        top, _ = object_members(text, ws(text, 0))
        mcp_start = [m for m in top if m[0] == 'mcp'][-1][2]
        members, close = object_members(text, mcp_start)
        found = [k for k, m in enumerate(members) if m[0] == name]
        if not found:
            return text
        k = found[-1]
        if len(members) == 1:
            text = text[:mcp_start + 1] + text[close:]
        elif k > 0:
            text = text[:members[k - 1][3]] + text[members[k][3]:]
        else:
            text = text[:members[0][1]] + text[members[1][1]:]

new_text = text
expected = dict(config)
expected['mcp'] = dict(config['mcp'])
try:
    for name in names:
        new_text = splice_out(new_text, name)
        del expected['mcp'][name]
    # By construction new_text is text minus those members, so this can't fail
    # unless the splice has a bug: it is a guard.
    same = parse(new_text) == expected
except (ValueError, IndexError, RecursionError):
    same = False
if not same:
    print(f"  [skip] {config_path}: removing the MCP servers would change more than them, so it was left untouched — remove them by hand")
    sys.exit(0)
try:
    write_atomic(config_path, new_text.encode('utf-8'))
except WriteRefused as e:
    print(f"  [warn] {e} — remove the devexp MCP servers from it by hand")
    sys.exit(0)
for name in names:
    print(f"  \033[0;31m-\033[0m {name}")
print(f"  Saved: {config_path}")
PYEOF
        echo ""
    fi
fi

# ── Remove hooks (Claude Code) ────────────────────────────────────────────────
if $REMOVE_CLAUDE; then
    settings_path="$HOME/.claude/settings.json"
    if [[ -f "$settings_path" && -f "$REPO_DIR/hooks/registry.json" ]]; then
        info "Removing hooks (Claude Code)..."
        python3 - "$REPO_DIR" "$settings_path" <<'PYEOF' || warn "Claude Code hooks: the settings.json step failed (see above) — remove the devexp hooks from $settings_path by hand"
import json, math, os, stat, sys, tempfile

class WriteRefused(Exception):
    pass

def write_atomic(path, data):
    # Replace the file at path with the bytes data, as fsutil.WriteFileAtomic
    # does (cli/internal/fsutil/atomic.go; uninstall.test.sh keeps every copy of
    # this function identical). A symlink is followed to the file it points at,
    # which is replaced while the link stays. A dangling link, a target that
    # isn't a regular file or isn't writable is refused. The new bytes go to a
    # temp file next to the target, fsync'd and given its mode (a new file gets
    # 0644 minus the umask), then renamed over it: an interrupted save leaves
    # the old file or the new one, never a partial one, and no temp file.
    # Replacing makes a new inode: hard links, xattrs, ACLs and setuid, setgid
    # and sticky bits don't carry over. Raises WriteRefused with a message.
    target = path
    if os.path.islink(path):
        try:
            os.stat(path)
        except FileNotFoundError:
            raise WriteRefused(f"{path} is a symlink to a file that does not exist, so it was left untouched")
        except OSError as e:
            raise WriteRefused(f"{path} is a symlink that can't be resolved, so it was left untouched: {e}")
        target = os.path.realpath(path)
    try:
        st = os.stat(target)
    except FileNotFoundError:
        st = None
    except OSError as e:
        raise WriteRefused(f"could not save {target}, so it was left untouched: {e}")
    mode = None
    if st is not None:
        if not stat.S_ISREG(st.st_mode):
            raise WriteRefused(f"{target} is not a regular file, so it was left untouched")
        if not os.access(target, os.W_OK):
            raise WriteRefused(f"{target} is not writable, so it was left untouched")
        mode = st.st_mode & 0o777
    directory = os.path.dirname(target) or '.'
    tmp = None
    try:
        if mode is None:
            # A new file: created with 0644 and never chmodded, so the umask
            # applies (0600 under umask 077), as with a plain open().
            while True:
                tmp = os.path.join(directory, '.' + os.path.basename(target) + '.tmp-' + os.urandom(8).hex())
                try:
                    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o644)
                    break
                except FileExistsError:
                    tmp = None
        else:
            fd, tmp = tempfile.mkstemp(prefix='.' + os.path.basename(target) + '.tmp-', dir=directory)
        with os.fdopen(fd, 'wb') as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        if mode is not None:
            os.chmod(tmp, mode)
        os.replace(tmp, target)
        tmp = None
    except OSError as e:
        raise WriteRefused(f"could not save {target}, so it was left untouched: {e}")
    finally:
        if tmp is not None:
            try:
                os.unlink(tmp)
            except OSError:
                pass
    try:
        dfd = os.open(directory, os.O_RDONLY)
        try:
            os.fsync(dfd)
        finally:
            os.close(dfd)
    except OSError:
        pass

repo_dir      = sys.argv[1]
settings_path = sys.argv[2]

# Identify devexp hooks by what the registry says they are, not by where they
# happen to live. Matching only on repo_dir left behind any registration made
# by an earlier release-binary install, which then ran forever from a stale
# cache -- see issue #93. Disabled hooks are included: their foreign-root
# copies must go too.
SCRIPT_DIR = 'hooks/claude-code/'
# A path with any of these is more than a plain path (arguments, env
# assignments, expansions, quoting), so devexp registers it single-quoted
# (#135). Same rules as commandPath/isManagedScriptPath in cli/internal/hooks.
SHELL_SYNTAX = set(' \t\n\r$~\'"`\\;&|<>()*?[]{}!#')
try:
    with open(os.path.join(repo_dir, 'hooks', 'registry.json')) as f:
        scripts = [
            h['claude_code']['script']
            for h in json.load(f)
            if h.get('claude_code', {}).get('script')
        ]
        managed = {os.path.basename(s) for s in scripts}
        # Other spellings of this repo's own scripts: the joined path verbatim,
        # as an install wrote it before paths were quoted (the shell splits it
        # when it needs quoting), and that path in double quotes, the natural
        # hand fix, when it has no $, backquote, \ or " (so the quotes are
        # literal). Same rules as requoteDevexpHooks in cli/internal/hooks.
        own = [os.path.normpath(os.path.join(repo_dir, s)) for s in scripts]
        legacy = set(own) | {
            '"' + p + '"' for p in own if not any(c in p for c in '$`\\"')
        }
except (OSError, ValueError, KeyError, TypeError, AttributeError, RecursionError):
    print("  [skip] could not read hooks/registry.json")
    sys.exit(0)

def shell_quote(s):
    return "'" + s.replace("'", "'\\''") + "'"

def command_path(cmd):
    # The path cmd runs, if cmd is a form devexp writes: a plain path with no
    # shell syntax, or exactly one single-quoted absolute path that re-quotes
    # to itself. Anything else (double quotes, arguments, '/a'b) is the user's.
    if not cmd:
        return None
    if not any(c in SHELL_SYNTAX for c in cmd):
        return cmd
    if len(cmd) < 2 or cmd[0] != "'" or cmd[-1] != "'":
        return None
    p = cmd[1:-1].replace("'\\''", "'")
    if not p.startswith('/') or shell_quote(p) != cmd:
        return None
    return p

def is_devexp_root(root):
    # Whether root holds a devexp hooks registry: hooks/registry.json, a regular
    # file holding a non-empty JSON array of objects, each with a non-empty
    # string "name". Same test as isDevexpRoot in cli/internal/hooks.
    p = os.path.join(root, 'hooks', 'registry.json')
    try:
        if not stat.S_ISREG(os.stat(p).st_mode):
            return False
        with open(p, encoding='utf-8') as f:
            reg = json.load(f)
    except (OSError, ValueError, RecursionError):
        return False
    return (isinstance(reg, list) and len(reg) > 0
            and all(isinstance(h, dict) and isinstance(h.get('name'), str) and h['name'] for h in reg))

roots = {}

def is_orphaned(p):
    # Whether p, the path a devexp-form command runs, is a script gone from
    # <root>/hooks/claude-code/ of a devexp install root: this repo, or another
    # checkout or asset cache devexp was installed from, whose registry has
    # since dropped the hook (#150). The name can't be checked against a
    # registry, so the root is. A user's hook directory has no registry; a root
    # deleted outright can't be told from a user's and is left alone. Same rules
    # as isStaleDevexpHook / isOrphanedDevexpHook in cli/internal/hooks.
    if not p.startswith('/') or os.path.normpath(p) != p:
        return False
    cut = p.rfind('/') + 1
    directory, script = p[:cut], p[cut:]
    if not script or not directory.endswith('/' + SCRIPT_DIR):
        return False
    root = directory[:-len('/' + SCRIPT_DIR)]
    if not root:
        return False
    try:
        os.stat(p)
        return False
    except FileNotFoundError:
        pass
    except OSError:
        return False
    if root not in roots:
        roots[root] = is_devexp_root(root)
    return roots[root]

def is_devexp_handler(h):
    # devexp registers only {"type": "command", "command": <path>}. A handler
    # with args is spawned without a shell (exec form), and one of another type
    # runs no command: both are the user's, whatever path they name (#137).
    return (isinstance(h, dict) and h.get('type') == 'command'
            and 'args' not in h and is_devexp_hook(h.get('command')))

def is_devexp_hook(cmd):
    # A devexp-form command whose basename is a registry script, with
    # hooks/claude-code/ directly above it at a path-segment boundary (so
    # my-hooks/claude-code/ is not devexp's), or this repo's legacy unquoted
    # registration, or one whose script is gone from a devexp install root.
    if not isinstance(cmd, str):
        return False
    if cmd in legacy:
        return True
    p = command_path(cmd)
    if p is None:
        return False
    if is_orphaned(p):
        return True
    base = os.path.basename(p)
    if base not in managed:
        return False
    head = p[:-len(base)]
    return head == SCRIPT_DIR or head.endswith('/' + SCRIPT_DIR)

# newline='' keeps line endings as they are: text mode would turn CRLF into LF
# across the whole file.
try:
    with open(settings_path, encoding='utf-8', newline='') as f:
        text = f.read()
except (OSError, ValueError) as e:
    print(f"  [skip] could not read settings.json: {e}")
    sys.exit(0)

class NotPortable(ValueError):
    pass

def reject_constant(name):
    raise NotPortable(name)

def finite_float(s):
    f = float(s)
    if math.isinf(f) or math.isnan(f):
        raise NotPortable(s)
    return f

def load(text):
    # Strict JSON, as Claude Code and devexp install read it: NaN and Infinity
    # are refused, and so is a number a float can't hold (1e400 reads as inf and
    # would be written back as Infinity, which isn't JSON).
    return json.loads(text, parse_constant=reject_constant, parse_float=finite_float)

try:
    settings = load(text)
except RecursionError:
    print("  [skip] settings.json is nested too deeply to edit, so it was left untouched -- remove the devexp hooks from it by hand")
    sys.exit(0)
except NotPortable as e:
    print(f"  [skip] settings.json holds {e}, which uninstall can't write back as JSON, so it was left untouched -- remove the devexp hooks from it by hand")
    sys.exit(0)
except ValueError:
    print("  [skip] settings.json is not valid JSON")
    sys.exit(0)

hooks_section = settings.get('hooks') if isinstance(settings, dict) else None
if not hooks_section:
    print("  [skip] no hooks configured")
    sys.exit(0)
if not isinstance(hooks_section, dict):
    print("  [skip] settings.json: unexpected shape")
    sys.exit(0)

# Remove devexp's handlers and nothing else: every other handler and entry
# keeps all its fields. An entry or event is dropped only when this emptied it;
# one that was already empty is the user's.
changed = False
for event, hook_list in list(hooks_section.items()):
    if not isinstance(hook_list, list):
        continue
    filtered = []
    for entry in hook_list:
        cmds = entry.get('hooks') if isinstance(entry, dict) else None
        if not isinstance(cmds, list) or not cmds:
            filtered.append(entry)
            continue
        # Filter per command, not by entry['hooks'][0]: an entry may hold more
        # than one, and judging it by its first silently mishandles the rest.
        kept_cmds = []
        for h in cmds:
            if is_devexp_handler(h):
                cmd = h['command']
                print(f"  \033[0;31m-\033[0m {event}: {os.path.basename(command_path(cmd) or cmd)}")
                changed = True
            else:
                kept_cmds.append(h)
        if kept_cmds:
            entry['hooks'] = kept_cmds
            filtered.append(entry)
    if filtered:
        hooks_section[event] = filtered
    elif hook_list:
        del hooks_section[event]

if not changed:
    print("  [skip] no devexp hooks found in settings.json")
    sys.exit(0)

def top_level_member(text, name):
    # (key start, value start, value end) of the last top-level member called
    # name -- the one json.loads keeps -- or None.
    decoder = json.JSONDecoder()
    def ws(i):
        while i < len(text) and text[i] in ' \t\n\r':
            i += 1
        return i
    found = None
    i = ws(0) + 1  # past '{'
    i = ws(i)
    if text[i] == '}':
        return None
    while True:
        key_start = i
        key, i = json.decoder.scanstring(text, i + 1)
        i = ws(ws(i) + 1)  # past ':'
        _, end = decoder.raw_decode(text, i)
        if key == name:
            found = (key_start, i, end)
        i = ws(end)
        if text[i] != ',':
            return found
        i = ws(i + 1)

# Only the hooks value is rewritten, in the layout of the line its key is on and
# with the file's own line ending (that of its first line); every other byte of
# settings.json stays as it was.
key_start, value_start, value_end = top_level_member(text, 'hooks')
lead = text[text.rfind('\n', 0, key_start) + 1:key_start]
first_eol = text.find('\n')
eol = '\r\n' if first_eol > 0 and text[first_eol - 1] == '\r' else '\n'
try:
    # allow_nan=False backs up load(): nothing it accepts is non-finite.
    if lead.strip(' \t'):
        value = json.dumps(hooks_section, ensure_ascii=False, allow_nan=False, separators=(',', ':'))
    else:
        value = json.dumps(hooks_section, ensure_ascii=False, allow_nan=False, indent=lead or '  ').replace('\n', eol + lead)
except (ValueError, RecursionError) as e:
    print(f"  [skip] settings.json: {e}, so it was left untouched")
    sys.exit(0)
new_text = text[:value_start] + value + text[value_end:]
# By construction new_text is text with only the hooks value replaced, so this
# can't fail unless top_level_member or the splice has a bug: it is a guard.
try:
    same = load(new_text) == settings
except (ValueError, RecursionError):
    same = False
if not same:
    print("  [skip] settings.json: rewriting it would change more than its hooks, so it was left untouched")
    sys.exit(0)
# Encoded here, so no text-mode newline translation applies. Atomic, and
# through a symlinked settings.json to the file it points at (#124).
try:
    write_atomic(settings_path, new_text.encode('utf-8'))
except WriteRefused as e:
    print(f"  [warn] {e} -- remove the devexp hooks from it by hand")
    sys.exit(0)
print(f"  Saved: {settings_path}")
PYEOF
        echo ""
    fi
fi

# ── Done ──────────────────────────────────────────────────────────────────────
success "Removed $removed item(s)."
echo ""
echo -e "${GREEN}${BOLD}Uninstall complete.${RESET}"
echo ""
echo "To reinstall at any time, run: ./install.sh"
echo ""
