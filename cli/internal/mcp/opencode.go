package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devexp/internal/ui"
)

type ocEntry struct {
	Type    string            `json:"type"`
	URL     string            `json:"url,omitempty"`
	Command []string          `json:"command,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func InstallOpencode(mcps []MCP, env map[string]string, configPath string, dryRun, reinstall bool) error {
	config := map[string]interface{}{}
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &config) //nolint:errcheck
	}

	mcpMap, ok := config["mcp"].(map[string]interface{})
	if !ok {
		mcpMap = make(map[string]interface{})
		config["mcp"] = mcpMap
	}

	if reinstall {
		for _, m := range mcps {
			if _, exists := mcpMap[m.Name]; exists {
				delete(mcpMap, m.Name)
				ui.Removed(m.Name)
			} else {
				ui.Skipped(m.Name, "not configured")
			}
		}
	}

	changed := false
	for _, m := range mcps {
		// Resolve env vars
		resolved := make(map[string]string)
		for k, v := range m.Env {
			if override, ok := env[k]; ok {
				resolved[k] = override
			} else {
				resolved[k] = v
			}
		}
		resolvedArgs := make([]string, len(m.Args))
		for i, a := range m.Args {
			resolvedArgs[i] = resolveStr(a, env)
		}
		resolvedHeaders := make(map[string]string)
		for k, v := range m.Headers {
			resolvedHeaders[k] = resolveStr(v, env)
		}

		// Check required env
		var missing []string
		for _, key := range m.RequiredEnv {
			if env[key] == "" {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			fmt.Printf("\n  \033[0;31m[REQUIRED]\033[0m %s — missing: %s\n", m.Name, strings.Join(missing, ", "))
			continue
		}

		transport := m.transport()
		var entry ocEntry
		if transport == "http" || transport == "sse" {
			entry = ocEntry{Type: "remote", URL: m.URL}
			if len(resolvedHeaders) > 0 {
				entry.Headers = resolvedHeaders
			}
		} else {
			cmd := append([]string{m.Command}, resolvedArgs...)
			entry = ocEntry{Type: "local", Command: cmd}
			if len(resolved) > 0 {
				entry.Env = resolved
			}
		}

		if dryRun {
			ui.DryRun(fmt.Sprintf("add mcp.%s (%s) to %s", m.Name, transport, configPath))
			continue
		}

		newJSON, _ := json.Marshal(entry)
		if existingRaw, exists := mcpMap[m.Name]; exists {
			existingJSON, _ := json.Marshal(existingRaw)
			if string(existingJSON) == string(newJSON) {
				ui.Skipped(m.Name, "already configured")
				continue
			}
			mcpMap[m.Name] = entry
			ui.Updated(m.Name)
		} else {
			mcpMap[m.Name] = entry
			ui.Added(m.Name)
		}
		changed = true
	}

	if changed && !dryRun {
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return err
		}
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return err
		}
		fmt.Printf("  Saved: %s\n", configPath)
	}

	return nil
}
