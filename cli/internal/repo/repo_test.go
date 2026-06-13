package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepoDir_DevexpDirEnv(t *testing.T) {
	t.Setenv("DEVEXP_DIR", "/some/path")

	got, err := findRepoDir()
	if err != nil {
		t.Fatalf("findRepoDir() error = %v", err)
	}
	if got != "/some/path" {
		t.Errorf("findRepoDir() = %q, want %q", got, "/some/path")
	}
}

func TestIsRepoDir(t *testing.T) {
	tests := []struct {
		name string
		dirs []string
		want bool
	}{
		{
			name: "true when agents, skills, mcps all present",
			dirs: []string{"agents", "skills", "mcps"},
			want: true,
		},
		{
			name: "false when a subdir is missing",
			dirs: []string{"agents", "skills"},
			want: false,
		},
		{
			name: "false for empty directory",
			dirs: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, sub := range tt.dirs {
				if err := os.Mkdir(filepath.Join(dir, sub), 0755); err != nil {
					t.Fatalf("Mkdir(%s) error = %v", sub, err)
				}
			}

			if got := isRepoDir(dir); got != tt.want {
				t.Errorf("isRepoDir(%s) = %v, want %v", dir, got, tt.want)
			}
		})
	}
}

func TestFindRepoDir_WalksUpToRepoRoot(t *testing.T) {
	t.Setenv("DEVEXP_DIR", "")

	root := t.TempDir()
	for _, sub := range []string{"agents", "skills", "mcps"} {
		if err := os.Mkdir(filepath.Join(root, sub), 0755); err != nil {
			t.Fatalf("Mkdir(%s) error = %v", sub, err)
		}
	}

	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", nested, err)
	}

	t.Chdir(nested)

	got, err := findRepoDir()
	if err != nil {
		t.Fatalf("findRepoDir() error = %v", err)
	}

	// Resolve symlinks (e.g. /tmp -> /private/tmp on macOS) before comparing.
	wantResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s) error = %v", root, err)
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s) error = %v", got, err)
	}

	if gotResolved != wantResolved {
		t.Errorf("findRepoDir() = %q, want %q", gotResolved, wantResolved)
	}
}
