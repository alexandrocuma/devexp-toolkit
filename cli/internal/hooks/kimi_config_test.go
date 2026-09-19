package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"devexp/internal/fsutil"
)

// kimiTestHooks is the real selection: the three fail-closed guards, as
// SelectKimi hands them over.
func kimiTestHooks() []KimiHook {
	return []KimiHook{
		{Name: "secret-guard", Event: "PreToolUse", Matcher: "^(Read|ReadMediaFile|Bash)$", Script: "hooks/claude-code/secret-guard.sh", FailClosed: true, Timeout: 45},
		{Name: "dangerous-cmd-guard", Event: "PreToolUse", Matcher: "^Bash$", Script: "hooks/claude-code/dangerous-cmd-guard.sh", FailClosed: true, Timeout: 45},
		{Name: "secret-in-write-guard", Event: "PreToolUse", Matcher: "^(Write|Edit)$", Script: "hooks/claude-code/secret-in-write-guard.sh", FailClosed: true, Timeout: 45},
	}
}

// kimiHome is a scratch $KIMI_CODE_HOME. Nothing in this file may touch the
// real one.
func kimiHome(t *testing.T, content string) (configPath, hooksDir string) {
	t.Helper()
	root := t.TempDir()
	configPath = filepath.Join(root, ".kimi-code", "config.toml")
	hooksDir = filepath.Join(root, ".kimi-code", "hooks")
	if content != "" {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return configPath, hooksDir
}

func writeKimi(t *testing.T, configPath, hooksDir string, selected []KimiHook) (bool, string, error) {
	t.Helper()
	var changed bool
	var err error
	out := captureOutput(t, func() { changed, err = WriteKimiHooks(configPath, hooksDir, selected, false) })
	return changed, stripANSI(out), err
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(b)
}

// tomlHooks decodes the hooks array of a rendered config, the way Kimi does.
func tomlHooks(t *testing.T, data string) []map[string]any {
	t.Helper()
	got, err := kimiHooksIn([]byte(data))
	if err != nil {
		t.Fatalf("the written config does not parse: %v\n%s", err, data)
	}
	return got
}

// TestWriteKimiHooksMissingFile: no config.toml at all. The block is the whole
// file, it parses, it holds exactly the selected hooks, and the new file is
// private — config.toml is where Kimi keeps provider API keys.
func TestWriteKimiHooksMissingFile(t *testing.T) {
	configPath, hooksDir := kimiHome(t, "")

	changed, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks())
	if err != nil || !changed {
		t.Fatalf("WriteKimiHooks() = %v, %v; want true, nil", changed, err)
	}

	got := readFile(t, configPath)
	if !strings.HasPrefix(got, kimiBlockBegin+"\n") {
		t.Errorf("a config devexp created should start with the marker, got:\n%s", got)
	}
	hooks := tomlHooks(t, got)
	if len(hooks) != 3 {
		t.Fatalf("got %d hooks, want 3:\n%s", len(hooks), got)
	}
	want := map[string]any{
		"event":   "PreToolUse",
		"matcher": "^Bash$",
		"command": "bash '" + filepath.Join(hooksDir, "kimi", "adapter.sh") + "' '" + filepath.Join(hooksDir, "claude-code", "dangerous-cmd-guard.sh") + "'",
		"timeout": int64(45),
	}
	if h := hooks[1]; !equalMaps(h, want) {
		t.Errorf("hook 1 = %#v, want %#v", h, want)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Errorf("a new config.toml has mode %v; it holds API keys and must not be group- or world-readable", info.Mode().Perm())
	}
}

func equalMaps(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestWriteKimiHooksEmptyFile: a zero-byte config.toml, which is what Kimi's
// own "starts empty" file becomes once its comments are deleted.
func TestWriteKimiHooksEmptyFile(t *testing.T) {
	configPath, hooksDir := kimiHome(t, "")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()); err != nil {
		t.Fatalf("WriteKimiHooks() error = %v", err)
	}
	got := readFile(t, configPath)
	if strings.HasPrefix(got, "\n") {
		t.Errorf("an empty file should not gain a leading blank line, got %q", got[:20])
	}
	if len(tomlHooks(t, got)) != 3 {
		t.Errorf("got %d hooks, want 3", len(tomlHooks(t, got)))
	}
}

