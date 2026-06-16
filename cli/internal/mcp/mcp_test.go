package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStr(t *testing.T) {
	tests := map[string]struct {
		in   string
		env  map[string]string
		want string
	}{
		"single var substituted":      {in: "${TOKEN}", env: map[string]string{"TOKEN": "abc"}, want: "abc"},
		"unknown var becomes empty":   {in: "${MISSING}", env: map[string]string{}, want: ""},
		"literal only unchanged":      {in: "no-vars-here", env: map[string]string{"X": "y"}, want: "no-vars-here"},
		"multiple vars in one string": {in: "${A}-${B}", env: map[string]string{"A": "1", "B": "2"}, want: "1-2"},
		"var embedded in literal":     {in: "pre-${V}-post", env: map[string]string{"V": "mid"}, want: "pre-mid-post"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := resolveStr(tt.in, tt.env); got != tt.want {
				t.Errorf("resolveStr(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTransport(t *testing.T) {
	tests := map[string]struct {
		transport string
		want      string
	}{
		"empty defaults to stdio": {transport: "", want: "stdio"},
		"http honored":            {transport: "http", want: "http"},
		"sse honored":             {transport: "sse", want: "sse"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := MCP{Transport: tt.transport}
			if got := m.transport(); got != tt.want {
				t.Errorf("transport() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScope(t *testing.T) {
	tests := map[string]struct {
		scope string
		want  string
	}{
		"empty defaults to user": {scope: "", want: "user"},
		"local honored":          {scope: "local", want: "local"},
		"project honored":        {scope: "project", want: "project"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := MCP{Scope: tt.scope}
			if got := m.scope(); got != tt.want {
				t.Errorf("scope() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadRegistry(t *testing.T) {
	tests := map[string]struct {
		content   string
		writeFile bool
		wantErr   bool
		wantLen   int
	}{
		"valid JSON array parsed": {
			content:   `[{"name":"a","command":"cmd-a"},{"name":"b","command":"cmd-b"}]`,
			writeFile: true,
			wantErr:   false,
			wantLen:   2,
		},
		"malformed JSON errors": {
			content:   `[{"name":`,
			writeFile: true,
			wantErr:   true,
		},
		"missing file errors": {
			writeFile: false,
			wantErr:   true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "registry.json")
			if tt.writeFile {
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("writing fixture: %v", err)
				}
			}

			mcps, err := LoadRegistry(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(mcps) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(mcps), tt.wantLen)
			}
		})
	}
}

func TestLoadFromRaw(t *testing.T) {
	tests := map[string]struct {
		raw     json.RawMessage
		wantErr bool
		wantNil bool
		wantLen int
	}{
		"empty raw returns nil,nil": {
			raw:     json.RawMessage{},
			wantNil: true,
		},
		"valid raw parsed": {
			raw:     json.RawMessage(`[{"name":"a"}]`),
			wantLen: 1,
		},
		"malformed raw errors": {
			raw:     json.RawMessage(`[{`),
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mcps, err := LoadFromRaw(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if mcps != nil {
					t.Errorf("expected nil slice, got %v", mcps)
				}
				return
			}
			if len(mcps) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(mcps), tt.wantLen)
			}
		})
	}
}

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns whatever was printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

func TestAddClaude_NonExec(t *testing.T) {
	tests := map[string]struct {
		mcp      MCP
		env      map[string]string
		dryRun   bool
		contains []string
	}{
		"missing required env returns nil, prints REQUIRED, no exec": {
			mcp:      MCP{Name: "needs-token", RequiredEnv: []string{"API_TOKEN"}},
			env:      map[string]string{},
			dryRun:   false,
			contains: []string{"[REQUIRED]", "API_TOKEN"},
		},
		"dry-run stdio prints claude mcp add -- command": {
			mcp:      MCP{Name: "local-server", Command: "node", Args: []string{"server.js"}},
			env:      map[string]string{},
			dryRun:   true,
			contains: []string{"[dry-run]", "claude mcp add --scope user", "node", "server.js"},
		},
		"dry-run http prints --transport": {
			mcp:      MCP{Name: "remote-server", Transport: "http", URL: "https://example.com/mcp"},
			env:      map[string]string{},
			dryRun:   true,
			contains: []string{"[dry-run]", "--transport http", "https://example.com/mcp"},
		},
		"missing env with setup instructions prints guidance": {
			mcp: MCP{
				Name:              "gated",
				Env:               map[string]string{"BASE": "default"},
				RequiredEnv:       []string{"SECRET"},
				SetupInstructions: "export SECRET=...\nget it from the dashboard",
			},
			env:      map[string]string{"BASE": "override"},
			dryRun:   false,
			contains: []string{"[REQUIRED]", "SECRET", "get it from the dashboard", "gated will not be available"},
		},
		"dry-run stdio resolves env-var args": {
			mcp: MCP{
				Name:    "templated",
				Command: "run",
				Args:    []string{"--key", "${API_KEY}"},
			},
			env:      map[string]string{"API_KEY": "xyz"},
			dryRun:   true,
			contains: []string{"[dry-run]", "--key xyz"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var err error
			out := captureStdout(t, func() {
				err = AddClaude(tt.mcp, tt.env, tt.dryRun)
			})
			if err != nil {
				t.Fatalf("AddClaude returned error: %v", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(out, want) {
					t.Errorf("stdout missing %q\ngot: %s", want, out)
				}
			}
		})
	}
}

func TestInstallOpencode(t *testing.T) {
	const mcpName = "my-server"

	// The entry InstallOpencode writes for a stdio MCP named my-server / cmd node.
	// Built via ocEntry so the marshaled key order matches what InstallOpencode
	// produces — that byte-for-byte match is what triggers the "skip" branch.
	wantEntry := func() ocEntry {
		return ocEntry{Type: "local", Command: []string{"node", "server.js"}}
	}

	tests := map[string]struct {
		seed          func(path string) // optional pre-existing config
		mcps          []MCP             // override; nil uses the default stdio my-server
		env           map[string]string // override; nil uses empty env
		dryRun        bool
		reinstall     bool
		wantNoFile    bool
		wantUnchanged bool // file must be byte-for-byte identical to the seed
		assert        func(t *testing.T, config map[string]interface{})
	}{
		"fresh write creates config with mcp entry": {
			assert: func(t *testing.T, config map[string]interface{}) {
				mcpMap, _ := config["mcp"].(map[string]interface{})
				entry, ok := mcpMap[mcpName].(map[string]interface{})
				if !ok {
					t.Fatalf("mcp.%s missing: %v", mcpName, config)
				}
				if entry["type"] != "local" {
					t.Errorf("type = %v, want local", entry["type"])
				}
				cmd, _ := entry["command"].([]interface{})
				if len(cmd) != 2 || cmd[0] != "node" || cmd[1] != "server.js" {
					t.Errorf("command = %v, want [node server.js]", cmd)
				}
			},
		},
		"merge preserves existing keys, adds new": {
			seed: func(path string) {
				seed := map[string]interface{}{
					"topLevel": "keep-me",
					"mcp": map[string]interface{}{
						"other": map[string]interface{}{"type": "local", "command": []interface{}{"x"}},
					},
				}
				data, _ := json.MarshalIndent(seed, "", "  ")
				_ = os.WriteFile(path, data, 0o644)
			},
			assert: func(t *testing.T, config map[string]interface{}) {
				if config["topLevel"] != "keep-me" {
					t.Errorf("top-level key lost: %v", config["topLevel"])
				}
				mcpMap, _ := config["mcp"].(map[string]interface{})
				if _, ok := mcpMap["other"]; !ok {
					t.Errorf("existing mcp.other lost")
				}
				if _, ok := mcpMap[mcpName]; !ok {
					t.Errorf("new mcp.%s not added", mcpName)
				}
			},
		},
		"merge identical entry skips, no rewrite": {
			wantUnchanged: true,
			seed: func(path string) {
				seed := map[string]interface{}{
					"mcp": map[string]interface{}{
						mcpName: wantEntry(),
					},
				}
				data, _ := json.MarshalIndent(seed, "", "  ")
				_ = os.WriteFile(path, data, 0o644)
			},
			assert: func(t *testing.T, config map[string]interface{}) {
				mcpMap, _ := config["mcp"].(map[string]interface{})
				if len(mcpMap) != 1 {
					t.Errorf("mcp map size = %d, want 1", len(mcpMap))
				}
			},
		},
		"dry-run does not write": {
			dryRun:     true,
			wantNoFile: true,
		},
		"http transport writes remote entry with resolved headers": {
			mcps: []MCP{{
				Name:      "remote-server",
				Transport: "http",
				URL:       "https://example.com/mcp",
				Headers:   map[string]string{"Authorization": "Bearer ${TOKEN}"},
			}},
			env: map[string]string{"TOKEN": "secret123"},
			assert: func(t *testing.T, config map[string]interface{}) {
				mcpMap, _ := config["mcp"].(map[string]interface{})
				entry, ok := mcpMap["remote-server"].(map[string]interface{})
				if !ok {
					t.Fatalf("mcp.remote-server missing: %v", config)
				}
				if entry["type"] != "remote" {
					t.Errorf("type = %v, want remote", entry["type"])
				}
				if entry["url"] != "https://example.com/mcp" {
					t.Errorf("url = %v", entry["url"])
				}
				headers, _ := entry["headers"].(map[string]interface{})
				if headers["Authorization"] != "Bearer secret123" {
					t.Errorf("header not resolved: %v", headers["Authorization"])
				}
			},
		},
		"reinstall removes existing entry then re-adds": {
			seed: func(path string) {
				// Seed an OLD version of my-server so reinstall must remove+re-add.
				seed := map[string]interface{}{
					"mcp": map[string]interface{}{
						mcpName: ocEntry{Type: "local", Command: []string{"old-cmd"}},
					},
				}
				data, _ := json.MarshalIndent(seed, "", "  ")
				_ = os.WriteFile(path, data, 0o644)
			},
			reinstall: true,
			assert: func(t *testing.T, config map[string]interface{}) {
				mcpMap, _ := config["mcp"].(map[string]interface{})
				entry, ok := mcpMap[mcpName].(map[string]interface{})
				if !ok {
					t.Fatalf("mcp.%s missing after reinstall", mcpName)
				}
				cmd, _ := entry["command"].([]interface{})
				if len(cmd) != 2 || cmd[0] != "node" {
					t.Errorf("entry not re-added with new command: %v", cmd)
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "opencode.json")

			var beforeBytes []byte
			if tt.seed != nil {
				tt.seed(configPath)
				beforeBytes, _ = os.ReadFile(configPath)
			}

			mcps := tt.mcps
			if mcps == nil {
				mcps = []MCP{{Name: mcpName, Command: "node", Args: []string{"server.js"}}}
			}
			env := tt.env
			if env == nil {
				env = map[string]string{}
			}

			var err error
			_ = captureStdout(t, func() {
				err = InstallOpencode(mcps, env, configPath, tt.dryRun, tt.reinstall)
			})
			if err != nil {
				t.Fatalf("InstallOpencode returned error: %v", err)
			}

			if tt.wantNoFile {
				if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
					t.Fatalf("expected no file written, stat err = %v", statErr)
				}
				return
			}

			data, readErr := os.ReadFile(configPath)
			if readErr != nil {
				t.Fatalf("config not written: %v", readErr)
			}

			// A skip path must leave the file byte-for-byte unchanged.
			if tt.wantUnchanged && string(data) != string(beforeBytes) {
				t.Errorf("file was rewritten despite identical entry")
			}

			var config map[string]interface{}
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatalf("config not valid JSON: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, config)
			}
		})
	}
}
