package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"devexp/internal/ui"
)

type Registry []Hook

type Hook struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ClaudeCode  HookCC `json:"claude_code"`
	Enabled     bool   `json:"enabled"`
}

type HookCC struct {
	Event   string `json:"event"`
	Matcher string `json:"matcher"`
	Script  string `json:"script"`
}

type hookEntry struct {
	Matcher string    `json:"matcher"`
	Hooks   []hookCmd `json:"hooks"`
}

type hookCmd struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func LoadRegistry(path string) (Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Registry
	return r, json.Unmarshal(data, &r)
}

func InstallClaude(registry Registry, repoDir, settingsPath string, disabled []string, dryRun bool) error {
	// Load existing settings as a raw map to preserve unknown fields
	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &raw) //nolint:errcheck
	}

	hooksMap := map[string][]hookEntry{}
	if hooksRaw, ok := raw["hooks"]; ok {
		json.Unmarshal(hooksRaw, &hooksMap) //nolint:errcheck
	}

	isDisabled := func(name string) bool {
		for _, d := range disabled {
			if d == name {
				return true
			}
		}
		return false
	}

	changed := false
	for _, hook := range registry {
		if !hook.Enabled {
			continue
		}
		if isDisabled(hook.Name) {
			ui.Skipped(hook.Name, "disabled in devexp.config.json")
			continue
		}
		cc := hook.ClaudeCode
		if cc.Event == "" || cc.Script == "" {
			continue
		}
		scriptAbs := filepath.Join(repoDir, cc.Script)

		if dryRun {
			ui.DryRun(fmt.Sprintf("add %s hook: %s", cc.Event, filepath.Base(cc.Script)))
			continue
		}

		// Skip if already registered
		alreadyIn := false
		for _, e := range hooksMap[cc.Event] {
			for _, h := range e.Hooks {
				if h.Command == scriptAbs {
					alreadyIn = true
					break
				}
			}
		}
		if alreadyIn {
			ui.Skipped(fmt.Sprintf("%s: %s", cc.Event, filepath.Base(cc.Script)), "already registered")
			continue
		}

		hooksMap[cc.Event] = append(hooksMap[cc.Event], hookEntry{
			Matcher: cc.Matcher,
			Hooks:   []hookCmd{{Type: "command", Command: scriptAbs}},
		})
		changed = true
		fmt.Printf("  \033[0;32m+\033[0m %s: %s\n", cc.Event, filepath.Base(cc.Script))
	}

	if !changed || dryRun {
		return nil
	}

	hooksBytes, err := json.Marshal(hooksMap)
	if err != nil {
		return err
	}
	raw["hooks"] = json.RawMessage(hooksBytes)

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, out, 0644); err != nil {
		return err
	}
	fmt.Printf("  Saved: %s\n", settingsPath)
	return nil
}
