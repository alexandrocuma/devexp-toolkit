package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// kimiRepo is a scratch checkout holding the hook sources InstallKimi copies:
// the adapter, the scan budget every guard sources, and the three guards.
func kimiRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	files := map[string]string{
		"hooks/kimi/adapter.sh":                          "#!/usr/bin/env bash\n# adapter\n",
		"hooks/claude-code/scan-budget.sh":               "# scan budget\n",
		"hooks/claude-code/secret-guard.sh":              "#!/usr/bin/env bash\n# secret-guard\n",
		"hooks/claude-code/dangerous-cmd-guard.sh":       "#!/usr/bin/env bash\n# dangerous-cmd-guard\n",
		"hooks/claude-code/secret-in-write-guard.sh":     "#!/usr/bin/env bash\n# secret-in-write-guard\n",
		"hooks/claude-code/graphify-session-sentinel.sh": "#!/usr/bin/env bash\n# not for kimi\n",
	}
	for rel, content := range files {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return repo
}

// kimiTestRegistry is a registry with one hook Kimi takes, one it refuses with
// a reason, and one with no Kimi block at all.
func kimiTestRegistry(t *testing.T) Registry {
	t.Helper()
	r, err := ParseRegistry([]byte(`[
  {"name": "secret-guard", "enabled": true,
   "kimi": {"event": "PreToolUse", "matcher": "^(Read|ReadMediaFile|Bash)$", "script": "hooks/claude-code/secret-guard.sh", "fail_closed": true, "timeout": 45}},
  {"name": "dangerous-cmd-guard", "enabled": true,
   "kimi": {"event": "PreToolUse", "matcher": "^Bash$", "script": "hooks/claude-code/dangerous-cmd-guard.sh", "fail_closed": true, "timeout": 45}},
  {"name": "lint-on-save", "enabled": true,
   "kimi": {"event": "PostToolUse", "script": "hooks/claude-code/lint-on-save.sh", "enabled": false, "reason": "Kimi discards a PostToolUse result"}},
  {"name": "graphify-session-sentinel", "enabled": false,
   "claude_code": {"event": "PostToolUse", "script": "hooks/claude-code/graphify-session-sentinel.sh"}}
]`))
	if err != nil {
		t.Fatalf("ParseRegistry: %v", err)
	}
	return r
}

func installKimi(t *testing.T, repo, hooksDir, configPath string, disabled, recorded []string, dryRun bool) ([]string, string, error) {
	t.Helper()
	var got []string
	var err error
	out := captureOutput(t, func() {
		got, err = InstallKimi(kimiTestRegistry(t), repo, hooksDir, configPath, disabled, recorded, dryRun)
	})
	return got, stripANSI(out), err
}

// The whole point of copying rather than registering the checkout: the scripts
// have to be there, executable, with the scan budget beside the guards, and
// only then may the commands be registered.
func TestInstallKimiCopiesThenRegisters(t *testing.T) {
	repo := kimiRepo(t)
	configPath, hooksDir := kimiHome(t, "")

	got, out, err := installKimi(t, repo, hooksDir, configPath, nil, nil, false)
	if err != nil {
		t.Fatalf("InstallKimi: %v\n%s", err, out)
	}

	want := []string{
		"kimi/adapter.sh",
		"claude-code/scan-budget.sh",
		"claude-code/secret-guard.sh",
		"claude-code/dangerous-cmd-guard.sh",
	}
	if !slices.Equal(got, want) {
		t.Errorf("owns %v, want %v", got, want)
	}
	for _, rel := range want {
		info, err := os.Stat(filepath.Join(hooksDir, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s is not executable (%v) — the adapter runs it with bash, and a guard that cannot run is an allow", rel, info.Mode().Perm())
		}
	}
	// The scan budget has to sit NEXT TO the guards: each one sources it as
	// "${BASH_SOURCE[0]%/*}/scan-budget.sh".
	guard := filepath.Join(hooksDir, "claude-code", "secret-guard.sh")
	if _, err := os.Stat(filepath.Join(filepath.Dir(guard), "scan-budget.sh")); err != nil {
		t.Errorf("scan-budget.sh is not beside the guards: %v", err)
	}

	conf := readFile(t, configPath)
	for _, rel := range []string{"kimi/adapter.sh", "claude-code/secret-guard.sh"} {
		if !strings.Contains(conf, filepath.Join(hooksDir, filepath.FromSlash(rel))) {
			t.Errorf("config.toml does not register the installed %s:\n%s", rel, conf)
		}
	}
	// Every command it registered names a file that exists — the invariant
	// that makes the ordering matter.
	for _, h := range tomlHooks(t, conf) {
		cmd, _ := h["command"].(string)
		for _, field := range strings.Split(cmd, "'") {
			if strings.HasSuffix(field, ".sh") {
				if _, err := os.Stat(field); err != nil {
					t.Errorf("registered command names a missing script %q: %v", field, err)
				}
			}
		}
	}
	// A hook Kimi cannot honour says why, on the run that would have
	// installed it.
	if !strings.Contains(out, "lint-on-save") || !strings.Contains(out, "PostToolUse result") {
		t.Errorf("the run does not say why lint-on-save is off for Kimi:\n%s", out)
	}
	if !strings.Contains(out, "Kimi hooks (2)") {
		t.Errorf("no summary line naming the hooks installed:\n%s", out)
	}
}

func TestInstallKimiDryRunWritesNothing(t *testing.T) {
	repo := kimiRepo(t)
	configPath, hooksDir := kimiHome(t, "")

	got, out, err := installKimi(t, repo, hooksDir, configPath, nil, nil, true)
	if err != nil {
		t.Fatalf("InstallKimi: %v\n%s", err, out)
	}
	if len(got) == 0 {
		t.Errorf("a dry run reported owning nothing, so its caller could not preview the manifest")
	}
	for _, p := range []string{hooksDir, configPath} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("a dry run created %q (%v)", p, err)
		}
	}
	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("a dry run said nothing about what it would write:\n%s", out)
	}
}

