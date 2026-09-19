package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"devexp/internal/config"
)

// ── doInstallKimi ─────────────────────────────────────────────────────────────
//
// Every case runs in a scratch HOME with KIMI_CODE_HOME pointed inside it, so
// nothing here can reach the developer's own ~/.kimi-code, ~/.claude or
// ~/.config.

// kimiRepo writes a source tree with two agents and two skills, one of the
// skills carrying a supporting file.
func kimiRepo(t *testing.T) string {
	t.Helper()
	repoDir := writeOpencodeHookRepo(t)
	agent := func(name, tools string) string {
		return "---\nname: " + name + "\ndescription: \"what " + name + " does\"\ncolor: cyan\nmemory: user\ntools: " + tools + "\n---\n\n# " + name + "\n\nRead `~/.claude/agents/other.md` and follow it.\n"
	}
	skill := func(name string) string {
		return "---\nname: " + name + "\ndescription: \"what " + name + " does\"\n---\n\n# " + name + "\n"
	}
	files := map[string]string{
		"agents/dev-agent.md":          agent("dev-agent", "Read, Bash, Agent, WebFetch"),
		"agents/other.md":              agent("other", "Read, Grep"),
		"agents/README.md":             "# never installed\n",
		"skills/graphify/SKILL.md":     skill("graphify"),
		"skills/graphify/refs/note.md": "# a supporting file\n",
		"skills/devxp/SKILL.md":        skill("devxp"),
	}
	for rel, content := range files {
		p := filepath.Join(repoDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repoDir
}

// kimiRun drives one install in the given scratch HOME and Kimi root.
func kimiRun(t *testing.T, repoDir string, opts *installOpts) (string, error) {
	t.Helper()
	if opts.repoDir == "" {
		opts.repoDir = repoDir
	}
	if opts.cfg == nil {
		opts.cfg = &config.Config{}
	}
	if opts.env == nil {
		opts.env = map[string]string{}
	}
	var err error
	out := captureStdout(t, func() { err = doInstallKimi(opts) })
	return out, err
}

// kimiScratch sets HOME and KIMI_CODE_HOME to fresh directories and returns the
// resolved paths. kimiCodeHome may be "" for the default ~/.kimi-code root.
func kimiScratch(t *testing.T, kimiCodeHome string) kimiPaths {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KIMI_CODE_HOME", kimiCodeHome)
	return testKimiPaths(t, kimiCodeHome, home)
}

func kimiManifest(t *testing.T, path string) (agents, skills []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m struct {
		Agents []string `json:"agents"`
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	sort.Strings(m.Agents)
	sort.Strings(m.Skills)
	return m.Agents, m.Skills
}

func TestKimiAgentsDir(t *testing.T) {
	// Always absolute, whatever the root: Kimi's expansion of `~` in a file
	// tool's path is not something devexp relies on.
	tests := map[string]string{
		"/home/u/.kimi-code": "/home/u/.kimi-code/agents",
		"/opt/kimi":          "/opt/kimi/agents",
	}
	for root, want := range tests {
		got := kimiAgentsDir(root)
		if got != want {
			t.Errorf("kimiAgentsDir(%q) = %q, want %q", root, got, want)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("kimiAgentsDir(%q) = %q, which is not absolute", root, got)
		}
	}
}

func TestDoInstallKimi_FreshInstall(t *testing.T) {
	repoDir := kimiRepo(t)
	p := kimiScratch(t, "")

	out, err := kimiRun(t, repoDir, &installOpts{})
	if err != nil {
		t.Fatalf("doInstallKimi() error = %v\n%s", err, out)
	}

	for _, rel := range []string{"agents/dev-agent.md", "agents/other.md",
		"skills/graphify/SKILL.md", "skills/graphify/refs/note.md", "skills/devxp/SKILL.md"} {
		if !exists(filepath.Join(p.root, filepath.FromSlash(rel))) {
			t.Errorf("%s was not installed", rel)
		}
	}
	if exists(filepath.Join(p.agents, "README.md")) {
		t.Error("agents/README.md was installed")
	}

	gotAgents, gotSkills := kimiManifest(t, p.manifest)
	if want := []string{"dev-agent.md", "other.md"}; !reflect.DeepEqual(gotAgents, want) {
		t.Errorf("manifest agents = %v, want %v", gotAgents, want)
	}
	if want := []string{"devxp", "graphify"}; !reflect.DeepEqual(gotSkills, want) {
		t.Errorf("manifest skills = %v, want %v", gotSkills, want)
	}

	// The transform ran, and it used the Kimi agents directory rather than
	// Claude Code's.
	installed := readCmdFile(t, filepath.Join(p.agents, "dev-agent.md"))
	if !strings.Contains(installed, "FetchURL") {
		t.Errorf("tools were not mapped for Kimi:\n%s", installed)
	}
	if strings.Contains(installed, "~/.claude/agents/") {
		t.Errorf("a body reference still points at Claude Code:\n%s", installed)
	}
	if !strings.Contains(installed, filepath.Join(p.root, "agents", "other.md")) {
		t.Errorf("the body reference was not repointed at the Kimi install:\n%s", installed)
	}
	if !strings.Contains(installed, "${base_prompt}") {
		t.Errorf("the installed agent does not opt into Kimi's base prompt:\n%s", installed)
	}
}

// A re-install of the same release must be a no-op on disk: nothing rewritten
// differently, nothing removed, nothing added.
func TestDoInstallKimi_ReinstallWritesNothingNew(t *testing.T) {
	repoDir := kimiRepo(t)
	p := kimiScratch(t, "")

	if out, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
		t.Fatalf("first install error = %v\n%s", err, out)
	}
	// The installed trees and the manifest, not the whole root: backupExisting
	// takes a fresh copy of whatever is already there on every run, so a second
	// run always adds a .devexp-backup-* directory. That is the Claude Code
	// path's behaviour too, and is not what this test is about.
	before := map[string]map[string]string{
		"agents": treeState(t, p.agents),
		"skills": treeState(t, p.skills),
	}
	beforeManifest := readCmdFile(t, p.manifest)

	out, err := kimiRun(t, repoDir, &installOpts{})
	if err != nil {
		t.Fatalf("second install error = %v\n%s", err, out)
	}
	after := map[string]map[string]string{
		"agents": treeState(t, p.agents),
		"skills": treeState(t, p.skills),
	}
	if !reflect.DeepEqual(before, after) {
		for kind := range before {
			if !reflect.DeepEqual(before[kind], after[kind]) {
				t.Errorf("a re-install changed %s:\nbefore %v\nafter  %v", kind, before[kind], after[kind])
			}
		}
	}
	if got := readCmdFile(t, p.manifest); got != beforeManifest {
		t.Errorf("a re-install changed the manifest:\nbefore %s\nafter  %s", beforeManifest, got)
	}
	if strings.Contains(out, "no longer in this release") {
		t.Errorf("a re-install reported something as stale:\n%s", out)
	}
	if strings.Contains(out, "left untouched") {
		t.Errorf("a re-install warned about its own files:\n%s", out)
	}
}

// A dry run must write nothing at all, and must still name every path a real
// run touches.
func TestDoInstallKimi_DryRunParity(t *testing.T) {
	repoDir := kimiRepo(t)
	p := kimiScratch(t, "")

	before := treeState(t, filepath.Dir(p.root))
	dry, err := kimiRun(t, repoDir, &installOpts{dryRun: true})
	if err != nil {
		t.Fatalf("dry run error = %v\n%s", err, dry)
	}
	if after := treeState(t, filepath.Dir(p.root)); !reflect.DeepEqual(before, after) {
		t.Errorf("a dry run wrote under the Kimi root:\nbefore %v\nafter  %v", before, after)
	}
	for _, want := range []string{
		filepath.Join(p.agents, "dev-agent.md"),
		filepath.Join(p.agents, "other.md"),
		filepath.Join(p.skills, "graphify"),
		filepath.Join(p.skills, "devxp"),
	} {
		if !strings.Contains(dry, want) {
			t.Errorf("the dry run does not name %s:\n%s", want, dry)
		}
	}

	// And the real run afterwards produces exactly what the preview named.
	if out, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
		t.Fatalf("real install error = %v\n%s", err, out)
	}
	for _, want := range []string{
		filepath.Join(p.agents, "dev-agent.md"),
		filepath.Join(p.skills, "graphify", "SKILL.md"),
	} {
		if !exists(want) {
			t.Errorf("%s was previewed but not written by the real run", want)
		}
	}
}

// A file the previous run installed and this one no longer ships is removed;
// a file of the user's own with a name devexp does not install is not.
func TestDoInstallKimi_StaleEntryRemovedAndUserFilesKept(t *testing.T) {
	repoDir := kimiRepo(t)
	p := kimiScratch(t, "")

	if out, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
		t.Fatalf("first install error = %v\n%s", err, out)
	}

	// The user's own agent and skill, which devexp never installed.
	mineAgent := filepath.Join(p.agents, "mine.md")
	mineSkill := filepath.Join(p.skills, "mine", "SKILL.md")
	for _, f := range []string{mineAgent, mineSkill} {
		os.MkdirAll(filepath.Dir(f), 0o755)      //nolint:errcheck
		os.WriteFile(f, []byte("keep\n"), 0o644) //nolint:errcheck
	}

	// Drop one agent and one skill from the release.
	os.Remove(filepath.Join(repoDir, "agents", "other.md")) //nolint:errcheck
	os.RemoveAll(filepath.Join(repoDir, "skills", "devxp")) //nolint:errcheck

	out, err := kimiRun(t, repoDir, &installOpts{})
	if err != nil {
		t.Fatalf("second install error = %v\n%s", err, out)
	}
	if exists(filepath.Join(p.agents, "other.md")) {
		t.Errorf("the stale agent was not removed:\n%s", out)
	}
	if exists(filepath.Join(p.skills, "devxp")) {
		t.Errorf("the stale skill was not removed:\n%s", out)
	}
	for _, f := range []string{mineAgent, mineSkill} {
		if got := readCmdFile(t, f); got != "keep\n" {
			t.Errorf("%s = %q, want the user's own file untouched\n%s", f, got, out)
		}
	}
	gotAgents, gotSkills := kimiManifest(t, p.manifest)
	if want := []string{"dev-agent.md"}; !reflect.DeepEqual(gotAgents, want) {
		t.Errorf("manifest agents = %v, want %v", gotAgents, want)
	}
	if want := []string{"graphify"}; !reflect.DeepEqual(gotSkills, want) {
		t.Errorf("manifest skills = %v, want %v", gotSkills, want)
	}
}

