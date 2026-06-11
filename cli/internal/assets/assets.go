// Package assets embeds the toolkit's agents, skills, hooks, and MCP registry
// so the devexp binary can install them without a cloned repo on disk.
//
// The embedded directories are staged from the repo root by
// scripts/stage-assets.sh — run it (or `./install.sh`, which runs it
// automatically) before building, or this package will fail to compile.
package assets

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:agents all:skills all:hooks all:mcps devexp.config.json uninstall.sh
var FS embed.FS

// ExtractFile copies srcPath out of fsys onto the real filesystem at destPath
// with the given permissions, creating parent directories as needed. It's used
// for the handful of assets (hook scripts, uninstall.sh) that must exist as
// real, executable files on disk even when the binary is running standalone
// from embedded assets rather than a cloned repo.
func ExtractFile(fsys fs.FS, srcPath, destPath string, perm os.FileMode) error {
	data, err := fs.ReadFile(fsys, srcPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, data, perm)
}
