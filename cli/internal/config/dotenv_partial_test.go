package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadDotenv_ReturnsPartialDataWithError pins the contract the installer
// depends on when it decides whether to warn.
//
// LoadDotenv does not return (nil, err) on a read failure — it returns
// everything it managed to parse *alongside* the error. That makes a discarded
// error here materially different from the usual "a missing file counts as
// empty": the caller is handed a map that looks complete and is not, so an MCP
// starts without a variable it was configured with and fails somewhere that
// points nowhere near the cause.
//
// If this contract ever changes to (nil, err), the warning at the call site
// becomes wrong rather than merely unnecessary — it would report a variable
// count of zero for a file that parsed fine up to the failure.
func TestLoadDotenv_ReturnsPartialDataWithError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	// A line past bufio.Scanner's token limit makes Scan stop and Err report,
	// after earlier lines have already been parsed.
	oversized := "HUGE=" + strings.Repeat("x", bufio.MaxScanTokenSize+1)
	body := "FIRST=one\nSECOND=two\n" + oversized + "\nNEVER_REACHED=three\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	env, err := LoadDotenv(path)
	if err == nil {
		t.Fatal("LoadDotenv() error = nil, want an error — the oversized line should have stopped the scan")
	}
	if env == nil {
		t.Fatal("LoadDotenv() returned a nil map with its error; the installer's warning reports len(env) and assumes partial data is returned")
	}
	for k, want := range map[string]string{"FIRST": "one", "SECOND": "two"} {
		if env[k] != want {
			t.Errorf("env[%q] = %q, want %q — lines before the failure should still be parsed", k, env[k], want)
		}
	}
	if _, ok := env["NEVER_REACHED"]; ok {
		t.Error("env has NEVER_REACHED, but the scan should have stopped before it — this test no longer exercises a partial read")
	}
}

// TestLoadDotenv_MissingFileIsNotExist pins the other half of the call site's
// decision: an absent file is the ordinary case and must stay distinguishable
// from an unreadable one, or the installer would warn on every run without an
// mcps/.env.
func TestLoadDotenv_MissingFileIsNotExist(t *testing.T) {
	_, err := LoadDotenv(filepath.Join(t.TempDir(), "absent.env"))
	if err == nil {
		t.Fatal("LoadDotenv() on a missing file returned no error")
	}
	if !os.IsNotExist(err) {
		t.Errorf("LoadDotenv() error = %v, want one os.IsNotExist recognises — the installer uses that to stay quiet when there is simply no .env", err)
	}
}