// A user's own file with a name devexp does install is backed up before it is
// replaced, as on the Claude Code path.
func TestDoInstallKimi_ExistingFilesBackedUp(t *testing.T) {
	repoDir := kimiRepo(t)
	p := kimiScratch(t, "")

	os.MkdirAll(p.agents, 0o755)                                                           //nolint:errcheck
	os.MkdirAll(filepath.Join(p.skills, "graphify"), 0o755)                                //nolint:errcheck
	os.WriteFile(filepath.Join(p.agents, "dev-agent.md"), []byte("mine\n"), 0o644)         //nolint:errcheck
	os.WriteFile(filepath.Join(p.skills, "graphify", "SKILL.md"), []byte("mine\n"), 0o644) //nolint:errcheck

	out, err := kimiRun(t, repoDir, &installOpts{})
	if err != nil {
		t.Fatalf("doInstallKimi() error = %v\n%s", err, out)
	}
	backups, _ := filepath.Glob(filepath.Join(p.root, ".devexp-backup-*"))
	if len(backups) != 1 {
		t.Fatalf("backup directories = %v, want exactly one", backups)
	}
	for _, rel := range []string{"dev-agent.md", filepath.Join("graphify", "SKILL.md")} {
		if got := readCmdFile(t, filepath.Join(backups[0], rel)); got != "mine\n" {
			t.Errorf("backup of %s = %q, want the user's original", rel, got)
		}
	}
	if got := readCmdFile(t, filepath.Join(p.agents, "dev-agent.md")); got == "mine\n" {
		t.Error("the agent was not replaced after being backed up")
	}
}

