package mcp

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"devexp/internal/ui"
)

var varRe = regexp.MustCompile(`\$\{(\w+)\}`)

func resolveStr(s string, env map[string]string) string {
	return varRe.ReplaceAllStringFunc(s, func(match string) string {
		key := varRe.FindStringSubmatch(match)[1]
		return env[key]
	})
}

func isInstalledClaude(name string) bool {
	out, err := exec.Command("claude", "mcp", "list").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), name)
}

// RemoveClaude drops one server from Claude Code's own registry by shelling
// out to `claude mcp remove`. It returns nothing on purpose: a server that is
// already absent, or a removal the CLI refuses, must not stop the run that
// called it. Both outcomes are reported as a line and the run continues.
func RemoveClaude(m MCP) {
	if !isInstalledClaude(m.Name) {
		ui.Skipped(m.Name, "not installed")
		return
	}
	cmd := exec.Command("claude", "mcp", "remove", m.Name)
	if out, err := cmd.CombinedOutput(); err != nil {
		ui.Warn(fmt.Sprintf("remove %s: %s", m.Name, strings.TrimSpace(string(out))))
		return
	}
	ui.Removed(m.Name)
}

// AddClaude registers one server through `claude mcp add`, resolving its
// ${VAR} placeholders first. env wins over the registry's own Env values, so
// a key set in mcps/.env overrides the default the registry ships; a
// RequiredEnv key is taken from env only when Env did not already supply it.
//
// A placeholder with no value resolves to the empty string rather than
// failing, which is why a caller checks RequiredEnv and tells the user what
// is missing instead of registering a server that cannot authenticate.
func AddClaude(m MCP, env map[string]string, dryRun bool) error {
	resolved := make(map[string]string)
	for k, v := range m.Env {
		if override, ok := env[k]; ok {
			resolved[k] = override
		} else {
			resolved[k] = v
		}
	}
	for _, key := range m.RequiredEnv {
		if v, ok := env[key]; ok {
			if _, already := resolved[key]; !already {
				resolved[key] = v
			}
		}
	}

	resolvedArgs := make([]string, len(m.Args))
	for i, a := range m.Args {
		resolvedArgs[i] = resolveStr(a, env)
	}

	var missing []string
	for _, key := range m.RequiredEnv {
		if env[key] == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		// The same notice every target prints (resolve.go).
		printRequired(m, missing)
		return nil
	}

	transport := m.transport()
	scope := m.scope()

	if dryRun {
		if transport == "http" || transport == "sse" {
			ui.DryRun(fmt.Sprintf("claude mcp add --scope %s --transport %s %s %s", scope, transport, m.Name, m.URL))
		} else {
			ui.DryRun(fmt.Sprintf("claude mcp add --scope %s %s -- %s %s", scope, m.Name, m.Command, strings.Join(resolvedArgs, " ")))
		}
		return nil
	}

	if isInstalledClaude(m.Name) {
		ui.Skipped(m.Name, "already installed")
		return nil
	}

	args := []string{"mcp", "add", "--scope", scope}

	if transport == "http" || transport == "sse" {
		args = append(args, "--transport", transport)
		for k, v := range m.Headers {
			args = append(args, "-H", fmt.Sprintf("%s: %s", k, resolveStr(v, env)))
		}
		args = append(args, m.Name, m.URL)
	} else {
		for k, v := range resolved {
			args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
		}
		args = append(args, m.Name, "--", m.Command)
		args = append(args, resolvedArgs...)
	}

	cmd := exec.Command("claude", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		ui.Warn(fmt.Sprintf("%s — %s", m.Name, strings.TrimSpace(string(out))))
		return nil
	}
	ui.Added(m.Name)
	return nil
}

// InstallClaude registers every server in order, stopping at the first one
// that fails to add — the servers before it stay registered, because there is
// no transaction across separate `claude mcp` calls to roll back.
//
// With reinstall, each server is removed before being added again, which is
// the only way to change an entry Claude Code already holds: `claude mcp add`
// will not overwrite one in place.
func InstallClaude(mcps []MCP, env map[string]string, dryRun, reinstall bool) error {
	for _, m := range mcps {
		if reinstall {
			RemoveClaude(m)
		}
		if err := AddClaude(m, env, dryRun); err != nil {
			return err
		}
	}
	return nil
}
