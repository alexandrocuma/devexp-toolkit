package hooks

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// captureOutput runs fn with os.Stdout redirected and returns what it printed.
// ui writes straight to stdout, so this is how install output is asserted
// (mirrors captureStdout in cli/cmd/install_test.go).
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		io.Copy(&b, r) //nolint:errcheck
		done <- b.String()
	}()
	defer func() { os.Stdout = orig }()
	fn()
	w.Close()
	return <-done
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

// writeOpencodeRepo writes hooks/opencode/ with the entry, utils.js,
// package.json and, per name, <name>.js plus a <name>.test.js that must never
// be installed.
func writeOpencodeRepo(t *testing.T, repoDir string, names ...string) {
	t.Helper()
	dir := filepath.Join(repoDir, "hooks", "opencode")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	files := map[string]string{
		"devexp-plugin.js": "/**\n * devexp-plugin.js — entry point for devexp opencode hooks\n */\nexport const DevExpPlugin = async () => ({});\n",
		"utils.js":         "/**\n * utils.js — shared utilities\n */\n",
		"package.json":     "{ \"type\": \"module\" }\n",
	}
	for _, n := range names {
		files[n+".js"] = "// module " + n + "\n"
		files[n+".test.js"] = "// test " + n + "\n"
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
}

// ocSpec builds an opencode target block for module hooks/opencode/<name>.js.
func ocSpec(name, export string, failClosed bool) TargetSpec {
	return TargetSpec{Event: "tool.execute.before", Module: "hooks/opencode/" + name + ".js", Export: export, FailClosed: failClosed}
}

// opencodeRegistry: two enabled hooks, one Claude-disabled hook switched on
// for opencode, and a Claude-only hook that opencode must never select.
func opencodeRegistry() Registry {
	graphify := ocSpec("graphify-read-guard", "graphifyReadGuard", false)
	graphify.Enabled = boolPtr(true)
	return Registry{
		{Name: "secret-guard", Enabled: true, Targets: map[string]TargetSpec{
			TargetClaudeCode: {Event: "PreToolUse", Script: "hooks/claude-code/secret-guard.sh"},
			TargetOpencode:   ocSpec("secret-guard", "secretGuard", true),
		}},
		{Name: "lint-on-save", Enabled: true, Targets: map[string]TargetSpec{
			TargetOpencode: {Event: "file.edited", Module: "hooks/opencode/lint-on-save.js", Export: "lintOnSave"},
		}},
		{Name: "graphify-read-guard", Enabled: false, Targets: map[string]TargetSpec{
			TargetOpencode: graphify,
		}},
		ccHook("claude-only", true, "PreToolUse", "Bash", "hooks/claude-code/claude-only.sh"),
	}
}

// listTree returns every file under root, slash-separated and relative.
func listTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	for _, rel := range listTree(t, root) {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", rel, err)
		}
		snap[rel] = string(data)
	}
	return snap
}

func names(hooks []Hook) []string {
	var out []string
	for _, h := range hooks {
		out = append(out, h.Name)
	}
	return out
}

// ── Selection ─────────────────────────────────────────────────────────────────

