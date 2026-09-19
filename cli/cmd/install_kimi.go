package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"devexp/internal/agents"
	"devexp/internal/hooks"
	"devexp/internal/manifest"
	"devexp/internal/mcp"
	"devexp/internal/skills"
	"devexp/internal/ui"
)

// ── Kimi Code CLI ─────────────────────────────────────────────────────────────
//
// MCP servers (#112), agents and skills (#113) and hooks (#114) all install
// here, so a Kimi run writes everything devexp ships, and #115 takes it all
// out again. What every run says instead is how much less of it Kimi honours
// than Claude Code does (warnKimiFeatureSubset).
//
// The order matches doInstallClaude: MCP servers first, then agents, then
// skills, then hooks. Each step is skipped by the --*-only flags that exclude
// it, and no combination of those flags leaves the run with nothing to do.
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

	// Set once the hooks step has put something there, so the summary names
	// the hooks directory only when it holds devexp's scripts — a run whose
	// every hook is off for Kimi must not point at an empty directory.
	hooksInstalled := false

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
			// pruned, and ./uninstall.sh, which reads that same manifest, has
			// nothing to go on either.
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

	if !opts.agentsOnly && !opts.skillsOnly && !opts.mcpsOnly {
		registry, err := hooks.LoadRegistry(filepath.Join(opts.repoDir, "hooks", "registry.json"))
		if err != nil {
			// As for Claude Code and opencode: a registry that will not load
			// costs the hooks step, not the run.
			ui.Warn(fmt.Sprintf("hooks registry: %v", err))
		} else {
			ui.Info(fmt.Sprintf("Installing hooks (Kimi → %q)...", p.hooks))
			disabled := resolveHookDisabled(registry, opts.selectedHooks, opts.cfg.DisabledHooks)
			// Unlike the other two targets, the guards are copied into the
			// Kimi root rather than run from the checkout, so this step both
			// writes files and edits config.toml. It hands back what it wrote
			// even when it fails, for the same reason the agent step does.
			installedHooks, err := hooks.InstallKimi(registry, opts.repoDir, p.hooks, p.config, disabled, old.Hooks, opts.dryRun)
			newManifest.Hooks = installedHooks
			if err != nil {
				return err
			}
			fmt.Println()
			hooksInstalled = len(installedHooks) > 0
		}
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
	// Only when the step ran and the registry loaded: a hooks line naming a
	// directory nothing was written to would be the one claim this summary
	// must not make.
	if hooksInstalled {
		fmt.Printf("  Hooks  : %q\n", p.hooks)
	}
	fmt.Println()
	warnKimiFeatureSubset(opts)
	ui.Info("Restart Kimi Code CLI to activate.")
	fmt.Println()
	return nil
}

// warnKimiFeatureSubset says what a devexp install is not, on Kimi. Everything
// devexp ships installs there — but four of those things behave differently
// enough that a user who assumes Claude Code's behaviour will be wrong about
// what is protecting them, and the last of those is a security property.
//
// Why here and not only in the docs: each item changes what the user does in
// the very next minute — which name they type to run a skill, whether they can
// trust a per-skill tool restriction they wrote, whether a hook they believe is
// guarding them actually is. A line in install.md is read once, by someone
// choosing a CLI; this is read by everyone who installs. It is also in
// docs/guides/install.md, in full and with the rest of the list, because eight
// items in terminal output is a wall nobody reads — so the terminal gets the
// four that change behaviour and a pointer to the rest.
//
// Only what this run installed: a --mcps-only run has no agents, skills or
// hooks to be wrong about, and warning about them would be noise.
func warnKimiFeatureSubset(opts *installOpts) {
	var items []string
	if !opts.skillsOnly && !opts.mcpsOnly {
		// Kimi has no per-agent system prompt slot: a custom agent body
		// replaces its own entirely, which is why every installed agent ends
		// with ${base_prompt}.
		items = append(items, "an agent's body replaces Kimi's whole system prompt — devexp's agents append ${base_prompt} to put it back")
	}
	if !opts.agentsOnly && !opts.mcpsOnly {
		// The two that silently do less than the source file says.
		items = append(items,
			"a skill's allowed-tools is ignored — Kimi applies no per-skill tool restriction",
			"skills are invoked as /skill:<name>, never as bare /<name>")
	}
	if !opts.agentsOnly && !opts.skillsOnly && !opts.mcpsOnly {
		// The one that is a security property rather than an inconvenience:
		// a hook the user believes is guarding them may not be.
		items = append(items, "hooks fail open — a hook that times out, cannot spawn or exits non-zero/2 is read as an allow, and the hooks Kimi cannot honour (ask-verdict, on-save) are off, with reasons")
	}
	if len(items) == 0 {
		return
	}
	ui.Warn("Kimi Code CLI supports less of what devexp's assets ask for than Claude Code does:")
	for _, item := range items {
		fmt.Printf("  - %s\n", item)
	}
	ui.Info("The full list — including `kimi doctor`'s blind spots and how a project .mcp.json can shadow these MCP entries — is in docs/guides/install.md (\"Kimi Code CLI\").")
	fmt.Println()
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
