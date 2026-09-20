package repocheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// assetCounts are the three numbers the repo repeats in prose, derived from
// disk. CLAUDE.md's own gotcha list names the failure this replaces: "Adding or
// removing an agent, skill or hook leaves counts stale in CLAUDE.md, README.md,
// docs/README.md and more."
type assetCounts struct{ agents, skills, hooks int }

func countAssets(t *testing.T) assetCounts {
	t.Helper()
	c := assetCounts{}

	entries, err := os.ReadDir(root("agents"))
	if err != nil {
		t.Fatalf("cannot read agents/: %v", err)
	}
	for _, e := range entries {
		// agents/opencode/ is deliberately not counted: the orchestrator is
		// opencode-exclusive and the prose says so explicitly ("34 agents...
		// plus an opencode-exclusive swarm orchestrator"). Not descending is
		// what encodes that, rather than a name to remember.
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && e.Name() != "README.md" {
			c.agents++
		}
	}

	skillEntries, err := os.ReadDir(root("skills"))
	if err != nil {
		t.Fatalf("cannot read skills/: %v", err)
	}
	for _, e := range skillEntries {
		if e.IsDir() {
			if _, err := os.Stat(root("skills", e.Name(), "SKILL.md")); err == nil {
				c.skills++
			}
		}
	}

	c.hooks = len(loadRegistry(t))
	return c
}

// spelled renders the small integers the prose writes as words.
var spelled = map[int]string{
	6: "six", 7: "seven", 8: "eight", 9: "nine", 10: "ten", 11: "eleven", 12: "twelve",
}

// countClaim is one place the prose states a count. Each is pinned explicitly
// rather than found by a sweep, so the failure can name the sentence to edit.
type countClaim struct {
	file string
	// want builds the exact substring the file must contain, from the counts
	// derived on disk.
	want func(assetCounts) string
	// why says what the sentence is, so a failure reads like the checklist
	// entry it replaces.
	why string
}

var countClaims = []countClaim{
	{"CLAUDE.md", func(c assetCounts) string { return fmt.Sprintf("(%d agents)", c.agents) }, "the Components line"},
	{"CLAUDE.md", func(c assetCounts) string { return fmt.Sprintf("(%d user commands)", c.skills) }, "the Components line"},
	{"CLAUDE.md", func(c assetCounts) string { return fmt.Sprintf("(%d hooks", c.hooks) }, "the Components line"},
	{"README.md", func(c assetCounts) string { return fmt.Sprintf("%d agents cover the full SDLC", c.agents) }, "the Agents section intro"},
	{"README.md", func(c assetCounts) string { return fmt.Sprintf("%d hooks ship in", c.hooks) }, "the Hooks section intro"},
	{"README.md", func(c assetCounts) string { return fmt.Sprintf("# %d agent markdown files", c.agents) }, "the repo-structure tree comment"},
	{filepath.Join("agents", "README.md"), func(c assetCounts) string { return fmt.Sprintf("%d agents covering", c.agents) }, "the Agent Catalog intro"},
	{filepath.Join("hooks", "README.md"), func(c assetCounts) string { return fmt.Sprintf("%d hooks", c.hooks) }, "the hooks overview"},
	{filepath.Join("docs", "README.md"), func(c assetCounts) string { return fmt.Sprintf("%d agents", c.agents) }, "the toolkit reference intro"},
	{filepath.Join("docs", "coverage.md"), func(c assetCounts) string { return fmt.Sprintf("Agents: %d", c.agents) }, "the totals line"},
	{filepath.Join("docs", "coverage.md"), func(c assetCounts) string { return fmt.Sprintf("Skills: %d", c.skills) }, "the totals line"},
	{filepath.Join("docs", "coverage.md"), func(c assetCounts) string {
		return fmt.Sprintf("Total components: %d", c.agents+c.skills)
	}, "the totals line"},
	{filepath.Join("skills", "README.md"), func(c assetCounts) string {
		return fmt.Sprintf("These %s commands", spelled[c.skills])
	}, "the opening sentence, which spells the number out"},
}

