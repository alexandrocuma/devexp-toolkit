package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ── plugins/ behind a symlinked parent ───────────────────────────────────────

func TestPluginsHome(t *testing.T) {
	for dir, want := range map[string]string{
		"/home/u/.config/opencode/plugins":  "/home/u",
		"/home/u/.config/opencode/plugins/": "/home/u",
		"/tmp/x/plugins":                    "/tmp/x",
		"/.config/opencode/plugins":         "/.config/opencode",
	} {
		if got := pluginsHome(dir); got != want {
			t.Errorf("pluginsHome(%q) = %q, want %q", dir, got, want)
		}
	}
}

// TestInstallOpencode_PluginsBehindSymlink: plugins/ is a real directory, but
// under a symlinked ~/.config/opencode or ~/.config, so it is in the dotfiles
// tree. The plugin is installed through the link, but neither a disabled
// module, the whole plugin, legacy flat files nor an empty devexp/ is removed
// through it. A HOME that is itself behind a symlink (as every macOS temp dir
// is, under /var -> /private/var) still gets its stale files removed.
func TestInstallOpencode_PluginsBehindSymlink(t *testing.T) {
	legacy := "/**\n * secret-guard.js — blocks accidental reads of .env and private key files\n */\n"
	tests := map[string]struct {
		link       string // relative to home: the symlinked directory, or ""
		linkHome   bool
		wantBehind bool
	}{
		"a linked ~/.config/opencode": {link: ".config/opencode", wantBehind: true},
		"a linked ~/.config":          {link: ".config", wantBehind: true},
		"home behind a symlink":       {linkHome: true},
	}
	for name, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(name+" "+dryRunName(dryRun), func(t *testing.T) {
				root := t.TempDir()
				home := filepath.Join(root, "home")
				if tt.linkHome {
					os.MkdirAll(filepath.Join(root, "real-home"), 0755) //nolint:errcheck
					os.Symlink(filepath.Join(root, "real-home"), home)  //nolint:errcheck
				}
				if tt.link != "" {
					real := filepath.Join(root, "dotfiles", filepath.Base(tt.link))
					linkPath := filepath.Join(home, filepath.FromSlash(tt.link))
					os.MkdirAll(real, 0755)                   //nolint:errcheck
					os.MkdirAll(filepath.Dir(linkPath), 0755) //nolint:errcheck
					if err := os.Symlink(real, linkPath); err != nil {
						t.Fatal(err)
					}
				}
				pluginsDir := filepath.Join(home, ".config", "opencode", "plugins")
				os.MkdirAll(pluginsDir, 0755) //nolint:errcheck
				repoDir := t.TempDir()
				writeOpencodeRepo(t, repoDir, allThree...)
				recorded := installFull(t, repoDir, pluginsDir)
				os.WriteFile(filepath.Join(pluginsDir, "secret-guard.js"), []byte(legacy), 0644) //nolint:errcheck
				before := snapshot(t, pluginsDir)

				var got []string
				var err error
				out := stripANSI(captureOutput(t, func() {
					got, err = InstallOpencode(opencodeRegistry(), repoDir, pluginsDir, allThree, recorded, dryRun)
					if err == nil {
						err = CleanLegacyOpencode(pluginsDir, filepath.Join(home, ".config", "opencode", "config.json"), dryRun)
					}
				}))
				if err != nil {
					t.Fatalf("install error = %v\n%s", err, out)
				}
				after := snapshot(t, pluginsDir)
				switch {
				case tt.wantBehind:
					if !reflect.DeepEqual(after, before) {
						t.Errorf("removed through a symlinked parent:\n got %v\nwant %v\n%s", after, before, out)
					}
					if !reflect.DeepEqual(got, recorded) {
						t.Errorf("InstallOpencode() = %v, want every file left behind recorded %v", got, recorded)
					}
					if !strings.Contains(out, pluginsDir+" is behind a symlink (it resolves to ") ||
						!strings.Contains(out, "devexp never removes files through it; remove these by hand") {
						t.Errorf("no behind-a-symlink warning:\n%s", out)
					}
					if strings.Contains(out, "[dry-run] remove") {
						t.Errorf("a removal was previewed:\n%s", out)
					}
				case dryRun:
					if !reflect.DeepEqual(after, before) || !strings.Contains(out, "[dry-run] remove") {
						t.Errorf("want removals previewed, nothing changed:\n%s", out)
					}
				default:
					if len(after) != 0 || len(got) != 0 {
						t.Errorf("left %v (recorded %v), want the plugin and legacy file removed:\n%s", after, got, out)
					}
					if _, err := os.Lstat(filepath.Join(pluginsDir, "devexp")); !os.IsNotExist(err) {
						t.Errorf("empty devexp/ not pruned: %v", err)
					}
				}
			})
		}
	}
}

// TestPruneOpencodeDir_BehindSymlink: an empty devexp/ under a symlinked
// ~/.config/opencode is left alone.
func TestPruneOpencodeDir_BehindSymlink(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	os.MkdirAll(filepath.Join(root, "dotfiles", "opencode", "plugins", "devexp"), 0755) //nolint:errcheck
	os.MkdirAll(filepath.Join(home, ".config"), 0755)                                   //nolint:errcheck
	if err := os.Symlink(filepath.Join(root, "dotfiles", "opencode"), filepath.Join(home, ".config", "opencode")); err != nil {
		t.Fatal(err)
	}
	pluginsDir := filepath.Join(home, ".config", "opencode", "plugins")
	pruneOpencodeDir(pluginsDir, false)
	if _, err := os.Lstat(filepath.Join(root, "dotfiles", "opencode", "plugins", "devexp")); err != nil {
		t.Errorf("devexp/ removed through a symlinked parent: %v", err)
	}
}

// TestCheckPluginRoots_UnresolvablePlugins: a plugins/ that can't be resolved
// counts as linked, so nothing is removed from it, while writes still go ahead.
func TestCheckPluginRoots_UnresolvablePlugins(t *testing.T) {
	pluginsDir := t.TempDir()
	orig := behindSymlink
	behindSymlink = func(string, string) (string, bool, error) { return "", false, errors.New("boom") }
	t.Cleanup(func() { behindSymlink = orig })
	if linked, err := checkPluginRoots(pluginsDir); err != nil || !linked {
		t.Errorf("checkPluginRoots() = (%v, %v), want linked and no error", linked, err)
	}
}
