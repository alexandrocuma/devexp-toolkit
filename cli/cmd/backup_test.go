package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"devexp/internal/config"
	"devexp/internal/manifest"
)

// ── Stale removal and symlinked target directories (#128) ────────────────────

// staleLayout is where one install target keeps its agents, skills and
// manifest, and how a skill looks on disk there.
type staleLayout struct {
	manifest, agents, skills string
	staleSkill               string // manifest entry of a stale skill
	staleSkillDisk           string // relative to skills: what that entry names on disk
	staleSkillFile           string // relative to skills: the file that makes it a skill
	installedSkillFile       string // relative to skills: the skill this run installs
}

// staleTargets drives the Claude Code and opencode installs, which share
// removeStale but differ in how a skill is recorded and laid out.
func staleTargets(t *testing.T) map[string]struct {
	install func(*installOpts) error
	layout  func(home string) staleLayout
} {
	return map[string]struct {
		install func(*installOpts) error
		layout  func(home string) staleLayout
	}{
		"claude": {doInstallClaude, func(home string) staleLayout {
			p := testClaudePaths(t, home)
			return staleLayout{manifest: p.manifest, agents: p.agents, skills: p.skills,
				staleSkill: "old-skill", staleSkillDisk: "old-skill",
				staleSkillFile: filepath.Join("old-skill", "SKILL.md"), installedSkillFile: filepath.Join("graphify", "SKILL.md")}
		}},
		"opencode": {doInstallOpencode, func(home string) staleLayout {
			p := testOpencodePaths(t, home)
			return staleLayout{manifest: p.manifest, agents: p.agents, skills: p.skills,
				staleSkill: "old-cmd", staleSkillDisk: "old-cmd.md",
				staleSkillFile: "old-cmd.md", installedSkillFile: "graphify.md"}
		}},
	}
}

// writeStaleRepo is writeOpencodeHookRepo plus one agent and one skill.
func writeStaleRepo(t *testing.T) string {
	t.Helper()
	repoDir := writeOpencodeHookRepo(t)
	for rel, content := range map[string]string{
		"agents/dev-agent.md":      "---\nname: dev-agent\n---\nbody\n",
		"skills/graphify/SKILL.md": "---\nname: graphify\n---\nbody\n",
	} {
		writeTestFile(t, filepath.Join(repoDir, filepath.FromSlash(rel)), content)
	}
	return repoDir
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}

func writeTestManifest(t *testing.T, path string, agents, skills []string) {
	t.Helper()
	data, err := json.Marshal(map[string][]string{"agents": agents, "skills": skills})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, string(data))
}

func runStaleInstall(t *testing.T, install func(*installOpts) error, repoDir string) string {
	t.Helper()
	var err error
	out := captureStdout(t, func() {
		err = install(&installOpts{repoDir: repoDir, cfg: &config.Config{}, env: map[string]string{}})
	})
	if err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}
	return out
}

func loadTestManifest(t *testing.T, path string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("manifest.Load error = %v", err)
	}
	return m
}

