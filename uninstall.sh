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

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

[[ -d "$REPO_DIR/agents" ]] || die "agents/ directory not found. Is this the devexp repo?"
[[ -d "$REPO_DIR/skills" ]] || die "skills/ directory not found. Is this the devexp repo?"

# ── Paths ─────────────────────────────────────────────────────────────────────
CLAUDE_AGENTS="$HOME/.claude/agents"
OPENCODE_AGENTS="$HOME/.config/opencode/agents"
SKILLS_DIR="$HOME/.claude/skills"   # shared between both CLIs

# ── Detect what's installed ───────────────────────────────────────────────────
HAS_CLAUDE_INSTALL=false
HAS_OPENCODE_INSTALL=false

for f in "$REPO_DIR/agents/"*.md; do
    [[ -f "$f" ]] || continue
    [[ -f "$CLAUDE_AGENTS/$(basename "$f")"   ]] && HAS_CLAUDE_INSTALL=true
    [[ -f "$OPENCODE_AGENTS/$(basename "$f")" ]] && HAS_OPENCODE_INSTALL=true
done

echo ""
echo -e "${BOLD}devexp Framework Uninstaller${RESET}"
echo "────────────────────────────────────────"
echo ""

if ! $HAS_CLAUDE_INSTALL && ! $HAS_OPENCODE_INSTALL; then
    info "Nothing to remove — no devexp agents found in Claude Code or opencode directories."
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
    local config_path="$HOME/.config/opencode/config.json"
    if [[ -f "$config_path" ]]; then
        info "Removing MCP servers (opencode)..."
        python3 - "$REPO_DIR/mcps/registry.json" "$config_path" <<'PYEOF'
import json, sys, os
with open(sys.argv[1]) as f:
    mcps = json.load(f)
config_path = sys.argv[2]
with open(config_path) as f:
    config = json.load(f)
changed = False
for mcp in mcps:
    name = mcp['name']
    if name in config.get('mcp', {}):
        del config['mcp'][name]
        changed = True
        print(f"  \033[0;31m-\033[0m {name}")
    else:
        print(f"  [skip] {name} — not configured")
if changed:
    with open(config_path, 'w') as f:
        json.dump(config, f, indent=2)
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
        cmd = ''
        if entry.get('hooks'):
            cmd = entry['hooks'][0].get('command', '')
        # Remove entries whose command path lives inside the devexp repo
        if repo_dir in cmd:
            script_name = os.path.basename(cmd)
            print(f"  \033[0;31m-\033[0m {event}: {script_name}")
            changed = True
        else:
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

# ── Remove hooks (opencode) ───────────────────────────────────────────────────
if $REMOVE_OPENCODE; then
    plugin_dest="$HOME/.config/opencode/plugins/devexp-plugin.js"
    config_path="$HOME/.config/opencode/config.json"

    if [[ -f "$plugin_dest" || -f "$config_path" ]]; then
        info "Removing hooks (opencode plugin)..."

        # Remove all devexp plugin modules (devexp-plugin.js + imported modules)
        local plugin_dir="$HOME/.config/opencode/plugins"
        for js_file in devexp-plugin.js utils.js secret-guard.js dangerous-cmd-guard.js large-file-guard.js lint-on-save.js; do
            if [[ -f "$plugin_dir/$js_file" ]]; then
                rm -f "$plugin_dir/$js_file"
                echo -e "  ${RED}-${RESET} $js_file"
            fi
        done
        [[ -f "$plugin_dir/package.json" ]] && rm -f "$plugin_dir/package.json" && echo -e "  ${RED}-${RESET} package.json"

        if [[ -f "$config_path" ]]; then
            python3 - "$config_path" "$plugin_dest" <<'PYEOF'
import json, sys, os

config_path = sys.argv[1]
plugin_path = sys.argv[2]

with open(config_path) as f:
    try:
        config = json.load(f)
    except json.JSONDecodeError:
        sys.exit(0)

plugins = config.get('plugin', [])
if plugin_path in plugins:
    config['plugin'] = [p for p in plugins if p != plugin_path]
    with open(config_path, 'w') as f:
        json.dump(config, f, indent=2)
    print(f"  - unregistered plugin from {config_path}")
else:
    print(f"  [skip] plugin not registered in {config_path}")
PYEOF
        fi
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
