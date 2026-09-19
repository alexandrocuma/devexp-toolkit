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

// ── Stale removal and symlinked target directories ───────────────────────────

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

// TestDoInstall_ParentSymlinks: a linked ~/.claude or ~/.config/opencode puts
// real agents/, skills/ and commands/ directories in the dotfiles tree, so the
// install writes through but removes nothing there, and records what it left.
// A HOME that is itself behind a symlink is cleaned up as usual.
func TestDoInstall_ParentSymlinks(t *testing.T) {
	for tname, tg := range staleTargets(t) {
		for _, linkHome := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s linkHome=%v", tname, linkHome), func(t *testing.T) {
				root := t.TempDir()
				home := filepath.Join(root, "home")
				fakeCLI(t)
				repoDir := writeStaleRepo(t)
				l := tg.layout(home)
				// The directory holding agents/ and skills/: ~/.claude, or
				// ~/.config/opencode.
				parent := filepath.Dir(l.agents)
				if linkHome {
					os.MkdirAll(filepath.Join(root, "real-home"), 0o755) //nolint:errcheck
					mustSymlink(t, filepath.Join(root, "real-home"), home)
				} else {
					real := filepath.Join(root, "dotfiles", filepath.Base(parent))
					os.MkdirAll(real, 0o755)                 //nolint:errcheck
					os.MkdirAll(filepath.Dir(parent), 0o755) //nolint:errcheck
					mustSymlink(t, real, parent)
				}
				t.Setenv("HOME", home)
				writeTestFile(t, filepath.Join(l.agents, "old.md"), "old\n")
				writeTestFile(t, filepath.Join(l.skills, l.staleSkillFile), "old\n")
				writeTestManifest(t, l.manifest, []string{"old.md"}, []string{l.staleSkill})

				out := runStaleInstall(t, tg.install, repoDir)

				stale := []string{filepath.Join(l.agents, "old.md"), filepath.Join(l.skills, l.staleSkillDisk)}
				m := loadTestManifest(t, l.manifest)
				if linkHome {
					for _, p := range stale {
						if _, err := os.Lstat(p); !os.IsNotExist(err) {
							t.Errorf("%s not removed under a HOME behind a symlink: %v\n%s", p, err, out)
						}
					}
					if strings.Contains(out, "behind a symlink") || !reflect.DeepEqual(m.Agents, []string{"dev-agent.md"}) || !reflect.DeepEqual(m.Skills, []string{"graphify"}) {
						t.Errorf("manifest %+v, want only what was installed:\n%s", m, out)
					}
					return
				}
				for _, p := range stale {
					if _, err := os.Lstat(p); err != nil {
						t.Errorf("%s removed through the symlinked %s: %v\n%s", p, parent, err, out)
					}
				}
				for _, dir := range []string{l.agents, l.skills} {
					if want := fmt.Sprintf("%q is behind a symlink (it resolves to ", dir); !strings.Contains(out, want) {
						t.Errorf("no warning %q:\n%s", want, out)
					}
				}
				if want := []string{"dev-agent.md", "old.md"}; !reflect.DeepEqual(m.Agents, want) {
					t.Errorf("manifest agents = %v, want %v", m.Agents, want)
				}
				if want := []string{"graphify", l.staleSkill}; !reflect.DeepEqual(m.Skills, want) {
					t.Errorf("manifest skills = %v, want %v", m.Skills, want)
				}
				if _, err := os.Stat(filepath.Join(l.agents, "dev-agent.md")); err != nil {
					t.Errorf("install did not write through the link: %v", err)
				}
			})
		}
	}
}

