// Package config reads the two optional files a devexp checkout may carry:
// devexp.config.json, which narrows what an install does, and mcps/.env,
// which supplies MCP secrets. Both are optional by design — a checkout with
// neither installs everything with no secrets, which is the default devexp
// ships. Neither file is ever written back.
package config

import (
	"encoding/json"

	"github.com/spf13/viper"
)

// Config is a whole devexp.config.json, already flattened out of viper's
// nested keys. Every field is a *narrowing*: an empty Disabled list disables
// nothing, and an empty Model leaves each agent's own frontmatter alone. The
// zero Config is therefore the full install, which is what a caller falls
// back to when the file is missing.
type Config struct {
	Model          string
	DisabledAgents []string
	DisabledSkills []string
	DisabledHooks  []string
	ExtraMCPs      json.RawMessage
}

// Load reads devexp.config.json. It never returns a nil *Config: on a missing
// or unreadable file it returns the zero Config alongside the error, so a
// caller that treats "no config" as "install everything" can warn and carry
// on without a nil check.
//
// An unparseable "mcps" key is dropped rather than raised. The rest of the
// file is still usable, and failing the whole install over an extra MCP block
// would cost more than it protects.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("json")

	if err := v.ReadInConfig(); err != nil {
		return &Config{}, err
	}

	cfg := &Config{
		Model:          v.GetString("model"),
		DisabledAgents: v.GetStringSlice("agents.disabled"),
		DisabledSkills: v.GetStringSlice("skills.disabled"),
		DisabledHooks:  v.GetStringSlice("hooks.disabled"),
	}

	if raw := v.Get("mcps"); raw != nil {
		if data, err := json.Marshal(raw); err == nil {
			cfg.ExtraMCPs = json.RawMessage(data)
		}
	}

	return cfg, nil
}

// IsAgentDisabled reports whether the config names this agent in
// agents.disabled. The match is exact, so a rename silently stops disabling.
func (c *Config) IsAgentDisabled(name string) bool { return sliceContains(c.DisabledAgents, name) }

// IsSkillDisabled is IsAgentDisabled for skills.disabled.
func (c *Config) IsSkillDisabled(name string) bool { return sliceContains(c.DisabledSkills, name) }

// IsHookDisabled is IsAgentDisabled for hooks.disabled.
func (c *Config) IsHookDisabled(name string) bool { return sliceContains(c.DisabledHooks, name) }

func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
