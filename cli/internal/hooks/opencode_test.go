package hooks

import (
	"encoding/json"
	"fmt"
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
			got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, false)
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
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, []string{"lint-on-save"}, nil, false); err != nil {
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
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, true); err != nil {
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
		captureOutput(t, func() { InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, false) }) //nolint:errcheck
		before := snapshot(t, pluginsDir)
		out := stripANSI(captureOutput(t, func() {
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, false); err != nil {
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
		captureOutput(t, func() { InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, false) }) //nolint:errcheck
		os.WriteFile(filepath.Join(repoDir, "hooks", "opencode", "secret-guard.js"), []byte("// v2\n"), 0644)  //nolint:errcheck
		out := stripANSI(captureOutput(t, func() {
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, false); err != nil {
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
			got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, []string{"secret-guard", "lint-on-save", "graphify-read-guard"}, nil, false)
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
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, false); err != nil {
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
					_, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, dryRun)
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
		_, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, false)
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
		if _, err = InstallOpencode(registry, root, pluginsDir, nil, nil, false); err != nil {
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

// TestWholePluginRemoval_EditedEntryStaysLoadable: the manifest is lost and an
// editor put `// @ts-check` above devexp.js's header. Neither install with
// every hook disabled nor uninstall may remove devexp/ from under that entry:
// opencode would then block every tool call ("devexp/hooks.json unreadable").
// Both keep everything, say why, and the plugin still loads and lets a
// harmless read through.
func TestWholePluginRemoval_EditedEntryStaysLoadable(t *testing.T) {
	node, nodeErr := exec.LookPath("node")
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	registry, err := LoadRegistry(filepath.Join(root, "hooks", "registry.json"))
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	var every []string
	for _, h := range registry {
		every = append(every, h.Name)
	}
	runs := map[string]func(pluginsDir string) ([]string, error){
		"install with every hook disabled": func(pluginsDir string) ([]string, error) {
			return InstallOpencode(registry, root, pluginsDir, every, nil, false)
		},
		"uninstall": func(pluginsDir string) ([]string, error) {
			return UninstallOpencode(registry, pluginsDir, nil, false)
		},
	}
	for name, run := range runs {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			pluginsDir := filepath.Join(tmp, "plugins")
			var installed []string
			captureOutput(t, func() { installed, err = InstallOpencode(registry, root, pluginsDir, nil, nil, false) })
			if err != nil {
				t.Fatalf("InstallOpencode() error = %v", err)
			}
			entry := filepath.Join(pluginsDir, "devexp.js")
			content, _ := os.ReadFile(entry)
			os.WriteFile(entry, append([]byte("// @ts-check\n"), content...), 0644) //nolint:errcheck
			before := treeState(t, tmp)

			var kept []string
			out := stripANSI(captureOutput(t, func() { kept, err = run(pluginsDir) }))
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if after := treeState(t, tmp); !reflect.DeepEqual(before, after) {
				t.Errorf("plugin changed: %v -> %v", stateKeys(before), stateKeys(after))
			}
			if !reflect.DeepEqual(kept, installed[1:]) {
				t.Errorf("kept %v, want every devexp/ file %v", kept, installed[1:])
			}
			if !strings.Contains(out, "devexp.js and devexp/ are both kept") || !strings.Contains(out, "not recognised as devexp's") {
				t.Errorf("no warning explaining why the plugin was kept:\n%s", out)
			}

			if nodeErr != nil {
				t.Skip("node not on PATH")
			}
			os.WriteFile(filepath.Join(tmp, "package.json"), []byte(`{"type":"module"}`), 0644) //nolint:errcheck
			probe := filepath.Join(tmp, "probe.mjs")
			os.WriteFile(probe, []byte(`
import { pathToFileURL } from 'url';
console.error = () => {};
const mod = await import(pathToFileURL(process.argv[2]).href);
const hooks = await mod.DevExpPlugin({ directory: process.argv[3], worktree: process.argv[3] });
try { await hooks['tool.execute.before']({ tool: 'read' }, { args: { filePath: '/tmp/notes.txt' } }); console.log('allowed'); }
catch (e) { console.log('blocked: ' + e.message); }
`), 0644) //nolint:errcheck
			res, err := exec.Command(node, probe, entry, tmp).CombinedOutput()
			if err != nil || strings.TrimSpace(string(res)) != "allowed" {
				t.Errorf("node probe (err %v) = %q, want the harmless read allowed", err, res)
			}
		})
	}
}

// ── Stale files and the devexp/ directory ─────────────────────────────────────

const devexpEntryHeader = "/**\n * devexp-plugin.js — entry point for devexp opencode hooks\n */\n"

func TestStalePlugins(t *testing.T) {
	tests := map[string]struct {
		onDisk   map[string]string // relative path -> content
		recorded []string
		keep     []string
		want     []string
		warns    int
	}{
		"recorded devexp files not kept are stale, entry first": {
			recorded: []string{"devexp/lint-on-save.js", "devexp.js", "devexp/hooks.json"},
			keep:     []string{"devexp/hooks.json"},
			want:     []string{"devexp.js", "devexp/lint-on-save.js"},
		},
		"paths devexp never installs are reported and never stale": {
			recorded: []string{"../config.json", "devexp/../other.js", "other.js", "devexp/sub/x.js", "devexp/", "devexp/.hidden.js", "devexp", "package.json", "/etc/passwd"},
			want:     nil,
			warns:    9,
		},
		"without a manifest, devexp files on disk are found": {
			onDisk: map[string]string{
				"devexp.js":              devexpEntryHeader,
				"devexp/hooks.json":      "[]",
				"devexp/utils.js":        "x",
				"devexp/package.json":    "{}",
				"devexp/secret-guard.js": "x",
				"devexp/mine.txt":        "user file",
				"devexp/unknown.js":      "not a registry module",
				"my-plugin.js":           "user plugin",
			},
			want: []string{"devexp.js", "devexp/utils.js", "devexp/package.json", "devexp/secret-guard.js", "devexp/hooks.json"},
		},
		"a devexp.js without the entry header is not found on disk": {
			onDisk: map[string]string{"devexp.js": "export const mine = 1\n"},
			want:   nil,
		},
		"recorded and on-disk paths are listed once": {
			onDisk:   map[string]string{"devexp/utils.js": "x"},
			recorded: []string{"devexp/utils.js"},
			want:     []string{"devexp/utils.js"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			pluginsDir := t.TempDir()
			for rel, content := range tt.onDisk {
				p := filepath.Join(pluginsDir, filepath.FromSlash(rel))
				os.MkdirAll(filepath.Dir(p), 0755)     //nolint:errcheck
				os.WriteFile(p, []byte(content), 0644) //nolint:errcheck
			}
			var got []string
			out := captureOutput(t, func() { got = stalePlugins(pluginsDir, opencodeRegistry(), tt.recorded, tt.keep) })
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stalePlugins() = %v, want %v", got, tt.want)
			}
			if n := strings.Count(out, "left untouched"); n != tt.warns {
				t.Errorf("warned %d times, want %d:\n%s", n, tt.warns, out)
			}
		})
	}
}

