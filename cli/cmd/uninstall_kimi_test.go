package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Uninstall (Kimi Code CLI) ─────────────────────────────────────────────────
//
// The manifest is the entire ownership record for the Kimi root, so every test
// here is really one question: with this manifest and this disk, what does
// devexp decide is its own? Getting that wrong in either direction is a bug
// with a user's files on the other end of it.

// installedKimi does a real install into a fresh scratch HOME and hands back
// the paths. Nothing is faked: a test that asserted an uninstall against a
// hand-built tree would pass while install and uninstall disagreed, which is
// the one failure this pairing exists to catch.
func installedKimi(t *testing.T) (repoDir string, p kimiPaths) {
	t.Helper()
	repoDir = kimiAssetRepo(t)
	p = kimiScratch(t, "")
	if _, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	return repoDir, p
}

// uninstallKimi runs the handler in the HOME the scratch helpers set.
func uninstallKimi(t *testing.T, dryRun bool) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() { err = doUninstallKimi(os.Getenv("HOME"), dryRun) })
	return out, err
}

func readKimiManifest(t *testing.T, path string) manifestShape {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m manifestShape
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m
}

type manifestShape struct {
	Agents []string          `json:"agents"`
	Skills []string          `json:"skills"`
	Hooks  []string          `json:"hooks"`
	MCPs   map[string]string `json:"mcps"`
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustNotExist(t *testing.T, path, why string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s still exists (%v) — %s", path, err, why)
	}
}

func mustExist(t *testing.T, path, why string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Errorf("%s is gone (%v) — %s", path, err, why)
	}
}

// TestDoUninstallKimi_RoundTrip is the whole promise in one test: install
// everything, take it out, and the root is as the user left it — their own
// files, their own config.toml settings and their own mcp.json entry intact.
func TestDoUninstallKimi_RoundTrip(t *testing.T) {
	repoDir := kimiAssetRepo(t)
	p := kimiScratch(t, "")

	// The user's own belongings, put there before devexp ever ran.
	if err := os.MkdirAll(p.root, 0o700); err != nil {
		t.Fatal(err)
	}
	userConfig := "[general]\ntheme = \"dark\"\n\n[[hooks]]\nevent = \"PreToolUse\"\ncommand = \"echo mine\"\n"
	writeFile(t, p.config, userConfig)
	writeFile(t, filepath.Join(p.agents, "my-own.md"), "my own agent\n")
	writeFile(t, filepath.Join(p.skills, "mine", "SKILL.md"), "# mine\n")
	writeFile(t, p.mcp, `{"mcpServers":{"my-own":{"command":"echo"}}}`)

	if _, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	// Sanity: the install really did write, or the uninstall proves nothing.
	mustExist(t, p.manifest, "the install should have recorded what it wrote")
	mustExist(t, filepath.Join(p.agents, "dev-agent.md"), "the install should have written an agent")

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v (%s)", err, out)
	}

	mustNotExist(t, p.manifest, "nothing of devexp's is left, so the record goes too")
	mustNotExist(t, filepath.Join(p.agents, "dev-agent.md"), "an installed agent must be removed")
	mustNotExist(t, filepath.Join(p.skills, "graphify"), "an installed skill must be removed")
	mustNotExist(t, p.hooks, "the emptied hooks tree must be pruned")

	// The user's files, untouched.
	mustExist(t, filepath.Join(p.agents, "my-own.md"), "a file the user wrote is never devexp's")
	mustExist(t, filepath.Join(p.skills, "mine", "SKILL.md"), "a skill the user wrote is never devexp's")

	got, err := os.ReadFile(p.config)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	if !strings.Contains(string(got), `theme = "dark"`) {
		t.Errorf("the user's own config.toml settings were lost:\n%s", got)
	}
	if !strings.Contains(string(got), "echo mine") {
		t.Errorf("the user's own [[hooks]] entry was removed:\n%s", got)
	}
	if strings.Contains(string(got), "adapter.sh") {
		t.Errorf("a devexp hook registration survived the uninstall:\n%s", got)
	}

	var mcp struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	raw, err := os.ReadFile(p.mcp)
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	if err := json.Unmarshal(raw, &mcp); err != nil {
		t.Fatalf("parse mcp.json: %v", err)
	}
	if _, ok := mcp.Servers["my-own"]; !ok {
		t.Errorf("the user's own MCP entry was removed:\n%s", raw)
	}
	if _, ok := mcp.Servers["probe"]; ok {
		t.Errorf("devexp's own MCP entry survived:\n%s", raw)
	}

	// Said out loud, because both look like leftovers otherwise.
	for _, want := range []string{".devexp-backup-", "agent-memory"} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary does not say %q is kept on purpose:\n%s", want, out)
		}
	}
}

