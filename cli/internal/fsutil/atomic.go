// Package fsutil holds the file-system primitives devexp's installers share,
// so every config, manifest and plugin write follows one set of rules.
package fsutil

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
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
	writeTemp  = func(f *os.File, data []byte) error { _, err := f.Write(data); return err }
	syncFile   = (*os.File).Sync
	tempSuffix = func() string { return strconv.FormatUint(rand.Uint64(), 36) }
	rename     = os.Rename
)

// WriteFileAtomic replaces the contents of the file at path with data, so a
// reader, or a crash at any point, sees the old contents or the new ones, never
// a truncated or partial file.
//
//   - A symlink is followed to its final target, and the target is replaced; the
//     link itself (and any chain of links) stays as it is. A plain rename over
//     path would turn a dotfiles-managed link into a regular file.
//   - The new file is written to a temp file in the target's own directory,
//     fsynced, given the target's permission bits, and renamed over the target;
//     the directory is then fsynced, best effort.
//   - A missing path creates a new file with perm minus the umask, as
//     os.WriteFile does: the temp file is created with perm and never chmodded,
//     so under umask 077 a config holding tokens stays private. Its parent
//     directory must exist. A dangling link, a target that is not a regular file, or one this
//     user may not write is refused (ErrDanglingSymlink, ErrNotRegular,
//     ErrReadOnly).
//   - Any failure — no temp file in a read-only directory, a full disk, a rename
//     the file system refuses (EXDEV/EBUSY for a bind-mounted file) — removes
//     the temp file and leaves the target untouched.
//
// Replacing a file makes a new inode, so what belongs to the old inode doesn't
// carry over: ownership (the new file belongs to the user running devexp, the
// owner of every file devexp edits under that user's HOME), a hard link to the
// old file (the other name keeps the old contents), extended attributes and
// ACLs, and the setuid, setgid and sticky bits (only permission bits are kept).
//
// The symlink checks and the write are not one atomic step: a link created at
// path between them is followed like any other. Only a writer running as the
// same user can do that, and it could as well edit the file itself.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	target, mode, exists, err := resolveTarget(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(target)
	// A replacement starts private and takes the old file's bits below; a new
	// file is created with perm, so the kernel applies the umask.
	createPerm := perm.Perm()
	if exists {
		createPerm = 0600
	}
	f, err := createTemp(dir, filepath.Base(target), createPerm)
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
	if exists {
		if err = f.Chmod(mode); err != nil {
			return fmt.Errorf("%s: chmod %s: %w — the file was left untouched", target, tmp, err)
		}
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

// resolveTarget returns the file WriteFileAtomic replaces for path, its
// permission bits, and whether it exists (false: a new file at path).
func resolveTarget(path string) (string, os.FileMode, bool, error) {
	li, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	target := path
	if li.Mode()&os.ModeSymlink != 0 {
		target, err = filepath.EvalSymlinks(path)
		if os.IsNotExist(err) {
			return "", 0, false, fmt.Errorf("%s %w, so it was left untouched — create the file the link points at, or replace the link, and re-run", path, ErrDanglingSymlink)
		}
		if err != nil {
			return "", 0, false, fmt.Errorf("%s is a symlink that can't be resolved, so it was left untouched: %w", path, err)
		}
	}
	fi, err := os.Stat(target)
	if err != nil {
		return "", 0, false, err
	}
	if !fi.Mode().IsRegular() {
		return "", 0, false, fmt.Errorf("%s %w, so it was left untouched", describe(path, target), ErrNotRegular)
	}
	if syscall.Access(target, accessWriteOK) != nil {
		return "", 0, false, fmt.Errorf("%s %w, so it was left untouched", describe(path, target), ErrReadOnly)
	}
	return target, fi.Mode().Perm(), true, nil
}

// createTemp creates a new file named .<base>.tmp-<random> in dir, exclusively
// and with perm (minus the umask), retrying on a name that already exists.
func createTemp(dir, base string, perm os.FileMode) (*os.File, error) {
	for try := 0; ; try++ {
		name := filepath.Join(dir, "."+base+".tmp-"+tempSuffix())
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if os.IsExist(err) && try < 100 {
			continue
		}
		return f, err
	}
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

// IsSymlink reports whether path itself, not what it resolves to, is a
// symlink (dangling or not). A path that can't be checked is not reported as
// one. It is the one check both policies use: installers never write through a
// symlinked entry (#124), and removeguard's callers never remove one (#128).
func IsSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}
