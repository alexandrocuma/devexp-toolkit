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

// ─── Tech specs: one status vocabulary across the skills ────────────────────

// specStatusListRe finds the one sentence that defines a tech spec's status
// values. /refine owns the vocabulary; every other skill only moves a spec
// along it, so this is the source the others are checked against.
var specStatusListRe = regexp.MustCompile("(?m)^Status values, in order: (.+)$")

// specStatusUseRe finds a status a skill writes or tests for — `Status: x`,
// the form every consumer uses when it names a spec state.
var specStatusUseRe = regexp.MustCompile("Status: ([a-z][a-z-]*)")

// TestSpecStatus_ValuesAgree fails when a skill names a spec status /refine
// never defined, or stops naming one it is responsible for setting. The
// statuses are plain prose in five files, read by agents that do exactly what
// the words say: a /deliver that set "building" while /release waited for
// "in-build" would leave every spec unretired, and nothing else would notice.
func TestSpecStatus_ValuesAgree(t *testing.T) {
	m := specStatusListRe.FindStringSubmatch(readFile(t, "skills", "refine", "SKILL.md"))
	if m == nil {
		t.Fatal("skills/refine/SKILL.md no longer has its `Status values, in order:` line.\n" +
			"  Fix: restore it — it is the single definition the other skills are checked against.")
	}
	defined := map[string]bool{}
	for _, v := range regexp.MustCompile("`([a-z][a-z-]*)`").FindAllStringSubmatch(m[1], -1) {
		defined[v[1]] = true
	}
	if len(defined) != 4 {
		t.Fatalf("skills/refine/SKILL.md defines %d spec statuses (%v), want the four of draft → approved → in-build → implemented.\n"+
			"  Fix: if the lifecycle really changed, update this test and every skill below in the same commit.", len(defined), defined)
	}

	// mustSet is the status each file is responsible for writing or stopping
	// on; losing that sentence is as broken as misspelling it.
	consumers := map[string]struct {
		path    []string
		mustSet []string
	}{
		"deliver":        {path: []string{"skills", "deliver", "SKILL.md"}, mustSet: []string{"draft", "in-build"}},
		"release":        {path: []string{"skills", "release", "SKILL.md"}, mustSet: []string{"implemented"}},
		"grooming-agent": {path: []string{"agents", "grooming-agent.md"}, mustSet: []string{"draft"}},
		"cleanup":        {path: []string{"skills", "cleanup", "SKILL.md"}, mustSet: nil},
		"update-docs":    {path: []string{"agents", "update-docs.md"}, mustSet: nil},
	}
	for name, c := range consumers {
		t.Run(name, func(t *testing.T) {
			rel := filepath.Join(c.path...)
			body := readFile(t, c.path...)
			used := map[string]bool{}
			for _, u := range specStatusUseRe.FindAllStringSubmatch(body, -1) {
				used[u[1]] = true
				if !defined[u[1]] {
					t.Errorf("%s names spec status %q, which skills/refine/SKILL.md does not define.\n"+
						"  Fix: use one of draft, approved, in-build, implemented.", rel, u[1])
				}
			}
			for _, want := range c.mustSet {
				if !used[want] {
					t.Errorf("%s no longer says `Status: %s`, the spec status it is responsible for.\n"+
						"  Fix: restore the sentence, or move the responsibility and this test together.", rel, want)
				}
			}
		})
	}
}

