package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	flagTargets       []string
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
	f.StringSliceVar(&flagTargets, "target", nil, "Install only for these CLIs ("+targetIDs()+"); repeatable or comma-separated. Default: every detected CLI")
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
	dryRun         bool
	reinstallMCPs  bool
	remove         bool
	mcpsOnly       bool
	agentsOnly     bool
	skillsOnly     bool
	targets        []target
	selectedAgents []string
	selectedMCPs   []string
	selectedHooks  []string
}

// installers dispatches each target to its installer. Adding a CLI is adding
// an entry here and a case in the target type, not a new branch in runInstall.
var installers = map[target]func(*installOpts) error{
	targetClaude:   doInstallClaude,
	targetOpencode: doInstallOpencode,
	targetKimi:     doInstallKimi,
}

// notYetSupported lists targets whose installer writes nothing yet, so a run
// that selected only these can be told it installed nothing instead of being
// congratulated. #112-#114 remove the Kimi entry as they fill it in.
var notYetSupported = map[target]bool{targetKimi: true}

// ── Entry point ───────────────────────────────────────────────────────────────

func runInstall(cmd *cobra.Command, args []string) error {
	// Before anything else, flags and wizard alike: repo.Resolve may already
	// write (a standalone binary extracts its assets under the user cache dir,
	// which a relative HOME puts under the current directory), and MCP
	// registration runs before any target path is built.
	if _, err := targetHome(os.Getenv("HOME")); err != nil {
		return fmt.Errorf("%w — refusing to install anything; set HOME and re-run", err)
	}

	fmt.Println()
	fmt.Println("\033[1mdevexp Framework Installer\033[0m")
	fmt.Println("────────────────────────────────────────")
	fmt.Println()

	// The asset root is printed as soon as it is decided: before a standalone
	// binary extracts its assets, and before anything is installed from it.
	src, err := repo.Resolve(version, announceAssetRoot)
	if err != nil {
		return err
	}
	repoDir := src.RepoDir

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
		cmd.Flags().Changed("skills-only") ||
		cmd.Flags().Changed("target")

	var opts *installOpts
	var targets []target

	if flagsProvided {
		// Non-interactive: use flags directly (CI / scripting path)
		if flagDryRun {
			fmt.Println("\033[1;33mDRY RUN MODE — no files will be written\033[0m")
			fmt.Println()
		}
		det := detectTargets()
		announceTargets(det)
		targets, err = resolveTargets(det, flagTargets)
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

		targets = wiz.targets
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

	installed := 0
	for _, t := range targets {
		install, ok := installers[t]
		if !ok {
			return fmt.Errorf("no installer for target %q", string(t))
		}
		if err := install(opts); err != nil {
			return fmt.Errorf("%s install: %w", string(t), err)
		}
		if !notYetSupported[t] {
			installed++
		}
	}

	// A run whose every target installs nothing yet has not succeeded, whatever
	// each installer printed on its way past. It exits non-zero rather than
	// letting "All done." stand in for an install that never happened.
	if skipped := skippedTargets(targets); len(skipped) > 0 {
		if installed == 0 {
			return fmt.Errorf("nothing was installed: %s", labelList(skipped)+" is not a supported install target yet (#110)")
		}
		ui.Warn("Skipped: " + labelList(skipped) + " — not a supported install target yet (#110).")
		fmt.Println()
	}

	fmt.Printf("\033[0;32m\033[1mAll done.\033[0m\n\n")
	return nil
}

// skippedTargets returns the selected targets that installed nothing.
func skippedTargets(targets []target) []target {
	var out []target
	for _, t := range targets {
		if notYetSupported[t] {
			out = append(out, t)
		}
	}
	return out
}

// labelList renders targets the way the user sees them named.
func labelList(targets []target) string {
	labels := make([]string, len(targets))
	for i, t := range targets {
		labels[i] = t.label()
	}
	return strings.Join(labels, ", ")
}

// announceAssetRoot tells the user which directory devexp installs from and
// how it was chosen, after any warning about a source checkout that was
// skipped (#134).
func announceAssetRoot(src repo.Source) {
	if src.Warning != "" {
		ui.Warn(src.Warning)
	}
	if src.Embedded {
		ui.Info("Running standalone — using assets bundled in this binary.")
	}
	ui.Info(fmt.Sprintf("Asset root: %s (%s)", src.RepoDir, src.Origin))
	fmt.Println()
}
