package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"devexp/internal/config"
)

// ── Hidden uninstall command ──────────────────────────────────────────────────

// installedOpencodeHome installs the three-hook test plugin into a temp HOME
// and points DEVEXP_DIR at its repo, as uninstall.sh does.
func installedOpencodeHome(t *testing.T) (home string, p opencodePaths) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	fakeCLI(t)
	repoDir := writeOpencodeHookRepo(t)
	t.Setenv("DEVEXP_DIR", repoDir)
	if out, err := runOpencode(t, repoDir, &config.Config{}); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	return home, testOpencodePaths(t, home)
}

func uninstallOpencode(t *testing.T, home string, dryRun bool) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() { err = doUninstallOpencode(home, dryRun) })
	return out, err
}

func readManifestJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest %s: %v", data, err)
	}
	return m
}

func copyLegacyOpencodeFixtures(t *testing.T, dir string) {
	t.Helper()
	fixtures := filepath.Join("..", "internal", "hooks", "testdata", "legacy-opencode")
	entries, err := os.ReadDir(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(fixtures, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDoUninstallOpencode_Manifest(t *testing.T) {
	t.Run("the plugins key is dropped once everything is removed, agents and skills kept", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		m := readManifestJSON(t, p.manifest)
		m["agents"] = []any{"a.md", "b.md"}
		m["skills"] = []any{"s"}
		data, _ := json.Marshal(m)
		os.WriteFile(p.manifest, data, 0o644) //nolint:errcheck

		if out, err := uninstallOpencode(t, home, false); err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if tree := pluginTree(t, p.plugins); len(tree) != 0 {
			t.Errorf("plugins tree = %v, want empty", tree)
		}
		got := readManifestJSON(t, p.manifest)
		if _, ok := got["plugins"]; ok {
			t.Errorf("manifest still has a plugins key: %v", got)
		}
		if !reflect.DeepEqual(got["agents"], m["agents"]) || !reflect.DeepEqual(got["skills"], m["skills"]) {
			t.Errorf("manifest = %v, want agents %v and skills %v kept", got, m["agents"], m["skills"])
		}
	})

	t.Run("files that had to stay stay recorded", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		readOnlyTestDir(t, filepath.Join(p.plugins, "devexp"))

		if out, err := uninstallOpencode(t, home, false); err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		want := []string{"devexp/utils.js", "devexp/package.json", "devexp/secret-guard.js", "devexp/lint-on-save.js", "devexp/graphify-read-guard.js", "devexp/hooks.json"}
		if got := loadManifestPlugins(t, home); !reflect.DeepEqual(got, want) {
			t.Errorf("manifest plugins = %v, want the kept %v", got, want)
		}
		if _, err := os.Lstat(filepath.Join(p.plugins, "devexp.js")); !os.IsNotExist(err) {
			t.Errorf("devexp.js not removed")
		}
	})

	t.Run("a missing manifest is not created", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		os.Remove(p.manifest) //nolint:errcheck

		if out, err := uninstallOpencode(t, home, false); err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if _, err := os.Lstat(p.manifest); !os.IsNotExist(err) {
			t.Errorf("manifest created: %v", err)
		}
		if tree := pluginTree(t, p.plugins); len(tree) != 0 {
			t.Errorf("plugins tree = %v, want the plugin found on disk removed", tree)
		}
	})

	t.Run("a missing manifest is not created even when files had to stay", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		os.Remove(p.manifest) //nolint:errcheck
		dotfiles := filepath.Join(t.TempDir(), "plugins")
		if err := os.Rename(p.plugins, dotfiles); err != nil {
			t.Fatal(err)
		}
		os.Symlink(dotfiles, p.plugins) //nolint:errcheck

		out, err := uninstallOpencode(t, home, false)
		if err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if !strings.Contains(out, "never removes files through it") {
			t.Errorf("no warning about the files left behind:\n%s", out)
		}
		if _, err := os.Lstat(p.manifest); !os.IsNotExist(err) {
			t.Errorf("manifest created: %v", err)
		}
	})

	t.Run("a manifest symlink is never written through", func(t *testing.T) {
		for _, dangling := range []bool{true, false} {
			t.Run(map[bool]string{true: "dangling", false: "to a manifest"}[dangling], func(t *testing.T) {
				home, p := installedOpencodeHome(t)
				dotfiles := t.TempDir()
				target := filepath.Join(dotfiles, "manifest.json")
				if dangling {
					os.Remove(p.manifest) //nolint:errcheck
					// Files have to stay (symlinked plugins/), so a save would happen.
					linked := filepath.Join(dotfiles, "plugins")
					if err := os.Rename(p.plugins, linked); err != nil {
						t.Fatal(err)
					}
					os.Symlink(linked, p.plugins) //nolint:errcheck
				} else if err := os.Rename(p.manifest, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, p.manifest); err != nil {
					t.Fatal(err)
				}
				before, _ := os.ReadFile(target)

				out, err := uninstallOpencode(t, home, false)
				if err != nil {
					t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
				}
				after, afterErr := os.ReadFile(target)
				if dangling && !os.IsNotExist(afterErr) {
					t.Errorf("manifest created behind the dangling link: %s", after)
				}
				if !dangling && !bytes.Equal(before, after) {
					t.Errorf("manifest behind the link changed:\n%s", after)
				}
				if fi, err := os.Lstat(p.manifest); err != nil || fi.Mode()&os.ModeSymlink == 0 {
					t.Errorf("manifest symlink replaced")
				}
				if !strings.Contains(out, "is a symlink, so it was left untouched") {
					t.Errorf("no warning about the manifest symlink:\n%s", out)
				}
				if !dangling {
					if tree := pluginTree(t, p.plugins); len(tree) != 0 {
						t.Errorf("plugins tree = %v, want the recorded plugin removed", tree)
					}
				}
			})
		}
	})

	t.Run("a malformed manifest is never rewritten and the plugin is found on disk", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		const bad = `{"agents": "oops"`
		os.WriteFile(p.manifest, []byte(bad), 0o644) //nolint:errcheck

		out, err := uninstallOpencode(t, home, false)
		if err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if got, _ := os.ReadFile(p.manifest); string(got) != bad {
			t.Errorf("manifest = %q, want it untouched", got)
		}
		if !strings.Contains(out, "is unreadable, so plugin files are identified from disk only") {
			t.Errorf("no warning about the manifest:\n%s", out)
		}
		if tree := pluginTree(t, p.plugins); len(tree) != 0 {
			t.Errorf("plugins tree = %v, want empty", tree)
		}
	})

	t.Run("a malformed manifest is not rewritten even when files had to stay", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		dotfiles := filepath.Join(t.TempDir(), "plugins")
		if err := os.Rename(p.plugins, dotfiles); err != nil {
			t.Fatal(err)
		}
		os.Symlink(dotfiles, p.plugins) //nolint:errcheck
		const bad = `{"agents": "oops"`
		os.WriteFile(p.manifest, []byte(bad), 0o644) //nolint:errcheck

		out, err := uninstallOpencode(t, home, false)
		if err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if !strings.Contains(out, "never removes files through it") {
			t.Errorf("no warning about the files left behind:\n%s", out)
		}
		if got, _ := os.ReadFile(p.manifest); string(got) != bad {
			t.Errorf("manifest = %q, want it untouched", got)
		}
	})

	t.Run("a manifest with a type mismatch is discarded whole, so a headerless devexp.js is kept", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("DEVEXP_DIR", writeOpencodeHookRepo(t))
		p := testOpencodePaths(t, home)
		os.MkdirAll(p.plugins, 0o755) //nolint:errcheck
		const entry = "export const Mine = async () => ({})\n"
		os.WriteFile(filepath.Join(p.plugins, "devexp.js"), []byte(entry), 0o644) //nolint:errcheck
		const m = `{"agents":5,"plugins":["devexp.js"]}`
		os.WriteFile(p.manifest, []byte(m), 0o644) //nolint:errcheck

		if out, err := uninstallOpencode(t, home, false); err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if got, _ := os.ReadFile(filepath.Join(p.plugins, "devexp.js")); string(got) != entry {
			t.Errorf("devexp.js = %q, want the unowned entry kept", got)
		}
		if got, _ := os.ReadFile(p.manifest); string(got) != m {
			t.Errorf("manifest = %q, want it untouched", got)
		}
	})
}

