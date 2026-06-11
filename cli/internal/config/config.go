package config

import (
	"encoding/json"

	"github.com/spf13/viper"
)

type Config struct {
	Model          string
	DisabledAgents []string
	DisabledSkills []string
	DisabledHooks  []string
	ExtraMCPs      json.RawMessage
}

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

func (c *Config) IsAgentDisabled(name string) bool { return sliceContains(c.DisabledAgents, name) }
func (c *Config) IsSkillDisabled(name string) bool { return sliceContains(c.DisabledSkills, name) }
func (c *Config) IsHookDisabled(name string) bool  { return sliceContains(c.DisabledHooks, name) }

func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
