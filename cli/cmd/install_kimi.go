package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"devexp/internal/agents"
	"devexp/internal/manifest"
	"devexp/internal/mcp"
	"devexp/internal/skills"
	"devexp/internal/ui"
)

// ── Kimi Code CLI ─────────────────────────────────────────────────────────────
//
// MCP servers (#112), agents and skills (#113) install here; hooks (#114) do
// not yet, and ./uninstall.sh does not remove a Kimi install yet (#115). So
// every run says what it did not install as well as what it did
// (notYetSupported in install.go).
//
// The order matches doInstallClaude: MCP servers first, then agents, then
// skills. Each step is skipped by the --*-only flags that exclude it, and
// since #113 no combination of those flags leaves the run with nothing to do.
//
// Resolving the paths first is also where a $KIMI_CODE_HOME devexp must not
// write to is caught, before anything is read or written.

func doInstallKimi(opts *installOpts) error {
	p, err := kimiTargetPaths(os.Getenv("KIMI_CODE_HOME"), os.Getenv("HOME"), time.Now())
	if err != nil {
		return err
	}
	ui.Info("Installing for Kimi Code CLI...")
	fmt.Println()

	// Kimi has no per-agent model: it comes from config.toml's
	// [secondary_model] or from the Agent tool's own argument. A model
	// override cannot be honoured here, and must not be silently ignored
	// either — but a run that installs no agents has nothing to say it about.
	if opts.cfg.Model != "" && !opts.mcpsOnly {
		ui.Warn(fmt.Sprintf("--model %q is ignored for Kimi Code CLI — it has no per-agent model; set the model in Kimi's own config instead.", opts.cfg.Model))
		fmt.Println()
	}

	old := loadOldManifest(p.manifest)
	// The whole struct, so a field this target does not write — and a field a
	// later version adds — is carried forward rather than dropped (#108).
	newManifest := *old

	// Deferred, so the record survives a step that fails after an earlier one
	// wrote. Before #113 there was nothing after the MCP step to fail; now a
	// failing agent or skill install would return with mcp.json already
	// merged and its ownership unrecorded, and the next run would read
	// devexp's own entries as the user's and never touch them again. The
	// manifest is the only ownership record Kimi has.
	if !opts.dryRun {
		defer func() {
			if err := manifest.Save(p.manifest, &newManifest); err != nil {
				ui.Warn(fmt.Sprintf("save manifest: %v", err))
			}
		}()
	}

	// How the installed agents are named in the bodies that reference them,
	// which is not the same as where they are written: the tilde form with the
	// default root, an absolute path with a custom one (kimiAgentsRef).
	agentsDir := p.agentsRef

	if !opts.agentsOnly && !opts.skillsOnly {
		owned, err := installMCPsKimi(opts, p, old.MCPs)
		if err != nil {
			// An mcp.json devexp cannot merge into without losing what is
			// there costs only the MCP step; it is not a reason to fail the
			// run, and since #113 there is always more to do afterwards.
			var refused *mcp.ConfigRefusedError
			if !errors.As(err, &refused) {
				return err
			}
			warnMCPsSkipped(refused, "mcpServers")
			fmt.Println()
		}
		newManifest.MCPs = owned
	}

	if !opts.skillsOnly && !opts.mcpsOnly {
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
			// A step that fails part-way has still written files, and it hands
			// them back with the error. Record them — merged with what was
			// already recorded — or the next run compares against a list that
			// never learned about them: a since-deselected agent is never
			// pruned, and until #115 ./uninstall.sh cannot remove it either.
			//
			// Stale removal is skipped on this path on purpose. The install
			// set is half-finished, so everything the step never reached would
			// look stale, and pruning against it would delete agents this run
			// simply did not get to.
			newManifest.Agents = mergeInstalled(old.Agents, installedAgents)
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

	if !opts.agentsOnly && !opts.mcpsOnly {
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
			// As for agents above: keep what was written, prune nothing.
			newManifest.Skills = mergeInstalled(old.Skills, installedSkills)
			return err
		}
		ui.Success(fmt.Sprintf("Installed %d skill(s).", len(installedSkills)))
		fmt.Println()

		kept := removeStale(p.home, p.skills, old.Skills, installedSkills, staleDir, (*os.Root).RemoveAll, opts.dryRun)
		newManifest.Skills = append(installedSkills, kept...)
	}

	ui.Success("Kimi Code CLI installation complete.")
	// Only what this run actually did. With three kinds of asset and three
	// --*-only flags, listing all three every time would claim work that did
	// not happen. Quoted, because the paths come from $KIMI_CODE_HOME (#111).
	if !opts.agentsOnly && !opts.skillsOnly {
		fmt.Printf("  MCPs   : %q\n", p.mcp)
	}
	if !opts.skillsOnly && !opts.mcpsOnly {
		fmt.Printf("  Agents : %q\n", p.agents)
	}
	if !opts.agentsOnly && !opts.mcpsOnly {
		fmt.Printf("  Skills : %q\n", p.skills)
	}
	fmt.Println()
	// Said here rather than in install.go's per-target notice, which lists the
	// asset kinds still to come: this is not a missing kind but a missing way
	// out. Telling someone what was written without telling them it cannot be
	// removed cleanly is half the story, and #115 is what closes it.
	ui.Warn("./uninstall.sh cannot remove a Kimi Code CLI install yet (#115) — until it can, what is listed above has to be removed by hand.")
	fmt.Println()
	ui.Info("Restart Kimi Code CLI to activate.")
	fmt.Println()
	return nil
}

// installMCPsKimi merges the selected registry MCPs into Kimi's mcp.json and
// returns what devexp now owns there, for the manifest. On a refusal it
// returns what was owned before, so a file devexp could not read never loses
// devexp its record of what it put there.
func installMCPsKimi(opts *installOpts, p kimiPaths, owned map[string]string) (map[string]string, error) {
	registry, err := loadFullRegistry(opts.repoDir, opts.cfg)
	if err != nil {
		return owned, err
	}
	registry = filterMCPs(registry, opts.selectedMCPs)

	ui.Info(fmt.Sprintf("Installing MCP servers (Kimi → %q)...", p.mcp))
	newOwned, err := mcp.InstallKimi(registry, opts.env, p.mcp, owned, opts.dryRun, opts.reinstallMCPs)
	fmt.Println()
	return newOwned, err
}

// mergeInstalled is what the manifest records when a step failed part-way: the
// names it did write, plus everything the previous run recorded. Order is
// what was installed first, then what is only in the old record, so a reader
// sees this run's set before the leftovers; duplicates are dropped.
//
// The union rather than either side alone: the new names are files that exist
// and must be tracked, and the old ones may still be on disk untouched, since
// a failed run removed nothing.
func mergeInstalled(old, installed []string) []string {
	seen := make(map[string]bool, len(installed)+len(old))
	out := make([]string, 0, len(installed)+len(old))
	for _, group := range [][]string{installed, old} {
		for _, name := range group {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}
