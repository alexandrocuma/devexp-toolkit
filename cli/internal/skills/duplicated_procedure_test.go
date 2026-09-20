package skills

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Some procedures are written out in more than one skill on purpose. /deliver
// and /improve both advertise that they need no other skill installed, so a
// step they share cannot be replaced with "see the other skill" — the copy has
// to stay. What cannot stay is the copies drifting apart unnoticed, which is
// how the worktree access grant came to be wrong in two independent ways at
// once: one copy wrote a key Claude Code does not read, and the other had lost
// the words "the main checkout's", so a grant written from inside a worktree
// was discarded with that tree.
//
// Prose has no compiler, so these tests are it. Two shapes, chosen to match
// what is actually being duplicated:
//
//   - executable text, asserted byte-for-byte. A shell function that guards a
//     delete has one correct spelling and no reason to be phrased differently
//     in two files.
//   - a procedure whose renderings differ on purpose, asserted by its
//     load-bearing CLAIMS. The grant is written out at three different lengths
//     for three different readers; demanding byte-identity there would be
//     unsatisfiable, and demanding nothing would be what let it drift.
//
// Running these locally: pass -count=1. Both tests read assets from outside
// this package's directory, and the test cache does not reliably notice when
// one of those files changes — so an edit to a SKILL.md can be answered with a
// cached PASS from before it. CI is unaffected, because a fresh checkout has an
// empty cache, but a local run that "passes" after a deliberate edit has told
// you nothing. That is this repo's own favourite defect, aimed at the tests
// meant to catch it.

// ─── Executable text: byte-identical wherever it appears ─────────────────────

// safeIDRe captures the shell guard that validates an identifier before it is
// interpolated into a delete pattern. It appears in /cleanup and /improve, both
// of which remove files from the user's home directory.
var safeIDRe = regexp.MustCompile(`(?m)^safe_id\(\)\s*\{.*$`)

// TestSharedSafeIDGuardIsIdentical asserts the guard is spelled identically
// everywhere it appears.
//
// This one is byte-exact rather than semantic, and the stakes are why. The
// function decides whether a caller-supplied ticket id may be pasted into an
// `rm -f` glob. A copy that drifted — one extra character class, one missing
// empty-string case — would keep passing every other test in the repo while
// accepting an identifier its twin rejects, in a code path whose entire job is
// deleting things.
func TestSharedSafeIDGuardIsIdentical(t *testing.T) {
	found := map[string]string{} // asset name -> the line
	for name, path := range repoSkillFiles(t) {
		for _, line := range safeIDRe.FindAllString(readAsset(t, path), -1) {
			trimmed := strings.TrimRight(line, " \t")
			if prev, ok := found[name]; ok && prev != trimmed {
				t.Errorf("%s defines safe_id() twice, differently:\n  %s\n  %s", name, prev, trimmed)
			}
			found[name] = trimmed
		}
	}

	if len(found) < 2 {
		// Not a pass. If the guard stopped appearing in two skills this test
		// would quietly assert nothing at all, which is the failure mode it
		// exists to prevent in the first place.
		t.Fatalf("safe_id() was found in %d skill(s); this guard is meaningless below two.\n"+
			"  If the procedure genuinely moved to one place, delete this test and say so.\n"+
			"  If it was renamed, update safeIDRe. Found: %v", len(found), keysOf(found))
	}

	names := keysOf(found)
	first := names[0]
	for _, n := range names[1:] {
		if found[n] != found[first] {
			t.Errorf("safe_id() has drifted between two skills that both use it to guard a delete.\n"+
				"  %s:\n    %s\n  %s:\n    %s\n"+
				"  Fix: make them identical. This function decides whether a ticket id may be\n"+
				"  interpolated into an rm pattern; a copy that accepts what the other rejects is a\n"+
				"  difference in what each skill is willing to delete.",
				first, found[first], n, found[n])
		}
	}
}

// ─── A shared procedure: asserted by its claims ──────────────────────────────

// grantClaim is one thing every description of the worktree access grant has to
// say. Each exists because its absence has already caused, or would cause, a
// specific failure — recorded here so a future edit that drops one has to argue
// with the reason rather than with a string match.
type grantClaim struct {
	// any of these substrings satisfies the claim, so the prose stays free to
	// vary in register and length.
	accept []string
	what   string
	why    string
}

var grantClaims = []grantClaim{
	{
		accept: []string{"permissions.additionalDirectories"},
		what:   "the key is nested under permissions",
		why: "a top-level additionalDirectories is not in Claude Code's settings schema, so it is " +
			"skipped: the step writes a file, exits 0, and grants nothing",
	},
	{
		accept: []string{"settings.local.json"},
		what:   "the file is settings.local.json, not settings.json",
		why: "the value is an absolute path on one machine; settings.json is the shared, committable " +
			"project file, and a home directory written there is meaningless to everyone else and can " +
			"be committed by a later git add -A",
	},
	{
		accept: []string{"main checkout"},
		what:   "the file written is the MAIN CHECKOUT's, not the worktree's",
		why: "project settings live in the primary tree, so a grant written inside a worktree is " +
			"thrown away when that tree is removed and the next stream starts unprivileged",
	},
	{
		accept: []string{"git worktree list"},
		what:   "both paths are derived from git worktree list",
		why: "the step may run from inside the worktree, where a relative ../<repo>-worktrees resolves " +
			"to nothing and $PWD is the wrong tree",
	},
}

// TestGrantProcedureStatesEveryLoadBearingClaim asserts that every asset
// describing the worktree access grant states all of them.
//
// Byte-identity is deliberately NOT asserted here, and that is a finding rather
// than a concession: there are no byte-identical copies to compare. /deliver
// carries the executable snippet, /improve carries a compressed paragraph that
// points at it, and the guide carries a third rendering for a human reading
// about the convention. They are different on purpose. What must not differ is
// what they each promise, so that is what is checked.
func TestGrantProcedureStatesEveryLoadBearingClaim(t *testing.T) {
	describing := 0
	for name, path := range grantAssetFiles(t) {
		content := readAsset(t, path)
		if !strings.Contains(content, "additionalDirectories") {
			continue // this asset does not describe the grant
		}
		describing++
		t.Run(name, func(t *testing.T) {
			for _, c := range grantClaims {
				if anyContains(content, c.accept) {
					continue
				}
				t.Errorf("%s describes the worktree access grant but never says %s.\n"+
					"  Why it matters: %s.\n"+
					"  Fix: say it, in whatever words suit this file — the wording is free, the claim is not.\n"+
					"  Expected one of: %s",
					name, c.what, c.why, strings.Join(quoteAll(c.accept), ", "))
			}
		})
	}

	if describing < 2 {
		t.Fatalf("only %d asset describes the grant, so nothing here is guarding against drift between copies.\n"+
			"  If the procedure was consolidated into one place, delete this test and say so in the commit.", describing)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func readAsset(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	return string(b)
}

func anyContains(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