func TestOwnedOnDisk_SymlinkedDevexpDir(t *testing.T) {
	pluginsDir := t.TempDir()
	target := t.TempDir()
	os.WriteFile(filepath.Join(target, "utils.js"), []byte("x"), 0644) //nolint:errcheck
	if err := os.Symlink(target, filepath.Join(pluginsDir, "devexp")); err != nil {
		t.Fatal(err)
	}
	if got := ownedOnDisk(pluginsDir, opencodeRegistry()); got != nil {
		t.Errorf("ownedOnDisk() = %v, want nothing inside a symlinked devexp/", got)
	}
}

func TestPruneOpencodeDir(t *testing.T) {
	tests := map[string]struct {
		file        bool
		symlink     bool
		pluginsLink bool
		dryRun      bool
		wantKept    bool
	}{
		"empty directory is removed":                         {wantKept: false},
		"non-empty directory is kept":                        {file: true, wantKept: true},
		"dry-run keeps an empty directory":                   {dryRun: true, wantKept: true},
		"a symlink is never removed":                         {symlink: true, wantKept: true},
		"nothing is removed through a symlinked plugins dir": {pluginsLink: true, wantKept: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			pluginsDir := t.TempDir()
			dir := filepath.Join(pluginsDir, "devexp")
			if tt.symlink {
				os.Symlink(t.TempDir(), dir) //nolint:errcheck
			} else {
				os.MkdirAll(dir, 0755) //nolint:errcheck
			}
			if tt.file {
				os.WriteFile(filepath.Join(dir, "mine.txt"), []byte("x"), 0644) //nolint:errcheck
			}
			if tt.pluginsLink {
				real := pluginsDir
				pluginsDir = filepath.Join(t.TempDir(), "plugins")
				os.Symlink(real, pluginsDir) //nolint:errcheck
			}
			pruneOpencodeDir(pluginsDir, tt.dryRun)
			_, err := os.Lstat(dir)
			if kept := err == nil; kept != tt.wantKept {
				t.Errorf("devexp/ kept = %v, want %v", kept, tt.wantKept)
			}
		})
	}
}

// installFull installs the three-hook test plugin and returns what the
// manifest would record.
func installFull(t *testing.T, repoDir, pluginsDir string) []string {
	t.Helper()
	var got []string
	var err error
	captureOutput(t, func() { got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, false) })
	if err != nil {
		t.Fatalf("InstallOpencode() error = %v", err)
	}
	return got
}

var allThree = []string{"secret-guard", "lint-on-save", "graphify-read-guard"}

// linkDir moves dir to a fresh location and leaves a symlink to it in its
// place, as a dotfiles setup or a maintainer live-linking a checkout would.
// It returns the link target.
func linkDir(t *testing.T, dir string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.Rename(dir, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dir); err != nil {
		t.Fatal(err)
	}
	return target
}

func dryRunName(dryRun bool) string { return map[bool]string{true: "dry-run", false: "real"}[dryRun] }

// TestInstallOpencode_SymlinkedDevexpDir: a symlinked devexp/ may point at a
// source checkout, so it is refused in every case — hooks enabled or all
// disabled, dry-run or not — and nothing behind it changes.
func TestInstallOpencode_SymlinkedDevexpDir(t *testing.T) {
	for _, disabled := range [][]string{nil, allThree} {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("disabled=%d %s", len(disabled), dryRunName(dryRun)), func(t *testing.T) {
				repoDir := t.TempDir()
				writeOpencodeRepo(t, repoDir, allThree...)
				pluginsDir := filepath.Join(t.TempDir(), "plugins")
				recorded := installFull(t, repoDir, pluginsDir)
				checkout := linkDir(t, filepath.Join(pluginsDir, "devexp"))
				before := snapshot(t, checkout)

				var err error
				captureOutput(t, func() {
					_, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, disabled, recorded, dryRun)
				})
				if err == nil || !strings.Contains(err.Error(), "is a symlink") {
					t.Errorf("InstallOpencode() error = %v, want a symlink refusal", err)
				}
				if after := snapshot(t, checkout); !reflect.DeepEqual(before, after) {
					t.Errorf("files behind the devexp symlink changed")
				}
				if fi, err := os.Lstat(filepath.Join(pluginsDir, "devexp")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
					t.Errorf("the devexp symlink itself was removed or replaced")
				}
			})
		}
	}
}

