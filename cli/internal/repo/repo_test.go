package repo

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
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
	if _, file, _, _ := runtime.Caller(0); !filepath.IsAbs(file) {
		// Built with -trimpath: no source root is recorded.
		if got := sourceRoot(); got != "" {
			t.Errorf("sourceRoot() = %q under -trimpath, want \"\"", got)
		}
		return
	}
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

// ownedByAnother makes fileIDs report every file named name as owned by a
// different user.
func ownedByAnother(t *testing.T, name string) {
	t.Helper()
	orig := fileIDs
	fileIDs = func(fi os.FileInfo) (uint32, uint32, bool) {
		uid, gid, ok := orig(fi)
		if fi.Name() == name {
			uid = uint32(currentUID() + 1)
		}
		return uid, gid, ok
	}
	t.Cleanup(func() { fileIDs = orig })
}

// withPrivateGroup makes privateGroup report gid as the user's private group,
// or no private group when ok is false.
func withPrivateGroup(t *testing.T, gid uint32, ok bool) {
	t.Helper()
	orig := privateGroup
	privateGroup = func() (uint32, bool) { return gid, ok }
	t.Cleanup(func() { privateGroup = orig })
}

// gidOf returns the group that owns path.
func gidOf(t *testing.T, path string) uint32 {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	_, gid, ok := statIDs(fi)
	if !ok {
		t.Skip("file ownership is not available on this platform")
	}
	return gid
}

// writeIn makes a checkout and writes rel inside it with mode perm.
func writeIn(t *testing.T, rel string, perm os.FileMode) string {
	t.Helper()
	d := t.TempDir()
	makeRepoDir(t, d)
	p := filepath.Join(d, rel)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, perm); err != nil {
		t.Fatal(err)
	}
	return d
}

// checkoutIn makes a checkout at dir/name, with dir created with mode perm.
func checkoutIn(t *testing.T, perm os.FileMode, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "parent")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, perm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) }) //nolint:errcheck
	root := filepath.Join(dir, name)
	makeRepoDir(t, root)
	return root
}

// chmodIn makes a checkout and sets the mode of rel inside it.
func chmodIn(t *testing.T, rel string, perm os.FileMode) string {
	t.Helper()
	d := t.TempDir()
	makeRepoDir(t, d)
	if err := os.Chmod(filepath.Join(d, rel), perm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(d, rel), 0o755) }) //nolint:errcheck
	return d
}

// TestLookupPrivateGroup: the user's primary group counts as a private group
// only when it has the user's name.
func TestLookupPrivateGroup(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		if _, ok := lookupPrivateGroup(); ok {
			t.Errorf("lookupPrivateGroup() ok with no current user (%v)", err)
		}
		return
	}
	g, gerr := user.LookupGroupId(u.Gid)
	wantOK := gerr == nil && g.Name == u.Username
	gid, ok := lookupPrivateGroup()
	if ok != wantOK {
		t.Errorf("lookupPrivateGroup() ok = %v, want %v (user %q, primary group %v, %v)", ok, wantOK, u.Username, g, gerr)
	}
	if ok && strconv.FormatUint(uint64(gid), 10) != u.Gid {
		t.Errorf("lookupPrivateGroup() = %d, want %s", gid, u.Gid)
	}
}

