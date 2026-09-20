package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"devexp/internal/manifest"
	"devexp/internal/removeguard"
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
		// A backup is best effort and never gates the install: a file that
		// cannot be copied means that one file has no backup, and the user is
		// told nothing because there is nothing they can do differently. What
		// they see is unchanged — the install proceeds either way.
		os.WriteFile(filepath.Join(backupDir, filepath.Base(src)), data, 0644) //nolint:errcheck // best-effort backup; a failed copy leaves that file unbacked and the install proceeds
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
		// Same contract as the agent backup above.
		skills.CopyDir(srcDir, filepath.Join(backupDir, name)) //nolint:errcheck // best-effort backup; a failed copy leaves that skill unbacked and the install proceeds
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

// Test seams: rootLstat checks one entry, openRoot pins the target directory
// and readDirNames lists it.
var (
	rootLstat    = (*os.Root).Lstat
	openRoot     = os.OpenRoot
	readDirNames = func(root *os.Root) ([]string, error) {
		f, err := root.Open(".")
		if err != nil {
			return nil, err
		}
		defer f.Close() //nolint:errcheck // read-only handle, nothing buffered: a close error cannot change what was already read
		return f.Readdirnames(-1)
	}
)

// removeStale removes from dir, via removeFn ((*os.Root).Remove or
// (*os.Root).RemoveAll), the entries of old (the previous manifest) that this
// run didn't install, reporting via ui. In dry-run mode it only reports what
// would be removed. home is the HOME dir that dir is built from. It returns the
// stale entries the manifest should go on recording, each once, so a later run
// can finish the job.
//
// An entry is removed only when all of these hold; otherwise it is kept on
// disk, with a warning unless it is already gone:
//   - it is a name devexp installs (isInstalledName);
//   - it is not a name this run installed apart from case;
//   - dir, read once, lists exactly that name, byte for byte. A
//     case-insensitive or normalization-insensitive filesystem (the macOS
//     default) resolves "retired-agent.md" to a user's "Retired-Agent.md", so
//     an entry that is on disk only under another spelling is not devexp's.
//     A file the user wrote under the exact recorded name can't be told
//     apart from devexp's, which is inherent to tracking by name;
//   - what is there can be checked and has the entry's shape: a regular file
//     for staleFile and staleCommand, a real directory for staleDir. A symlink
//     is never removed: it is the user's own setup, as for opencode plugin
//     files (inside a real skill directory, RemoveAll unlinks a symlink
//     without following it);
//   - it is not the same file on disk as one this run installed;
//   - removal isn't blocked (openRemovalDir): dir is not a symlink, not behind
//     one below home, and didn't change while it was checked. The installers
//     write through a symlinked directory, but nothing is removed through it,
//     as for opencode plugins/: the entries that pass every other check are
//     listed in one warning to remove by hand.
//
// Of the entries kept, those left behind a blocked dir and those that couldn't
// be checked or removed are returned; the others aren't devexp's on disk.
// Removals go through an *os.Root opened on the directory that was checked, so
// swapping dir for a symlink mid-run can't redirect them. Names and paths are
// printed quoted, so nothing from the manifest reaches the terminal raw.
func removeStale(home, dir string, old, installed []string, shape staleShape, removeFn func(root *os.Root, name string) error, dryRun bool) (kept []string) {
	var candidates []string
	seen := map[string]bool{}
	for _, name := range manifest.Stale(old, installed) {
		if seen[name] {
			continue
		}
		seen[name] = true
		if !isInstalledName(name, shape) {
			ui.Warn(fmt.Sprintf("%q left untouched: listed in the manifest but not a name devexp installs in %q", name, dir))
			continue
		}
		if twin, ok := foldMatch(name, installed); ok {
			ui.Warn(fmt.Sprintf("%q left untouched: the same name as %q, installed by this run, apart from case", name, twin))
			continue
		}
		candidates = append(candidates, name)
	}
	if len(candidates) == 0 {
		return nil
	}

	sd, err := openRemovalDir(home, dir)
	switch {
	case os.IsNotExist(err):
		return nil // nothing of devexp's is left there
	case err != nil:
		ui.Warn(fmt.Sprintf("%q can't be checked (%v), so these stale entries were left untouched: %s", dir, pathErrCause(err), quoteAll(candidates)))
		return candidates
	}
	defer sd.root.Close() //nolint:errcheck // read-only handle opened to enumerate the directory; nothing is written through it

	var leftBehind []string
	for _, name := range candidates {
		disk := onDisk(name, shape)
		path := filepath.Join(dir, disk)
		if !sd.names[disk] {
			if variant, ok := foldMatch(disk, sd.listed); ok {
				ui.Warn(fmt.Sprintf("%q left untouched: on disk only as %q, which differs in case — devexp removes only the exact name it recorded", path, variant))
			} else if _, err := rootLstat(sd.root, disk); err == nil {
				ui.Warn(fmt.Sprintf("%q left untouched: on disk only under a spelling that differs in Unicode normalization — devexp removes only the exact name it recorded", path))
			}
			continue
		}
		fi, err := rootLstat(sd.root, disk)
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
		if twin, ok := sameFileAs(fi, sd.root, installed, shape); ok {
			ui.Warn(fmt.Sprintf("%q left untouched: the same file as %q, installed by this run", path, twin))
			continue
		}
		if sd.blocked != "" {
			leftBehind = append(leftBehind, name)
			kept = append(kept, name)
			continue
		}
		if dryRun {
			ui.DryRun(fmt.Sprintf("remove %q (no longer in this release)", path))
			continue
		}
		// An entry that vanished between the check and the remove was removed
		// by someone else: there is nothing left to record or retry, so it is
		// neither kept nor warned about.
		if err := removeFn(sd.root, disk); err != nil && !os.IsNotExist(err) {
			ui.Warn(fmt.Sprintf("remove %q: %v", path, pathErrCause(err)))
			kept = append(kept, name)
			continue
		}
		ui.Removed(fmt.Sprintf("%q", disk))
	}
	if len(leftBehind) > 0 {
		paths := make([]string, len(leftBehind))
		for i, name := range leftBehind {
			paths[i] = filepath.Join(dir, onDisk(name, shape))
		}
		ui.Warn(fmt.Sprintf("%q %s; remove these by hand: %s", dir, sd.blocked, quoteAll(paths)))
	}
	return kept
}