func TestSelectOpencode(t *testing.T) {
	offForOpencode := ocSpec("off", "off", false)
	offForOpencode.Enabled = boolPtr(false)

	tests := map[string]struct {
		registry Registry
		disabled []string
		want     []string
	}{
		"enabled hooks and opencode-enabled graphify, in registry order": {
			registry: opencodeRegistry(),
			want:     []string{"secret-guard", "lint-on-save", "graphify-read-guard"},
		},
		"disabled by name is excluded": {
			registry: opencodeRegistry(),
			disabled: []string{"lint-on-save", "graphify-read-guard"},
			want:     []string{"secret-guard"},
		},
		"opencode.enabled false on an enabled hook is excluded": {
			registry: Registry{{Name: "off", Enabled: true, Targets: map[string]TargetSpec{TargetOpencode: offForOpencode}}},
			want:     nil,
		},
		"empty module is excluded": {
			registry: Registry{{Name: "nomod", Enabled: true, Targets: map[string]TargetSpec{TargetOpencode: {Export: "x"}}}},
			want:     nil,
		},
		"claude-only hook is excluded": {
			registry: Registry{ccHook("cc", true, "PreToolUse", "Bash", "hooks/claude-code/cc.sh")},
			want:     nil,
		},
		"order follows the registry, not the disabled list": {
			registry: Registry{
				{Name: "b", Enabled: true, Targets: map[string]TargetSpec{TargetOpencode: ocSpec("b", "b", false)}},
				{Name: "a", Enabled: true, Targets: map[string]TargetSpec{TargetOpencode: ocSpec("a", "a", false)}},
			},
			disabled: []string{"z"},
			want:     []string{"b", "a"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := names(SelectOpencode(tt.registry, tt.disabled)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SelectOpencode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── hooks.json ────────────────────────────────────────────────────────────────

func TestOpencodeSelectionJSON(t *testing.T) {
	got, err := opencodeSelectionJSON(SelectOpencode(opencodeRegistry(), []string{"graphify-read-guard"}))
	if err != nil {
		t.Fatalf("opencodeSelectionJSON() error = %v", err)
	}
	want := `[
  {
    "name": "secret-guard",
    "module": "secret-guard.js",
    "export": "secretGuard",
    "failClosed": true
  },
  {
    "name": "lint-on-save",
    "module": "lint-on-save.js",
    "export": "lintOnSave",
    "failClosed": false
  }
]
`
	if string(got) != want {
		t.Errorf("opencodeSelectionJSON() =\n%s\nwant\n%s", got, want)
	}
}

func TestOpencodeSelectionJSON_Rejects(t *testing.T) {
	hook := func(module, export string) []Hook {
		return []Hook{{Name: "h", Enabled: true, Targets: map[string]TargetSpec{TargetOpencode: {Module: module, Export: export}}}}
	}
	tests := map[string][]Hook{
		"empty selection (the entry blocks on [])": nil,
		"module outside hooks/opencode":            hook("hooks/other/h.js", "h"),
		"module nested below hooks/opencode":       hook("hooks/opencode/sub/h.js", "h"),
		"module that is not a .js file name":       hook("hooks/opencode/h.ts", "h"),
		"module with a leading dot":                hook("hooks/opencode/.h.js", "h"),
		"module that is a test file":               hook("hooks/opencode/h.test.js", "h"),
		"module named utils.js":                    hook("hooks/opencode/utils.js", "h"),
		"module with a parent segment":             hook("hooks/opencode/../h.js", "h"),
		"empty export":                             hook("hooks/opencode/h.js", ""),
	}
	for name, selected := range tests {
		t.Run(name, func(t *testing.T) {
			if got, err := opencodeSelectionJSON(selected); err == nil {
				t.Errorf("opencodeSelectionJSON() = %s, want an error", got)
			}
		})
	}
}

// TestOpencodeSelectionJSON_Contract checks the Go writer against the contract
// the entry enforces (docs/development/hook-authoring-guide.md, "The
// devexp/hooks.json contract", which devexp-plugin.test.js case 11 also
// parses): the documented example is reproduced exactly from the real
// registry, and the full real selection is a non-empty array in registry order
// whose entries carry exactly name/module/export/failClosed.
func TestOpencodeSelectionJSON_Contract(t *testing.T) {
	registry, err := LoadRegistry(filepath.Join("..", "..", "..", "hooks", "registry.json"))
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}

	guide, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "development", "hook-authoring-guide.md"))
	if err != nil {
		t.Fatalf("ReadFile(guide) error = %v", err)
	}
	section := string(guide)[strings.Index(string(guide), "### The `devexp/hooks.json` contract"):]
	block := regexp.MustCompile("(?s)```json\n(.*?)```").FindStringSubmatch(section)
	if block == nil {
		t.Fatalf("no json example in the hooks.json contract section")
	}
	var documented []map[string]any
	if err := json.Unmarshal([]byte(block[1]), &documented); err != nil {
		t.Fatalf("documented example: %v", err)
	}
	var docNames []string
	for _, e := range documented {
		docNames = append(docNames, e["name"].(string))
	}
	var subset []Hook
	for _, h := range registry {
		for _, n := range docNames {
			if h.Name == n {
				subset = append(subset, h)
			}
		}
	}
	written, err := opencodeSelectionJSON(subset)
	if err != nil {
		t.Fatalf("opencodeSelectionJSON() error = %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(written, &got); err != nil {
		t.Fatalf("written hooks.json: %v", err)
	}
	if !reflect.DeepEqual(got, documented) {
		t.Errorf("writer output for %v = %v, documented contract = %v", docNames, got, documented)
	}

	selected := SelectOpencode(registry, nil)
	written, err = opencodeSelectionJSON(selected)
	if err != nil {
		t.Fatalf("opencodeSelectionJSON(full) error = %v", err)
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(written, &entries); err != nil {
		t.Fatalf("full hooks.json is not an array of objects: %v", err)
	}
	if len(entries) == 0 || len(entries) != len(selected) {
		t.Fatalf("full hooks.json has %d entries, want %d (> 0)", len(entries), len(selected))
	}
	for i, e := range entries {
		var keys []string
		for k := range e {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if !reflect.DeepEqual(keys, []string{"export", "failClosed", "module", "name"}) {
			t.Errorf("entry %d keys = %v, want exactly export/failClosed/module/name", i, keys)
		}
		var entry selectionEntry
		for k, v := range map[string]any{"name": &entry.Name, "module": &entry.Module, "export": &entry.Export, "failClosed": &entry.FailClosed} {
			if err := json.Unmarshal(e[k], v); err != nil {
				t.Errorf("entry %d %s has the wrong type: %v", i, k, err)
			}
		}
		spec, _ := selected[i].Target(TargetOpencode)
		if entry.Name != selected[i].Name || !moduleFile.MatchString(entry.Module) || entry.Export != spec.Export || entry.FailClosed != spec.FailClosed {
			t.Errorf("entry %d = %+v, want registry hook %s (%+v) in order", i, entry, selected[i].Name, spec)
		}
	}
}

// TestRepoOpencodeModules checks the real hooks/opencode sources against what
// the installer assumes: every registry module is installable and imports no
// relative file but ./utils.js (which always ships), and the entry still
// carries the header the installer uses to recognise its own devexp.js.
func TestRepoOpencodeModules(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	registry, err := LoadRegistry(filepath.Join(root, "hooks", "registry.json"))
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	relImport := regexp.MustCompile(`(?m)(?:from|import)\s*\(?\s*['"](\.[^'"]*)['"]`)
	for _, h := range registry {
		if _, err := opencodeModuleBase(h); err != nil {
			t.Errorf("%v", err)
			continue
		}
		spec, _ := h.Target(TargetOpencode)
		src, err := os.ReadFile(filepath.Join(root, spec.Module))
		if err != nil {
			t.Errorf("%s: %v", h.Name, err)
			continue
		}
		for _, m := range relImport.FindAllStringSubmatch(string(src), -1) {
			if m[1] != "./utils.js" {
				t.Errorf("%s imports %q; the installed devexp/ holds only the modules, utils.js and package.json", spec.Module, m[1])
			}
		}
	}
	entry, err := os.ReadFile(filepath.Join(root, "hooks", "opencode", opencodeEntrySrc))
	if err != nil {
		t.Fatalf("ReadFile(entry) error = %v", err)
	}
	if !isDevexpEntry(entry) {
		t.Errorf("hooks/opencode/%s lost its header; the installer would refuse to update an installed devexp.js", opencodeEntrySrc)
	}
}

// ── InstallOpencode ───────────────────────────────────────────────────────────

func TestInstallOpencode(t *testing.T) {
	allFiles := []string{
		"devexp.js",
		"devexp/graphify-read-guard.js",
		"devexp/hooks.json",
		"devexp/lint-on-save.js",
		"devexp/package.json",
		"devexp/secret-guard.js",
		"devexp/utils.js",
	}

	setup := func(t *testing.T) (repoDir, pluginsDir string) {
		repoDir = t.TempDir()
		writeOpencodeRepo(t, repoDir, "secret-guard", "lint-on-save", "graphify-read-guard", "claude-only")
		return repoDir, filepath.Join(t.TempDir(), "plugins")
	}

	t.Run("installs exactly the entry, utils, package.json, selection and selected modules", func(t *testing.T) {
		repoDir, pluginsDir := setup(t)
		var got []string
		var err error
		out := captureOutput(t, func() {
			got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, false)
		})
		if err != nil {
			t.Fatalf("InstallOpencode() error = %v", err)
		}
		if tree := listTree(t, pluginsDir); !reflect.DeepEqual(tree, allFiles) {
			t.Errorf("plugins tree = %v, want %v", tree, allFiles)
		}
		if len(got) != len(allFiles) || got[0] != "devexp.js" {
			t.Errorf("returned %v, want %d paths with devexp.js first", got, len(allFiles))
		}
		sorted := append([]string(nil), got...)
		sort.Strings(sorted)
		if !reflect.DeepEqual(sorted, allFiles) {
			t.Errorf("returned %v, want the installed files %v", got, allFiles)
		}
		entry, _ := os.ReadFile(filepath.Join(pluginsDir, "devexp.js"))
		src, _ := os.ReadFile(filepath.Join(repoDir, "hooks", "opencode", "devexp-plugin.js"))
		if string(entry) != string(src) {
			t.Errorf("devexp.js is not a copy of devexp-plugin.js")
		}
		plain := stripANSI(out)
		if !strings.Contains(plain, "opencode hooks (3): secret-guard, lint-on-save, graphify-read-guard") {
			t.Errorf("output does not list the installed hooks:\n%s", plain)
		}
		if !strings.Contains(plain, "+ devexp.js") || !strings.Contains(plain, "+ devexp/hooks.json") {
			t.Errorf("output does not report added files:\n%s", plain)
		}
		// The entry is written last so it never points at missing modules.
		if strings.Index(plain, "+ devexp.js") < strings.Index(plain, "+ devexp/hooks.json") {
			t.Errorf("devexp.js was written before hooks.json:\n%s", plain)
		}
	})

	t.Run("a disabled hook is neither copied nor selected", func(t *testing.T) {
		repoDir, pluginsDir := setup(t)
		out := captureOutput(t, func() {
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, []string{"lint-on-save"}, false); err != nil {
				t.Fatalf("InstallOpencode() error = %v", err)
			}
		})
		if _, err := os.Stat(filepath.Join(pluginsDir, "devexp", "lint-on-save.js")); !os.IsNotExist(err) {
			t.Errorf("devexp/lint-on-save.js installed while disabled")
		}
		selection, _ := os.ReadFile(filepath.Join(pluginsDir, "devexp", "hooks.json"))
		if strings.Contains(string(selection), "lint-on-save") {
			t.Errorf("hooks.json lists a disabled hook: %s", selection)
		}
		if !strings.Contains(stripANSI(out), "[skip] lint-on-save — disabled") {
			t.Errorf("output does not report the disabled hook:\n%s", out)
		}
	})

	t.Run("dry-run lists every file and writes nothing", func(t *testing.T) {
		repoDir, pluginsDir := setup(t)
		out := captureOutput(t, func() {
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, true); err != nil {
				t.Fatalf("InstallOpencode() error = %v", err)
			}
		})
		if _, err := os.Stat(pluginsDir); !os.IsNotExist(err) {
			t.Errorf("dry-run created %s", pluginsDir)
		}
		plain := stripANSI(out)
		for _, f := range allFiles {
			if !strings.Contains(plain, "[dry-run] write "+filepath.Join(pluginsDir, f)+"\n") {
				t.Errorf("dry-run output missing %s:\n%s", f, plain)
			}
		}
		if n := strings.Count(plain, "[dry-run] write "); n != len(allFiles) {
			t.Errorf("dry-run listed %d files, want %d", n, len(allFiles))
		}
	})

	t.Run("a second run changes nothing and prints no file lines", func(t *testing.T) {
		repoDir, pluginsDir := setup(t)
		captureOutput(t, func() { InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, false) }) //nolint:errcheck
		before := snapshot(t, pluginsDir)
		out := stripANSI(captureOutput(t, func() {
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, false); err != nil {
				t.Fatalf("InstallOpencode() error = %v", err)
			}
		}))
		if regexp.MustCompile(`(?m)^\s+[+~] `).MatchString(out) {
			t.Errorf("idempotent run reported changes:\n%s", out)
		}
		if after := snapshot(t, pluginsDir); !reflect.DeepEqual(before, after) {
			t.Errorf("idempotent run changed files")
		}
	})

	t.Run("a changed source module is updated", func(t *testing.T) {
		repoDir, pluginsDir := setup(t)
		captureOutput(t, func() { InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, false) })     //nolint:errcheck
		os.WriteFile(filepath.Join(repoDir, "hooks", "opencode", "secret-guard.js"), []byte("// v2\n"), 0644) //nolint:errcheck
		out := stripANSI(captureOutput(t, func() {
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, false); err != nil {
				t.Fatalf("InstallOpencode() error = %v", err)
			}
		}))
		if !strings.Contains(out, "~ devexp/secret-guard.js — updated") {
			t.Errorf("output does not report the update:\n%s", out)
		}
		if got, _ := os.ReadFile(filepath.Join(pluginsDir, "devexp", "secret-guard.js")); string(got) != "// v2\n" {
			t.Errorf("devexp/secret-guard.js = %q, want the new source", got)
		}
	})

	t.Run("every hook disabled installs nothing and says why", func(t *testing.T) {
		repoDir, pluginsDir := setup(t)
		var got []string
		var err error
		out := captureOutput(t, func() {
			got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, []string{"secret-guard", "lint-on-save", "graphify-read-guard"}, false)
		})
		if err != nil || got != nil {
			t.Errorf("InstallOpencode() = (%v, %v), want (nil, nil)", got, err)
		}
		if _, err := os.Stat(pluginsDir); !os.IsNotExist(err) {
			t.Errorf("plugins dir created with every hook disabled")
		}
		if !strings.Contains(out, "every hook is disabled") {
			t.Errorf("output does not say why nothing was installed:\n%s", out)
		}
	})

	t.Run("foreign files in plugins are untouched", func(t *testing.T) {
		repoDir, pluginsDir := setup(t)
		os.MkdirAll(pluginsDir, 0755)                                                             //nolint:errcheck
		os.WriteFile(filepath.Join(pluginsDir, "other.js"), []byte("export const x = 1\n"), 0644) //nolint:errcheck
		os.WriteFile(filepath.Join(pluginsDir, "package.json"), []byte(`{"main":"x.js"}`), 0644)  //nolint:errcheck
		captureOutput(t, func() {
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, false); err != nil {
				t.Fatalf("InstallOpencode() error = %v", err)
			}
		})
		if got, _ := os.ReadFile(filepath.Join(pluginsDir, "other.js")); string(got) != "export const x = 1\n" {
			t.Errorf("other.js = %q, changed", got)
		}
		if got, _ := os.ReadFile(filepath.Join(pluginsDir, "package.json")); string(got) != `{"main":"x.js"}` {
			t.Errorf("package.json = %q, changed", got)
		}
	})
}

