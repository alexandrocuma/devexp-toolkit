package hooks

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"devexp/internal/removeguard"
	"devexp/internal/ui"
)

// ── Removing the copied Kimi hook scripts ─────────────────────────────────────
//
// The same removal rules as agents and skills (#124, #128), which this path
// did not have and had to gain: a script recorded in the manifest was removed
// with a plain Lstat + Remove on filepath.Join(hooksDir, rel). isKimiHookPath
// vets the recorded *string* — no traversal, no absolute path, the right
// shape — but a string check says nothing about what is actually on disk, and
// two things got through it (PR #176 review):
//
//   - **Removal through a linked parent.** With hooks/kimi, hooks/, or the
//     whole Kimi root symlinked into a dotfiles checkout, the recorded script
//     was deleted *inside that checkout*, and the emptied linked directories
//     were then pruned. No hand-edited manifest was needed: an ordinary
//     install plus a user who symlinks their hooks directory. In the same run
//     agents/ and skills/ correctly refused, so one uninstall protected
//     linked agents and deleted linked hook scripts.
//   - **A case-folded match.** On a case-insensitive filesystem — the macOS
//     default — a recorded "kimi/ADAPTER.sh" removed the on-disk
//     "kimi/adapter.sh", because the name was joined and opened rather than
//     matched against what the directory actually lists.
//
// So removal goes the way removeStale goes in cli/cmd: pin each directory
// with os.OpenRoot, list it once, remove only a name that listing holds byte
// for byte, and refuse entirely when the directory is a symlink or behind
// one. The listing and the removal share one handle, so swapping the
// directory for a symlink mid-run cannot redirect either.
//
// home is what the guard resolves symlinks from. It is the Kimi root's
// parent, not $HOME — kimiPaths.home — so a $KIMI_CODE_HOME outside the home
// directory is still guarded rather than unguardable.

// Test seams, mirroring cli/cmd/backup.go.
var (
	kimiOpenRoot     = os.OpenRoot
	kimiRootLstat    = (*os.Root).Lstat
	kimiReadDirNames = func(root *os.Root) ([]string, error) {
		entries, err := root.FS().(interface {
			ReadDir(string) ([]os.DirEntry, error)
		}).ReadDir(".")
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names, nil
	}
)

// kimiDir is one pinned hooks subdirectory, listed once.
type kimiDir struct {
	root    *os.Root
	names   map[string]bool
	blocked string // why nothing may be removed from it, or ""
}

// openKimiRemovalDir pins dir and lists it. It mirrors openRemovalDir in
// cli/cmd/backup.go; the rules must stay identical, because a user with a
// symlinked hooks directory and a symlinked agents directory has to get the
// same answer for both.
func openKimiRemovalDir(home, dir string) (*kimiDir, error) {
	lfi, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	d := &kimiDir{names: map[string]bool{}}
	if lfi.Mode()&os.ModeSymlink != 0 {
		d.blocked = "is a symlink — devexp never removes files through it"
	} else if resolved, behind, err := removeguard.BehindSymlink(home, dir); err != nil {
		// Unresolvable, or not under home at all. Either way it cannot be
		// checked, so nothing may be removed from it.
		d.blocked = fmt.Sprintf("could not be checked (%v) — devexp removes nothing it cannot check", err)
	} else if behind {
		d.blocked = fmt.Sprintf("is behind a symlink (it resolves to %q) — devexp never removes files through it", resolved)
	}

	root, err := kimiOpenRoot(dir)
	if err != nil {
		return nil, err
	}
	if d.blocked == "" {
		// The directory that was checked must be the one that was opened. Like
		// the BehindSymlink call in pruneEmptyKimiDirs, this closes a race
		// rather than a reachable single-threaded path: nothing but a
		// concurrent swap between the Lstat above and this Stat can make it
		// fire, so a mutation that removes it survives every test and always
		// will. Kept deliberately; not a missing assertion.
		if st, err := root.Stat("."); err != nil || !os.SameFile(st, lfi) {
			d.blocked = "changed while devexp was checking it, so nothing was removed from it"
		}
	}
	names, err := kimiReadDirNames(root)
	if err != nil {
		root.Close()
		return nil, err
	}
	d.root = root
	for _, n := range names {
		d.names[n] = true
	}
	return d, nil
}