// TestRemoveStale_SymlinkedTargetDir: through a target directory that is a
// symlink nothing is removed or previewed as removed, in a real run or a dry
// run. The stale entries still there are listed in one warning and returned
// for the manifest, each once however often the manifest repeats it; one
// already gone is dropped, and an entry that isn't devexp's (a symlink) is
// warned about as usual and not returned.
func TestRemoveStale_SymlinkedTargetDir(t *testing.T) {
	tests := map[string]struct {
		shape    staleShape
		removeFn func(*os.Root, string) error
		stale    []string // present in the link target
	}{
		"agent files":       {shape: staleFile, removeFn: (*os.Root).Remove, stale: []string{"old.md", "older.md"}},
		"skill directories": {shape: staleDir, removeFn: (*os.Root).RemoveAll, stale: []string{"old-skill", "older-skill"}},
		"opencode commands": {shape: staleCommand, removeFn: (*os.Root).Remove, stale: []string{"old-cmd", "older-cmd"}},
	}
	for name, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s dryRun=%v", name, dryRun), func(t *testing.T) {
				root := t.TempDir()
				dotfiles := filepath.Join(root, "dotfiles")
				target := filepath.Join(root, "home", "target")
				var listed []string
				for _, e := range tt.stale {
					writeStaleEntry(t, dotfiles, e, tt.shape, "user data\n")
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
				removeFn := func(r *os.Root, name string) error {
					removed = append(removed, name)
					return tt.removeFn(r, name)
				}
				gone := "gone"
				if tt.shape == staleFile {
					gone = "gone.md"
				}
				entries := append(append([]string{}, tt.stale...), gone, linkedEntry, tt.stale[0])

				var kept []string
				out := captureStdout(t, func() { kept = removeStale(filepath.Dir(target), target, entries, nil, tt.shape, removeFn, dryRun) })

				if len(removed) > 0 {
					t.Errorf("removed %v through a symlinked directory\n%s", removed, out)
				}
				if after := treeBytes(t, root); !reflect.DeepEqual(after, before) {
					t.Errorf("tree changed:\n got %v\nwant %v\n%s", after, before, out)
				}
				if strings.Contains(out, "[dry-run]") {
					t.Errorf("a removal through the symlink was previewed:\n%s", out)
				}
				want := fmt.Sprintf("%q is a symlink — devexp never removes files through it; remove these by hand: %s\n", target, strings.Join(listed, ", "))
				if strings.Count(out, "never removes files through it") != 1 || !strings.Contains(out, want) {
					t.Errorf("want exactly one warning %q:\n%s", want, out)
				}
				if !strings.Contains(out, fmt.Sprintf("%q left untouched: no longer in this release, but a symlink", filepath.Join(target, onDisk(linkedEntry, tt.shape)))) {
					t.Errorf("no warning for the symlinked entry:\n%s", out)
				}
				if !reflect.DeepEqual(kept, tt.stale) {
					t.Errorf("kept = %v, want %v, each once", kept, tt.stale)
				}
			})
		}
	}
}

// writeStaleEntry puts what a stale entry names on disk under dir: a skill
// directory with a SKILL.md, or a file.
func writeStaleEntry(t *testing.T, dir, entry string, shape staleShape, content string) {
	t.Helper()
	if shape == staleDir {
		writeTestFile(t, filepath.Join(dir, entry, "SKILL.md"), content)
		return
	}
	writeTestFile(t, filepath.Join(dir, onDisk(entry, shape)), content)
}

