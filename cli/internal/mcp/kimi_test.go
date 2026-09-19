package mcp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Every schema rule asserted here was confirmed against the real Kimi Code CLI
// 2.0.1: each invalid case below made it log "mcp config initial load failed"
// and load no MCP server at all, and each valid one loaded.
// TestValidateKimiEntry_URL: Kimi validates with zod's url(), which is
// `new URL(value.trim())` — no protocol constraint, and no host constraint
// beyond what the URL standard imposes itself. Rows marked "measured" were run
// against the real 2.0.1 with a loading entry and a known-bad entry as
// controls in the same batch; the rest are `new URL` in node, the same parser.
// It matters in both directions: this feeds a warning that tells the user an
// entry is costing them every MCP server in the file.
func TestValidateKimiEntry_URL(t *testing.T) {
	cases := map[string]struct {
		url  string
		want bool // whether Kimi loads it
	}{
		"a plain https url":       {url: "https://e.com/mcp", want: true},
		"a port":                  {url: "http://e.com:8080/mcp", want: true},
		"an ipv6 host":            {url: "http://[::1]:8080/mcp", want: true},
		"an uppercase scheme":     {url: "HTTPS://E.com", want: true},
		"a non-http scheme":       {url: "ftp://e.com/x", want: true},
		"a file url with no host": {url: "file:///etc/passwd", want: true},
		"surrounding whitespace":  {url: "  https://e.com/mcp  ", want: true},
		// measured: port 0 is a valid port to the URL standard.
		"a port of zero": {url: "https://e.com:0/mcp", want: true},

		"no scheme at all":          {url: "not a url", want: false},
		"a scheme and nothing else": {url: "https://", want: false},
		"an empty string":           {url: "", want: false},
		"a port out of range":       {url: "https://e.com:99999999999999/mcp", want: false},
		// measured: Go fails on these too, and tolerating every parse error
		// was letting them through.
		"a port that is not a number":   {url: "https://e.com:abc/", want: false},
		"an unterminated ipv6 bracket":  {url: "https://[abc", want: false},
		"a space in the host":           {url: "http://e xample.com/", want: false},
		"two ports":                     {url: "https://e.com:8080:9090/", want: false},
		"userinfo but no host":          {url: "http://user@:80", want: false},
		"a bad port on a custom scheme": {url: "custom://e.com:abc/x", want: false},
		"an unterminated ipv6 literal":  {url: "https://[::1", want: false},
		"a NUL byte":                    {url: "https://e\x00.com/", want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			entry, err := json.Marshal(map[string]string{"transport": "http", "url": tc.url})
			if err != nil {
				t.Fatal(err)
			}
			got := validateKimiEntry(entry)
			if tc.want && got != nil {
				t.Errorf("rejected %q (%v), but Kimi loads it", tc.url, got)
			}
			if !tc.want && got == nil {
				t.Errorf("accepted %q, but Kimi rejects the whole file for it", tc.url)
			}
		})
	}
}

// TestValidateKimiEntry_URLGoIsStricter: the three classes where Go's parser
// refuses a URL the URL constructor accepts. They are the only parse failures
// devexp tolerates — everything else Go refuses, Kimi refuses too — so they
// are pinned apart from the matrix above, as their own rule.
func TestValidateKimiEntry_URLGoIsStricter(t *testing.T) {
	for name, u := range map[string]string{
		"a stray percent escape":  "https://e.com/%zz",
		"an embedded tab":         "https://e.com\t/mcp",
		"a space in the userinfo": "https://user:pa ss@e.com/mcp",
	} {
		t.Run(name, func(t *testing.T) {
			entry, err := json.Marshal(map[string]string{"transport": "http", "url": u})
			if err != nil {
				t.Fatal(err)
			}
			if err := validateKimiEntry(entry); err != nil {
				t.Errorf("rejected %q (%v), but Kimi loads it", u, err)
			}
		})
	}
}