func TestInstallOpencode_Refuses(t *testing.T) {
	tests := map[string]struct {
		prepare func(t *testing.T, pluginsDir, outside string)
		// check asserts the conflicting path survived as it was
		check func(t *testing.T, pluginsDir, outside string)
	}{
		"a devexp.js that is not a devexp entry": {
			prepare: func(t *testing.T, pluginsDir, _ string) {
				os.MkdirAll(pluginsDir, 0755)                                                                 //nolint:errcheck
				os.WriteFile(filepath.Join(pluginsDir, "devexp.js"), []byte("export const mine = 1\n"), 0644) //nolint:errcheck
			},
			check: func(t *testing.T, pluginsDir, _ string) {
				if got, _ := os.ReadFile(filepath.Join(pluginsDir, "devexp.js")); string(got) != "export const mine = 1\n" {
					t.Errorf("user devexp.js = %q, overwritten", got)
				}
			},
		},
		"a destination that is a symlink": {
			prepare: func(t *testing.T, pluginsDir, outside string) {
				os.MkdirAll(filepath.Join(pluginsDir, "devexp"), 0755) //nolint:errcheck
				os.WriteFile(outside, []byte("precious\n"), 0644)      //nolint:errcheck
				if err := os.Symlink(outside, filepath.Join(pluginsDir, "devexp", "utils.js")); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, _, outside string) {
				if got, _ := os.ReadFile(outside); string(got) != "precious\n" {
					t.Errorf("symlink target = %q, written through", got)
				}
			},
		},
		"devexp that is a file": {
			prepare: func(t *testing.T, pluginsDir, _ string) {
				os.MkdirAll(pluginsDir, 0755)                                             //nolint:errcheck
				os.WriteFile(filepath.Join(pluginsDir, "devexp"), []byte("mine\n"), 0644) //nolint:errcheck
			},
			check: func(t *testing.T, pluginsDir, _ string) {
				if got, _ := os.ReadFile(filepath.Join(pluginsDir, "devexp")); string(got) != "mine\n" {
					t.Errorf("devexp file = %q, changed", got)
				}
			},
		},
	}
	for name, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(name, func(t *testing.T) {
				repoDir := t.TempDir()
				writeOpencodeRepo(t, repoDir, "secret-guard", "lint-on-save", "graphify-read-guard")
				pluginsDir := filepath.Join(t.TempDir(), "plugins")
				outside := filepath.Join(t.TempDir(), "outside.txt")
				tt.prepare(t, pluginsDir, outside)
				var err error
				captureOutput(t, func() {
					_, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, dryRun)
				})
				if err == nil {
					t.Errorf("InstallOpencode(dryRun=%v) error = nil, want a refusal", dryRun)
				}
				if _, statErr := os.Stat(filepath.Join(pluginsDir, "devexp", "hooks.json")); statErr == nil {
					t.Errorf("hooks.json written despite the refusal")
				}
				tt.check(t, pluginsDir, outside)
			})
		}
	}
}

