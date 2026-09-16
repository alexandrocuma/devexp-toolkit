package repo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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
	tests := map[string]struct {
		dirs []string
		want bool
	}{
		"true when agents, skills, mcps all present": {
			dirs: []string{"agents", "skills", "mcps"},
			want: true,
		},
		"false when a subdir is missing": {
			dirs: []string{"agents", "skills"},
			want: false,
		},
		"false for empty directory": {
			dirs: nil,
			want: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
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

// ── extractFS ─────────────────────────────────────────────────────────────────

func TestExtractFS(t *testing.T) {
	src := fstest.MapFS{
		"top.md":                      {Data: []byte("top level")},
		"hooks/claude-code/guard.sh":  {Data: []byte("#!/bin/sh\necho hi\n")},
		"skills/deep/nested/SKILL.md": {Data: []byte("# nested skill")},
		"mcps/registry.json":          {Data: []byte("[]")},
	}

	dest := t.TempDir()
	if err := extractFS(src, dest); err != nil {
		t.Fatalf("extractFS() error = %v", err)
	}

	tests := map[string]struct {
		rel      string
		want     string
		wantPerm os.FileMode
	}{
		"top-level file": {
			rel: "top.md", want: "top level", wantPerm: 0o644,
		},
		"shell script is made executable": {
			rel:  filepath.Join("hooks", "claude-code", "guard.sh"),
			want: "#!/bin/sh\necho hi\n", wantPerm: 0o755,
		},
		"deeply nested file, parents created": {
			rel:  filepath.Join("skills", "deep", "nested", "SKILL.md"),
			want: "# nested skill", wantPerm: 0o644,
		},
		"non-script keeps 0644": {
			rel: filepath.Join("mcps", "registry.json"), want: "[]", wantPerm: 0o644,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dest, tt.rel)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("Stat(%s) error = %v", tt.rel, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", tt.rel, err)
			}
			if string(data) != tt.want {
				t.Errorf("content = %q, want %q", data, tt.want)
			}
			if perm := info.Mode().Perm(); perm != tt.wantPerm {
				t.Errorf("perm = %o, want %o", perm, tt.wantPerm)
			}
		})
	}

	t.Run("directory structure is recreated", func(t *testing.T) {
		for _, dir := range []string{"hooks", filepath.Join("hooks", "claude-code"), filepath.Join("skills", "deep", "nested")} {
			info, err := os.Stat(filepath.Join(dest, dir))
			if err != nil {
				t.Fatalf("Stat(%s) error = %v", dir, err)
			}
			if !info.IsDir() {
				t.Errorf("%s is not a directory", dir)
			}
		}
	})
}

// ── extractEmbedded ───────────────────────────────────────────────────────────

// withTempCache redirects extractEmbedded's destination into a temp dir, so a
// test never writes to the developer's real cache.
func withTempCache(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	orig := userCacheDir
	userCacheDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userCacheDir = orig })
	return base
}

func TestExtractEmbedded(t *testing.T) {
	t.Run("extracts the embedded assets and writes a version marker", func(t *testing.T) {
		base := withTempCache(t)

		dest, err := extractEmbedded("v1.2.3")
		if err != nil {
			t.Fatalf("extractEmbedded() error = %v", err)
		}
		if want := filepath.Join(base, "devexp", "assets"); dest != want {
			t.Errorf("dest = %q, want %q", dest, want)
		}
		for _, sub := range []string{"agents", "skills", "mcps", "hooks"} {
			if info, err := os.Stat(filepath.Join(dest, sub)); err != nil || !info.IsDir() {
				t.Errorf("expected %s/ to be extracted, err = %v", sub, err)
			}
		}
		marker, err := os.ReadFile(filepath.Join(dest, ".devexp-version"))
		if err != nil {
			t.Fatalf("reading version marker: %v", err)
		}
		if strings.TrimSpace(string(marker)) != "v1.2.3" {
			t.Errorf("marker = %q, want %q", marker, "v1.2.3")
		}
	})

	t.Run("reuses a prior extraction of the same version", func(t *testing.T) {
		withTempCache(t)

		dest, err := extractEmbedded("v1.0.0")
		if err != nil {
			t.Fatalf("first extractEmbedded() error = %v", err)
		}
		sentinel := filepath.Join(dest, "sentinel.txt")
		if err := os.WriteFile(sentinel, []byte("kept"), 0o644); err != nil {
			t.Fatalf("WriteFile sentinel error = %v", err)
		}

		if _, err := extractEmbedded("v1.0.0"); err != nil {
			t.Fatalf("second extractEmbedded() error = %v", err)
		}
		if _, err := os.Stat(sentinel); err != nil {
			t.Errorf("same version should reuse the extraction, but it was rebuilt: %v", err)
		}
	})

	t.Run("re-extracts when the version changes", func(t *testing.T) {
		withTempCache(t)

		dest, err := extractEmbedded("v1.0.0")
		if err != nil {
			t.Fatalf("first extractEmbedded() error = %v", err)
		}
		sentinel := filepath.Join(dest, "sentinel.txt")
		if err := os.WriteFile(sentinel, []byte("stale"), 0o644); err != nil {
			t.Fatalf("WriteFile sentinel error = %v", err)
		}

		if _, err := extractEmbedded("v2.0.0"); err != nil {
			t.Fatalf("upgrade extractEmbedded() error = %v", err)
		}
		if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
			t.Errorf("a version change should discard the old extraction, stat err = %v", err)
		}
	})
}

