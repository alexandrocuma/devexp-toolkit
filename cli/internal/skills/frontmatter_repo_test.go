package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// ── Repo-asset guards: what Kimi Code CLI requires of a shipped skill ─────────
//
// Kimi parses a SKILL.md's front matter as strict YAML and skips the whole
// skill when it doesn't parse, so an unquoted `description:` holding ": " —
// which Claude Code accepts — silently costs a Kimi user the skill. These
// tests read the real `skills/` tree rather than a fixture, because the thing
// being guarded is the shipped asset, not the installer.
//
// splitRepoFrontmatter is deliberately a second implementation of Kimi's fence
// rule rather than a call into the installer: the guard has to hold against
// what Kimi does, so it must not pass merely because the installer and the
// test agree with each other.

// splitRepoFrontmatter applies Kimi's rule: line 0 must trim to "---", and the
// first later line that trims to "---" closes the block.
func splitRepoFrontmatter(content string) (fm, body string, ok bool) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), true
		}
	}
	return "", "", false
}

// repoSkillFiles lists every shipped skill's SKILL.md, keyed by skill name.
func repoSkillFiles(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", root, err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name(), "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		out[e.Name()] = p
	}
	if len(out) == 0 {
		t.Fatalf("no skills found under %s", root)
	}
	return out
}

func TestRepoSkillsFrontmatterStrictYAML(t *testing.T) {
	for name, path := range repoSkillFiles(t) {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", path, err)
			}
			fm, body, ok := splitRepoFrontmatter(string(data))
			if !ok {
				t.Fatalf("%s has no front matter block — Kimi requires one", path)
			}
			var parsed map[string]any
			if err := yaml.Unmarshal([]byte(fm), &parsed); err != nil {
				t.Fatalf("%s front matter is not strict YAML, so Kimi would skip the skill: %v", path, err)
			}
			if got, _ := parsed["name"].(string); got != name {
				t.Errorf("%s name = %q, want %q (the directory name)", path, parsed["name"], name)
			}
			if desc, _ := parsed["description"].(string); strings.TrimSpace(desc) == "" {
				t.Errorf("%s description = %q, want a non-empty string — Kimi requires it", path, parsed["description"])
			}
			if strings.TrimSpace(body) == "" {
				t.Errorf("%s has an empty body", path)
			}
			if typ, present := parsed["type"]; present {
				s, _ := typ.(string)
				if !kimiSkillTypes[s] {
					t.Errorf("%s type = %q, want one of prompt/inline/flow — any other value drops the skill silently", path, typ)
				}
			}
		})
	}
}

// kimiSkillParameterRe matches what Kimi's skill-parameter expander rewrites in
// a SKILL.md body: $ARGUMENTS, $ARGUMENTS[n], and $0-$9 (a bare $ followed by
// digits and not a word character). It substitutes them — with the empty string
// when the skill is invoked without arguments — so a shell snippet using
// positional parameters is silently corrupted for Kimi users. The braced
// ${1} form and awk's $(0) are not matched, and both are exact substitutes.
//
// Agent bodies use a different expander and are not affected; that one is
// guarded in internal/agents.
var kimiSkillParameterRe = regexp.MustCompile(`\$ARGUMENTS|\$[0-9]`)

func TestRepoSkillsNoKimiParameterExpansion(t *testing.T) {
	for name, path := range repoSkillFiles(t) {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", path, err)
			}
			_, body, ok := splitRepoFrontmatter(string(data))
			if !ok {
				body = string(data)
			}
			for i, line := range strings.Split(body, "\n") {
				if m := kimiSkillParameterRe.FindString(line); m != "" {
					t.Errorf("%s body line %d uses %q, which Kimi's skill-parameter expander rewrites: %s\n"+
						"use the braced ${n} form in shell, or $(0) in awk", path, i+1, m, strings.TrimSpace(line))
				}
			}
		})
	}
}

// ── Repo-asset guard: the worktree grant must name a key Claude Code reads ────
//
// Claude Code's settings schema defines `permissions.additionalDirectories`
// and no top-level `additionalDirectories`. An unrecognised top-level key is
// skipped, so a grant step that writes one looks like it worked, grants
// nothing, and leaves every out-of-project write prompting — which is what
// /deliver Phase 1.5 and /improve both shipped that way until it was caught.
//
// The failure is invisible at runtime: nothing errors, the file is written,
// and only a permission prompt much later hints at it. So it is guarded here,
// against the shipped assets, rather than left to review.