// TestInstallOpencode_SymlinkedPluginsDir: devexp writes through a symlinked
// plugins/ but never removes anything through it. Files it would have removed
// are listed in a warning and stay recorded.
func TestInstallOpencode_SymlinkedPluginsDir(t *testing.T) {
	setup := func(t *testing.T) (repoDir, pluginsDir, target string, recorded []string) {
		repoDir = t.TempDir()
		writeOpencodeRepo(t, repoDir, allThree...)
		pluginsDir = filepath.Join(t.TempDir(), "plugins")
		recorded = installFull(t, repoDir, pluginsDir)
		target = linkDir(t, pluginsDir)
		// A module from an earlier release, still recorded: stale on this run.
		os.WriteFile(filepath.Join(target, "devexp", "old-hook.js"), []byte("old"), 0644) //nolint:errcheck
		return repoDir, pluginsDir, target, append(recorded, "devexp/old-hook.js")
	}
	linkKept := func(t *testing.T, pluginsDir string) {
		t.Helper()
		if fi, err := os.Lstat(pluginsDir); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("the plugins symlink itself was removed or replaced")
		}
	}

	t.Run("hooks enabled: installs inside the link target and removes nothing", func(t *testing.T) {
		repoDir, pluginsDir, target, recorded := setup(t)
		os.WriteFile(filepath.Join(repoDir, "hooks", "opencode", "secret-guard.js"), []byte("// v2\n"), 0644) //nolint:errcheck
		var got []string
		var err error
		out := captureOutput(t, func() { got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, recorded, false) })
		if err != nil {
			t.Fatalf("InstallOpencode() error = %v", err)
		}
		if b, _ := os.ReadFile(filepath.Join(target, "devexp", "secret-guard.js")); string(b) != "// v2\n" {
			t.Errorf("devexp/secret-guard.js in the link target = %q, want the updated module", b)
		}
		if _, err := os.Stat(filepath.Join(target, "devexp", "old-hook.js")); err != nil {
			t.Errorf("stale devexp/old-hook.js removed through the symlink")
		}
		if !contains(got, "devexp/old-hook.js") || !contains(got, "devexp.js") {
			t.Errorf("InstallOpencode() = %v, want the written files plus the kept stale file", got)
		}
		if !strings.Contains(out, "never removes files through it") || !strings.Contains(out, "old-hook.js") {
			t.Errorf("no warning listing the file left behind:\n%s", out)
		}
		for _, e := range listTree(t, target) {
			if strings.Contains(e, ".tmp-") {
				t.Errorf("temp file %s left in the link target", e)
			}
		}
		linkKept(t, pluginsDir)
	})

	t.Run("hooks enabled, dry-run: writes nothing", func(t *testing.T) {
		repoDir, pluginsDir, target, recorded := setup(t)
		before := snapshot(t, target)
		captureOutput(t, func() {
			if _, err := InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, recorded, true); err != nil {
				t.Fatalf("InstallOpencode() error = %v", err)
			}
		})
		if after := snapshot(t, target); !reflect.DeepEqual(before, after) {
			t.Errorf("dry-run changed the link target")
		}
	})

	for _, dryRun := range []bool{false, true} {
		t.Run("every hook disabled: nothing removed, "+dryRunName(dryRun), func(t *testing.T) {
			repoDir, pluginsDir, target, recorded := setup(t)
			before := snapshot(t, target)
			var got []string
			var err error
			out := captureOutput(t, func() {
				got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, allThree, recorded, dryRun)
			})
			if err != nil {
				t.Fatalf("InstallOpencode() error = %v", err)
			}
			if after := snapshot(t, target); !reflect.DeepEqual(before, after) {
				t.Errorf("files removed through the plugins symlink")
			}
			if len(got) != len(recorded) || got[0] != "devexp.js" {
				t.Errorf("InstallOpencode() = %v, want every file left behind still recorded", got)
			}
			if !strings.Contains(out, "never removes files through it; remove these by hand") || !strings.Contains(out, "devexp.js") {
				t.Errorf("no warning listing what was left behind:\n%s", out)
			}
			linkKept(t, pluginsDir)
		})
	}

	t.Run("a dangling or non-directory link is refused", func(t *testing.T) {
		for name, target := range map[string]func(t *testing.T) string{
			"dangling": func(t *testing.T) string { return filepath.Join(t.TempDir(), "gone") },
			"to a file": func(t *testing.T) string {
				f := filepath.Join(t.TempDir(), "file")
				os.WriteFile(f, []byte("x"), 0644) //nolint:errcheck
				return f
			},
		} {
			t.Run(name, func(t *testing.T) {
				repoDir := t.TempDir()
				writeOpencodeRepo(t, repoDir, allThree...)
				pluginsDir := filepath.Join(t.TempDir(), "plugins")
				tgt := target(t)
				os.Symlink(tgt, pluginsDir) //nolint:errcheck
				var err error
				captureOutput(t, func() { _, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, nil, false) })
				if err == nil || !strings.Contains(err.Error(), "doesn't point at a directory") {
					t.Errorf("InstallOpencode() error = %v, want a refusal", err)
				}
			})
		}
	})
}