// TestExtractEmbedded_NoUsableCacheDir: with no cache dir, or a relative one,
// extraction is refused — never redirected to os.TempDir() (a relative $TMPDIR,
// or the shared /tmp another user can plant) and never under the cwd (#126).
func TestExtractEmbedded_NoUsableCacheDir(t *testing.T) {
	tests := map[string]struct {
		cacheDir func() (string, error)
		wantErr  string
	}{
		"the cache dir lookup fails": {
			cacheDir: func() (string, error) { return "", errors.New("$HOME is not defined") },
			wantErr:  "no user cache dir",
		},
		"the cache dir is relative": {
			cacheDir: func() (string, error) { return "relcache", nil },
			wantErr:  `user cache dir "relcache" is not an absolute path`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			orig := userCacheDir
			userCacheDir = tt.cacheDir
			t.Cleanup(func() { userCacheDir = orig })

			// A relative TMPDIR under the cwd, plus an absolute one: the old
			// fallback wrote to whichever os.TempDir() returned.
			for tname, tmp := range map[string]string{"relative TMPDIR": "tmprel", "absolute TMPDIR": t.TempDir()} {
				t.Run(tname, func(t *testing.T) {
					cwd := t.TempDir()
					t.Chdir(cwd)
					t.Setenv("TMPDIR", tmp)
					dest, err := extractEmbedded("v1.0.0")
					if err == nil || dest != "" || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "absolute path") {
						t.Errorf("extractEmbedded() = %q, %v; want a refusal containing %q", dest, err, tt.wantErr)
					}
					if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
						t.Errorf("wrote under the cwd: %v", entries)
					}
					if filepath.IsAbs(tmp) {
						if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
							t.Errorf("wrote under TMPDIR: %v", entries)
						}
					}
				})
			}
		})
	}
}

// ── Resolve dispatch ──────────────────────────────────────────────────────────

func TestResolve(t *testing.T) {
	t.Run("uses a live repo when one is found", func(t *testing.T) {
		root := t.TempDir()
		for _, sub := range []string{"agents", "skills", "mcps"} {
			if err := os.Mkdir(filepath.Join(root, sub), 0o755); err != nil {
				t.Fatalf("Mkdir(%s) error = %v", sub, err)
			}
		}
		t.Setenv("DEVEXP_DIR", root)

		got, err := Resolve("v1.0.0")
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if got.Embedded {
			t.Errorf("Embedded = true, want false when a repo dir is found")
		}
		if got.RepoDir != root {
			t.Errorf("RepoDir = %q, want %q", got.RepoDir, root)
		}
	})

	t.Run("falls back to embedded extraction when no repo is found", func(t *testing.T) {
		base := withTempCache(t)
		// DEVEXP_DIR unset and cwd under a temp dir, so the walk up finds no
		// repo — otherwise `go test` run inside this checkout would find the
		// toolkit itself and never reach the fallback.
		t.Setenv("DEVEXP_DIR", "")
		t.Chdir(t.TempDir())

		got, err := Resolve("v3.0.0")
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if !got.Embedded {
			t.Fatalf("Embedded = false, want true when no repo dir exists")
		}
		if want := filepath.Join(base, "devexp", "assets"); got.RepoDir != want {
			t.Errorf("RepoDir = %q, want %q", got.RepoDir, want)
		}
		if !isRepoDir(got.RepoDir) {
			t.Errorf("extracted dir should look like a repo dir")
		}
	})
}
