package mcp

import (
	"encoding/json"
	"os"
)

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

func LoadRegistry(path string) ([]MCP, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mcps []MCP
	return mcps, json.Unmarshal(data, &mcps)
}

func LoadFromRaw(raw json.RawMessage) ([]MCP, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var mcps []MCP
	return mcps, json.Unmarshal(raw, &mcps)
}