// TestInstallOpencode_EntryOwnership: a devexp.js the manifest records is
// repaired even when damaged; one it doesn't record and that isn't a devexp
// entry is never replaced.
func TestInstallOpencode_EntryOwnership(t *testing.T) {
	tests := map[string]struct {
		recorded bool
		wantErr  bool
	}{
		"a truncated devexp.js recorded in the manifest is repaired":   {recorded: true},
		"a truncated devexp.js the manifest doesn't record is refused": {recorded: false, wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			repoDir := t.TempDir()
			writeOpencodeRepo(t, repoDir, allThree...)
			pluginsDir := filepath.Join(t.TempDir(), "plugins")
			recorded := installFull(t, repoDir, pluginsDir)
			entry := filepath.Join(pluginsDir, "devexp.js")
			os.WriteFile(entry, []byte("/*"), 0644) //nolint:errcheck — what an interrupted write used to leave
			if !tt.recorded {
				recorded = nil
			}
			var err error
			captureOutput(t, func() {
				_, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, nil, recorded, false)
			})
			got, _ := os.ReadFile(entry)
			if tt.wantErr {
				if err == nil || string(got) != "/*" {
					t.Errorf("InstallOpencode() error = %v, devexp.js = %q; want a refusal and the file kept", err, got)
				}
				return
			}
			src, _ := os.ReadFile(filepath.Join(repoDir, "hooks", "opencode", "devexp-plugin.js"))
			if err != nil || string(got) != string(src) {
				t.Errorf("InstallOpencode() error = %v, devexp.js = %q; want it repaired", err, got)
			}
		})
	}
}

// TestInstallOpencode_LostManifest: with no recorded list, the plugin files
// devexp recognises on disk are still removed when every hook is disabled, and
// a normal install reports the full list again.
func TestInstallOpencode_LostManifest(t *testing.T) {
	t.Run("every hook disabled removes the plugin", func(t *testing.T) {
		repoDir := t.TempDir()
		writeOpencodeRepo(t, repoDir, allThree...)
		pluginsDir := filepath.Join(t.TempDir(), "plugins")
		installFull(t, repoDir, pluginsDir)
		os.WriteFile(filepath.Join(pluginsDir, "my-plugin.js"), []byte("mine"), 0644) //nolint:errcheck

		var got []string
		var err error
		captureOutput(t, func() { got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, allThree, nil, false) })
		if err != nil || got != nil {
			t.Errorf("InstallOpencode() = (%v, %v), want (nil, nil)", got, err)
		}
		if tree := listTree(t, pluginsDir); !reflect.DeepEqual(tree, []string{"my-plugin.js"}) {
			t.Errorf("plugins tree = %v, want only my-plugin.js", tree)
		}
	})

	t.Run("a normal install returns the full list", func(t *testing.T) {
		repoDir := t.TempDir()
		writeOpencodeRepo(t, repoDir, allThree...)
		pluginsDir := filepath.Join(t.TempDir(), "plugins")
		want := installFull(t, repoDir, pluginsDir)
		if got := installFull(t, repoDir, pluginsDir); !reflect.DeepEqual(got, want) || len(got) != 7 {
			t.Errorf("InstallOpencode() = %v, want %v", got, want)
		}
	})
}

// TestInstallOpencode_KeptEntryKeepsPlugin: when devexp.js must stay (here a
// symlink into dotfiles), devexp/ stays with it and the files remain recorded,
// because an entry without hooks.json blocks every opencode tool call.
func TestInstallOpencode_KeptEntryKeepsPlugin(t *testing.T) {
	repoDir := t.TempDir()
	writeOpencodeRepo(t, repoDir, allThree...)
	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	recorded := installFull(t, repoDir, pluginsDir)
	dotfiles := filepath.Join(t.TempDir(), "devexp.js")
	entry := filepath.Join(pluginsDir, "devexp.js")
	os.Rename(entry, dotfiles)  //nolint:errcheck
	os.Symlink(dotfiles, entry) //nolint:errcheck
	before := snapshot(t, filepath.Join(pluginsDir, "devexp"))

	var got []string
	var err error
	out := captureOutput(t, func() { got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, allThree, recorded, false) })
	if err != nil {
		t.Fatalf("InstallOpencode() error = %v", err)
	}
	if after := snapshot(t, filepath.Join(pluginsDir, "devexp")); !reflect.DeepEqual(before, after) {
		t.Errorf("devexp/ changed while devexp.js was kept")
	}
	if fi, err := os.Lstat(entry); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the devexp.js symlink was removed")
	}
	if len(got) != len(recorded) || got[0] != "devexp.js" {
		t.Errorf("InstallOpencode() = %v, want the kept files %v recorded", got, recorded)
	}
	if !strings.Contains(out, "hooks remain active") || !strings.Contains(out, "a symlink") {
		t.Errorf("output does not explain that the plugin stays active and why:\n%s", out)
	}
}

// ── Legacy flat install ───────────────────────────────────────────────────────

const legacyFixtures = "testdata/legacy-opencode"