// TestRemoveStale_BehindSymlink: a target directory that is real but sits
// under a symlinked ~/.claude or ~/.config/opencode is in the dotfiles tree,
// so nothing is removed from it; a HOME that is itself behind a symlink (as
// every macOS temp dir is, under /var -> /private/var) changes nothing.
func TestRemoveStale_BehindSymlink(t *testing.T) {
	tests := map[string]struct {
		rel        string // the target, relative to home
		link       string // relative to home: the directory that is a symlink, or ""
		linkHome   bool   // home itself is a symlink
		wantBehind bool
	}{
		"a linked ~/.claude":          {rel: ".claude/agents", link: ".claude", wantBehind: true},
		"a linked ~/.config/opencode": {rel: ".config/opencode/commands", link: ".config/opencode", wantBehind: true},
		"a linked ~/.config":          {rel: ".config/opencode/agents", link: ".config", wantBehind: true},
		"home behind a symlink":       {rel: ".claude/agents", linkHome: true},
		"no symlink":                  {rel: ".claude/agents"},
	}
	for name, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s dryRun=%v", name, dryRun), func(t *testing.T) {
				root := t.TempDir()
				home := filepath.Join(root, "home")
				if tt.linkHome {
					os.MkdirAll(filepath.Join(root, "real-home"), 0o755) //nolint:errcheck
					mustSymlink(t, filepath.Join(root, "real-home"), home)
				}
				dir := filepath.Join(home, filepath.FromSlash(tt.rel))
				if tt.link != "" {
					linkPath := filepath.Join(home, filepath.FromSlash(tt.link))
					real := filepath.Join(root, "dotfiles", filepath.Base(tt.link))
					os.MkdirAll(real, 0o755)                   //nolint:errcheck
					os.MkdirAll(filepath.Dir(linkPath), 0o755) //nolint:errcheck
					mustSymlink(t, real, linkPath)
				}
				writeTestFile(t, filepath.Join(dir, "old.md"), "old\n")

				var kept []string
				out := captureStdout(t, func() { kept = removeStale(home, dir, []string{"old.md"}, nil, staleFile, (*os.Root).Remove, dryRun) })

				_, statErr := os.Lstat(filepath.Join(dir, "old.md"))
				switch {
				case tt.wantBehind:
					resolved, _ := filepath.EvalSymlinks(dir)
					want := fmt.Sprintf("%q is behind a symlink (it resolves to %q) — devexp never removes files through it; remove these by hand: %q\n",
						dir, resolved, filepath.Join(dir, "old.md"))
					if statErr != nil || !reflect.DeepEqual(kept, []string{"old.md"}) || !strings.Contains(out, want) || strings.Contains(out, "[dry-run]") {
						t.Errorf("stat err %v, kept %v, want the file kept and recorded with %q:\n%s", statErr, kept, want, out)
					}
				case dryRun:
					if statErr != nil || len(kept) != 0 || !strings.Contains(out, "[dry-run]") {
						t.Errorf("stat err %v, kept %v, want a removal preview:\n%s", statErr, kept, out)
					}
				default:
					if !os.IsNotExist(statErr) || len(kept) != 0 || strings.Contains(out, "never removes") {
						t.Errorf("stat err %v, kept %v, want old.md removed:\n%s", statErr, kept, out)
					}
				}
			})
		}
	}
}

// TestRemoveStale_ExactName: an entry is removed only under the exact name
// devexp recorded. A case-insensitive or normalization-insensitive filesystem
// (the macOS default) resolves another spelling to the user's own file; that
// is simulated through the rootLstat seam, so the test means the same on any
// filesystem. The entry is dropped from the manifest with a warning, and a run
// after the user replaced a linked directory can't remove their file.
func TestRemoveStale_ExactName(t *testing.T) {
	const nfc, nfd = "caf\u00e9.md", "cafe\u0301.md"
	tests := map[string]struct {
		shape         staleShape
		recorded      string
		onDisk        string // the user's own entry
		wantWarn      string
		insensitiveFS bool // resolve names apart from case and normalization
	}{
		"an agent differing in case": {shape: staleFile, recorded: "retired-agent.md", onDisk: "Retired-Agent.md",
			wantWarn: `retired-agent.md" left untouched: on disk only as "Retired-Agent.md", which differs in case`},
		"an agent differing in case, on a case-insensitive filesystem": {shape: staleFile, recorded: "retired-agent.md", onDisk: "Retired-Agent.md", insensitiveFS: true,
			wantWarn: `retired-agent.md" left untouched: on disk only as "Retired-Agent.md", which differs in case`},
		"a skill differing in case": {shape: staleDir, recorded: "old-skill", onDisk: "Old-Skill", insensitiveFS: true,
			wantWarn: `old-skill" left untouched: on disk only as "Old-Skill", which differs in case`},
		"a command differing in case": {shape: staleCommand, recorded: "old-cmd", onDisk: "OLD-CMD.md", insensitiveFS: true,
			wantWarn: `old-cmd.md" left untouched: on disk only as "OLD-CMD.md", which differs in case`},
		"an agent differing in normalization": {shape: staleFile, recorded: nfc, onDisk: nfd, insensitiveFS: true,
			wantWarn: "left untouched: on disk only under a spelling that differs in Unicode normalization"},
	}
	for name, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s dryRun=%v", name, dryRun), func(t *testing.T) {
				dir := filepath.Join(t.TempDir(), "agents")
				userPath := filepath.Join(dir, tt.onDisk)
				if tt.shape == staleDir {
					writeTestFile(t, filepath.Join(userPath, "SKILL.md"), "user data\n")
				} else {
					writeTestFile(t, userPath, "user data\n")
				}
				if tt.insensitiveFS {
					orig := rootLstat
					rootLstat = func(r *os.Root, n string) (os.FileInfo, error) {
						if strings.EqualFold(n, tt.onDisk) || (n == nfc && tt.onDisk == nfd) {
							n = tt.onDisk
						}
						return orig(r, n)
					}
					t.Cleanup(func() { rootLstat = orig })
				}
				var removed []string
				removeFn := func(r *os.Root, n string) error {
					removed = append(removed, n)
					return r.RemoveAll(n)
				}

				var kept []string
				out := captureStdout(t, func() {
					kept = removeStale(filepath.Dir(dir), dir, []string{tt.recorded}, nil, tt.shape, removeFn, dryRun)
				})

				if len(removed) > 0 || strings.Contains(out, "[dry-run]") {
					t.Errorf("removed or previewed %v under another spelling:\n%s", removed, out)
				}
				if _, err := os.Lstat(userPath); err != nil {
					t.Errorf("the user's %s is gone: %v", tt.onDisk, err)
				}
				if len(kept) != 0 {
					t.Errorf("kept = %v, want the entry dropped: what is on disk isn't devexp's", kept)
				}
				if !strings.Contains(out, tt.wantWarn) {
					t.Errorf("no warning %q:\n%s", tt.wantWarn, out)
				}
			})
		}
	}
}