// TestDoUninstallKimi_MissingManifest: with no record there is nothing devexp
// may claim, so it must remove nothing at all — not even files that happen to
// carry the names it installs.
func TestDoUninstallKimi_MissingManifest(t *testing.T) {
	p := kimiScratch(t, "")
	// A tree that looks exactly like a devexp install, with no manifest. It is
	// the user's, and guessing would delete it.
	writeFile(t, filepath.Join(p.agents, "dev-agent.md"), "looks like ours, is not\n")
	writeFile(t, filepath.Join(p.skills, "graphify", "SKILL.md"), "# also not ours\n")

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("a missing manifest must not be an error: %v", err)
	}
	if !strings.Contains(out, "No devexp install recorded") {
		t.Errorf("the run does not say there is nothing recorded:\n%s", out)
	}
	mustExist(t, filepath.Join(p.agents, "dev-agent.md"), "with no manifest, nothing is devexp's")
	mustExist(t, filepath.Join(p.skills, "graphify", "SKILL.md"), "with no manifest, nothing is devexp's")
}

// TestDoUninstallKimi_PartialInstall: a manifest recording more than is on
// disk — the state a part-way install leaves. The entries that are there go,
// the ones that never existed are not an error, and the record is cleared.
func TestDoUninstallKimi_PartialInstall(t *testing.T) {
	p := kimiScratch(t, "")
	writeFile(t, filepath.Join(p.agents, "dev-agent.md"), "installed\n")
	writeFile(t, p.manifest, `{"agents":["dev-agent.md","never-written.md"],"skills":["gone-too"]}`)

	if _, err := uninstallKimi(t, false); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	mustNotExist(t, filepath.Join(p.agents, "dev-agent.md"), "the one entry that existed must go")
	mustNotExist(t, p.manifest, "an entry that was never on disk is not a reason to keep the record")
}

// TestDoUninstallKimi_HookPathOutsideHooksDir: the manifest is a file on disk,
// so a path in it is untrusted input. One that escapes the hooks directory
// must be refused by name and kept in the record, never joined and removed.
func TestDoUninstallKimi_HookPathOutsideHooksDir(t *testing.T) {
	p := kimiScratch(t, "")
	outside := filepath.Join(p.home, "precious.txt")
	writeFile(t, outside, "not devexp's, not even in the Kimi root\n")
	writeFile(t, filepath.Join(p.hooks, "kimi", "adapter.sh"), "#!/bin/sh\n")
	writeFile(t, p.manifest, `{"agents":[],"skills":[],"hooks":["../../precious.txt","kimi/adapter.sh"]}`)

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	mustExist(t, outside, "a manifest path that escapes the hooks dir must never be followed")
	mustNotExist(t, filepath.Join(p.hooks, "kimi", "adapter.sh"), "the legitimate entry beside it must still go")
	if !strings.Contains(out, "precious.txt") || !strings.Contains(out, "not a name devexp installs") {
		t.Errorf("the refusal was not reported:\n%s", out)
	}
	// Kept in the record: devexp could not act on it, so it must not forget it.
	if m := readKimiManifest(t, p.manifest); len(m.Hooks) != 1 || !strings.Contains(m.Hooks[0], "precious.txt") {
		t.Errorf("the refused path was not kept in the manifest: %+v", m)
	}
}

