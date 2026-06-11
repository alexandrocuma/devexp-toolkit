package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"devexp/internal/agents"
	"devexp/internal/config"
	"devexp/internal/hooks"
	"devexp/internal/mcp"
	"devexp/internal/repo"
	"devexp/internal/skills"
	"devexp/internal/ui"
)

var (
	flagDryRun        bool
	flagModel         string
	flagReinstallMCPs bool
	flagMCPsOnly      bool
	flagAgentsOnly    bool
	flagSkillsOnly    bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install devexp agents, skills, hooks, and MCP servers",
	RunE:  runInstall,
}

func init() {
	f := installCmd.Flags()
	f.BoolVarP(&flagDryRun, "dry-run", "n", false, "Preview without making changes")
	f.StringVar(&flagModel, "model", "", "Override model for all agents (sonnet, opus, haiku, gpt4o, deepseek, kimi, or full model ID)")
	f.BoolVar(&flagReinstallMCPs, "reinstall-mcps", false, "Remove registry MCPs then re-add them (forces config refresh)")
	f.BoolVar(&flagMCPsOnly, "mcps-only", false, "Only register MCP servers — skip agents, skills, and hooks")
	f.BoolVar(&flagAgentsOnly, "agents-only", false, "Only install agents — skip skills, hooks, and MCPs")
	f.BoolVar(&flagSkillsOnly, "skills-only", false, "Only install skills — skip agents, hooks, and MCPs")
	rootCmd.AddCommand(installCmd)
}

// installOpts carries all resolved installation parameters.
type installOpts struct {
	repoDir        string
	cfg            *config.Config
	env            map[string]string
	dryRun         bool
	reinstallMCPs  bool
	mcpsOnly       bool
	agentsOnly     bool
	skillsOnly     bool
	selectedAgents []string // nil = all (respects cfg.DisabledAgents); non-nil = explicit list
	selectedMCPs   []string // nil = all; non-nil = explicit list from wizard
	selectedHooks  []string // nil = all (respects cfg.DisabledHooks); non-nil = explicit list from wizard
}

// wizardResult holds the answers collected from the interactive wizard.
type wizardResult struct {
	dryRun          bool
	reinstallMCPs   bool
	remove          bool
	mcpsOnly        bool
	agentsOnly      bool
	skillsOnly      bool
	installClaude   bool
	installOpencode bool
	selectedAgents  []string
	selectedMCPs    []string
	selectedHooks   []string
}

// ── Entry point ───────────────────────────────────────────────────────────────