// TestRemoveStale_DirReplaced: removals can't be redirected by swapping the
// target directory for a symlink. A swap before the directory is opened is
// caught by comparing it with what os.Lstat saw; a swap after it is opened
// doesn't matter, because every removal goes through the *os.Root pinned to
// the directory that was checked.
func TestRemoveStale_DirReplaced(t *testing.T) {
	type layout struct{ dir, moved, dotfiles string }
	swap := func(t *testing.T, l layout) {
		t.Helper()
		if err := os.Rename(l.dir, l.moved); err != nil {
			t.Fatal(err)
		}
		mustSymlink(t, l.dotfiles, l.dir)
	}
	setup := func(t *testing.T, shape staleShape, entry string) layout {
		root := t.TempDir()
		l := layout{dir: filepath.Join(root, "home", "agents"), moved: filepath.Join(root, "home", "agents.moved"), dotfiles: filepath.Join(root, "dotfiles")}
		writeStaleEntry(t, l.dir, entry, shape, "old\n")
		writeStaleEntry(t, l.dotfiles, entry, shape, "user data\n")
		return l
	}
	shapes := map[string]struct {
		shape    staleShape
		entry    string
		removeFn func(*os.Root, string) error
	}{
		"agent file":      {staleFile, "old.md", (*os.Root).Remove},
		"skill directory": {staleDir, "old-skill", (*os.Root).RemoveAll},
	}
	for sname, sh := range shapes {
		t.Run("swapped before it is opened: "+sname, func(t *testing.T) {
			l := setup(t, sh.shape, sh.entry)
			orig := openRoot
			openRoot = func(name string) (*os.Root, error) {
				swap(t, l)
				return orig(name)
			}
			t.Cleanup(func() { openRoot = orig })
			before := treeBytes(t, l.dotfiles)

			var kept []string
			out := captureStdout(t, func() {
				kept = removeStale(filepath.Dir(l.dir), l.dir, []string{sh.entry}, nil, sh.shape, sh.removeFn, false)
			})

			if after := treeBytes(t, l.dotfiles); !reflect.DeepEqual(after, before) {
				t.Errorf("removed through the swapped-in symlink:\n got %v\nwant %v\n%s", after, before, out)
			}
			if !reflect.DeepEqual(kept, []string{sh.entry}) || !strings.Contains(out, "changed while devexp was checking it, so nothing was removed from it; remove these by hand") {
				t.Errorf("kept = %v, want the entry kept with a warning:\n%s", kept, out)
			}
		})
		t.Run("swapped after it is opened: "+sname, func(t *testing.T) {
			l := setup(t, sh.shape, sh.entry)
			before := treeBytes(t, l.dotfiles)
			removeFn := func(r *os.Root, name string) error {
				swap(t, l)
				return sh.removeFn(r, name)
			}

			var kept []string
			out := captureStdout(t, func() {
				kept = removeStale(filepath.Dir(l.dir), l.dir, []string{sh.entry}, nil, sh.shape, removeFn, false)
			})

			if after := treeBytes(t, l.dotfiles); !reflect.DeepEqual(after, before) {
				t.Errorf("removed through the swapped-in symlink:\n got %v\nwant %v\n%s", after, before, out)
			}
			if _, err := os.Lstat(filepath.Join(l.moved, sh.entry)); !os.IsNotExist(err) {
				t.Errorf("the entry in the directory that was checked wasn't removed: %v\n%s", err, out)
			}
			if len(kept) != 0 {
				t.Errorf("kept = %v, want none", kept)
			}
		})
	}
}