func TestValidateKimiEntry(t *testing.T) {
	cases := map[string]struct {
		entry   string
		wantErr string // "" = valid
	}{
		"a stdio entry":                         {entry: `{"transport":"stdio","command":"npx","args":["-y","pkg"],"env":{"A":"b"},"cwd":"/tmp"}`},
		"an http entry":                         {entry: `{"transport":"http","url":"https://example.com/mcp","headers":{"X":"y"}}`},
		"an sse entry":                          {entry: `{"transport":"sse","url":"https://example.com/sse"}`},
		"no transport, with a command":          {entry: `{"command":"npx"}`},
		"no transport, with a url":              {entry: `{"url":"https://example.com/mcp"}`},
		"unknown keys, which Kimi tolerates":    {entry: `{"command":"npx","somethingElse":1}`},
		"the common fields":                     {entry: `{"command":"npx","enabled":true,"deferred":false,"startupTimeoutMs":1000,"toolTimeoutMs":2000,"enabledTools":["a"],"disabledTools":["b"]}`},
		"stdio's executor and runtime_id":       {entry: `{"command":"npx","executor":"kaos","runtime_id":"r1"}`},
		"a remote entry's auth and bearer name": {entry: `{"transport":"http","url":"https://e.com","auth":"oauth","bearerTokenEnvVar":"TOK"}`},

		"an unknown transport":            {entry: `{"transport":"carrier-pigeon","command":"x"}`, wantErr: "not one of Kimi's transports"},
		"a transport that is a number":    {entry: `{"transport":1,"command":"x"}`, wantErr: `"transport" is not a string`},
		"neither command nor url":         {entry: `{"env":{"A":"b"}}`, wantErr: "no \"command\" or \"url\""},
		"an empty command":                {entry: `{"transport":"stdio","command":""}`, wantErr: `"command" is empty`},
		"a missing command":               {entry: `{"transport":"stdio","args":["x"]}`, wantErr: `has no "command"`},
		"args that are not strings":       {entry: `{"command":"sh","args":[1]}`, wantErr: `"args" is not a list of strings`},
		"env values that are not strings": {entry: `{"command":"sh","env":{"A":1}}`, wantErr: `"env" is not an object of strings`},
		"a url that is not one":           {entry: `{"transport":"http","url":"not a url"}`, wantErr: `"url" is not a URL`},
		"a url with no host":              {entry: `{"transport":"http","url":"https://"}`, wantErr: `"url" is not a URL`},
		"a missing url":                   {entry: `{"transport":"sse"}`, wantErr: `has no "url"`},
		"headers that are not strings":    {entry: `{"transport":"http","url":"https://e.com","headers":{"X":2}}`, wantErr: `"headers" is not an object of strings`},
		"an out-of-range timeout":         {entry: `{"command":"sh","startupTimeoutMs":0}`, wantErr: `"startupTimeoutMs" is not a whole number`},
		"a timeout that is not a number":  {entry: `{"command":"sh","toolTimeoutMs":"soon"}`, wantErr: `"toolTimeoutMs" is not a number`},
		"an enabled that is not a bool":   {entry: `{"command":"sh","enabled":"yes"}`, wantErr: `"enabled" is not true or false`},
		"an unknown executor":             {entry: `{"command":"sh","executor":"docker"}`, wantErr: `"executor" is "docker"`},
		"an empty bearer token env var":   {entry: `{"transport":"http","url":"https://e.com","bearerTokenEnvVar":""}`, wantErr: `"bearerTokenEnvVar" is empty`},
		"an entry that is not an object":  {entry: `"nope"`, wantErr: "is not a JSON object"},

		// encoding/json decodes null into a slice, map, bool or string without
		// complaining; zod's .optional() means absent, and Kimi rejects the
		// whole file for an explicit null. Each of these was confirmed against
		// the real 2.0.1.
		"null args":           {entry: `{"command":"echo","args":null}`, wantErr: `"args" is null`},
		"null env":            {entry: `{"command":"echo","env":null}`, wantErr: `"env" is null`},
		"null cwd":            {entry: `{"command":"echo","cwd":null}`, wantErr: `"cwd" is null`},
		"null enabled":        {entry: `{"command":"echo","enabled":null}`, wantErr: `"enabled" is null`},
		"null headers":        {entry: `{"transport":"http","url":"https://e.com","headers":null}`, wantErr: `"headers" is null`},
		"null transport":      {entry: `{"transport":null,"command":"echo"}`, wantErr: `"transport" is null`},
		"null timeout":        {entry: `{"command":"echo","startupTimeoutMs":null}`, wantErr: `"startupTimeoutMs" is null`},
		"null executor":       {entry: `{"command":"echo","executor":null}`, wantErr: `"executor" is null`},
		"null tool list":      {entry: `{"command":"echo","enabledTools":null}`, wantErr: `"enabledTools" is null`},
		"null bearer env var": {entry: `{"transport":"http","url":"https://e.com","bearerTokenEnvVar":null}`, wantErr: `"bearerTokenEnvVar" is null`},
		"null url":            {entry: `{"transport":"http","url":null}`, wantErr: `"url" is null`},
		"null command":        {entry: `{"transport":"stdio","command":null}`, wantErr: `"command" is null`},
		// A null *member* is the same hole one level down: decoded into
		// []string or map[string]string it arrives as "", and Kimi rejects
		// array(string()) / record(string(), string()) for it. Measured
		// against 2.0.1 with controls.
		"a null inside args":        {entry: `{"command":"echo","args":["a",null]}`, wantErr: `"args" has a null at position 1`},
		"a null inside env":         {entry: `{"command":"echo","env":{"A":null}}`, wantErr: `"env" has a null for "A"`},
		"a null inside headers":     {entry: `{"transport":"http","url":"https://e.com","headers":{"X":null}}`, wantErr: `"headers" has a null for "X"`},
		"a null inside a tool list": {entry: `{"command":"echo","enabledTools":["a",null]}`, wantErr: `"enabledTools" has a null at position 1`},
		"a number inside args":      {entry: `{"command":"echo","args":["a",1]}`, wantErr: `"args" is not a list of strings`},
		"a number inside env":       {entry: `{"command":"echo","env":{"A":1}}`, wantErr: `"env" is not an object of strings`},
		// Kimi strips unknown keys before validating, null or not.
		"an unknown key that is null": {entry: `{"command":"echo","whatever":null}`},
		// Kimi infers from `typeof obj.command === "string"`, so a null
		// command beside a real url is an http entry, not a broken stdio one.
		"a null command beside a url": {entry: `{"command":null,"url":"https://e.com/mcp"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateKimiEntry(json.RawMessage(tc.entry))
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("error = %v, want it accepted", err)
			case tc.wantErr != "" && err == nil:
				t.Errorf("accepted, want an error containing %q", tc.wantErr)
			case tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func kimiFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.json")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// install runs InstallKimi and returns what it printed, so every case can
// assert on the output the user sees as well as on the file.
func install(t *testing.T, path string, mcps []MCP, env, owned map[string]string, dryRun, reinstall bool) (map[string]string, string, error) {
	t.Helper()
	var got map[string]string
	var err error
	out := captureStdout(t, func() {
		got, err = InstallKimi(mcps, env, path, owned, dryRun, reinstall)
	})
	return got, out, err
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s is not valid JSON (%v):\n%s", path, err, data)
	}
	return v
}

func serversOf(t *testing.T, path string) map[string]any {
	t.Helper()
	servers, ok := readJSON(t, path)["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no mcpServers object", path)
	}
	return servers
}

var context7 = MCP{Name: "context7", Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}}

// ── The merge ─────────────────────────────────────────────────────────────────

func TestInstallKimi_Fresh(t *testing.T) {
	path := kimiFile(t, "")
	owned, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	// ui.Added and ui.Removed colour the marker, so the assertion holds the
	// reset code and the name rather than "+ context7".
	if !strings.Contains(out, "\033[0m context7") || !strings.Contains(out, "Saved: "+strconv.Quote(path)) {
		t.Errorf("output does not report the write:\n%s", out)
	}

	want := map[string]any{
		"transport": "stdio",
		"command":   "npx",
		"args":      []any{"-y", "@upstash/context7-mcp"},
	}
	if got := serversOf(t, path)["context7"]; !reflect.DeepEqual(got, want) {
		t.Errorf("entry = %#v, want %#v", got, want)
	}
	// The entry holds resolved values from mcps/.env, tokens included.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 for a new mcp.json", fi.Mode().Perm())
	}
	if owned["context7"] == "" {
		t.Errorf("owned = %v, want a fingerprint for the entry devexp wrote", owned)
	}
	// Kimi's own writer ends the file with a newline; so does devexp.
	data, _ := os.ReadFile(path)
	if !strings.HasSuffix(string(data), "}\n") {
		t.Errorf("file does not end in a newline:\n%q", data)
	}
}

func TestInstallKimi_Transports(t *testing.T) {
	path := kimiFile(t, "")
	mcps := []MCP{
		{Name: "remote", Transport: "http", URL: "https://example.com/mcp?a=1&b=2", Headers: map[string]string{"Authorization": "Bearer ${TOKEN}"}},
		{Name: "streamed", Transport: "sse", URL: "https://example.com/sse"},
		{Name: "local", Command: "node", Args: []string{"${DIR}/index.js"}, Env: map[string]string{"MODE": "on"}},
	}
	_, out, err := install(t, path, mcps, map[string]string{"TOKEN": "s3cret", "DIR": "/opt"}, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	servers := serversOf(t, path)
	remote := servers["remote"].(map[string]any)
	if remote["transport"] != "http" || remote["url"] != "https://example.com/mcp?a=1&b=2" {
		t.Errorf("http entry = %#v", remote)
	}
	// encoding/json escapes & by default, which would rewrite a query string
	// devexp is only copying.
	if data, _ := os.ReadFile(path); strings.Contains(string(data), `\u0026`) {
		t.Errorf("the & in the URL was escaped:\n%s", data)
	}
	if remote["headers"].(map[string]any)["Authorization"] != "Bearer s3cret" {
		t.Errorf("headers were not resolved: %#v", remote)
	}
	if servers["streamed"].(map[string]any)["transport"] != "sse" {
		t.Errorf("sse entry = %#v — Kimi never infers sse, so it has to be written", servers["streamed"])
	}
	local := servers["local"].(map[string]any)
	if got := local["args"].([]any)[0]; got != "/opt/index.js" {
		t.Errorf("args = %v, want ${DIR} resolved at install time", local["args"])
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "${") {
		t.Errorf("a placeholder reached the file — Kimi expands nothing:\n%s", data)
	}
}

func TestInstallKimi_PreservesWhatItDidNotWrite(t *testing.T) {
	path := kimiFile(t, `{
  "$schema": "https://example.com/schema.json",
  "mcpServers": {
    "mine": {"transport": "sse", "url": "https://mine.example/sse", "headers": {"X-Mine": "1"}},
    "other": {"command": "my-tool", "args": ["--serve"], "startupTimeoutMs": 90000}
  },
  "note": {"nested": [1, 2.5, true, null]}
}
`)
	_, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	doc := readJSON(t, path)
	if doc["$schema"] != "https://example.com/schema.json" {
		t.Errorf("an unrelated top-level key was lost: %#v", doc)
	}
	wantNote := map[string]any{"nested": []any{float64(1), 2.5, true, nil}}
	if !reflect.DeepEqual(doc["note"], wantNote) {
		t.Errorf("note = %#v, want %#v", doc["note"], wantNote)
	}
	servers := serversOf(t, path)
	mine := servers["mine"].(map[string]any)
	if mine["url"] != "https://mine.example/sse" || mine["headers"].(map[string]any)["X-Mine"] != "1" {
		t.Errorf("a user entry changed: %#v", mine)
	}
	// A number the user wrote comes back as they wrote it, not rounded
	// through float64 and re-rendered.
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "90000") {
		t.Errorf("a user's number was re-rendered:\n%s", data)
	}
	// Key order is the user's, with devexp's entry appended.
	data, _ := os.ReadFile(path)
	if i, j := strings.Index(string(data), `"$schema"`), strings.Index(string(data), `"mcpServers"`); i > j {
		t.Errorf("top-level keys were reordered:\n%s", data)
	}
	if i, j := strings.Index(string(data), `"mine"`), strings.Index(string(data), `"context7"`); i > j {
		t.Errorf("the user's entries were reordered:\n%s", data)
	}
}

func TestInstallKimi_SecondRunWritesNothing(t *testing.T) {
	path := kimiFile(t, "")
	owned, _, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	again, out, err := install(t, path, []MCP{context7}, nil, owned, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "already configured") {
		t.Errorf("output does not say the entry was already there:\n%s", out)
	}
	if strings.Contains(out, "Saved:") || strings.Contains(out, "updated") {
		t.Errorf("a second run with unchanged MCPs reported a write (#159):\n%s", out)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("the file changed:\nbefore %s\nafter  %s", before, after)
	}
	if fi, _ := os.Stat(path); !fi.ModTime().Equal(old) {
		t.Errorf("the file was rewritten: mtime moved to %v", fi.ModTime())
	}
	if !reflect.DeepEqual(again, owned) {
		t.Errorf("ownership changed: %v then %v", owned, again)
	}
}

func TestInstallKimi_Ownership(t *testing.T) {
	changed := MCP{Name: "context7", Command: "npx", Args: []string{"-y", "@upstash/context7-mcp@2"}}

	t.Run("a changed registry entry is updated", func(t *testing.T) {
		path := kimiFile(t, "")
		owned, _, err := install(t, path, []MCP{context7}, nil, nil, false, false)
		if err != nil {
			t.Fatal(err)
		}
		again, out, err := install(t, path, []MCP{changed}, nil, owned, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "context7 — updated") {
			t.Errorf("output does not report the update:\n%s", out)
		}
		args := serversOf(t, path)["context7"].(map[string]any)["args"].([]any)
		if args[1] != "@upstash/context7-mcp@2" {
			t.Errorf("entry = %v, want the new args", args)
		}
		if again["context7"] == owned["context7"] {
			t.Errorf("the fingerprint did not change with the entry")
		}
	})

	t.Run("a user's entry of the same name is left alone", func(t *testing.T) {
		path := kimiFile(t, `{"mcpServers":{"context7":{"command":"my-context7","args":["--mine"]}}}`)
		before, _ := os.ReadFile(path)
		owned, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "user-defined entry") {
			t.Errorf("output does not explain the skip:\n%s", out)
		}
		if after, _ := os.ReadFile(path); string(after) != string(before) {
			t.Errorf("the user's entry was rewritten:\n%s", after)
		}
		if _, claimed := owned["context7"]; claimed {
			t.Errorf("devexp claimed an entry it did not write: %v", owned)
		}
	})

	t.Run("a devexp entry the user has edited is left alone", func(t *testing.T) {
		path := kimiFile(t, "")
		owned, _, err := install(t, path, []MCP{context7}, nil, nil, false, false)
		if err != nil {
			t.Fatal(err)
		}
		edited := `{"mcpServers":{"context7":{"transport":"stdio","command":"npx","args":["-y","@upstash/context7-mcp"],"env":{"MINE":"1"}}}}`
		if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
			t.Fatal(err)
		}
		again, out, err := install(t, path, []MCP{context7}, nil, owned, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "user-defined entry") {
			t.Errorf("output does not explain the skip:\n%s", out)
		}
		// And says so once. This MCP is still installed — pruning is for the
		// ones that are not, so it has nothing to say here.
		if strings.Contains(out, "no longer installed by devexp") {
			t.Errorf("a still-selected MCP was reported as no longer installed:\n%s", out)
		}
		if got, _ := os.ReadFile(path); string(got) != edited {
			t.Errorf("the user's edit was overwritten:\n%s", got)
		}
		if _, claimed := again["context7"]; claimed {
			t.Errorf("devexp kept claiming an entry the user changed: %v", again)
		}
	})

	t.Run("--reinstall-mcps replaces a user's entry", func(t *testing.T) {
		path := kimiFile(t, `{"mcpServers":{"context7":{"command":"my-context7"}}}`)
		owned, out, err := install(t, path, []MCP{context7}, nil, nil, false, true)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "context7 — updated") {
			t.Errorf("output does not report the replacement:\n%s", out)
		}
		if got := serversOf(t, path)["context7"].(map[string]any)["command"]; got != "npx" {
			t.Errorf("command = %v, want devexp's", got)
		}
		if owned["context7"] == "" {
			t.Errorf("devexp did not record what it just wrote: %v", owned)
		}
	})
}

func TestInstallKimi_RemovesWhatItNoLongerInstalls(t *testing.T) {
	t.Run("an entry devexp wrote and no longer installs is removed", func(t *testing.T) {
		path := kimiFile(t, "")
		owned, _, err := install(t, path, []MCP{context7, {Name: "gone", Command: "old-tool"}}, nil, nil, false, false)
		if err != nil {
			t.Fatal(err)
		}
		again, out, err := install(t, path, []MCP{context7}, nil, owned, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "\033[0m gone") || !strings.Contains(out, "Saved:") {
			t.Errorf("output does not report the removal:\n%s", out)
		}
		if _, still := serversOf(t, path)["gone"]; still {
			t.Errorf("the stale entry is still there")
		}
		if _, tracked := again["gone"]; tracked {
			t.Errorf("a removed entry is still tracked: %v", again)
		}
	})

	t.Run("one the user has edited since is kept", func(t *testing.T) {
		path := kimiFile(t, "")
		owned, _, err := install(t, path, []MCP{context7, {Name: "gone", Command: "old-tool"}}, nil, nil, false, false)
		if err != nil {
			t.Fatal(err)
		}
		edited := `{"mcpServers":{"context7":{"transport":"stdio","command":"npx","args":["-y","@upstash/context7-mcp"]},"gone":{"transport":"stdio","command":"old-tool","args":["--mine"]}}}`
		if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
			t.Fatal(err)
		}
		_, out, err := install(t, path, []MCP{context7}, nil, owned, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "has been edited") {
			t.Errorf("output does not explain why it stayed:\n%s", out)
		}
		if got, _ := os.ReadFile(path); string(got) != edited {
			t.Errorf("an edited entry was removed anyway:\n%s", got)
		}
	})
}

func TestInstallKimi_MissingRequiredEnv(t *testing.T) {
	path := kimiFile(t, "")
	uiInspector := MCP{
		Name:              "ui-inspector",
		Command:           "node",
		Args:              []string{"${UI_INSPECTOR_DIR}/dist/index.js"},
		RequiredEnv:       []string{"UI_INSPECTOR_DIR"},
		SetupInstructions: "clone the repo\nthen set UI_INSPECTOR_DIR",
	}
	owned, out, err := install(t, path, []MCP{uiInspector}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	for _, want := range []string{"[REQUIRED]", "UI_INSPECTOR_DIR", "clone the repo", "will not be available"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	if len(owned) != 0 {
		t.Errorf("owned = %v, want nothing recorded", owned)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a file was written for an MCP that was never configured (%v)", err)
	}
}

func TestInstallKimi_SkipsProjectScope(t *testing.T) {
	path := kimiFile(t, "")
	_, out, err := install(t, path, []MCP{{Name: "proj", Command: "x", Scope: "project"}}, nil, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "project-scoped") {
		t.Errorf("output does not explain the skip:\n%s", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a project-scoped MCP was written to the user file (%v)", err)
	}
}

// A registry entry devexp itself cannot write validly is skipped rather than
// written: one entry Kimi rejects costs the user every MCP server in the file.
func TestInstallKimi_SkipsAnEntryKimiWouldReject(t *testing.T) {
	path := kimiFile(t, `{"mcpServers":{}}`)
	before, _ := os.ReadFile(path)
	_, out, err := install(t, path, []MCP{{Name: "bad", Transport: "http", URL: "not a url"}}, nil, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Kimi rejects") {
		t.Errorf("output does not say why it was skipped:\n%s", out)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("the file was written anyway:\n%s", after)
	}
}

// An entry that is already in the file and already unloadable is left alone,
// but has to be reported: while it is there Kimi loads no MCP server at all.
func TestInstallKimi_WarnsAboutAnInvalidEntryItDidNotWrite(t *testing.T) {
	path := kimiFile(t, `{"mcpServers":{"theirs":{"transport":"carrier-pigeon","command":"x"}}}`)
	_, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "no MCP servers at all") {
		t.Errorf("output does not say what the bad entry costs:\n%s", out)
	}
	servers := serversOf(t, path)
	if _, ok := servers["context7"]; !ok {
		t.Errorf("devexp's entry was not written: %v", servers)
	}
	if theirs := servers["theirs"].(map[string]any); theirs["transport"] != "carrier-pigeon" {
		t.Errorf("the invalid entry was changed: %#v", theirs)
	}
}

func TestInstallKimi_DryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "mcp.json")
	owned, out, err := install(t, path, []MCP{context7}, nil, nil, true, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "add mcpServers.context7 (stdio) to "+strconv.Quote(path)) {
		t.Errorf("output does not describe the change:\n%s", out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a dry run created %v", entries)
	}
	if owned != nil {
		t.Errorf("owned = %v, want the previous ownership unchanged", owned)
	}
}

// ── Files devexp refuses to touch ─────────────────────────────────────────────

func TestInstallKimi_RefusesUnmergeableFiles(t *testing.T) {
	cases := map[string]string{
		"not JSON at all":                     `{ nope`,
		"a comment, which Kimi rejects too":   "{\n  // mine\n  \"mcpServers\": {}\n}",
		"a trailing comma":                    `{"mcpServers": {},}`,
		"a top level that is not an object":   `["mcpServers"]`,
		"an mcpServers that is not an object": `{"mcpServers": []}`,
		"a second value after the first":      `{"mcpServers": {}} {"mcpServers": {}}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := kimiFile(t, content)
			owned := map[string]string{"context7": "old"}
			got, out, err := install(t, path, []MCP{context7}, nil, owned, false, false)

			var refused *ConfigRefusedError
			if !errors.As(err, &refused) {
				t.Fatalf("error = %v, want a ConfigRefusedError\n%s", err, out)
			}
			if refused.Path != path || !strings.Contains(strings.Join(refused.Servers, ","), "context7") {
				t.Errorf("refusal = %+v, want it to name the file and the servers", refused)
			}
			if data, _ := os.ReadFile(path); string(data) != content {
				t.Errorf("the file changed:\n%s", data)
			}
			if !reflect.DeepEqual(got, owned) {
				t.Errorf("owned = %v, want what devexp knew before it gave up", got)
			}
		})
	}
}