const kimiUserConfig = `# ~/.kimi-code/config.toml
# My own notes, which must survive.

default_model = "kimi-k2"
telemetry  =  false

[providers.kimi]
type = "kimi"
api_key = "sk-secret"   # inline comment

[models.k2]
provider = "kimi"
model = "kimi-k2-0905-preview"
max_context_size = 262144
`

// TestWriteKimiHooksKeepsUserContent: every byte the user wrote is still
// there, in the same order, with its comments and its odd spacing, and the
// block is appended at the end where a top-level [[hooks]] is safe.
func TestWriteKimiHooksKeepsUserContent(t *testing.T) {
	configPath, hooksDir := kimiHome(t, kimiUserConfig)

	if _, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()); err != nil {
		t.Fatalf("WriteKimiHooks() error = %v", err)
	}
	got := readFile(t, configPath)
	if !strings.HasPrefix(got, kimiUserConfig) {
		t.Fatalf("the user's bytes did not survive verbatim:\n%s", got)
	}
	if want := kimiUserConfig + "\n" + kimiBlockBegin + "\n"; !strings.HasPrefix(got, want) {
		t.Errorf("the block should follow one blank line, got:\n%q", got[len(kimiUserConfig):len(kimiUserConfig)+40])
	}
	if !strings.HasSuffix(got, kimiBlockEnd+"\n") {
		t.Errorf("the file should end with the end marker, got:\n%s", got)
	}

	// The user's own values are still the ones Kimi reads.
	var doc map[string]any
	if err := toml.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("the written config does not parse: %v", err)
	}
	if doc["default_model"] != "kimi-k2" || doc["telemetry"] != false {
		t.Errorf("the user's top-level keys changed: %#v", doc)
	}
}

// TestWriteKimiHooksReplacesBlockInPlace: a block already in the middle of the
// file is replaced where it stands, and not one byte around it moves.
func TestWriteKimiHooksReplacesBlockInPlace(t *testing.T) {
	configPath, hooksDir := kimiHome(t, kimiUserConfig)
	if _, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// Put a tail after the block, so a replacement that appended instead of
	// editing in place would be visible.
	tail := "\n[[hooks]]\nevent = \"Stop\"\ncommand = \"echo mine\"\n"
	first := readFile(t, configPath) + tail
	if err := os.WriteFile(configPath, []byte(first), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Fewer hooks this run: one stopped being installed.
	changed, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:1])
	if err != nil || !changed {
		t.Fatalf("WriteKimiHooks() = %v, %v; want true, nil", changed, err)
	}
	got := readFile(t, configPath)

	if !strings.HasPrefix(got, kimiUserConfig) {
		t.Errorf("the user's leading bytes moved:\n%s", got)
	}
	if !strings.HasSuffix(got, tail) {
		t.Errorf("the user's trailing hook moved or was lost:\n%s", got)
	}
	hooks := tomlHooks(t, got)
	if len(hooks) != 2 {
		t.Fatalf("got %d hooks, want 2 (one of devexp's, one of the user's):\n%s", len(hooks), got)
	}
	if hooks[1]["command"] != "echo mine" {
		t.Errorf("the user's hook is not the last one: %#v", hooks)
	}
	if !strings.Contains(hooks[0]["command"].(string), "secret-guard.sh") {
		t.Errorf("devexp's remaining hook is wrong: %#v", hooks[0])
	}
	if strings.Contains(got, "dangerous-cmd-guard.sh") {
		t.Errorf("a hook that stopped being installed did not leave the block:\n%s", got)
	}
}

