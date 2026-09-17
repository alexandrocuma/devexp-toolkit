package removeguard

import (
	"os"
	"path/filepath"
	"testing"
)

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, oldname, newname string) {
	t.Helper()
	mkdir(t, filepath.Dir(newname))
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatal(err)
	}
}

// TestBehindSymlink builds each layout in a fresh scratch root. On macOS that
// root is under /var, itself a symlink to /private/var, so every case also
// checks that a link above home doesn't count.
func TestBehindSymlink(t *testing.T) {
	tests := map[string]struct {
		setup      func(t *testing.T, root string) (home, dir string)
		wantBehind bool
	}{
		"a real directory": {setup: func(t *testing.T, root string) (string, string) {
			home := filepath.Join(root, "home")
			mkdir(t, filepath.Join(home, ".claude", "agents"))
			return home, filepath.Join(home, ".claude", "agents")
		}},
		"home itself behind a symlink": {setup: func(t *testing.T, root string) (string, string) {
			mkdir(t, filepath.Join(root, "real-home", ".claude", "agents"))
			symlink(t, filepath.Join(root, "real-home"), filepath.Join(root, "home"))
			home := filepath.Join(root, "home")
			return home, filepath.Join(home, ".claude", "agents")
		}},
		"the directory is a symlink": {wantBehind: true, setup: func(t *testing.T, root string) (string, string) {
			home := filepath.Join(root, "home")
			mkdir(t, filepath.Join(root, "dotfiles", "agents"))
			symlink(t, filepath.Join(root, "dotfiles", "agents"), filepath.Join(home, ".claude", "agents"))
			return home, filepath.Join(home, ".claude", "agents")
		}},
		"a linked ~/.claude": {wantBehind: true, setup: func(t *testing.T, root string) (string, string) {
			home := filepath.Join(root, "home")
			mkdir(t, filepath.Join(root, "dotfiles", "claude", "agents"))
			symlink(t, filepath.Join(root, "dotfiles", "claude"), filepath.Join(home, ".claude"))
			return home, filepath.Join(home, ".claude", "agents")
		}},
		"a linked ~/.config/opencode": {wantBehind: true, setup: func(t *testing.T, root string) (string, string) {
			home := filepath.Join(root, "home")
			mkdir(t, filepath.Join(root, "dotfiles", "opencode", "plugins"))
			symlink(t, filepath.Join(root, "dotfiles", "opencode"), filepath.Join(home, ".config", "opencode"))
			return home, filepath.Join(home, ".config", "opencode", "plugins")
		}},
		"a relative link that stays in place": {setup: func(t *testing.T, root string) (string, string) {
			home := filepath.Join(root, "home")
			mkdir(t, filepath.Join(home, ".claude", "agents"))
			symlink(t, ".claude", filepath.Join(home, "claude-alias"))
			return home, filepath.Join(home, ".claude", "agents")
		}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			home, dir := tt.setup(t, t.TempDir())
			resolved, behind, err := BehindSymlink(home, dir)
			if err != nil {
				t.Fatalf("BehindSymlink() error = %v", err)
			}
			if behind != tt.wantBehind {
				t.Errorf("BehindSymlink() behind = %v (resolved %s), want %v", behind, resolved, tt.wantBehind)
			}
		})
	}
}

func TestBehindSymlink_Errors(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "home"))
	for name, dir := range map[string]string{
		"a directory outside home": filepath.Join(root, "elsewhere"),
		"home's parent":            root,
		"a missing directory":      filepath.Join(root, "home", ".claude", "agents"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, behind, err := BehindSymlink(filepath.Join(root, "home"), dir); err == nil || behind {
				t.Errorf("BehindSymlink(%s) = (%v, %v), want an error", dir, behind, err)
			}
		})
	}
}

func TestIsSymlink(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "dir"))
	symlink(t, filepath.Join(root, "dir"), filepath.Join(root, "link"))
	symlink(t, filepath.Join(root, "nowhere"), filepath.Join(root, "dangling"))
	for path, want := range map[string]bool{
		filepath.Join(root, "dir"):      false,
		filepath.Join(root, "link"):     true,
		filepath.Join(root, "dangling"): true,
		filepath.Join(root, "missing"):  false,
	} {
		if got := IsSymlink(path); got != want {
			t.Errorf("IsSymlink(%s) = %v, want %v", path, got, want)
		}
	}
}