// removalDir is a target directory opened for stale removal.
type removalDir struct {
	root    *os.Root        // the directory that was checked
	names   map[string]bool // its entries, exactly as listed
	listed  []string
	blocked string // why nothing may be removed from it, or ""
}

// openRemovalDir opens dir, a target directory under home, for removeStale: it
// pins the directory with os.OpenRoot and lists it once. Removal is blocked,
// though the directory is still read, when:
//   - dir is a symlink;
//   - dir is behind one: a directory between home and dir is a symlink
//     (removeguard.BehindSymlink);
//   - the directory opened is not the one os.Lstat saw, because dir was
//     replaced in between.
//
// A dir that doesn't exist returns an error satisfying os.IsNotExist.
func openRemovalDir(home, dir string) (*removalDir, error) {
	lfi, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	sd := &removalDir{names: map[string]bool{}}
	if lfi.Mode()&os.ModeSymlink != 0 {
		sd.blocked = "is a symlink — devexp never removes files through it"
	} else if resolved, behind, err := removeguard.BehindSymlink(home, dir); err != nil {
		return nil, err
	} else if behind {
		sd.blocked = fmt.Sprintf("is behind a symlink (it resolves to %q) — devexp never removes files through it", resolved)
	}
	root, err := openRoot(dir)
	if err != nil {
		return nil, err
	}
	if sd.blocked == "" {
		if st, err := root.Stat("."); err != nil || !os.SameFile(st, lfi) {
			sd.blocked = "changed while devexp was checking it, so nothing was removed from it"
		}
	}
	names, err := readDirNames(root)
	if err != nil {
		root.Close() //nolint:errcheck // closing after an error that is already being returned; a close error would replace the real cause
		return nil, err
	}
	sd.root, sd.listed = root, names
	for _, n := range names {
		sd.names[n] = true
	}
	return sd, nil
}

func quoteAll(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}

func foldMatch(name string, installed []string) (string, bool) {
	for _, in := range installed {
		if strings.EqualFold(name, in) {
			return in, true
		}
	}
	return "", false
}

func sameFileAs(fi os.FileInfo, root *os.Root, installed []string, shape staleShape) (string, bool) {
	for _, in := range installed {
		if other, err := rootLstat(root, onDisk(in, shape)); err == nil && os.SameFile(fi, other) {
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
