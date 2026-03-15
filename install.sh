#!/usr/bin/env bash
set -euo pipefail

# ── Colors ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
RESET='\033[0m'

# ── Flags ─────────────────────────────────────────────────────────────────────
DRY_RUN=false
SELECTED_MODEL=""
for arg in "$@"; do
    case "$arg" in
        --dry-run|-n) DRY_RUN=true ;;
        --model=*)    SELECTED_MODEL="${arg#--model=}" ;;
        --model)      shift; SELECTED_MODEL="$1" ;;
        --help|-h)
            echo "Usage: ./install.sh [--dry-run|-n] [--model <alias|model-id>]"
            echo "  --dry-run, -n     Preview what would be installed without making changes"
            echo "  --model <value>   Override model for all agents (optional — agents inherit CLI default if omitted)"
            echo ""
            echo "  Aliases:"
            echo "    Anthropic : sonnet, opus, haiku"
            echo "    OpenAI    : gpt4, gpt4o, o3, o4mini"
            echo "    DeepSeek  : deepseek, deepseek-r1"
            echo "    Kimi      : kimi, kimi-turbo"
            echo ""
            echo "  Or pass a full model ID: openai/gpt-4o, moonshot/kimi-k2.5, etc."
            exit 0 ;;
    esac
done

# ── Helpers ───────────────────────────────────────────────────────────────────
info()    { echo -e "${BLUE}[devexp]${RESET} $*"; }
success() { echo -e "${GREEN}[devexp]${RESET} $*"; }
warn()    { echo -e "${YELLOW}[devexp]${RESET} $*"; }
error()   { echo -e "${RED}[devexp] ERROR:${RESET} $*" >&2; }
die()     { error "$*"; exit 1; }
dryrun()  { echo -e "${YELLOW}[dry-run]${RESET} $*"; }

# Wrappers that respect --dry-run
run_cp()     { $DRY_RUN && dryrun "cp $1 $2" || cp "$1" "$2"; }
run_mkdir()  { $DRY_RUN && dryrun "mkdir -p $1" || mkdir -p "$1"; }
run_python() { $DRY_RUN && { dryrun "transform + write $2"; cat /dev/null; } || "$@"; }

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

[[ -d "$REPO_DIR/agents" ]] || die "agents/ directory not found in repo."
[[ -d "$REPO_DIR/skills" ]] || die "skills/ directory not found in repo."

# ── Load devexp.config.json (team/org overrides) ──────────────────────────────
CONFIG_FILE="$REPO_DIR/devexp.config.json"
CONFIG_DISABLED_AGENTS=()
CONFIG_DISABLED_SKILLS=()
CONFIG_DISABLED_HOOKS=()
CONFIG_MODEL=""
CONFIG_EXTRA_MCPS="[]"

if [[ -f "$CONFIG_FILE" ]] && command -v python3 &>/dev/null; then
    _cfg_out=$(python3 - "$CONFIG_FILE" <<'PYEOF'
import json, sys

try:
    with open(sys.argv[1]) as f:
        cfg = json.load(f)
except Exception as e:
    print(f'warn:devexp.config.json parse error: {e}', file=sys.stderr)
    cfg = {}

def bash_arr(items):
    return '(' + ' '.join(f'"{i}"' for i in (items or [])) + ')'

disabled_agents = bash_arr(cfg.get('agents', {}).get('disabled', []))
disabled_skills = bash_arr(cfg.get('skills', {}).get('disabled', []))
disabled_hooks  = bash_arr(cfg.get('hooks',  {}).get('disabled', []))
model           = cfg.get('model') or ''
extra_mcps      = json.dumps(cfg.get('mcps', []))

print(f'CONFIG_DISABLED_AGENTS={disabled_agents}')
print(f'CONFIG_DISABLED_SKILLS={disabled_skills}')
print(f'CONFIG_DISABLED_HOOKS={disabled_hooks}')
print(f'CONFIG_MODEL="{model}"')
print(f'CONFIG_EXTRA_MCPS={repr(extra_mcps)}')
PYEOF
    )
    eval "$_cfg_out"
    # --model flag takes precedence over config file
    [[ -z "$SELECTED_MODEL" && -n "$CONFIG_MODEL" ]] && SELECTED_MODEL="$CONFIG_MODEL"
    # Report non-empty config
    _has_config=false
    [[ ${#CONFIG_DISABLED_AGENTS[@]} -gt 0 ]] && _has_config=true
    [[ ${#CONFIG_DISABLED_SKILLS[@]} -gt 0 ]] && _has_config=true
    [[ ${#CONFIG_DISABLED_HOOKS[@]} -gt 0 ]]  && _has_config=true
    [[ -n "$CONFIG_MODEL" ]]                   && _has_config=true
    [[ "$CONFIG_EXTRA_MCPS" != "[]" ]]         && _has_config=true
    $_has_config && info "devexp.config.json loaded — org overrides applied."
fi

# Helper: returns 0 if $1 is in remaining args
is_disabled() {
    local name="$1"; shift
    local item
    for item in "$@"; do
        [[ "$item" == "$name" ]] && return 0
    done
    return 1
}

# ── Detect installed CLIs ─────────────────────────────────────────────────────
HAS_CLAUDE=false
HAS_OPENCODE=false
command -v claude    &>/dev/null && HAS_CLAUDE=true
command -v opencode  &>/dev/null && HAS_OPENCODE=true

# ── Determine targets ─────────────────────────────────────────────────────────
INSTALL_CLAUDE=false
INSTALL_OPENCODE=false

echo ""
echo -e "${BOLD}devexp Framework Installer${RESET}"
echo "────────────────────────────────────────"
$DRY_RUN && echo -e "${YELLOW}DRY RUN MODE — no files will be written${RESET}"
echo ""

if $HAS_CLAUDE && $HAS_OPENCODE; then
    info "Detected: Claude Code and opencode"
    echo ""
    echo "  [1] Claude Code only"
    echo "  [2] opencode only"
    echo "  [3] Both"
    echo ""
    read -r -p "Install for which CLI? [1/2/3]: " choice
    case "$choice" in
        1) INSTALL_CLAUDE=true ;;
        2) INSTALL_OPENCODE=true ;;
        3) INSTALL_CLAUDE=true; INSTALL_OPENCODE=true ;;
        *) die "Invalid choice." ;;
    esac
elif $HAS_CLAUDE; then
    info "Detected: Claude Code"
    INSTALL_CLAUDE=true
elif $HAS_OPENCODE; then
    info "Detected: opencode"
    INSTALL_OPENCODE=true
else
    warn "No supported CLI detected (claude or opencode)."
    echo ""
    echo "  [1] Install for Claude Code"
    echo "  [2] Install for opencode"
    echo "  [3] Install for both"
    echo "  [q] Quit"
    echo ""
    read -r -p "Choice [1/2/3/q]: " choice
    case "$choice" in
        1) INSTALL_CLAUDE=true ;;
        2) INSTALL_OPENCODE=true ;;
        3) INSTALL_CLAUDE=true; INSTALL_OPENCODE=true ;;
        q|Q) info "Aborted."; exit 0 ;;
        *) die "Invalid choice." ;;
    esac