// TestDoUninstallKimi_UserEditedMCPEntry: the fingerprint is what tells
// devexp's entry from the user's. Once they have changed it, it is theirs.
func TestDoUninstallKimi_UserEditedMCPEntry(t *testing.T) {
	repoDir, p := installedKimi(t)
	_ = repoDir

	before := readKimiManifest(t, p.manifest)
	if len(before.MCPs) == 0 {
		t.Fatal("the install recorded no MCP ownership, so this test proves nothing")
	}

	// The user edits the entry devexp wrote.
	raw, err := os.ReadFile(p.mcp)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers["probe"] == nil {
		t.Fatalf("expected devexp's probe entry in %s:\n%s", p.mcp, raw)
	}
	servers["probe"] = map[string]any{"command": "echo", "args": []any{"mine now"}}
	edited, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p.mcp, string(edited))

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	after, err := os.ReadFile(p.mcp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "mine now") {
		t.Errorf("an entry the user edited was removed anyway:\n%s", after)
	}
	if !strings.Contains(out, "edited") {
		t.Errorf("the run does not say the entry was left because it changed:\n%s", out)
	}
}

// TestDoUninstallKimi_SymlinkedEntryKept: a symlink is the user's own setup,
// wherever it points. devexp never removes one — on install or on uninstall.
func TestDoUninstallKimi_SymlinkedEntryKept(t *testing.T) {
	p := kimiScratch(t, "")
	real := filepath.Join(p.home, "dotfiles", "dev-agent.md")
	writeFile(t, real, "the user's real file, somewhere else\n")
	if err := os.MkdirAll(p.agents, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(p.agents, "dev-agent.md")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeFile(t, p.manifest, `{"agents":["dev-agent.md"],"skills":[]}`)

	if _, err := uninstallKimi(t, false); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	mustExist(t, link, "a symlinked entry is never removed")
	mustExist(t, real, "and what it points at is certainly never removed")
}

// TestDoUninstallKimi_SymlinkedRootKept: devexp writes through a symlinked
// target directory but never removes through one — a dotfiles checkout must
// not be edited from here. The guard resolves from the Kimi root's parent, so
// it has to hold for a linked root too.
func TestDoUninstallKimi_SymlinkedRootKept(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(home, "dotfiles", "kimi")
	if err := os.MkdirAll(filepath.Join(real, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".kimi-code")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("KIMI_CODE_HOME", "")

	agent := filepath.Join(real, "agents", "dev-agent.md")
	writeFile(t, agent, "installed through a linked root\n")
	writeFile(t, filepath.Join(real, ".devexp-manifest.json"), `{"agents":["dev-agent.md"],"skills":[]}`)

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	mustExist(t, agent, "nothing is removed through a symlinked Kimi root")
	if !strings.Contains(out, "symlink") {
		t.Errorf("the run does not say why nothing was removed:\n%s", out)
	}
}

// TestDoUninstallKimi_DryRunRemovesNothing: the preview uninstall.sh shows
// before its confirmation. It must be exactly a preview.
func TestDoUninstallKimi_DryRunRemovesNothing(t *testing.T) {
	_, p := installedKimi(t)

	before := readKimiManifest(t, p.manifest)
	out, err := uninstallKimi(t, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, "Dry run") {
		t.Errorf("the run does not say it was a dry run:\n%s", out)
	}
	mustExist(t, p.manifest, "a dry run removes nothing, the manifest least of all")
	mustExist(t, filepath.Join(p.agents, "dev-agent.md"), "a dry run removes no agent")
	mustExist(t, filepath.Join(p.skills, "graphify"), "a dry run removes no skill")
	// Hook scripts are covered separately (TestDoUninstallKimi_DryRunKeepsHookScripts):
	// the shared fixture repo ships no hook Kimi can honour, so this install
	// writes no hooks directory to assert about.

	after := readKimiManifest(t, p.manifest)
	if len(after.Agents) != len(before.Agents) || len(after.Skills) != len(before.Skills) {
		t.Errorf("a dry run rewrote the manifest: %+v -> %+v", before, after)
	}

	// And the real run after it still works: a dry run must not leave state
	// that makes the removal a no-op.
	if _, err := uninstallKimi(t, false); err != nil {
		t.Fatalf("uninstall after dry run: %v", err)
	}
	mustNotExist(t, p.manifest, "the real run after a dry run must still remove everything")
}

// TestDoUninstallKimi_KeepsWhatItCouldNotRemove: what stays on disk stays in
// the record, so a later run can finish rather than forgetting it exists.
func TestDoUninstallKimi_KeepsWhatItCouldNotRemove(t *testing.T) {
	p := kimiScratch(t, "")
	// A recorded skill that is on disk as a plain file, not the directory the
	// manifest says: devexp removes only what has the shape it installed.
	writeFile(t, filepath.Join(p.skills, "graphify"), "not a skill directory\n")
	writeFile(t, p.manifest, `{"agents":[],"skills":["graphify"]}`)

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	mustExist(t, filepath.Join(p.skills, "graphify"), "an entry of the wrong shape is left alone")
	if !strings.Contains(out, "left untouched") {
		t.Errorf("the run does not say the entry was left:\n%s", out)
	}
	// And it is dropped from the record rather than kept: removeStale's rule,
	// inherited from the install side, is that something of the wrong shape is
	// not devexp's file at all. Keeping it would mean devexp went on claiming
	// a file it had just decided was the user's.
	mustNotExist(t, p.manifest, "nothing devexp owns is left, so the record goes")
}

// TestDoUninstallKimi_DryRunKeepsHookScripts covers what the shared fixture
// cannot: a hooks tree, recorded in the manifest, previewed and not removed.
func TestDoUninstallKimi_DryRunKeepsHookScripts(t *testing.T) {
	p := kimiScratch(t, "")
	adapter := filepath.Join(p.hooks, "kimi", "adapter.sh")
	guard := filepath.Join(p.hooks, "claude-code", "secret-guard.sh")
	writeFile(t, adapter, "#!/bin/sh\n")
	writeFile(t, guard, "#!/bin/sh\n")
	writeFile(t, p.config, "# devexp:hooks:begin\n[[hooks]]\nevent = \"PreToolUse\"\ncommand = \"bash '"+adapter+"' '"+guard+"'\"\n# devexp:hooks:end\n")
	writeFile(t, p.manifest, `{"agents":[],"skills":[],"hooks":["kimi/adapter.sh","claude-code/secret-guard.sh"]}`)

	if _, err := uninstallKimi(t, true); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	mustExist(t, adapter, "a dry run removes no hook script")
	mustExist(t, guard, "a dry run removes no hook script")
	mustExist(t, p.manifest, "a dry run removes no manifest")
	if got, err := os.ReadFile(p.config); err != nil || !strings.Contains(string(got), "devexp:hooks:begin") {
		t.Errorf("a dry run rewrote config.toml: %s (%v)", got, err)
	}

	// The real run then does remove them, so the preview was a preview of
	// something real rather than of nothing.
	if _, err := uninstallKimi(t, false); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	mustNotExist(t, adapter, "the real run removes the adapter")
	mustNotExist(t, guard, "the real run removes the guard")
	if got, err := os.ReadFile(p.config); err != nil || strings.Contains(string(got), "adapter.sh") {
		t.Errorf("the registration survived: %s (%v)", got, err)
	}
}

// TestDoUninstallKimi_SymlinkedManifestNotWritten: a manifest that is a
// symlink describes a file somewhere else. Saving through it would edit that
// file, so it is left exactly as it is.
func TestDoUninstallKimi_SymlinkedManifestNotWritten(t *testing.T) {
	p := kimiScratch(t, "")
	real := filepath.Join(p.home, "dotfiles", "manifest.json")
	writeFile(t, real, `{"agents":["dev-agent.md"],"skills":[]}`)
	if err := os.MkdirAll(p.root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, p.manifest); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeFile(t, filepath.Join(p.agents, "dev-agent.md"), "installed\n")

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	// The removal still happens — only the record is not written through.
	mustNotExist(t, filepath.Join(p.agents, "dev-agent.md"), "the entry is still removed")
	mustExist(t, p.manifest, "a symlinked manifest is never removed")
	if got, err := os.ReadFile(real); err != nil || !strings.Contains(string(got), "dev-agent.md") {
		t.Errorf("the file the manifest links to was rewritten: %s (%v)", got, err)
	}
	if !strings.Contains(out, "symlink") {
		t.Errorf("the run does not say the manifest was left alone:\n%s", out)
	}
}

// TestDoUninstallKimi_BadKimiHomeRemovesNothing: a root devexp must not write
// to is a root it must not remove from either, and the refusal comes before
// anything is read.
func TestDoUninstallKimi_BadKimiHomeRemovesNothing(t *testing.T) {
	home := t.TempDir()
	for _, bad := range []string{"relative/path", home} {
		t.Setenv("HOME", home)
		t.Setenv("KIMI_CODE_HOME", bad)
		if _, err := uninstallKimi(t, false); err == nil {
			t.Errorf("KIMI_CODE_HOME=%q was accepted; want a refusal", bad)
		}
	}
}

// TestDoUninstallKimi_CustomRootOutsideHome: kimiPaths.home is the Kimi root's
// *parent*, not $HOME, and that is the whole reason $KIMI_CODE_HOME may point
// outside the home directory at all. Passing $HOME to the removal guard
// instead would make it answer "not under home" for such a root, and every
// removal would quietly do nothing behind a warning — an uninstall that
// reports success and removes nothing.
//
// Only a root outside $HOME can tell the two apart: under the default
// ~/.kimi-code the parent *is* $HOME, so every other test here passes either
// way. Found by mutation (M5).
func TestDoUninstallKimi_CustomRootOutsideHome(t *testing.T) {
	repoDir := kimiAssetRepo(t)
	// Siblings, so the root is genuinely outside HOME rather than below it.
	base := t.TempDir()
	home := filepath.Join(base, "home")
	outside := filepath.Join(base, "elsewhere", "kimi")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("KIMI_CODE_HOME", outside)
	p := testKimiPaths(t, outside, home)

	if _, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	mustExist(t, filepath.Join(p.agents, "dev-agent.md"), "the install should have written into the custom root")
	// The hook scripts specifically: this assertion existed before the fixture
	// installed any, so it reached an empty list and the guard on the hook
	// path stayed untested while looking covered (PR #176 re-review). Each
	// kind gets its own removal guard call, so each needs its own evidence.
	before := readKimiManifest(t, p.manifest)
	if len(before.Hooks) == 0 {
		t.Fatal("the fixture recorded no hook scripts, so this test cannot reach the hook removal path")
	}
	for _, rel := range before.Hooks {
		mustExist(t, filepath.Join(p.hooks, filepath.FromSlash(rel)), "the install should have copied the hook script")
	}

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v (%s)", err, out)
	}
	mustNotExist(t, filepath.Join(p.agents, "dev-agent.md"), "an agent in a root outside HOME must still be removed")
	mustNotExist(t, filepath.Join(p.skills, "graphify"), "a skill in a root outside HOME must still be removed")
	// Every one of them, not just that the directory went: the mutant strands
	// all five scripts and reports "some entries were left in place", which is
	// a half-uninstall leaving every guard script behind.
	for _, rel := range before.Hooks {
		mustNotExist(t, filepath.Join(p.hooks, filepath.FromSlash(rel)), "a hook script in a root outside HOME must still be removed")
	}
	mustNotExist(t, p.manifest, "and the record goes with them")
	if strings.Contains(out, "not under") {
		t.Errorf("the removal guard refused a root outside HOME:\n%s", out)
	}
	if strings.Contains(out, "left in place") {
		t.Errorf("the run reported a half-uninstall:\n%s", out)
	}
}

// TestDoUninstallKimi_UnreadableManifest: a manifest that exists but will not
// parse. This is the state an interrupted write leaves — a power cut during
// an install is enough — and it used to be silently destructive: an empty
// manifest owns nothing, so nothing was removed; "the record is empty" was
// then trivially true, so the manifest was deleted; and the run printed
// success. Everything devexp wrote was stranded with no record of it, the
// guards deregistered while their scripts stayed, and a re-run said there was
// nothing to remove. Found in PR #176 review.
func TestDoUninstallKimi_UnreadableManifest(t *testing.T) {
	for name, body := range map[string]string{
		// Exactly what `head -c` on a real manifest produces.
		"truncated mid-write": `{"agents":["dev-agent.md","other.md"],"ski`,
		"not json at all":     "\x00\x01 not json",
		"wrong type":          `{"agents":{"a":1},"skills":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			p := kimiScratch(t, "")
			agent := filepath.Join(p.agents, "dev-agent.md")
			script := filepath.Join(p.hooks, "kimi", "adapter.sh")
			writeFile(t, agent, "installed\n")
			writeFile(t, script, "#!/bin/sh\n")
			writeFile(t, p.config, "# devexp:hooks:begin\n# devexp:hooks:end\n")
			writeFile(t, p.manifest, body)

			out, err := uninstallKimi(t, false)
			if err == nil {
				t.Fatalf("an unreadable manifest must stop the run, not remove things:\n%s", out)
			}
			// Nothing touched: not the files, and above all not the record,
			// which is the only thing that can still repair this by hand.
			mustExist(t, p.manifest, "the record must survive so the install stays repairable")
			mustExist(t, agent, "nothing may be removed without a record of what is devexp's")
			mustExist(t, script, "a hook script must not be stranded by deregistration")
			if got, rerr := os.ReadFile(p.manifest); rerr != nil || string(got) != body {
				t.Errorf("the manifest was rewritten: %q (%v)", got, rerr)
			}
			if got, rerr := os.ReadFile(p.config); rerr != nil || !strings.Contains(string(got), "devexp:hooks:begin") {
				t.Errorf("the hooks block was stripped while the scripts stayed: %q (%v)", got, rerr)
			}
			if !strings.Contains(err.Error(), "only record") {
				t.Errorf("the error does not say why nothing was removed: %v", err)
			}
			if strings.Contains(out, "Removed devexp from Kimi") {
				t.Errorf("the run claimed success:\n%s", out)
			}
		})
	}
}

// TestDoUninstallKimi_SymlinkedHooksDirKept: the hook scripts go through the
// same removal rules as agents and skills. They did not: a plain Lstat +
// Remove deleted the recorded script inside a dotfiles checkout when
// hooks/kimi, hooks/ or the root was symlinked, and then pruned the emptied
// linked directories — while agents/ and skills/ in the same run correctly
// refused. Found in PR #176 review.
func TestDoUninstallKimi_SymlinkedHooksDirKept(t *testing.T) {
	// Each case links a different level, because the guard has to refuse at
	// every one of them: the entry's own directory, its parent, and the root.
	for name, link := range map[string]string{
		"hooks/kimi is a symlink": "kimi",
		"hooks/ is a symlink":     "",
	} {
		t.Run(name, func(t *testing.T) {
			p := kimiScratch(t, "")
			dotfiles := filepath.Join(p.home, "dotfiles", "hooks")
			writeFile(t, filepath.Join(dotfiles, "kimi", "adapter.sh"), "#!/bin/sh\n")

			var linked string
			if link == "" {
				linked = p.hooks
			} else {
				linked = filepath.Join(p.hooks, link)
				if err := os.MkdirAll(p.hooks, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			src := dotfiles
			if link != "" {
				src = filepath.Join(dotfiles, "kimi")
			}
			if err := os.Symlink(src, linked); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			writeFile(t, p.manifest, `{"agents":[],"skills":[],"hooks":["kimi/adapter.sh"]}`)

			out, err := uninstallKimi(t, false)
			if err != nil {
				t.Fatalf("uninstall: %v", err)
			}
			mustExist(t, filepath.Join(dotfiles, "kimi", "adapter.sh"),
				"a hook script is never removed through a symlinked directory")
			mustExist(t, linked, "and the user's own symlink is never unlinked")
			// The precise reason, not just the word "symlink": the directory
			// devexp refuses being a link itself, and its *parent* being one,
			// are different situations and tell the user to look in different
			// places. Asserting only the shared word let a mutation that
			// dropped the first check entirely survive on the second one.
			want := "is a symlink"
			if link == "" {
				want = "is behind a symlink"
			}
			if !strings.Contains(out, want) {
				t.Errorf("the run does not say %q — the reason it gave was:\n%s", want, out)
			}
			// Kept in the record: it is still devexp's, and forgetting it
			// would strand it for ever.
			m := readKimiManifest(t, p.manifest)
			if len(m.Hooks) != 1 || m.Hooks[0] != "kimi/adapter.sh" {
				t.Errorf("the script devexp could not remove was dropped from the record: %+v", m)
			}
		})
	}
}

// TestDoUninstallKimi_EmptyDirBehindSymlinkNotPruned: the pruning step is
// subject to the same rule as the removal. A user whose hooks/ is symlinked
// into a dotfiles checkout, and who has already deleted the script by hand,
// leaves an empty kimi/ *inside that checkout* — which devexp must not tidy
// away, because it is reaching through a link to do it. Found by mutation
// (N9): the removal tests could not reach this, since nothing there empties a
// directory behind a link.
func TestDoUninstallKimi_EmptyDirBehindSymlinkNotPruned(t *testing.T) {
	p := kimiScratch(t, "")
	dotfiles := filepath.Join(p.home, "dotfiles", "hooks")
	// Real, empty, and inside the user's checkout.
	emptyDir := filepath.Join(dotfiles, "kimi")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p.hooks), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dotfiles, p.hooks); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Recorded, but already gone from disk — the ordinary "user tidied up
	// first" case.
	writeFile(t, p.manifest, `{"agents":[],"skills":[],"hooks":["kimi/adapter.sh"]}`)

	if _, err := uninstallKimi(t, false); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	mustExist(t, emptyDir, "an empty directory behind a symlinked parent is never pruned")
	mustExist(t, p.hooks, "and the user's own symlink is never unlinked")
	mustExist(t, dotfiles, "nor the directory it points at")
}

// TestDoUninstallKimi_HookScriptCaseMismatch: on a case-insensitive
// filesystem — the macOS default — joining a recorded name and opening it
// resolves a different file. Only the exact name the directory lists may be
// removed, which is what removeStale already does for agents and skills.
func TestDoUninstallKimi_HookScriptCaseMismatch(t *testing.T) {
	p := kimiScratch(t, "")
	onDisk := filepath.Join(p.hooks, "kimi", "adapter.sh")
	writeFile(t, onDisk, "the user's own, or ours under its real name\n")
	// The manifest records a spelling that is not what is on disk.
	writeFile(t, p.manifest, `{"agents":[],"skills":[],"hooks":["kimi/ADAPTER.sh"]}`)

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, statErr := os.Lstat(onDisk); statErr != nil {
		t.Fatalf("a name differing only in case removed %q:\n%s", onDisk, out)
	}
	if !strings.Contains(out, "differs in case") {
		t.Errorf("the run does not say why it was left:\n%s", out)
	}
}

// TestDoUninstallKimi_SymlinkedHookScriptKept: the entry itself being a
// symlink, with the directory perfectly ordinary. devexp never removes one,
// here as everywhere else.
func TestDoUninstallKimi_SymlinkedHookScriptKept(t *testing.T) {
	p := kimiScratch(t, "")
	real := filepath.Join(p.home, "dotfiles", "adapter.sh")
	writeFile(t, real, "#!/bin/sh\n")
	if err := os.MkdirAll(filepath.Join(p.hooks, "kimi"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(p.hooks, "kimi", "adapter.sh")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeFile(t, p.manifest, `{"agents":[],"skills":[],"hooks":["kimi/adapter.sh"]}`)

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	mustExist(t, link, "a symlinked hook script is never removed")
	mustExist(t, real, "and certainly not what it points at")
	if !strings.Contains(out, "symlink") {
		t.Errorf("the run does not say why it was left:\n%s", out)
	}
}

// TestDoUninstallKimi_HookScriptNotARegularFile: a recorded ".sh" name that is
// on disk as something else. The earlier repro used a *non-empty* directory,
// where removal fails anyway and the check never had to do anything — an
// empty directory, or a fifo, would go through the pinned handle unchecked
// (PR #176 re-review). Both are left alone, and both stay recorded.
func TestDoUninstallKimi_HookScriptNotARegularFile(t *testing.T) {
	t.Run("an empty directory", func(t *testing.T) {
		p := kimiScratch(t, "")
		asDir := filepath.Join(p.hooks, "kimi", "adapter.sh")
		if err := os.MkdirAll(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, p.manifest, `{"agents":[],"skills":[],"hooks":["kimi/adapter.sh"]}`)

		out, err := uninstallKimi(t, false)
		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		mustExist(t, asDir, "an empty directory under a recorded script name is not a script")
		if !strings.Contains(out, "not a regular file") {
			t.Errorf("the run does not say why it was left:\n%s", out)
		}
		if m := readKimiManifest(t, p.manifest); len(m.Hooks) != 1 {
			t.Errorf("the entry devexp would not remove was dropped from the record: %+v", m)
		}
	})

	t.Run("a fifo", func(t *testing.T) {
		p := kimiScratch(t, "")
		if err := os.MkdirAll(filepath.Join(p.hooks, "kimi"), 0o755); err != nil {
			t.Fatal(err)
		}
		fifo := filepath.Join(p.hooks, "kimi", "adapter.sh")
		if err := mkfifoForTest(fifo); err != nil {
			t.Skipf("fifo unavailable: %v", err)
		}
		writeFile(t, p.manifest, `{"agents":[],"skills":[],"hooks":["kimi/adapter.sh"]}`)

		out, err := uninstallKimi(t, false)
		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		mustExist(t, fifo, "a fifo under a recorded script name is not a script")
		if !strings.Contains(out, "not a regular file") {
			t.Errorf("the run does not say why it was left:\n%s", out)
		}
	})
}

// TestDoUninstallKimi_EmptyManifestNotDeleted: a manifest that parses but
// records nothing. devexp's own operations never write one, so this is not a
// live bug — but the code deleted it whenever the kept record came out empty,
// regardless of whether the old one held anything, which is a narrower form of
// the symptom the unreadable-manifest fix closed: manifest gone, files on
// disk, exit 0, "Removed devexp" (PR #176 re-review).
func TestDoUninstallKimi_EmptyManifestNotDeleted(t *testing.T) {
	for name, body := range map[string]string{
		"all fields empty": `{"agents":[],"skills":[],"hooks":[],"mcps":{}}`,
		"an empty object":  `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			p := kimiScratch(t, "")
			agent := filepath.Join(p.agents, "dev-agent.md")
			writeFile(t, agent, "on disk, unrecorded\n")
			writeFile(t, p.manifest, body)

			if _, err := uninstallKimi(t, false); err != nil {
				t.Fatalf("uninstall: %v", err)
			}
			mustExist(t, agent, "an unrecorded file is never devexp's")
			mustExist(t, p.manifest, "a manifest that recorded nothing was never emptied by this run")
		})
	}
}

