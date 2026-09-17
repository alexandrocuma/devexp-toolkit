package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"devexp/internal/fsutil"
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
	config, err := loadOpencodeConfig(configPath)
	if err != nil {
		names := make([]string, 0, len(mcps))
		for _, m := range mcps {
			names = append(names, m.Name)
		}
		return &ConfigRefusedError{Path: configPath, Reason: err, Servers: names}
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
		// Atomic, and through a symlinked config.json to the file it points
		// at, keeping the link (#124).
		if err := fsutil.WriteFileAtomic(configPath, data, 0644); err != nil {
			return fmt.Errorf("mcp: %w", err)
		}
		fmt.Printf("  Saved: %s\n", configPath)
	}

	return nil
}

// ConfigRefusedError is InstallOpencode's error for a config.json it can't
// merge into without losing what is there (loadOpencodeConfig). The file was
// left untouched and no MCP server was added; Servers are the ones the install
// would have added, for the user to add by hand.
type ConfigRefusedError struct {
	Path    string
	Reason  error
	Servers []string
}

func (e *ConfigRefusedError) Error() string {
	return fmt.Sprintf("mcp: %v — it was left untouched and no MCP servers were added; fix it and re-run", e.Reason)
}

func (e *ConfigRefusedError) Unwrap() error { return e.Reason }

// loadOpencodeConfig reads config.json for merging MCP servers into it. A
// missing or blank file is an empty config. Anything devexp can't merge into
// without losing what is there is an error, never an empty config that would
// then be written over the file (#124): unreadable, not strict JSON (opencode
// also accepts comments, which encoding/json can't keep), a top level that
// isn't an object, or an "mcp" that isn't an object. Numbers are kept as
// written, not rounded through float64.
func loadOpencodeConfig(configPath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]interface{}{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON (%v)", configPath, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%s is not valid JSON (trailing data after the top-level value)", configPath)
	}
	config, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s is not a JSON object", configPath)
	}
	if m, present := config["mcp"]; present && m != nil {
		if _, ok := m.(map[string]interface{}); !ok {
			return nil, errors.New(configPath + `: "mcp" is not a JSON object`)
		}
	}
	return config, nil
}
