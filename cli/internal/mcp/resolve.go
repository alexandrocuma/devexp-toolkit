package mcp

import (
	"fmt"
	"sort"
	"strings"

	"devexp/internal/ui"
)

// ── Registry resolution ───────────────────────────────────────────────────────
//
// The registry never holds a secret: it holds ${VAR} placeholders plus
// required_env, and the values come from the environment and mcps/.env
// (docs/development/mcp-guide.md). Whoever writes a config file has to turn
// those into real values first, and for Kimi that is not optional — Kimi
// expands nothing at all when it reads mcp.json, so a ${VAR} left in the file
// is spawned literally ("spawn ${SHELL} ENOENT").
//
// AddClaude hands the same job to the `claude` CLI one field at a time; this
// is the same two rules in one place, for a target that needs the finished
// values: an explicit value from the environment beats the registry's default,
// and a required_env value with no default is added.

// resolved is one registry MCP with every ${VAR} expanded, ready to be written
// into a config file that does no expansion of its own.
type resolved struct {
	name      string
	transport string
	scope     string
	command   string
	args      []string
	env       map[string]string
	url       string
	headers   map[string]string
	// missing names what has to be set before this MCP can be configured at
	// all: a required_env key that is unset or empty, and any ${VAR} that
	// resolved to nothing. The second kind matters more for Kimi than for
	// Claude Code, where resolveStr's empty string merely produces a broken
	// server: in mcp.json a single unusable entry is not what it costs, the
	// whole file is, and with it every other MCP server the user has.
	missing []string
}

func resolveMCP(m MCP, env map[string]string) resolved {
	r := resolved{
		name:      m.Name,
		transport: m.transport(),
		scope:     m.scope(),
	}
	unset := map[string]bool{}
	resolve := func(s string) string {
		for _, name := range varRe.FindAllStringSubmatch(s, -1) {
			if env[name[1]] == "" {
				unset[name[1]] = true
			}
		}
		return resolveStr(s, env)
	}

	r.command = resolve(m.Command)
	r.url = resolve(m.URL)
	if len(m.Args) > 0 {
		r.args = make([]string, len(m.Args))
		for i, a := range m.Args {
			r.args[i] = resolve(a)
		}
	}
	if len(m.Headers) > 0 {
		r.headers = make(map[string]string, len(m.Headers))
		for k, v := range m.Headers {
			r.headers[k] = resolve(v)
		}
	}
	if len(m.Env) > 0 || len(m.RequiredEnv) > 0 {
		r.env = make(map[string]string, len(m.Env)+len(m.RequiredEnv))
	}
	for k, v := range m.Env {
		if override, ok := env[k]; ok {
			r.env[k] = override
			continue
		}
		r.env[k] = resolve(v)
	}
	for _, key := range m.RequiredEnv {
		if v, ok := env[key]; ok {
			if _, already := r.env[key]; !already {
				r.env[key] = v
			}
		}
	}

	// required_env first, in the order the registry declares it, then the
	// placeholders that resolved to nothing, sorted — the values come out of a
	// map, and a message that reorders itself between runs is a message nobody
	// trusts.
	for _, key := range m.RequiredEnv {
		if env[key] == "" {
			r.missing = append(r.missing, key)
			delete(unset, key)
		}
	}
	rest := make([]string, 0, len(unset))
	for key := range unset {
		rest = append(rest, key)
	}
	sort.Strings(rest)
	r.missing = append(r.missing, rest...)
	return r
}

// printRequired is the notice for an MCP that cannot be configured because the
// values it needs are not set. It is the one every target prints, so it lives
// here rather than in the target that happens to have had it first.
func printRequired(m MCP, missing []string) {
	ui.Required(m.Name, missing)
	if m.SetupInstructions != "" {
		fmt.Println()
		for _, line := range strings.Split(m.SetupInstructions, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}
	fmt.Printf("\n  %s will not be available until these are set.\n\n", m.Name)
}
