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

func AddClaude(m MCP, env map[string]string, dryRun bool) error {
	// Resolve env vars
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

	// Resolve ${VAR} in args
	resolvedArgs := make([]string, len(m.Args))
	for i, a := range m.Args {
		resolvedArgs[i] = resolveStr(a, env)
	}

	// Check required env
	var missing []string
	for _, key := range m.RequiredEnv {
		if env[key] == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		ui.Required(m.Name, missing)
		if m.SetupInstructions != "" {
			fmt.Println()
			for _, line := range strings.Split(m.SetupInstructions, "\n") {
				fmt.Printf("  %s\n", line)
			}
		}
		fmt.Printf("\n  %s will not be available until these are set.\n\n", m.Name)
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