// TestWriteKimiHooksIdempotent: a second install with the same selection
// writes nothing at all — not the same bytes again, nothing. The mtime proves
// the file was not replaced.
func TestWriteKimiHooksIdempotent(t *testing.T) {
	configPath, hooksDir := kimiHome(t, kimiUserConfig)
	if _, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()); err != nil {
		t.Fatalf("first write: %v", err)
	}
	before := readFile(t, configPath)
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	changed, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks())
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if changed {
		t.Error("the second install reported a change; it should have written nothing")
	}
	if after := readFile(t, configPath); after != before {
		t.Errorf("the second install changed the bytes:\n%s", after)
	}
	info2, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info2.ModTime().Equal(info.ModTime()) {
		t.Errorf("the second install replaced the file (mtime %v → %v)", info.ModTime(), info2.ModTime())
	}
}

// TestRemoveKimiHooks: the block comes out and the file is exactly what it was
// before devexp ever touched it, blank line included.
func TestRemoveKimiHooks(t *testing.T) {
	configPath, hooksDir := kimiHome(t, kimiUserConfig)
	if _, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()); err != nil {
		t.Fatalf("write: %v", err)
	}

	var changed bool
	var err error
	captureOutput(t, func() { changed, err = RemoveKimiHooks(configPath, hooksDir, false) })
	if err != nil || !changed {
		t.Fatalf("RemoveKimiHooks() = %v, %v; want true, nil", changed, err)
	}
	if got := readFile(t, configPath); got != kimiUserConfig {
		t.Errorf("removal did not restore the file byte for byte:\n%q", got)
	}

	// A second removal is not a change, and a file with no block never was.
	captureOutput(t, func() { changed, err = RemoveKimiHooks(configPath, hooksDir, false) })
	if err != nil || changed {
		t.Errorf("second RemoveKimiHooks() = %v, %v; want false, nil", changed, err)
	}
	missing := filepath.Join(t.TempDir(), "nope", "config.toml")
	captureOutput(t, func() { changed, err = RemoveKimiHooks(missing, hooksDir, false) })
	if err != nil || changed {
		t.Errorf("RemoveKimiHooks(missing) = %v, %v; want false, nil", changed, err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Error("removal created a config.toml that was not there")
	}
}

// TestWriteKimiHooksKeepsUserHooks: the user's own [[hooks]], before and after
// devexp's block, are left exactly as they are and still load.
func TestWriteKimiHooksKeepsUserHooks(t *testing.T) {
	const content = `[[hooks]]
event = "SessionStart"
command = "echo hello"

[[hooks]]
event = "Stop"
matcher = ""
command = "notify-send done"
timeout = 5
`
	configPath, hooksDir := kimiHome(t, content)

	if _, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()); err != nil {
		t.Fatalf("WriteKimiHooks() error = %v\n%s", err, out)
	}
	got := readFile(t, configPath)
	if !strings.HasPrefix(got, content) {
		t.Fatalf("the user's hooks were rewritten:\n%s", got)
	}
	hooks := tomlHooks(t, got)
	if len(hooks) != 5 {
		t.Fatalf("got %d hooks, want 5 (2 of the user's + 3 of devexp's):\n%s", len(hooks), got)
	}
	if hooks[0]["command"] != "echo hello" || hooks[1]["timeout"] != int64(5) {
		t.Errorf("the user's hooks changed: %#v", hooks[:2])
	}
}

// TestWriteKimiHooksCRLF: a CRLF file keeps its line endings, and the block
// does not mix the two.
func TestWriteKimiHooksCRLF(t *testing.T) {
	content := strings.ReplaceAll("default_model = \"kimi-k2\"\n\n[providers.kimi]\ntype = \"kimi\"\n", "\n", "\r\n")
	configPath, hooksDir := kimiHome(t, content)

	if _, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()); err != nil {
		t.Fatalf("WriteKimiHooks() error = %v", err)
	}
	got := readFile(t, configPath)
	if strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Errorf("the block mixed LF into a CRLF file:\n%q", got)
	}
	if len(tomlHooks(t, got)) != 3 {
		t.Errorf("the CRLF file does not hold devexp's three hooks:\n%q", got)
	}
	// And it round-trips.
	captureOutput(t, func() { RemoveKimiHooks(configPath, hooksDir, false) }) //nolint:errcheck
	if back := readFile(t, configPath); back != content {
		t.Errorf("removal did not restore the CRLF file:\n%q", back)
	}
}

