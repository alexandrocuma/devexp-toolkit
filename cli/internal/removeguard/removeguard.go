// Package removeguard decides whether devexp may remove anything from one of
// its target directories. It is removal-only: installs still write through a
// symlinked directory (a dotfiles setup), but nothing is ever removed through
// one (#108, #128).
package removeguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BehindSymlink reports whether dir, a target directory under home such as
// home/.claude/agents, resolves anywhere other than where its path says: dir
// itself, or a directory between home and dir (~/.claude, ~/.config,
// ~/.config/opencode), is a symlink. resolved is where dir really is.
//
// Both sides are resolved from home, so a symlink above home cancels out:
// macOS /var -> /private/var, or a linked /home, never makes a directory count
// as behind a symlink. Every devexp target is built from HOME
// (cli/cmd/paths.go); there is no other configured config directory.
//
// An error means dir can't be checked, and callers must not remove from it.
// dir must exist.
func BehindSymlink(home, dir string) (resolved string, behind bool, err error) {
	rel, err := filepath.Rel(home, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false, fmt.Errorf("%s is not under %s", dir, home)
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", false, err
	}
	resolved, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false, err
	}
	return resolved, resolved != filepath.Join(realHome, rel), nil
}

// IsSymlink reports whether path itself, not what it resolves to, is a
// symlink. A path that can't be checked is not reported as one.
func IsSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}