// refineParallelGroomPrompt returns the quoted subagent prompt in /refine's
// "5b" section: the only instruction a parallel groom receives. Text outside
// the quote is read by the orchestrator, not by the groom, so it does not count.
func refineParallelGroomPrompt(t *testing.T) string {
	t.Helper()
	body := readFile(t, "skills", "refine", "SKILL.md")
	start := strings.Index(body, "\n#### 5b.")
	if start < 0 {
		t.Fatal("skills/refine/SKILL.md has no `#### 5b.` section (parallel grooming of an epic).\n" +
			"  Fix: restore it, or move this test with it.")
	}
	section := body[start+1:]
	if end := regexp.MustCompile(`\n#{2,4} `).FindStringIndex(section); end != nil {
		section = section[:end[0]]
	}
	var quoted []string
	for _, line := range strings.Split(section, "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, ">") {
			quoted = append(quoted, l)
		}
	}
	if len(quoted) == 0 {
		t.Fatal("the `5b.` section of skills/refine/SKILL.md has no quoted subagent prompt.\n" +
			"  Fix: restore the `> \"Read ~/.claude/agents/grooming-agent.md …\"` prompt.")
	}
	return strings.Join(quoted, "\n")
}

// TestRefineParallelGroomDefersEverySharedWrite fails when the grooming agent
// writes something every groom of a repo shares and /refine's parallel prompt
// does not tell the groom to leave it alone. Several grooms writing the same
// memory index, project memory file, graph or atlas at once corrupt it, and
// nothing reports it: each groom still returns a good plan.
func TestRefineParallelGroomDefersEverySharedWrite(t *testing.T) {
	agent := readFile(t, "agents", "grooming-agent.md")
	prompt := refineParallelGroomPrompt(t)

	// agentSays is how agents/grooming-agent.md asks for the shared write;
	// promptSays is how the 5b prompt tells a parallel groom not to do it.
	sharedWrites := map[string]struct {
		agentSays  string
		promptSays string
	}{
		"memory index":        {agentSays: "Update `MEMORY.md` index", promptSays: "do not update `MEMORY.md`"},
		"project memory file": {agentSays: "`<PROJECT-NAME>.md`", promptSays: "do not write `<PROJECT-NAME>.md`"},
		"graph rebuild":       {agentSays: "/graphify --update", promptSays: "do not run `/graphify --update`"},
		"atlas rebuild":       {agentSays: "for a full rebuild", promptSays: "do not rebuild it"},
	}
	for name, w := range sharedWrites {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(agent, w.agentSays) {
				t.Errorf("agents/grooming-agent.md no longer says %q, so this test no longer knows how it asks for the %s.\n"+
					"  Fix: point agentSays at the new wording, and check the 5b prompt in skills/refine/SKILL.md still defers it.", w.agentSays, name)
			}
			if !strings.Contains(strings.ToLower(prompt), strings.ToLower(w.promptSays)) {
				t.Errorf("the 5b prompt in skills/refine/SKILL.md does not say %q: parallel grooms would each do the %s at once.\n"+
					"  Fix: tell the groom subagent not to, and do it once in the orchestrator after the grooms return.", w.promptSays, name)
			}
		})
	}
}

// TestRefineNeverStartsCustomAgentByName fails when /refine starts a subagent
// with a custom agent's name. That name is not a valid subagent type, so the
// call fails or silently runs a different agent; custom agents are run by
// reading their file.
func TestRefineNeverStartsCustomAgentByName(t *testing.T) {
	custom := map[string]bool{}
	for _, dir := range []string{"agents", filepath.Join("agents", "opencode")} {
		files, err := filepath.Glob(root(dir, "*.md"))
		if err != nil {
			t.Fatalf("cannot list %s: %v", dir, err)
		}
		for _, f := range files {
			if name := strings.TrimSuffix(filepath.Base(f), ".md"); name != "README" {
				custom[name] = true
			}
		}
	}
	if !custom["grooming-agent"] {
		t.Fatal("agents/grooming-agent.md was not found, so the custom agent list is wrong.")
	}
	body := readFile(t, "skills", "refine", "SKILL.md")
	for _, m := range regexp.MustCompile(`subagent_type\W{0,4}([A-Za-z0-9_-]+)`).FindAllStringSubmatch(body, -1) {
		if custom[m[1]] {
			t.Errorf("skills/refine/SKILL.md starts the custom agent %q as a subagent type.\n"+
				"  Fix: start a general-purpose subagent and tell it to read ~/.claude/agents/%s.md and follow it.", m[1], m[1])
		}
	}
}