// copyLegacyFixtures copies the real legacy files into dir.
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

	t.Run("a read-only config is left untouched in a writable directory", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write a read-only file, so access(2) can't observe the mode")
		}
		for _, dryRun := range []bool{false, true} {
			pluginsDir := t.TempDir()
			configPath := filepath.Join(t.TempDir(), "config.json")
			content := `{"plugin":["` + filepath.Join(pluginsDir, legacyEntry) + `","a"]}`
			if err := os.WriteFile(configPath, []byte(content), 0444); err != nil {
				t.Fatalf("WriteFile error = %v", err)
			}
			var err error
			out := captureOutput(t, func() { err = CleanLegacyOpencode(pluginsDir, configPath, dryRun) })
			if err != nil {
				t.Fatalf("dryRun=%v: CleanLegacyOpencode() error = %v", dryRun, err)
			}
			if got, _ := os.ReadFile(configPath); string(got) != content {
				t.Errorf("dryRun=%v: config.json =\n%s\nwant untouched", dryRun, got)
			}
			if fi, _ := os.Stat(configPath); fi.Mode().Perm() != 0444 {
				t.Errorf("dryRun=%v: config.json mode = %v, want 0444 kept", dryRun, fi.Mode().Perm())
			}
			if !strings.Contains(out, "is not writable, so it was left untouched") {
				t.Errorf("dryRun=%v: output has no not-writable warning:\n%s", dryRun, out)
			}
			// only the warning mentions the entry — no removed / would-remove line
			if n := strings.Count(out, "plugin entry"); n != 1 {
				t.Errorf("dryRun=%v: output reports a removal it didn't make:\n%s", dryRun, out)
			}
		}
	})

	t.Run("a symlinked config is never rewritten or replaced", func(t *testing.T) {
		pluginsDir := t.TempDir()
		real := filepath.Join(t.TempDir(), "dotfiles-config.json")
		content := `{"plugin":["` + filepath.Join(pluginsDir, legacyEntry) + `","a"]}`
		os.WriteFile(real, []byte(content), 0644) //nolint:errcheck
		configPath := filepath.Join(t.TempDir(), "config.json")
		os.Symlink(real, configPath) //nolint:errcheck
		var err error
		out := captureOutput(t, func() { err = CleanLegacyOpencode(pluginsDir, configPath, false) })
		if err != nil {
			t.Fatalf("CleanLegacyOpencode() error = %v", err)
		}
		if got, _ := os.ReadFile(real); string(got) != content {
			t.Errorf("symlink target = %q, rewritten", got)
		}
		if fi, _ := os.Lstat(configPath); fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("config.json symlink replaced by a file")
		}
		if !strings.Contains(out, "remove the plugin entry") {
			t.Errorf("no warning telling the user to remove the entry by hand:\n%s", out)
		}
	})

	t.Run("a symlinked plugins dir keeps its legacy files and lists them", func(t *testing.T) {
		checkout := t.TempDir()
		copyLegacyFixtures(t, checkout)
		before := snapshot(t, checkout)
		pluginsDir := filepath.Join(t.TempDir(), "plugins")
		os.Symlink(checkout, pluginsDir) //nolint:errcheck
		// The config entry is not a file removal through the link: its rules apply.
		configPath := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(configPath, []byte(`{"plugin":["`+filepath.Join(pluginsDir, legacyEntry)+`","a"]}`), 0644) //nolint:errcheck
		var err error
		out := captureOutput(t, func() { err = CleanLegacyOpencode(pluginsDir, configPath, false) })
		if err != nil {
			t.Errorf("CleanLegacyOpencode() error = %v", err)
		}
		if after := snapshot(t, checkout); !reflect.DeepEqual(before, after) {
			t.Errorf("legacy-named files behind the symlink were removed")
		}
		if !strings.Contains(out, "remove these by hand") || !strings.Contains(out, "package.json") {
			t.Errorf("no warning listing the legacy files left behind:\n%s", out)
		}
		if got, _ := os.ReadFile(configPath); string(got) != `{"plugin":["a"]}` {
			t.Errorf("config.json = %s, want the legacy entry removed as usual", got)
		}
	})

	t.Run("a symlinked devexp dir is refused", func(t *testing.T) {
		pluginsDir := t.TempDir()
		copyLegacyFixtures(t, pluginsDir)
		os.Symlink(t.TempDir(), filepath.Join(pluginsDir, "devexp")) //nolint:errcheck
		var err error
		captureOutput(t, func() { err = CleanLegacyOpencode(pluginsDir, filepath.Join(t.TempDir(), "config.json"), false) })
		if err == nil {
			t.Errorf("CleanLegacyOpencode() error = nil, want a symlink refusal")
		}
	})

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

// readOnlyDir makes dir unwritable for the rest of the test: files in it can
// still be rewritten in place, but no temp file can be created next to them.
func readOnlyDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) }) //nolint:errcheck
	if f, err := os.CreateTemp(dir, "probe"); err == nil {
		f.Close()
		os.Remove(f.Name()) //nolint:errcheck
		t.Skip("directory permissions are not enforced (running as root?)")
	}
}

// TestWrites_NeverInPlace: plugin files and config.json are only ever replaced
// through a temp file and a rename. Where no temp file can be created, the
// write fails and the old bytes stay — an in-place write would have succeeded
// and could have been cut short.
func TestWrites_NeverInPlace(t *testing.T) {
	t.Run("plugin file", func(t *testing.T) {
		pluginsDir := t.TempDir()
		dir := filepath.Join(pluginsDir, "devexp")
		os.MkdirAll(dir, 0755)                                            //nolint:errcheck
		os.WriteFile(filepath.Join(dir, "utils.js"), []byte("old"), 0644) //nolint:errcheck
		readOnlyDir(t, dir)
		var err error
		captureOutput(t, func() { err = writeIfChanged(pluginsDir, "devexp/utils.js", []byte("new")) })
		if got, _ := os.ReadFile(filepath.Join(dir, "utils.js")); err == nil || string(got) != "old" {
			t.Errorf("writeIfChanged() error = %v, file = %q; want an error and the old bytes", err, got)
		}
	})

	t.Run("config.json", func(t *testing.T) {
		pluginsDir := t.TempDir()
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config.json")
		content := `{"plugin":["` + filepath.Join(pluginsDir, legacyEntry) + `"]}`
		os.WriteFile(configPath, []byte(content), 0644) //nolint:errcheck
		readOnlyDir(t, dir)
		var err error
		captureOutput(t, func() { err = CleanLegacyOpencode(pluginsDir, configPath, false) })
		if got, _ := os.ReadFile(configPath); err == nil || string(got) != content {
			t.Errorf("CleanLegacyOpencode() error = %v, config = %q; want an error and the old bytes", err, got)
		}
	})
}

