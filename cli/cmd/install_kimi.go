package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"devexp/internal/agents"
	"devexp/internal/manifest"
	"devexp/internal/skills"
	"devexp/internal/ui"
)

// ── Kimi Code CLI ─────────────────────────────────────────────────────────────
//
// Agents and skills install here. MCPs (#112) and hooks (#114) do not yet, and
// ./uninstall.sh does not remove a Kimi install yet (#115). So this is a real
// install that is not a complete one, and it says exactly that: runInstall
// still refuses "All done." for a run whose only target is Kimi (install.go,
// notYetSupported), and the notice at the end names what is missing rather
// than claiming nothing was written.
//
// Resolving the paths first is also where a broken $KIMI_CODE_HOME is caught,
// before anything is read or written.

func doInstallKimi(opts *installOpts) error {
	p, err := kimiTargetPaths(os.Getenv("KIMI_CODE_HOME"), os.Getenv("HOME"), time.Now())
	if err != nil {
		return err
	}
	ui.Info("Installing for Kimi Code CLI...")
	fmt.Println()

	if opts.mcpsOnly {
		// #112 replaces this with the MCP step.
		ui.Warn("MCP servers are not installed for Kimi Code CLI yet (#112), and --mcps-only skips everything else, so nothing was installed.")
		fmt.Println()
		return nil
	}

	// Kimi has no per-agent model: it comes from config.toml's
	// [secondary_model] or from the Agent tool's own argument. A model override
	// cannot be honoured here, and must not be silently ignored either.
	if opts.cfg.Model != "" {
		ui.Warn(fmt.Sprintf("--model %q is ignored for Kimi Code CLI — it has no per-agent model; set the model in Kimi's own config instead.", opts.cfg.Model))
		fmt.Println()
	}

	old := loadOldManifest(p.manifest)
	newManifest := &manifest.Manifest{Agents: old.Agents, Skills: old.Skills}

	// How the installed agents are *named* in the bodies that reference them,
	// which is not the same as where they are written: the tilde form with the
	// default root, an absolute path with a custom one (kimiAgentsRef).
	agentsDir := p.agentsRef

	if !opts.skillsOnly {
		backupExisting(p.agents, "*.md", p.backup, opts.dryRun)
		ui.Info("Installing agents (transformed for Kimi Code CLI)...")
		disabled := resolveAgentDisabled(opts.repoDir, opts.selectedAgents, opts.cfg.DisabledAgents)
		installedAgents, err := agents.InstallKimi(
			filepath.Join(opts.repoDir, "agents"),
			p.agents,
			agentsDir,
			disabled,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Installed %d agent(s).", len(installedAgents)))
		fmt.Println()

		// p.home, not $HOME: kimiPaths.home is the Kimi root's parent, so the
		// removal guard always has a directory to resolve symlinks from — even
		// when $KIMI_CODE_HOME points outside $HOME, which is the whole reason
		// that variable exists. Passing $HOME would make the guard report "not
		// under home" for such a root, and every removal would quietly do
		// nothing behind a warning.
		//
		// What couldn't be removed stays recorded, so a later run can finish.
		kept := removeStale(p.home, p.agents, old.Agents, installedAgents, staleFile, (*os.Root).Remove, opts.dryRun)
		newManifest.Agents = append(installedAgents, kept...)
	}

	if !opts.agentsOnly {
		backupExistingDirs(p.skills, p.backup, opts.dryRun)
		ui.Info("Installing skills...")
		installedSkills, err := skills.InstallKimi(
			filepath.Join(opts.repoDir, "skills"),
			p.skills,
			agentsDir,
			opts.cfg.DisabledSkills,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Installed %d skill(s).", len(installedSkills)))
		fmt.Println()

		kept := removeStale(p.home, p.skills, old.Skills, installedSkills, staleDir, (*os.Root).RemoveAll, opts.dryRun)
		newManifest.Skills = append(installedSkills, kept...)
	}

	if !opts.dryRun {
		if err := manifest.Save(p.manifest, newManifest); err != nil {
			ui.Warn(fmt.Sprintf("save manifest: %v", err))
		}
	}

	ui.Success("Kimi Code CLI: agents and skills installed.")
	// Quoted: the destinations come from $KIMI_CODE_HOME, and nothing from
	// outside devexp reaches the terminal raw (backup.go).
	fmt.Printf("  Agents : %q\n", p.agents)
	fmt.Printf("  Skills : %q\n", p.skills)
	fmt.Println()
	ui.Warn("Kimi Code CLI is not a complete install target yet (#110): MCP servers (#112) and hooks (#114) were not installed, and ./uninstall.sh does not remove a Kimi install yet (#115).")
	fmt.Println()
	ui.Info("Restart Kimi Code CLI to activate.")
	fmt.Println()
	return nil
}
