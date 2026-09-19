// Package repo locates the devexp-toolkit's assets (agents, skills, hooks,
// MCP registry) on disk — whether from a devexp-toolkit checkout or, for
// standalone binaries, from a copy of the assets embedded at build time.
package repo

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"devexp/internal/assets"
)

// Source describes where devexp should read its assets from. RepoDir always
// points at a real directory on disk containing agents/, skills/, hooks/, and
// mcps/ — either a live devexp-toolkit checkout (Embedded == false) or a
// materialized copy of the binary's embedded assets (Embedded == true) — so
// existing file-based installers work unchanged in both cases. Origin says, in
// words, how RepoDir was chosen. Warning, when set, explains why a dev build
// is not using the checkout it was built from.
type Source struct {
	RepoDir  string
	Embedded bool
	Origin   string
	Warning  string
}

// Origins, as reported to the user before anything is installed.
const (
	OriginDevexpDir = "DEVEXP_DIR"
	OriginSourceDir = "the devexp-toolkit checkout this binary was built from"
	OriginEmbedded  = "assets bundled in this binary, extracted to the user cache"
)

// devBuild is the version of a binary built without goreleaser's -X flag
// (`go build`, `go run`, `go test`); every other value is a tagged build.
const devBuild = "dev"

// Resolve decides which assets to install from:
//   - DEVEXP_DIR, when set, in every build. It must be a devexp-toolkit
//     checkout; otherwise Resolve fails rather than look elsewhere, since the
//     user named a directory and silently installing from another would hide
//     the mistake.
//   - dev builds only: the checkout the binary was compiled from (sourceRoot),
//     if it still is a devexp-toolkit checkout. This is what ./install.sh's
//     bin/devexp, `go run` and `go test` use.
//   - otherwise the assets embedded in the binary, extracted to a cache dir.
//
// No other directory on disk is ever used: not the one the binary sits in, and
// not the current directory or its parents. Release builds therefore only use
// DEVEXP_DIR or their bundled assets, and dev builds only DEVEXP_DIR, their own
// source checkout, or their bundled assets. The marker file (isRepoDir) tells a
// devexp-toolkit checkout apart from other directories; it is not what decides
// which directory is trusted — that is the rule above.
//
// announce, when non-nil, is called with the chosen Source once it is decided
// and before Resolve writes anything (extracting the embedded assets), so the
// caller can show which asset root is used.
//
// RepoDir is always absolute. Installers build paths that outlive this process
// from it — Claude Code hook commands are repoDir/<script> — and a relative one
// would resolve against whatever directory those are later run from.
func Resolve(version string, announce func(Source)) (Source, error) {
	src, err := locate(version)
	if err != nil {
		return Source{}, err
	}
	if announce != nil {
		announce(src)
	}
	if src.Embedded {
		if _, err := extractEmbedded(version); err != nil {
			return Source{}, fmt.Errorf("failed to extract the assets bundled in this binary: %w", err)
		}
	}
	return src, nil
}

// locate decides Resolve's Source without writing anything.
func locate(version string) (Source, error) {
	if d := os.Getenv("DEVEXP_DIR"); d != "" {
		dir, err := devexpDir(d)
		if err != nil {
			return Source{}, err
		}
		return absoluteSource(Source{RepoDir: dir, Origin: OriginDevexpDir})
	}
	var warning string
	if version == devBuild {
		var dir string
		if dir, warning = sourceCheckout(); dir != "" {
			return absoluteSource(Source{RepoDir: dir, Origin: OriginSourceDir})
		}
	}
	dir, err := embeddedDir(version)
	if err != nil {
		return Source{}, fmt.Errorf("failed to extract the assets bundled in this binary: %w", err)
	}
	return absoluteSource(Source{RepoDir: dir, Embedded: true, Origin: OriginEmbedded, Warning: warning})
}

// absoluteSource refuses a Source whose RepoDir is not absolute.
func absoluteSource(src Source) (Source, error) {
	if !filepath.IsAbs(src.RepoDir) {
		return Source{}, fmt.Errorf("devexp asset dir %q is not an absolute path", src.RepoDir)
	}
	return src, nil
}