// TestWriteKimiHooksSymlink: a config.toml symlinked into a dotfiles checkout
// stays a symlink, and what it points at is what changes.
func TestWriteKimiHooksSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need a privilege on Windows")
	}
	root := t.TempDir()
	real := filepath.Join(root, "dotfiles", "config.toml")
	if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(real, []byte(kimiUserConfig), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	configPath := filepath.Join(root, ".kimi-code", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(real, configPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	hooksDir := filepath.Join(root, ".kimi-code", "hooks")
	if _, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()); err != nil {
		t.Fatalf("WriteKimiHooks() error = %v", err)
	}
	if !fsutil.IsSymlink(configPath) {
		t.Error("the symlink was replaced by a regular file")
	}
	if got := readFile(t, real); !strings.Contains(got, kimiBlockBegin) {
		t.Errorf("the link target was not written:\n%s", got)
	}
}

// TestWriteKimiHooksReadOnly: a file this user may not write is refused by
// fsutil and left exactly as it was — a rename into a writable directory would
// otherwise undo their chmod silently.
func TestWriteKimiHooksReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root may write anything")
	}
	configPath, hooksDir := kimiHome(t, kimiUserConfig)
	if err := os.Chmod(configPath, 0o400); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	_, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks())
	if !errors.Is(err, fsutil.ErrReadOnly) {
		t.Fatalf("WriteKimiHooks() error = %v; want ErrReadOnly", err)
	}
	if got := readFile(t, configPath); got != kimiUserConfig {
		t.Errorf("a refused write changed the file:\n%s", got)
	}
}

// TestWriteKimiHooksRefusals covers every config devexp will not edit. In each
// case the file must come out byte for byte as it went in, and the error must
// say so.
func TestWriteKimiHooksRefusals(t *testing.T) {
	tests := map[string]struct {
		content string
		wantErr string
	}{
		"not valid TOML": {
			content: "default_model = \n[providers\n",
			wantErr: "is not valid TOML",
		},
		"a begin marker with no end": {
			content: kimiUserConfig + "\n" + kimiBlockBegin + "\n[[hooks]]\nevent = \"Stop\"\ncommand = \"x\"\n",
			wantErr: "markers are unbalanced",
		},
		"an end marker with no begin": {
			content: kimiUserConfig + "\n" + kimiBlockEnd + "\n",
			wantErr: "markers are unbalanced",
		},
		"two blocks": {
			content: kimiBlockBegin + "\n" + kimiBlockEnd + "\n" + kimiBlockBegin + "\n" + kimiBlockEnd + "\n",
			wantErr: "markers are unbalanced",
		},
		"the markers the wrong way round": {
			content: kimiBlockEnd + "\n" + kimiBlockBegin + "\n",
			wantErr: "markers are unbalanced",
		},
		"a hooks key that is not an array of tables": {
			content: "hooks = \"off\"\n",
			wantErr: "was left untouched",
		},
		"a statically defined hooks array": {
			content: "hooks = [{ event = \"Stop\", command = \"x\" }]\n",
			wantErr: "was left untouched",
		},
		"a bare key sitting after devexp's block": {
			// The block is replaced in place, and `telemetry` would land
			// inside devexp's last [[hooks]] — the extra key Kimi's strict
			// schema drops the whole section for.
			content: kimiBlockBegin + "\n[[hooks]]\nevent = \"Stop\"\ncommand = \"x\"\n" + kimiBlockEnd + "\ntelemetry = false\n",
			wantErr: "was left untouched",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			configPath, hooksDir := kimiHome(t, tc.content)

			_, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks())
			if err == nil {
				t.Fatalf("WriteKimiHooks() error = nil; want one mentioning %q\n%s", tc.wantErr, readFile(t, configPath))
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("WriteKimiHooks() error = %q; want it to mention %q", err, tc.wantErr)
			}
			if got := readFile(t, configPath); got != tc.content {
				t.Errorf("a refused write changed the file:\n%q", got)
			}
		})
	}
}

