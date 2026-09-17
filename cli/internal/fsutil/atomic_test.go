package fsutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { // umask-proof
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// entries lists dir, so a test can see that no temp file was left behind.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func isSymlink(t *testing.T, path string) bool {
	t.Helper()
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
}

func TestWriteFileAtomic_NewFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := WriteFileAtomic(p, []byte("new"), 0640); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
	if got := read(t, p); got != "new" {
		t.Errorf("content = %q, want new", got)
	}
	if got := mode(t, p); got != 0640 {
		t.Errorf("mode = %v, want 0640", got)
	}
	if got := entries(t, dir); len(got) != 1 {
		t.Errorf("dir holds %q, want only config.json", got)
	}
}

func TestWriteFileAtomic_ReplacesAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	write(t, p, "old contents that are longer", 0600)
	if err := WriteFileAtomic(p, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
	if got := read(t, p); got != "new" {
		t.Errorf("content = %q, want new", got)
	}
	if got := mode(t, p); got != 0600 {
		t.Errorf("mode = %v, want the file's own 0600, not perm", got)
	}
	if got := entries(t, dir); len(got) != 1 {
		t.Errorf("dir holds %q, want only settings.json", got)
	}
}

// A symlinked file (dotfiles) keeps its link; the file it points at is
// replaced, in the target's directory, with the target's mode.
func TestWriteFileAtomic_Symlink(t *testing.T) {
	for name, chain := range map[string]int{"one link": 1, "a chain of links": 2} {
		t.Run(name, func(t *testing.T) {
			home, dotfiles := t.TempDir(), t.TempDir()
			target := filepath.Join(dotfiles, "settings.json")
			write(t, target, "old", 0600)
			p := target
			for i := 0; i < chain; i++ {
				link := filepath.Join(home, strings.Repeat("l", i+1)+".json")
				if err := os.Symlink(p, link); err != nil {
					t.Fatal(err)
				}
				p = link
			}
			if err := WriteFileAtomic(p, []byte("new"), 0644); err != nil {
				t.Fatalf("WriteFileAtomic() error = %v", err)
			}
			for _, e := range entries(t, home) {
				if !isSymlink(t, filepath.Join(home, e)) {
					t.Errorf("%s holds %q, not a symlink: a link was replaced", home, e)
				}
			}
			if !isSymlink(t, p) {
				t.Fatalf("%s is no longer a symlink", p)
			}
			if got := read(t, target); got != "new" {
				t.Errorf("target content = %q, want new", got)
			}
			if got := mode(t, target); got != 0600 {
				t.Errorf("target mode = %v, want 0600", got)
			}
			if got := entries(t, dotfiles); len(got) != 1 {
				t.Errorf("dotfiles holds %q, want only settings.json (no temp file)", got)
			}
		})
	}
}

// A relative link resolves against the link's own directory, not the process's.
func TestWriteFileAtomic_RelativeSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "real"), 0755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "real", "c.json"), "old", 0644)
	link := filepath.Join(root, "c.json")
	if err := os.Symlink("real/c.json", link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(link, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
	if !isSymlink(t, link) || read(t, filepath.Join(root, "real", "c.json")) != "new" {
		t.Errorf("relative link not followed to its target")
	}
}

func TestWriteFileAtomic_Refusals(t *testing.T) {
	tests := map[string]struct {
		setup   func(t *testing.T, dir string) string // returns the path to write
		want    error
		root    bool // needs permissions enforced
		wantMsg string
	}{
		"a dangling symlink": {
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "config.json")
				os.Symlink(filepath.Join(dir, "nowhere", "config.json"), p) //nolint:errcheck
				return p
			},
			want: ErrDanglingSymlink,
		},
		"a symlink loop": {
			setup: func(t *testing.T, dir string) string {
				a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
				os.Symlink(b, a) //nolint:errcheck
				os.Symlink(a, b) //nolint:errcheck
				return a
			},
			wantMsg: "can't be resolved",
		},
		"a directory": {
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "config.json")
				os.Mkdir(p, 0755) //nolint:errcheck
				return p
			},
			want: ErrNotRegular,
		},
		"a symlink to a directory": {
			setup: func(t *testing.T, dir string) string {
				d := filepath.Join(dir, "d")
				os.Mkdir(d, 0755) //nolint:errcheck
				p := filepath.Join(dir, "config.json")
				os.Symlink(d, p) //nolint:errcheck
				return p
			},
			want: ErrNotRegular,
		},
		"a read-only file": {
			root: true,
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "config.json")
				write(t, p, "old", 0444)
				return p
			},
			want: ErrReadOnly,
		},
		"a symlink to a read-only file": {
			root: true,
			setup: func(t *testing.T, dir string) string {
				target := filepath.Join(dir, "real.json")
				write(t, target, "old", 0444)
				p := filepath.Join(dir, "config.json")
				os.Symlink(target, p) //nolint:errcheck
				return p
			},
			want: ErrReadOnly,
		},
		"a read-only directory": {
			root: true,
			setup: func(t *testing.T, dir string) string {
				d := filepath.Join(dir, "ro")
				os.Mkdir(d, 0755) //nolint:errcheck
				p := filepath.Join(d, "config.json")
				write(t, p, "old", 0644)
				os.Chmod(d, 0555)                       //nolint:errcheck
				t.Cleanup(func() { os.Chmod(d, 0755) }) //nolint:errcheck
				return p
			},
			wantMsg: "can't create a temp file",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if tt.root {
				skipIfRoot(t)
			}
			dir := t.TempDir()
			p := tt.setup(t, dir)
			before := snapshot(t, dir)
			err := WriteFileAtomic(p, []byte("new"), 0644)
			if err == nil {
				t.Fatalf("WriteFileAtomic() = nil, want a refusal")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %v, want it to say %q", err, tt.wantMsg)
			}
			if !strings.Contains(err.Error(), "left untouched") {
				t.Errorf("error = %v, want it to say the file was left untouched", err)
			}
			if after := snapshot(t, dir); after != before {
				t.Errorf("directory changed:\nbefore %s\nafter  %s", before, after)
			}
		})
	}
}

