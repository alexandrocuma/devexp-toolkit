// Package repo locates the devexp-toolkit's assets (agents, skills, hooks,
// MCP registry) on disk — whether from a devexp-toolkit checkout or, for
// standalone binaries, from a copy of the assets embedded at build time.
package repo

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

// Resolve decides which assets to install from (#134):
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
// would resolve against whatever directory those are later run from (#126).
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
			return "", fmt.Sprintf("the checkout this binary was built from, %s, can't be verified as yours (%v) — using the assets bundled in this binary instead; to install from it, make it owned by you and not writable by group or others, or set DEVEXP_DIR to it", root, err)
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

// ownerOf and currentUID are indirected so tests can stand in for files owned
// by someone else.
var (
	ownerOf    = statOwner
	currentUID = os.Getuid
)

// ownedDirs are the parts of a checkout ownedCheckout requires to be the
// user's: the root, what is installed from it, and where hook scripts and the
// hooks registry live. Entries marked optional may be absent.
var ownedDirs = []struct {
	rel      string
	optional bool
}{
	{rel: "."},
	{rel: "agents"},
	{rel: "skills"},
	{rel: "mcps"},
	{rel: "hooks", optional: true},
	{rel: filepath.Join("hooks", "claude-code"), optional: true},
	{rel: filepath.Join("hooks", "opencode"), optional: true},
}

// ownedCheckout verifies that the checkout at root (a path recorded when the
// binary was built) is the user's, so a different directory that appeared at
// that path since is not used. Unix only; elsewhere it always fails.
//
//   - With symlinks resolved, the root and each of ownedDirs must be a real
//     directory, owned by the current user and not writable by group or others.
//   - The directory holding the resolved root must be owned by the user or by
//     root, and either not writable by group or others or sticky (so nobody else
//     can rename or replace the entry).
//   - If the recorded root is itself a symlink, the link must be owned by the
//     user or by root, and the directory holding it must pass the same check.
func ownedCheckout(root string) error {
	uid := currentUID()
	if uid < 0 {
		return fmt.Errorf("file ownership is not available on this platform")
	}
	link, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if link.Mode()&os.ModeSymlink != 0 {
		if owner, ok := ownerOf(link); !ok || (owner != uint32(uid) && owner != 0) {
			return fmt.Errorf("%s is a symlink owned by someone else", root)
		}
		if err := safeParent(filepath.Dir(root), uint32(uid)); err != nil {
			return err
		}
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	if err := safeParent(filepath.Dir(resolved), uint32(uid)); err != nil {
		return err
	}
	for _, d := range ownedDirs {
		p := filepath.Join(resolved, d.rel)
		fi, err := os.Lstat(p)
		switch {
		case d.optional && os.IsNotExist(err):
			continue
		case err != nil:
			return err
		case !fi.IsDir():
			return fmt.Errorf("%s is not a directory", p)
		}
		if owner, ok := ownerOf(fi); !ok || owner != uint32(uid) {
			return fmt.Errorf("%s is not owned by you", p)
		}
		if fi.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("%s is writable by group or others", p)
		}
	}
	return nil
}

// safeParent checks the directory holding a checkout: owned by uid or root,
// and not writable by group or others unless it is sticky.
func safeParent(dir string, uid uint32) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if owner, ok := ownerOf(fi); !ok || (owner != uid && owner != 0) {
		return fmt.Errorf("%s is not owned by you or root", dir)
	}
	if fi.Mode().Perm()&0o022 != 0 && fi.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("%s is writable by group or others", dir)
	}
	return nil
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
// With no usable cache directory it refuses rather than fall back (#126). The
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
	if version != devBuild {
		data, err := os.ReadFile(filepath.Join(dest, versionFile))
		if err == nil && strings.TrimSpace(string(data)) == version {
			return dest, nil
		}
	}

	parent, name := filepath.Split(dest)
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
	if fi, err := os.Lstat(dest); err == nil && fi.Mode()&os.ModeSymlink == 0 {
		// A plain directory, from a version that extracted in place: move it
		// aside so the symlink can take its name.
		old, err := os.MkdirTemp(parent, "."+name+".")
		if err != nil {
			return "", err
		}
		if err := os.Rename(dest, filepath.Join(old, name)); err != nil && !os.IsNotExist(err) {
			os.Remove(old) //nolint:errcheck
			return "", err
		}
		defer os.RemoveAll(old) //nolint:errcheck
	}
	if err := os.Rename(link, dest); err != nil {
		return "", err
	}
	swapped = true
	if prev := filepath.Base(previous); previous == prev && strings.HasPrefix(prev, "."+name+".") && prev != filepath.Base(tree) {
		os.RemoveAll(filepath.Join(parent, prev)) //nolint:errcheck
	}
	sweepStale(parent, name)
	return dest, nil
}

// staleAfter is how old a leftover extraction must be before sweepStale
// removes it; a younger one may belong to a run still in progress.
const staleAfter = time.Hour

// sweepStale removes extractions next to parent/name that it doesn't point at
// and that are older than staleAfter: leftovers of runs that were killed, or
// that raced with another run and lost.
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

// extractFS writes every file in fsys to dest, preserving directory structure
// and marking shell scripts executable.
func extractFS(fsys fs.FS, dest string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		perm := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			perm = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, perm)
	})
}