// TestDoInstall_SymlinkedTargetDirs: agents/, skills/ and commands/ linked
// from a dotfiles tree are written through, but no stale entry is removed
// through them. Each one left behind is listed in a warning and stays in the
// manifest, and a run after the links are replaced with real directories
// removes them.
func TestDoInstall_SymlinkedTargetDirs(t *testing.T) {
	for tname, tg := range staleTargets(t) {
		t.Run(tname, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			fakeCLI(t)
			repoDir := writeStaleRepo(t)
			l := tg.layout(home)

			dotfiles := filepath.Join(t.TempDir(), "dotfiles")
			linked := map[string]string{l.agents: filepath.Join(dotfiles, "agents"), l.skills: filepath.Join(dotfiles, "skills")}
			writeTestFile(t, filepath.Join(linked[l.agents], "old.md"), "user data\n")
			writeTestFile(t, filepath.Join(linked[l.skills], l.staleSkillFile), "user data\n")
			if tname == "claude" {
				writeTestFile(t, filepath.Join(linked[l.skills], "old-skill", "notes.md"), "user notes\n")
			}
			for link, dir := range linked {
				os.MkdirAll(filepath.Dir(link), 0o755) //nolint:errcheck
				mustSymlink(t, dir, link)
			}
			writeTestManifest(t, l.manifest, []string{"old.md"}, []string{l.staleSkill})
			before := treeBytes(t, dotfiles)

			out := runStaleInstall(t, tg.install, repoDir)

			after := treeBytes(t, dotfiles)
			for rel, content := range before {
				if after[rel] != content {
					t.Errorf("%s removed or changed through the symlinked directory (now %q)", rel, after[rel])
				}
			}
			if t.Failed() {
				t.Logf("install output:\n%s", out)
			}
			for _, p := range []string{filepath.Join(linked[l.agents], "dev-agent.md"), filepath.Join(linked[l.skills], l.installedSkillFile)} {
				if _, err := os.Stat(p); err != nil {
					t.Errorf("install did not write through the link: %v", err)
				}
			}
			staleOnDisk := map[string]string{
				l.agents: filepath.Join(l.agents, "old.md"),
				l.skills: filepath.Join(l.skills, l.staleSkillDisk),
			}
			for dir, path := range staleOnDisk {
				want := fmt.Sprintf("%q is a symlink — devexp never removes files through it; remove these by hand: %q\n", dir, path)
				if !strings.Contains(out, want) {
					t.Errorf("no warning %q:\n%s", want, out)
				}
				if fi, err := os.Lstat(dir); err != nil || fi.Mode()&os.ModeSymlink == 0 {
					t.Errorf("%s is no longer a symlink", dir)
				}
			}
			m := loadTestManifest(t, l.manifest)
			if want := []string{"dev-agent.md", "old.md"}; !reflect.DeepEqual(m.Agents, want) {
				t.Errorf("manifest agents = %v, want %v (the stale agent left behind stays recorded)", m.Agents, want)
			}
			if want := []string{"graphify", l.staleSkill}; !reflect.DeepEqual(m.Skills, want) {
				t.Errorf("manifest skills = %v, want %v (the stale skill left behind stays recorded)", m.Skills, want)
			}

			// The user replaces each link with the directory it pointed at.
			for link, dir := range linked {
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(dir, link); err != nil {
					t.Fatal(err)
				}
			}
			out = runStaleInstall(t, tg.install, repoDir)

			for _, path := range staleOnDisk {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Errorf("%s not removed once the directory is real, stat err = %v\n%s", path, err, out)
				}
			}
			m = loadTestManifest(t, l.manifest)
			if !reflect.DeepEqual(m.Agents, []string{"dev-agent.md"}) || !reflect.DeepEqual(m.Skills, []string{"graphify"}) {
				t.Errorf("manifest = %+v, want only what this run installed", m)
			}
		})
	}
}

// TestDoInstall_StaleRemovalRealDirs: with real target directories, stale
// removal is unchanged. A valid stale entry is removed and dropped from the
// manifest; a symlinked entry pointing at the user's own data, and an entry
// naming a path outside the directory, remove nothing and aren't recorded.
func TestDoInstall_StaleRemovalRealDirs(t *testing.T) {
	for tname, tg := range staleTargets(t) {
		t.Run(tname, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			fakeCLI(t)
			repoDir := writeStaleRepo(t)
			l := tg.layout(home)

			notes := filepath.Join(home, "notes")
			writeTestFile(t, filepath.Join(notes, "linked.md"), "user data\n")
			writeTestFile(t, filepath.Join(notes, "linked-skill", "SKILL.md"), "user data\n")
			writeTestFile(t, filepath.Join(l.agents, "old.md"), "old\n")
			writeTestFile(t, filepath.Join(l.skills, l.staleSkillFile), "old\n")
			mustSymlink(t, filepath.Join(notes, "linked.md"), filepath.Join(l.agents, "linked.md"))
			// A skill entry that is a symlink: a directory for Claude Code, a
			// command file for opencode.
			linkedSkill, linkedSkillDisk, linkTarget := "linked-skill", "linked-skill", filepath.Join(notes, "linked-skill")
			if tname == "opencode" {
				linkedSkill, linkedSkillDisk, linkTarget = "linked-cmd", "linked-cmd.md", filepath.Join(notes, "linked.md")
			}
			mustSymlink(t, linkTarget, filepath.Join(l.skills, linkedSkillDisk))
			writeTestManifest(t, l.manifest,
				[]string{"old.md", "linked.md", relPath(t, l.agents, filepath.Join(notes, "linked.md"))},
				[]string{l.staleSkill, linkedSkill, relPath(t, l.skills, filepath.Join(notes, "linked-skill"))})
			before := treeBytes(t, notes)

			out := runStaleInstall(t, tg.install, repoDir)

			if after := treeBytes(t, notes); !reflect.DeepEqual(after, before) {
				t.Errorf("user data changed:\n got %v\nwant %v\n%s", after, before, out)
			}
			for _, p := range []string{filepath.Join(l.agents, "old.md"), filepath.Join(l.skills, l.staleSkillDisk)} {
				if _, err := os.Lstat(p); !os.IsNotExist(err) {
					t.Errorf("valid stale entry %s not removed, stat err = %v\n%s", p, err, out)
				}
			}
			for _, p := range []string{filepath.Join(l.agents, "linked.md"), filepath.Join(l.skills, linkedSkillDisk)} {
				if fi, err := os.Lstat(p); err != nil || fi.Mode()&os.ModeSymlink == 0 {
					t.Errorf("symlinked entry %s removed, err = %v", p, err)
				}
			}
			if strings.Contains(out, "never removes files through it") {
				t.Errorf("a real directory was reported as a symlink:\n%s", out)
			}
			m := loadTestManifest(t, l.manifest)
			if !reflect.DeepEqual(m.Agents, []string{"dev-agent.md"}) || !reflect.DeepEqual(m.Skills, []string{"graphify"}) {
				t.Errorf("manifest = %+v, want only what this run installed: nothing kept here is devexp's", m)
			}
		})
	}
}