// topLevelAdditionalDirsRe matches the key being written at the settings root
// in either of the two forms these assets can carry it: a JS member write
// through any receiver (`x.additionalDirectories = …` / `.push(`, dot or
// bracket), and a JSON object key (`"additionalDirectories":`) in an example.
//
// Reads are deliberately allowed: migrating the dead key off an existing file
// has to look at it (`Array.isArray(s.additionalDirectories) ? …`) and remove
// it (`delete s.additionalDirectories`). Banning every mention would make the
// migration unwritable, so only writing the key is an error.
//
// The JSON-key arm is filtered by grantLineIsNested below rather than by the
// pattern, because whether such a key is top-level depends on the object it
// sits in, which a line-wise regex cannot see.
var topLevelAdditionalDirsRe = regexp.MustCompile(
	`(?:^|[^.\w"'])[A-Za-z_$][\w$]*\s*(?:\.\s*additionalDirectories|\[\s*["']additionalDirectories["']\s*\])\s*(?:=\s*(?:[^=]|$)|\.\s*push\s*\()` +
		`|["']additionalDirectories["']\s*:`)

// grantLineIsNested reports whether a matched line is the correct nested form,
// or prose about it, and so is not a finding.
func grantLineIsNested(line string) bool {
	return strings.Contains(line, "permissions.additionalDirectories") ||
		strings.Contains(line, "permissions\"") ||
		strings.Contains(line, "s.permissions")
}

// grantAssetFiles lists every shipped asset that could carry the grant step.
// It is deliberately wider than repoSkillFiles: docs/guides/worktree-per-ticket.md
// documents the same step and regressed alongside the skills, so a
// guard scoped to SKILL.md alone would let the doc drift back on its own.
func grantAssetFiles(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, path := range repoSkillFiles(t) {
		out["skill:"+name] = path
	}
	root := filepath.Join("..", "..", "..")
	for _, rel := range []string{
		filepath.Join("docs", "guides", "worktree-per-ticket.md"),
		filepath.Join("docs", "guides", "cleanup-safety.md"),
	} {
		p := filepath.Join(root, rel)
		if _, err := os.Stat(p); err == nil {
			out["doc:"+rel] = p
		}
	}
	return out
}

func TestRepoAssetsGrantUsesNestedPermissionsKey(t *testing.T) {
	for name, path := range grantAssetFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		content := string(data)
		if !strings.Contains(content, "additionalDirectories") {
			continue // this asset does not describe the grant
		}
		t.Run(name, func(t *testing.T) {
			for i, line := range strings.Split(content, "\n") {
				if topLevelAdditionalDirsRe.MatchString(line) && !grantLineIsNested(line) {
					t.Errorf("%s:%d writes a top-level additionalDirectories, which Claude Code skips — "+
						"nest it under permissions.additionalDirectories:\n  %s",
						path, i+1, strings.TrimSpace(line))
				}
			}
			if !strings.Contains(content, "permissions.additionalDirectories") {
				t.Errorf("%s mentions additionalDirectories but never the nested "+
					"permissions.additionalDirectories key", path)
			}
		})
	}
}

// The guard above only ever asserts against the current tree, so on its own it
// would also pass if the pattern stopped matching anything. These pin what it
// must catch and what it must leave alone — including the exact top-level
// write that shipped, and the migration lines that legitimately touch the
// legacy key.
func TestTopLevelAdditionalDirsRe(t *testing.T) {
	flagged := func(line string) bool {
		return topLevelAdditionalDirsRe.MatchString(line) && !grantLineIsNested(line)
	}

	mustFlag := map[string]string{
		"the #180 regression":                 `    s.additionalDirectories = s.additionalDirectories || [];`,
		"push onto the root":                  `    if (!s.additionalDirectories.includes(dir)) s.additionalDirectories.push(dir);`,
		"bracket assignment":                  `  settings["additionalDirectories"] = [dir];`,
		"spaced assignment":                   `  s . additionalDirectories  = [];`,
		"an unfamiliar receiver":              `  obj.additionalDirectories = [dir];`,
		"assignment wrapped to the next line": `    cfg.additionalDirectories =`,
		"a flat JSON example":                 `  "additionalDirectories": ["/path/to/worktrees"]`,
	}
	for name, line := range mustFlag {
		t.Run("catches "+name, func(t *testing.T) {
			if !flagged(line) {
				t.Errorf("did not flag a top-level write:\n  %s", line)
			}
		})
	}

	mustAllow := map[string]string{
		"the nested write":         `    s.permissions.additionalDirectories = list;`,
		"nested read":              `    const cur = s.permissions.additionalDirectories;`,
		"nested JSON example":      `    "permissions": { "additionalDirectories": ["/wt"] }`,
		"migration read":           `    for (const d of Array.isArray(s.additionalDirectories) ? s.additionalDirectories : []) {`,
		"migration delete":         `    delete s.additionalDirectories;`,
		"prose naming the key":     "  under **`permissions.additionalDirectories`** (nested under `permissions`)",
		"prose naming the old one": "  a top-level `additionalDirectories` is not part of the schema",
	}
	for name, line := range mustAllow {
		t.Run("allows "+name, func(t *testing.T) {
			if flagged(line) {
				t.Errorf("wrongly flagged:\n  %s", line)
			}
		})
	}
}
