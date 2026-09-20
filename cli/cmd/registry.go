package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"devexp/internal/config"
	"devexp/internal/hooks"
	"devexp/internal/mcp"
	"devexp/internal/ui"
)

// loadFullRegistry reads the registry and appends any extra MCPs from config.
func loadFullRegistry(repoDir string, cfg *config.Config) ([]mcp.MCP, error) {
	registry, err := mcp.LoadRegistry(filepath.Join(repoDir, "mcps", "registry.json"))
	if err != nil {
		return nil, fmt.Errorf("load MCP registry: %w", err)
	}
	if len(cfg.ExtraMCPs) > 0 {
		extra, err := mcp.LoadFromRaw(cfg.ExtraMCPs)
		if err != nil {
			ui.Warn(fmt.Sprintf("extra MCPs from config: %v", err))
		} else {
			registry = append(registry, extra...)
		}
	}
	return registry, nil
}

// filterMCPs returns only MCPs whose names are in selected. nil is "the wizard
// did not ask", which is every MCP; a non-nil empty slice is "it asked and the
// answer was none", which is none of them.
func filterMCPs(registry []mcp.MCP, selected []string) []mcp.MCP {
	if selected == nil {
		return registry
	}
	sel := make(map[string]bool, len(selected))
	for _, name := range selected {
		sel[name] = true
	}
	var out []mcp.MCP
	for _, m := range registry {
		if sel[m.Name] {
			out = append(out, m)
		}
	}
	return out
}

func buildEnv(dotenv map[string]string, repoDir string) map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		k, v, _ := strings.Cut(e, "=")
		env[k] = v
	}
	env["DEVEXP_DIR"] = repoDir
	for k, v := range dotenv {
		env[k] = v
	}
	return env
}

// listAgentNames reads the agents directory and returns sorted agent names.
func listAgentNames(repoDir string) []string {
	entries, err := os.ReadDir(filepath.Join(repoDir, "agents"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		if name == "README" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// listHookNames reads the hooks registry and returns the names of enabled hooks.
func listHookNames(repoDir string) []string {
	registry, err := hooks.LoadRegistry(filepath.Join(repoDir, "hooks", "registry.json"))
	if err != nil {
		return nil
	}
	var names []string
	for _, h := range registry {
		if h.Enabled {
			names = append(names, h.Name)
		}
	}
	return names
}

// resolveHookDisabled builds the disabled list for the hooks installer.
// When selected is non-nil (wizard path), enabled hooks not in selected are
// disabled — including when it is empty, which disables all of them.
// When selected is nil (flag path), falls back to cfgDisabled from config.
func resolveHookDisabled(registry hooks.Registry, selected, cfgDisabled []string) []string {
	if selected == nil {
		return cfgDisabled
	}
	sel := make(map[string]bool, len(selected))
	for _, name := range selected {
		sel[name] = true
	}
	var disabled []string
	for _, h := range registry {
		if h.Enabled && !sel[h.Name] {
			disabled = append(disabled, h.Name)
		}
	}
	return disabled
}

// resolveAgentDisabled builds the disabled list for the agent installer.
// When selected is non-nil (wizard path), agents not in selected are disabled —
// including when it is empty, which disables all of them.
// When selected is nil (flag path), falls back to cfgDisabled from config.
func resolveAgentDisabled(repoDir string, selected, cfgDisabled []string) []string {
	if selected == nil {
		return cfgDisabled
	}
	sel := make(map[string]bool, len(selected))
	for _, name := range selected {
		sel[name] = true
	}
	all := listAgentNames(repoDir)
	var disabled []string
	for _, name := range all {
		if !sel[name] {
			disabled = append(disabled, name)
		}
	}
	return disabled
}
