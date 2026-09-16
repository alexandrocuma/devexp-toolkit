package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"devexp/internal/config"
	"devexp/internal/repo"
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
