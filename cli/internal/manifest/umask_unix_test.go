//go:build unix

package manifest

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestSave_NewManifestHonoursUmask: a new manifest is created with the umask
// applied, as os.WriteFile did.
func TestSave_NewManifestHonoursUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(old) })
	p := filepath.Join(t.TempDir(), "nested", ".devexp-manifest.json")
	if err := Save(p, &Manifest{Agents: []string{"a.md"}}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("manifest mode = %v, want 0600 under umask 077", fi.Mode().Perm())
	}
}