// TestWriteKimiHooksUnreadable: a config.toml devexp cannot read at all is
// refused rather than replaced with a fresh one.
func TestWriteKimiHooksUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("POSIX mode bits, and root reads anything")
	}
	configPath, hooksDir := kimiHome(t, kimiUserConfig)
	if err := os.Chmod(configPath, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(configPath, 0o600) }) //nolint:errcheck

	_, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks())
	if err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("WriteKimiHooks() error = %v; want one about not being readable", err)
	}
}

// TestWriteKimiHooksSkipsUnwritableHook: a hook devexp itself cannot render
// validly is left out, loudly, and the rest are still installed — writing it
// would cost the user every hook in the file.
func TestWriteKimiHooksSkipsUnwritableHook(t *testing.T) {
	tests := map[string]KimiHook{
		"an event Kimi does not know": {Name: "bad-event", Event: "OnceInAWhile", Script: "hooks/claude-code/x.sh", Timeout: 45},
		"a timeout above Kimi's 600":  {Name: "bad-timeout", Event: "PreToolUse", Script: "hooks/claude-code/x.sh", Timeout: 900},
		"a timeout below Kimi's 1":    {Name: "negative", Event: "PreToolUse", Script: "hooks/claude-code/x.sh", Timeout: -1},
		"a matcher spanning lines":    {Name: "multiline", Event: "PreToolUse", Matcher: "^a$\n^b$", Script: "hooks/claude-code/x.sh", Timeout: 45},
	}

	for name, bad := range tests {
		t.Run(name, func(t *testing.T) {
			configPath, hooksDir := kimiHome(t, "")

			_, out, err := writeKimi(t, configPath, hooksDir, append([]KimiHook{bad}, kimiTestHooks()...))
			if err != nil {
				t.Fatalf("WriteKimiHooks() error = %v", err)
			}
			if !strings.Contains(out, bad.Name) || !strings.Contains(out, "skipping it") {
				t.Errorf("the skipped hook was not reported; output was:\n%s", out)
			}
			got := readFile(t, configPath)
			if strings.Contains(got, bad.Name) {
				t.Errorf("the bad hook was written anyway:\n%s", got)
			}
			if len(tomlHooks(t, got)) != 3 {
				t.Errorf("the good hooks were not installed:\n%s", got)
			}
		})
	}
}

// TestWriteKimiHooksWarnsAboutForeignInvalidHook: a hook the user already has
// that Kimi rejects costs them devexp's guards too, and only a diagnostic
// nobody reads says so. devexp leaves it alone and says it out loud.
func TestWriteKimiHooksWarnsAboutForeignInvalidHook(t *testing.T) {
	const content = `[[hooks]]
event = "Stop"
command = "echo mine"
name = "my hook"
`
	configPath, hooksDir := kimiHome(t, content)

	_, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks())
	if err != nil {
		t.Fatalf("WriteKimiHooks() error = %v", err)
	}
	if !strings.Contains(out, "strict") || !strings.Contains(out, "index 0") {
		t.Errorf("the user's unloadable hook was not reported; output was:\n%s", out)
	}
	if got := readFile(t, configPath); !strings.HasPrefix(got, content) {
		t.Errorf("devexp edited the user's hook instead of leaving it:\n%s", got)
	}
}

// TestWriteKimiHooksDryRun writes nothing and says what it would have done.
func TestWriteKimiHooksDryRun(t *testing.T) {
	configPath, hooksDir := kimiHome(t, kimiUserConfig)

	var changed bool
	var err error
	out := captureOutput(t, func() { changed, err = WriteKimiHooks(configPath, hooksDir, kimiTestHooks(), true) })
	if err != nil || !changed {
		t.Fatalf("WriteKimiHooks(dryRun) = %v, %v; want true, nil", changed, err)
	}
	if !strings.Contains(stripANSI(out), "[dry-run]") {
		t.Errorf("a dry run said nothing:\n%s", out)
	}
	if got := readFile(t, configPath); got != kimiUserConfig {
		t.Errorf("a dry run wrote to the file:\n%s", got)
	}
}