// TestRemoveStale_SymlinkedTargetDir: through a target directory that is a
// symlink nothing is removed or previewed as removed, in a real run or a dry
// run. The stale entries still there are listed in one warning and returned
// for the manifest; one already gone is dropped, and an entry that isn't
// devexp's (a symlink) is warned about as usual and not returned.
func TestRemoveStale_SymlinkedTargetDir(t *testing.T) {
	tests := map[string]struct {
		shape    staleShape
		removeFn func(string) error
		stale    []string // present in the link target
	}{
		"agent files":       {shape: staleFile, removeFn: os.Remove, stale: []string{"old.md", "older.md"}},
		"skill directories": {shape: staleDir, removeFn: os.RemoveAll, stale: []string{"old-skill", "older-skill"}},
		"opencode commands": {shape: staleCommand, removeFn: os.Remove, stale: []string{"old-cmd", "older-cmd"}},
	}
	for name, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s dryRun=%v", name, dryRun), func(t *testing.T) {
				root := t.TempDir()
				dotfiles := filepath.Join(root, "dotfiles")
				target := filepath.Join(root, "home", "target")
				var listed []string
				for _, e := range tt.stale {
					p := filepath.Join(dotfiles, onDisk(e, tt.shape))
					if tt.shape == staleDir {
						writeTestFile(t, filepath.Join(p, "SKILL.md"), "user data\n")
					} else {
						writeTestFile(t, p, "user data\n")
					}
					listed = append(listed, fmt.Sprintf("%q", filepath.Join(target, onDisk(e, tt.shape))))
				}
				writeTestFile(t, filepath.Join(root, "elsewhere.md"), "user data\n")
				linkedEntry := "linked.md"
				if tt.shape != staleFile {
					linkedEntry = "linked"
				}
				mustSymlink(t, filepath.Join(root, "elsewhere.md"), filepath.Join(dotfiles, onDisk(linkedEntry, tt.shape)))
				os.MkdirAll(filepath.Dir(target), 0o755) //nolint:errcheck
				mustSymlink(t, dotfiles, target)
				before := treeBytes(t, root)
				var removed []string
				removeFn := func(path string) error {
					removed = append(removed, path)
					return tt.removeFn(path)
				}
				gone := "gone"
				if tt.shape == staleFile {
					gone = "gone.md"
				}
				entries := append(append([]string{}, tt.stale...), gone, linkedEntry)

				var kept []string
				out := captureStdout(t, func() { kept = removeStale(target, entries, nil, tt.shape, removeFn, dryRun) })

				if len(removed) > 0 {
					t.Errorf("removed %v through a symlinked directory\n%s", removed, out)
				}
				if after := treeBytes(t, root); !reflect.DeepEqual(after, before) {
					t.Errorf("tree changed:\n got %v\nwant %v\n%s", after, before, out)
				}
				if !isSymlink(target) {
					t.Errorf("target is no longer a symlink")
				}
				if strings.Contains(out, "[dry-run]") {
					t.Errorf("a removal through the symlink was previewed:\n%s", out)
				}
				want := fmt.Sprintf("%q is a symlink — devexp never removes files through it; remove these by hand: %s\n", target, strings.Join(listed, ", "))
				if strings.Count(out, "is a symlink — devexp never removes files through it") != 1 || !strings.Contains(out, want) {
					t.Errorf("want exactly one warning %q:\n%s", want, out)
				}
				if !strings.Contains(out, fmt.Sprintf("%q left untouched: no longer in this release, but a symlink", filepath.Join(target, onDisk(linkedEntry, tt.shape)))) {
					t.Errorf("no warning for the symlinked entry:\n%s", out)
				}
				if !reflect.DeepEqual(kept, tt.stale) {
					t.Errorf("kept = %v, want %v", kept, tt.stale)
				}
			})
		}
	}
}