// ─── Plan template: the flow grooming writes for the plan deck ─────────────

// phaseRegion returns the part of body from the line starting with from up to
// the next line starting with to. A whole-file search would let a mention in
// another phase satisfy the check, so each contract is read only where it is
// written.
func phaseRegion(t *testing.T, rel, body, from, to string) string {
	t.Helper()
	start := strings.Index(body, "\n"+from)
	if start < 0 {
		t.Fatalf("%s has no line starting %q.\n"+
			"  Fix: restore the heading, or move this test's anchor with it.", rel, from)
	}
	region := body[start+1:]
	if end := strings.Index(region, "\n"+to); end >= 0 {
		region = region[:end]
	}
	return region
}

// groomingPhase7 is the plan template: the only text that tells
// grooming-agent which sections a plan has and how they are spelled.
func groomingPhase7(t *testing.T) string {
	t.Helper()
	return phaseRegion(t, "agents/grooming-agent.md", readFile(t, "agents", "grooming-agent.md"),
		"### Phase 7", "### Phase 8")
}

// planTemplateHeadings returns the headings on Phase 7's one fixed list —
// the line that tells grooming-agent which level-2 headings a plan uses. A
// heading named only in a later bullet is a description of a section, not a
// promise to write it under that name.
func planTemplateHeadings(t *testing.T) map[string]bool {
	t.Helper()
	const lead = "- Use exactly these level-2 headings"
	for _, line := range strings.Split(groomingPhase7(t), "\n") {
		if strings.HasPrefix(line, lead) {
			got := map[string]bool{}
			for _, m := range planHeadingRe.FindAllStringSubmatch(line, -1) {
				got[m[1]] = true
			}
			return got
		}
	}
	t.Fatalf("Phase 7 of agents/grooming-agent.md has no line starting %q.\n"+
		"  Fix: restore the fixed list of plan headings — the plan deck finds each section by it.", lead)
	return nil
}

// planTemplateWant is the fixed heading list the plan deck depends on.
var planTemplateWant = []string{
	"## Files that change", "## Files confirmed NOT changing", "## Execution Steps",
	"## Risks", "## Affected Release Targets", "## Decisions", "## Diagrams",
}

// TestGroomingAgent_DiagramsFormat fails when Phase 7 stops describing the
// before → after flow in the form the plan deck parses. The deck may add
// nothing the plan lacks, so a plan written without the flow, or in another
// form, gets a deck with no flow diagram and no error anywhere.
func TestGroomingAgent_DiagramsFormat(t *testing.T) {
	phase7 := groomingPhase7(t)
	format := map[string]struct{ mustSay string }{
		"section heading":   {mustSay: "`## Diagrams`"},
		"before subsection": {mustSay: "`### Flow: before`"},
		"after subsection":  {mustSay: "`### Flow: after`"},
		"fenced text block": {mustSay: "```text"},
		"edge form":         {mustSay: "`a -> b : label`"},
		"one spelling":      {mustSay: "exactly one spelling"},
		"no-flow escape":    {mustSay: "`Not applicable — <why>`"},
		"never empty":       {mustSay: "Never leave the section empty"},
	}
	for name, f := range format {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(phase7, f.mustSay) {
				t.Errorf("Phase 7 of agents/grooming-agent.md no longer says %s.\n"+
					"  Fix: restore it — /refine Phase 6 reads plans in exactly this form, and a plan written otherwise loses its diagram silently.", f.mustSay)
			}
		})
	}
	fixed := planTemplateHeadings(t)
	for _, h := range planTemplateWant {
		if !fixed[h] {
			t.Errorf("Phase 7's fixed heading list in agents/grooming-agent.md does not name `%s`.\n"+
				"  Fix: add it back to the `Use exactly these level-2 headings` line.", h)
		}
	}
}

// releaseTargetsAddrRe extracts the start address of the sed range /release
// uses to cut the `## Affected Release Targets` section out of a plan.
var releaseTargetsAddrRe = regexp.MustCompile(`sed -n -E '/([^/]*Affected Release Targets)/,`)

