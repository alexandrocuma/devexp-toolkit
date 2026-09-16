package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devexp/internal/manifest"
	"devexp/internal/skills"
	"devexp/internal/ui"
)

// backupExisting copies pre-existing files matching glob into backupDir.
func backupExisting(dir, pattern, backupDir string, dryRun bool) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 || dryRun {
		return
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return
	}
	for _, src := range matches {
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		os.WriteFile(filepath.Join(backupDir, filepath.Base(src)), data, 0644) //nolint:errcheck
	}
}

// backupExistingDirs copies pre-existing skill directories (immediate
// subdirectories of dir containing a SKILL.md) into backupDir/<name>/.
func backupExistingDirs(dir, backupDir string, dryRun bool) {
	if dryRun {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		srcDir := filepath.Join(dir, name)
		if _, err := os.Stat(filepath.Join(srcDir, "SKILL.md")); err != nil {
			continue
		}
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return
		}
		skills.CopyDir(srcDir, filepath.Join(backupDir, name)) //nolint:errcheck
	}
}

// loadOldManifest reads the previous run's manifest. An unreadable or corrupt
// manifest is reported and treated as empty: the install carries on, nothing
// counts as stale on this run (an empty old list marks nothing for removal),
// and the manifest is rewritten at the end as usual.
func loadOldManifest(path string) *manifest.Manifest {
	old, err := manifest.Load(path)
	if err != nil {
		ui.Warn(fmt.Sprintf("manifest %s is unreadable, so no stale agents or skills are removed this run: %v", path, err))
	}
	return old
}

// staleShape is what a stale manifest entry names on disk.
type staleShape int

const (
	staleFile staleShape = iota // <name>.md, a regular file: agents, opencode commands
	staleDir                    // <name>, a real directory: Claude Code skills
)

// isInstalledName reports whether name has the shape of an entry devexp
// installs directly in a target directory: a bare name, ending in .md for a
// file. The manifest is a file on disk, so an entry can be anything; joined
// onto the target directory unchecked, "../x" removed a file outside it, and
// for skills (removed recursively) "", "." or ".." removed the skills
// directory itself or its parent.
func isInstalledName(name string, shape staleShape) bool {
	if name == "" || name == "." || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return false
	}
	if shape == staleFile {
		return strings.HasSuffix(name, ".md") && name != ".md"
	}
	return true
}

// removeStale removes each entry in stale from dir via removeFn, reporting via
// ui. In dry-run mode it only reports what would be removed.
//
// An entry is removed only when it is a name devexp installs (isInstalledName)
// and what is on disk has that shape: a regular file for staleFile, a real
// directory for staleDir. Anything else is kept with a warning. A symlink is
// never removed: it is the user's own setup, as for opencode plugin files.
func removeStale(dir string, stale []string, shape staleShape, removeFn func(path string) error, dryRun bool) {
	for _, name := range stale {
		if !isInstalledName(name, shape) {
			ui.Warn(fmt.Sprintf("%q left untouched: listed in the manifest but not a name devexp installs in %s", name, dir))
			continue
		}
		path := filepath.Join(dir, name)
		if fi, err := os.Lstat(path); err == nil && !hasStaleShape(fi, shape) {
			ui.Warn(fmt.Sprintf("%s left untouched: no longer in this release, but %s", path, describeMode(fi)))
			continue
		}
		if dryRun {
			ui.DryRun(fmt.Sprintf("remove %s (no longer in this release)", path))
			continue
		}
		if err := removeFn(path); err != nil && !os.IsNotExist(err) {
			ui.Warn(fmt.Sprintf("remove %s: %v", path, err))
			continue
		}
		ui.Removed(name)
	}
}

func hasStaleShape(fi os.FileInfo, shape staleShape) bool {
	if shape == staleDir {
		return fi.IsDir()
	}
	return fi.Mode().IsRegular()
}

func describeMode(fi os.FileInfo) string {
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return "a symlink — devexp never removes one"
	case fi.IsDir():
		return "a directory, not a file devexp installs"
	case fi.Mode().IsRegular():
		return "a file, not a directory devexp installs"
	}
	return "not a file or directory devexp installs"
}