// TestInstallOpencode_EntryRemovalFailureKeepsPlugin: if devexp.js can't be
// deleted, devexp/ must not be emptied either.
func TestInstallOpencode_EntryRemovalFailureKeepsPlugin(t *testing.T) {
	repoDir := t.TempDir()
	writeOpencodeRepo(t, repoDir, allThree...)
	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	recorded := installFull(t, repoDir, pluginsDir)
	before := snapshot(t, filepath.Join(pluginsDir, "devexp"))
	readOnlyDir(t, pluginsDir) // devexp.js can't be unlinked; devexp/ stays writable

	var got []string
	var err error
	out := captureOutput(t, func() { got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, allThree, recorded, false) })
	if err != nil {
		t.Fatalf("InstallOpencode() error = %v", err)
	}
	if after := snapshot(t, filepath.Join(pluginsDir, "devexp")); !reflect.DeepEqual(before, after) {
		t.Errorf("devexp/ emptied although devexp.js could not be removed")
	}
	if len(got) != len(recorded) {
		t.Errorf("InstallOpencode() = %v, want every kept file %v recorded", got, recorded)
	}
	if !strings.Contains(out, "hooks remain active") {
		t.Errorf("no warning that the plugin stays active:\n%s", out)
	}
}

// TestRemovePluginFiles_RechecksRoots: removal re-checks plugins/ and devexp/
// itself, so a directory swapped for a symlink after InstallOpencode's checks
// is still never removed through.
func TestRemovePluginFiles_RechecksRoots(t *testing.T) {
	pluginsDir := t.TempDir()
	checkout := t.TempDir()
	for _, n := range []string{"utils.js", "hooks.json", "secret-guard.js"} {
		os.WriteFile(filepath.Join(checkout, n), []byte(n), 0644) //nolint:errcheck
	}
	os.Symlink(checkout, filepath.Join(pluginsDir, "devexp")) //nolint:errcheck
	stale := []string{"devexp/utils.js", "devexp/hooks.json", "devexp/secret-guard.js"}

	var kept []string
	out := captureOutput(t, func() { kept = removePluginFiles(pluginsDir, stale, "no longer installed", false) })
	if len(listTree(t, checkout)) != 3 {
		t.Errorf("files removed through the symlinked devexp/: %v left", listTree(t, checkout))
	}
	if !reflect.DeepEqual(kept, stale) || !strings.Contains(out, "is a symlink") {
		t.Errorf("removePluginFiles() kept %v, output %q; want all kept with a symlink warning", kept, out)
	}

	t.Run("a plugins dir swapped for a symlink", func(t *testing.T) {
		real := t.TempDir()
		os.MkdirAll(filepath.Join(real, "devexp"), 0755) //nolint:errcheck
		stale := []string{"devexp.js", "devexp/utils.js"}
		for _, rel := range stale {
			os.WriteFile(filepath.Join(real, filepath.FromSlash(rel)), []byte(rel), 0644) //nolint:errcheck
		}
		pluginsDir := filepath.Join(t.TempDir(), "plugins")
		os.Symlink(real, pluginsDir) //nolint:errcheck
		for _, dryRun := range []bool{false, true} {
			var kept []string
			out := captureOutput(t, func() { kept = removePluginFiles(pluginsDir, stale, "no longer installed", dryRun) })
			if len(listTree(t, real)) != 2 || !reflect.DeepEqual(kept, stale) {
				t.Errorf("dryRun=%v: removed through the plugins symlink (kept %v, left %v)", dryRun, kept, listTree(t, real))
			}
			if !strings.Contains(out, "never removes files through it") || strings.Contains(out, "[dry-run] remove") {
				t.Errorf("dryRun=%v: output %q, want the left-behind warning and no removal lines", dryRun, out)
			}
		}
	})
}

// ── Uninstall ─────────────────────────────────────────────────────────────────

// treeState records everything under root: files with their content,
// directories, and symlinks with their target relative to root. Two fixtures
// built in different temp dirs compare equal.
func treeState(t *testing.T, root string) map[string]string {
	t.Helper()
	state := map[string]string{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil || p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, _ := os.Readlink(p)
			if r, err := filepath.Rel(root, target); err == nil {
				target = r
			}
			state[rel] = "symlink -> " + filepath.ToSlash(target)
		case info.IsDir():
			state[rel] = "dir"
		default:
			data, _ := os.ReadFile(p)
			state[rel] = string(data)
		}
		return nil
	})
	return state
}

