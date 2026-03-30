#!/usr/bin/env bash
set -euo pipefail

# ── Colors ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
RESET='\033[0m'

# ── Flags ─────────────────────────────────────────────────────────────────────
DRY_RUN=false
SELECTED_MODEL=""
REINSTALL_OPENVIKING=false
REINSTALL_JINA=false
MCPS_ONLY=false
AGENTS_ONLY=false
SKILLS_ONLY=false

for arg in "$@"; do
    case "$arg" in
        --dry-run|-n)           DRY_RUN=true ;;
        --model=*)              SELECTED_MODEL="${arg#--model=}" ;;
        --model)                shift; SELECTED_MODEL="$1" ;;
        --reinstall-openviking) REINSTALL_OPENVIKING=true ;;
        --reinstall-jina)       REINSTALL_JINA=true ;;
        --mcps-only)            MCPS_ONLY=true ;;
        --agents-only)          AGENTS_ONLY=true ;;
        --skills-only)          SKILLS_ONLY=true ;;
        --help|-h)
            echo "Usage: ./install.sh [--dry-run|-n] [--model <alias|model-id>] [--reinstall-openviking] [--reinstall-jina] [--mcps-only] [--agents-only] [--skills-only]"
            echo "  --dry-run, -n           Preview what would be installed without making changes"
            echo "  --model <value>         Override model for all agents (optional — agents inherit CLI default if omitted)"
            echo "  --reinstall-openviking  Wipe ~/.openviking/venv and reinstall from scratch"
            echo "  --reinstall-jina        Wipe Jina embeddings server and reinstall from scratch"
            echo "  --mcps-only             Only register MCP servers — skip agents, skills, and hooks"
            echo "  --agents-only           Only install agents — skip skills, hooks, and MCPs"
            echo "  --skills-only           Only install skills — skip agents, hooks, and MCPs"
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

run_cp()    { $DRY_RUN && dryrun "cp $1 $2"    || cp "$1" "$2"; }
run_mkdir() { $DRY_RUN && dryrun "mkdir -p $1" || mkdir -p "$1"; }

py() { python3 "$REPO_DIR/scripts/$1.py" "${@:2}"; }

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

[[ -d "$REPO_DIR/agents"  ]] || die "agents/ directory not found in repo."
[[ -d "$REPO_DIR/skills"  ]] || die "skills/ directory not found in repo."
[[ -d "$REPO_DIR/scripts" ]] || die "scripts/ directory not found in repo."

# ── Load devexp.config.json ───────────────────────────────────────────────────
CONFIG_FILE="$REPO_DIR/devexp.config.json"
CONFIG_DISABLED_AGENTS=()
CONFIG_DISABLED_SKILLS=()
CONFIG_DISABLED_HOOKS=()
CONFIG_MODEL=""
CONFIG_EXTRA_MCPS="[]"