// ── Live checkout detection ───────────────────────────────────────────────────

// markerFile identifies a devexp-toolkit checkout: a regular file at its root
// whose first line is markerID. It is committed to the repo and embedded in
// the binary, so the extracted assets are a checkout too.
const (
	markerFile = ".devexp-toolkit"
	markerID   = "devexp-toolkit"
)

// devexpDir resolves DEVEXP_DIR to an absolute path and requires a checkout.
func devexpDir(d string) (string, error) {
	abs, err := filepath.Abs(d)
	if err != nil {
		return "", fmt.Errorf("DEVEXP_DIR %q: %w", d, err)
	}
	if !isRepoDir(abs) {
		return "", fmt.Errorf("DEVEXP_DIR is %q (%s), which is not a devexp-toolkit checkout (it needs the %s marker file, agents/, skills/ and mcps/) — point it at a devexp-toolkit clone or unset it", d, abs, markerFile)
	}
	return abs, nil
}

// sourceRoot returns the root of the checkout this binary was compiled from,
// or "" when that is unknown. It is indirected so tests can stand in for
// binaries built elsewhere.
var sourceRoot = func() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return sourceRootOf(file)
}

// sourceFileInRepo is where this file lives in a devexp-toolkit checkout.
var sourceFileInRepo = filepath.Join("cli", "internal", "repo", "repo.go")

// sourceRootOf derives the checkout root from this file's compile-time path.
// `go build`, `go run` and `go test` record an absolute path; a -trimpath
// build records a module-relative one, which names no directory, so it yields
// "" and the binary uses its bundled assets.
func sourceRootOf(file string) string {
	suffix := string(filepath.Separator) + sourceFileInRepo
	if !filepath.IsAbs(file) || !strings.HasSuffix(file, suffix) {
		return ""
	}
	return strings.TrimSuffix(file, suffix)
}

// sourceCheckout returns this dev build's source checkout when it is a
// devexp-toolkit checkout that verifiably belongs to the user (ownedCheckout).
// Otherwise dir is "", and warning explains why the checkout is skipped when
// there is one to explain: it can't be verified as the user's, it is gone
// (moved or deleted since the build), or it lacks the marker (a clone or fork
// from before the marker existed).
func sourceCheckout() (dir, warning string) {
	root := sourceRoot()
	switch {
	case root == "":
		return "", ""
	case isRepoDir(root):
		if err := ownedCheckout(root); err != nil {
			return "", fmt.Sprintf("the checkout this binary was built from, %s, can't be verified as yours (%v) — using the assets bundled in this binary instead; to install from it, make its files owned by you, not symlinks, and not writable by others or by a group other than your own, or set DEVEXP_DIR to it", root, err)
		}
		return root, ""
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return "", fmt.Sprintf("the devexp-toolkit checkout this binary was built from, %s, no longer exists — using the assets bundled in this binary instead; rebuild devexp from your clone (rm bin/devexp && ./install.sh) to install from it", root)
	}
	if hasAssetDirs(root) {
		return "", fmt.Sprintf("%s, the checkout this binary was built from, has agents/, skills/ and mcps/ but no valid %s marker file, so it was skipped — using the assets bundled in this binary instead; pull the latest devexp-toolkit (or restore %s) to install from it", root, markerFile, markerFile)
	}
	return "", ""
}

// fileIDs, currentUID and privateGroup are indirected so tests can stand in
// for files owned by someone else and for other group setups.
var (
	fileIDs      = statIDs
	currentUID   = os.Getuid
	privateGroup = lookupPrivateGroup
)