// snapshot renders every entry under dir with its type and, for files, content.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error { //nolint:errcheck
		if err != nil {
			return nil
		}
		b.WriteString(p + " " + fi.Mode().String())
		if fi.Mode().IsRegular() {
			c, _ := os.ReadFile(p)
			b.WriteString(" " + string(c))
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			l, _ := os.Readlink(p)
			b.WriteString(" -> " + l)
		}
		b.WriteString("\n")
		return nil
	})
	return b.String()
}

// A write or rename that fails part-way leaves the original bytes and no temp
// file.
func TestWriteFileAtomic_FailureLeavesOriginal(t *testing.T) {
	tests := map[string]struct {
		writeTemp func(f *os.File, data []byte) error
		syncFile  func(f *os.File) error
		rename    func(string, string) error
		wantMsg   string
	}{
		"fsync fails (I/O error)": {
			syncFile: func(*os.File) error { return syscall.EIO },
			wantMsg:  "sync",
		},
		"write fails half-way (disk full)": {
			writeTemp: func(f *os.File, data []byte) error {
				f.Write(data[:len(data)/2]) //nolint:errcheck
				return syscall.ENOSPC
			},
		},
		"rename fails": {
			rename: func(string, string) error { return &os.LinkError{Op: "rename", Err: syscall.EACCES} },
		},
		"rename across devices": {
			rename:  func(o, n string) error { return &os.LinkError{Op: "rename", Old: o, New: n, Err: syscall.EXDEV} },
			wantMsg: "can't be replaced atomically",
		},
		"rename over a bind mount": {
			rename:  func(o, n string) error { return &os.LinkError{Op: "rename", Old: o, New: n, Err: syscall.EBUSY} },
			wantMsg: "bind-mounted",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if tt.writeTemp != nil {
				old := writeTemp
				writeTemp = tt.writeTemp
				t.Cleanup(func() { writeTemp = old })
			}
			if tt.syncFile != nil {
				old := syncFile
				syncFile = tt.syncFile
				t.Cleanup(func() { syncFile = old })
			}
			if tt.rename != nil {
				old := rename
				rename = tt.rename
				t.Cleanup(func() { rename = old })
			}
			dir := t.TempDir()
			p := filepath.Join(dir, "settings.json")
			write(t, p, `{"original": true}`, 0600)
			err := WriteFileAtomic(p, []byte(`{"replacement": "much longer than the original"}`), 0644)
			if err == nil {
				t.Fatal("WriteFileAtomic() = nil, want the injected failure")
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %v, want it to say %q", err, tt.wantMsg)
			}
			if got := read(t, p); got != `{"original": true}` {
				t.Errorf("content = %q, want the original", got)
			}
			if got := entries(t, dir); len(got) != 1 {
				t.Errorf("dir holds %q, want only settings.json (temp file removed)", got)
			}
		})
	}
}

// TestWriteFileAtomic_KilledMidWrite runs a write in a child process that is
// SIGKILLed after the temp file is fully written and synced but before the
// rename: nothing gets to clean up, and the original must still be whole.
func TestWriteFileAtomic_KilledMidWrite(t *testing.T) {
	if p := os.Getenv("FSUTIL_KILL_BEFORE_RENAME"); p != "" {
		rename = func(string, string) error {
			syscall.Kill(os.Getpid(), syscall.SIGKILL) //nolint:errcheck
			select {}
		}
		WriteFileAtomic(p, []byte("new contents that never land"), 0644) //nolint:errcheck
		os.Exit(0)                                                       // unreachable: the rename kills the process
	}
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "settings.json")
	write(t, target, "original", 0600)
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestWriteFileAtomic_KilledMidWrite$")
	cmd.Env = append(os.Environ(), "FSUTIL_KILL_BEFORE_RENAME="+link)
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("child = %v, want it killed by SIGKILL before the rename", err)
	}
	if got := read(t, target); got != "original" {
		t.Errorf("target content = %q, want the original", got)
	}
	if !isSymlink(t, link) {
		t.Errorf("link replaced")
	}
	var temps int
	for _, e := range entries(t, filepath.Dir(target)) {
		if strings.HasPrefix(e, ".settings.json.tmp-") {
			temps++
			if got := read(t, filepath.Join(filepath.Dir(target), e)); got != "new contents that never land" {
				t.Errorf("temp file = %q, want the fully written new contents", got)
			}
		}
	}
	if temps != 1 {
		t.Errorf("temp files next to the target = %d, want the 1 the killed write left there (proving it was written beside the target, not the link)", temps)
	}
}

// TestIsSymlink: the path itself, dangling or not; a path that can't be checked
// is not one. removeguard's removal checks and the installers' write checks
// share it.
func TestIsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.Symlink(filepath.Join(root, "dir"), filepath.Join(root, "link"))         //nolint:errcheck
	os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "dangling")) //nolint:errcheck
	write(t, filepath.Join(root, "file"), "x", 0o644)
	for path, want := range map[string]bool{
		filepath.Join(root, "dir"):      false,
		filepath.Join(root, "file"):     false,
		filepath.Join(root, "link"):     true,
		filepath.Join(root, "dangling"): true,
		filepath.Join(root, "missing"):  false,
	} {
		if got := IsSymlink(path); got != want {
			t.Errorf("IsSymlink(%s) = %v, want %v", path, got, want)
		}
	}
}
