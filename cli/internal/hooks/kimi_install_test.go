package hooks

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
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

	// The name the comment on isKimiHookPath warns about, with a real file to
	// lose: two levels up from the hooks directory is the Kimi root's parent,
	// which for the default root is $HOME.
	sshKey := filepath.Join(filepath.Dir(filepath.Dir(hooksDir)), ".ssh", "id_rsa")
	if err := os.MkdirAll(filepath.Dir(sshKey), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(sshKey, []byte("not a key\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bad := []string{"../config.toml.keepme", "../../.ssh/id_rsa", "/etc/passwd", "claude-code/../../x.sh", "kimi/adapter.sh\nfake"}
	got, out, err := installKimi(t, repo, hooksDir, configPath, nil, bad, false)
	if err != nil {
		t.Fatalf("InstallKimi: %v\n%s", err, out)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("a traversal in the manifest removed a file outside the hooks directory: %v", err)
	}
	if _, err := os.Stat(sshKey); err != nil {
		t.Errorf("a two-level traversal in the manifest removed %q: %v", sshKey, err)
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

// The other half-finished path: the scripts are all on disk and the
// registration is refused. The scripts are inert without a command naming
// them, but they are real files in the user's Kimi root, and the manifest is
// the only record that they are devexp's.
func TestInstallKimiFailedRegistrationRecordsTheScripts(t *testing.T) {
	repo := kimiRepo(t)
	// A config.toml Kimi cannot read either, so devexp refuses it untouched.
	configPath, hooksDir := kimiHome(t, "model = \"unterminated\n")

	got, out, err := installKimi(t, repo, hooksDir, configPath, nil, nil, false)
	if err == nil {
		t.Fatalf("an unparseable config.toml was written to anyway\n%s", out)
	}
	for _, rel := range kimiInstallFiles(nil) {
		if !slices.Contains(got, rel) {
			t.Errorf("the refused registration dropped %s from the record: %v", rel, got)
		}
	}
	if !slices.Contains(got, "claude-code/secret-guard.sh") {
		t.Errorf("the refused registration dropped the guards it copied: %v", got)
	}
	if strings.Contains(readFile(t, configPath), kimiBlockBegin) {
		t.Errorf("a refused config.toml was rewritten:\n%s", readFile(t, configPath))
	}
}

// ── the fallback timeout ─────────────────────────────────────────────────────
//
// A hook Kimi kills at its timeout does not block the tool call: runHook
// returns allowResult for the timeout kill like every other non-2 outcome. So
// a guard cancelled mid-scan is an ALLOW, and the registered timeout has to
// clear the ceiling of the guards' own scan budget — the largest budget a
// guard can actually run with — or the budget's own exit 2 never lands.
//
// scan-budget.test.sh checks the timeouts the registry SPELLS OUT. This checks
// the one it does not: kimiDefaultTimeout, which SelectKimi supplies when a
// kimi block omits `timeout`, and which exists for exactly this reason. Kimi's
// own default is 30 seconds, below the ceiling, so leaving the constant
// unpinned means a value that silently disarms every guard it applies to.
//
// The ceiling is read out of the shell that enforces it, so lowering one and
// not the other is caught rather than mirrored.
func TestKimiDefaultTimeoutClearsTheScanBudgetCeiling(t *testing.T) {
	ceilingMs := scanBudgetCeilingMs(t)
	if kimiDefaultTimeout*1000 <= ceilingMs {
		t.Fatalf("kimiDefaultTimeout is %ds, which is not above the %dms scan-budget ceiling — Kimi would kill a guard mid-scan, and it reads a killed hook as an allow", kimiDefaultTimeout, ceilingMs)
	}
	if kimiDefaultTimeout > kimiMaxTimeout || kimiDefaultTimeout < kimiMinTimeout {
		t.Errorf("kimiDefaultTimeout is %d, outside Kimi's %d..%d, so the entry would be invalid", kimiDefaultTimeout, kimiMinTimeout, kimiMaxTimeout)
	}

	// And the path that uses it: a kimi block with no timeout of its own.
	registry, err := ParseRegistry([]byte(`[
  {"name": "no-timeout-guard", "enabled": true,
   "kimi": {"event": "PreToolUse", "matcher": "^Bash$", "script": "hooks/claude-code/secret-guard.sh", "fail_closed": true}}
]`))
	if err != nil {
		t.Fatalf("ParseRegistry: %v", err)
	}
	selected, _ := SelectKimi(registry, nil)
	if len(selected) != 1 {
		t.Fatalf("SelectKimi selected %d hooks, want 1", len(selected))
	}
	if selected[0].Timeout*1000 <= ceilingMs {
		t.Errorf("a kimi block with no timeout is registered with %ds, which does not clear the %dms ceiling", selected[0].Timeout, ceilingMs)
	}
}

// scanBudgetCeilingMs reads DEVEXP_SCAN_BUDGET_MAX_MS out of the shell helper
// that enforces it.
func scanBudgetCeilingMs(t *testing.T) int {
	t.Helper()
	path := filepath.Join("..", "..", "..", "hooks", "claude-code", "scan-budget.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	m := regexp.MustCompile(`(?m)^DEVEXP_SCAN_BUDGET_MAX_MS=(\d+)`).FindSubmatch(data)
	if m == nil {
		t.Fatalf("%s no longer sets DEVEXP_SCAN_BUDGET_MAX_MS, so the ceiling this test compares against is gone", path)
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("DEVEXP_SCAN_BUDGET_MAX_MS is not a number: %v", err)
	}
	return n
}

// ── the installed mode ───────────────────────────────────────────────────────
//
// The Kimi root sits beside the user's provider API keys, and a checkout with
// a slack umask must not widen what lands in it. The copy masks the source's
// mode to the owner's bits and forces read+execute, so a source that is
// world-writable arrives 0700 and a source that is not executable arrives
// executable anyway — a guard the adapter cannot run is an allow.
func TestInstallKimiNarrowsTheInstalledMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	repo := kimiRepo(t)
	src := filepath.Join(repo, "hooks", "claude-code", "secret-guard.sh")
	if err := os.Chmod(src, 0o777); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	notExec := filepath.Join(repo, "hooks", "kimi", "adapter.sh")
	if err := os.Chmod(notExec, 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	configPath, hooksDir := kimiHome(t, "")

	if _, out, err := installKimi(t, repo, hooksDir, configPath, nil, nil, false); err != nil {
		t.Fatalf("InstallKimi: %v\n%s", err, out)
	}

	for rel, want := range map[string]os.FileMode{
		"claude-code/secret-guard.sh": 0o700, // 0777 narrowed to the owner
		"kimi/adapter.sh":             0o700, // 0644 given back its execute bit
	} {
		info, err := os.Stat(filepath.Join(hooksDir, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s installed %v, want %v", rel, got, want)
		}
	}
}
