// Package mcp installs the MCP servers in mcps/registry.json into each CLI
// that takes them, and each one takes them differently: Claude Code through
// its own `claude mcp` command, opencode and Kimi Code CLI by merging into a
// JSON config devexp does not own. The merging paths matter most — they must
// give back a file that is byte-identical outside the keys devexp wrote.
package mcp

import (
	"encoding/json"
	"os"
)

// MCP is one entry of mcps/registry.json. Two zero values carry meaning
// rather than being unset: an empty Transport is "stdio" and an empty Scope
// is "user".
//
// Env holds ${VAR} placeholders, never secrets — the values come from
// mcps/.env at install time. RequiredEnv names the keys that must resolve for
// the server to work, and is what lets a run tell the user which are missing
// instead of installing something that cannot start.
type MCP struct {
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Transport         string            `json:"transport"`
	URL               string            `json:"url"`
	Command           string            `json:"command"`
	Args              []string          `json:"args"`
	Scope             string            `json:"scope"`
	Env               map[string]string `json:"env"`
	RequiredEnv       []string          `json:"required_env"`
	Headers           map[string]string `json:"headers"`
	SetupInstructions string            `json:"setup_instructions"`
}

func (m *MCP) transport() string {
	if m.Transport == "" {
		return "stdio"
	}
	return m.Transport
}

func (m *MCP) scope() string {
	if m.Scope == "" {
		return "user"
	}
	return m.Scope
}

// LoadRegistry reads mcps/registry.json from disk. On a decode error the
// returned slice is whatever json got through before failing, so callers must
// check the error rather than the length — a partial registry would install a
// truncated set and report success.
func LoadRegistry(path string) ([]MCP, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mcps []MCP
	return mcps, json.Unmarshal(data, &mcps)
}

// LoadFromRaw decodes the extra MCPs a devexp.config.json may add, which
// arrive already embedded in another document. Empty input is (nil, nil):
// configuring no extra servers is the normal case, not a failure.
func LoadFromRaw(raw json.RawMessage) ([]MCP, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var mcps []MCP
	return mcps, json.Unmarshal(raw, &mcps)
}