func TestInstallOpencode_MissingSourceWritesNothing(t *testing.T) {
	repoDir := t.TempDir()
	writeOpencodeRepo(t, repoDir, "secret-guard", "graphify-read-guard") // no lint-on-save.js
	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	var err error
	captureOutput(t, func() {
		_, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, false)
	})
	if err == nil {
		t.Fatalf("InstallOpencode() error = nil, want a missing-source error")
	}
	if _, statErr := os.Stat(pluginsDir); !os.IsNotExist(statErr) {
		t.Errorf("plugins dir written despite a missing source")
	}
}

// TestInstallOpencode_NodeLoadsInstalledPlugin installs the real hooks into a
// temp plugins/ and loads it with node the way opencode does — every
// top-level .js file, every export called — to prove the Go-written tree and
// hooks.json are what the entry accepts: one plugin, no load errors, and the
// .env read blocked with Claude Code's exact message.
func TestInstallOpencode_NodeLoadsInstalledPlugin(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	registry, err := LoadRegistry(filepath.Join(root, "hooks", "registry.json"))
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	tmp := t.TempDir()
	pluginsDir := filepath.Join(tmp, "plugins")
	captureOutput(t, func() {
		if _, err = InstallOpencode(registry, root, pluginsDir, nil, false); err != nil {
			t.Fatalf("InstallOpencode() error = %v", err)
		}
	})
	// Node needs an ESM marker above plugins/ for devexp.js (opencode's Bun
	// does not); the installer itself must never write plugins/package.json.
	os.WriteFile(filepath.Join(tmp, "package.json"), []byte(`{"type":"module"}`), 0644) //nolint:errcheck

	loader := filepath.Join(tmp, "load.mjs")
	os.WriteFile(loader, []byte(`
import { readdirSync } from 'fs';
import { pathToFileURL } from 'url';
import { join } from 'path';
const dir = process.argv[2];
const logged = [];
console.error = (...a) => logged.push(a.join(' '));
const plugins = [];
for (const f of readdirSync(dir).filter((n) => /\.(js|ts)$/.test(n))) {
  const mod = await import(pathToFileURL(join(dir, f)).href);
  for (const [name, v] of Object.entries(mod)) {
    if (typeof v !== 'function') throw new Error(f + ': export ' + name + ' is not a function');
    plugins.push(await v({ directory: dir }));
  }
}
const chains = plugins.filter((p) => p['tool.execute.before']);
const blocked = [];
for (const p of chains) {
  try { await p['tool.execute.before']({ tool: 'read' }, { args: { filePath: '/x/' + '.env' } }); }
  catch (e) { blocked.push(e.message); }
}
process.stdout.write(JSON.stringify({ plugins: plugins.length, chains: chains.length, blocked, logged }));
`), 0644) //nolint:errcheck

	out, err := exec.Command(node, loader, pluginsDir).CombinedOutput()
	if err != nil {
		t.Fatalf("node loader failed: %v\n%s", err, out)
	}
	var res struct {
		Plugins, Chains int
		Blocked, Logged []string
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("loader output %q: %v", out, err)
	}
	want := `[devexp secret-guard] Blocked access to ".env". This file may contain secrets. If intentional, confirm with the user first.`
	if res.Plugins != 1 || res.Chains != 1 {
		t.Errorf("loaded %d plugins with %d guard chains, want exactly 1 and 1", res.Plugins, res.Chains)
	}
	if len(res.Blocked) != 1 || res.Blocked[0] != want {
		t.Errorf("blocked = %q, want exactly [%q]", res.Blocked, want)
	}
	if len(res.Logged) != 0 {
		t.Errorf("load logged errors: %q", res.Logged)
	}
}