// TestRemoveStale_Kept: removeStale returns only what the manifest should go
// on recording. Removed, already gone and not-devexp's entries are dropped;
// an entry that can't be checked or removed is kept for a later run.
func TestRemoveStale_Kept(t *testing.T) {
	t.Run("removed, gone and rejected entries are dropped", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "agents")
		writeTestFile(t, filepath.Join(dir, "old.md"), "old\n")
		writeTestFile(t, filepath.Join(root, "user.md"), "user data\n")
		mustSymlink(t, filepath.Join(root, "user.md"), filepath.Join(dir, "linked.md"))
		writeTestFile(t, filepath.Join(dir, "new.md"), "new\n")

		var kept []string
		out := captureStdout(t, func() {
			kept = removeStale(dir, []string{"old.md", "gone.md", "linked.md", "../user.md", "NEW.md", "new.md"}, []string{"new.md"}, staleFile, os.Remove, false)
		})
		if len(kept) != 0 {
			t.Errorf("kept = %v, want none\n%s", kept, out)
		}
		if _, err := os.Lstat(filepath.Join(dir, "old.md")); !os.IsNotExist(err) {
			t.Errorf("old.md not removed: %v", err)
		}
	})

	t.Run("an entry that can't be removed is kept", func(t *testing.T) {
		dir := t.TempDir()
		for _, n := range []string{"boom.md", "fine.md"} {
			writeTestFile(t, filepath.Join(dir, n), "old\n")
		}
		removeFn := func(path string) error {
			if filepath.Base(path) == "boom.md" {
				return &os.PathError{Op: "remove", Path: path, Err: fs.ErrPermission}
			}
			return os.Remove(path)
		}
		var kept []string
		captureStdout(t, func() { kept = removeStale(dir, []string{"boom.md", "fine.md"}, nil, staleFile, removeFn, false) })
		if !reflect.DeepEqual(kept, []string{"boom.md"}) {
			t.Errorf("kept = %v, want [boom.md]", kept)
		}
	})

	t.Run("an entry that can't be checked is kept, one already gone is not", func(t *testing.T) {
		dir := t.TempDir()
		orig := lstat
		lstat = func(path string) (os.FileInfo, error) {
			if filepath.Base(path) == "gone-skill" {
				return nil, &os.PathError{Op: "lstat", Path: path, Err: fs.ErrNotExist}
			}
			return nil, &os.PathError{Op: "lstat", Path: path, Err: fs.ErrPermission}
		}
		t.Cleanup(func() { lstat = orig })
		for _, dryRun := range []bool{false, true} {
			var kept []string
			captureStdout(t, func() {
				kept = removeStale(dir, []string{"old-skill", "gone-skill"}, nil, staleDir, os.RemoveAll, dryRun)
			})
			if !reflect.DeepEqual(kept, []string{"old-skill"}) {
				t.Errorf("dryRun=%v: kept = %v, want [old-skill]", dryRun, kept)
			}
		}
	})
}

// TestRemoveStale_SymlinkInsideSkill: a real stale skill directory is removed
// whole, but a symlink inside it is unlinked, never followed: the user data it
// points at stays.
func TestRemoveStale_SymlinkInsideSkill(t *testing.T) {
	root := t.TempDir()
	skills := filepath.Join(root, "skills")
	user := filepath.Join(root, "user")
	writeTestFile(t, filepath.Join(user, "notes", "keep.md"), "user data\n")
	writeTestFile(t, filepath.Join(user, "keep.md"), "user data\n")
	writeTestFile(t, filepath.Join(skills, "old-skill", "SKILL.md"), "old\n")
	mustSymlink(t, filepath.Join(user, "notes"), filepath.Join(skills, "old-skill", "references"))
	mustSymlink(t, filepath.Join(user, "keep.md"), filepath.Join(skills, "old-skill", "keep.md"))
	before := treeBytes(t, user)

	var kept []string
	out := captureStdout(t, func() { kept = removeStale(skills, []string{"old-skill"}, nil, staleDir, os.RemoveAll, false) })

	if _, err := os.Lstat(filepath.Join(skills, "old-skill")); !os.IsNotExist(err) {
		t.Errorf("stale skill not removed: %v\n%s", err, out)
	}
	if after := treeBytes(t, user); !reflect.DeepEqual(after, before) {
		t.Errorf("user data changed through a symlink inside the skill:\n got %v\nwant %v", after, before)
	}
	if len(kept) != 0 {
		t.Errorf("kept = %v, want none", kept)
	}
}