// TestRelease_ReadsTargetsByHeading fails when /release stops reading the
// targets section by its heading. Plans mention the section by name in prose
// too (health report, risks, validation notes), and a range that starts on a
// mention ships whatever targets that prose happens to list.
func TestRelease_ReadsTargetsByHeading(t *testing.T) {
	if !strings.Contains(groomingPhase7(t), "`## Affected Release Targets`") {
		t.Fatal("Phase 7 of agents/grooming-agent.md no longer names `## Affected Release Targets`, the heading /release reads.\n" +
			"  Fix: restore the heading, or change /release's sed address in the same commit.")
	}
	m := releaseTargetsAddrRe.FindStringSubmatch(readFile(t, "skills", "release", "SKILL.md"))
	if m == nil {
		t.Fatal("skills/release/SKILL.md has no `sed -n -E '/…Affected Release Targets/,/^## /p'` line.\n" +
			"  Fix: restore it — it is how /release binds a plan's targets.")
	}
	addr, err := regexp.Compile(m[1])
	if err != nil {
		t.Fatalf("/release's sed address %q is not a valid extended regexp: %v", m[1], err)
	}
	lines := map[string]struct {
		line  string
		match bool
	}{
		"fixed heading":      {line: "## Affected Release Targets", match: true},
		"older h3 heading":   {line: "### Affected Release Targets", match: true},
		"older bold heading": {line: "**Affected Release Targets**: cli", match: true},
		"prose mention":      {line: "- Release Impact feeds `## Affected Release Targets`", match: false},
		"inline mention":     {line: "take the plan's Affected Release Targets section", match: false},
	}
	for name, l := range lines {
		t.Run(name, func(t *testing.T) {
			if got := addr.MatchString(l.line); got != l.match {
				t.Errorf("/release's sed address %q matches %q = %v, want %v.\n"+
					"  Fix: anchor the address on the heading (`^(#{2,3} |\\*\\*)Affected Release Targets`), never on any mention.", m[1], l.line, got, l.match)
			}
		})
	}
}

// ─── Plan deck: the diagram rules reach every deck subagent ────────────────

// refineDeckRules returns the Phase 6 paragraphs an epic's deck subagents
// receive word for word: from **Input** up to **Publish where the host can.**,
// which subagents never do. A rule placed outside this span reaches a
// single-ticket deck but silently skips every epic deck.
func refineDeckRules(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, "**Input is the persisted plan")
	end := strings.Index(body, "**Publish where the host can.**")
	if start < 0 || end < start {
		t.Fatal("skills/refine/SKILL.md no longer has Phase 6's **Input is the persisted plan** … **Publish where the host can.** span.\n" +
			"  Fix: restore both paragraphs, or move this test's anchors with them.")
	}
	return body[start:end]
}

