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
        python3 - "$REPO_DIR/mcps/registry.json" "$config_path" <<'PYEOF'
import json, sys, os, tempfile
config_path = sys.argv[2]
# Like the plugin step (removeLegacyConfigEntry), a symlinked config.json is
# someone's dotfiles setup: never written through, never replaced.
if os.path.islink(config_path):
    print(f"  [skip] {config_path} is a symlink, so it was left untouched — remove the devexp MCP servers from it by hand")
    sys.exit(0)
try:
    with open(sys.argv[1]) as f:
        mcps = json.load(f)
    with open(config_path) as f:
        config = json.load(f)
except (OSError, ValueError) as e:
    print(f"  [skip] {config_path}: {e}")
    sys.exit(0)
if not isinstance(config, dict) or not isinstance(config.get('mcp', {}), dict) or not isinstance(mcps, list):
    print(f"  [skip] {config_path}: unexpected shape")
    sys.exit(0)
changed = False
for mcp in mcps:
    name = mcp.get('name') if isinstance(mcp, dict) else None
    if not isinstance(name, str):
        continue
    if name in config.get('mcp', {}):
        del config['mcp'][name]
        changed = True
        print(f"  \033[0;31m-\033[0m {name}")
    else:
        print(f"  [skip] {name} — not configured")
if not changed:
    sys.exit(0)
if not os.access(config_path, os.W_OK):
    print(f"  [warn] {config_path} is not writable, so it was left untouched — remove the devexp MCP servers from it by hand")
    sys.exit(0)
# Replace atomically: a temp file in the same directory, fsync'd, given the
# old file's mode, then renamed over it. An interrupted save leaves the old
# file or the new one, never a truncated one, and no temp file.
tmp = None
try:
    mode = os.stat(config_path).st_mode & 0o7777
    fd, tmp = tempfile.mkstemp(prefix='.config.json.tmp-', dir=os.path.dirname(config_path))
    with os.fdopen(fd, 'w') as f:
        json.dump(config, f, indent=2)
        f.flush()
        os.fsync(f.fileno())
    os.chmod(tmp, mode)
    os.replace(tmp, config_path)
    tmp = None
except OSError as e:
    print(f"  [warn] could not save {config_path}, so it was left untouched: {e}")
    sys.exit(0)
finally:
    if tmp is not None:
        try:
            os.unlink(tmp)
        except OSError:
            pass
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
        python3 - "$REPO_DIR" "$settings_path" <<'PYEOF'
import json, sys, os

repo_dir      = sys.argv[1]
settings_path = sys.argv[2]

# Identify devexp hooks by what the registry says they are, not by where they
# happen to live. Matching only on repo_dir left behind any registration made
# by an earlier release-binary install, which then ran forever from a stale
# cache -- see issue #93. Disabled hooks are included: their foreign-root
# copies must go too.
SCRIPT_DIR = 'hooks/claude-code/'
try:
    with open(os.path.join(repo_dir, 'hooks', 'registry.json')) as f:
        managed = {
            os.path.basename(h['claude_code']['script'])
            for h in json.load(f)
            if h.get('claude_code', {}).get('script')
        }
except (OSError, json.JSONDecodeError, KeyError):
    print("  [skip] could not read hooks/registry.json")
    sys.exit(0)

def is_devexp_hook(cmd):
    return bool(cmd) and os.path.basename(cmd) in managed and SCRIPT_DIR in cmd.replace('\\', '/')

with open(settings_path) as f:
    try:
        settings = json.load(f)
    except json.JSONDecodeError:
        print("  [skip] settings.json is not valid JSON")
        sys.exit(0)

hooks_section = settings.get('hooks', {})
if not hooks_section:
    print("  [skip] no hooks configured")
    sys.exit(0)

changed = False
for event, hook_list in list(hooks_section.items()):
    filtered = []
    for entry in hook_list:
        # Filter per command, not by entry['hooks'][0]: an entry may hold more
        # than one, and judging it by its first silently mishandles the rest.
        kept_cmds = []
        for h in entry.get('hooks', []):
            cmd = h.get('command', '')
            if is_devexp_hook(cmd):
                print(f"  \033[0;31m-\033[0m {event}: {os.path.basename(cmd)}")
                changed = True
            else:
                kept_cmds.append(h)
        if not entry.get('hooks'):
            filtered.append(entry)
        elif kept_cmds:
            entry['hooks'] = kept_cmds
            filtered.append(entry)
    hooks_section[event] = filtered

# Clean up empty event keys
settings['hooks'] = {k: v for k, v in hooks_section.items() if v}

if changed:
    with open(settings_path, 'w') as f:
        json.dump(settings, f, indent=2)
    print(f"  Saved: {settings_path}")
else:
    print("  [skip] no devexp hooks found in settings.json")
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