// TestWriteKimiHooksRelativeHooksDir: Kimi runs a hook from whatever directory
// it happens to be in, so a relative script path would find nothing.
func TestWriteKimiHooksRelativeHooksDir(t *testing.T) {
	configPath, _ := kimiHome(t, kimiUserConfig)

	_, _, err := writeKimi(t, configPath, "hooks", kimiTestHooks())
	if err == nil || !strings.Contains(err.Error(), "not absolute") {
		t.Fatalf("WriteKimiHooks() error = %v; want one about the path not being absolute", err)
	}
	if got := readFile(t, configPath); got != kimiUserConfig {
		t.Errorf("a refused write changed the file:\n%s", got)
	}
}

// TestKimiCommandQuoting: the command is one shell word per path, and a Kimi
// home with a quote or a space in it stays one word.
func TestKimiCommandQuoting(t *testing.T) {
	h := KimiHook{Name: "secret-guard", Event: "PreToolUse", Script: "hooks/claude-code/secret-guard.sh"}

	got := KimiCommand("/home/jo's dir/.kimi-code/hooks", h)
	want := `bash '/home/jo'\''s dir/.kimi-code/hooks/kimi/adapter.sh' '/home/jo'\''s dir/.kimi-code/hooks/claude-code/secret-guard.sh'`
	if got != want {
		t.Errorf("KimiCommand() =\n%s\nwant\n%s", got, want)
	}
}

// TestWriteKimiHooksAwkwardHome: a Kimi home holding a quote, a space, a
// backslash and a TOML-significant character survives both the TOML escaping
// and the shell quoting — the rendered command reads back as the exact path.
func TestWriteKimiHooksAwkwardHome(t *testing.T) {
	root := t.TempDir()
	hooksDir := filepath.Join(root, `jo's "kimi" \ dir`, "hooks")
	configPath := filepath.Join(root, "config.toml")

	if _, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:1]); err != nil {
		t.Fatalf("WriteKimiHooks() error = %v", err)
	}
	hooks := tomlHooks(t, readFile(t, configPath))
	if len(hooks) != 1 {
		t.Fatalf("got %d hooks, want 1", len(hooks))
	}
	if got, want := hooks[0]["command"], KimiCommand(hooksDir, kimiTestHooks()[0]); got != want {
		t.Errorf("command read back as\n%v\nwant\n%v", got, want)
	}
}

// TestKimiMarkersAreNotMatchedInsideValues: a marker quoted in a string or
// mentioned mid-line is not a marker, so a config that talks about devexp is
// still one devexp can edit.
func TestKimiMarkersAreNotMatchedInsideValues(t *testing.T) {
	content := "note = \"" + kimiBlockBegin + "\"\nother = \"x " + kimiBlockEnd + "\"\n"
	configPath, hooksDir := kimiHome(t, content)

	if _, _, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()); err != nil {
		t.Fatalf("WriteKimiHooks() error = %v", err)
	}
	got := readFile(t, configPath)
	if !strings.HasPrefix(got, content) {
		t.Errorf("the quoted markers were treated as devexp's block:\n%s", got)
	}
	if len(tomlHooks(t, got)) != 3 {
		t.Errorf("got %d hooks, want 3:\n%s", len(tomlHooks(t, got)), got)
	}
}

// TestKimiBlockEntriesMatchKimisSchema: every entry devexp renders holds
// HookDefSchema's keys and nothing else, because the schema is strict and one
// extra key drops every hook in the user's file.
func TestKimiBlockEntriesMatchKimisSchema(t *testing.T) {
	block := renderKimiBlock(kimiEntries("/k/hooks", kimiTestHooks()))
	hooks := tomlHooks(t, block)
	if len(hooks) != 3 {
		t.Fatalf("got %d hooks, want 3", len(hooks))
	}
	for i, h := range hooks {
		if err := validateForeignKimiHook(h); err != nil {
			t.Errorf("entry %d is one Kimi would reject: %v", i, err)
		}
	}
}