func runInstall(cmd *cobra.Command, args []string) error {
	fmt.Println()
	fmt.Println("\033[1mdevexp Framework Installer\033[0m")
	fmt.Println("────────────────────────────────────────")
	fmt.Println()

	src, err := repo.Resolve(version)
	if err != nil {
		return err
	}
	repoDir := src.RepoDir
	if src.Embedded {
		ui.Info("Running standalone — using assets bundled in this binary.")
		fmt.Println()
	}

	cfg, err := config.Load(filepath.Join(repoDir, "devexp.config.json"))
	if err != nil {
		ui.Warn("devexp.config.json not found — using defaults")
		cfg = &config.Config{}
	}
	if flagModel != "" {
		cfg.Model = flagModel
	}

	dotenv, _ := config.LoadDotenv(filepath.Join(repoDir, "mcps", ".env"))
	if dotenv == nil {
		dotenv = map[string]string{}
	} else if len(dotenv) > 0 {
		ui.Info(fmt.Sprintf("Loaded %d var(s) from mcps/.env", len(dotenv)))
		fmt.Println()
	}

	env := buildEnv(dotenv, repoDir)

	// ── Decide: interactive wizard or flag-based path ─────────────────────────
	flagsProvided := cmd.Flags().Changed("dry-run") ||
		cmd.Flags().Changed("reinstall-mcps") ||
		cmd.Flags().Changed("mcps-only") ||
		cmd.Flags().Changed("agents-only") ||
		cmd.Flags().Changed("skills-only")

	var opts *installOpts
	installClaude := false
	installOpencode := false

	if flagsProvided {
		// Non-interactive: use flags directly (CI / scripting path)
		if flagDryRun {
			fmt.Println("\033[1;33mDRY RUN MODE — no files will be written\033[0m")
			fmt.Println()
		}
		installClaude, installOpencode, err = detectTargets()
		if err != nil {
			return err
		}
		fmt.Println()
		opts = &installOpts{
			repoDir:       repoDir,
			cfg:           cfg,
			env:           env,
			dryRun:        flagDryRun,
			reinstallMCPs: flagReinstallMCPs,
			mcpsOnly:      flagMCPsOnly,
			agentsOnly:    flagAgentsOnly,
			skillsOnly:    flagSkillsOnly,
		}
	} else {
		// Interactive wizard
		registry, _ := loadFullRegistry(repoDir, cfg)
		agentNames := listAgentNames(repoDir)

		wiz, err := runWizard(repoDir, registry, agentNames)
		if err != nil {
			return err
		}

		if wiz.remove {
			return runRemove(repoDir)
		}

		if wiz.dryRun {
			fmt.Println("\033[1;33mDRY RUN MODE — no files will be written\033[0m")
			fmt.Println()
		}

		installClaude = wiz.installClaude
		installOpencode = wiz.installOpencode
		opts = &installOpts{
			repoDir:        repoDir,
			cfg:            cfg,
			env:            env,
			dryRun:         wiz.dryRun,
			reinstallMCPs:  wiz.reinstallMCPs,
			mcpsOnly:       wiz.mcpsOnly,
			agentsOnly:     wiz.agentsOnly,
			skillsOnly:     wiz.skillsOnly,
			selectedAgents: wiz.selectedAgents,
			selectedMCPs:   wiz.selectedMCPs,
			selectedHooks:  wiz.selectedHooks,
		}
	}

	if cfg.Model != "" {
		ui.Info(fmt.Sprintf("Model: \033[1m%s\033[0m", cfg.Model))
		fmt.Println()
	}

	if installClaude {
		if err := doInstallClaude(opts); err != nil {
			return fmt.Errorf("claude install: %w", err)
		}
	}
	if installOpencode {
		if err := doInstallOpencode(opts); err != nil {
			return fmt.Errorf("opencode install: %w", err)
		}
	}

	fmt.Printf("\033[0;32m\033[1mAll done.\033[0m\n\n")
	return nil
}

// ── Interactive wizard ────────────────────────────────────────────────────────

func runWizard(repoDir string, registry []mcp.MCP, agentNames []string) (*wizardResult, error) {
	result := &wizardResult{}

	// ── Pre-flight: Action ────────────────────────────────────────────────────
	action, err := ui.SelectAction()
	if err != nil {
		return nil, err
	}
	fmt.Println()

	switch action {
	case ui.ActionDryRun:
		result.dryRun = true
	case ui.ActionReinstallMCPs:
		result.reinstallMCPs = true
	case ui.ActionRemove:
		result.remove = true
		return result, nil
	}

	// ── Pre-flight: Scope ─────────────────────────────────────────────────────
	scope, err := ui.SelectScope()
	if err != nil {
		return nil, err
	}
	result.mcpsOnly = scope == ui.ScopeMCPsOnly
	result.agentsOnly = scope == ui.ScopeAgentsOnly
	result.skillsOnly = scope == ui.ScopeSkillsOnly
	fmt.Println()

	// ── Section 1: Platform ───────────────────────────────────────────────────
	hasClaude := commandExists("claude")
	hasOpencode := commandExists("opencode")

	switch {
	case hasClaude && hasOpencode:
		ui.Info("Detected: Claude Code and opencode")
		fmt.Println()
		choice, err := ui.SelectPlatform()
		if err != nil {
			return nil, err
		}
		result.installClaude = choice == "Claude Code" || choice == "Both"
		result.installOpencode = choice == "opencode" || choice == "Both"
	case hasClaude:
		ui.Info("Detected: Claude Code")
		result.installClaude = true
	case hasOpencode:
		ui.Info("Detected: opencode")
		result.installOpencode = true
	default:
		return nil, fmt.Errorf("no supported CLI detected (claude or opencode)")
	}
	fmt.Println()

	// ── Section 2: Agents ─────────────────────────────────────────────────────
	if !result.mcpsOnly && !result.skillsOnly && len(agentNames) > 0 {
		selectedAgents, err := ui.MultiSelect("Agents to install", agentNames)
		if err != nil {
			return nil, err
		}
		result.selectedAgents = selectedAgents
		fmt.Println()
	}

	// ── Section 3: MCPs ───────────────────────────────────────────────────────
	if !result.agentsOnly && !result.skillsOnly && len(registry) > 0 {
		mcpNames := make([]string, len(registry))
		for i, m := range registry {
			mcpNames[i] = m.Name
		}
		selectedMCPs, err := ui.MultiSelect("MCPs to install", mcpNames)
		if err != nil {
			return nil, err
		}
		result.selectedMCPs = selectedMCPs
		fmt.Println()
	}

	// ── Section 4: Hooks ──────────────────────────────────────────────────────
	if !result.mcpsOnly && !result.agentsOnly && !result.skillsOnly {
		hookNames := listHookNames(repoDir)
		if len(hookNames) > 0 {
			selectedHooks, err := ui.MultiSelect("Hooks to install", hookNames)
			if err != nil {
				return nil, err
			}
			result.selectedHooks = selectedHooks
			fmt.Println()
		}
	}

	return result, nil
}

