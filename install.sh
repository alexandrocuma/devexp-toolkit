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
for arg in "$@"; do
    case "$arg" in
        --dry-run|-n) DRY_RUN=true ;;
        --help|-h)
            echo "Usage: ./install.sh [--dry-run|-n]"
            echo "  --dry-run, -n   Preview what would be installed without making changes"
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
        run_cp "$f" "$AGENTS_TARGET/$(basename "$f")"
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
        run_mkdir "$SKILLS_TARGET/$skill"
        if [[ -f "$d/skill.md" ]]; then
            run_cp "$d/skill.md" "$SKILLS_TARGET/$skill/skill.md"
            echo -e "  ${GREEN}+${RESET} $skill/skill.md"
            (( count++ )) || true
        fi
    done
    success "Installed $count skill(s)."
    echo ""

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
#   - Maps:   model aliases  (sonnet → anthropic/claude-sonnet-4-20250514, etc.)
#   - Maps:   tools list → YAML object disabling tools not in the list
#   - Adds:   mode: subagent
transform_agent_for_opencode() {
    local src="$1"
    python3 - "$src" <<'PYEOF'
import re, sys

OPENCODE_TOOLS = {'read', 'write', 'edit', 'bash', 'glob', 'grep', 'webfetch', 'websearch'}

CLAUDE_TO_OC = {
    'read': 'read', 'write': 'write', 'edit': 'edit', 'bash': 'bash',
    'glob': 'glob', 'grep': 'grep', 'webfetch': 'webfetch', 'websearch': 'websearch',
}

MODEL_MAP = {
    'sonnet': 'anthropic/claude-sonnet-4-5',
    'opus':   'anthropic/claude-opus-4-5',
    'haiku':  'anthropic/claude-haiku-4-5',
}

SKIP_KEYS = {'name', 'color', 'memory'}

with open(sys.argv[1]) as f:
    content = f.read()

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
        new_lines.append(f'model: {MODEL_MAP.get(val, val)}')
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
    # opencode reads skills from ~/.claude/skills as a compatibility fallback
    local SKILLS_TARGET="$HOME/.claude/skills"
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
        local t="$SKILLS_TARGET/$(basename "$d")/skill.md"
        [[ -f "$t" ]] && conflicts+=("$t")
    done

    $DRY_RUN || backup_conflicts "$BACKUP_DIR" "${conflicts[@]+"${conflicts[@]}"}"

    # Install shared agents (transformed from Claude Code format; skip READMEs)
    local count=0
    info "Installing agents (transformed for opencode)..."
    for f in "$REPO_DIR/agents/"*.md; do
        [[ -f "$f" ]] || continue
        [[ "$(basename "$f")" == "README.md" ]] && continue
        if $DRY_RUN; then
            dryrun "transform + write $AGENTS_TARGET/$(basename "$f")"
        else
            transform_agent_for_opencode "$f" > "$AGENTS_TARGET/$(basename "$f")"
        fi
        echo -e "  ${GREEN}+${RESET} $(basename "$f")"
        (( count++ )) || true
    done

    # Install opencode-exclusive agents (already in opencode format — no transform)
    if [[ -d "$REPO_DIR/agents/opencode" ]]; then
        for f in "$REPO_DIR/agents/opencode/"*.md; do
            [[ -f "$f" ]] || continue
            run_cp "$f" "$AGENTS_TARGET/$(basename "$f")"
            echo -e "  ${GREEN}+${RESET} $(basename "$f") ${YELLOW}(opencode-exclusive)${RESET}"
            (( count++ )) || true
        done
    fi
    success "Installed $count agent(s)."
    echo ""

    # Install skills (shared path — opencode reads ~/.claude/skills natively)
    count=0
    info "Installing skills (to ~/.claude/skills — opencode reads this natively)..."
    for d in "$REPO_DIR/skills/"/*/; do
        [[ -d "$d" ]] || continue
        local skill="$(basename "$d")"
        run_mkdir "$SKILLS_TARGET/$skill"
        if [[ -f "$d/skill.md" ]]; then
            run_cp "$d/skill.md" "$SKILLS_TARGET/$skill/skill.md"
            echo -e "  ${GREEN}+${RESET} $skill/skill.md"
            (( count++ )) || true
        fi
    done
    success "Installed $count skill(s)."
    echo ""

    success "opencode installation complete."
    echo "  Agents: $AGENTS_TARGET"
    echo "  Skills: $SKILLS_TARGET (shared path)"
    echo ""
    info "Restart opencode to activate."
    echo ""
}

# ── Run ───────────────────────────────────────────────────────────────────────
$INSTALL_CLAUDE   && install_claude
$INSTALL_OPENCODE && install_opencode

echo -e "${GREEN}${BOLD}All done.${RESET}"
echo ""
