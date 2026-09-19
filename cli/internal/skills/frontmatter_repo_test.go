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