// ── Stale files and the devexp/ directory ─────────────────────────────────────

func TestOwnedStalePlugins(t *testing.T) {
	entry := "/**\n * devexp-plugin.js — entry point for devexp opencode hooks\n */\n"
	tests := map[string]struct {
		entryContent *string // nil = no devexp.js on disk
		stale        []string
		want         []string
	}{
		"devexp files are kept for removal": {
			stale: []string{"devexp.js", "devexp/lint-on-save.js", "devexp/hooks.json", "devexp/package.json", "devexp/utils.js"},
			want:  []string{"devexp.js", "devexp/lint-on-save.js", "devexp/hooks.json", "devexp/package.json", "devexp/utils.js"},
		},
		"a devexp.js that is still a devexp entry may be removed": {
			entryContent: &entry,
			stale:        []string{"devexp.js"},
			want:         []string{"devexp.js"},
		},
		"a devexp.js replaced by the user is never removed": {
			entryContent: func() *string { s := "export const mine = 1\n"; return &s }(),
			stale:        []string{"devexp.js"},
			want:         nil,
		},
		"paths devexp never installs are never removed": {
			stale: []string{"../config.json", "devexp/../other.js", "other.js", "devexp/sub/x.js", "devexp/", "devexp/.hidden.js", "devexp", "package.json", "/etc/passwd"},
			want:  nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			pluginsDir := t.TempDir()
			if tt.entryContent != nil {
				os.WriteFile(filepath.Join(pluginsDir, "devexp.js"), []byte(*tt.entryContent), 0644) //nolint:errcheck
			}
			var got []string
			out := captureOutput(t, func() { got = OwnedStalePlugins(pluginsDir, tt.stale) })
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OwnedStalePlugins() = %v, want %v", got, tt.want)
			}
			if refused := len(tt.stale) - len(tt.want); strings.Count(out, "left untouched") != refused {
				t.Errorf("warned %d times, want %d:\n%s", strings.Count(out, "left untouched"), refused, out)
			}
		})
	}
}