// TestRefineDeckSVGRulesTravelToSubagents fails when a diagram rule of the
// plan deck sits where a deck subagent never reads it, when the subagent is
// no longer told to copy the diagram catalog, or when a sign-off stops
// counting diagrams. Each of these leaves epic decks without the rule and
// raises no error: the subagent still returns a deck that looks finished.
func TestRefineDeckSVGRulesTravelToSubagents(t *testing.T) {
	body := readFile(t, "skills", "refine", "SKILL.md")
	rules := refineDeckRules(t, body)
	inSpan := map[string]struct{ mustSay string }{
		"diagram catalog":         {mustSay: "**Diagram catalog.**"},
		"no foreignObject":        {mustSay: "foreignObject"},
		"accessible svg":          {mustSay: `role="img"`},
		"svg description":         {mustSay: "<desc>"},
		"no href from plan text":  {mustSay: "xlink:href"},
		"plan text in details":    {mustSay: "<details>"},
		"missing source stated":   {mustSay: "not in the plan"},
		"edges only inside fence": {mustSay: "fenced `text` blocks"},
		"changed edge label":      {mustSay: "with a different label it is **changed**"},
		"top to bottom":           {mustSay: "reads top to bottom"},
		"never left to right":     {mustSay: "No diagram runs left to right"},
		"four slides, no more":    {mustSay: "A deck has at most four slides"},
		"too long gets no deck":   {mustSay: "gets **no deck**"},
		"per-node traceability":   {mustSay: "every arrow label"},
	}
	for name, r := range inSpan {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(rules, r.mustSay) {
				t.Errorf("Phase 6 of skills/refine/SKILL.md does not say %q inside the paragraphs deck subagents copy (**Input** … **The file**).\n"+
					"  Fix: put the rule inside one of those paragraphs, not after **Publish where the host can.**", r.mustSay)
			}
		})
	}

	copyList := regexp.MustCompile(`copies the (\*\*Input\*\*[^\n]*?) paragraphs of this phase word for word`).FindStringSubmatch(body)
	if copyList == nil {
		t.Fatal("skills/refine/SKILL.md no longer tells deck subagents which Phase 6 paragraphs to copy word for word.\n" +
			"  Fix: restore the epic bullet that lists them.")
	}
	var named []string
	for _, m := range regexp.MustCompile(`\*\*([^*]+)\*\*`).FindAllStringSubmatch(copyList[1], -1) {
		named = append(named, m[1])
	}
	// Every paragraph that opens with a bold lead in the span is a rule block;
	// the orchestrator copies only the ones the list names, so an unnamed one
	// never reaches an epic deck.
	for _, m := range regexp.MustCompile(`(?m)^\*\*([^*]+)\*\*`).FindAllStringSubmatch(rules, -1) {
		lead := m[1]
		listed := false
		for _, n := range named {
			if strings.HasPrefix(lead, n) {
				listed = true
			}
		}
		if !listed {
			t.Errorf("Phase 6 paragraph **%s** is not in the deck subagent copy list (%s): epic decks would never receive it.\n"+
				"  Fix: name it in the list, or fold it into a paragraph the list names.", lead, copyList[1])
		}
	}
	if len(named) < 6 {
		t.Errorf("the deck subagent copy list names %d paragraphs (%v), want the six Phase 6 rule blocks.", len(named), named)
	}
	if !strings.Contains(body, "counts of diagrams") {
		t.Error("deck subagents are no longer asked to return a count of diagrams.\n" +
			"  Fix: ask for `counts of diagrams, decisions, steps and open questions`.")
	}
	if n := strings.Count(body, "Diagrams: N"); n < 2 {
		t.Errorf("skills/refine/SKILL.md shows `Diagrams: N` %d time(s); the single-ticket sign-off and the epic list both need it.\n"+
			"  Fix: add `Diagrams: N` beside `Decisions: D` in both.", n)
	}
}

// ─── Plan deck: short labels, written by grooming and drawn by the deck ────

// TestShortLabels_LimitsAgree fails when the label limits grooming writes to
// and the limits the deck draws to drift apart, or drop out of either side.
// The deck copies labels and never shortens them, so a plan written to a looser
// limit than the deck expects fills the diagrams with prose again — and a deck
// rule outside the copied span reaches single-ticket decks only. Neither raises
// an error: the deck still looks finished.
func TestShortLabels_LimitsAgree(t *testing.T) {
	phase7 := groomingPhase7(t)
	deck := refineDeckRules(t, readFile(t, "skills", "refine", "SKILL.md"))
	limits := map[string]struct{ plan, deck string }{
		"box label":       {plan: "at most five words", deck: "at most five words in a box"},
		"arrow label":     {plan: "at most four words", deck: "at most four words on an arrow"},
		"copied verbatim": {plan: "copied as written", deck: "copied as written"},
		"bold lead":       {plan: "**short label**", deck: "**short label**"},
		"no rewording":    {plan: "never shortens", deck: "Never shorten, reword or truncate"},
		"missing label":   {plan: "bare number", deck: "never a rewording"},
		"short captions":  {plan: "", deck: "at most 12 words"},
	}
	for name, l := range limits {
		t.Run(name, func(t *testing.T) {
			if l.plan != "" && !strings.Contains(phase7, l.plan) {
				t.Errorf("Phase 7 of agents/grooming-agent.md no longer says %q: plans would carry labels the deck cannot draw as written.\n"+
					"  Fix: restore the short-label rule in Phase 7, matching Phase 6 of skills/refine/SKILL.md.", l.plan)
			}
			if !strings.Contains(deck, l.deck) {
				t.Errorf("Phase 6 of skills/refine/SKILL.md does not say %q inside the paragraphs deck subagents copy (**Input** … **The file**).\n"+
					"  Fix: restore the label rule there, matching Phase 7 of agents/grooming-agent.md.", l.deck)
			}
		})
	}
}

