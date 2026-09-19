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
	// Execute prints the error itself; without this cobra prints it too, so a
	// failed install said everything twice. uninstallCmd already does this.
	SilenceErrors: true,
	RunE:          runInstall,
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

// notYetSupported lists, per target, the asset kinds its installer does not
// write yet, so a partial install is never reported as a complete one. Kimi
// installs MCP servers (#112) and agents and skills (#113); #114 adds hooks
// and removes the entry, this map and partialTargets with it.
//
// Nothing needs installsNothing any more. It existed because Kimi installed
// MCP servers only, so --agents-only or --skills-only against it wrote
// nothing; both now install what they name, and no combination of flags
// leaves a Kimi run empty-handed. The "nothing was installed" branch went
// with it.
var notYetSupported = map[target][]string{
	targetKimi: {"hooks"},
}

// ── Entry point ───────────────────────────────────────────────────────────────

func runInstall(cmd *cobra.Command, args []string) error {
	// The flags parsed, so anything that fails from here on is a run that went
	// wrong, not a command typed wrong, and the usage block helps nobody. It
	// matters more now that a deliberate outcome — selecting only a target
	// that installs nothing yet — exits non-zero: the notice explaining it
	// would otherwise be three screens above the usage dump. A bad flag still
	// gets usage, because this line has not run yet.
	cmd.SilenceUsage = true

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

	for _, t := range targets {
		install, ok := installers[t]
		if !ok {
			return fmt.Errorf("no installer for target %q", string(t))
		}
		if err := install(opts); err != nil {
			return fmt.Errorf("%s install: %w", string(t), err)
		}
	}

	// The per-target notice scrolls past in a multi-target run, so what a
	// partly-supported target did not install is repeated in the summary.
	said := false
	for _, t := range partialTargets(targets) {
		ui.Warn(fmt.Sprintf("%s: %s are not installed for it yet (#114).", t.label(), joinAnd(notYetSupported[t])))
		said = true
	}
	if said {
		fmt.Println()
	}

	fmt.Printf("\033[0;32m\033[1mAll done.\033[0m\n\n")
	return nil
}

// partialTargets returns the selected targets that still install only part of
// what devexp ships.
func partialTargets(targets []target) []target {
	var out []target
	for _, t := range targets {
		if len(notYetSupported[t]) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// joinAnd renders a list the way a sentence needs it: "agents, skills and
// hooks". The target announcement deliberately uses commas throughout
// (announceTargets), but these are read as prose, not as a set.
func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
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
// skipped.
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