// ── Remove ────────────────────────────────────────────────────────────────────

func runRemove(repoDir string) error {
	script := filepath.Join(repoDir, "uninstall.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("uninstall.sh not found at %s", script)
	}
	cmd := exec.Command("/bin/bash", script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ── CLI auto-detect (flag path) ───────────────────────────────────────────────

func detectTargets() (claude, opencode bool, err error) {
	hasClaude := commandExists("claude")
	hasOpencode := commandExists("opencode")

	switch {
	case hasClaude && hasOpencode:
		ui.Info("Detected: Claude Code and opencode")
		fmt.Println()
		choice, err := ui.SelectPlatform()
		if err != nil {
			return false, false, err
		}
		return choice == "Claude Code" || choice == "Both",
			choice == "opencode" || choice == "Both",
			nil
	case hasClaude:
		ui.Info("Detected: Claude Code")
		return true, false, nil
	case hasOpencode:
		ui.Info("Detected: opencode")
		return false, true, nil
	default:
		return false, false, fmt.Errorf("no supported CLI detected (claude or opencode)")
	}
}

// ── Claude Code ───────────────────────────────────────────────────────────────

func doInstallClaude(opts *installOpts) error {
	home := os.Getenv("HOME")
	agentsTarget := filepath.Join(home, ".claude", "agents")
	skillsTarget := filepath.Join(home, ".claude", "skills")
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	backupDir := filepath.Join(home, ".claude", ".devexp-backup-"+time.Now().Format("20060102T150405"))

	ui.Info("Installing for Claude Code...")
	fmt.Println()

	if opts.mcpsOnly {
		ui.Info("MCPs only — skipping agents, skills, and hooks.")
		fmt.Println()
		return installMCPsClaude(opts)
	}

	if !opts.agentsOnly && !opts.skillsOnly {
		if err := installMCPsClaude(opts); err != nil {
			return err
		}
	}

	if !opts.skillsOnly {
		backupExisting(agentsTarget, "*.md", backupDir, opts.dryRun)
		ui.Info("Installing agents...")
		disabled := resolveAgentDisabled(opts.repoDir, opts.selectedAgents, opts.cfg.DisabledAgents)
		count, err := agents.InstallClaude(
			filepath.Join(opts.repoDir, "agents"),
			agentsTarget,
			opts.cfg.Model,
			disabled,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Installed %d agent(s).", count))
		fmt.Println()
	}

	if !opts.agentsOnly {
		ui.Info("Installing skills...")
		count, err := skills.InstallClaude(
			filepath.Join(opts.repoDir, "skills"),
			skillsTarget,
			opts.cfg.DisabledSkills,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Installed %d skill(s).", count))
		fmt.Println()
	}

	if !opts.agentsOnly && !opts.skillsOnly {
		registry, err := hooks.LoadRegistry(filepath.Join(opts.repoDir, "hooks", "registry.json"))
		if err != nil {
			ui.Warn(fmt.Sprintf("hooks registry: %v", err))
		} else {
			ui.Info("Installing hooks (Claude Code)...")
			disabled := resolveHookDisabled(registry, opts.selectedHooks, opts.cfg.DisabledHooks)
			if err := hooks.InstallClaude(registry, opts.repoDir, settingsPath, disabled, opts.dryRun); err != nil {
				return err
			}
			fmt.Println()
		}
	}

	ui.Success("Claude Code installation complete.")
	fmt.Printf("  Agents : %s\n", agentsTarget)
	fmt.Printf("  Skills : %s\n", skillsTarget)
	fmt.Println()
	ui.Info("Restart Claude Code to activate.")
	fmt.Println()
	return nil
}

// ── opencode ──────────────────────────────────────────────────────────────────

func doInstallOpencode(opts *installOpts) error {
	home := os.Getenv("HOME")
	agentsTarget := filepath.Join(home, ".config", "opencode", "agents")
	skillsTarget := filepath.Join(home, ".config", "opencode", "commands")
	configPath := filepath.Join(home, ".config", "opencode", "config.json")

	ui.Warn("opencode installs a feature subset — multi-agent orchestration (Agent/Skill/Task tools), persistent memory, and terminal colors are unavailable. Orchestrator skills like /deliver and /improve run in degraded mode. Claude Code is recommended for full functionality.")
	fmt.Println()
	ui.Info("Installing for opencode...")
	fmt.Println()

	if opts.mcpsOnly {
		ui.Info("MCPs only — skipping agents, skills, and hooks.")
		fmt.Println()
		return installMCPsOpencode(opts, configPath)
	}

	if !opts.agentsOnly && !opts.skillsOnly {
		if err := installMCPsOpencode(opts, configPath); err != nil {
			return err
		}
	}

	if !opts.skillsOnly {
		ui.Info("Installing agents (transformed for opencode)...")
		disabled := resolveAgentDisabled(opts.repoDir, opts.selectedAgents, opts.cfg.DisabledAgents)
		count, err := agents.InstallOpencode(
			filepath.Join(opts.repoDir, "agents"),
			agentsTarget,
			opts.cfg.Model,
			disabled,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		excl, err := agents.InstallOpencodeExclusive(
			filepath.Join(opts.repoDir, "agents", "opencode"),
			agentsTarget,
			opts.cfg.Model,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Installed %d agent(s).", count+excl))
		fmt.Println()
	}

	if !opts.agentsOnly {
		ui.Info("Installing skills (to ~/.config/opencode/commands)...")
		count, err := skills.InstallOpencode(
			filepath.Join(opts.repoDir, "skills"),
			skillsTarget,
			opts.cfg.DisabledSkills,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Installed %d skill(s).", count))
		fmt.Println()
	}

	ui.Success("opencode installation complete.")
	fmt.Printf("  Agents : %s\n", agentsTarget)
	fmt.Printf("  Skills : %s\n", skillsTarget)
	fmt.Println()
	ui.Info("Restart opencode to activate.")
	fmt.Println()
	return nil
}

// ── MCP helpers ───────────────────────────────────────────────────────────────

func installMCPsClaude(opts *installOpts) error {
	registry, err := loadFullRegistry(opts.repoDir, opts.cfg)
	if err != nil {
		return err
	}
	registry = filterMCPs(registry, opts.selectedMCPs)

	ui.Info("Installing MCP servers (Claude Code)...")
	if err := mcp.InstallClaude(registry, opts.env, opts.dryRun, opts.reinstallMCPs); err != nil {
		return err
	}
	fmt.Println()
	return nil
}

func installMCPsOpencode(opts *installOpts, configPath string) error {
	registry, err := loadFullRegistry(opts.repoDir, opts.cfg)
	if err != nil {
		return err
	}
	registry = filterMCPs(registry, opts.selectedMCPs)

	ui.Info(fmt.Sprintf("Installing MCP servers (opencode → %s)...", configPath))
	if err := mcp.InstallOpencode(registry, opts.env, configPath, opts.dryRun, opts.reinstallMCPs); err != nil {
		return err
	}
	fmt.Println()
	return nil
}

// loadFullRegistry reads the registry and appends any extra MCPs from config.
func loadFullRegistry(repoDir string, cfg *config.Config) ([]mcp.MCP, error) {
	registry, err := mcp.LoadRegistry(filepath.Join(repoDir, "mcps", "registry.json"))
	if err != nil {
		return nil, fmt.Errorf("load MCP registry: %w", err)
	}
	if len(cfg.ExtraMCPs) > 0 {
		extra, err := mcp.LoadFromRaw(cfg.ExtraMCPs)
		if err != nil {
			ui.Warn(fmt.Sprintf("extra MCPs from config: %v", err))
		} else {
			registry = append(registry, extra...)
		}
	}
	return registry, nil
}

// filterMCPs returns only MCPs whose names are in selected (nil = no filter).
func filterMCPs(registry []mcp.MCP, selected []string) []mcp.MCP {
	if selected == nil {
		return registry
	}
	sel := make(map[string]bool, len(selected))
	for _, name := range selected {
		sel[name] = true
	}
	var out []mcp.MCP
	for _, m := range registry {
		if sel[m.Name] {
			out = append(out, m)
		}
	}
	return out
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func buildEnv(dotenv map[string]string, repoDir string) map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		k, v, _ := strings.Cut(e, "=")
		env[k] = v
	}
	env["DEVEXP_DIR"] = repoDir
	for k, v := range dotenv {
		env[k] = v
	}
	return env
}

// listAgentNames reads the agents directory and returns sorted agent names.
func listAgentNames(repoDir string) []string {
	entries, err := os.ReadDir(filepath.Join(repoDir, "agents"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		if name == "README" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// listHookNames reads the hooks registry and returns the names of enabled hooks.
func listHookNames(repoDir string) []string {
	registry, err := hooks.LoadRegistry(filepath.Join(repoDir, "hooks", "registry.json"))
	if err != nil {
		return nil
	}
	var names []string
	for _, h := range registry {
		if h.Enabled {
			names = append(names, h.Name)
		}
	}
	return names
}

// resolveHookDisabled builds the disabled list for the hooks installer.
// When selected is non-nil (wizard path), enabled hooks not in selected are disabled.
// When selected is nil (flag path), falls back to cfgDisabled from config.
func resolveHookDisabled(registry hooks.Registry, selected, cfgDisabled []string) []string {
	if selected == nil {
		return cfgDisabled
	}
	sel := make(map[string]bool, len(selected))
	for _, name := range selected {
		sel[name] = true
	}
	var disabled []string
	for _, h := range registry {
		if h.Enabled && !sel[h.Name] {
			disabled = append(disabled, h.Name)
		}
	}
	return disabled
}

// resolveAgentDisabled builds the disabled list for the agent installer.
// When selected is non-nil (wizard path), agents not in selected are disabled.
// When selected is nil (flag path), falls back to cfgDisabled from config.
func resolveAgentDisabled(repoDir string, selected, cfgDisabled []string) []string {
	if selected == nil {
		return cfgDisabled
	}
	sel := make(map[string]bool, len(selected))
	for _, name := range selected {
		sel[name] = true
	}
	all := listAgentNames(repoDir)
	var disabled []string
	for _, name := range all {
		if !sel[name] {
			disabled = append(disabled, name)
		}
	}
	return disabled
}

// backupExisting copies pre-existing files matching glob into backupDir.
func backupExisting(dir, pattern, backupDir string, dryRun bool) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 || dryRun {
		return
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return
	}
	for _, src := range matches {
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		os.WriteFile(filepath.Join(backupDir, filepath.Base(src)), data, 0644) //nolint:errcheck
	}
}