func stateKeys(state map[string]string) []string {
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// moveAndLink moves path into root/<name> and leaves a symlink in its place.
func moveAndLink(t *testing.T, path, root, name string) {
	t.Helper()
	target := filepath.Join(root, name)
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// uninstallCase is one plugin directory state. arrange builds it under root
// (plugins/ is root/plugins; anything a link points at also lives in root)
// and returns the manifest's recorded plugins list.
type uninstallCase struct {
	arrange     func(t *testing.T, root, repoDir, pluginsDir string) []string
	nilRegistry bool
	dryRun      bool
	// want lists what is left under root; nil means nothing may change.
	want     []string
	keptAll  bool     // UninstallOpencode returns every recorded path as kept
	wantKept []string // otherwise, what it returns as kept
	wantErr  string   // the roots are refused
	wantOut  string
	outCount int // how often wantOut appears; 0 means at least once
}

func uninstallCases() map[string]uninstallCase {
	full := func(t *testing.T, _, repoDir, pluginsDir string) []string {
		return installFull(t, repoDir, pluginsDir)
	}
	return map[string]uninstallCase{
		"a recorded install is removed and devexp/ pruned": {
			arrange: full,
			want:    []string{"plugins"},
		},
		"foreign files next to the plugin are kept": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				recorded := installFull(t, repoDir, pluginsDir)
				writeFiles(t, pluginsDir, map[string]string{
					"my-plugin.js":      "export const mine = 1\n",
					"package.json":      `{"main":"x.js"}`,
					"devexp/note.txt":   "mine\n",
					"devexp/mine.js.md": "mine\n",
				})
				return recorded
			},
			want: []string{"plugins", "plugins/devexp", "plugins/devexp/mine.js.md", "plugins/devexp/note.txt", "plugins/my-plugin.js", "plugins/package.json"},
		},
		"a lost manifest still removes the plugin found on disk": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				installFull(t, repoDir, pluginsDir)
				writeFiles(t, pluginsDir, map[string]string{"my-plugin.js": "mine\n"})
				return nil
			},
			want: []string{"plugins", "plugins/my-plugin.js"},
		},
		"a recorded devexp.js without the header is removed": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				recorded := installFull(t, repoDir, pluginsDir)
				writeFiles(t, pluginsDir, map[string]string{"devexp.js": "/*"})
				return recorded
			},
			want: []string{"plugins"},
		},
		"an unrecorded foreign devexp.js keeps devexp/ with it": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				installFull(t, repoDir, pluginsDir)
				writeFiles(t, pluginsDir, map[string]string{"devexp.js": "export const Mine = async () => ({})\n"})
				return nil
			},
			wantKept: []string{"devexp/utils.js", "devexp/package.json", "devexp/secret-guard.js", "devexp/lint-on-save.js", "devexp/graphify-read-guard.js", "devexp/hooks.json"},
			wantOut:  "not recognised as devexp's",
		},
		"a lost manifest and an edited entry (// @ts-check) keep the whole plugin": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				installFull(t, repoDir, pluginsDir)
				entry := filepath.Join(pluginsDir, "devexp.js")
				content, err := os.ReadFile(entry)
				if err != nil {
					t.Fatal(err)
				}
				writeFiles(t, pluginsDir, map[string]string{"devexp.js": "// @ts-check\n" + string(content)})
				return nil
			},
			wantKept: []string{"devexp/utils.js", "devexp/package.json", "devexp/secret-guard.js", "devexp/lint-on-save.js", "devexp/graphify-read-guard.js", "devexp/hooks.json"},
			wantOut:  "not recognised as devexp's",
		},
		"a symlinked devexp.js keeps the whole plugin": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				recorded := installFull(t, repoDir, pluginsDir)
				moveAndLink(t, filepath.Join(pluginsDir, "devexp.js"), root, "dotfiles-devexp.js")
				return recorded
			},
			keptAll: true,
			wantOut: "hooks remain active",
		},
		"an unrecorded symlinked devexp.js still keeps devexp/": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				installFull(t, repoDir, pluginsDir)
				moveAndLink(t, filepath.Join(pluginsDir, "devexp.js"), root, "dotfiles-devexp.js")
				return nil
			},
			wantKept: []string{"devexp/utils.js", "devexp/package.json", "devexp/secret-guard.js", "devexp/lint-on-save.js", "devexp/graphify-read-guard.js", "devexp/hooks.json"},
			wantOut:  "hooks remain active",
		},
		"an entry that can't be removed keeps the whole plugin": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				recorded := installFull(t, repoDir, pluginsDir)
				readOnlyDir(t, pluginsDir)
				return recorded
			},
			keptAll: true,
			wantOut: "hooks remain active",
		},
		"nothing is removed through a symlinked plugins/": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				recorded := installFull(t, repoDir, pluginsDir)
				moveAndLink(t, pluginsDir, root, "dotfiles-plugins")
				return recorded
			},
			keptAll: true,
			wantOut: "never removes files through it; remove these by hand",
		},
		"a dangling plugins/ link is refused": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				recorded := installFull(t, repoDir, pluginsDir)
				moveAndLink(t, pluginsDir, root, "moved")
				if err := os.Rename(filepath.Join(root, "moved"), filepath.Join(root, "elsewhere")); err != nil {
					t.Fatal(err)
				}
				return recorded
			},
			wantErr: "doesn't point at a directory",
		},
		"a plugins/ that is a file is refused": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				writeFiles(t, root, map[string]string{"plugins": "not a dir"})
				return []string{"devexp.js", "devexp/hooks.json"}
			},
			wantErr: "is not a directory",
		},
		"a symlinked devexp/ is refused": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				recorded := installFull(t, repoDir, pluginsDir)
				moveAndLink(t, filepath.Join(pluginsDir, "devexp"), root, "checkout")
				return recorded
			},
			wantErr: "is a symlink",
		},
		"hostile recorded paths are reported and never removed": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				writeFiles(t, root, map[string]string{
					"x":                         "outside\n",
					"plugins/devexp/sub/a.js":   "nested\n",
					"plugins/devexp/a.txt":      "not a module\n",
					"plugins/devexp/.hidden.js": "hidden\n",
				})
				return []string{"../x", "devexp/../../x", "/etc/hosts", "devexp/sub/a.js", "devexp/a.txt", "devexp/.hidden.js"}
			},
			wantOut:  "listed in the manifest but not a devexp plugin file",
			outCount: 6,
		},
		"without a registry, recorded and fixed names go and an unrecorded module stays": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				var recorded []string
				for _, rel := range installFull(t, repoDir, pluginsDir) {
					if rel != "devexp/lint-on-save.js" {
						recorded = append(recorded, rel)
					}
				}
				return recorded
			},
			nilRegistry: true,
			want:        []string{"plugins", "plugins/devexp", "plugins/devexp/lint-on-save.js"},
		},
		"a dry run changes nothing": {
			arrange: full,
			dryRun:  true,
		},
		"an empty plugins/ has nothing to remove": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string {
				if err := os.MkdirAll(pluginsDir, 0755); err != nil {
					t.Fatal(err)
				}
				return nil
			},
			wantOut: "not installed",
		},
		"a missing plugins/ has nothing to remove": {
			arrange: func(t *testing.T, root, repoDir, pluginsDir string) []string { return nil },
			want:    []string{},
		},
	}
}