// TestRemoveStale_DirCantBeChecked: when the target directory can't be read,
// nothing is removed and every candidate is kept, with one warning; a target
// directory that doesn't exist has nothing left to remove or record.
func TestRemoveStale_DirCantBeChecked(t *testing.T) {
	t.Run("unreadable", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "agents")
		writeTestFile(t, filepath.Join(dir, "old.md"), "old\n")
		orig := readDirNames
		readDirNames = func(*os.Root) ([]string, error) { return nil, fs.ErrPermission }
		t.Cleanup(func() { readDirNames = orig })

		var kept []string
		out := captureStdout(t, func() {
			kept = removeStale(filepath.Dir(dir), dir, []string{"old.md", "../bad.md", "old.md"}, nil, staleFile, (*os.Root).Remove, false)
		})
		if _, err := os.Lstat(filepath.Join(dir, "old.md")); err != nil {
			t.Errorf("old.md removed: %v", err)
		}
		if !reflect.DeepEqual(kept, []string{"old.md"}) {
			t.Errorf("kept = %v, want [old.md]", kept)
		}
		if want := fmt.Sprintf("%q can't be checked (permission denied), so these stale entries were left untouched: \"old.md\"\n", dir); !strings.Contains(out, want) {
			t.Errorf("no warning %q:\n%s", want, out)
		}
	})
	t.Run("not under home", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "agents")
		writeTestFile(t, filepath.Join(dir, "old.md"), "old\n")
		var kept []string
		out := captureStdout(t, func() {
			kept = removeStale(filepath.Join(root, "home"), dir, []string{"old.md"}, nil, staleFile, (*os.Root).Remove, false)
		})
		if _, err := os.Lstat(filepath.Join(dir, "old.md")); err != nil || !reflect.DeepEqual(kept, []string{"old.md"}) || !strings.Contains(out, "can't be checked") {
			t.Errorf("stat err %v, kept %v, want old.md kept with a warning:\n%s", err, kept, out)
		}
	})
	t.Run("missing", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "agents")
		var kept []string
		out := captureStdout(t, func() {
			kept = removeStale(filepath.Dir(dir), dir, []string{"old.md"}, nil, staleFile, (*os.Root).Remove, false)
		})
		if len(kept) != 0 || out != "" {
			t.Errorf("kept = %v, output %q; want nothing", kept, out)
		}
	})
}