// lookupPrivateGroup returns the gid of the current user's private group: the
// user's primary group, when that group has the user's name (the "user private
// group" convention, where a umask of 002 is the default). ok is false when
// the primary group is named otherwise or a lookup fails.
func lookupPrivateGroup() (gid uint32, ok bool) {
	u, err := user.Current()
	if err != nil || u.Username == "" {
		return 0, false
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil || g.Name != u.Username {
		return 0, false
	}
	id, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(id), true
}

// checkoutWalks are the trees of a checkout ownedCheckout checks entry by
// entry: what is installed from it and where hook scripts and the hooks
// registry live. checkoutFiles are files at the root that are read or run.
// Entries marked optional may be absent.
var (
	checkoutWalks = []struct {
		rel      string
		optional bool
	}{
		{rel: "agents"},
		{rel: "skills"},
		{rel: "mcps"},
		{rel: "hooks", optional: true},
	}
	checkoutFiles = []string{markerFile, "devexp.config.json", "uninstall.sh"}
)

// ownership holds what ownedCheckout compares files against.
type ownership struct {
	uid      uint32
	upg      uint32 // the user's private group, when upgOK
	upgOK    bool
	upgKnown bool
}

// groupWriteOK reports whether group write on a file with gid is acceptable:
// only when gid is the user's private group, which no one else is in.
func (o *ownership) groupWriteOK(gid uint32) bool {
	if !o.upgKnown {
		o.upg, o.upgOK = privateGroup()
		o.upgKnown = true
	}
	return o.upgOK && gid == o.upg
}

// check verifies one entry: not a symlink, owned by the user, not writable by
// others, and writable by group only through the user's private group.
func (o *ownership) check(path string, fi os.FileInfo) error {
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", path)
	}
	uid, gid, ok := fileIDs(fi)
	if !ok || uid != o.uid {
		return fmt.Errorf("%s is not owned by you", path)
	}
	return o.checkWrite(path, fi, gid)
}

func (o *ownership) checkWrite(path string, fi os.FileInfo, gid uint32) error {
	perm := fi.Mode().Perm()
	if perm&0o002 != 0 || perm&0o020 != 0 && !o.groupWriteOK(gid) {
		return fmt.Errorf("%s is writable by group or others", path)
	}
	return nil
}

