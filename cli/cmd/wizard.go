package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"devexp/internal/mcp"
	"devexp/internal/ui"
)

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
