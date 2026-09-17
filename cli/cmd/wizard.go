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
	// Shares announceTargets/selectTargets with the flag path, so the rule for
	// which CLIs to install for lives in exactly one place. The trailing blank
	// line stays here: the flag path's caller prints its own.
	hasClaude := commandExists("claude")
	hasOpencode := commandExists("opencode")

	choice, err := announceTargets(hasClaude, hasOpencode)
	if err != nil {
		return nil, err
	}
	installClaude, installOpencode, err := selectTargets(hasClaude, hasOpencode, choice)
	if err != nil {
		return nil, err
	}
	result.installClaude = installClaude
	result.installOpencode = installOpencode
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

// runRemove runs uninstall.sh from repoDir, which must be the asset root
// repo.Resolve chose — DEVEXP_DIR, this dev build's own source checkout, or the
// bundled assets — never a directory found some other way.
func runRemove(repoDir string) error {
	script := filepath.Join(repoDir, "uninstall.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("uninstall.sh not found at %s", script)
	}
	cmd := exec.Command("/bin/bash", script)
	// uninstall.sh delegates plugin removal to this binary, which may be the
	// only devexp binary around: a standalone install has no bin/devexp.
	if exe, err := os.Executable(); err == nil {
		cmd.Env = append(os.Environ(), "DEVEXP_BIN="+exe)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