// TestRemoveStale_Kept: removeStale returns only what the manifest should go
// on recording, each name once. Removed, already gone and not-devexp's entries
// are dropped; an entry that can't be checked or removed is kept for a later
// run.
func TestRemoveStale_Kept(t *testing.T) {
	t.Run("removed, gone and rejected entries are dropped", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "agents")
		writeTestFile(t, filepath.Join(dir, "old.md"), "old\n")
		writeTestFile(t, filepath.Join(root, "user.md"), "user data\n")
		mustSymlink(t, filepath.Join(root, "user.md"), filepath.Join(dir, "linked.md"))
		writeTestFile(t, filepath.Join(dir, "new.md"), "new\n")
		if err := os.Link(filepath.Join(dir, "new.md"), filepath.Join(dir, "hard.md")); err != nil {
			t.Fatal(err)
		}

		var kept []string
		out := captureStdout(t, func() {
			kept = removeStale(root, dir, []string{"old.md", "gone.md", "linked.md", "../user.md", "NEW.md", "hard.md", "old.md", "new.md"}, []string{"new.md"}, staleFile, (*os.Root).Remove, false)
		})
		if len(kept) != 0 {
			t.Errorf("kept = %v, want none\n%s", kept, out)
		}
		if _, err := os.Lstat(filepath.Join(dir, "old.md")); !os.IsNotExist(err) {
			t.Errorf("old.md not removed: %v", err)
		}
		for _, n := range []string{"new.md", "hard.md"} {
			if got, _ := os.ReadFile(filepath.Join(dir, n)); string(got) != "new\n" {
				t.Errorf("%s = %q, want the installed file untouched", n, got)
			}
		}
		if !strings.Contains(out, `hard.md" left untouched: the same file as "new.md", installed by this run`) {
			t.Errorf("no same-file warning:\n%s", out)
		}
		if n := strings.Count(out, `"old.md"`); n != 1 {
			t.Errorf("old.md reported %d times, want once:\n%s", n, out)
		}
	})

	t.Run("an entry that can't be removed is kept, once", func(t *testing.T) {
		dir := t.TempDir()
		for _, n := range []string{"boom.md", "fine.md"} {
			writeTestFile(t, filepath.Join(dir, n), "old\n")
		}
		removeFn := func(r *os.Root, name string) error {
			if name == "boom.md" {
				return &os.PathError{Op: "remove", Path: name, Err: fs.ErrPermission}
			}
			return r.Remove(name)
		}
		var kept []string
		captureStdout(t, func() {
			kept = removeStale(filepath.Dir(dir), dir, []string{"boom.md", "fine.md", "boom.md"}, nil, staleFile, removeFn, false)
		})
		if !reflect.DeepEqual(kept, []string{"boom.md"}) {
			t.Errorf("kept = %v, want [boom.md]", kept)
		}
	})

	t.Run("an entry removed by someone else meanwhile is dropped silently", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "old.md"), "old\n")
		removeFn := func(r *os.Root, name string) error {
			r.Remove(name) //nolint:errcheck
			return r.Remove(name)
		}
		var kept []string
		out := captureStdout(t, func() {
			kept = removeStale(filepath.Dir(dir), dir, []string{"old.md"}, nil, staleFile, removeFn, false)
		})
		if len(kept) != 0 || strings.Contains(out, "left untouched") || strings.Contains(out, "remove \"") {
			t.Errorf("kept = %v, output %q; want it dropped with no warning", kept, out)
		}
	})

	t.Run("an entry that can't be checked is kept, one already gone is not", func(t *testing.T) {
		dir := t.TempDir()
		writeStaleEntry(t, dir, "old-skill", staleDir, "old\n")
		writeStaleEntry(t, dir, "gone-skill", staleDir, "old\n")
		orig := rootLstat
		rootLstat = func(_ *os.Root, name string) (os.FileInfo, error) {
			if name == "gone-skill" {
				return nil, &os.PathError{Op: "lstat", Path: name, Err: fs.ErrNotExist}
			}
			return nil, &os.PathError{Op: "lstat", Path: name, Err: fs.ErrPermission}
		}
		t.Cleanup(func() { rootLstat = orig })
		for _, dryRun := range []bool{false, true} {
			var kept []string
			captureStdout(t, func() {
				kept = removeStale(filepath.Dir(dir), dir, []string{"old-skill", "gone-skill"}, nil, staleDir, (*os.Root).RemoveAll, dryRun)
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
	out := captureStdout(t, func() {
		kept = removeStale(root, skills, []string{"old-skill"}, nil, staleDir, (*os.Root).RemoveAll, false)
	})

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