// TestDoUninstallKimi_HookScriptNormalizationTwin: on APFS a name can exist
// only under a different Unicode normalization. Nothing wrong is removed
// either way, but the entry has to keep its place in the manifest and say so,
// or a real file is stranded with no record — the fold branch beside it and
// removeStale both already do this.
func TestDoUninstallKimi_HookScriptNormalizationTwin(t *testing.T) {
	p := kimiScratch(t, "")
	// "é" as NFC (U+00E9) on disk; the manifest records NFD (e + U+0301).
	onDisk := filepath.Join(p.hooks, "kimi", "caf\u00e9.sh")
	writeFile(t, onDisk, "#!/bin/sh\n")
	writeFile(t, p.manifest, `{"agents":[],"skills":[],"hooks":["kimi/cafe\u0301.sh"]}`)

	out, err := uninstallKimi(t, false)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	m := readKimiManifest(t, p.manifest)
	if _, statErr := os.Lstat(onDisk); statErr != nil {
		// The filesystem folded the two spellings together and removed it.
		// That is the case the fold/normalization branches exist to prevent.
		t.Fatalf("a name differing only in normalization removed %q:\n%s", onDisk, out)
	}
	// Either the handle can see it under the recorded spelling (APFS folds
	// normalization, so it can) and it must be reported and kept, or it
	// genuinely is not there and there is nothing to record.
	if strings.Contains(out, "Unicode normalization") {
		if len(m.Hooks) != 1 {
			t.Errorf("a reported normalization twin was dropped from the record: %+v", m)
		}
		return
	}
	t.Skip("this filesystem does not fold Unicode normalization, so the branch is unreachable here")
}