fi

echo ""

# ── Model selection ───────────────────────────────────────────────────────────
# Agents inherit the CLI's default model. --model overrides all agents at install time.
if [[ -n "$SELECTED_MODEL" ]]; then
    info "Model override: ${BOLD}$SELECTED_MODEL${RESET} (will be set on all agents)"
    echo ""
fi

# ── Shared: backup helper ─────────────────────────────────────────────────────
backup_conflicts() {
    local backup_dir="$1"
    shift
    local conflicts=("$@")

    if [[ ${#conflicts[@]} -gt 0 ]]; then
        warn "Backing up ${#conflicts[@]} existing file(s) to:"
        warn "  $backup_dir"
        mkdir -p "$backup_dir"
        for f in "${conflicts[@]}"; do
            # Strip home dir prefix to get a relative path for backup structure
            local rel="${f#"$HOME/"}"
            local dest="$backup_dir/$rel"
            mkdir -p "$(dirname "$dest")"
            cp "$f" "$dest"
        done
        success "Backup complete."
        echo ""
    fi
}

# ── Claude Code installation ──────────────────────────────────────────────────
install_claude() {
    local AGENTS_TARGET="$HOME/.claude/agents"
    local SKILLS_TARGET="$HOME/.claude/skills"
    local BACKUP_DIR="$HOME/.claude/.devexp-backup-$(date +%Y%m%dT%H%M%S)"

    info "Installing for Claude Code..."
    echo ""

    run_mkdir "$AGENTS_TARGET" || die "Failed to create $AGENTS_TARGET"
    run_mkdir "$SKILLS_TARGET"  || die "Failed to create $SKILLS_TARGET"

    # Collect conflicts
    local conflicts=()
    for f in "$REPO_DIR/agents/"*.md; do
        [[ -f "$f" ]] || continue
        [[ "$(basename "$f")" == "README.md" ]] && continue
        local t="$AGENTS_TARGET/$(basename "$f")"
        [[ -f "$t" ]] && conflicts+=("$t")
    done
    for d in "$REPO_DIR/skills/"/*/; do
        [[ -d "$d" ]] || continue
        local t="$SKILLS_TARGET/$(basename "$d")/skill.md"
        [[ -f "$t" ]] && conflicts+=("$t")
    done

    $DRY_RUN || backup_conflicts "$BACKUP_DIR" "${conflicts[@]+"${conflicts[@]}"}"

    # Install agents (skip README.md and any non-agent .md files)
    local count=0
    info "Installing agents..."
    for f in "$REPO_DIR/agents/"*.md; do
        [[ -f "$f" ]] || continue
        [[ "$(basename "$f")" == "README.md" ]] && continue
        local agent_name
        agent_name="$(basename "$f" .md)"
        if is_disabled "$agent_name" "${CONFIG_DISABLED_AGENTS[@]+"${CONFIG_DISABLED_AGENTS[@]}"}"; then
            echo -e "  ${YELLOW}[skip]${RESET} $(basename "$f") (disabled in devexp.config.json)"
            continue
        fi
        local dest="$AGENTS_TARGET/$(basename "$f")"
        if $DRY_RUN; then
            dryrun "write $dest"
        elif [[ -n "$SELECTED_MODEL" ]]; then
            sed "s/^model:.*/model: $SELECTED_MODEL/" "$f" > "$dest"
        else
            cp "$f" "$dest"
        fi
        echo -e "  ${GREEN}+${RESET} $(basename "$f")"
        (( count++ )) || true
    done
    success "Installed $count agent(s)."
    echo ""

    # Install skills
    count=0
    info "Installing skills..."
    for d in "$REPO_DIR/skills/"/*/; do
        [[ -d "$d" ]] || continue
        local skill="$(basename "$d")"
        if is_disabled "$skill" "${CONFIG_DISABLED_SKILLS[@]+"${CONFIG_DISABLED_SKILLS[@]}"}"; then
            echo -e "  ${YELLOW}[skip]${RESET} $skill (disabled in devexp.config.json)"
            continue
        fi
        run_mkdir "$SKILLS_TARGET/$skill"
        if [[ -f "$d/skill.md" ]]; then
            run_cp "$d/skill.md" "$SKILLS_TARGET/$skill/skill.md"
            echo -e "  ${GREEN}+${RESET} $skill/skill.md"
            (( count++ )) || true
        fi
    done
    success "Installed $count skill(s)."
    echo ""

    # Install MCPs (base registry + config extras)
    install_mcps_claude
    install_extra_mcps_claude

    # Install hooks
    install_hooks_claude "${CONFIG_DISABLED_HOOKS[@]+"${CONFIG_DISABLED_HOOKS[@]}"}"

    success "Claude Code installation complete."
    echo "  Agents: $AGENTS_TARGET"
    echo "  Skills: $SKILLS_TARGET"
    echo ""
    info "Restart Claude Code to activate."
    echo ""
}

# ── opencode agent frontmatter transformation ─────────────────────────────────
# Transforms Claude Code agent frontmatter to opencode format:
#   - Strips: name, color, memory (not supported by opencode)
#   - Maps:   model aliases (sonnet/opus/haiku → full opencode IDs); custom IDs pass through as-is
#   - Maps:   tools list → YAML object disabling tools not in the list
#   - Adds:   mode: subagent
transform_agent_for_opencode() {
    local src="$1"
    local model="$2"
    python3 - "$src" "$model" <<'PYEOF'
import re, sys

OPENCODE_TOOLS = {'read', 'write', 'edit', 'bash', 'glob', 'grep', 'webfetch', 'websearch'}

CLAUDE_TO_OC = {
    'read': 'read', 'write': 'write', 'edit': 'edit', 'bash': 'bash',
    'glob': 'glob', 'grep': 'grep', 'webfetch': 'webfetch', 'websearch': 'websearch',
}

MODEL_MAP = {
    # Anthropic
    'sonnet':       'anthropic/claude-sonnet-4-6',
    'opus':         'anthropic/claude-opus-4-6',
    'haiku':        'anthropic/claude-haiku-4-5-20251001',
    # OpenAI
    'gpt4':         'openai/gpt-4.1-2025-04-14',
    'gpt4o':        'openai/gpt-4o',
    'o3':           'openai/o3-2025-04-16',
    'o4mini':       'openai/o4-mini-2025-04-16',
    # DeepSeek
    'deepseek':     'deepseek/deepseek-chat',
    'deepseek-r1':  'deepseek/deepseek-reasoner',
    # Kimi (Moonshot)
    'kimi':         'moonshot/kimi-k2.5',
    'kimi-turbo':   'moonshot/kimi-k2-turbo-preview',
}

SKIP_KEYS = {'name', 'color', 'memory'}

with open(sys.argv[1]) as f:
    content = f.read()

# Selected model alias from installer (overrides per-agent model)
selected_model = sys.argv[2] if len(sys.argv) > 2 and sys.argv[2] else None

parts = content.split('---', 2)
if len(parts) < 3:
    sys.stdout.write(content)
    sys.exit(0)

_, fm, body = parts
lines = fm.strip().split('\n')
new_lines = []

for line in lines:
    m = re.match(r'^([a-zA-Z_]+)\s*:(.*)', line)
    if not m:
        new_lines.append(line)
        continue

    key = m.group(1).lower()
    val = m.group(2).strip()

    if key in SKIP_KEYS:
        continue
    elif key == 'model':
        alias = selected_model if selected_model else val
        new_lines.append(f'model: {MODEL_MAP.get(alias, alias)}')
    elif key == 'tools':
        claude_tools = {t.strip().lower() for t in val.split(',')}
        enabled  = {CLAUDE_TO_OC[t] for t in claude_tools if t in CLAUDE_TO_OC}
        disabled = sorted(OPENCODE_TOOLS - enabled)
        if disabled:
            new_lines.append('tools:')
            for t in disabled:
                new_lines.append(f'  {t}: false')
        # If all opencode tools are enabled, omit the tools section (default = all on)
    else:
        new_lines.append(line)

new_lines.append('mode: subagent')
sys.stdout.write('---\n' + '\n'.join(new_lines) + '\n---' + body)
PYEOF
}

# ── opencode installation ─────────────────────────────────────────────────────
install_opencode() {
    local AGENTS_TARGET="$HOME/.config/opencode/agents"
    # opencode uses its own commands directory — flat .md files, filename = command name
    local SKILLS_TARGET="$HOME/.config/opencode/commands"
    local BACKUP_DIR="$HOME/.config/opencode/.devexp-backup-$(date +%Y%m%dT%H%M%S)"

    info "Installing for opencode..."
    echo ""

    command -v python3 &>/dev/null || die "python3 is required for opencode install (used for frontmatter transformation)"

    run_mkdir "$AGENTS_TARGET" || die "Failed to create $AGENTS_TARGET"
    run_mkdir "$SKILLS_TARGET"  || die "Failed to create $SKILLS_TARGET"

    # Collect conflicts
    local conflicts=()
    for f in "$REPO_DIR/agents/"*.md; do
        [[ -f "$f" ]] || continue
        [[ "$(basename "$f")" == "README.md" ]] && continue
        local t="$AGENTS_TARGET/$(basename "$f")"
        [[ -f "$t" ]] && conflicts+=("$t")
    done
    # opencode-exclusive agents (agents/opencode/) — installed as-is, no transform
    for f in "$REPO_DIR/agents/opencode/"*.md; do
        [[ -f "$f" ]] || continue
        local t="$AGENTS_TARGET/$(basename "$f")"
        [[ -f "$t" ]] && conflicts+=("$t")
    done
    for d in "$REPO_DIR/skills/"/*/; do
        [[ -d "$d" ]] || continue
        local t="$SKILLS_TARGET/$(basename "$d").md"
        [[ -f "$t" ]] && conflicts+=("$t")
    done

    $DRY_RUN || backup_conflicts "$BACKUP_DIR" "${conflicts[@]+"${conflicts[@]}"}"

    # Install shared agents (transformed from Claude Code format; skip READMEs)
    local count=0
    info "Installing agents (transformed for opencode)..."
    for f in "$REPO_DIR/agents/"*.md; do
        [[ -f "$f" ]] || continue
        [[ "$(basename "$f")" == "README.md" ]] && continue
        local agent_name
        agent_name="$(basename "$f" .md)"
        if is_disabled "$agent_name" "${CONFIG_DISABLED_AGENTS[@]+"${CONFIG_DISABLED_AGENTS[@]}"}"; then
            echo -e "  ${YELLOW}[skip]${RESET} $(basename "$f") (disabled in devexp.config.json)"
            continue
        fi
        if $DRY_RUN; then
            dryrun "transform + write $AGENTS_TARGET/$(basename "$f")"
        else
            transform_agent_for_opencode "$f" "$SELECTED_MODEL" > "$AGENTS_TARGET/$(basename "$f")"
        fi
        echo -e "  ${GREEN}+${RESET} $(basename "$f")"
        (( count++ )) || true
    done

    # Install opencode-exclusive agents (already in opencode format — copy as-is, optional model override)
    if [[ -d "$REPO_DIR/agents/opencode" ]]; then
        for f in "$REPO_DIR/agents/opencode/"*.md; do
            [[ -f "$f" ]] || continue
            local dest="$AGENTS_TARGET/$(basename "$f")"
            if $DRY_RUN; then
                dryrun "write $dest"
            elif [[ -n "$SELECTED_MODEL" ]]; then
                python3 - "$f" "$SELECTED_MODEL" <<'PYEOF' > "$dest"
import re, sys

MODEL_MAP = {
    'sonnet':       'anthropic/claude-sonnet-4-6',
    'opus':         'anthropic/claude-opus-4-6',
    'haiku':        'anthropic/claude-haiku-4-5-20251001',
    'gpt4':         'openai/gpt-4.1-2025-04-14',
    'gpt4o':        'openai/gpt-4o',
    'o3':           'openai/o3-2025-04-16',
    'o4mini':       'openai/o4-mini-2025-04-16',
    'deepseek':     'deepseek/deepseek-chat',
    'deepseek-r1':  'deepseek/deepseek-reasoner',
    'kimi':         'moonshot/kimi-k2.5',
    'kimi-turbo':   'moonshot/kimi-k2-turbo-preview',
}

with open(sys.argv[1]) as f:
    content = f.read()

resolved = MODEL_MAP.get(sys.argv[2], sys.argv[2])
result = re.sub(r'^model:.*$', f'model: {resolved}', content, flags=re.MULTILINE)
sys.stdout.write(result)
PYEOF
            else
                cp "$f" "$dest"
            fi
            echo -e "  ${GREEN}+${RESET} $(basename "$f") ${YELLOW}(opencode-exclusive)${RESET}"
            (( count++ )) || true
        done
    fi
    success "Installed $count agent(s)."
    echo ""

    # Install skills as opencode commands (flat .md files, name: line stripped — filename is the command name)
    count=0
    info "Installing skills (to ~/.config/opencode/commands — opencode slash commands)..."
    run_mkdir "$SKILLS_TARGET"
    for d in "$REPO_DIR/skills/"/*/; do
        [[ -d "$d" ]] || continue
        local skill="$(basename "$d")"
        if is_disabled "$skill" "${CONFIG_DISABLED_SKILLS[@]+"${CONFIG_DISABLED_SKILLS[@]}"}"; then
            echo -e "  ${YELLOW}[skip]${RESET} $skill (disabled in devexp.config.json)"
            continue
        fi
        if [[ -f "$d/skill.md" ]]; then
            local dest="$SKILLS_TARGET/$skill.md"
            if $DRY_RUN; then
                dryrun "write $dest"
            else
                # Strip 'name:' line from frontmatter — opencode derives name from filename
                sed '/^name:/d' "$d/skill.md" > "$dest"
            fi
            echo -e "  ${GREEN}+${RESET} $skill.md"
            (( count++ )) || true
        fi
    done
    success "Installed $count skill(s)."
    echo ""

    # Install MCPs (base registry + config extras)
    install_mcps_opencode
    install_extra_mcps_opencode

    # Install hooks (opencode plugin)
    install_hooks_opencode "${CONFIG_DISABLED_HOOKS[@]+"${CONFIG_DISABLED_HOOKS[@]}"}"

    success "opencode installation complete."
    echo "  Agents: $AGENTS_TARGET"
    echo "  Skills: $SKILLS_TARGET"
    echo ""
    info "Restart opencode to activate."
    echo ""
}

# ── MCP installation (Claude Code) ────────────────────────────────────────────
install_mcps_claude() {
    local registry="$REPO_DIR/mcps/registry.json"
    [[ -f "$registry" ]] || return 0
    command -v claude &>/dev/null || return 0

    info "Installing MCP servers (Claude Code)..."
    local count=0

    python3 - "$registry" "$REPO_DIR/mcps/.env" "$($DRY_RUN && echo 1 || echo 0)" "${SKIPPED_MCPS_FILE:-}" <<'PYEOF'
import json, sys, subprocess, os

with open(sys.argv[1]) as f:
    mcps = json.load(f)
dotenv_path      = sys.argv[2]
dry_run          = sys.argv[3] == "1"
skipped_mcps_file = sys.argv[4] if len(sys.argv) > 4 else ""

# Load mcps/.env if it exists
dotenv = {}
if os.path.exists(dotenv_path):
    with open(dotenv_path) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                k, _, v = line.partition('=')
                dotenv[k.strip()] = v.strip()
    if dotenv:
        print(f"  Loaded {len(dotenv)} var(s) from mcps/.env")

# Merged env: shell → .env overrides (so .env takes precedence for MCP vars)
merged_env = {**os.environ, **dotenv}

for mcp in mcps:
    name              = mcp['name']
    transport         = mcp.get('transport', 'stdio')
    url               = mcp.get('url', '')
    command           = mcp.get('command', '')
    args              = mcp.get('args', [])
    scope             = mcp.get('scope', 'user')
    env_vars          = mcp.get('env', {})
    required_env      = mcp.get('required_env', [])
    setup_instructions = mcp.get('setup_instructions', '')

    # Resolve env_vars: substitute from merged_env if value is empty
    resolved = {k: merged_env.get(k, v) for k, v in env_vars.items()}
    # Also pick up required_env values from merged_env
    for key in required_env:
        if key in merged_env and key not in resolved:
            resolved[key] = merged_env[key]

    missing = [e for e in required_env if not merged_env.get(e)]
    if missing:
        print(f"\n  \033[0;31m[REQUIRED]\033[0m {name} — missing required env vars:")
        for key in missing:
            print(f"    {key}=<your-value>")
        if setup_instructions:
            print()
            for line in setup_instructions.split('\n'):
                print(f"  {line}")
        print(f"\n  {name} will not be available until these are set.\n")
        if skipped_mcps_file:
            with open(skipped_mcps_file, 'a') as f:
                f.write(name + '\n')
        continue

    # Build --env flags for claude mcp add (stdio only)
    env_flags = []
    for k, v in resolved.items():
        env_flags += ['--env', f'{k}={v}']

    if dry_run:
        if transport in ('sse', 'http'):
            print(f"  [dry-run] claude mcp add --scope {scope} --transport {transport} {name} {url}")
        else:
            env_preview = ' '.join(f'--env {k}=***' for k in resolved) if resolved else ''
            print(f"  [dry-run] claude mcp add --scope {scope} {env_preview} {name} -- {command} {' '.join(args)}")
        continue

    result = subprocess.run(['claude', 'mcp', 'list'], capture_output=True, text=True)
    if name in result.stdout:
        print(f"  [skip] {name} — already installed")
        continue

    if transport in ('sse', 'http'):
        r = subprocess.run(
            ['claude', 'mcp', 'add', '--scope', scope, '--transport', transport, name, url],
            capture_output=True, text=True
        )
    else:
        r = subprocess.run(
            ['claude', 'mcp', 'add', '--scope', scope] + env_flags + [name, '--', command] + args,
            capture_output=True, text=True
        )
    if r.returncode == 0:
        print(f"  \033[0;32m+\033[0m {name}")
    else:
        print(f"  \033[1;33m[warn]\033[0m {name} — {r.stderr.strip()}", file=sys.stderr)
PYEOF
    echo ""
}

# ── MCP installation (opencode) ───────────────────────────────────────────────
install_mcps_opencode() {
    local registry="$REPO_DIR/mcps/registry.json"
    [[ -f "$registry" ]] || return 0

    local config_path="$HOME/.config/opencode/config.json"
    info "Installing MCP servers (opencode → $config_path)..."

    python3 - "$registry" "$config_path" "$REPO_DIR/mcps/.env" "$($DRY_RUN && echo 1 || echo 0)" "${SKIPPED_MCPS_FILE:-}" <<'PYEOF'
import json, sys, os

with open(sys.argv[1]) as f:
    mcps = json.load(f)
config_path       = sys.argv[2]
dotenv_path       = sys.argv[3]
dry_run           = sys.argv[4] == "1"
skipped_mcps_file = sys.argv[5] if len(sys.argv) > 5 else ""

# Load mcps/.env if it exists
dotenv = {}
if os.path.exists(dotenv_path):
    with open(dotenv_path) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                k, _, v = line.partition('=')
                dotenv[k.strip()] = v.strip()
    if dotenv:
        print(f"  Loaded {len(dotenv)} var(s) from mcps/.env")

merged_env = {**os.environ, **dotenv}

# Load existing config or start fresh
config = {}
if os.path.exists(config_path):
    with open(config_path) as f:
        try:
            config = json.load(f)
        except json.JSONDecodeError:
            config = {}

if 'mcp' not in config:
    config['mcp'] = {}

added = []
for mcp in mcps:
    name               = mcp['name']
    transport          = mcp.get('transport', 'stdio')
    url                = mcp.get('url', '')
    command            = mcp.get('command', '')
    args               = mcp.get('args', [])
    env_vars           = mcp.get('env', {})
    required_env       = mcp.get('required_env', [])
    setup_instructions = mcp.get('setup_instructions', '')

    # Resolve env values from .env / shell
    resolved = {k: merged_env.get(k, v) for k, v in env_vars.items()}
    for key in required_env:
        if key in merged_env and key not in resolved:
            resolved[key] = merged_env[key]

    missing = [e for e in required_env if not merged_env.get(e)]
    if missing:
        print(f"\n  \033[0;31m[REQUIRED]\033[0m {name} — missing required env vars:")
        for key in missing:
            print(f"    {key}=<your-value>")
        if setup_instructions:
            print()
            for line in setup_instructions.split('\n'):
                print(f"  {line}")
        print(f"\n  {name} will not be available until these are set.\n")
        if skipped_mcps_file:
            with open(skipped_mcps_file, 'a') as f:
                f.write(name + '\n')
        continue

    if transport in ('sse', 'http'):
        entry = {'type': 'remote', 'url': url}
    else:
        entry = {'type': 'local', 'command': [command] + args}
        if resolved:
            entry['env'] = resolved

    if dry_run:
        print(f"  [dry-run] add mcp.{name} ({transport}) to {config_path}")
        continue

    if name in config['mcp']:
        print(f"  [skip] {name} — already configured")
        continue

    config['mcp'][name] = entry
    added.append(name)
    print(f"  \033[0;32m+\033[0m {name}")

if added and not dry_run:
    os.makedirs(os.path.dirname(config_path), exist_ok=True)
    with open(config_path, 'w') as f:
        json.dump(config, f, indent=2)
    print(f"  Saved: {config_path}")
PYEOF
    echo ""
}

# ── Hook installation (Claude Code) ──────────────────────────────────────────
install_hooks_claude() {
    local registry="$REPO_DIR/hooks/registry.json"
    [[ -f "$registry" ]] || return 0

    local settings="$HOME/.claude/settings.json"
    # Remaining args are disabled hook names
    local disabled_csv
    disabled_csv=$(printf '%s,' "$@")

    info "Installing hooks (Claude Code)..."

    python3 - "$registry" "$REPO_DIR" "$settings" "$($DRY_RUN && echo 1 || echo 0)" "$disabled_csv" <<'PYEOF'
import json, sys, os

registry_path = sys.argv[1]
repo_dir      = sys.argv[2]
settings_path = sys.argv[3]
dry_run       = sys.argv[4] == "1"
disabled      = set(filter(None, sys.argv[5].split(','))) if len(sys.argv) > 5 else set()

with open(registry_path) as f:
    hooks = json.load(f)

# Load or initialise settings.json
settings = {}
if os.path.exists(settings_path):
    with open(settings_path) as f:
        try:
            settings = json.load(f)
        except json.JSONDecodeError:
            settings = {}

if 'hooks' not in settings:
    settings['hooks'] = {}

changed = False
for hook in hooks:
    if not hook.get('enabled', True):
        continue
    if hook.get('name') in disabled:
        print(f"  [skip] {hook['name']} (disabled in devexp.config.json)")
        continue
    cc     = hook.get('claude_code', {})
    event  = cc.get('event')
    script = cc.get('script')
    if not event or not script:
        continue

    script_abs = os.path.join(repo_dir, script)

    if dry_run:
        print(f"  [dry-run] add {event} hook: {os.path.basename(script)}")
        continue

    if event not in settings['hooks']:
        settings['hooks'][event] = []

    # Idempotency: skip if this exact script is already registered
    existing_cmds = [
        h.get('hooks', [{}])[0].get('command', '')
        for h in settings['hooks'][event]
        if h.get('hooks')
    ]
    if script_abs in existing_cmds:
        print(f"  [skip] {event}: {os.path.basename(script)} — already registered")
        continue

    settings['hooks'][event].append({
        'matcher': cc.get('matcher', '.*'),
        'hooks': [{'type': 'command', 'command': script_abs}]
    })
    changed = True
    print(f"  \033[0;32m+\033[0m {event}: {os.path.basename(script)}")

if changed and not dry_run:
    os.makedirs(os.path.dirname(settings_path), exist_ok=True)
    with open(settings_path, 'w') as f:
        json.dump(settings, f, indent=2)
    print(f"  Saved: {settings_path}")
PYEOF
    echo ""
}

# ── Hook installation (opencode) ──────────────────────────────────────────────
install_hooks_opencode() {
    local registry="$REPO_DIR/hooks/registry.json"
    [[ -f "$registry" ]] || return 0

    local plugin_src="$REPO_DIR/hooks/opencode/devexp-plugin.js"
    local plugin_dir_src="$REPO_DIR/hooks/opencode"
    [[ -f "$plugin_src" ]] || return 0

    # Check if ALL hooks are disabled — if so, skip plugin install entirely
    local all_hook_names=()
    while IFS= read -r name; do
        [[ -n "$name" ]] && all_hook_names+=("$name")
    done < <(python3 -c "import json; [print(h['name']) for h in json.load(open('$registry'))]" 2>/dev/null)

    local enabled_count=0
    for hname in "${all_hook_names[@]+"${all_hook_names[@]}"}"; do
        is_disabled "$hname" "$@" || (( enabled_count++ )) || true
    done
    if [[ $enabled_count -eq 0 && ${#all_hook_names[@]} -gt 0 ]]; then
        info "Skipping opencode plugin — all hooks disabled in devexp.config.json."
        echo ""
        return 0
    fi

    local plugin_dir="$HOME/.config/opencode/plugins"
    local plugin_dest="$plugin_dir/devexp-plugin.js"
    local config_path="$HOME/.config/opencode/config.json"

    info "Installing hooks (opencode plugin)..."

    if $DRY_RUN; then
        for js_file in "$plugin_dir_src"/*.js; do
            dryrun "cp $(basename "$js_file") → $plugin_dir/"
        done
        [[ -f "$plugin_dir_src/package.json" ]] && dryrun "cp package.json → $plugin_dir/"
        dryrun "register plugin path in $config_path"
        echo ""
        return 0
    fi

    run_mkdir "$plugin_dir"
    # Copy all JS modules — devexp-plugin.js imports the others via relative paths
    for js_file in "$plugin_dir_src"/*.js; do
        cp "$js_file" "$plugin_dir/$(basename "$js_file")"
        echo -e "  ${GREEN}+${RESET} $(basename "$js_file") → $plugin_dir/"
    done
    # Copy package.json so Node treats the directory as ESM
    if [[ -f "$plugin_dir_src/package.json" ]]; then
        cp "$plugin_dir_src/package.json" "$plugin_dir/package.json"
        echo -e "  ${GREEN}+${RESET} package.json → $plugin_dir/"
    fi

    # Register the plugin path in opencode config.json
    python3 - "$config_path" "$plugin_dest" <<'PYEOF'
import json, sys, os

config_path = sys.argv[1]
plugin_path = sys.argv[2]

config = {}
if os.path.exists(config_path):
    with open(config_path) as f:
        try:
            config = json.load(f)
        except json.JSONDecodeError:
            config = {}

if 'plugin' not in config:
    config['plugin'] = []

if plugin_path in config['plugin']:
    print(f"  [skip] plugin already registered in {config_path}")
else:
    config['plugin'].append(plugin_path)
    os.makedirs(os.path.dirname(config_path), exist_ok=True)
    with open(config_path, 'w') as f:
        json.dump(config, f, indent=2)
    print(f"  + registered plugin in {config_path}")
PYEOF
    echo ""
}

# ── Extra MCP installation from devexp.config.json (Claude Code) ─────────────
install_extra_mcps_claude() {
    [[ "$CONFIG_EXTRA_MCPS" == "[]" ]] && return 0
    command -v claude &>/dev/null || return 0

    info "Installing extra MCP servers from devexp.config.json (Claude Code)..."

    python3 - "$CONFIG_EXTRA_MCPS" "$REPO_DIR/mcps/.env" "$($DRY_RUN && echo 1 || echo 0)" <<'PYEOF'
import json, sys, subprocess, os

mcps        = json.loads(sys.argv[1])
dotenv_path = sys.argv[2]
dry_run     = sys.argv[3] == "1"

if not mcps:
    sys.exit(0)

dotenv = {}
if os.path.exists(dotenv_path):
    with open(dotenv_path) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                k, _, v = line.partition('=')
                dotenv[k.strip()] = v.strip()

merged_env = {**os.environ, **dotenv}

for mcp in mcps:
    name         = mcp['name']
    command      = mcp['command']
    args         = mcp.get('args', [])
    scope        = mcp.get('scope', 'user')
    env_vars     = mcp.get('env', {})
    required_env = mcp.get('required_env', [])

    resolved = {k: merged_env.get(k, v) for k, v in env_vars.items()}
    for key in required_env:
        if key in merged_env and key not in resolved:
            resolved[key] = merged_env[key]

    missing = [e for e in required_env if not merged_env.get(e)]
    if missing:
        print(f"  [skip] {name} — missing env: {', '.join(missing)}")
        continue

    env_flags = []
    for k, v in resolved.items():
        env_flags += ['--env', f'{k}={v}']

    if dry_run:
        env_preview = ' '.join(f'--env {k}=***' for k in resolved) if resolved else ''
        print(f"  [dry-run] claude mcp add --scope {scope} {env_preview} {name} -- {command} {' '.join(args)}")
        continue

    result = subprocess.run(['claude', 'mcp', 'list'], capture_output=True, text=True)
    if name in result.stdout:
        print(f"  [skip] {name} — already installed")
        continue

    r = subprocess.run(
        ['claude', 'mcp', 'add', '--scope', scope] + env_flags + [name, '--', command] + args,
        capture_output=True, text=True
    )
    if r.returncode == 0:
        print(f"  \033[0;32m+\033[0m {name}")
    else:
        print(f"  \033[1;33m[warn]\033[0m {name} — {r.stderr.strip()}", file=sys.stderr)
PYEOF
    echo ""
}

# ── Extra MCP installation from devexp.config.json (opencode) ─────────────────
install_extra_mcps_opencode() {
    [[ "$CONFIG_EXTRA_MCPS" == "[]" ]] && return 0

    local config_path="$HOME/.config/opencode/config.json"
    info "Installing extra MCP servers from devexp.config.json (opencode → $config_path)..."

    python3 - "$CONFIG_EXTRA_MCPS" "$config_path" "$REPO_DIR/mcps/.env" "$($DRY_RUN && echo 1 || echo 0)" <<'PYEOF'
import json, sys, os

mcps        = json.loads(sys.argv[1])
config_path = sys.argv[2]
dotenv_path = sys.argv[3]
dry_run     = sys.argv[4] == "1"

if not mcps:
    sys.exit(0)

dotenv = {}
if os.path.exists(dotenv_path):
    with open(dotenv_path) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                k, _, v = line.partition('=')
                dotenv[k.strip()] = v.strip()

merged_env = {**os.environ, **dotenv}

config = {}
if os.path.exists(config_path):
    with open(config_path) as f:
        try:
            config = json.load(f)
        except json.JSONDecodeError:
            config = {}

if 'mcp' not in config:
    config['mcp'] = {}

added = []
for mcp in mcps:
    name         = mcp['name']
    command      = mcp['command']
    args         = mcp.get('args', [])
    env_vars     = mcp.get('env', {})
    required_env = mcp.get('required_env', [])

    resolved = {k: merged_env.get(k, v) for k, v in env_vars.items()}
    for key in required_env:
        if key in merged_env and key not in resolved:
            resolved[key] = merged_env[key]

    missing = [e for e in required_env if not merged_env.get(e)]
    if missing:
        print(f"\n  \033[0;31m[REQUIRED]\033[0m {name} — missing required env vars:")
        for key in missing:
            print(f"    {key}=<your-value>")
        print(f"\n  {name} will not be available until these are set.\n")
        continue

    entry = {'type': 'local', 'command': [command] + args}
    if resolved:
        entry['env'] = resolved

    if dry_run:
        print(f"  [dry-run] add mcp.{name} to {config_path}")
        continue

    if name in config['mcp']:
        print(f"  [skip] {name} — already configured")
        continue

    config['mcp'][name] = entry
    added.append(name)
    print(f"  \033[0;32m+\033[0m {name}")

if added and not dry_run:
    os.makedirs(os.path.dirname(config_path), exist_ok=True)
    with open(config_path, 'w') as f:
        json.dump(config, f, indent=2)
    print(f"  Saved: {config_path}")
PYEOF
    echo ""
}

# ── Docker services ───────────────────────────────────────────────────────────
start_docker_services() {
    local registry="$REPO_DIR/mcps/registry.json"
    [[ -f "$registry" ]] || return 0
    command -v docker &>/dev/null || return 0

    # Collect MCPs that have a docker_compose field and all required_env satisfied
    local services
    services=$(python3 - "$registry" "$REPO_DIR/mcps/.env" <<'PYEOF'
import json, sys, os

with open(sys.argv[1]) as f:
    mcps = json.load(f)
dotenv_path = sys.argv[2]

dotenv = {}
if os.path.exists(dotenv_path):
    with open(dotenv_path) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                k, _, v = line.partition('=')
                dotenv[k.strip()] = v.strip()

merged_env = {**os.environ, **dotenv}

for mcp in mcps:
    dc = mcp.get('docker_compose')
    if not dc:
        continue
    required_env = mcp.get('required_env', [])
    missing = [e for e in required_env if not merged_env.get(e)]
    if missing:
        continue
    print(f"{mcp['name']}|{dc}")
PYEOF
    )

    [[ -z "$services" ]] && return 0

    info "Starting Docker services..."

    while IFS='|' read -r name compose_rel; do
        local compose_file="$REPO_DIR/$compose_rel"
        [[ -f "$compose_file" ]] || continue

        # Generate ov.conf for openviking before starting container
        if [[ "$name" == "openviking" ]]; then
            _generate_openviking_conf
        fi

        if $DRY_RUN; then
            dryrun "docker compose -f $compose_rel up -d"
            continue
        fi

        if docker compose -f "$compose_file" ps --quiet 2>/dev/null | grep -q .; then
            echo -e "  ${YELLOW}[running]${RESET} $name — already up"
        else
            echo -e "  Starting ${BOLD}$name${RESET}..."
            docker compose -f "$compose_file" up -d \
                && echo -e "  ${GREEN}+${RESET} $name started" \
                || warn "$name — docker compose failed (check: docker compose -f $compose_rel logs)"
        fi
    done <<< "$services"

    echo ""
}

_generate_openviking_conf() {
    local conf_dir="$HOME/.openviking"
    local conf_file="$conf_dir/ov.conf"

    [[ -f "$conf_file" ]] && return 0  # already exists, don't overwrite

    # Load required VLM vars from shell or mcps/.env
    local dotenv_file="$REPO_DIR/mcps/.env"
    local vlm_key="${OPENVIKING_VLM_API_KEY:-}"
    local vlm_model="${OPENVIKING_VLM_MODEL:-}"

    if [[ -f "$dotenv_file" ]]; then
        _val() { grep -E "^$1=" "$dotenv_file" | tail -1 | cut -d= -f2-; }
        [[ -z "$vlm_key"   ]] && vlm_key="$(_val OPENVIKING_VLM_API_KEY)"
        [[ -z "$vlm_model" ]] && vlm_model="$(_val OPENVIKING_VLM_MODEL)"
    fi

    if [[ -z "$vlm_key" || -z "$vlm_model" ]]; then
        warn "openviking: skipping ov.conf generation — OPENVIKING_VLM_API_KEY and OPENVIKING_VLM_MODEL must be set in mcps/.env"
        return 0
    fi

    mkdir -p "$conf_dir"
    python3 - "$conf_file" "$vlm_key" "$vlm_model" <<'PYEOF'
import json, sys

conf_file = sys.argv[1]
vlm_key   = sys.argv[2]
vlm_model = sys.argv[3]

conf = {
    "storage": {
        "workspace": "/app/data"
    },
    "log": {
        "level": "INFO",
        "output": "stdout"
    },
    "embedding": {
        "dense": {
            "api_base":  "http://jina-embed:80/v1",
            "api_key":   "local",
            "provider":  "openai",
            "dimension": 768,
            "model":     "jinaai/jina-embeddings-v2-base-en"
        }
    },
    "vlm": {
        "provider": "litellm",
        "api_key":  vlm_key,
        "model":    vlm_model
    }
}

with open(conf_file, 'w') as f:
    json.dump(conf, f, indent=2)

print(f"  Generated: {conf_file}")
PYEOF
}

# ── Run ───────────────────────────────────────────────────────────────────────
SKIPPED_MCPS_FILE="$(mktemp)"
export SKIPPED_MCPS_FILE

start_docker_services
$INSTALL_CLAUDE   && install_claude
$INSTALL_OPENCODE && install_opencode

# ── Final status ──────────────────────────────────────────────────────────────
skipped_mcps=()
if [[ -s "$SKIPPED_MCPS_FILE" ]]; then
    while IFS= read -r name; do
        [[ -n "$name" ]] && skipped_mcps+=("$name")
    done < "$SKIPPED_MCPS_FILE"
fi
rm -f "$SKIPPED_MCPS_FILE"

if [[ ${#skipped_mcps[@]} -gt 0 ]]; then
    # Deduplicate
    IFS=$'\n' read -r -d '' -a skipped_mcps < <(printf '%s\n' "${skipped_mcps[@]}" | sort -u && printf '\0') || true
    echo -e "${YELLOW}${BOLD}Install incomplete.${RESET}"
    echo ""
    warn "The following MCP(s) were not installed due to missing required env vars:"
    for name in "${skipped_mcps[@]}"; do
        echo -e "  ${RED}✗${RESET} $name"
    done
    echo ""
    warn "Set the missing vars in ${BOLD}mcps/.env${RESET} and re-run ${BOLD}./install.sh${RESET}"
    echo ""
    exit 1
else
    echo -e "${GREEN}${BOLD}All done.${RESET}"
    echo ""
fi
