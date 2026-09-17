//go:build unix

package mcp

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestInstallOpencode_NewConfigHonoursUmask (#157 review): config.json holds
// resolved MCP env values and headers (tokens). A new one is created with the
// umask applied, as before #124, so under umask 077 it is 0600.
func TestInstallOpencode_NewConfigHonoursUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(old) })
	p := filepath.Join(t.TempDir(), "config.json")
	mcps := []MCP{{Name: "remote", Transport: "http", URL: "https://example.com", Headers: map[string]string{"Authorization": "Bearer ${TOKEN}"}}}
	var err error
	_ = captureStdout(t, func() { err = InstallOpencode(mcps, map[string]string{"TOKEN": "secret"}, p, false, false) })
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("config.json mode = %v, want 0600 under umask 077", fi.Mode().Perm())
	}
}
