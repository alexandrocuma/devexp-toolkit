package repocheck

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"devexp/internal/hooks"
)

// root is the repository root, relative to this package directory. Every path
// in this file is joined onto it, so the tests read the real assets rather than
// a fixture — a fixture would drift from the repo exactly like the prose does.
func root(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", ".."}, parts...)...)
}

func loadRegistry(t *testing.T) hooks.Registry {
	t.Helper()
	reg, err := hooks.LoadRegistry(root("hooks", "registry.json"))
	if err != nil {
		t.Fatalf("hooks/registry.json does not load: %v", err)
	}
	return reg
}

// readFile reads a repo file, failing the test with the path a developer would
// type rather than the test-relative one.
func readFile(t *testing.T, rel ...string) string {
	t.Helper()
	b, err := os.ReadFile(root(rel...))
	if err != nil {
		t.Fatalf("cannot read %s: %v", filepath.Join(rel...), err)
	}
	return string(b)
}

// ─── Hooks: the registry against the disk ────────────────────────────────────

// nonHookFiles are files that live beside the hooks but are not hooks, so they
// have no registry entry and must not be reported as missing one. Each is
// listed with what it is, because "add it to the allowlist" is the wrong fix
// for a real hook that was simply never registered — the whole point of
// TestHooks_OnDiskAreRegistered is to catch that case.
var nonHookFiles = map[string]string{
	// Sourced by the guards to enforce a wall-clock scan budget; it registers
	// nothing itself and is copied next to the guards for Kimi.
	"scan-budget.sh": "shared budget library, sourced by the guards",
	// The language-agnostic comment-reference scanner. comment-refs-on-save
	// invokes it per file and scripts/check-comment-refs.sh runs it over the
	// whole tree in CI, so it has two callers and no envelope of its own.
	"comment-refs.sh": "shared comment scanner, invoked by the hook and by CI",
	// The opencode plugin entry point. It *reads* the registry and wires every
	// module in it, so it is the consumer, never an entry.
	"devexp-plugin.js": "opencode plugin entry point, wires the registry",
	// Shared helpers imported by the opencode hook modules.
	"utils.js": "shared helpers imported by the hook modules",
}

// TestHooks_RegistryFilesExist checks the forward direction: every path the
// registry names resolves to a file. A registry entry pointing at a script that
// was renamed or deleted installs a hook registration for a command that cannot
// run — and a hook that cannot start is an allow, so the guard silently stops
// guarding.
func TestHooks_RegistryFilesExist(t *testing.T) {
	for _, h := range loadRegistry(t) {
		for _, f := range []struct {
			target, field, path string
		}{
			{hooks.TargetClaudeCode, "claude_code.script", mustSpec(t, h, hooks.TargetClaudeCode).Script},
			{hooks.TargetOpencode, "opencode.module", mustSpec(t, h, hooks.TargetOpencode).Module},
		} {
			if f.path == "" {
				t.Errorf("hooks/registry.json: %q has no %s.\n"+
					"  Fix: add it, or remove the %s block if the hook does not ship there.",
					h.Name, f.field, f.target)
				continue
			}
			if _, err := os.Stat(root(filepath.FromSlash(f.path))); err != nil {
				t.Errorf("hooks/registry.json: %q names %s = %q, which does not exist.\n"+
					"  Fix: create that file, or correct the path in the registry entry.",
					h.Name, f.field, f.path)
			}
		}
		// The kimi block reuses the Claude Code script, so it is checked too
		// when present — a stale path here ships a Kimi registration for a
		// command that cannot run, and Kimi reads that as an allow.
		if k, ok := h.Target(hooks.TargetKimi); ok && k.Script != "" {
			if _, err := os.Stat(root(filepath.FromSlash(k.Script))); err != nil {
				t.Errorf("hooks/registry.json: %q names kimi.script = %q, which does not exist.\n"+
					"  Fix: correct the path. Kimi treats a hook it cannot start as an allow.",
					h.Name, k.Script)
			}
		}
	}
}

func mustSpec(t *testing.T, h hooks.Hook, target string) hooks.TargetSpec {
	t.Helper()
	spec, ok := h.Target(target)
	if !ok {
		t.Errorf("hooks/registry.json: %q has no %s block.\n"+
			"  Fix: add one. Every hook maps to both Claude Code and opencode.", h.Name, target)
	}
	return spec
}