// TestAssetCounts_ProseMatchesDisk fails when a count in prose no longer matches
// what is on disk. Adding an agent without updating the counts fails CI here.
func TestAssetCounts_ProseMatchesDisk(t *testing.T) {
	c := countAssets(t)
	if spelled[c.skills] == "" {
		t.Fatalf("skills count is %d, which this test cannot spell.\n"+
			"  Fix: add %d to the `spelled` map in this file, then update skills/README.md to match.", c.skills, c.skills)
	}

	for _, claim := range countClaims {
		body := readFile(t, strings.Split(claim.file, string(filepath.Separator))...)
		want := claim.want(c)
		if strings.Contains(body, want) {
			continue
		}
		t.Errorf("%s no longer says %q (%s).\n"+
			"  On disk right now: %d agents, %d skills, %d hooks.\n"+
			"  Fix: open %s and update that sentence. The number moved; the prose did not.",
			claim.file, want, claim.why, c.agents, c.skills, c.hooks, claim.file)
	}
}

// countedFiles is every file TestAssetCounts_ProseMatchesDisk pins, used below
// to decide where an unregistered claim is worth reporting.
func countedFiles() map[string]bool {
	m := map[string]bool{}
	for _, c := range countClaims {
		m[c.file] = true
	}
	return m
}

// claimRe finds "<n> agents", "<n> skills", "<n> hooks" and the like anywhere in
// the documentation set.
var claimRe = regexp.MustCompile(`(?i)\b(\d+)\s+(agents?|skills?|hooks?|user commands?)\b`)

// approximate marks a claim as a deliberate round number rather than an exact
// count — "~30 specialist capabilities" is prose, not an assertion.
var approximate = regexp.MustCompile(`[~≈]\s*\d+\s*$`)

// TestAssetCounts_NoUnregisteredClaims sweeps the documentation for a count this
// test does not know about. Without it, the table above only protects the
// sentences that existed when it was written, and the next person to state a
// count in a new file reintroduces exactly the drift this package exists to
// stop.
func TestAssetCounts_NoUnregisteredClaims(t *testing.T) {
	c := countAssets(t)
	known := countedFiles()
	exact := map[string]int{
		"agent": c.agents, "agents": c.agents,
		"skill": c.skills, "skills": c.skills,
		"user command": c.skills, "user commands": c.skills,
		"hook": c.hooks, "hooks": c.hooks,
	}

	// The sweep is scoped to the index and catalog documents — the places a
	// count is a *claim about the toolkit*. Three kinds of file are excluded
	// deliberately, and each exclusion is load-bearing:
	//
	//   CHANGELOG.md — a historical record. "14 agents" in a v0.4.0 entry was
	//   true when it was written and must never be "corrected"; rewriting
	//   history to match today is the opposite of what a changelog is for.
	//
	//   agents/*.md, skills/*/SKILL.md — prose inside an asset's own body.
	//   agents/synthesis.md says "at least 2 agent reports", which is an
	//   instruction to the model, not a count of this repo.
	//
	//   docs/guides/, docs/development/ — narrative that cites numbers about
	//   other things entirely ("13 workflow presets", "3 script suites").
	docs := []string{
		"CLAUDE.md",
		"README.md",
		filepath.Join("agents", "README.md"),
		filepath.Join("skills", "README.md"),
		filepath.Join("hooks", "README.md"),
		filepath.Join("docs", "README.md"),
		filepath.Join("docs", "coverage.md"),
		filepath.Join("docs", "reference", "agents.md"),
		filepath.Join("docs", "reference", "skills.md"),
		filepath.Join("docs", "reference", "hooks.md"),
	}
	sort.Strings(docs)

	for _, rel := range docs {
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		b, err := os.ReadFile(root(rel))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			for _, m := range claimRe.FindAllStringSubmatchIndex(line, -1) {
				full := line[m[0]:m[1]]
				num := line[m[2]:m[3]]
				noun := strings.ToLower(line[m[4]:m[5]])
				if approximate.MatchString(line[:m[1]-len(noun)]) {
					continue
				}
				want, tracked := exact[noun]
				if !tracked {
					continue
				}
				if fmt.Sprint(want) == num {
					// Correct today. Only flag it if nothing pins it, because a
					// correct-by-luck number is what this package removes.
					if !known[rel] {
						t.Errorf("%s states %q, and no assertion pins it.\n"+
							"  Fix: add a countClaim for %s to counts_test.go, so the next person who adds an\n"+
							"       asset is told about this sentence instead of leaving it stale.", rel, full, rel)
					}
					continue
				}
				t.Errorf("%s states %q, but disk has %d.\n"+
					"  Fix: correct that sentence.", rel, full, want)
			}
		}
	}
}
