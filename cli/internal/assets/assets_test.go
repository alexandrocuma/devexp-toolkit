package assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFile(t *testing.T) {
	tests := map[string]struct {
		src       string
		destRel   string
		perm      os.FileMode
		wantErr   bool
		wantWrite bool
	}{
		"writes contents + perm, creates parent dirs": {
			src:       "uninstall.sh",
			destRel:   filepath.Join("nested", "deeper", "uninstall.sh"),
			perm:      0o755,
			wantErr:   false,
			wantWrite: true,
		},
		"missing source returns error, no partial write": {
			src:       "does-not-exist.sh",
			destRel:   filepath.Join("nested", "missing.sh"),
			perm:      0o644,
			wantErr:   true,
			wantWrite: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), tt.destRel)

			err := ExtractFile(FS, tt.src, dest, tt.perm)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			info, statErr := os.Stat(dest)
			if !tt.wantWrite {
				if !os.IsNotExist(statErr) {
					t.Fatalf("expected dest to not exist, stat err = %v", statErr)
				}
				return
			}

			if statErr != nil {
				t.Fatalf("expected dest written, stat err = %v", statErr)
			}

			want, readErr := fs.ReadFile(FS, tt.src)
			if readErr != nil {
				t.Fatalf("reading embedded source: %v", readErr)
			}
			got, readErr := os.ReadFile(dest)
			if readErr != nil {
				t.Fatalf("reading dest: %v", readErr)
			}
			if string(got) != string(want) {
				t.Errorf("content mismatch: dest has %d bytes, source has %d bytes", len(got), len(want))
			}

			if perm := info.Mode().Perm(); perm != tt.perm {
				t.Errorf("perm = %o, want %o", perm, tt.perm)
			}
		})
	}
}

func TestEmbeddedFS(t *testing.T) {
	tests := map[string]struct {
		path  string
		isDir bool
	}{
		"agents dir":         {path: "agents", isDir: true},
		"skills dir":         {path: "skills", isDir: true},
		"hooks dir":          {path: "hooks", isDir: true},
		"mcps dir":           {path: "mcps", isDir: true},
		"devexp.config.json": {path: "devexp.config.json", isDir: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			info, err := fs.Stat(FS, tt.path)
			if err != nil {
				t.Fatalf("fs.Stat(%q): %v", tt.path, err)
			}
			if info.IsDir() != tt.isDir {
				t.Errorf("%q IsDir() = %v, want %v", tt.path, info.IsDir(), tt.isDir)
			}
			if tt.isDir {
				entries, err := fs.ReadDir(FS, tt.path)
				if err != nil {
					t.Fatalf("fs.ReadDir(%q): %v", tt.path, err)
				}
				if len(entries) == 0 {
					t.Errorf("%q is empty", tt.path)
				}
			}
		})
	}
}
