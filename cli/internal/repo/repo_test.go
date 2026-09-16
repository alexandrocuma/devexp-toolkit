package repo

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// makeShapeDir creates dir with the toolkit's directory names — agents/,
// skills/, mcps/, and a hooks registry naming a Claude Code hook — but no
// marker file: the shape of many projects that are not a devexp-toolkit
// checkout.
func makeShapeDir(t *testing.T, dir string) {
	t.Helper()
	for _, sub := range []string{"agents", "skills", "mcps", filepath.Join("hooks", "claude-code")} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", sub, err)
		}
	}
	registry := `[{"name": "shape-hook", "enabled": true, "claude_code": {"event": "PreToolUse", "script": "hooks/claude-code/shape-hook.sh"}}]`
	if err := os.WriteFile(filepath.Join(dir, "hooks", "registry.json"), []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
}

// makeRepoDir creates a devexp-toolkit checkout: the shape plus the marker.
func makeRepoDir(t *testing.T, dir string) {
	t.Helper()
	makeShapeDir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, markerFile), []byte(markerID+"\n# comment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLocate_DevexpDir: DEVEXP_DIR is resolved to a clean absolute
// path, relative forms included (#126), and must be a devexp-toolkit checkout,
// not merely a directory of the same shape (#134) — in dev and tagged builds.
func TestLocate_DevexpDir(t *testing.T) {
	tests := map[string]struct {
		devexpDir string // relative values are relative to the test's cwd
		wantRepo  string // relative to the cwd; "" = want an error
	}{
		"absolute":              {devexpDir: "<cwd>/repo", wantRepo: "repo"},
		"absolute, uncleaned":   {devexpDir: "<cwd>/repo/../repo/", wantRepo: "repo"},
		"dot":                   {devexpDir: ".", wantRepo: "."},
		"relative":              {devexpDir: "repo", wantRepo: "repo"},
		"relative through ..":   {devexpDir: "sub/../repo", wantRepo: "repo"},
		"not a repo":            {devexpDir: "sub"},
		"does not exist":        {devexpDir: "missing"},
		"absolute, not a repo":  {devexpDir: "<cwd>/sub"},
		"same shape, no marker": {devexpDir: "shape"},
		"inside a checkout":     {devexpDir: "repo/agents"},
	}
	for name, tt := range tests {
		for _, version := range []string{devBuild, "v1.0.0"} {
			t.Run(name+", "+version, func(t *testing.T) {
				cwd, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				if tt.wantRepo == "." {
					makeRepoDir(t, cwd)
				}
				makeRepoDir(t, filepath.Join(cwd, "repo"))
				makeShapeDir(t, filepath.Join(cwd, "shape"))
				os.MkdirAll(filepath.Join(cwd, "sub"), 0o755) //nolint:errcheck
				t.Chdir(cwd)
				withSourceRoot(t, "")
				withTempCache(t)
				t.Setenv("DEVEXP_DIR", strings.ReplaceAll(tt.devexpDir, "<cwd>", cwd))

				got, err := locate(version)
				if tt.wantRepo == "" {
					if err == nil || got != (Source{}) || !strings.Contains(err.Error(), "not a devexp-toolkit checkout") {
						t.Errorf("locate() = %+v, %v; want a not-a-checkout error", got, err)
					}
					return
				}
				if want := (Source{RepoDir: filepath.Join(cwd, tt.wantRepo), Origin: OriginDevexpDir}); err != nil || got != want {
					t.Errorf("locate() = %+v, %v; want %+v", got, err, want)
				}
			})
		}
	}
}

// TestIsRepoDir: only a directory with the marker file — a regular file whose
// first line is the toolkit's id — and agents/, skills/, mcps/ is a checkout.
func TestIsRepoDir(t *testing.T) {
	marker := func(content string) func(t *testing.T, dir string) {
		return func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, markerFile), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	tests := map[string]struct {
		dirs   []string
		marker func(t *testing.T, dir string)
		want   bool
	}{
		"true with the marker and agents, skills, mcps": {
			dirs: []string{"agents", "skills", "mcps"}, marker: marker("devexp-toolkit\n# note\n"), want: true,
		},
		"true with a marker without a trailing newline": {
			dirs: []string{"agents", "skills", "mcps"}, marker: marker("devexp-toolkit"), want: true,
		},
		"true with a CRLF marker": {
			dirs: []string{"agents", "skills", "mcps"}, marker: marker("devexp-toolkit\r\n"), want: true,
		},
		"false for the same shape without the marker": {
			dirs: []string{"agents", "skills", "mcps", "hooks"}, want: false,
		},
		"false when the marker names something else": {
			dirs: []string{"agents", "skills", "mcps"}, marker: marker("devexp-toolkit-fork\n"), want: false,
		},
		"false when the id is not the first line": {
			dirs: []string{"agents", "skills", "mcps"}, marker: marker("# note\ndevexp-toolkit\n"), want: false,
		},
		"false for an empty marker": {
			dirs: []string{"agents", "skills", "mcps"}, marker: marker(""), want: false,
		},
		"false for an oversized marker": {
			dirs: []string{"agents", "skills", "mcps"}, marker: marker("devexp-toolkit\n" + strings.Repeat("#", 5000)), want: false,
		},
		"false when the marker is a directory": {
			dirs: []string{"agents", "skills", "mcps", markerFile}, want: false,
		},
		"false when the marker is a symlink to a real marker": {
			dirs: []string{"agents", "skills", "mcps"},
			marker: func(t *testing.T, dir string) {
				real := filepath.Join(t.TempDir(), markerFile)
				if err := os.WriteFile(real, []byte(markerID+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, filepath.Join(dir, markerFile)); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		"false with the marker when a subdir is missing": {
			dirs: []string{"agents", "skills"}, marker: marker("devexp-toolkit\n"), want: false,
		},
		"false for empty directory": {
			dirs: nil, want: false,
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
			if tt.marker != nil {
				tt.marker(t, dir)
			}

			if got := isRepoDir(dir); got != tt.want {
				t.Errorf("isRepoDir(%s) = %v, want %v", dir, got, tt.want)
			}
		})
	}
}

// withSourceRoot makes sourceRoot report root, standing in for a dev binary
// compiled from that checkout ("" for a -trimpath build).
func withSourceRoot(t *testing.T, root string) {
	t.Helper()
	orig := sourceRoot
	sourceRoot = func() string { return root }
	t.Cleanup(func() { sourceRoot = orig })
}

func TestSourceRootOf(t *testing.T) {
	tests := map[string]struct {
		file string
		want string
	}{
		"absolute path in a checkout": {file: "/src/devexp-toolkit/cli/internal/repo/repo.go", want: "/src/devexp-toolkit"},
		"-trimpath, module-relative":  {file: "devexp/internal/repo/repo.go"},
		"relative, repo layout":       {file: "cli/internal/repo/repo.go"},
		"relative, below a directory": {file: "src/devexp-toolkit/cli/internal/repo/repo.go"},
		"absolute, another file":      {file: "/src/devexp-toolkit/cli/internal/repo/other.go"},
		"absolute, not at a segment":  {file: "/src/mycli/internal/repo/repo.go"},
		"empty":                       {file: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := sourceRootOf(filepath.FromSlash(tt.file)); got != filepath.FromSlash(tt.want) {
				t.Errorf("sourceRootOf(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

// TestSourceRoot_RealCheckout: `go test` compiles this package from this
// repository, so its source root is the repository root, and the committed
// marker makes it a checkout.
func TestSourceRoot_RealCheckout(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceRoot(); got != root {
		t.Errorf("sourceRoot() = %q, want %q", got, root)
	}
	if !isRepoDir(root) {
		t.Errorf("isRepoDir(%s) = false; the repository root must be a checkout", root)
	}
}

// TestLocate_NoDevexpDir: without DEVEXP_DIR a dev build uses only the
// checkout it was built from, and a tagged build only its bundled assets. A
// checkout around the current directory is never used, whatever it holds.
func TestLocate_NoDevexpDir(t *testing.T) {
	const (
		unmarkedWarning = "no valid .devexp-toolkit marker file, so it was skipped"
		goneWarning     = "no longer exists"
	)
	tests := map[string]struct {
		version     string
		source      func(t *testing.T) string // the build's source root
		wantSource  bool                      // RepoDir is the source root; else the bundled assets
		wantWarning string                    // substring; "" = no warning
	}{
		"dev build, its source checkout": {
			version: devBuild, source: func(t *testing.T) string { d := t.TempDir(); makeRepoDir(t, d); return d }, wantSource: true,
		},
		"dev build, no source root (-trimpath)": {
			version: devBuild, source: func(*testing.T) string { return "" },
		},
		"dev build, source checkout without the marker": {
			version: devBuild, source: func(t *testing.T) string { d := t.TempDir(); makeShapeDir(t, d); return d }, wantWarning: unmarkedWarning,
		},
		"dev build, source checkout with an invalid marker": {
			version: devBuild,
			source: func(t *testing.T) string {
				d := t.TempDir()
				makeShapeDir(t, d)
				os.WriteFile(filepath.Join(d, markerFile), []byte("other\n"), 0o644) //nolint:errcheck
				return d
			},
			wantWarning: unmarkedWarning,
		},
		"dev build, source checkout gone": {
			version: devBuild, source: func(t *testing.T) string { return filepath.Join(t.TempDir(), "moved") }, wantWarning: goneWarning,
		},
		"dev build, source root not laid out as a checkout": {
			version: devBuild, source: func(t *testing.T) string { return t.TempDir() },
		},
		"tagged build, a source checkout": {
			version: "v1.0.0", source: func(t *testing.T) string { d := t.TempDir(); makeRepoDir(t, d); return d },
		},
		"tagged build, no source root": {
			version: "v1.0.0", source: func(*testing.T) string { return "" },
		},
	}
	// Where the binary is run from: inside another checkout, including its
	// bin/ (as for a binary copied or linked there), or anywhere else.
	cwds := map[string]func(t *testing.T) string{
		"run in another checkout's bin/": func(t *testing.T) string {
			d := t.TempDir()
			makeRepoDir(t, d)
			os.MkdirAll(filepath.Join(d, "bin"), 0o755) //nolint:errcheck
			return filepath.Join(d, "bin")
		},
		"run below another checkout": func(t *testing.T) string {
			d := t.TempDir()
			makeRepoDir(t, d)
			return filepath.Join(d, "agents")
		},
		"run at another checkout's root": func(t *testing.T) string {
			d := t.TempDir()
			makeRepoDir(t, d)
			return d
		},
		"run outside any checkout": func(t *testing.T) string { return t.TempDir() },
	}
	for name, tt := range tests {
		for cname, cwd := range cwds {
			t.Run(name+", "+cname, func(t *testing.T) {
				base := withTempCache(t)
				t.Setenv("DEVEXP_DIR", "")
				t.Chdir(cwd(t))
				source := tt.source(t)
				withSourceRoot(t, source)

				got, err := locate(tt.version)
				if err != nil {
					t.Fatalf("locate() error = %v", err)
				}
				want := Source{RepoDir: filepath.Join(base, "devexp", "assets"), Embedded: true, Origin: OriginEmbedded}
				if tt.wantSource {
					want = Source{RepoDir: source, Origin: OriginSourceDir}
				}
				warning := got.Warning
				got.Warning = ""
				if got != want {
					t.Errorf("locate() = %+v, want %+v", got, want)
				}
				if tt.wantWarning == "" && warning != "" || tt.wantWarning != "" && (!strings.Contains(warning, tt.wantWarning) || !strings.Contains(warning, source)) {
					t.Errorf("warning = %q, want one containing %q and %q", warning, tt.wantWarning, source)
				}
				if entries, _ := os.ReadDir(base); len(entries) != 0 {
					t.Errorf("locate wrote to the cache: %v", entries)
				}
			})
		}
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
		if !isRepoDir(dest) {
			t.Errorf("the extracted assets are not a devexp-toolkit checkout (missing %s?)", markerFile)
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

	t.Run("a dev build never reuses an extraction", func(t *testing.T) {
		withTempCache(t)

		dest, err := extractEmbedded(devBuild)
		if err != nil {
			t.Fatalf("first extractEmbedded() error = %v", err)
		}
		sentinel := filepath.Join(dest, "sentinel.txt")
		if err := os.WriteFile(sentinel, []byte("stale"), 0o644); err != nil {
			t.Fatalf("WriteFile sentinel error = %v", err)
		}

		if _, err := extractEmbedded(devBuild); err != nil {
			t.Fatalf("second extractEmbedded() error = %v", err)
		}
		if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
			t.Errorf("a dev build reused its previous extraction, stat err = %v", err)
		}
		if !isRepoDir(dest) {
			t.Errorf("the re-extracted assets are not a devexp-toolkit checkout")
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
// or the shared /tmp) and never under the cwd (#126).
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
	t.Run("uses a checkout named by DEVEXP_DIR, dev and tagged builds alike", func(t *testing.T) {
		for _, version := range []string{devBuild, "v1.0.0"} {
			base := withTempCache(t)
			root := t.TempDir()
			makeRepoDir(t, root)
			t.Setenv("DEVEXP_DIR", root)

			var announced []Source
			got, err := Resolve(version, func(s Source) { announced = append(announced, s) })
			if err != nil {
				t.Fatalf("Resolve(%s) error = %v", version, err)
			}
			want := Source{RepoDir: root, Origin: OriginDevexpDir}
			if got != want {
				t.Errorf("Resolve(%s) = %+v, want %+v", version, got, want)
			}
			if !reflect.DeepEqual(announced, []Source{want}) {
				t.Errorf("announced %+v, want exactly %+v", announced, want)
			}
			if entries, _ := os.ReadDir(base); len(entries) != 0 {
				t.Errorf("extracted the embedded assets anyway: %v", entries)
			}
		}
	})

	notACheckout := map[string]func(t *testing.T) string{
		"an empty dir":              func(t *testing.T) string { return t.TempDir() },
		"the same shape, no marker": func(t *testing.T) string { d := t.TempDir(); makeShapeDir(t, d); return d },
	}
	for name, dir := range notACheckout {
		t.Run("a DEVEXP_DIR that is "+name+" is an error, with no fallback", func(t *testing.T) {
			base := withTempCache(t)
			t.Setenv("DEVEXP_DIR", dir(t))
			t.Chdir(t.TempDir())

			announced := false
			got, err := Resolve("v3.0.0", func(Source) { announced = true })
			if err == nil || got != (Source{}) || !strings.Contains(err.Error(), "not a devexp-toolkit checkout") {
				t.Errorf("Resolve() = %+v, %v; want a not-a-checkout error", got, err)
			}
			if announced {
				t.Errorf("announced an asset root for a refused DEVEXP_DIR")
			}
			if entries, _ := os.ReadDir(base); len(entries) != 0 {
				t.Errorf("extracted the embedded assets anyway: %v", entries)
			}
		})
	}

	t.Run("a relative DEVEXP_DIR resolves to an absolute RepoDir", func(t *testing.T) {
		cwd, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		makeRepoDir(t, filepath.Join(cwd, "repo"))
		t.Chdir(cwd)
		t.Setenv("DEVEXP_DIR", "sub/../repo")

		got, err := Resolve("v1.0.0", nil)
		if want := filepath.Join(cwd, "repo"); err != nil || got.RepoDir != want || got.Embedded {
			t.Errorf("Resolve() = %+v, %v; want RepoDir %q", got, err, want)
		}
	})

	t.Run("a relative RepoDir is refused", func(t *testing.T) {
		if got, err := absoluteSource(Source{RepoDir: "repo"}); err == nil || got != (Source{}) {
			t.Errorf("absoluteSource(repo) = %+v, %v; want an error", got, err)
		}
		if got, err := absoluteSource(Source{RepoDir: "/repo", Embedded: true}); err != nil || got.RepoDir != "/repo" || !got.Embedded {
			t.Errorf("absoluteSource(/repo) = %+v, %v; want it unchanged", got, err)
		}
	})

	// fallbacks: where the embedded assets are used. The cwd is always inside
	// a checkout, which is never used.
	fallbacks := map[string]struct {
		version string
		source  func(t *testing.T) string
		warning string
	}{
		"tagged build": {
			version: "v3.0.0",
			source:  func(t *testing.T) string { d := t.TempDir(); makeRepoDir(t, d); return d },
		},
		"dev build without a source root": {
			version: devBuild,
			source:  func(*testing.T) string { return "" },
		},
		"dev build whose source checkout lacks the marker": {
			version: devBuild,
			source:  func(t *testing.T) string { d := t.TempDir(); makeShapeDir(t, d); return d },
			warning: "no valid .devexp-toolkit marker file",
		},
	}
	for name, tt := range fallbacks {
		t.Run("falls back to the embedded assets: "+name, func(t *testing.T) {
			base := withTempCache(t)
			withSourceRoot(t, tt.source(t))
			t.Setenv("DEVEXP_DIR", "")
			cwd := t.TempDir()
			makeRepoDir(t, cwd)
			t.Chdir(cwd)
			want := Source{RepoDir: filepath.Join(base, "devexp", "assets"), Embedded: true, Origin: OriginEmbedded}

			calls := 0
			got, err := Resolve(tt.version, func(s Source) {
				calls++
				if !strings.Contains(s.Warning, tt.warning) || (tt.warning == "") != (s.Warning == "") {
					t.Errorf("announced warning %q, want %q", s.Warning, tt.warning)
				}
				s.Warning = ""
				if s != want {
					t.Errorf("announced %+v, want %+v", s, want)
				}
				// Announced before anything is written.
				if entries, _ := os.ReadDir(base); len(entries) != 0 {
					t.Errorf("wrote before announcing the asset root: %v", entries)
				}
			})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			got.Warning = ""
			if got != want || calls != 1 {
				t.Errorf("Resolve() = %+v after %d announcements, want %+v after 1", got, calls, want)
			}
			if !isRepoDir(got.RepoDir) {
				t.Errorf("extracted dir should be a devexp-toolkit checkout")
			}
		})
	}
}