func TestInstallKimi_EmptyFileIsAnEmptyConfig(t *testing.T) {
	path := kimiFile(t, "   \n")
	_, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	if _, ok := serversOf(t, path)["context7"]; !ok {
		t.Errorf("nothing was written into a blank file")
	}
}

func TestInstallKimi_NoMcpServersKey(t *testing.T) {
	// A file Kimi accepts, with no servers in it yet.
	path := kimiFile(t, `{"note":"mine"}`)
	_, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	if readJSON(t, path)["note"] != "mine" {
		t.Errorf("the user's key was lost")
	}
	if _, ok := serversOf(t, path)["context7"]; !ok {
		t.Errorf("no entry was added")
	}
}

// A dotfiles-managed mcp.json is a symlink; the file it points at is what gets
// replaced, and the link stays a link (#124).
func TestInstallKimi_FollowsASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "mcp.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	_, out, err := install(t, link, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced by a regular file")
	}
	if _, ok := serversOf(t, real)["context7"]; !ok {
		t.Errorf("the file the link points at was not updated")
	}
	if got, _ := os.Stat(real); got.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want the existing file's 0644 kept", got.Mode().Perm())
	}
}

func TestInstallKimi_RefusesAReadOnlyFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes a read-only file regardless")
	}
	content := `{"mcpServers":{}}`
	path := kimiFile(t, content)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o600) }) //nolint:errcheck

	_, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err == nil {
		t.Fatalf("install succeeded on a read-only file\n%s", out)
	}
	if !strings.Contains(err.Error(), "left untouched") {
		t.Errorf("error = %v, want it to say the file was left alone", err)
	}
	// The change lines are held back until the write succeeds: "+ context7"
	// above a failure is the line a skimming reader keeps.
	if strings.Contains(out, "\033[0m context7") || strings.Contains(out, "Saved:") {
		t.Errorf("reported a change that never reached the file:\n%s", out)
	}
	if data, _ := os.ReadFile(path); string(data) != content {
		t.Errorf("the file changed:\n%s", data)
	}
}

