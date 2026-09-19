package mcp

import (
	"reflect"
	"testing"
)

func TestResolveMCP(t *testing.T) {
	env := map[string]string{"DIR": "/opt/tool", "TOKEN": "s3cret", "EMPTY": ""}

	t.Run("every field is expanded, including the url and headers", func(t *testing.T) {
		got := resolveMCP(MCP{
			Name:      "remote",
			Transport: "http",
			URL:       "https://example.com/${DIR}",
			Headers:   map[string]string{"Authorization": "Bearer ${TOKEN}"},
		}, env)
		if got.url != "https://example.com//opt/tool" {
			t.Errorf("url = %q", got.url)
		}
		if got.headers["Authorization"] != "Bearer s3cret" {
			t.Errorf("headers = %v", got.headers)
		}
		if len(got.missing) != 0 {
			t.Errorf("missing = %v, want none", got.missing)
		}
	})

	t.Run("command and args are expanded", func(t *testing.T) {
		got := resolveMCP(MCP{Name: "local", Command: "node", Args: []string{"${DIR}/dist/index.js", "--flag"}}, env)
		if got.command != "node" || !reflect.DeepEqual(got.args, []string{"/opt/tool/dist/index.js", "--flag"}) {
			t.Errorf("= %q %v", got.command, got.args)
		}
		if got.transport != "stdio" || got.scope != "user" {
			t.Errorf("transport/scope = %q/%q, want the defaults", got.transport, got.scope)
		}
	})

	t.Run("an explicit value beats the registry default, and required_env is injected", func(t *testing.T) {
		got := resolveMCP(MCP{
			Name:        "local",
			Command:     "node",
			Env:         map[string]string{"TOKEN": "placeholder", "MODE": "${DIR}"},
			RequiredEnv: []string{"TOKEN", "DIR"},
		}, env)
		want := map[string]string{"TOKEN": "s3cret", "MODE": "/opt/tool", "DIR": "/opt/tool"}
		if !reflect.DeepEqual(got.env, want) {
			t.Errorf("env = %v, want %v", got.env, want)
		}
	})

	t.Run("an unset required_env is missing, in the order the registry declares it", func(t *testing.T) {
		got := resolveMCP(MCP{Name: "x", Command: "node", RequiredEnv: []string{"NOPE", "EMPTY"}}, env)
		if !reflect.DeepEqual(got.missing, []string{"NOPE", "EMPTY"}) {
			t.Errorf("missing = %v", got.missing)
		}
	})

	// Kimi expands nothing, so a placeholder that resolved to nothing would be
	// written as a command that cannot run — and one entry Kimi rejects, or a
	// server that dies on spawn, is not a local problem in a file where one bad
	// entry is enough to lose every server.
	t.Run("a placeholder that resolved to nothing is missing too, and sorted", func(t *testing.T) {
		got := resolveMCP(MCP{Name: "x", Command: "${UNSET_B}", Args: []string{"${UNSET_A}"}}, env)
		if !reflect.DeepEqual(got.missing, []string{"UNSET_A", "UNSET_B"}) {
			t.Errorf("missing = %v, want both placeholders, sorted", got.missing)
		}
	})

	t.Run("a placeholder named in required_env is reported once", func(t *testing.T) {
		got := resolveMCP(MCP{Name: "x", Command: "node", Args: []string{"${NOPE}/x"}, RequiredEnv: []string{"NOPE"}}, env)
		if !reflect.DeepEqual(got.missing, []string{"NOPE"}) {
			t.Errorf("missing = %v, want one entry", got.missing)
		}
	})
}
