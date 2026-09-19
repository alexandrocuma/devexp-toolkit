package agents

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// ── Repo-asset guards: what Kimi Code CLI requires of a shipped agent ─────────
//
// Kimi parses an agent's front matter as strict YAML, requires `description`
// and a non-empty body, and skips a file that fails either — logging it
// somewhere only a Kimi user would look. These tests read the real `agents/`
// tree, because the thing guarded is the shipped asset.
//
// splitRepoFrontmatter here is a second implementation of Kimi's fence rule on
// purpose (see the same note in internal/skills): a guard that called the
// installer's own splitter would pass whenever the two agreed with each other.

var kimiAgentNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// kimiBuiltinProfiles are Kimi's own agent profiles. A shipped agent may not
// take one of these names without `override: true`, which devexp never emits.
var kimiBuiltinProfiles = map[string]bool{"agent": true, "coder": true, "explore": true, "plan": true}

// kimiPromptVars are every variable Kimi substitutes when it renders an agent
// body. A shipped body must not contain one by accident: they are plausible
// shell variable names, and `${skills}` in particular expands to the whole
// skill catalog. `base_prompt` is excluded here — the Kimi transform adds it
// deliberately, and it is not in any source body.
var kimiPromptVars = []string{
	"role_additional", "product_name", "reply_style_guide", "notify_user_guidance",
	"os", "windows_notes", "shell", "cwd", "cwd_listing", "agents_md",
	"additional_dirs_info", "additional_dirs_section", "skills", "skills_section",
	"plugin_sections", "base_prompt",
}

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

// repoAgentFiles lists every shipped agent, keyed by its name (the filename
// without .md). README.md and the opencode-exclusive directory are excluded,
// exactly as the installers exclude them.
func repoAgentFiles(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "agents")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", root, err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" {
			continue
		}
		out[strings.TrimSuffix(e.Name(), ".md")] = filepath.Join(root, e.Name())
	}
	if len(out) == 0 {
		t.Fatalf("no agents found under %s", root)
	}
	return out
}

func TestRepoAgentsFrontmatterStrictYAML(t *testing.T) {
	for name, path := range repoAgentFiles(t) {
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
				t.Fatalf("%s front matter is not strict YAML, so Kimi would skip the agent: %v", path, err)
			}
			got, _ := parsed["name"].(string)
			if got != name {
				t.Errorf("%s name = %q, want %q (the filename)", path, parsed["name"], name)
			}
			if !kimiAgentNameRe.MatchString(got) {
				t.Errorf("%s name = %q, want kebab-case — Kimi refuses anything else", path, got)
			}
			if kimiBuiltinProfiles[got] {
				t.Errorf("%s name = %q, which is a Kimi built-in profile; it would need override: true", path, got)
			}
			if desc, _ := parsed["description"].(string); strings.TrimSpace(desc) == "" {
				t.Errorf("%s description = %q, want a non-empty string — Kimi requires it", path, parsed["description"])
			}
			if strings.TrimSpace(body) == "" {
				t.Errorf("%s has an empty body — Kimi requires one", path)
			}
			for _, v := range kimiPromptVars {
				if strings.Contains(body, "${"+v+"}") {
					t.Errorf("%s body contains ${%s}, which Kimi substitutes when it renders the prompt", path, v)
				}
			}
		})
	}
}