// verifyKimiConfig is the last check before the file is replaced: whatever
// went wrong between building the entries and rendering the document, the old
// file stays. Nothing in the merge can reach it today, which is the point —
// it is what makes a future bug in the rendering a refusal rather than a user
// with no MCP servers.
func TestVerifyKimiConfig(t *testing.T) {
	wrote := map[string]string{"context7": "hash"}
	cases := map[string]struct {
		data    string
		wantErr string
	}{
		"a file devexp wrote":           {data: `{"mcpServers":{"context7":{"transport":"stdio","command":"npx"}}}`},
		"not JSON":                      {data: `{ nope`, wantErr: "would not have been readable"},
		"no mcpServers":                 {data: `{"other":1}`, wantErr: `would have had no "mcpServers"`},
		"mcpServers not an object":      {data: `{"mcpServers":[]}`, wantErr: "is not an object"},
		"the entry did not make it":     {data: `{"mcpServers":{"other":{"command":"x"}}}`, wantErr: `would not have contained the "context7" entry`},
		"the entry is one Kimi rejects": {data: `{"mcpServers":{"context7":{"transport":"stdio","command":""}}}`, wantErr: `entry Kimi rejects`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := verifyKimiConfig([]byte(tc.data), wrote)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("error = %v, want it accepted", err)
			case tc.wantErr != "" && err == nil:
				t.Errorf("accepted, want an error containing %q", tc.wantErr)
			case tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// An entry with an explicit null is one Kimi rejects, and it takes every other
// MCP server in the file with it — so the warning that exists to say that has
// to fire for it. This is the case the validator used to wave through.
func TestInstallKimi_WarnsAboutANullField(t *testing.T) {
	path := kimiFile(t, `{"mcpServers":{"theirs":{"command":"echo","args":null}}}`)
	_, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "no MCP servers at all") || !strings.Contains(out, `"args" is null`) {
		t.Errorf("output does not report the null field:\n%s", out)
	}
}

// An MCP that is selected but cannot be configured this run — an unset
// required_env in a fresh clone or in CI, a project-scoped entry, a registry
// entry devexp cannot write validly — is not an MCP that was deselected. The
// entry devexp wrote last run stays, and stays devexp's.
func TestInstallKimi_KeepsAnEntryItCannotConfigureThisRun(t *testing.T) {
	needsEnv := MCP{
		Name:        "ui-inspector",
		Command:     "node",
		Args:        []string{"${UI_INSPECTOR_DIR}/dist/index.js"},
		RequiredEnv: []string{"UI_INSPECTOR_DIR"},
	}
	cases := map[string]struct {
		mcp  MCP
		env  map[string]string
		want string // what the output has to say
	}{
		"a required env var is unset this run": {mcp: needsEnv, want: "[REQUIRED]"},
		"the registry entry became one Kimi rejects": {
			mcp:  MCP{Name: "ui-inspector", Transport: "http", URL: "not a url"},
			want: "Kimi rejects",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := kimiFile(t, "")
			env := map[string]string{"UI_INSPECTOR_DIR": "/opt/ui"}
			owned, out, err := install(t, path, []MCP{needsEnv}, env, nil, false, false)
			if err != nil {
				t.Fatalf("first install: %v\n%s", err, out)
			}
			before, _ := os.ReadFile(path)

			// Second run: same selection, but this one cannot write it.
			again, out, err := install(t, path, []MCP{tc.mcp}, tc.env, owned, false, false)

			if err != nil {
				t.Fatalf("second install: %v\n%s", err, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("output does not explain the skip (%q):\n%s", tc.want, out)
			}
			// ui.Removed's exact marker: the [REQUIRED] block and the
			// warnings name the MCP too, and only this line is a deletion.
			if strings.Contains(out, "\033[0;31m-\033[0m ui-inspector") {
				t.Errorf("the entry was deleted:\n%s", out)
			}
			if strings.Contains(out, "Saved:") {
				t.Errorf("the file was rewritten for an MCP that could not be configured:\n%s", out)
			}
			if after, _ := os.ReadFile(path); string(after) != string(before) {
				t.Errorf("the file changed:\nbefore %s\nafter  %s", before, after)
			}
			if again["ui-inspector"] != owned["ui-inspector"] {
				t.Errorf("ownership was dropped for an MCP that is still selected: %v", again)
			}
		})
	}
}

// An entry that merely happens to match what devexp installs is the user's.
// Adopting it would let a later run, with that MCP deselected, delete
// something they wrote.
func TestInstallKimi_DoesNotAdoptAnIdenticalUserEntry(t *testing.T) {
	identical := `{
  "mcpServers": {
    "context7": {
      "transport": "stdio",
      "command": "npx",
      "args": [
        "-y",
        "@upstash/context7-mcp"
      ]
    }
  }
}
`
	path := kimiFile(t, identical)
	owned, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "already configured") {
		t.Errorf("output does not report the match:\n%s", out)
	}
	if _, claimed := owned["context7"]; claimed {
		t.Errorf("devexp adopted an entry it never wrote: %v", owned)
	}

	// The run that used to delete it: the MCP is no longer selected.
	_, out, err = install(t, path, nil, nil, owned, false, false)
	if err != nil {
		t.Fatalf("second install: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(path); string(got) != identical {
		t.Errorf("a user-written entry was deleted:\n%s", got)
	}
}

// "mcpServers": null is not "no servers" — Kimi rejects the whole file for it.
func TestInstallKimi_NullMcpServers(t *testing.T) {
	t.Run("it is reported, and writing repairs the file", func(t *testing.T) {
		path := kimiFile(t, `{"note":"mine","mcpServers":null}`)
		_, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
		if err != nil {
			t.Fatalf("error = %v\n%s", err, out)
		}
		if !strings.Contains(out, `"mcpServers": null`) {
			t.Errorf("output does not report it:\n%s", out)
		}
		if _, ok := serversOf(t, path)["context7"]; !ok {
			t.Errorf("the file was not repaired")
		}
		if readJSON(t, path)["note"] != "mine" {
			t.Errorf("the user's key was lost")
		}
	})

	// The run that writes nothing is the one where the notice is all the user
	// gets, so it has to be there.
	t.Run("it is reported even when nothing is written", func(t *testing.T) {
		content := `{"mcpServers":null}`
		path := kimiFile(t, content)
		_, out, err := install(t, path, nil, nil, nil, false, false)
		if err != nil {
			t.Fatalf("error = %v\n%s", err, out)
		}
		if !strings.Contains(out, "loads no MCP servers at all") {
			t.Errorf("output does not report it:\n%s", out)
		}
		if got, _ := os.ReadFile(path); string(got) != content {
			t.Errorf("the file changed:\n%s", got)
		}
	})
}

func TestOwnsEntry(t *testing.T) {
	entry := json.RawMessage(`{"command":"npx"}`)
	fingerprint := entryFingerprint(entry)

	cases := map[string]struct {
		existing json.RawMessage
		recorded string
		want     bool
	}{
		"the entry devexp recorded": {existing: entry, recorded: fingerprint, want: true},
		"an entry that has changed": {existing: json.RawMessage(`{"command":"mine"}`), recorded: fingerprint},
		"nothing recorded":          {existing: entry, recorded: ""},
		// Both sides fingerprint to "" here. Comparing them equal would let
		// devexp claim — and then overwrite or delete — an entry it never
		// wrote, which is the hand-edited or truncated manifest case.
		"an unfingerprintable entry against an empty record": {existing: json.RawMessage(`{oops`), recorded: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ownsEntry(tc.existing, tc.recorded); got != tc.want {
				t.Errorf("ownsEntry(%s, %q) = %v, want %v", tc.existing, tc.recorded, got, tc.want)
			}
		})
	}
}

// The fingerprint decides whether an entry is still the one devexp wrote, so
// it has to see numbers as the file wrote them. Through float64, 1.0 and 1 are
// the same value and a 20-digit integer is neither of the numbers it was.
func TestEntryFingerprint_NumbersAsWritten(t *testing.T) {
	pairs := [][2]string{
		{`{"toolTimeoutMs":1.0}`, `{"toolTimeoutMs":1}`},
		{`{"n":10000000000000000001}`, `{"n":10000000000000000002}`},
	}
	for _, pair := range pairs {
		a, b := entryFingerprint(json.RawMessage(pair[0])), entryFingerprint(json.RawMessage(pair[1]))
		if a == "" || b == "" {
			t.Fatalf("no fingerprint for %s / %s", pair[0], pair[1])
		}
		if a == b {
			t.Errorf("%s and %s fingerprint the same — numbers are being rounded through float64", pair[0], pair[1])
		}
	}
	// The same bytes, differently formatted, still have to match: re-indenting
	// a file is not an edit.
	spaced := entryFingerprint(json.RawMessage("{\n  \"command\" : \"npx\"\n}"))
	if spaced != entryFingerprint(json.RawMessage(`{"command":"npx"}`)) {
		t.Errorf("whitespace changed the fingerprint")
	}
}

// A registry that changes an MCP to project scope is saying devexp never
// writes it to Kimi's user file — not "not this run". The entry devexp put
// there under the old scope is stale, so it goes the way any other
// no-longer-installed entry goes.
func TestInstallKimi_PrunesAnMCPThatBecameProjectScoped(t *testing.T) {
	path := kimiFile(t, "")
	owned, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("first install: %v\n%s", err, out)
	}

	scoped := context7
	scoped.Scope = "project"
	again, out, err := install(t, path, []MCP{scoped}, nil, owned, false, false)

	if err != nil {
		t.Fatalf("second install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "project-scoped") {
		t.Errorf("output does not explain the skip:\n%s", out)
	}
	if _, still := serversOf(t, path)["context7"]; still {
		t.Errorf("the entry devexp no longer installs here is still in the file")
	}
	if _, tracked := again["context7"]; tracked {
		t.Errorf("a removed entry is still tracked: %v", again)
	}
}

// --reinstall-mcps is the documented way to hand an entry back to devexp, so
// it has to work for the case that needs it most: a manifest that was lost,
// where the entry on disk is exactly what devexp installs and devexp cannot
// prove it wrote it.
func TestInstallKimi_ReinstallReclaimsAfterALostManifest(t *testing.T) {
	path := kimiFile(t, "")
	if _, out, err := install(t, path, []MCP{context7}, nil, nil, false, false); err != nil {
		t.Fatalf("first install: %v\n%s", err, out)
	}
	before, _ := os.ReadFile(path)

	// The manifest is gone: nothing is owned any more.
	plain, out, err := install(t, path, []MCP{context7}, nil, nil, false, false)
	if err != nil {
		t.Fatalf("second install: %v\n%s", err, out)
	}
	if _, claimed := plain["context7"]; claimed {
		t.Errorf("a normal run adopted the entry: %v", plain)
	}

	reclaimed, out, err := install(t, path, []MCP{context7}, nil, nil, false, true)

	if err != nil {
		t.Fatalf("reinstall: %v\n%s", err, out)
	}
	if reclaimed["context7"] == "" {
		t.Errorf("--reinstall-mcps did not hand the entry back: %v", reclaimed)
	}
	// Reclaiming is a manifest change, not a file change.
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("the file was rewritten:\n%s", after)
	}
	if strings.Contains(out, "Saved:") {
		t.Errorf("an unchanged entry was rewritten:\n%s", out)
	}
	// And from here on it is devexp's again: a registry change updates it.
	changed := MCP{Name: "context7", Command: "npx", Args: []string{"-y", "@upstash/context7-mcp@2"}}
	_, out, err = install(t, path, []MCP{changed}, nil, reclaimed, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "context7 — updated") {
		t.Errorf("the reclaimed entry is still treated as the user's:\n%s", out)
	}
}