// removeKimiFiles deletes recorded scripts this run no longer installs and
// returns the ones still on disk, which the manifest must go on recording.
//
// Anything it will not remove is kept in that record rather than dropped: a
// script left behind a symlinked directory is still devexp's, and forgetting
// it would strand it for ever.
func removeKimiFiles(home, hooksDir string, stale []string, dryRun bool) (kept []string) {
	// Group by subdirectory, so each is opened, checked and listed exactly
	// once however many scripts it holds.
	byDir := map[string][]string{}
	var order []string
	for _, rel := range stale {
		if !isKimiHookPath(rel) {
			ui.Warn(fmt.Sprintf("the manifest records a hook file as %q, which is not a name devexp installs; it was left alone", rel))
			kept = append(kept, rel)
			continue
		}
		sub := path.Dir(rel)
		if _, seen := byDir[sub]; !seen {
			order = append(order, sub)
		}
		byDir[sub] = append(byDir[sub], rel)
	}

	removedAny := false
	for _, sub := range order {
		dir := filepath.Join(hooksDir, filepath.FromSlash(sub))
		d, err := openKimiRemovalDir(home, dir)
		switch {
		case os.IsNotExist(err):
			continue // nothing of devexp's is left there
		case err != nil:
			ui.Warn(fmt.Sprintf("%q can't be checked (%v), so these hook scripts were left untouched: %s", dir, err, strings.Join(quoteAllKimi(byDir[sub]), ", ")))
			kept = append(kept, byDir[sub]...)
			continue
		}
		if d.blocked != "" {
			ui.Warn(fmt.Sprintf("%q %s; remove these by hand: %s", dir, d.blocked, strings.Join(quoteAllKimi(byDir[sub]), ", ")))
			kept = append(kept, byDir[sub]...)
			d.root.Close()
			continue
		}
		for _, rel := range byDir[sub] {
			if removeOneKimiFile(d, dir, rel, dryRun, &kept) {
				removedAny = true
			}
		}
		d.root.Close()
	}

	// Only when something actually went: a run that removed nothing has no
	// business tidying directories it did not empty.
	if !dryRun && removedAny {
		pruneEmptyKimiDirs(home, hooksDir)
	}
	return kept
}

// removeOneKimiFile removes one recorded script through the pinned handle,
// and reports whether it did. Anything kept is appended to kept.
func removeOneKimiFile(d *kimiDir, dir, rel string, dryRun bool, kept *[]string) bool {
	base := path.Base(rel)
	// The listing, byte for byte. A name that is on disk only under another
	// spelling — different case, different Unicode normalization — is not the
	// name devexp recorded, and on a case-insensitive filesystem it may well
	// be the user's own file.
	if !d.names[base] {
		if variant, ok := foldMatchKimi(base, d.names); ok {
			ui.Warn(fmt.Sprintf("%q left untouched: on disk only as %q, which differs in case — devexp removes only the exact name it recorded", filepath.Join(dir, base), variant))
			*kept = append(*kept, rel)
			return false
		}
		// Reachable through the pinned handle but absent from the listing:
		// the name exists on disk under a different Unicode normalization
		// (NFC vs NFD, routine on APFS). Nothing wrong is deleted either way,
		// but staying silent dropped it from the manifest and stranded a real
		// file with no record of it. Same treatment as the fold branch above,
		// and as removeStale's own branch for this.
		if _, err := kimiRootLstat(d.root, base); err == nil {
			ui.Warn(fmt.Sprintf("%q left untouched: on disk only under a spelling that differs in Unicode normalization — devexp removes only the exact name it recorded", filepath.Join(dir, base)))
			*kept = append(*kept, rel)
		}
		return false // already gone: not a removal, and not worth a line
	}
	fi, err := kimiRootLstat(d.root, base)
	switch {
	case os.IsNotExist(err):
		return false
	case err != nil:
		ui.Warn(fmt.Sprintf("%q left untouched: %v", filepath.Join(dir, base), err))
		*kept = append(*kept, rel)
		return false
	case fi.Mode()&os.ModeSymlink != 0:
		// The user's own setup, as everywhere else devexp removes.
		ui.Warn(fmt.Sprintf("%q is a symlink — left untouched (devexp never removes one)", filepath.Join(dir, base)))
		*kept = append(*kept, rel)
		return false
	case !fi.Mode().IsRegular():
		ui.Warn(fmt.Sprintf("%q left untouched: no longer installed, but it is not a regular file", filepath.Join(dir, base)))
		*kept = append(*kept, rel)
		return false
	}
	if dryRun {
		ui.DryRun("remove " + filepath.Join(dir, base))
		*kept = append(*kept, rel)
		return false
	}
	if err := d.root.Remove(base); err != nil {
		ui.Warn(fmt.Sprintf("could not remove %q (%v); it stays recorded so a later run can finish", filepath.Join(dir, base), err))
		*kept = append(*kept, rel)
		return false
	}
	ui.Removed(rel)
	return true
}