// TestHooks_OnDiskAreRegistered checks the reverse direction, which is the one
// docs/guides/workflows.md warns about: "the opencode and kimi mappings in step
// 3 are the ones people miss... Without the opencode mapping, opencode silently
// lacks the hook." A file written and never registered does nothing, everywhere,
// and nothing said so.
func TestHooks_OnDiskAreRegistered(t *testing.T) {
	reg := loadRegistry(t)

	registered := map[string]bool{}
	for _, h := range reg {
		for _, target := range []string{hooks.TargetClaudeCode, hooks.TargetOpencode} {
			spec, _ := h.Target(target)
			for _, p := range []string{spec.Script, spec.Module} {
				if p != "" {
					registered[path.Base(p)] = true
				}
			}
		}
	}

	for _, dir := range []struct{ path, ext string }{
		{filepath.Join("hooks", "claude-code"), ".sh"},
		{filepath.Join("hooks", "opencode"), ".js"},
	} {
		entries, err := os.ReadDir(root(dir.path))
		if err != nil {
			t.Fatalf("cannot read %s: %v", dir.path, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, dir.ext) || strings.Contains(name, ".test.") {
				continue
			}
			if registered[name] || nonHookFiles[name] != "" {
				continue
			}
			t.Errorf("%s exists on disk but no hooks/registry.json entry names it.\n"+
				"  Fix: add a registry entry with both a claude_code and an opencode block —\n"+
				"       a hook that is never registered runs nowhere, and nothing else reports that.\n"+
				"       If this file is not a hook, add it to nonHookFiles in this test with what it is.",
				filepath.Join(dir.path, name))
		}
	}
}

// TestHooks_KimiBlockIsDeliberate asserts every hook says what it does under
// Kimi. A hook with no kimi block is simply absent there, and absent-by-accident
// is the failure; a block with enabled:false is a decision, and the decision has
// to carry its reason so the next person does not have to re-derive it.
func TestHooks_KimiBlockIsDeliberate(t *testing.T) {
	for _, h := range loadRegistry(t) {
		k, ok := h.Target(hooks.TargetKimi)
		if !ok {
			t.Errorf("hooks/registry.json: %q has no kimi block.\n"+
				"  Fix: add one. Enabled, or {\"enabled\": false, \"reason\": \"...\"} saying why not —\n"+
				"       a hook with no block is absent under Kimi, which is indistinguishable from an oversight.",
				h.Name)
			continue
		}
		if k.Enabled != nil && !*k.Enabled && strings.TrimSpace(k.Reason) == "" {
			t.Errorf("hooks/registry.json: %q has kimi.enabled = false with no reason.\n"+
				"  Fix: add \"reason\" saying what about Kimi makes the hook unsafe or useless there\n"+
				"       (e.g. \"Kimi runs an ask as an allow\"). Without it the decision reads as a mistake.",
				h.Name)
		}
	}
}

// TestHooks_AreExercisedByATest asserts each hook is named by at least one test
// file. Note the shape: the repo groups hook tests by *behaviour*, not one file
// per hook — on-save-path.test.sh covers all three on-save hooks, and
// fail-closed.test.sh covers the guards collectively. So this looks for a
// reference to the hook rather than for a same-named test file, which would
// demand thirteen new files and contradict the convention the repo already has.
func TestHooks_AreExercisedByATest(t *testing.T) {
	testBlobs := map[string]string{} // suite dir -> concatenated test sources
	for _, dir := range []string{filepath.Join("hooks", "claude-code"), filepath.Join("hooks", "opencode")} {
		entries, err := os.ReadDir(root(dir))
		if err != nil {
			t.Fatalf("cannot read %s: %v", dir, err)
		}
		var sb strings.Builder
		for _, e := range entries {
			if strings.Contains(e.Name(), ".test.") {
				sb.WriteString(readFile(t, dir, e.Name()))
			}
		}
		testBlobs[dir] = sb.String()
	}

	for _, h := range loadRegistry(t) {
		for _, c := range []struct{ target, dir, p string }{
			{hooks.TargetClaudeCode, filepath.Join("hooks", "claude-code"), mustSpec(t, h, hooks.TargetClaudeCode).Script},
			{hooks.TargetOpencode, filepath.Join("hooks", "opencode"), mustSpec(t, h, hooks.TargetOpencode).Module},
		} {
			if c.p == "" {
				continue
			}
			base := path.Base(c.p)
			stem := strings.TrimSuffix(strings.TrimSuffix(base, ".sh"), ".js")
			if strings.Contains(testBlobs[c.dir], base) || strings.Contains(testBlobs[c.dir], stem) {
				continue
			}
			t.Errorf("%s (hook %q, target %s) is named by no test in %s.\n"+
				"  Fix: cover it in an existing suite — tests here group by behaviour, so it may belong in\n"+
				"       on-save-path or fail-closed rather than in a file of its own.",
				base, h.Name, c.target, c.dir)
		}
	}
}

