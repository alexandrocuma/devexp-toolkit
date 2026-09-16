// Package repo locates the devexp-toolkit's assets (agents, skills, hooks,
// MCP registry) on disk — whether from a live cloned repo or, for standalone
// binaries, from a copy of the assets embedded at build time.
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
// mcps/ — either a live cloned repo (Embedded == false) or a materialized copy
// of the binary's embedded assets (Embedded == true) — so existing file-based
// installers work unchanged in both cases.
type Source struct {
	RepoDir  string
	Embedded bool
}

// Resolve finds a live devexp-toolkit repo on disk (DEVEXP_DIR env var, next to
// the running binary, or by walking up from the current directory). If none is
// found — e.g. a standalone binary downloaded outside of a clone — it
// extracts the assets embedded in the binary at build time to a per-version
// cache directory and uses that instead.
//
// A DEVEXP_DIR that isn't a devexp repo is an error, not a reason to look
// elsewhere: the user named a directory, and silently installing from another
// one would hide the mistake.
//
// RepoDir is always absolute. Installers build paths that outlive this process
// from it — Claude Code hook commands are repoDir/<script> — and a relative one
// would resolve against whatever directory those are later run from (#126).
func Resolve(version string) (Source, error) {
	dir, err := findRepoDir()
	switch {
	case err == nil:
		return absoluteSource(Source{RepoDir: dir})
	case !errors.Is(err, errRepoNotFound):
		return Source{}, err
	}

	dir, err = extractEmbedded(version)
	if err != nil {
		return Source{}, fmt.Errorf("no devexp repo found on disk and failed to extract embedded assets: %w", err)
	}
	return absoluteSource(Source{RepoDir: dir, Embedded: true})
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
var errRepoNotFound = errors.New("no devexp repo found")

func findRepoDir() (string, error) {
	if d := os.Getenv("DEVEXP_DIR"); d != "" {
		abs, err := filepath.Abs(d)
		if err != nil {
			return "", fmt.Errorf("DEVEXP_DIR %q: %w", d, err)
		}
		if !isRepoDir(abs) {
			return "", fmt.Errorf("DEVEXP_DIR is %q (%s), which is not a devexp repo (it needs agents/, skills/ and mcps/) — point it at a devexp-toolkit clone or unset it", d, abs)
		}
		return abs, nil
	}
	if exe, err := os.Executable(); err == nil {
		if candidate := filepath.Dir(filepath.Dir(exe)); isRepoDir(candidate) {
			return candidate, nil
		}
	}
	cwd, _ := os.Getwd()
	for dir := cwd; ; {
		if isRepoDir(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errRepoNotFound
}

func isRepoDir(dir string) bool {
	for _, sub := range []string{"agents", "skills", "mcps"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); err != nil {
			return false
		}
	}
	return true
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
	base, err := userCacheDir()
	if err != nil {
		return "", fmt.Errorf("no user cache dir to extract the bundled assets to (%v) — set HOME (or XDG_CACHE_HOME) to an absolute path and re-run", err)
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("user cache dir %q is not an absolute path, refusing to extract the bundled assets there — set HOME (or XDG_CACHE_HOME) to an absolute path and re-run", base)
	}
	dest := filepath.Join(base, "devexp", "assets")
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
