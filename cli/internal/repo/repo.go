// Package repo locates the devexp-toolkit's assets (agents, skills, hooks,
// MCP registry) on disk — whether from a devexp-toolkit checkout or, for
// standalone binaries, from a copy of the assets embedded at build time.
package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"devexp/internal/assets"
)

// Source describes where devexp should read its assets from. RepoDir always
// points at a real directory on disk containing agents/, skills/, hooks/, and
// mcps/ — either a live devexp-toolkit checkout (Embedded == false) or a
// materialized copy of the binary's embedded assets (Embedded == true) — so
// existing file-based installers work unchanged in both cases. Origin says, in
// words, how RepoDir was chosen.
type Source struct {
	RepoDir  string
	Embedded bool
	Origin   string
}

// Origins, as reported to the user before anything is installed.
const (
	OriginDevexpDir  = "DEVEXP_DIR"
	OriginBinaryDir  = "the devexp-toolkit checkout this binary was built in"
	OriginWorkingDir = "the devexp-toolkit checkout containing the current directory"
	OriginEmbedded   = "assets bundled in this binary, extracted to the user cache"
)

// devBuild is the version of a binary built without goreleaser's -X flag
// (`go build`, `go run`, `go test`); every other value is a tagged build.
const devBuild = "dev"

// Resolve finds the devexp-toolkit checkout to read assets from. If none is
// found it extracts the assets embedded in the binary at build time to a
// per-version cache directory and uses that instead.
//
// Where it looks depends on the build (#134):
//   - every build: DEVEXP_DIR, when set;
//   - dev builds only: the directory above the binary (bin/devexp in a clone,
//     as built by install.sh), then each directory from the cwd up.
//
// A tagged release build never adopts a checkout it merely finds on disk: it
// carries its release's assets, and DEVEXP_DIR is how to point it at a clone.
// Whatever the lookup, a directory counts only if it is a devexp-toolkit
// checkout (isRepoDir), not merely one with the same directory names.
//
// A DEVEXP_DIR that isn't a checkout is an error, not a reason to look
// elsewhere: the user named a directory, and silently installing from another
// one would hide the mistake.
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
			return Source{}, fmt.Errorf("no devexp-toolkit checkout found on disk and failed to extract embedded assets: %w", err)
		}
	}
	return src, nil
}

// locate decides Resolve's Source without writing anything: a checkout found
// by findRepoDir, else the directory the embedded assets are extracted to.
func locate(version string) (Source, error) {
	dir, origin, err := findRepoDir(version != devBuild)
	switch {
	case err == nil:
		return absoluteSource(Source{RepoDir: dir, Origin: origin})
	case !errors.Is(err, errRepoNotFound):
		return Source{}, err
	}
	dir, err = embeddedDir()
	if err != nil {
		return Source{}, fmt.Errorf("no devexp-toolkit checkout found on disk and failed to extract embedded assets: %w", err)
	}
	return absoluteSource(Source{RepoDir: dir, Embedded: true, Origin: OriginEmbedded})
}

// absoluteSource refuses a Source whose RepoDir is not absolute.
func absoluteSource(src Source) (Source, error) {
	if !filepath.IsAbs(src.RepoDir) {
		return Source{}, fmt.Errorf("devexp asset dir %q is not an absolute path", src.RepoDir)
	}
	return src, nil
}

// ── Live repo detection ───────────────────────────────────────────────────────

// errRepoNotFound means no live repo was found, so Resolve may fall back to the
// embedded assets. Any other findRepoDir error stops Resolve.
var errRepoNotFound = errors.New("no devexp-toolkit checkout found")

// markerFile identifies a devexp-toolkit checkout: a regular file at its root
// whose first line is markerID. It is committed to the repo and embedded in
// the binary, so the extracted assets are a checkout too.
const (
	markerFile = ".devexp-toolkit"
	markerID   = "devexp-toolkit"
)

// executable is indirected so tests can place the binary inside a checkout.
var executable = os.Executable

// findRepoDir returns the checkout to use and how it was found. tagged
// disables the lookups next to the binary and up from the cwd (see Resolve).
func findRepoDir(tagged bool) (dir, origin string, err error) {
	if d := os.Getenv("DEVEXP_DIR"); d != "" {
		abs, err := filepath.Abs(d)
		if err != nil {
			return "", "", fmt.Errorf("DEVEXP_DIR %q: %w", d, err)
		}
		if !isRepoDir(abs) {
			return "", "", fmt.Errorf("DEVEXP_DIR is %q (%s), which is not a devexp-toolkit checkout (it needs the %s marker file, agents/, skills/ and mcps/) — point it at a devexp-toolkit clone or unset it", d, abs, markerFile)
		}
		return abs, OriginDevexpDir, nil
	}
	if tagged {
		return "", "", errRepoNotFound
	}
	if exe, err := executable(); err == nil {
		if candidate := filepath.Dir(filepath.Dir(exe)); isRepoDir(candidate) {
			return candidate, OriginBinaryDir, nil
		}
	}
	cwd, _ := os.Getwd()
	for dir := cwd; ; {
		if isRepoDir(dir) {
			return dir, OriginWorkingDir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", errRepoNotFound
}

// isRepoDir reports whether dir is a devexp-toolkit checkout: it holds the
// marker file — a regular file, not a symlink, whose first line is markerID —
// and agents/, skills/ and mcps/. The directory names alone are not enough;
// plenty of other projects have them.
func isRepoDir(dir string) bool {
	if !hasMarker(filepath.Join(dir, markerFile)) {
		return false
	}
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
// the destination directory, reusing a prior extraction when present.
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

	if data, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(data)) == version {
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