// ─── Plan deck: the diagram catalog against the plan template ──────────────

// planHeadingRe finds a backticked level-2 plan heading, such as
// `## Execution Steps`. Exactly two `#`: the catalog's prose about older plans
// names `### Steps` and must never count as a source.
var planHeadingRe = regexp.MustCompile("`(## [A-Z][^`]*)`")

// minCatalogSources is how many distinct source headings the catalog must
// name. A table that was deleted or reformatted would otherwise compare zero
// headings and pass.
const minCatalogSources = 7

// TestDeckCatalog_MatchesPlanTemplate fails when /refine's diagram catalog
// names a plan section grooming-agent no longer writes, or when the flow
// section drops out of either side. The deck may only draw what the plan
// holds, so a source heading the template stopped writing is a diagram that
// disappears from every deck with no error.
func TestDeckCatalog_MatchesPlanTemplate(t *testing.T) {
	refine := readFile(t, "skills", "refine", "SKILL.md")
	phase6 := phaseRegion(t, "skills/refine/SKILL.md", refine, "### Phase 6", "### Phase 7")
	at := strings.Index(phase6, "**Diagram catalog.**")
	if at < 0 {
		t.Fatal("Phase 6 of skills/refine/SKILL.md has no `**Diagram catalog.**` line.\n" +
			"  Fix: restore it and its table — it is the list of plan sections the deck draws from.")
	}
	// The catalog is the table under that line: skip to its first row, then
	// stop at the first line that is not a row.
	var rows []string
	started := false
	for _, line := range strings.Split(phase6[at:], "\n")[1:] {
		if strings.HasPrefix(line, "|") {
			started = true
			rows = append(rows, line)
		} else if started {
			break
		}
	}
	catalog := map[string]bool{}
	for _, m := range planHeadingRe.FindAllStringSubmatch(strings.Join(rows, "\n"), -1) {
		catalog[m[1]] = true
	}
	if len(catalog) < minCatalogSources {
		t.Fatalf("the diagram catalog in skills/refine/SKILL.md names %d source headings (%v), want at least %d.\n"+
			"  Fix: write each source as a backticked level-2 heading in the table rows, as `## Execution Steps`.", len(catalog), catalog, minCatalogSources)
	}

	template := planTemplateHeadings(t)
	for name, side := range map[string]struct {
		file     string
		headings map[string]bool
	}{
		"refine catalog":    {file: "the diagram catalog in skills/refine/SKILL.md", headings: catalog},
		"grooming template": {file: "Phase 7 of agents/grooming-agent.md", headings: template},
	} {
		if !side.headings["## Diagrams"] {
			t.Errorf("%s (%s) no longer names `## Diagrams`: decks would lose the before → after flow.\n"+
				"  Fix: restore it on this side.", side.file, name)
		}
	}
	for heading := range catalog {
		t.Run(heading, func(t *testing.T) {
			if !template[heading] {
				t.Errorf("the diagram catalog in skills/refine/SKILL.md draws from `%s`, which Phase 7 of agents/grooming-agent.md does not write.\n"+
					"  Fix: rename the catalog source to the heading Phase 7 writes, or add the heading to Phase 7 — in the same commit.", heading)
			}
		})
	}
}