// ─── Catalogs: docs/reference against the asset directories ──────────────────

var agentRowRe = regexp.MustCompile(`(?m)^\| ` + "`" + `([a-z0-9-]+\.md)` + "`" + ` \|`)

// TestAgentCatalog_MatchesDisk asserts docs/reference/agents.md lists every
// agent exactly once, and lists nothing that does not exist.
//
// agents/opencode/orchestrator.md is excluded structurally rather than by name:
// it is catalogued as `opencode/orchestrator.md`, with a slash, so the row
// pattern above does not match it and the directory walk below does not descend
// into agents/opencode/. That is what keeps it out of the "34" without a
// special case anyone has to remember.
func TestAgentCatalog_MatchesDisk(t *testing.T) {
	onDisk := map[string]bool{}
	entries, err := os.ReadDir(root("agents"))
	if err != nil {
		t.Fatalf("cannot read agents/: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" {
			continue
		}
		onDisk[e.Name()] = true
	}

	seen := map[string]int{}
	for _, m := range agentRowRe.FindAllStringSubmatch(readFile(t, "docs", "reference", "agents.md"), -1) {
		seen[m[1]]++
	}

	for name := range onDisk {
		switch seen[name] {
		case 1:
		case 0:
			t.Errorf("agents/%s has no row in docs/reference/agents.md.\n"+
				"  Fix: add a catalog row — | `%s` | <name> | <purpose> | <example trigger> |", name, name)
		default:
			t.Errorf("docs/reference/agents.md lists `%s` %d times.\n"+
				"  Fix: remove the duplicate rows.", name, seen[name])
		}
	}
	for name := range seen {
		if !onDisk[name] {
			t.Errorf("docs/reference/agents.md lists `%s`, which is not in agents/.\n"+
				"  Fix: delete the row, or restore the file if the agent was removed by mistake.", name)
		}
	}
}

var skillRowRe = regexp.MustCompile(`(?m)^\| ` + "`" + `/([a-z0-9-]+)[^` + "`" + `]*` + "`" + ` \|`)

// TestSkillCatalog_MatchesDisk asserts docs/reference/skills.md lists every
// skill directory exactly once.
func TestSkillCatalog_MatchesDisk(t *testing.T) {
	onDisk := map[string]bool{}
	entries, err := os.ReadDir(root("skills"))
	if err != nil {
		t.Fatalf("cannot read skills/: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(root("skills", e.Name(), "SKILL.md")); err == nil {
			onDisk[e.Name()] = true
		}
	}

	seen := map[string]int{}
	for _, m := range skillRowRe.FindAllStringSubmatch(readFile(t, "docs", "reference", "skills.md"), -1) {
		seen[m[1]]++
	}

	for name := range onDisk {
		if seen[name] == 0 {
			t.Errorf("skills/%s/SKILL.md has no row in docs/reference/skills.md.\n"+
				"  Fix: add a catalog row — | `/%s` | <one-line purpose> |", name, name)
		}
	}
	for name := range seen {
		if !onDisk[name] {
			t.Errorf("docs/reference/skills.md lists `/%s`, which has no skills/%s/SKILL.md.\n"+
				"  Fix: delete the row, or restore the skill directory.", name, name)
		}
	}
}