// #128: nothing is removed through a symlinked target directory or from behind
// one, and what could not be removed stays in the manifest so a later run can
// finish the job.
func TestDoInstallKimi_RemovalBlockedBySymlink(t *testing.T) {
	t.Run("a symlinked target directory", func(t *testing.T) {
		repoDir := kimiRepo(t)
		p := kimiScratch(t, "")
		real := t.TempDir()
		os.MkdirAll(p.root, 0o755) //nolint:errcheck
		mustSymlink(t, real, p.agents)

		if out, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
			t.Fatalf("first install error = %v\n%s", err, out)
		}
		// The install writes through a symlinked directory, as the other
		// targets do; only removal is refused.
		if !exists(filepath.Join(real, "dev-agent.md")) {
			t.Error("the install did not write through the symlinked directory")
		}

		os.Remove(filepath.Join(repoDir, "agents", "other.md")) //nolint:errcheck
		out, err := kimiRun(t, repoDir, &installOpts{})
		if err != nil {
			t.Fatalf("second install error = %v\n%s", err, out)
		}
		if !exists(filepath.Join(real, "other.md")) {
			t.Errorf("a file was removed through a symlinked directory:\n%s", out)
		}
		if !strings.Contains(out, "is a symlink") || !strings.Contains(out, "remove these by hand") {
			t.Errorf("the blocked removal was not reported:\n%s", out)
		}
		gotAgents, _ := kimiManifest(t, p.manifest)
		if !reflect.DeepEqual(gotAgents, []string{"dev-agent.md", "other.md"}) {
			t.Errorf("manifest agents = %v, want the kept entry retained so a later run retries", gotAgents)
		}
	})

	// A $KIMI_CODE_HOME that is itself a symlink. The install follows it; the
	// removal guard resolves the target directory from the Kimi root's parent
	// and refuses.
	t.Run("a Kimi root behind a symlink", func(t *testing.T) {
		repoDir := kimiRepo(t)
		home := t.TempDir()
		real := filepath.Join(t.TempDir(), "real-kimi")
		os.MkdirAll(real, 0o755) //nolint:errcheck
		link := filepath.Join(home, "linked-kimi")
		mustSymlink(t, real, link)
		t.Setenv("HOME", home)
		t.Setenv("KIMI_CODE_HOME", link)
		p := testKimiPaths(t, link, home)

		if out, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
			t.Fatalf("first install error = %v\n%s", err, out)
		}
		if !exists(filepath.Join(real, "agents", "dev-agent.md")) {
			t.Error("the install did not write through the linked Kimi root")
		}

		os.Remove(filepath.Join(repoDir, "agents", "other.md")) //nolint:errcheck
		out, err := kimiRun(t, repoDir, &installOpts{})
		if err != nil {
			t.Fatalf("second install error = %v\n%s", err, out)
		}
		if !exists(filepath.Join(real, "agents", "other.md")) {
			t.Errorf("a file was removed from behind a symlinked Kimi root:\n%s", out)
		}
		if !strings.Contains(out, "behind a symlink") {
			t.Errorf("the blocked removal was not reported:\n%s", out)
		}
		gotAgents, _ := kimiManifest(t, p.manifest)
		if !reflect.DeepEqual(gotAgents, []string{"dev-agent.md", "other.md"}) {
			t.Errorf("manifest agents = %v, want the kept entry retained", gotAgents)
		}
	})
}

