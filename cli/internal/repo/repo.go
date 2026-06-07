// Package repo locates the devexp-toolkit's assets (agents, skills, hooks,
// MCP registry) on disk — whether from a live cloned repo or, for standalone
// binaries, from a copy of the assets embedded at build time.
package repo

import (
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
func Resolve(version string) (Source, error) {
	if dir, err := findRepoDir(); err == nil {
		return Source{RepoDir: dir}, nil
	}

	dir, err := extractEmbedded(version)
	if err != nil {
		return Source{}, fmt.Errorf("no devexp repo found on disk and failed to extract embedded assets: %w", err)
	}
	return Source{RepoDir: dir, Embedded: true}, nil
}

// ── Live repo detection ───────────────────────────────────────────────────────

func findRepoDir() (string, error) {
	if d := os.Getenv("DEVEXP_DIR"); d != "" {
		return d, nil
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
	return "", fmt.Errorf("not found")
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

// extractEmbedded materializes assets.FS onto disk under the user's cache
// directory, keyed by binary version so an upgrade gets a fresh copy. Returns
// the destination directory, reusing a prior extraction when present.
func extractEmbedded(version string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
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