func TestInstallKimiSecondRunChangesNothing(t *testing.T) {
	repo := kimiRepo(t)
	configPath, hooksDir := kimiHome(t, "")
	if _, out, err := installKimi(t, repo, hooksDir, configPath, nil, nil, false); err != nil {
		t.Fatalf("first install: %v\n%s", err, out)
	}
	before := kimiTree(t, hooksDir)
	beforeConf := readFile(t, configPath)

	_, out, err := installKimi(t, repo, hooksDir, configPath, nil, nil, false)
	if err != nil {
		t.Fatalf("second install: %v\n%s", err, out)
	}
	if after := kimiTree(t, hooksDir); !slices.Equal(before, after) {
		t.Errorf("a second install rewrote the scripts:\nbefore %v\nafter  %v", before, after)
	}
	if readFile(t, configPath) != beforeConf {
		t.Errorf("a second install rewrote config.toml")
	}
	if !strings.Contains(out, "already up to date") {
		t.Errorf("the second run does not say the scripts were already there:\n%s", out)
	}
}

// A hook the user switched off is not installed and not registered, and its
// script is taken back off disk — a command devexp no longer writes must not
// be left with a script for some other config to find.
func TestInstallKimiDisabledHookIsRemoved(t *testing.T) {
	repo := kimiRepo(t)
	configPath, hooksDir := kimiHome(t, "")
	owned, out, err := installKimi(t, repo, hooksDir, configPath, nil, nil, false)
	if err != nil {
		t.Fatalf("first install: %v\n%s", err, out)
	}

	got, out, err := installKimi(t, repo, hooksDir, configPath, []string{"dangerous-cmd-guard"}, owned, false)
	if err != nil {
		t.Fatalf("second install: %v\n%s", err, out)
	}
	if slices.Contains(got, "claude-code/dangerous-cmd-guard.sh") {
		t.Errorf("a disabled hook is still owned: %v", got)
	}
	if _, err := os.Stat(filepath.Join(hooksDir, "claude-code", "dangerous-cmd-guard.sh")); !os.IsNotExist(err) {
		t.Errorf("a disabled hook's script is still on disk (%v)", err)
	}
	if strings.Contains(readFile(t, configPath), "dangerous-cmd-guard") {
		t.Errorf("a disabled hook is still registered:\n%s", readFile(t, configPath))
	}
	if !strings.Contains(out, "disabled") {
		t.Errorf("the run does not say the hook was disabled:\n%s", out)
	}
}

