package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

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
	staleFile    staleShape = iota // <name>.md, a regular file: agents
	staleDir                       // <name>, a real directory: Claude Code skills
	staleCommand                   // <name>, recorded without .md, a regular <name>.md file: opencode commands
)

// isInstalledName reports whether name has the shape of an entry devexp
// installs directly in a target directory: a bare name, ending in .md for a
// file. The manifest is a file on disk, so an entry can be anything; joined
// onto the target directory unchecked, "../x" removed a file outside it, and
// for skills (removed recursively) "", "." or ".." removed the skills
// directory itself or its parent. No installed name has a control character,
// and one printed raw could fake output lines or clear the terminal.
func isInstalledName(name string, shape staleShape) bool {
	if name == "" || name == "." || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) ||
		strings.ContainsFunc(name, unicode.IsControl) {
		return false
	}
	switch shape {
	case staleFile:
		return strings.HasSuffix(name, ".md") && name != ".md"
	case staleCommand:
		return isInstalledName(name+".md", staleFile)
	}
	return true
}

// onDisk is the name an entry has in its target directory: opencode commands
// are recorded without the .md their file has.
func onDisk(name string, shape staleShape) string {
	if shape == staleCommand {
		return name + ".md"
	}
	return name
}

// lstat is os.Lstat; tests swap it to inject errors.
var lstat = os.Lstat

// removeStale removes from dir, via removeFn, the entries of old (the previous
// manifest) that this run didn't install, reporting via ui. In dry-run mode it
// only reports what would be removed. It returns the stale entries it had to
// leave on disk that the manifest should go on recording, so a later run can
// finish the job.
//
// An entry is kept, with a warning, unless all of these hold:
//   - it is a name devexp installs (isInstalledName);
//   - it is not a name this run installed apart from case, nor the same file
//     on disk as one. manifest.Stale compares names exactly, but a
//     case-insensitive filesystem (the macOS default) resolves "DEV-AGENT.md"
//     to the dev-agent.md this run just wrote, and ignores Unicode
//     normalization too. The case check works in a dry run; os.SameFile
//     catches any other variant once the install is on disk, so a dry run
//     can still preview removing a normalization-only variant;
//   - what is on disk can be checked and has the entry's shape: a regular
//     file for staleFile and staleCommand, a real directory for staleDir. A
//     symlink is never removed: it is the user's own setup, as for opencode
//     plugin files. (Inside a real skill directory, os.RemoveAll unlinks a
//     symlink without following it.);
//   - dir itself is not a symlink (a dotfiles setup). The installers write
//     through one, but nothing is ever removed through it (#128), as for
//     opencode plugins/: the entries that pass every other check are listed
//     in one warning to remove by hand.
//
// Of the entries kept, those left behind a symlinked dir and those that
// couldn't be checked or removed are returned; the others aren't devexp's on
// disk. An entry that is already gone needs nothing. Names and paths are
// printed quoted, so nothing from the manifest reaches the terminal raw.
func removeStale(dir string, old, installed []string, shape staleShape, removeFn func(path string) error, dryRun bool) (kept []string) {
	linked := isSymlink(dir)
	var leftBehind []string
	for _, name := range manifest.Stale(old, installed) {
		if !isInstalledName(name, shape) {
			ui.Warn(fmt.Sprintf("%q left untouched: listed in the manifest but not a name devexp installs in %q", name, dir))
			continue
		}
		if twin, ok := foldMatch(name, installed); ok {
			ui.Warn(fmt.Sprintf("%q left untouched: the same name as %q, installed by this run, apart from case", name, twin))
			continue
		}
		path := filepath.Join(dir, onDisk(name, shape))
		fi, err := lstat(path)
		switch {
		case os.IsNotExist(err):
			continue
		case err != nil:
			ui.Warn(fmt.Sprintf("%q left untouched: %v", path, pathErrCause(err)))
			kept = append(kept, name)
			continue
		case !hasStaleShape(fi, shape):
			ui.Warn(fmt.Sprintf("%q left untouched: no longer in this release, but %s", path, describeMode(fi)))
			continue
		}
		if twin, ok := sameFileAs(fi, dir, installed, shape); ok {
			ui.Warn(fmt.Sprintf("%q left untouched: the same file as %q, installed by this run", path, twin))
			continue
		}
		if linked {
			leftBehind = append(leftBehind, fmt.Sprintf("%q", path))
			kept = append(kept, name)
			continue
		}
		if dryRun {
			ui.DryRun(fmt.Sprintf("remove %q (no longer in this release)", path))
			continue
		}
		if err := removeFn(path); err != nil && !os.IsNotExist(err) {
			ui.Warn(fmt.Sprintf("remove %q: %v", path, pathErrCause(err)))
			kept = append(kept, name)
			continue
		}
		ui.Removed(fmt.Sprintf("%q", onDisk(name, shape)))
	}
	if len(leftBehind) > 0 {
		ui.Warn(fmt.Sprintf("%q is a symlink — devexp never removes files through it; remove these by hand: %s", dir, strings.Join(leftBehind, ", ")))
	}
	return kept
}

// isSymlink reports whether path itself, not what it resolves to, is a
// symlink. A path that can't be checked is not reported as one: an entry under
// it then fails its own check, and is kept.
func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

func foldMatch(name string, installed []string) (string, bool) {
	for _, in := range installed {
		if strings.EqualFold(name, in) {
			return in, true
		}
	}
	return "", false
}

func sameFileAs(fi os.FileInfo, dir string, installed []string, shape staleShape) (string, bool) {
	for _, in := range installed {
		if other, err := lstat(filepath.Join(dir, onDisk(in, shape))); err == nil && os.SameFile(fi, other) {
			return in, true
		}
	}
	return "", false
}

// pathErrCause drops the path an *os.PathError repeats, which callers print
// quoted themselves.
func pathErrCause(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
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