func TestDoUninstallOpencode_DryRun(t *testing.T) {
	home, p := installedOpencodeHome(t)
	copyLegacyOpencodeFixtures(t, p.plugins)
	cfg := `{"theme":"x","plugin":["` + filepath.Join(p.plugins, "devexp-plugin.js") + `","npm-plugin"]}`
	os.WriteFile(p.config, []byte(cfg), 0o644) //nolint:errcheck
	base := filepath.Dir(p.plugins)
	before := treeBytes(t, base)

	out, err := uninstallOpencode(t, home, true)
	if err != nil {
		t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
	}
	if after := treeBytes(t, base); !reflect.DeepEqual(before, after) {
		t.Errorf("dry run changed ~/.config/opencode")
	}
	for _, want := range []string{"(uninstall)", "remove plugin entry", "devexp-plugin.js"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output lacks %q:\n%s", want, out)
		}
	}
}

func TestDoUninstallOpencode_Legacy(t *testing.T) {
	t.Run("legacy files and the config entry go, everything else stays byte for byte", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		copyLegacyOpencodeFixtures(t, p.plugins)
		os.WriteFile(filepath.Join(p.plugins, "my-plugin.js"), []byte("mine\n"), 0o644) //nolint:errcheck
		cfg := `{"theme":"x","plugin":["` + filepath.Join(p.plugins, "devexp-plugin.js") + `","npm-plugin"]}`
		os.WriteFile(p.config, []byte(cfg), 0o644) //nolint:errcheck

		if out, err := uninstallOpencode(t, home, false); err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if tree := pluginTree(t, p.plugins); !reflect.DeepEqual(tree, []string{"my-plugin.js"}) {
			t.Errorf("plugins tree = %v, want only my-plugin.js", tree)
		}
		if got, _ := os.ReadFile(p.config); string(got) != `{"theme":"x","plugin":["npm-plugin"]}` {
			t.Errorf("config.json = %s, want only the legacy entry spliced out", got)
		}
	})

	t.Run("a symlinked config.json is left untouched", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		copyLegacyOpencodeFixtures(t, p.plugins)
		dotfiles := filepath.Join(t.TempDir(), "config.json")
		cfg := `{"plugin":["` + filepath.Join(p.plugins, "devexp-plugin.js") + `"]}`
		os.WriteFile(dotfiles, []byte(cfg), 0o644) //nolint:errcheck
		os.Remove(p.config)                        //nolint:errcheck
		if err := os.Symlink(dotfiles, p.config); err != nil {
			t.Fatal(err)
		}

		out, err := uninstallOpencode(t, home, false)
		if err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if got, _ := os.ReadFile(dotfiles); string(got) != cfg {
			t.Errorf("link target = %s, want it byte-identical", got)
		}
		if fi, err := os.Lstat(p.config); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("config.json symlink replaced")
		}
		if !strings.Contains(out, "is a symlink, so it was left untouched") {
			t.Errorf("no warning about the symlinked config.json:\n%s", out)
		}
	})

	t.Run("refused roots fail the run and change nothing, config.json included", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		copyLegacyOpencodeFixtures(t, p.plugins)
		cfg := `{"plugin":["` + filepath.Join(p.plugins, "devexp-plugin.js") + `"]}`
		os.WriteFile(p.config, []byte(cfg), 0o644) //nolint:errcheck
		checkout := filepath.Join(t.TempDir(), "checkout")
		if err := os.Rename(filepath.Join(p.plugins, "devexp"), checkout); err != nil {
			t.Fatal(err)
		}
		os.Symlink(checkout, filepath.Join(p.plugins, "devexp")) //nolint:errcheck
		base := filepath.Dir(p.plugins)
		before, beforeCheckout := treeBytes(t, base), treeBytes(t, checkout)
		manifestBefore, _ := os.ReadFile(p.manifest)

		out, err := uninstallOpencode(t, home, false)
		if err == nil || !strings.Contains(err.Error(), "is a symlink") {
			t.Fatalf("doUninstallOpencode() error = %v, want the symlink refusal\n%s", err, out)
		}
		if after := treeBytes(t, base); !reflect.DeepEqual(before, after) {
			t.Errorf("~/.config/opencode changed on a refused run")
		}
		if after := treeBytes(t, checkout); !reflect.DeepEqual(beforeCheckout, after) {
			t.Errorf("files behind the devexp symlink changed")
		}
		if got, _ := os.ReadFile(p.manifest); !bytes.Equal(got, manifestBefore) {
			t.Errorf("manifest rewritten on a refused run")
		}
	})
}