func TestPruneOpencodeDir(t *testing.T) {
	tests := map[string]struct {
		file     bool
		dryRun   bool
		wantKept bool
	}{
		"empty directory is removed":       {wantKept: false},
		"non-empty directory is kept":      {file: true, wantKept: true},
		"dry-run keeps an empty directory": {dryRun: true, wantKept: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			pluginsDir := t.TempDir()
			dir := filepath.Join(pluginsDir, "devexp")
			os.MkdirAll(dir, 0755) //nolint:errcheck
			if tt.file {
				os.WriteFile(filepath.Join(dir, "mine.txt"), []byte("x"), 0644) //nolint:errcheck
			}
			PruneOpencodeDir(pluginsDir, tt.dryRun)
			_, err := os.Stat(dir)
			if kept := err == nil; kept != tt.wantKept {
				t.Errorf("devexp/ kept = %v, want %v", kept, tt.wantKept)
			}
		})
	}
}

// ── Legacy flat install ───────────────────────────────────────────────────────

const legacyFixtures = "testdata/legacy-opencode"

// copyLegacyFixtures copies the real pre-46ca772 files into dir.
func copyLegacyFixtures(t *testing.T, dir string, only ...string) {
	t.Helper()
	entries, err := os.ReadDir(legacyFixtures)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", legacyFixtures, err)
	}
	for _, e := range entries {
		if len(only) > 0 && !contains(only, e.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(legacyFixtures, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TestLegacyFixtures pins the fixtures to the closed legacy set: every legacy
// name has a fixture carrying the signature, and nothing else is in the set.
func TestLegacyFixtures(t *testing.T) {
	entries, err := os.ReadDir(legacyFixtures)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	var js []string
	for _, e := range entries {
		if e.Name() == "package.json" {
			data, _ := os.ReadFile(filepath.Join(legacyFixtures, e.Name()))
			if strings.TrimSpace(string(data)) != legacyPackageJSON {
				t.Errorf("legacy package.json fixture = %q, want %q", data, legacyPackageJSON)
			}
			continue
		}
		js = append(js, e.Name())
		data, _ := os.ReadFile(filepath.Join(legacyFixtures, e.Name()))
		if !isLegacyDevexpFile(e.Name(), data) {
			t.Errorf("fixture %s does not match the legacy signature", e.Name())
		}
	}
	want := append([]string(nil), legacyNames...)
	sort.Strings(want)
	sort.Strings(js)
	if !reflect.DeepEqual(js, want) {
		t.Errorf("fixture JS files = %v, legacy set = %v", js, want)
	}
}

func TestIsLegacyDevexpFile(t *testing.T) {
	tests := map[string]struct {
		name    string
		content string
		want    bool
	}{
		"legacy name with its header":               {"utils.js", "/**\n * utils.js — shared utilities for devexp opencode hook modules\n */\n", true},
		"CRLF line endings":                         {"utils.js", "/**\r\n * utils.js — shared utilities\r\n */\r\n", true},
		"legacy name, user content":                 {"utils.js", "export const x = 1\n", false},
		"legacy name, another file's header":        {"utils.js", "/**\n * secret-guard.js — blocks\n */\n", false},
		"header without the opening comment":        {"utils.js", "// x\n * utils.js — shared\n", false},
		"header without the em dash":                {"utils.js", "/**\n * utils.js - shared\n */\n", false},
		"name outside the legacy set with a header": {"my-plugin.js", "/**\n * my-plugin.js — mine\n */\n", false},
		"the new entry name is not legacy":          {"devexp.js", "/**\n * devexp.js — x\n */\n", false},
		"empty file":                                {"utils.js", "", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isLegacyDevexpFile(tt.name, []byte(tt.content)); got != tt.want {
				t.Errorf("isLegacyDevexpFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestCleanLegacyOpencode_Files(t *testing.T) {
	tests := map[string]struct {
		prepare   func(t *testing.T, dir string)
		dryRun    bool
		wantLeft  []string
		wantWarns int
	}{
		"legacy files and package.json are removed, foreign files kept": {
			prepare: func(t *testing.T, dir string) {
				copyLegacyFixtures(t, dir)
				os.WriteFile(filepath.Join(dir, "my-plugin.js"), []byte("export const mine = async () => ({})\n"), 0644) //nolint:errcheck
			},
			wantLeft: []string{"my-plugin.js"},
		},
		"a user utils.js without the devexp header is kept with a warning": {
			prepare: func(t *testing.T, dir string) {
				copyLegacyFixtures(t, dir, "secret-guard.js")
				os.WriteFile(filepath.Join(dir, "utils.js"), []byte("/**\n * my helpers\n */\nexport const x = 1\n"), 0644) //nolint:errcheck
			},
			wantLeft:  []string{"utils.js"},
			wantWarns: 1,
		},
		"a legacy name that is a directory is kept with a warning": {
			prepare: func(t *testing.T, dir string) {
				os.MkdirAll(filepath.Join(dir, "lint-on-save.js"), 0755) //nolint:errcheck
			},
			wantLeft:  []string{"lint-on-save.js/"},
			wantWarns: 1,
		},
		"legacy package.json without any legacy match is kept": {
			prepare: func(t *testing.T, dir string) {
				copyLegacyFixtures(t, dir, "package.json")
			},
			wantLeft: []string{"package.json"},
		},
		"a different package.json is kept even with legacy matches": {
			prepare: func(t *testing.T, dir string) {
				copyLegacyFixtures(t, dir, "utils.js")
				os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{ "type": "module", "main": "x.js" }`), 0644) //nolint:errcheck
			},
			wantLeft: []string{"package.json"},
		},
		"a legacy-named symlink is kept": {
			prepare: func(t *testing.T, dir string) {
				target := filepath.Join(t.TempDir(), "utils.js")
				copyLegacyFixtures(t, filepath.Dir(target), "utils.js")
				os.Symlink(target, filepath.Join(dir, "utils.js")) //nolint:errcheck
			},
			wantLeft:  []string{"utils.js"},
			wantWarns: 1,
		},
		"dry-run removes nothing": {
			prepare: func(t *testing.T, dir string) {
				copyLegacyFixtures(t, dir)
			},
			dryRun: true,
			wantLeft: func() []string {
				out := append([]string{"package.json"}, legacyNames...)
				sort.Strings(out)
				return out
			}(),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			tt.prepare(t, dir)
			var err error
			out := captureOutput(t, func() {
				err = CleanLegacyOpencode(dir, filepath.Join(t.TempDir(), "config.json"), tt.dryRun)
			})
			if err != nil {
				t.Fatalf("CleanLegacyOpencode() error = %v", err)
			}
			var left []string
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				n := e.Name()
				if e.IsDir() {
					n += "/"
				}
				left = append(left, n)
			}
			sort.Strings(left)
			if !reflect.DeepEqual(left, tt.wantLeft) {
				t.Errorf("left in plugins = %v, want %v", left, tt.wantLeft)
			}
			if got := strings.Count(out, "left untouched"); got != tt.wantWarns {
				t.Errorf("warnings = %d, want %d:\n%s", got, tt.wantWarns, out)
			}
		})
	}
}

func TestCleanLegacyOpencode_Config(t *testing.T) {
	const L = "@LEGACY@" // replaced with <pluginsDir>/devexp-plugin.js
	tests := map[string]struct {
		config string
		want   string // "" = byte-identical to config
		dryRun bool
	}{
		"exact entry removed, other entries and keys kept": {
			config: `{"theme":"x","plugin":["@LEGACY@","npm-plugin"]}`,
			want:   `{"theme":"x","plugin":["npm-plugin"]}`,
		},
		"indented config keeps every other byte": {
			config: "{\n  \"theme\": \"x\",\n  \"plugin\": [\n    \"a\",\n    \"@LEGACY@\",\n    \"b\"\n  ],\n  \"z\": 1.50\n}\n",
			want:   "{\n  \"theme\": \"x\",\n  \"plugin\": [\n    \"a\",\n    \"b\"\n  ],\n  \"z\": 1.50\n}\n",
		},
		"only the legacy entry removes the plugin key": {
			config: "{\n  \"theme\": \"x\",\n  \"plugin\": [\"@LEGACY@\"]\n}\n",
			want:   "{\n  \"theme\": \"x\"\n}\n",
		},
		"plugin as the first key": {
			config: `{"plugin":["@LEGACY@"],"theme":"x"}`,
			want:   `{"theme":"x"}`,
		},
		"plugin as the only key": {
			config: "{ \"plugin\": [ \"@LEGACY@\" ] }\n",
			want:   "{}\n",
		},
		"legacy entry first in the array": {
			config: `{"plugin":["@LEGACY@", "npm-plugin"]}`,
			want:   `{"plugin":["npm-plugin"]}`,
		},
		"duplicate legacy entries are all removed": {
			config: `{"plugin":["@LEGACY@","a","@LEGACY@"]}`,
			want:   `{"plugin":["a"]}`,
		},
		"non-string elements and escapes are preserved": {
			config: `{"a":"<b>\u00e9&","plugin":[["tuple",{"k":1}],"@LEGACY@",7]}`,
			want:   `{"a":"<b>\u00e9&","plugin":[["tuple",{"k":1}],7]}`,
		},
		"near-miss .bak path is kept":        {config: `{"plugin":["@LEGACY@.bak"]}`},
		"file:// form is kept":               {config: `{"plugin":["file://@LEGACY@"]}`},
		"a different directory is kept":      {config: `{"plugin":["/other/plugins/devexp-plugin.js"]}`},
		"no plugin key is untouched":         {config: `{"theme":"x"}`},
		"plugin that is not an array":        {config: `{"plugin":"@LEGACY@"}`},
		"legacy string elsewhere is ignored": {config: `{"other":["@LEGACY@"]}`},
		"malformed config is untouched":      {config: `{"plugin":["@LEGACY@",}`},
		"JSONC comments are untouched":       {config: "{\n // c\n \"plugin\": [\"@LEGACY@\"]\n}"},
		"dry-run changes nothing":            {config: `{"plugin":["@LEGACY@","a"]}`, dryRun: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			pluginsDir := t.TempDir()
			legacy := filepath.Join(pluginsDir, legacyEntry)
			configPath := filepath.Join(t.TempDir(), "config.json")
			config := strings.ReplaceAll(tt.config, L, legacy)
			if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
				t.Fatalf("WriteFile error = %v", err)
			}
			want := config
			if tt.want != "" {
				want = strings.ReplaceAll(tt.want, L, legacy)
			}
			var err error
			out := captureOutput(t, func() { err = CleanLegacyOpencode(pluginsDir, configPath, tt.dryRun) })
			if err != nil {
				t.Fatalf("CleanLegacyOpencode() error = %v", err)
			}
			got, _ := os.ReadFile(configPath)
			if string(got) != want {
				t.Errorf("config.json =\n%s\nwant\n%s", got, want)
			}
			if fi, _ := os.Stat(configPath); fi.Mode().Perm() != 0600 {
				t.Errorf("config.json mode = %v, want 0600 preserved", fi.Mode().Perm())
			}
			// dry-run reports the removal it would make
			if report := tt.want != "" || tt.dryRun; report != strings.Contains(out, "plugin entry") {
				t.Errorf("output reports removal = %v, want %v:\n%s", !report, report, out)
			}
		})
	}

	t.Run("a missing config is not created", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "config.json")
		if err := CleanLegacyOpencode(t.TempDir(), configPath, false); err != nil {
			t.Fatalf("CleanLegacyOpencode() error = %v", err)
		}
		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Errorf("config.json created")
		}
	})
}