// A $KIMI_CODE_HOME outside $HOME is the whole reason that variable exists, and
// stale removal has to keep working there. removeStale is given the Kimi root's
// parent, not $HOME; with $HOME the removal guard would report the directory as
// "not under home" and every removal would quietly do nothing.
func TestDoInstallKimi_StaleRemovalOutsideHome(t *testing.T) {
	repoDir := kimiRepo(t)
	outside := filepath.Join(t.TempDir(), "kimi")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KIMI_CODE_HOME", outside)
	p := testKimiPaths(t, outside, home)

	if out, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
		t.Fatalf("first install error = %v\n%s", err, out)
	}
	os.Remove(filepath.Join(repoDir, "agents", "other.md")) //nolint:errcheck

	out, err := kimiRun(t, repoDir, &installOpts{})
	if err != nil {
		t.Fatalf("second install error = %v\n%s", err, out)
	}
	if exists(filepath.Join(p.agents, "other.md")) {
		t.Errorf("a stale agent survived under a Kimi root outside HOME:\n%s", out)
	}
	if strings.Contains(out, "can't be checked") {
		t.Errorf("the removal guard could not check a Kimi root outside HOME:\n%s", out)
	}
	gotAgents, _ := kimiManifest(t, p.manifest)
	if !reflect.DeepEqual(gotAgents, []string{"dev-agent.md"}) {
		t.Errorf("manifest agents = %v, want the stale entry dropped", gotAgents)
	}
}