func TestDoUninstallOpencode_Registry(t *testing.T) {
	t.Run("an unreadable registry warns and the recorded files still go", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		t.Setenv("DEVEXP_DIR", t.TempDir())

		out, err := uninstallOpencode(t, home, false)
		if err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if !strings.Contains(out, "hooks registry:") {
			t.Errorf("no registry warning:\n%s", out)
		}
		if tree := pluginTree(t, p.plugins); len(tree) != 0 {
			t.Errorf("plugins tree = %v, want empty", tree)
		}
	})

	t.Run("without DEVEXP_DIR the embedded registry identifies modules on disk", func(t *testing.T) {
		home, p := installedOpencodeHome(t)
		os.Remove(p.manifest) //nolint:errcheck
		t.Setenv("DEVEXP_DIR", "")

		out, err := uninstallOpencode(t, home, false)
		if err != nil {
			t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
		}
		if strings.Contains(out, "hooks registry:") {
			t.Errorf("embedded registry not read:\n%s", out)
		}
		if tree := pluginTree(t, p.plugins); len(tree) != 0 {
			t.Errorf("plugins tree = %v, want every module removed", tree)
		}
	})
}

// executeRoot runs the root command with args, then resets its args, output
// and every flag, so the next run starts from the defaults.
func executeRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	defer func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		// A slice flag's DefValue renders as "[]", which Set would parse as the
		// one-element slice ["[]"] rather than as empty. SliceValue.Replace is
		// the only reset that round-trips.
		reset := func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				sv.Replace(nil) //nolint:errcheck
			} else {
				f.Value.Set(f.DefValue) //nolint:errcheck
			}
			f.Changed = false
		}
		rootCmd.Flags().VisitAll(reset)
		for _, c := range rootCmd.Commands() {
			c.Flags().VisitAll(reset)
		}
	}()
	var err error
	stdout := captureStdout(t, func() { err = rootCmd.Execute() })
	return buf.String() + stdout, err
}