if [[ -f "$CONFIG_FILE" ]] && command -v python3 &>/dev/null; then
    eval "$(py load_config "$CONFIG_FILE")"
    [[ -z "$SELECTED_MODEL" && -n "$CONFIG_MODEL" ]] && SELECTED_MODEL="$CONFIG_MODEL"
    _has_config=false
    [[ ${#CONFIG_DISABLED_AGENTS[@]} -gt 0 ]] && _has_config=true
    [[ ${#CONFIG_DISABLED_SKILLS[@]} -gt 0 ]] && _has_config=true
    [[ ${#CONFIG_DISABLED_HOOKS[@]}  -gt 0 ]] && _has_config=true
    [[ -n "$CONFIG_MODEL" ]]                   && _has_config=true
    [[ "$CONFIG_EXTRA_MCPS" != "[]" ]]         && _has_config=true
    $_has_config && info "devexp.config.json loaded — org overrides applied."
fi

# ── Helpers ───────────────────────────────────────────────────────────────────
is_disabled() {
    local name="$1"; shift
    local item
    for item in "$@"; do [[ "$item" == "$name" ]] && return 0; done
    return 1
}

backup_conflicts() {
    local backup_dir="$1"; shift
    local conflicts=("$@")
    if [[ ${#conflicts[@]} -gt 0 ]]; then
        warn "Backing up ${#conflicts[@]} existing file(s) to:"
        warn "  $backup_dir"
        mkdir -p "$backup_dir"
        for f in "${conflicts[@]}"; do
            local rel="${f#"$HOME/"}"
            local dest="$backup_dir/$rel"
            mkdir -p "$(dirname "$dest")"
            cp "$f" "$dest"
        done
        success "Backup complete."
        echo ""
    fi
}

# ── Detect installed CLIs ─────────────────────────────────────────────────────
HAS_CLAUDE=false
HAS_OPENCODE=false
command -v claude   &>/dev/null && HAS_CLAUDE=true
command -v opencode &>/dev/null && HAS_OPENCODE=true

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

if [[ -n "$SELECTED_MODEL" ]]; then
    info "Model override: ${BOLD}$SELECTED_MODEL${RESET} (will be set on all agents)"
    echo ""
fi

# ── MCP / hook installation ───────────────────────────────────────────────────
install_mcps_claude() {
    local registry="$REPO_DIR/mcps/registry.json"
    [[ -f "$registry" ]] || return 0
    command -v claude &>/dev/null || return 0
    info "Installing MCP servers (Claude Code)..."
    py install_mcps_claude "$registry" "$REPO_DIR/mcps/.env" \
        "$($DRY_RUN && echo 1 || echo 0)" "${SKIPPED_MCPS_FILE:-}"
    echo ""
}

install_extra_mcps_claude() {
    [[ "$CONFIG_EXTRA_MCPS" == "[]" ]] && return 0
    command -v claude &>/dev/null || return 0
    info "Installing extra MCP servers from devexp.config.json (Claude Code)..."
    py install_extra_mcps_claude "$CONFIG_EXTRA_MCPS" "$REPO_DIR/mcps/.env" \
        "$($DRY_RUN && echo 1 || echo 0)"
    echo ""
}

install_mcps_opencode() {
    local registry="$REPO_DIR/mcps/registry.json"
    [[ -f "$registry" ]] || return 0
    local config_path="$HOME/.config/opencode/config.json"
    info "Installing MCP servers (opencode → $config_path)..."
    py install_mcps_opencode "$registry" "$config_path" "$REPO_DIR/mcps/.env" \
        "$($DRY_RUN && echo 1 || echo 0)" "${SKIPPED_MCPS_FILE:-}"
    echo ""
}

install_extra_mcps_opencode() {
    [[ "$CONFIG_EXTRA_MCPS" == "[]" ]] && return 0
    local config_path="$HOME/.config/opencode/config.json"
    info "Installing extra MCP servers from devexp.config.json (opencode → $config_path)..."
    py install_extra_mcps_opencode "$CONFIG_EXTRA_MCPS" "$config_path" "$REPO_DIR/mcps/.env" \
        "$($DRY_RUN && echo 1 || echo 0)"
    echo ""
}

install_hooks_claude() {
    local registry="$REPO_DIR/hooks/registry.json"
    [[ -f "$registry" ]] || return 0
    local settings="$HOME/.claude/settings.json"
    local disabled_csv; disabled_csv=$(printf '%s,' "$@")
    info "Installing hooks (Claude Code)..."
    py install_hooks_claude "$registry" "$REPO_DIR" "$settings" \
        "$($DRY_RUN && echo 1 || echo 0)" "$disabled_csv"
    echo ""
}

install_hooks_opencode() {
    local registry="$REPO_DIR/hooks/registry.json"
    [[ -f "$registry" ]] || return 0
    local plugin_src="$REPO_DIR/hooks/opencode/devexp-plugin.js"
    local plugin_dir_src="$REPO_DIR/hooks/opencode"
    [[ -f "$plugin_src" ]] || return 0

    # Skip if all hooks are disabled
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
    for js_file in "$plugin_dir_src"/*.js; do
        cp "$js_file" "$plugin_dir/$(basename "$js_file")"
        echo -e "  ${GREEN}+${RESET} $(basename "$js_file") → $plugin_dir/"
    done
    if [[ -f "$plugin_dir_src/package.json" ]]; then
        cp "$plugin_dir_src/package.json" "$plugin_dir/package.json"
        echo -e "  ${GREEN}+${RESET} package.json → $plugin_dir/"
    fi

    py register_oc_plugin "$config_path" "$plugin_dest"
    echo ""
}

# ── Claude Code installation ──────────────────────────────────────────────────
install_claude() {
    local AGENTS_TARGET="$HOME/.claude/agents"
    local SKILLS_TARGET="$HOME/.claude/skills"
    local BACKUP_DIR="$HOME/.claude/.devexp-backup-$(date +%Y%m%dT%H%M%S)"

    info "Installing for Claude Code..."
    echo ""

    if $MCPS_ONLY; then
        info "MCPs only — skipping agents, skills, and hooks."
        echo ""
        install_mcps_claude
        install_extra_mcps_claude
        success "Claude Code MCP installation complete."
        echo ""
        return 0
    fi

    run_mkdir "$AGENTS_TARGET" || die "Failed to create $AGENTS_TARGET"
    run_mkdir "$SKILLS_TARGET"  || die "Failed to create $SKILLS_TARGET"

    # Collect conflicts for backup
    local conflicts=()
    if ! $SKILLS_ONLY; then
        for f in "$REPO_DIR/agents/"*.md; do
            [[ -f "$f" ]] || continue
            [[ "$(basename "$f")" == "README.md" ]] && continue
            local t="$AGENTS_TARGET/$(basename "$f")"
            [[ -f "$t" ]] && conflicts+=("$t")
        done
    fi
    if ! $AGENTS_ONLY; then
        for d in "$REPO_DIR/skills/"/*/; do
            [[ -d "$d" ]] || continue
            local t="$SKILLS_TARGET/$(basename "$d")/SKILL.md"
            [[ -f "$t" ]] && conflicts+=("$t")
        done
    fi
    $DRY_RUN || backup_conflicts "$BACKUP_DIR" "${conflicts[@]+"${conflicts[@]}"}"

    if ! $SKILLS_ONLY; then
        local count=0
        info "Installing agents..."
        for f in "$REPO_DIR/agents/"*.md; do
            [[ -f "$f" ]] || continue
            [[ "$(basename "$f")" == "README.md" ]] && continue
            local agent_name; agent_name="$(basename "$f" .md)"
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
    fi

    if ! $AGENTS_ONLY; then
        local count=0
        info "Installing skills..."
        for d in "$REPO_DIR/skills/"/*/; do
            [[ -d "$d" ]] || continue
            local skill; skill="$(basename "$d")"
            if is_disabled "$skill" "${CONFIG_DISABLED_SKILLS[@]+"${CONFIG_DISABLED_SKILLS[@]}"}"; then
                echo -e "  ${YELLOW}[skip]${RESET} $skill (disabled in devexp.config.json)"
                continue
            fi
            run_mkdir "$SKILLS_TARGET/$skill"
            if [[ -f "$d/SKILL.md" ]]; then
                run_cp "$d/SKILL.md" "$SKILLS_TARGET/$skill/SKILL.md"
                echo -e "  ${GREEN}+${RESET} $skill/SKILL.md"
                (( count++ )) || true
            fi
        done
        success "Installed $count skill(s)."
        echo ""
    fi

    if ! $AGENTS_ONLY && ! $SKILLS_ONLY; then
        install_mcps_claude
        install_extra_mcps_claude
        install_hooks_claude "${CONFIG_DISABLED_HOOKS[@]+"${CONFIG_DISABLED_HOOKS[@]}"}"
    fi

    success "Claude Code installation complete."
    echo "  Agents: $AGENTS_TARGET"
    echo "  Skills: $SKILLS_TARGET"
    echo ""
    info "Restart Claude Code to activate."
    echo ""
}

# ── opencode installation ─────────────────────────────────────────────────────
install_opencode() {
    local AGENTS_TARGET="$HOME/.config/opencode/agents"
    local SKILLS_TARGET="$HOME/.config/opencode/commands"
    local BACKUP_DIR="$HOME/.config/opencode/.devexp-backup-$(date +%Y%m%dT%H%M%S)"

    info "Installing for opencode..."
    echo ""

    if $MCPS_ONLY; then
        info "MCPs only — skipping agents, skills, and hooks."
        echo ""
        install_mcps_opencode
        install_extra_mcps_opencode
        success "opencode MCP installation complete."
        echo ""
        return 0
    fi

    if ! $SKILLS_ONLY; then
        command -v python3 &>/dev/null || die "python3 is required for opencode install (frontmatter transformation)"
    fi

    run_mkdir "$AGENTS_TARGET" || die "Failed to create $AGENTS_TARGET"
    run_mkdir "$SKILLS_TARGET"  || die "Failed to create $SKILLS_TARGET"

    # Collect conflicts for backup
    local conflicts=()
    if ! $SKILLS_ONLY; then
        for f in "$REPO_DIR/agents/"*.md; do
            [[ -f "$f" ]] || continue
            [[ "$(basename "$f")" == "README.md" ]] && continue
            local t="$AGENTS_TARGET/$(basename "$f")"
            [[ -f "$t" ]] && conflicts+=("$t")
        done
        if [[ -d "$REPO_DIR/agents/opencode" ]]; then
            for f in "$REPO_DIR/agents/opencode/"*.md; do
                [[ -f "$f" ]] || continue
                local t="$AGENTS_TARGET/$(basename "$f")"
                [[ -f "$t" ]] && conflicts+=("$t")
            done
        fi
    fi
    if ! $AGENTS_ONLY; then
        for d in "$REPO_DIR/skills/"/*/; do
            [[ -d "$d" ]] || continue
            local t="$SKILLS_TARGET/$(basename "$d").md"
            [[ -f "$t" ]] && conflicts+=("$t")
        done
    fi
    $DRY_RUN || backup_conflicts "$BACKUP_DIR" "${conflicts[@]+"${conflicts[@]}"}"

    if ! $SKILLS_ONLY; then
        local count=0
        info "Installing agents (transformed for opencode)..."
        for f in "$REPO_DIR/agents/"*.md; do
            [[ -f "$f" ]] || continue
            [[ "$(basename "$f")" == "README.md" ]] && continue
            local agent_name; agent_name="$(basename "$f" .md)"
            if is_disabled "$agent_name" "${CONFIG_DISABLED_AGENTS[@]+"${CONFIG_DISABLED_AGENTS[@]}"}"; then
                echo -e "  ${YELLOW}[skip]${RESET} $(basename "$f") (disabled in devexp.config.json)"
                continue
            fi
            if $DRY_RUN; then
                dryrun "transform + write $AGENTS_TARGET/$(basename "$f")"
            else
                py transform_agent "$f" "$SELECTED_MODEL" > "$AGENTS_TARGET/$(basename "$f")"
            fi
            echo -e "  ${GREEN}+${RESET} $(basename "$f")"
            (( count++ )) || true
        done

        # opencode-exclusive agents (already in opencode format — copy or model-substitute only)
        if [[ -d "$REPO_DIR/agents/opencode" ]]; then
            for f in "$REPO_DIR/agents/opencode/"*.md; do
                [[ -f "$f" ]] || continue
                local dest="$AGENTS_TARGET/$(basename "$f")"
                if $DRY_RUN; then
                    dryrun "write $dest"
                elif [[ -n "$SELECTED_MODEL" ]]; then
                    py transform_agent "$f" "$SELECTED_MODEL" --model-only > "$dest"
                else
                    cp "$f" "$dest"
                fi
                echo -e "  ${GREEN}+${RESET} $(basename "$f") ${YELLOW}(opencode-exclusive)${RESET}"
                (( count++ )) || true
            done
        fi

        success "Installed $count agent(s)."
        echo ""
    fi

    if ! $AGENTS_ONLY; then
        local count=0
        info "Installing skills (to ~/.config/opencode/commands)..."
        for d in "$REPO_DIR/skills/"/*/; do
            [[ -d "$d" ]] || continue
            local skill; skill="$(basename "$d")"
            if is_disabled "$skill" "${CONFIG_DISABLED_SKILLS[@]+"${CONFIG_DISABLED_SKILLS[@]}"}"; then
                echo -e "  ${YELLOW}[skip]${RESET} $skill (disabled in devexp.config.json)"
                continue
            fi
            if [[ -f "$d/SKILL.md" ]]; then
                local dest="$SKILLS_TARGET/$skill.md"
                if $DRY_RUN; then
                    dryrun "write $dest"
                else
                    # Strip 'name:' line — opencode derives name from filename
                    sed '/^name:/d' "$d/SKILL.md" > "$dest"
                fi
                echo -e "  ${GREEN}+${RESET} $skill.md"
                (( count++ )) || true
            fi
        done
        success "Installed $count skill(s)."
        echo ""
    fi

    if ! $AGENTS_ONLY && ! $SKILLS_ONLY; then
        install_mcps_opencode
        install_extra_mcps_opencode
        install_hooks_opencode "${CONFIG_DISABLED_HOOKS[@]+"${CONFIG_DISABLED_HOOKS[@]}"}"
    fi

    success "opencode installation complete."
    echo "  Agents: $AGENTS_TARGET"
    echo "  Skills: $SKILLS_TARGET"
    echo ""
    info "Restart opencode to activate."
    echo ""
}

# ── Jina embeddings server ────────────────────────────────────────────────────
JINA_PORT=8082

_setup_jina_pip() {
    local venv_dir="$HOME/.openviking/jina-venv"
    local pid_file="$HOME/.openviking/jina.pid"
    local log_file="$HOME/.openviking/jina.log"
    local python="$venv_dir/bin/python"
    local pip="$venv_dir/bin/pip"

    local python_bin
    python_bin=$(which python3.11 2>/dev/null || which python3 2>/dev/null)
    if $DRY_RUN; then
        dryrun "$python_bin -m venv $venv_dir"
    elif [[ ! -x "$python" || ! -x "$pip" ]]; then
        echo -e "  Creating Jina venv at ${BOLD}$venv_dir${RESET} (using $python_bin)..."
        rm -rf "$venv_dir"
        "$python_bin" -m venv "$venv_dir" \
            || { warn "jina: failed to create venv"; return 1; }
        echo -e "  ${GREEN}+${RESET} venv created"
    fi

    # click is pinned to <8.2 — click 8.3+ breaks infinity-emb's CLI
    if $DRY_RUN; then
        dryrun "$pip install 'infinity-emb[server,torch]==0.0.76' -q && $pip install 'click==8.1.8' -q"
    else
        echo -e "  Installing ${BOLD}infinity-emb[server,torch]${RESET} (first run downloads ~200MB model)..."
        "$pip" install "infinity-emb[server,torch]==0.0.76" -q \
            && "$pip" install "click==8.1.8" -q \
            && echo -e "  ${GREEN}+${RESET} infinity-emb installed" \
            || { warn "jina: pip install failed"; return 1; }
    fi

    if $DRY_RUN; then
        dryrun "nohup $venv_dir/bin/infinity_emb v2 ... --port $JINA_PORT > $log_file 2>&1 &"
        return 0
    fi

    nohup "$venv_dir/bin/infinity_emb" v2 \
        --model-id jinaai/jina-embeddings-v2-base-en \
        --engine torch \
        --no-bettertransformer \
        --port "$JINA_PORT" \
        --host 0.0.0.0 \
        > "$log_file" 2>&1 &
    echo $! > "$pid_file"
    echo -e "  ${GREEN}+${RESET} Jina embeddings server started via pip (pid $(cat "$pid_file"), port $JINA_PORT)"
    echo -e "  ${YELLOW}[note]${RESET} First run downloads the model — check $log_file if slow"
}

_setup_jina_docker() {
    local container="devexp-jina-embed"
    local pid_file="$HOME/.openviking/jina.pid"

    if $DRY_RUN; then
        dryrun "docker run -d --name $container -p 127.0.0.1:$JINA_PORT:80 ghcr.io/huggingface/text-embeddings-inference:cpu-1.8 --model-id jinaai/jina-embeddings-v2-base-en"
        return 0
    fi

    docker rm -f "$container" 2>/dev/null || true
    docker run -d \
        --name "$container" \
        --restart unless-stopped \
        -p "127.0.0.1:$JINA_PORT:80" \
        ghcr.io/huggingface/text-embeddings-inference:cpu-1.8 \
        --model-id jinaai/jina-embeddings-v2-base-en \
        && echo -e "  ${GREEN}+${RESET} Jina embeddings server started via Docker (port $JINA_PORT)" \
        || { warn "jina: docker run failed — falling back to pip"; _setup_jina_pip; return; }

    echo "docker:$container" > "$pid_file"
}

_setup_jina_embeddings() {
    local pid_file="$HOME/.openviking/jina.pid"

    mkdir -p "$HOME/.openviking"

    if $REINSTALL_JINA; then
        info "jina: reinstalling from scratch..."
        if [[ -f "$pid_file" ]]; then
            local entry; entry=$(cat "$pid_file")
            if [[ "$entry" == docker:* ]]; then
                docker rm -f "${entry#docker:}" 2>/dev/null && echo -e "  Removed Docker container"
            else
                kill "$entry" 2>/dev/null && echo -e "  Stopped process (pid $entry)"
            fi
            rm -f "$pid_file"
        fi
        rm -rf "$HOME/.openviking/jina-venv"
        echo -e "  ${GREEN}+${RESET} wiped Jina install"
    fi

    if ! $REINSTALL_JINA && [[ -f "$pid_file" ]]; then
        local entry; entry=$(cat "$pid_file")
        if [[ "$entry" == docker:* ]]; then
            local cname="${entry#docker:}"
            if docker ps --filter "name=$cname" --format '{{.Names}}' 2>/dev/null | grep -q "$cname"; then
                echo -e "  ${YELLOW}[running]${RESET} Jina embeddings Docker container already up — skipping"
                echo -e "  To reinstall: ./install.sh --reinstall-jina"
                return 0
            fi
        elif kill -0 "$entry" 2>/dev/null; then
            echo -e "  ${YELLOW}[running]${RESET} Jina embeddings server already up (pid $entry) — skipping"
            echo -e "  To reinstall: ./install.sh --reinstall-jina"
            return 0
        fi
        rm -f "$pid_file"
    fi

    if ! $REINSTALL_JINA && lsof -ti:"$JINA_PORT" >/dev/null 2>&1; then
        echo -e "  ${YELLOW}[running]${RESET} Port $JINA_PORT already in use — assuming Jina is running, skipping"
        return 0
    fi

    info "Setting up Jina embeddings server (port $JINA_PORT)..."
    local os_type; os_type=$(uname -s)
    if [[ "$os_type" == "Darwin" ]]; then
        echo -e "  macOS detected — using pip (infinity-emb)"
        _setup_jina_pip
    elif command -v docker &>/dev/null; then
        echo -e "  Linux + Docker detected — using HuggingFace TEI image"
        _setup_jina_docker
    else
        echo -e "  Linux (no Docker) — using pip (infinity-emb)"
        _setup_jina_pip
    fi
}

# ── OpenViking setup ──────────────────────────────────────────────────────────
_setup_openviking() {
    local conf_file="$HOME/.openviking/ov.conf"
    local server_script="$REPO_DIR/mcps/openviking/server.py"
    local venv_dir="$HOME/.openviking/venv"
    local pid_file="$HOME/.openviking/mcp.pid"
    local log_file="$HOME/.openviking/mcp.log"
    local python="$venv_dir/bin/python"
    local pip="$venv_dir/bin/pip"

    local dotenv_file="$REPO_DIR/mcps/.env"
    local vlm_key="${OPENVIKING_VLM_API_KEY:-}"
    local vlm_model="${OPENVIKING_VLM_MODEL:-}"

    if [[ -f "$dotenv_file" ]]; then
        _val() { grep -E "^$1=" "$dotenv_file" | tail -1 | cut -d= -f2- | tr -d '\r' | xargs; }
        [[ -z "$vlm_key"   ]] && vlm_key="$(_val OPENVIKING_VLM_API_KEY)"
        [[ -z "$vlm_model" ]] && vlm_model="$(_val OPENVIKING_VLM_MODEL)"
    fi

    if [[ -z "$vlm_key" || -z "$vlm_model" ]]; then
        warn "openviking: skipping — OPENVIKING_VLM_API_KEY and OPENVIKING_VLM_MODEL must be set in mcps/.env"
        return 0
    fi

    mkdir -p "$HOME/.openviking"

    if $REINSTALL_OPENVIKING; then
        info "openviking: reinstalling from scratch..."
        if [[ -f "$pid_file" ]]; then
            local old_pid; old_pid=$(cat "$pid_file")
            kill "$old_pid" 2>/dev/null && echo -e "  Stopped running server (pid $old_pid)"
            rm -f "$pid_file"
        fi
        rm -rf "$venv_dir"
        rm -f "$conf_file"
        echo -e "  ${GREEN}+${RESET} wiped venv and config"
    fi

    if ! $REINSTALL_OPENVIKING && [[ -f "$pid_file" ]]; then
        local running_pid; running_pid=$(cat "$pid_file")
        if kill -0 "$running_pid" 2>/dev/null; then
            echo -e "  ${YELLOW}[running]${RESET} openviking MCP server already up (pid $running_pid) — skipping"
            echo -e "  To reinstall: ./install.sh --reinstall-openviking"
            return 0
        fi
        rm -f "$pid_file"
    fi

    if $DRY_RUN; then
        dryrun "python3 -m venv $venv_dir"
    elif [[ ! -x "$python" || ! -x "$pip" ]]; then
        echo -e "  Creating venv at ${BOLD}$venv_dir${RESET}..."
        rm -rf "$venv_dir"
        python3 -m venv "$venv_dir" \
            || { warn "openviking: failed to create venv"; return 1; }
        echo -e "  ${GREEN}+${RESET} venv created"
    fi

    if $DRY_RUN; then
        dryrun "$pip install openviking mcp --upgrade --force-reinstall"
    else
        echo -e "  Installing ${BOLD}openviking${RESET} + mcp into venv..."
        "$pip" install openviking mcp --upgrade --force-reinstall -q \
            && echo -e "  ${GREEN}+${RESET} packages installed" \
            || { warn "openviking: pip install failed"; return 1; }
    fi

    if [[ ! -f "$conf_file" ]]; then
        if $DRY_RUN; then
            dryrun "generate $conf_file"
        else
            py gen_ov_conf "$conf_file" "$vlm_key" "$vlm_model" "$JINA_PORT"
        fi
    fi

    if $DRY_RUN; then
        dryrun "nohup $python $server_script --config $conf_file > $log_file 2>&1 &"
        return 0
    fi

    if [[ -f "$pid_file" ]]; then
        local old_pid; old_pid=$(cat "$pid_file")
        if kill -0 "$old_pid" 2>/dev/null; then
            kill "$old_pid" 2>/dev/null
            sleep 1
        fi
        rm -f "$pid_file"
    fi

    nohup "$python" "$server_script" \
        --config "$conf_file" \
        --data "$HOME/.openviking/data" \
        > "$log_file" 2>&1 &
    echo $! > "$pid_file"
    echo -e "  ${GREEN}+${RESET} openviking MCP server started (pid $(cat "$pid_file"), log: $log_file)"
}

# ── Docker services ───────────────────────────────────────────────────────────
start_docker_services() {
    local registry="$REPO_DIR/mcps/registry.json"
    [[ -f "$registry" ]] || return 0
    command -v docker &>/dev/null || return 0

    local services
    services=$(py docker_services "$registry" "$REPO_DIR/mcps/.env")
    [[ -z "$services" ]] && return 0

    info "Starting Docker services..."
    while IFS='|' read -r name compose_rel; do
        local compose_file="$REPO_DIR/$compose_rel"
        [[ -f "$compose_file" ]] || continue
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

# ── Run ───────────────────────────────────────────────────────────────────────
SKIPPED_MCPS_FILE="$(mktemp)"
export SKIPPED_MCPS_FILE

# Service setup only runs when MCPs are in scope
if ! $AGENTS_ONLY && ! $SKILLS_ONLY; then
    start_docker_services
    _setup_jina_embeddings
    _setup_openviking
fi

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