// foldMatchKimi finds a listed name that differs from want only in case.
func foldMatchKimi(want string, names map[string]bool) (string, bool) {
	for n := range names {
		if strings.EqualFold(n, want) {
			return n, true
		}
	}
	return "", false
}

func quoteAllKimi(items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = fmt.Sprintf("%q", s)
	}
	slices.Sort(out)
	return out
}

// isKimiHookPath reports whether rel has the shape of something devexp
// installs below the hooks directory: "<dir>/<file>.sh", slash-separated,
// with no traversal, no absolute path and no control character — the last
// because the name is printed and a raw escape reaches the terminal (#111).
//
// It vets the recorded string only. What is actually on disk is checked
// separately, through the pinned directory handle above: a string that looks
// right can still name a symlink, a directory, or a file that exists only
// under a different spelling.
func isKimiHookPath(rel string) bool {
	if rel == "" || rel != path.Clean(rel) || path.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return false
	}
	if strings.ContainsAny(rel, "\\") || strings.ContainsFunc(rel, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return false
	}
	dir, file := path.Split(rel)
	return (dir == "kimi/" || dir == "claude-code/") && file != "" && strings.HasSuffix(file, ".sh")
}

// pruneEmptyKimiDirs removes the two subdirectories, and then the hooks
// directory itself, once nothing devexp put there is left. Failures are
// ignored: a directory holding something else is a directory to leave alone,
// and that is exactly what a non-empty Rmdir refuses.
//
// A symlinked directory is never removed, and never pruned through: unlinking
// it would take out the user's link, and removing through it would reach into
// whatever it points at. Same rule as the removal itself.
//
// **Do not delete the BehindSymlink call because a mutation test says it is
// unreachable.** An earlier version of this comment argued it was equivalent
// — that pruning only runs after a removal, a removal needs an unblocked
// directory, and everything pruned is under the same hooksDir. That argument
// is wrong, and the review proved it: replace hooks/ with a symlink *after*
// the removals and *before* the prune, and the Lstat below follows the new
// link and sees an ordinary directory. BehindSymlink is then the only thing
// that refuses, and the user's dotfiles checkout is what gets pruned.
//
// So this covers a race the Lstat structurally cannot see — which is also why
// no single-threaded test kills the mutant. The surviving mutant is a
// limitation of the harness, not a missing assertion.
func pruneEmptyKimiDirs(home, hooksDir string) {
	prune := func(dir string) {
		fi, err := os.Lstat(dir)
		if err != nil || fi.Mode()&os.ModeSymlink != 0 {
			return
		}
		if _, behind, err := removeguard.BehindSymlink(home, dir); err != nil || behind {
			return
		}
		os.Remove(dir) //nolint:errcheck
	}
	for _, dir := range []string{"kimi", "claude-code"} {
		prune(filepath.Join(hooksDir, dir))
	}
	prune(hooksDir)
}