// ownedCheckout verifies that the checkout at root (a path recorded when the
// binary was built) is the user's, so a different directory that appeared at
// that path since is not used. Unix only; elsewhere it always fails.
//
//   - With symlinks resolved, the root, every entry under agents/, skills/,
//     mcps/ and hooks/, and the root files in checkoutFiles must not be
//     symlinks, must be owned by the current user, must not be writable by
//     others, and may be writable by group only when that group is the user's
//     private group (lookupPrivateGroup).
//   - The directory holding the resolved root must be owned by the user or by
//     root, and not writable by others or by a group other than the user's
//     private group — unless it is sticky, so nobody else can rename or
//     replace the entry.
//   - If the recorded root is itself a symlink, the link must be owned by the
//     user or by root, and the directory holding it must pass the same check.
func ownedCheckout(root string) error {
	uid := currentUID()
	if uid < 0 {
		return fmt.Errorf("file ownership is not available on this platform")
	}
	o := &ownership{uid: uint32(uid)}
	link, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if link.Mode()&os.ModeSymlink != 0 {
		if owner, _, ok := fileIDs(link); !ok || (owner != o.uid && owner != 0) {
			return fmt.Errorf("%s is a symlink owned by someone else", root)
		}
		if err := o.safeParent(filepath.Dir(root)); err != nil {
			return err
		}
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	if err := o.safeParent(filepath.Dir(resolved)); err != nil {
		return err
	}
	fi, err := os.Lstat(resolved)
	if err != nil {
		return err
	}
	if err := o.check(resolved, fi); err != nil {
		return err
	}
	for _, name := range checkoutFiles {
		p := filepath.Join(resolved, name)
		fi, err := os.Lstat(p)
		if os.IsNotExist(err) && name != markerFile {
			continue
		}
		if err != nil {
			return err
		}
		if err := o.check(p, fi); err != nil {
			return err
		}
	}
	for _, w := range checkoutWalks {
		top := filepath.Join(resolved, w.rel)
		if _, err := os.Lstat(top); w.optional && os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			return o.check(p, fi)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// safeParent checks the directory holding a checkout: owned by the user or
// root, and not writable by others or by a group other than the user's private
// group, unless it is sticky.
func (o *ownership) safeParent(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	owner, gid, ok := fileIDs(fi)
	if !ok || (owner != o.uid && owner != 0) {
		return fmt.Errorf("%s is not owned by you or root", dir)
	}
	if fi.Mode()&os.ModeSticky != 0 {
		return nil
	}
	return o.checkWrite(dir, fi, gid)
}

// isRepoDir reports whether dir is a devexp-toolkit checkout: it holds the
// marker file — a regular file, not a symlink, whose first line is markerID —
// and agents/, skills/ and mcps/.
func isRepoDir(dir string) bool {
	return hasMarker(filepath.Join(dir, markerFile)) && hasAssetDirs(dir)
}

// hasAssetDirs reports whether dir has agents/, skills/ and mcps/.
func hasAssetDirs(dir string) bool {
	for _, sub := range []string{"agents", "skills", "mcps"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); err != nil {
			return false
		}
	}
	return true
}

// hasMarker reports whether path is a regular file, of at most 4 KiB, whose
// first line is markerID.
func hasMarker(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 4096 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	first, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimRight(first, "\r") == markerID
}

// ── Embedded asset extraction ─────────────────────────────────────────────────

// userCacheDir is indirected so tests can redirect extraction into a temp
// directory. Writing to the real user cache from a test would leave artifacts
// on the developer's machine and make the result depend on what was already
// cached there.
var userCacheDir = os.UserCacheDir

// extractEmbedded materializes assets.FS onto disk under the user's cache
// directory and returns that directory (embeddedDir). A tagged build reuses a
// prior extraction of the same version; a dev build — every one is version
// "dev", whatever it embeds — always extracts afresh, into its own directory,
// so it never touches the one a release install's hooks point into.
//
// The directory is a symlink to a sibling holding one complete extraction.
// A new extraction goes into a fresh sibling and is swapped in by renaming a
// symlink over the old one, which is atomic: a failed or interrupted
// extraction leaves the previous tree as it was, and concurrent runs each
// swap in a complete tree, so the directory always points at a complete one.
// The sibling it pointed at before is then removed.
//
// With no usable cache directory it refuses rather than fall back. The
// old fallback, os.TempDir(), was either a relative $TMPDIR — a directory under
// wherever devexp runs, wiped and re-extracted — or the shared /tmp, which is
// not private to the user yet was reused whenever its version marker matched.
// A relative cache dir (a relative HOME, or XDG_CACHE_HOME on Linux) is
// refused for the same reason.
func extractEmbedded(version string) (string, error) {
	dest, err := embeddedDir(version)
	if err != nil {
		return "", err
	}
	parent, name := filepath.Split(dest)
	if version != devBuild {
		data, err := os.ReadFile(filepath.Join(dest, versionFile))
		if err == nil && strings.TrimSpace(string(data)) == version {
			sweepStale(parent, name)
			return dest, nil
		}
	}

	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	tree, err := os.MkdirTemp(parent, "."+name+".")
	if err != nil {
		return "", err
	}
	link := tree + ".link"
	swapped := false
	defer func() {
		if !swapped {
			os.RemoveAll(tree) //nolint:errcheck
			os.Remove(link)    //nolint:errcheck
		}
	}()
	if err := os.Chmod(tree, 0o755); err != nil {
		return "", err
	}
	if err := extractTree(assets.FS, tree); err != nil {
		return "", err
	}
	// Only a tree whose extraction succeeded is ever swapped in, so the version
	// file read through dest always describes a complete tree.
	if err := os.WriteFile(filepath.Join(tree, versionFile), []byte(version), 0o644); err != nil {
		return "", err
	}

	previous, _ := os.Readlink(dest)
	if err := os.Symlink(filepath.Base(tree), link); err != nil {
		return "", err
	}
	// retired is the tree dest pointed at before the swap. It is not removed
	// here: hooks may be starting from it right now. Its mtime is refreshed so
	// sweepStale removes it only after staleAfter.
	retired := ""
	if prev := filepath.Base(previous); previous == prev && strings.HasPrefix(prev, "."+name+".") && prev != filepath.Base(tree) {
		retired = filepath.Join(parent, prev)
	}
	movedAside := ""
	if fi, err := os.Lstat(dest); err == nil && fi.Mode()&os.ModeSymlink == 0 {
		// A plain directory, from a version that extracted in place: move it
		// aside so the symlink can take its name, into a sibling sweepStale
		// removes later like any retired tree.
		old, err := os.MkdirTemp(parent, "."+name+".")
		if err != nil {
			return "", err
		}
		if err := rename(dest, filepath.Join(old, name)); err != nil {
			os.Remove(old) //nolint:errcheck
			if !os.IsNotExist(err) {
				return "", err
			}
		} else {
			movedAside = filepath.Join(old, name)
			retired = old
		}
	}
	if err := rename(link, dest); err != nil {
		if movedAside != "" {
			// Put the only complete tree back rather than leave dest missing.
			if rename(movedAside, dest) == nil {
				os.Remove(filepath.Dir(movedAside)) //nolint:errcheck
			}
		}
		return "", err
	}
	swapped = true
	if retired != "" {
		t := time.Now()
		os.Chtimes(retired, t, t) //nolint:errcheck
	}
	sweepStale(parent, name)
	return dest, nil
}

// rename is os.Rename, indirected so tests can fail the swap.
var rename = os.Rename

// staleAfter is how long an extraction dest no longer points at is kept
// before sweepStale removes it. A younger one may belong to a run still in
// progress, or be a retired tree that hooks started just before a swap are
// still running from; hooks can run for minutes (on-save hooks run test
// suites), and a kept tree costs about a megabyte.
const staleAfter = time.Hour

// sweepStale removes extractions next to parent/name that it doesn't point at
// and that are older than staleAfter: trees retired by a swap, and leftovers
// of runs that were killed or that raced with another run and lost.
func sweepStale(parent, name string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, "."+name+".") {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < staleAfter {
			continue
		}
		if target, _ := os.Readlink(filepath.Join(parent, name)); target == n {
			continue
		}
		os.RemoveAll(filepath.Join(parent, n)) //nolint:errcheck
	}
}

// versionFile records which version an extraction is of.
const versionFile = ".devexp-version"

// extractTree is extractFS, indirected so tests can interrupt an extraction.
var extractTree = extractFS

// embeddedDir is where extractEmbedded puts the assets: devexp/assets under the
// user cache dir, which must be absolute, or devexp/assets-dev for a dev build.
// It writes nothing.
func embeddedDir(version string) (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", fmt.Errorf("no user cache dir to extract the bundled assets to (%v) — set HOME (or XDG_CACHE_HOME) to an absolute path and re-run", err)
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("user cache dir %q is not an absolute path, refusing to extract the bundled assets there — set HOME (or XDG_CACHE_HOME) to an absolute path and re-run", base)
	}
	name := "assets"
	if version == devBuild {
		name = "assets-dev"
	}
	return filepath.Join(base, "devexp", name), nil
}

// extractFS writes every file in fsys into dest, an existing directory,
// preserving directory structure and marking shell scripts executable.
//
// Directories are created one level at a time (WalkDir visits a directory
// before its contents), never with MkdirAll: if dest is removed mid-run — a
// stalled extraction swept by another run — extraction fails instead of
// quietly recreating part of the tree.
func extractFS(fsys fs.FS, dest string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if path == "." {
			fi, err := os.Stat(dest)
			if err == nil && !fi.IsDir() {
				err = fmt.Errorf("%s is not a directory", dest)
			}
			return err
		}
		if d.IsDir() {
			return os.Mkdir(target, 0o755)
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		perm := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			perm = 0o755
		}
		return os.WriteFile(target, data, perm)
	})
}