// Every hook off: the block comes out, the scripts come out, and the empty
// directories go with them, so the Kimi root looks like one devexp never
// touched.
func TestInstallKimiEverythingDisabled(t *testing.T) {
	repo := kimiRepo(t)
	configPath, hooksDir := kimiHome(t, "model = \"k2\"\n")
	owned, out, err := installKimi(t, repo, hooksDir, configPath, nil, nil, false)
	if err != nil {
		t.Fatalf("first install: %v\n%s", err, out)
	}

	got, out, err := installKimi(t, repo, hooksDir, configPath, []string{"secret-guard", "dangerous-cmd-guard"}, owned, false)
	if err != nil {
		t.Fatalf("second install: %v\n%s", err, out)
	}
	if len(got) != 0 {
		t.Errorf("owns %v after every hook was disabled, want nothing", got)
	}
	if _, err := os.Stat(hooksDir); !os.IsNotExist(err) {
		t.Errorf("the hooks directory survived with nothing in it (%v)", err)
	}
	conf := readFile(t, configPath)
	if strings.Contains(conf, kimiBlockBegin) || strings.Contains(conf, "[[hooks]]") {
		t.Errorf("devexp's block is still in config.toml:\n%s", conf)
	}
	if !strings.Contains(conf, `model = "k2"`) {
		t.Errorf("the user's own config did not survive:\n%s", conf)
	}
}

// A source script that cannot be read stops the step BEFORE anything is
// registered: a registered command whose script is missing is a silent allow.
func TestInstallKimiMissingSourceRegistersNothing(t *testing.T) {
	repo := kimiRepo(t)
	if err := os.Remove(filepath.Join(repo, "hooks", "claude-code", "dangerous-cmd-guard.sh")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	configPath, hooksDir := kimiHome(t, "")

	got, out, err := installKimi(t, repo, hooksDir, configPath, nil, nil, false)
	if err == nil {
		t.Fatalf("a missing guard script installed anyway\n%s", out)
	}
	if !strings.Contains(err.Error(), "dangerous-cmd-guard.sh") {
		t.Errorf("the error does not name the script: %v", err)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Errorf("a half-finished copy still registered commands (%v)", statErr)
	}
	// What it did write has to come back, or the next run has no record of it
	// and #115 cannot remove it either (#113's lesson).
	for _, rel := range []string{"kimi/adapter.sh", "claude-code/scan-budget.sh", "claude-code/secret-guard.sh"} {
		if !slices.Contains(got, rel) {
			t.Errorf("the failed step did not report writing %s: %v", rel, got)
		}
		if _, err := os.Stat(filepath.Join(hooksDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s was reported but is not on disk: %v", rel, err)
		}
	}
}

// The manifest is a file on disk, so an entry in it may be anything. A
// recorded name that is not one devexp installs is never joined onto hooksDir
// and removed.
func TestInstallKimiRefusesToRemoveAnythingButItsOwn(t *testing.T) {
	repo := kimiRepo(t)
	configPath, hooksDir := kimiHome(t, "")
	outside := filepath.Join(filepath.Dir(hooksDir), "config.toml.keepme")
	if err := os.MkdirAll(filepath.Dir(outside), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(outside, []byte("keep\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bad := []string{"../config.toml.keepme", "/etc/passwd", "claude-code/../../x.sh", "kimi/adapter.sh\nfake"}
	got, out, err := installKimi(t, repo, hooksDir, configPath, nil, bad, false)
	if err != nil {
		t.Fatalf("InstallKimi: %v\n%s", err, out)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("a traversal in the manifest removed a file outside the hooks directory: %v", err)
	}
	for _, name := range bad {
		if !slices.Contains(got, name) {
			t.Errorf("a name devexp refused to act on was dropped from the record: %q not in %v", name, got)
		}
	}
	if !strings.Contains(out, "not a name devexp installs") {
		t.Errorf("the run does not say it left those names alone:\n%s", out)
	}
}

func TestInstallKimiRelativeHooksDir(t *testing.T) {
	repo := kimiRepo(t)
	configPath, _ := kimiHome(t, "")

	_, out, err := installKimi(t, repo, "hooks", configPath, nil, nil, false)
	if err == nil || !strings.Contains(err.Error(), "not absolute") {
		t.Fatalf("a relative hooks dir was accepted: err = %v\n%s", err, out)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Errorf("a refused install still wrote config.toml (%v)", statErr)
	}
}

// kimiTree lists every file below dir with its mode and content, so a test can
// say "nothing changed" about the whole tree.
func kimiTree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out = append(out, rel+" "+info.Mode().Perm().String()+" "+string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("Walk(%s): %v", dir, err)
	}
	slices.Sort(out)
	return out
}
