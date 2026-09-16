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
	dir, err := embeddedDir()
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
// devexp-toolkit checkout. Otherwise dir is "", and warning explains why the
// checkout is skipped when there is one to explain: it is gone (moved or
// deleted since the build), or it lacks the marker (a clone or fork from
// before the marker existed).
func sourceCheckout() (dir, warning string) {
	root := sourceRoot()
	switch {
	case root == "":
		return "", ""
	case isRepoDir(root):
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
// directory, keyed by binary version so an upgrade gets a fresh copy. Returns
// the destination directory, reusing a prior extraction of the same tagged
// version; a dev build always extracts afresh.
//
// With no usable cache directory it refuses rather than fall back (#126). The
// old fallback, os.TempDir(), was either a relative $TMPDIR — a directory under
// wherever devexp runs, wiped and re-extracted — or the shared /tmp, which is
// not private to the user yet was reused whenever its version marker matched.
// A relative cache dir (a relative HOME, or XDG_CACHE_HOME on Linux) is
// refused for the same reason.
func extractEmbedded(version string) (string, error) {
	dest, err := embeddedDir()
	if err != nil {
		return "", err
	}
	marker := filepath.Join(dest, ".devexp-version")

	// A dev build's version says nothing about which assets it embeds — every
	// one is "dev" — so it never reuses an extraction.
	if data, err := os.ReadFile(marker); err == nil && version != devBuild && strings.TrimSpace(string(data)) == version {
		return dest, nil
	}

	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := extractFS(assets.FS, dest); err != nil {
		return "", err
	}
	if err := os.WriteFile(marker, []byte(version), 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

// embeddedDir is where extractEmbedded puts the assets: devexp/assets under the
// user cache dir, which must be absolute. It writes nothing.
func embeddedDir() (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", fmt.Errorf("no user cache dir to extract the bundled assets to (%v) — set HOME (or XDG_CACHE_HOME) to an absolute path and re-run", err)
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("user cache dir %q is not an absolute path, refusing to extract the bundled assets there — set HOME (or XDG_CACHE_HOME) to an absolute path and re-run", base)
	}
	return filepath.Join(base, "devexp", "assets"), nil
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