func TestUninstallCmd(t *testing.T) {
	t.Run("a missing or unknown target is an error naming the supported ones", func(t *testing.T) {
		for _, args := range [][]string{{"uninstall"}, {"uninstall", "--target", "claude"}} {
			out, err := executeRoot(t, args...)
			if err == nil || !strings.Contains(err.Error(), "unsupported target") || !strings.Contains(err.Error(), "supported: opencode") {
				t.Errorf("%v: error = %v, want an unsupported-target error naming opencode", args, err)
			}
			if strings.Contains(out, "Usage:") {
				t.Errorf("%v: usage dumped on error:\n%s", args, out)
			}
		}
	})

	t.Run("the command is hidden and its help names every target", func(t *testing.T) {
		out, err := executeRoot(t, "--help")
		if err != nil || strings.Contains(out, "uninstall") {
			t.Errorf("root help (err %v) lists the hidden command:\n%s", err, out)
		}
		out, err = executeRoot(t, "uninstall", "--help")
		// uninstall.sh's probe matches "supported: " followed by the target.
		if err != nil || !strings.Contains(out, supportedUninstallTargets()) {
			t.Errorf("uninstall --help (err %v) lacks %q:\n%s", err, supportedUninstallTargets(), out)
		}
		for target := range uninstallTargets {
			if !strings.Contains(out, target) {
				t.Errorf("uninstall --help does not name target %q", target)
			}
		}
	})

	t.Run("flags reach the handler", func(t *testing.T) {
		_, p := installedOpencodeHome(t)
		before := treeBytes(t, p.plugins)
		out, err := executeRoot(t, "uninstall", "--target", "opencode", "--dry-run", "--yes")
		if err != nil {
			t.Fatalf("uninstall --dry-run error = %v\n%s", err, out)
		}
		if after := treeBytes(t, p.plugins); !reflect.DeepEqual(before, after) {
			t.Errorf("--dry-run removed files")
		}
		if _, err := executeRoot(t, "uninstall", "--target", "opencode", "-y"); err != nil {
			t.Fatalf("uninstall error = %v", err)
		}
		if tree := pluginTree(t, p.plugins); len(tree) != 0 {
			t.Errorf("plugins tree = %v, want empty", tree)
		}
	})
}