// TestLocate_NoDevexpDir: without DEVEXP_DIR a dev build uses only the
// checkout it was built from, and only while that checkout verifiably is the
// user's; a tagged build uses only its bundled assets. A checkout around the
// current directory is never used, whatever it holds.
func TestLocate_NoDevexpDir(t *testing.T) {
	const (
		unmarkedWarning   = "no valid .devexp-toolkit marker file, so it was skipped"
		goneWarning       = "no longer exists"
		unverifiedWarning = "can't be verified as yours"
	)
	checkout := func(t *testing.T) string { d := t.TempDir(); makeRepoDir(t, d); return d }
	tests := map[string]struct {
		version     string
		source      func(t *testing.T) string // the build's source root
		wantSource  bool                      // RepoDir is the source root; else the bundled assets
		wantWarning string                    // substring; "" = no warning
	}{
		"dev build, its source checkout": {
			version: devBuild, source: checkout, wantSource: true,
		},
		"dev build, source checkout in a sticky shared dir": {
			version: devBuild, source: func(t *testing.T) string { return checkoutIn(t, 0o777|os.ModeSticky, "toolkit") }, wantSource: true,
		},
		"dev build, source checkout reached through a symlink you own": {
			version: devBuild,
			source: func(t *testing.T) string {
				link := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(checkout(t), link); err != nil {
					t.Fatal(err)
				}
				return link
			},
			wantSource: true,
		},
		"dev build, source checkout owned by another user": {
			version: devBuild,
			source: func(t *testing.T) string {
				ownedByAnother(t, "theirs")
				return checkoutIn(t, 0o755, "theirs")
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, source checkout whose hooks/claude-code is another user's": {
			version: devBuild, source: func(t *testing.T) string { ownedByAnother(t, "claude-code"); return checkout(t) }, wantWarning: unverifiedWarning,
		},
		"dev build, source checkout whose agents/ is another user's": {
			version: devBuild, source: func(t *testing.T) string { ownedByAnother(t, "agents"); return checkout(t) }, wantWarning: unverifiedWarning,
		},
		"dev build, source checkout writable by group": {
			version: devBuild, source: func(t *testing.T) string { return chmodIn(t, ".", 0o775) }, wantWarning: unverifiedWarning,
		},
		"dev build, source checkout whose hooks/ is writable by others": {
			version: devBuild, source: func(t *testing.T) string { return chmodIn(t, "hooks", 0o757) }, wantWarning: unverifiedWarning,
		},
		"dev build, source checkout whose mcps/ is writable by group": {
			version: devBuild, source: func(t *testing.T) string { return chmodIn(t, "mcps", 0o775) }, wantWarning: unverifiedWarning,
		},
		"dev build, source checkout with a symlinked hooks/": {
			version: devBuild,
			source: func(t *testing.T) string {
				d := checkout(t)
				os.RemoveAll(filepath.Join(d, "hooks")) //nolint:errcheck
				if err := os.Symlink(t.TempDir(), filepath.Join(d, "hooks")); err != nil {
					t.Fatal(err)
				}
				return d
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, source checkout in a dir writable by others": {
			version: devBuild, source: func(t *testing.T) string { return checkoutIn(t, 0o777, "toolkit") }, wantWarning: unverifiedWarning,
		},
		"dev build, source checkout in a dir writable by group": {
			version: devBuild, source: func(t *testing.T) string { return checkoutIn(t, 0o775, "toolkit") }, wantWarning: unverifiedWarning,
		},
		"dev build, source checkout in a dir owned by another user": {
			version: devBuild,
			source: func(t *testing.T) string {
				ownedByAnother(t, "parent")
				return checkoutIn(t, 0o755, "toolkit")
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, source checkout in a sticky dir owned by another user": {
			version: devBuild,
			source: func(t *testing.T) string {
				ownedByAnother(t, "parent")
				return checkoutIn(t, 0o777|os.ModeSticky, "toolkit")
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, source path a symlink in a dir writable by others": {
			version: devBuild,
			source: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "shared")
				os.Mkdir(dir, 0o755)                       //nolint:errcheck
				os.Chmod(dir, 0o777)                       //nolint:errcheck
				t.Cleanup(func() { os.Chmod(dir, 0o755) }) //nolint:errcheck
				link := filepath.Join(dir, "toolkit")
				if err := os.Symlink(checkout(t), link); err != nil {
					t.Fatal(err)
				}
				return link
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, source path a symlink owned by another user": {
			version: devBuild,
			source: func(t *testing.T) string {
				link := filepath.Join(t.TempDir(), "their-link")
				if err := os.Symlink(checkout(t), link); err != nil {
					t.Fatal(err)
				}
				ownedByAnother(t, "their-link")
				return link
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, source checkout writable by its user's private group": {
			version: devBuild,
			source: func(t *testing.T) string {
				d := chmodIn(t, ".", 0o775)
				withPrivateGroup(t, gidOf(t, d), true)
				return d
			},
			wantSource: true,
		},
		"dev build, hook script and hooks/ writable by the user's private group": {
			version: devBuild,
			source: func(t *testing.T) string {
				d := writeIn(t, filepath.Join("hooks", "claude-code", "guard.sh"), 0o775)
				os.Chmod(filepath.Join(d, "hooks"), 0o775) //nolint:errcheck
				withPrivateGroup(t, gidOf(t, d), true)
				return d
			},
			wantSource: true,
		},
		"dev build, source checkout writable by a shared group": {
			version: devBuild,
			source: func(t *testing.T) string {
				d := chmodIn(t, ".", 0o775)
				withPrivateGroup(t, gidOf(t, d)+1, true)
				return d
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, source checkout group-writable, private group lookup failed": {
			version: devBuild,
			source: func(t *testing.T) string {
				d := chmodIn(t, ".", 0o775)
				withPrivateGroup(t, gidOf(t, d), false)
				return d
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, source checkout writable by others, even with a private group": {
			version: devBuild,
			source: func(t *testing.T) string {
				d := chmodIn(t, ".", 0o757)
				withPrivateGroup(t, gidOf(t, d), true)
				return d
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, source checkout in a dir writable by the user's private group": {
			version: devBuild,
			source: func(t *testing.T) string {
				root := checkoutIn(t, 0o775, "toolkit")
				withPrivateGroup(t, gidOf(t, filepath.Dir(root)), true)
				return root
			},
			wantSource: true,
		},
		"dev build, source checkout in a dir writable by others, even with a private group": {
			version: devBuild,
			source: func(t *testing.T) string {
				root := checkoutIn(t, 0o777, "toolkit")
				withPrivateGroup(t, gidOf(t, filepath.Dir(root)), true)
				return root
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, a hook script writable by others": {
			version: devBuild, source: func(t *testing.T) string { return writeIn(t, filepath.Join("hooks", "claude-code", "guard.sh"), 0o757) }, wantWarning: unverifiedWarning,
		},
		"dev build, a hook script writable by a shared group": {
			version: devBuild, source: func(t *testing.T) string { return writeIn(t, filepath.Join("hooks", "claude-code", "guard.sh"), 0o775) }, wantWarning: unverifiedWarning,
		},
		"dev build, hooks registry owned by another user": {
			version: devBuild, source: func(t *testing.T) string { ownedByAnother(t, "registry.json"); return checkout(t) }, wantWarning: unverifiedWarning,
		},
		"dev build, an agent file owned by another user": {
			version: devBuild, source: func(t *testing.T) string {
				ownedByAnother(t, "a.md")
				return writeIn(t, filepath.Join("agents", "a.md"), 0o644)
			}, wantWarning: unverifiedWarning,
		},
		"dev build, a skill file that is a symlink": {
			version: devBuild,
			source: func(t *testing.T) string {
				d := checkout(t)
				target := filepath.Join(t.TempDir(), "SKILL.md")
				os.WriteFile(target, []byte("x"), 0o644) //nolint:errcheck
				if err := os.Symlink(target, filepath.Join(d, "skills", "SKILL.md")); err != nil {
					t.Fatal(err)
				}
				return d
			},
			wantWarning: unverifiedWarning,
		},
		"dev build, an MCP registry writable by group": {
			version: devBuild, source: func(t *testing.T) string { return writeIn(t, filepath.Join("mcps", "registry.json"), 0o664) }, wantWarning: unverifiedWarning,
		},
		"dev build, uninstall.sh owned by another user": {
			version: devBuild, source: func(t *testing.T) string { ownedByAnother(t, "uninstall.sh"); return writeIn(t, "uninstall.sh", 0o755) }, wantWarning: unverifiedWarning,
		},
		"dev build, devexp.config.json writable by others": {
			version: devBuild, source: func(t *testing.T) string { return writeIn(t, "devexp.config.json", 0o646) }, wantWarning: unverifiedWarning,
		},
		"dev build, marker owned by another user": {
			version: devBuild, source: func(t *testing.T) string { ownedByAnother(t, markerFile); return checkout(t) }, wantWarning: unverifiedWarning,
		},
		"dev build, file ownership unavailable": {
			version: devBuild,
			source: func(t *testing.T) string {
				orig := currentUID
				currentUID = func() int { return -1 }
				t.Cleanup(func() { currentUID = orig })
				return checkout(t)
			},
			wantWarning: "file ownership is not available",
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
				withPrivateGroup(t, 0, false) // cases opt in to a private group
				source := tt.source(t)
				withSourceRoot(t, source)

				got, err := locate(tt.version)
				if err != nil {
					t.Fatalf("locate() error = %v", err)
				}
				want := Source{RepoDir: embeddedPath(base, tt.version), Embedded: true, Origin: OriginEmbedded}
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

// embeddedPath is where the embedded assets of version go under the cache base.
func embeddedPath(base, version string) string {
	if version == devBuild {
		return filepath.Join(base, "devexp", "assets-dev")
	}
	return filepath.Join(base, "devexp", "assets")
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

		previous, _ := os.Readlink(dest)
		if _, err := extractEmbedded("v2.0.0"); err != nil {
			t.Fatalf("upgrade extractEmbedded() error = %v", err)
		}
		if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
			t.Errorf("a version change should replace the old extraction, stat err = %v", err)
		}
		assertOneCompleteTree(t, dest, "v2.0.0", previous)
	})

	t.Run("keeps the retired tree through the grace period, then sweeps it", func(t *testing.T) {
		withTempCache(t)
		dest, err := extractEmbedded("v1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		parent := filepath.Dir(dest)
		previous, _ := os.Readlink(dest)
		retired := filepath.Join(parent, previous)
		// Aged as if extracted long ago: the swap must refresh it.
		old := time.Now().Add(-3 * time.Hour)
		os.Chtimes(retired, old, old) //nolint:errcheck

		if _, err := extractEmbedded("v2.0.0"); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(filepath.Join(retired, "hooks"))
		if err != nil || !fi.IsDir() {
			t.Fatalf("the retired tree was removed at the swap: %v", err)
		}
		if info, _ := os.Stat(retired); time.Since(info.ModTime()) > time.Minute {
			t.Errorf("the retired tree's mtime was not refreshed: %v", info.ModTime())
		}
		assertOneCompleteTree(t, dest, "v2.0.0", previous)

		// Reusing the extraction sweeps too, once the grace period is over.
		if _, err := extractEmbedded("v2.0.0"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(retired); err != nil {
			t.Errorf("the retired tree was swept within the grace period: %v", err)
		}
		os.Chtimes(retired, old, old) //nolint:errcheck
		if _, err := extractEmbedded("v2.0.0"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(retired); !os.IsNotExist(err) {
			t.Errorf("the retired tree outlived the grace period: %v", err)
		}
		assertOneCompleteTree(t, dest, "v2.0.0")
	})

	t.Run("a stalled extraction whose tree was swept fails and swaps nothing in", func(t *testing.T) {
		// Each tree is removed when "a" is read, as a sweep by another run
		// would; what comes next must not recreate it: a directory, or a file.
		for name, next := range map[string]fstest.MapFile{
			"b/c/y.txt": {Data: []byte("y")},
			"z.txt":     {Data: []byte("z")},
		} {
			withTempCache(t)
			dest, err := extractEmbedded("v1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(dest, "sentinel.txt")
			os.WriteFile(sentinel, []byte("kept"), 0o644) //nolint:errcheck
			orig := extractTree
			var removed string
			extractTree = func(_ fs.FS, tree string) error {
				fsys := fstest.MapFS{"a": {Mode: fs.ModeDir | 0o755}, name: &next}
				return orig(removeOnOpen{FS: fsys, name: "a", remove: func() { removed = tree; os.RemoveAll(tree) }}, tree) //nolint:errcheck
			}
			_, err = extractEmbedded("v2.0.0")
			extractTree = orig
			if err == nil {
				t.Errorf("%s: extractEmbedded() succeeded after its tree was removed mid-extraction", name)
			}
			if removed == "" {
				t.Fatalf("%s: the extraction never reached a/", name)
			}
			if _, err := os.Stat(removed); !os.IsNotExist(err) {
				t.Errorf("%s: the removed tree was partially recreated: %v", name, err)
			}
			if _, err := os.Stat(sentinel); err != nil {
				t.Errorf("%s: the previous tree was disturbed: %v", name, err)
			}
			assertOneCompleteTree(t, dest, "v1.0.0")
		}
	})

	t.Run("a failed swap puts a directory extracted in place back", func(t *testing.T) {
		base := withTempCache(t)
		dest := embeddedPath(base, "v2.0.0")
		if err := os.MkdirAll(filepath.Join(dest, "agents"), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dest, versionFile), []byte("v1.0.0"), 0o644)   //nolint:errcheck
		os.WriteFile(filepath.Join(dest, "agents", "old.md"), []byte("x"), 0o644) //nolint:errcheck
		orig := rename
		t.Cleanup(func() { rename = orig })
		rename = func(from, to string) error {
			if to == dest && strings.HasSuffix(from, ".link") {
				return errors.New("swap failed")
			}
			return orig(from, to)
		}

		if _, err := extractEmbedded("v2.0.0"); err == nil || !strings.Contains(err.Error(), "swap failed") {
			t.Fatalf("extractEmbedded() error = %v, want the swap failure", err)
		}
		fi, err := os.Lstat(dest)
		if err != nil || !fi.IsDir() {
			t.Fatalf("%s is not the in-place directory any more: %v, %v", dest, fi, err)
		}
		if data, err := os.ReadFile(filepath.Join(dest, "agents", "old.md")); err != nil || string(data) != "x" {
			t.Errorf("the in-place tree was not put back intact: %q, %v", data, err)
		}
		if left := siblings(t, dest); len(left) != 0 {
			t.Errorf("left behind: %v", left)
		}
	})

	t.Run("dev and tagged builds extract to separate directories", func(t *testing.T) {
		base := withTempCache(t)

		tagged, err := extractEmbedded("v1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(tagged, "sentinel.txt")
		if err := os.WriteFile(sentinel, []byte("kept"), 0o644); err != nil {
			t.Fatal(err)
		}
		dev, err := extractEmbedded(devBuild)
		if err != nil {
			t.Fatal(err)
		}
		if tagged != embeddedPath(base, "v1.0.0") || dev != embeddedPath(base, devBuild) || tagged == dev {
			t.Errorf("dirs = %q (tagged), %q (dev); want %q and %q", tagged, dev, embeddedPath(base, "v1.0.0"), embeddedPath(base, devBuild))
		}
		if _, err := os.Stat(sentinel); err != nil {
			t.Errorf("a dev extraction disturbed the tagged one: %v", err)
		}
		assertOneCompleteTree(t, tagged, "v1.0.0")
		assertOneCompleteTree(t, dev, devBuild)
	})

	t.Run("an interrupted extraction leaves the previous tree intact", func(t *testing.T) {
		for _, version := range []string{devBuild, "v2.0.0"} {
			withTempCache(t)
			prev := "v1.0.0"
			if version == devBuild {
				prev = devBuild
			}
			dest, err := extractEmbedded(prev)
			if err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(dest, "sentinel.txt")
			if err := os.WriteFile(sentinel, []byte("kept"), 0o644); err != nil {
				t.Fatal(err)
			}
			orig := extractTree
			extractTree = func(fsys fs.FS, dest string) error {
				os.MkdirAll(filepath.Join(dest, "agents"), 0o755)                             //nolint:errcheck
				os.WriteFile(filepath.Join(dest, "agents", "partial.md"), []byte("x"), 0o644) //nolint:errcheck
				return errors.New("interrupted")
			}
			_, err = extractEmbedded(version)
			extractTree = orig
			if err == nil || !strings.Contains(err.Error(), "interrupted") {
				t.Fatalf("extractEmbedded(%s) error = %v, want the interruption", version, err)
			}
			if _, err := os.Stat(sentinel); err != nil {
				t.Errorf("%s: the previous tree was disturbed: %v", version, err)
			}
			assertOneCompleteTree(t, dest, prev)
		}
	})

	t.Run("replaces a directory extracted in place by an older binary", func(t *testing.T) {
		base := withTempCache(t)
		dest := embeddedPath(base, "v2.0.0")
		if err := os.MkdirAll(filepath.Join(dest, "agents"), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dest, versionFile), []byte("v1.0.0"), 0o644)   //nolint:errcheck
		os.WriteFile(filepath.Join(dest, "agents", "old.md"), []byte("x"), 0o644) //nolint:errcheck

		if got, err := extractEmbedded("v2.0.0"); err != nil || got != dest {
			t.Fatalf("extractEmbedded() = %q, %v", got, err)
		}
		if _, err := os.Stat(filepath.Join(dest, "agents", "old.md")); !os.IsNotExist(err) {
			t.Errorf("the old in-place extraction is still reachable: %v", err)
		}
		target, _ := os.Readlink(dest)
		var retired []string
		for _, n := range siblings(t, dest) {
			if n != target {
				retired = append(retired, n)
			}
		}
		if len(retired) != 1 {
			t.Fatalf("retired trees = %v, want the moved-aside directory", retired)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(dest), retired[0], "assets", "agents", "old.md")); err != nil {
			t.Errorf("the moved-aside directory was removed at the swap: %v", err)
		}
		assertOneCompleteTree(t, dest, "v2.0.0", retired...)
	})

	t.Run("removes old leftovers, keeps recent ones", func(t *testing.T) {
		base := withTempCache(t)
		dest, err := extractEmbedded("v1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		parent, name := filepath.Split(dest)
		leftover := func(n string, age time.Duration) string {
			p := filepath.Join(parent, "."+name+"."+n)
			os.MkdirAll(filepath.Join(p, "agents"), 0o755) //nolint:errcheck
			when := time.Now().Add(-age)
			os.Chtimes(p, when, when) //nolint:errcheck
			return p
		}
		old := leftover("old", 2*time.Hour)
		recent := leftover("recent", time.Minute)
		other := filepath.Join(base, "devexp", ".assets-dev.old")
		os.MkdirAll(other, 0o755)                                                     //nolint:errcheck
		os.Chtimes(other, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)) //nolint:errcheck

		if _, err := extractEmbedded("v2.0.0"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Errorf("old leftover kept: %v", err)
		}
		if _, err := os.Stat(recent); err != nil {
			t.Errorf("recent leftover removed: %v", err)
		}
		if _, err := os.Stat(other); err != nil {
			t.Errorf("another build's directory was swept: %v", err)
		}
		// The current tree is never swept, however old.
		target, _ := os.Readlink(dest)
		os.Chtimes(filepath.Join(parent, target), time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)) //nolint:errcheck
		sweepStale(parent, name)
		if !isRepoDir(dest) {
			t.Errorf("the current extraction was swept")
		}
	})

	t.Run("concurrent runs leave a complete tree", func(t *testing.T) {
		for _, version := range []string{devBuild, "v3.0.0"} {
			withTempCache(t)
			dest, err := extractEmbedded(version)
			if err != nil {
				t.Fatal(err)
			}
			if version != devBuild {
				// Force every run below to re-extract.
				os.WriteFile(filepath.Join(dest, versionFile), []byte("stale"), 0o644) //nolint:errcheck
			}
			var wg sync.WaitGroup
			errs := make(chan error, 16)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := extractEmbedded(version); err != nil {
						errs <- err
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Errorf("%s: concurrent extractEmbedded() error = %v", version, err)
			}
			if !isRepoDir(dest) {
				t.Errorf("%s: the extraction is not a complete checkout", version)
			}
			if data, err := os.ReadFile(filepath.Join(dest, versionFile)); err != nil || (version != devBuild && string(data) == "stale") {
				t.Errorf("%s: version file = %q, %v", version, data, err)
			}
			assertNoPartialTrees(t, dest)
		}
	})
}

// assertOneCompleteTree checks that dest is a symlink to a complete extraction
// of version, and that the only other extractions next to it are the retired
// trees named in kept (base names, or "*" for any number).
func assertOneCompleteTree(t *testing.T, dest, version string, kept ...string) {
	t.Helper()
	fi, err := os.Lstat(dest)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink to an extraction: %v, %v", dest, fi, err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, versionFile)); err != nil || string(data) != version {
		t.Errorf("version file = %q, %v; want %q", data, err, version)
	}
	if !isRepoDir(dest) {
		t.Errorf("%s is not a complete checkout", dest)
	}
	target, _ := os.Readlink(dest)
	for _, n := range siblings(t, dest) {
		if n != target && !slices.Contains(kept, n) {
			t.Errorf("left behind next to %s: %s", filepath.Base(dest), n)
		}
	}
}

// siblings lists the extraction directories next to dest.
func siblings(t *testing.T, dest string) []string {
	t.Helper()
	parent, name := filepath.Split(dest)
	entries, _ := os.ReadDir(parent)
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "."+name+".") {
			out = append(out, e.Name())
		}
	}
	return out
}

// removeOnOpen is an fs.FS that runs remove the first time name is opened,
// standing in for another run sweeping a stalled extraction's tree.
type removeOnOpen struct {
	fs.FS
	name   string
	remove func()
}

func (r removeOnOpen) Open(name string) (fs.File, error) {
	if name == r.name && r.remove != nil {
		r.remove()
	}
	return r.FS.Open(name)
}

// assertNoPartialTrees checks that every extraction left next to dest is
// complete: no half-written tree survives, whatever the interleaving.
func assertNoPartialTrees(t *testing.T, dest string) {
	t.Helper()
	parent, name := filepath.Split(dest)
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "."+name+".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(parent, e.Name(), versionFile)); err != nil {
			t.Errorf("partial extraction left behind: %s", e.Name())
		}
	}
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
			want := Source{RepoDir: embeddedPath(base, tt.version), Embedded: true, Origin: OriginEmbedded}

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
