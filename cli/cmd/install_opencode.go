package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"devexp/internal/agents"
	"devexp/internal/hooks"
	"devexp/internal/manifest"
	"devexp/internal/mcp"
	"devexp/internal/skills"
	"devexp/internal/ui"
)

// ── opencode ──────────────────────────────────────────────────────────────────

func doInstallOpencode(opts *installOpts) error {
	p := opencodeTargetPaths(os.Getenv("HOME"))
	agentsTarget := p.agents
	skillsTarget := p.skills
	configPath := p.config

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

	manifestPath := p.manifest
	old := loadOldManifest(manifestPath)
	newManifest := &manifest.Manifest{Agents: old.Agents, Skills: old.Skills, Plugins: old.Plugins}

	if !opts.skillsOnly {
		ui.Info("Installing agents (transformed for opencode)...")
		disabled := resolveAgentDisabled(opts.repoDir, opts.selectedAgents, opts.cfg.DisabledAgents)
		installedAgents, err := agents.InstallOpencode(
			filepath.Join(opts.repoDir, "agents"),
			agentsTarget,
			opts.cfg.Model,
			disabled,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		installedExclusive, err := agents.InstallOpencodeExclusive(
			filepath.Join(opts.repoDir, "agents", "opencode"),
			agentsTarget,
			opts.cfg.Model,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		allInstalledAgents := append(installedAgents, installedExclusive...)
		ui.Success(fmt.Sprintf("Installed %d agent(s).", len(allInstalledAgents)))
		fmt.Println()

		newManifest.Agents = allInstalledAgents
		removeStale(agentsTarget, manifest.Stale(old.Agents, allInstalledAgents), os.Remove, opts.dryRun)
	}

	if !opts.agentsOnly {
		ui.Info("Installing skills (to ~/.config/opencode/commands)...")
		installedSkills, err := skills.InstallOpencode(
			filepath.Join(opts.repoDir, "skills"),
			skillsTarget,
			opts.cfg.DisabledSkills,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Installed %d skill(s).", len(installedSkills)))
		fmt.Println()

		newManifest.Skills = installedSkills
		staleSkills := manifest.Stale(old.Skills, installedSkills)
		staleSkillFiles := make([]string, len(staleSkills))
		for i, s := range staleSkills {
			staleSkillFiles[i] = s + ".md"
		}
		removeStale(skillsTarget, staleSkillFiles, os.Remove, opts.dryRun)
	}

	if !opts.agentsOnly && !opts.skillsOnly {
		registry, err := hooks.LoadRegistry(filepath.Join(opts.repoDir, "hooks", "registry.json"))
		if err != nil {
			ui.Warn(fmt.Sprintf("hooks registry: %v", err))
		} else {
			ui.Info(fmt.Sprintf("Installing hooks (opencode plugin → %s)...", p.plugins))
			disabled := resolveHookDisabled(registry, opts.selectedHooks, opts.cfg.DisabledHooks)
			// InstallOpencode validates before it changes anything and removes
			// the plugin files it no longer installs itself.
			installedPlugins, err := hooks.InstallOpencode(registry, opts.repoDir, p.plugins, disabled, old.Plugins, opts.dryRun)
			if err != nil {
				return err
			}
			newManifest.Plugins = installedPlugins
			// Only once the new plugin is in place: a refused install must not
			// take away a working legacy one.
			if err := hooks.CleanLegacyOpencode(p.plugins, configPath, opts.dryRun); err != nil {
				ui.Warn(fmt.Sprintf("legacy opencode plugin cleanup: %v", err))
			}
			fmt.Println()
		}
	}

	if !opts.dryRun {
		if err := manifest.Save(manifestPath, newManifest); err != nil {
			ui.Warn(fmt.Sprintf("save manifest: %v", err))
		}
	}

	ui.Success("opencode installation complete.")
	fmt.Printf("  Agents : %s\n", agentsTarget)
	fmt.Printf("  Skills : %s\n", skillsTarget)
	if !opts.agentsOnly && !opts.skillsOnly {
		fmt.Printf("  Hooks  : %s\n", p.plugins)
	}
	fmt.Println()
	ui.Info("Restart opencode to activate.")
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