// #124: a symlinked entry is left untouched, its name stays in the manifest,
// and it is never reported as stale on the next run.
func TestDoInstallKimi_SymlinkedEntriesKept(t *testing.T) {
	repoDir := kimiRepo(t)
	p := kimiScratch(t, "")
	dotfiles := t.TempDir()

	mine := filepath.Join(dotfiles, "dev-agent.md")
	os.WriteFile(mine, []byte("my own agent\n"), 0o644) //nolint:errcheck
	mineSkill := filepath.Join(dotfiles, "graphify")
	os.MkdirAll(mineSkill, 0o755)                                                 //nolint:errcheck
	os.WriteFile(filepath.Join(mineSkill, "SKILL.md"), []byte("my own\n"), 0o644) //nolint:errcheck

	os.MkdirAll(p.agents, 0o755) //nolint:errcheck
	os.MkdirAll(p.skills, 0o755) //nolint:errcheck
	mustSymlink(t, mine, filepath.Join(p.agents, "dev-agent.md"))
	mustSymlink(t, mineSkill, filepath.Join(p.skills, "graphify"))

	out, err := kimiRun(t, repoDir, &installOpts{})
	if err != nil {
		t.Fatalf("doInstallKimi() error = %v\n%s", err, out)
	}
	if got := readCmdFile(t, mine); got != "my own agent\n" {
		t.Errorf("the symlinked agent was written through: %q", got)
	}
	if got := readCmdFile(t, filepath.Join(mineSkill, "SKILL.md")); got != "my own\n" {
		t.Errorf("the symlinked skill was written through: %q", got)
	}
	// Still tracked, so the next run does not treat them as stale.
	gotAgents, gotSkills := kimiManifest(t, p.manifest)
	if !reflect.DeepEqual(gotAgents, []string{"dev-agent.md", "other.md"}) {
		t.Errorf("manifest agents = %v, want the symlinked entry kept", gotAgents)
	}
	if !reflect.DeepEqual(gotSkills, []string{"devxp", "graphify"}) {
		t.Errorf("manifest skills = %v, want the symlinked entry kept", gotSkills)
	}
}

