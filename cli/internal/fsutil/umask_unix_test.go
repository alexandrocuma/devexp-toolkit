//go:build unix

package fsutil

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// withUmask runs the test under mask, restoring the old one after. The umask
// is process-wide, so no test in this package may run in parallel.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })
}

// TestWriteFileAtomic_Umask: a new file gets perm minus the umask, as with
// os.WriteFile — under umask 077 a config holding tokens is created 0600, not
// world-readable. A replaced file keeps its own bits whatever the umask.
func TestWriteFileAtomic_Umask(t *testing.T) {
	tests := map[string]struct {
		umask    int
		existing os.FileMode // 0: no file yet
		perm     os.FileMode
		want     os.FileMode
	}{
		"new file, umask 077":           {umask: 0o077, perm: 0o644, want: 0o600},
		"new file, umask 022":           {umask: 0o022, perm: 0o644, want: 0o644},
		"new file, umask 027":           {umask: 0o027, perm: 0o664, want: 0o640},
		"new file, umask 0":             {umask: 0, perm: 0o640, want: 0o640},
		"replaced 0644 file, umask 077": {umask: 0o077, existing: 0o644, perm: 0o600, want: 0o644},
		"replaced 0600 file, umask 022": {umask: 0o022, existing: 0o600, perm: 0o644, want: 0o600},
		"replaced 0664 file, umask 077": {umask: 0o077, existing: 0o664, perm: 0o644, want: 0o664},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "config.json")
			if tt.existing != 0 {
				write(t, p, "old", tt.existing)
			}
			withUmask(t, tt.umask)
			if err := WriteFileAtomic(p, []byte("new"), tt.perm); err != nil {
				t.Fatalf("WriteFileAtomic() error = %v", err)
			}
			if got := mode(t, p); got != tt.want {
				t.Errorf("mode = %v, want %v", got, tt.want)
			}
			if got := entries(t, dir); len(got) != 1 {
				t.Errorf("dir holds %q, want only config.json", got)
			}
		})
	}
}

// TestWriteFileAtomic_UmaskTempFile: under umask 077 the temp file of a
// new config is private from the moment it exists, so a crash never leaves the
// contents readable by others either.
func TestWriteFileAtomic_UmaskTempFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	withUmask(t, 0o077)
	var tempMode os.FileMode
	old := rename
	rename = func(tmp, target string) error {
		fi, err := os.Stat(tmp)
		if err != nil {
			return err
		}
		tempMode = fi.Mode().Perm()
		return old(tmp, target)
	}
	t.Cleanup(func() { rename = old })
	if err := WriteFileAtomic(p, []byte(`{"token":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if tempMode != 0o600 {
		t.Errorf("temp file mode = %v, want 0600", tempMode)
	}
}

// TestWriteFileAtomic_ReplacementTempStartsPrivate: when replacing a file, the
// temp file holding the new contents is 0600 from creation until it takes the
// old file's bits, whatever the umask — a 0600 settings.json is never readable
// by others mid-save.
func TestWriteFileAtomic_ReplacementTempStartsPrivate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	write(t, p, "old", 0o600)
	withUmask(t, 0o022)
	var atWrite os.FileMode
	old := writeTemp
	writeTemp = func(f *os.File, data []byte) error {
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		atWrite = fi.Mode().Perm()
		return old(f, data)
	}
	t.Cleanup(func() { writeTemp = old })
	if err := WriteFileAtomic(p, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if atWrite != 0o600 {
		t.Errorf("temp file mode while writing = %v, want 0600", atWrite)
	}
	if got := mode(t, p); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}
}
