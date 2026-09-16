package cmd

import (
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

// ── Claude Code ───────────────────────────────────────────────────────────────

func doInstallClaude(opts *installOpts) error {
	p, err := claudeTargetPaths(os.Getenv("HOME"), time.Now())
	if err != nil {
		return err
	}
	agentsTarget := p.agents
	skillsTarget := p.skills
	settingsPath := p.settings
	backupDir := p.backup

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

	manifestPath := p.manifest
	old := loadOldManifest(manifestPath)
	newManifest := &manifest.Manifest{Agents: old.Agents, Skills: old.Skills}

	if !opts.skillsOnly {
		backupExisting(agentsTarget, "*.md", backupDir, opts.dryRun)
		ui.Info("Installing agents...")
		disabled := resolveAgentDisabled(opts.repoDir, opts.selectedAgents, opts.cfg.DisabledAgents)
		installedAgents, err := agents.InstallClaude(
			filepath.Join(opts.repoDir, "agents"),
			agentsTarget,
			opts.cfg.Model,
			disabled,
			opts.dryRun,
		)
		if err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Installed %d agent(s).", len(installedAgents)))
		fmt.Println()

		newManifest.Agents = installedAgents
		removeStale(agentsTarget, old.Agents, installedAgents, staleFile, os.Remove, opts.dryRun)
	}

	if !opts.agentsOnly {
		backupExistingDirs(skillsTarget, backupDir, opts.dryRun)
		ui.Info("Installing skills...")
		installedSkills, err := skills.InstallClaude(
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
		removeStale(skillsTarget, old.Skills, installedSkills, staleDir, os.RemoveAll, opts.dryRun)
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

	if !opts.dryRun {
		if err := manifest.Save(manifestPath, newManifest); err != nil {
			ui.Warn(fmt.Sprintf("save manifest: %v", err))
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