func TestDoInstallKimi_Scopes(t *testing.T) {
	t.Run("--agents-only leaves the skills manifest alone", func(t *testing.T) {
		repoDir := kimiRepo(t)
		p := kimiScratch(t, "")
		if out, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
			t.Fatalf("first install error = %v\n%s", err, out)
		}
		os.RemoveAll(filepath.Join(repoDir, "skills", "devxp")) //nolint:errcheck

		out, err := kimiRun(t, repoDir, &installOpts{agentsOnly: true})
		if err != nil {
			t.Fatalf("doInstallKimi() error = %v\n%s", err, out)
		}
		_, gotSkills := kimiManifest(t, p.manifest)
		if want := []string{"devxp", "graphify"}; !reflect.DeepEqual(gotSkills, want) {
			t.Errorf("manifest skills = %v, want %v — an agents-only run must not retire a skill", gotSkills, want)
		}
		if !exists(filepath.Join(p.skills, "devxp")) {
			t.Error("an agents-only run removed a skill")
		}
	})

	t.Run("--skills-only leaves the agents manifest alone", func(t *testing.T) {
		repoDir := kimiRepo(t)
		p := kimiScratch(t, "")
		if out, err := kimiRun(t, repoDir, &installOpts{}); err != nil {
			t.Fatalf("first install error = %v\n%s", err, out)
		}
		os.Remove(filepath.Join(repoDir, "agents", "other.md")) //nolint:errcheck

		out, err := kimiRun(t, repoDir, &installOpts{skillsOnly: true})
		if err != nil {
			t.Fatalf("doInstallKimi() error = %v\n%s", err, out)
		}
		gotAgents, _ := kimiManifest(t, p.manifest)
		if want := []string{"dev-agent.md", "other.md"}; !reflect.DeepEqual(gotAgents, want) {
			t.Errorf("manifest agents = %v, want %v", gotAgents, want)
		}
		if !exists(filepath.Join(p.agents, "other.md")) {
			t.Error("a skills-only run removed an agent")
		}
	})

	t.Run("--mcps-only installs nothing until #112", func(t *testing.T) {
		repoDir := kimiRepo(t)
		p := kimiScratch(t, "")
		before := treeState(t, filepath.Dir(p.root))
		out, err := kimiRun(t, repoDir, &installOpts{mcpsOnly: true})
		if err != nil {
			t.Fatalf("doInstallKimi() error = %v\n%s", err, out)
		}
		if after := treeState(t, filepath.Dir(p.root)); !reflect.DeepEqual(before, after) {
			t.Error("an --mcps-only run wrote agents or skills")
		}
		if !strings.Contains(out, "#112") {
			t.Errorf("the output does not say why nothing was installed:\n%s", out)
		}
	})
}

func TestDoInstallKimi_DisabledAndModel(t *testing.T) {
	t.Run("a disabled agent is not installed and not tracked", func(t *testing.T) {
		repoDir := kimiRepo(t)
		p := kimiScratch(t, "")
		out, err := kimiRun(t, repoDir, &installOpts{cfg: &config.Config{DisabledAgents: []string{"other"}, DisabledSkills: []string{"devxp"}}})
		if err != nil {
			t.Fatalf("doInstallKimi() error = %v\n%s", err, out)
		}
		gotAgents, gotSkills := kimiManifest(t, p.manifest)
		if !reflect.DeepEqual(gotAgents, []string{"dev-agent.md"}) {
			t.Errorf("manifest agents = %v, want only the enabled one", gotAgents)
		}
		if !reflect.DeepEqual(gotSkills, []string{"graphify"}) {
			t.Errorf("manifest skills = %v, want only the enabled one", gotSkills)
		}
		if exists(filepath.Join(p.agents, "other.md")) || exists(filepath.Join(p.skills, "devxp")) {
			t.Error("a disabled agent or skill was installed")
		}
		// A disabled agent must not be offered as a delegation target either.
		if strings.Contains(readCmdFile(t, filepath.Join(p.agents, "dev-agent.md")), "- other\n") {
			t.Error("a disabled agent appears in another agent's subagents list")
		}
	})

	// Kimi has no per-agent model, so a --model override cannot be honoured —
	// and must not be quietly ignored.
	t.Run("--model is a no-op with a warning", func(t *testing.T) {
		repoDir := kimiRepo(t)
		kimiScratch(t, "")
		out, err := kimiRun(t, repoDir, &installOpts{cfg: &config.Config{Model: "sonnet"}})
		if err != nil {
			t.Fatalf("doInstallKimi() error = %v\n%s", err, out)
		}
		if !strings.Contains(out, "ignored for Kimi Code CLI") || !strings.Contains(out, "sonnet") {
			t.Errorf("no warning that the model override is ignored:\n%s", out)
		}
	})
}

func readCmdFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
