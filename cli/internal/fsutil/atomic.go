// Package fsutil holds the file-system primitives devexp's installers share,
// so every config, manifest and plugin write follows one set of rules.
package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Errors WriteFileAtomic returns, wrapped with the path, when it refuses to
// write. The file is left exactly as it was.
var (
	// ErrDanglingSymlink: the path is a symlink whose chain ends at nothing.
	// Writing would create a file wherever the link points, such as inside a
	// dotfiles checkout, which nobody asked for.
	ErrDanglingSymlink = errors.New("is a symlink to a file that does not exist")
	// ErrNotRegular: the path, or what its links resolve to, is a directory,
	// FIFO, device or socket.
	ErrNotRegular = errors.New("is not a regular file")
	// ErrReadOnly: the file exists and this user may not write it. A rename
	// in a writable directory would still replace it, silently undoing the
	// user's chmod.
	ErrReadOnly = errors.New("is not writable")
)

// accessWriteOK is access(2)'s W_OK.
const accessWriteOK = 0x2

// Indirected so tests can fail or interrupt a write at each step.
var (
	writeTemp = func(f *os.File, data []byte) error { _, err := f.Write(data); return err }
	syncFile  = (*os.File).Sync
	rename    = os.Rename
)

// WriteFileAtomic replaces the contents of the file at path with data, so a
// reader, or a crash at any point, sees the old contents or the new ones, never
// a truncated or partial file.
//
//   - A symlink is followed to its final target, and the target is replaced; the
//     link itself (and any chain of links) stays as it is. A plain rename over
//     path would turn a dotfiles-managed link into a regular file.
//   - The new file is written to a temp file in the target's own directory,
//     fsynced, given the target's permission bits (perm for a new file), and
//     renamed over the target; the directory is then fsynced, best effort.
//   - A missing path creates a new file with perm; its parent directory must
//     exist. A dangling link, a target that is not a regular file, or one this
//     user may not write is refused (ErrDanglingSymlink, ErrNotRegular,
//     ErrReadOnly).
//   - Any failure — no temp file in a read-only directory, a full disk, a rename
//     the file system refuses (EXDEV/EBUSY for a bind-mounted file) — removes
//     the temp file and leaves the target untouched.
//
// Ownership is not copied: the new file belongs to the user running devexp,
// which is the owner of every file devexp edits under that user's HOME.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	target, mode, err := resolveTarget(path, perm)
	if err != nil {
		return err
	}

	dir := filepath.Dir(target)
	f, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("%s: can't create a temp file next to it to replace it atomically, so it was left untouched: %w", target, err)
	}
	tmp := f.Name()
	closed := false
	defer func() {
		if err != nil {
			if !closed {
				f.Close() //nolint:errcheck
			}
			os.Remove(tmp) //nolint:errcheck
		}
	}()
	if err = writeTemp(f, data); err != nil {
		return fmt.Errorf("%s: write %s: %w — the file was left untouched", target, tmp, err)
	}
	if err = syncFile(f); err != nil {
		return fmt.Errorf("%s: sync %s: %w — the file was left untouched", target, tmp, err)
	}
	if err = f.Chmod(mode); err != nil {
		return fmt.Errorf("%s: chmod %s: %w — the file was left untouched", target, tmp, err)
	}
	closed = true
	if err = f.Close(); err != nil {
		return fmt.Errorf("%s: close %s: %w — the file was left untouched", target, tmp, err)
	}
	if err = rename(tmp, target); err != nil {
		if errors.Is(err, syscall.EXDEV) || errors.Is(err, syscall.EBUSY) {
			return fmt.Errorf("%s can't be replaced atomically (%w) — it may be a bind-mounted file or on another file system; it was left untouched", target, err)
		}
		return fmt.Errorf("%s: %w — the file was left untouched", target, err)
	}
	syncDir(dir)
	return nil
}

// resolveTarget returns the file WriteFileAtomic replaces for path, and the
// permission bits the new file gets.
func resolveTarget(path string, perm os.FileMode) (string, os.FileMode, error) {
	li, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, perm, nil
	}
	if err != nil {
		return "", 0, err
	}
	target := path
	if li.Mode()&os.ModeSymlink != 0 {
		target, err = filepath.EvalSymlinks(path)
		if os.IsNotExist(err) {
			return "", 0, fmt.Errorf("%s %w, so it was left untouched — create the file the link points at, or replace the link, and re-run", path, ErrDanglingSymlink)
		}
		if err != nil {
			return "", 0, fmt.Errorf("%s is a symlink that can't be resolved, so it was left untouched: %w", path, err)
		}
	}
	fi, err := os.Stat(target)
	if err != nil {
		return "", 0, err
	}
	if !fi.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%s %w, so it was left untouched", describe(path, target), ErrNotRegular)
	}
	if syscall.Access(target, accessWriteOK) != nil {
		return "", 0, fmt.Errorf("%s %w, so it was left untouched", describe(path, target), ErrReadOnly)
	}
	return target, fi.Mode().Perm(), nil
}

// describe names path, and the file it resolves to when that differs.
func describe(path, target string) string {
	if path == target {
		return path
	}
	return fmt.Sprintf("%s (a symlink to %s)", path, target)
}

// syncDir fsyncs dir so the rename itself survives a crash. Not every file
// system supports it; the rename has already happened either way.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	d.Sync()  //nolint:errcheck
	d.Close() //nolint:errcheck
}