func TestUninstallOpencode(t *testing.T) {
	for name, tt := range uninstallCases() {
		t.Run(name, func(t *testing.T) {
			repoDir := t.TempDir()
			writeOpencodeRepo(t, repoDir, allThree...)
			root := t.TempDir()
			pluginsDir := filepath.Join(root, "plugins")
			recorded := tt.arrange(t, root, repoDir, pluginsDir)
			registry := opencodeRegistry()
			if tt.nilRegistry {
				registry = nil
			}
			before := treeState(t, root)

			var kept []string
			var err error
			out := stripANSI(captureOutput(t, func() { kept, err = UninstallOpencode(registry, pluginsDir, recorded, tt.dryRun) }))

			after := treeState(t, root)
			for rel, content := range after {
				if was, ok := before[rel]; !ok || was != content {
					t.Errorf("%s was created or changed: %q -> %q", rel, was, content)
				}
			}
			want := tt.want
			if want == nil {
				want = stateKeys(before)
			}
			if got := stateKeys(after); !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
				t.Errorf("left under root = %v, want %v", got, want)
			}

			switch {
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("UninstallOpencode() error = %v, want %q", err, tt.wantErr)
				}
			case err != nil:
				t.Errorf("UninstallOpencode() error = %v", err)
			case tt.keptAll:
				if len(kept) != len(recorded) || kept[0] != "devexp.js" {
					t.Errorf("UninstallOpencode() kept %v, want every recorded path %v, entry first", kept, recorded)
				}
			case !reflect.DeepEqual(kept, tt.wantKept):
				t.Errorf("UninstallOpencode() kept %v, want %v", kept, tt.wantKept)
			}

			if tt.wantOut != "" {
				n := strings.Count(out, tt.wantOut)
				if (tt.outCount == 0 && n == 0) || (tt.outCount > 0 && n != tt.outCount) {
					t.Errorf("output has %q %d times, want %d (0 = at least once):\n%s", tt.wantOut, n, tt.outCount, out)
				}
			}
			if tt.dryRun {
				if n := strings.Count(out, " (uninstall)"); n != len(recorded) {
					t.Errorf("dry run listed %d removals, want %d:\n%s", n, len(recorded), out)
				}
			}
		})
	}
}

// TestUninstallOpencode_MatchesInstall pins uninstall to install: removing the
// plugin with every hook disabled and uninstalling it must leave the same
// tree, keep the same files and refuse the same roots, for every fixture.
func TestUninstallOpencode_MatchesInstall(t *testing.T) {
	for name, tt := range uninstallCases() {
		t.Run(name, func(t *testing.T) {
			repoDir := t.TempDir()
			writeOpencodeRepo(t, repoDir, allThree...)
			registry := opencodeRegistry()
			if tt.nilRegistry {
				registry = nil
			}
			arrange := func() (root, pluginsDir string, recorded []string) {
				root = t.TempDir()
				pluginsDir = filepath.Join(root, "plugins")
				return root, pluginsDir, tt.arrange(t, root, repoDir, pluginsDir)
			}
			installRoot, installPlugins, installRecorded := arrange()
			uninstallRoot, uninstallPlugins, uninstallRecorded := arrange()
			if !reflect.DeepEqual(treeState(t, installRoot), treeState(t, uninstallRoot)) {
				t.Fatalf("the two fixtures differ before the run")
			}

			var installKept, uninstallKept []string
			var installErr, uninstallErr error
			captureOutput(t, func() {
				installKept, installErr = InstallOpencode(registry, repoDir, installPlugins, allThree, installRecorded, tt.dryRun)
				uninstallKept, uninstallErr = UninstallOpencode(registry, uninstallPlugins, uninstallRecorded, tt.dryRun)
			})

			if got, want := treeState(t, uninstallRoot), treeState(t, installRoot); !reflect.DeepEqual(got, want) {
				t.Errorf("uninstall left %v, install with every hook disabled left %v", stateKeys(got), stateKeys(want))
			}
			if (installErr == nil) != (uninstallErr == nil) {
				t.Errorf("install error = %v, uninstall error = %v; want both or neither", installErr, uninstallErr)
			}
			if installErr == nil && !reflect.DeepEqual(installKept, uninstallKept) {
				t.Errorf("uninstall kept %v, install kept %v", uninstallKept, installKept)
			}
		})
	}
}

// TestWarnLeftBehind_SaysWhy: a plugins/ that is itself a symlink is named as
// one; one reached through a linked parent says where it resolves.
func TestWarnLeftBehind_SaysWhy(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "dotfiles", "plugins")
	os.MkdirAll(real, 0755) //nolint:errcheck
	link := filepath.Join(root, "plugins")
	os.Symlink(real, link) //nolint:errcheck
	out := captureOutput(t, func() { warnLeftBehind(link, []string{"devexp.js"}) })
	if !strings.Contains(out, link+" is a symlink — devexp never removes files through it; remove these by hand: devexp.js") {
		t.Errorf("symlinked plugins/ warning = %q", out)
	}
	os.MkdirAll(filepath.Join(root, "real-config", "opencode", "plugins"), 0755)   //nolint:errcheck
	os.Symlink(filepath.Join(root, "real-config"), filepath.Join(root, ".config")) //nolint:errcheck
	behind := filepath.Join(root, ".config", "opencode", "plugins")
	out = captureOutput(t, func() { warnLeftBehind(behind, []string{"devexp.js"}) })
	if !strings.Contains(out, behind+" is behind a symlink (it resolves to ") {
		t.Errorf("behind-a-symlink warning = %q", out)
	}
}