// TestUninstallCmd_RefusesBadHome: with HOME unset, empty or relative, target
// paths would resolve under the current directory. Nothing there is touched.
func TestUninstallCmd_RefusesBadHome(t *testing.T) {
	for name, set := range badHomes {
		t.Run(name, func(t *testing.T) {
			cwd := t.TempDir()
			t.Chdir(cwd)
			t.Setenv("DEVEXP_DIR", writeOpencodeHookRepo(t))
			plugins := filepath.Join(cwd, "home", ".config", "opencode", "plugins")
			for _, dir := range []string{filepath.Join(cwd, ".config", "opencode", "plugins"), plugins} {
				os.MkdirAll(filepath.Join(dir, "devexp"), 0o755)                                                                                        //nolint:errcheck
				os.WriteFile(filepath.Join(dir, "devexp.js"), []byte("/**\n * devexp-plugin.js — entry point for devexp opencode hooks\n */\n"), 0o644) //nolint:errcheck
				os.WriteFile(filepath.Join(dir, "devexp", "utils.js"), []byte("x"), 0o644)                                                              //nolint:errcheck
			}
			before := treeBytes(t, cwd)
			set(t)

			out, err := executeRoot(t, "uninstall", "--target", "opencode")
			if err == nil || !strings.Contains(err.Error(), "HOME") || !strings.Contains(err.Error(), "refusing to remove anything") {
				t.Errorf("uninstall error = %v, want a HOME refusal\n%s", err, out)
			}
			if after := treeBytes(t, cwd); !reflect.DeepEqual(before, after) {
				t.Errorf("files under the current directory changed")
			}
		})
	}
}

func readOnlyTestDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) }) //nolint:errcheck
	if f, err := os.CreateTemp(dir, "probe"); err == nil {
		f.Close()
		os.Remove(f.Name()) //nolint:errcheck
		t.Skip("directory permissions are not enforced (running as root?)")
	}
}

// TestDoUninstallOpencode_LeavesAssetCache: uninstall.sh may run from the
// embedded-asset cache, and repo.Resolve wipes that cache when its version
// marker differs. The uninstall command must never resolve the repo.
func TestDoUninstallOpencode_LeavesAssetCache(t *testing.T) {
	home, p := installedOpencodeHome(t)
	t.Setenv("DEVEXP_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Chdir(t.TempDir()) // no repo to find by walking up
	base, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache dir: %v", err)
	}
	cache := filepath.Join(base, "devexp", "assets")
	if !strings.HasPrefix(cache, home) {
		t.Skipf("user cache dir %s is not under the temp HOME", cache)
	}
	os.MkdirAll(cache, 0o755)                                                          //nolint:errcheck
	os.WriteFile(filepath.Join(cache, ".devexp-version"), []byte("some-other"), 0o644) //nolint:errcheck
	os.WriteFile(filepath.Join(cache, "uninstall.sh"), []byte("#!/bin/bash\n"), 0o755) //nolint:errcheck
	before := treeBytes(t, cache)

	if out, err := uninstallOpencode(t, home, false); err != nil {
		t.Fatalf("doUninstallOpencode() error = %v\n%s", err, out)
	}
	if after := treeBytes(t, cache); !reflect.DeepEqual(before, after) {
		t.Errorf("the asset cache uninstall.sh runs from was changed: %v -> %v", before, after)
	}
	if tree := pluginTree(t, p.plugins); len(tree) != 0 {
		t.Errorf("plugins tree = %v, want empty", tree)
	}
}
