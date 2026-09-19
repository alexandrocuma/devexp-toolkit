package agents

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// kimiAgent builds a source agent with the given front-matter lines.
func kimiAgent(fm ...string) string {
	return "---\n" + strings.Join(fm, "\n") + "\n---\n\n# Body\n\nSome instructions.\n"
}

const kimiAgentsDir = "/home/u/.kimi-code/agents"

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// parseKimiOutput splits a transformed agent and decodes its front matter.
func parseKimiOutput(t *testing.T, out string) (map[string]any, string) {
	t.Helper()
	fm, body, ok := splitFrontmatter(out)
	if !ok {
		t.Fatalf("transformed agent has no front matter:\n%s", out)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(fm), &parsed); err != nil {
		t.Fatalf("transformed front matter is not valid YAML (Kimi would skip the agent): %v\n%s", err, fm)
	}
	return parsed, body
}

// TestKimiTools is the tool-map table. The case that matters most is TaskList:
// Kimi has a tool by that exact name which lists background tasks, so a
// transform that passed through any name Kimi happens to accept would install
// an agent with a capability nobody asked for.
func TestKimiTools(t *testing.T) {
	tests := map[string]struct {
		claude      []string
		wantTools   []string
		wantDropped []string
	}{
		"names Kimi shares keep their spelling": {
			claude:    []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep", "Agent", "Skill", "WebSearch"},
			wantTools: []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep", "Agent", "Skill", "WebSearch"},
		},
		"WebFetch becomes FetchURL": {
			claude:    []string{"WebFetch"},
			wantTools: []string{"FetchURL"},
		},
		"TaskList becomes TodoList and never stays TaskList": {
			claude:    []string{"TaskList"},
			wantTools: []string{"TodoList"},
		},
		"all four task tools and TodoWrite collapse onto one TodoList": {
			claude:    []string{"TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "TodoWrite"},
			wantTools: []string{"TodoList"},
		},
		"declaration order is preserved": {
			claude:    []string{"Grep", "Bash", "Read"},
			wantTools: []string{"Grep", "Bash", "Read"},
		},
		"a tool with no Kimi equivalent is dropped and reported": {
			claude:      []string{"Read", "NotebookEdit", "Bash"},
			wantTools:   []string{"Read", "Bash"},
			wantDropped: []string{"NotebookEdit"},
		},
		"an mcp server tool passes through": {
			claude:    []string{"mcp__context7__query-docs"},
			wantTools: []string{"mcp__context7__query-docs"},
		},
		"blank entries are ignored": {
			claude:    []string{"Read", "", "  ", "Bash"},
			wantTools: []string{"Read", "Bash"},
		},
		"every name dropped leaves nothing": {
			claude:      []string{"WebFetch2", "NotebookEdit"},
			wantDropped: []string{"WebFetch2", "NotebookEdit"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tools, dropped := kimiTools(tt.claude)
			if !reflect.DeepEqual(tools, tt.wantTools) {
				t.Errorf("kimiTools() tools = %v, want %v", tools, tt.wantTools)
			}
			if !reflect.DeepEqual(dropped, tt.wantDropped) {
				t.Errorf("kimiTools() dropped = %v, want %v", dropped, tt.wantDropped)
			}
			for _, got := range tools {
				if got == "TaskList" || got == "TaskOutput" || got == "TaskStop" || got == "WebFetch" || got == "TodoWrite" {
					t.Errorf("kimiTools() emitted %q, which in Kimi is either absent or an unrelated tool", got)
				}
			}
		})
	}
}

func TestTransformForKimi(t *testing.T) {
	t.Run("keeps the description value whatever quoting the source used", func(t *testing.T) {
		sources := map[string]struct{ fm, want string }{
			"unquoted": {
				fm:   `description: A plain description — with an em dash.`,
				want: "A plain description — with an em dash.",
			},
			"quoted with escapes": {
				fm:   `description: "Line one.\n\n<example>\nuser: \"do it\"\n</example>"`,
				want: "Line one.\n\n<example>\nuser: \"do it\"\n</example>",
			},
			"value containing a colon-space, which is why the sources are quoted": {
				fm:   `description: "Release phase: takes work from merge to shipped."`,
				want: "Release phase: takes work from merge to shipped.",
			},
		}
		for name, tt := range sources {
			t.Run(name, func(t *testing.T) {
				out, _, err := transformForKimi(kimiAgent("name: a", tt.fm), "a", kimiAgentsDir, nil)
				if err != nil {
					t.Fatalf("transformForKimi() error = %v", err)
				}
				parsed, _ := parseKimiOutput(t, out)
				if got, _ := parsed["description"].(string); got != tt.want {
					t.Errorf("description = %q, want %q", got, tt.want)
				}
			})
		}
	})

	t.Run("drops the keys Kimi ignores and emits tools as a list", func(t *testing.T) {
		out, dropped, err := transformForKimi(kimiAgent(
			"name: a",
			`description: "d"`,
			"tools: Read, WebFetch, TaskList, NotebookEdit",
			"color: cyan",
			"memory: user",
			"model: sonnet",
		), "a", kimiAgentsDir, nil)
		if err != nil {
			t.Fatalf("transformForKimi() error = %v", err)
		}
		parsed, _ := parseKimiOutput(t, out)
		for _, k := range []string{"color", "memory", "model"} {
			if _, present := parsed[k]; present {
				t.Errorf("%s survived the transform; Kimi ignores it, so it must not suggest otherwise", k)
			}
		}
		tools, ok := parsed["tools"].([]any)
		if !ok {
			t.Fatalf("tools = %#v, want a YAML list", parsed["tools"])
		}
		var got []string
		for _, v := range tools {
			got = append(got, v.(string))
		}
		if want := []string{"Read", "FetchURL", "TodoList"}; !reflect.DeepEqual(got, want) {
			t.Errorf("tools = %v, want %v", got, want)
		}
		if want := []string{"NotebookEdit"}; !reflect.DeepEqual(dropped, want) {
			t.Errorf("dropped = %v, want %v", dropped, want)
		}
	})

	t.Run("subagents is emitted only for an agent that declares the Agent tool", func(t *testing.T) {
		allow := []string{"helper", "explore", "plan"}
		for _, tc := range []struct {
			tools string
			want  bool
		}{
			{"Read, Bash, Agent", true},
			{"Read, Bash", false},
		} {
			out, _, err := transformForKimi(kimiAgent("name: a", `description: "d"`, "tools: "+tc.tools), "a", kimiAgentsDir, allow)
			if err != nil {
				t.Fatalf("transformForKimi() error = %v", err)
			}
			parsed, _ := parseKimiOutput(t, out)
			if _, present := parsed["subagents"]; present != tc.want {
				t.Errorf("tools %q: subagents present = %v, want %v", tc.tools, present, tc.want)
			}
		}
	})

	t.Run("an agent with no tools key keeps none, so Kimi grants it everything as Claude Code does", func(t *testing.T) {
		out, _, err := transformForKimi(kimiAgent("name: a", `description: "d"`), "a", kimiAgentsDir, nil)
		if err != nil {
			t.Fatalf("transformForKimi() error = %v", err)
		}
		parsed, _ := parseKimiOutput(t, out)
		if _, present := parsed["tools"]; present {
			t.Errorf("tools was invented for a source that declares none: %#v", parsed["tools"])
		}
	})

	t.Run("an unknown key is carried over rather than silently dropped", func(t *testing.T) {
		out, _, err := transformForKimi(kimiAgent("name: a", `description: "d"`, "whenToUse: for tests"), "a", kimiAgentsDir, nil)
		if err != nil {
			t.Fatalf("transformForKimi() error = %v", err)
		}
		parsed, _ := parseKimiOutput(t, out)
		if got, _ := parsed["whenToUse"].(string); got != "for tests" {
			t.Errorf("whenToUse = %q, want it carried over", got)
		}
	})

	t.Run("the body is copied byte for byte apart from the path rewrite and the footer", func(t *testing.T) {
		src := "---\nname: a\ndescription: \"d\"\n---\n\n# A\n\nRead `~/.claude/agents/gen-docs.md` and follow it.\nKeep `~/.claude/agent-memory/a/MEMORY.md`.\n"
		out, _, err := transformForKimi(src, "a", kimiAgentsDir, nil)
		if err != nil {
			t.Fatalf("transformForKimi() error = %v", err)
		}
		// Asserted on the output string, not on a re-split body: the point is
		// that the bytes after the closing fence are unchanged.
		want := "---\n\n# A\n\nRead `" + kimiAgentsDir + "/gen-docs.md` and follow it.\nKeep `~/.claude/agent-memory/a/MEMORY.md`.\n" + kimiBasePromptFooter
		if !strings.HasSuffix(out, want) {
			t.Errorf("body = %q\nwant it to end with %q", out, want)
		}
	})

	t.Run("every transformed agent opts into Kimi's base prompt exactly once", func(t *testing.T) {
		out, _, err := transformForKimi(kimiAgent("name: a", `description: "d"`), "a", kimiAgentsDir, nil)
		if err != nil {
			t.Fatalf("transformForKimi() error = %v", err)
		}
		// Without this, Kimi renders the body as the whole system prompt: no
		// tool guidance, no AGENTS.md, no skills catalog, and no visible
		// failure.
		if n := strings.Count(out, "${base_prompt}"); n != 1 {
			t.Errorf("output contains %d ${base_prompt} markers, want exactly 1", n)
		}
		if !strings.HasSuffix(out, kimiBasePromptFooter) {
			t.Errorf("the base-prompt footer is not last:\n%q", out[len(out)-80:])
		}
	})

	t.Run("refusals", func(t *testing.T) {
		tests := map[string]struct{ src, wantErr string }{
			"no front matter":                 {"# Just a body\n", "no front matter"},
			"front matter is not YAML":        {kimiAgent("name: a", `description: Release: shipped`), "not valid YAML"},
			"front matter is not a mapping":   {"---\n- one\n- two\n---\nbody\n", "not a mapping"},
			"no description":                  {kimiAgent("name: a", "tools: Read"), "no description"},
			"empty description":               {kimiAgent("name: a", `description: "  "`), "description is empty"},
			"name is not kebab-case":          {kimiAgent("name: Bad_Name", `description: "d"`), "not kebab-case"},
			"name is a Kimi built-in":         {kimiAgent("name: plan", `description: "d"`), "built-in profiles"},
			"empty body":                      {"---\nname: a\ndescription: \"d\"\n---\n\n\n", "body is empty"},
			"tools is not a string or a list": {"---\nname: a\ndescription: \"d\"\ntools:\n  read: true\n---\nbody\n", "tools:"},
			// Kimi loads such an agent, lists it, and offers it as a delegation
			// target with no tools, no error and nothing in its log.
			"every tool dropped": {kimiAgent("name: a", `description: "d"`, "tools: NotebookEdit, SlashCommand"), "can do nothing"},
		}
		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				_, _, err := transformForKimi(tt.src, "a", kimiAgentsDir, nil)
				if err == nil {
					t.Fatalf("transformForKimi() error = nil, want one mentioning %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("transformForKimi() error = %v, want it to mention %q", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("a body with no blank line after the fence still closes the front matter", func(t *testing.T) {
		out, _, err := transformForKimi("---\nname: a\ndescription: \"d\"\n---\nStraight into the body.\n", "a", kimiAgentsDir, nil)
		if err != nil {
			t.Fatalf("transformForKimi() error = %v", err)
		}
		if strings.Contains(out, "---Straight") {
			t.Fatalf("the closing fence was glued onto the body, so Kimi would not read this as an agent:\n%s", out)
		}
		parsed, body := parseKimiOutput(t, out)
		if got, _ := parsed["name"].(string); got != "a" {
			t.Errorf("name = %q, want %q", got, "a")
		}
		if !strings.HasPrefix(body, "Straight into the body.") {
			t.Errorf("body = %q, want it to start with the source body", body)
		}
	})

	t.Run("a missing name falls back to the filename, as Kimi's own does", func(t *testing.T) {
		out, _, err := transformForKimi("---\ndescription: \"d\"\n---\nbody\n", "from-file", kimiAgentsDir, nil)
		if err != nil {
			t.Fatalf("transformForKimi() error = %v", err)
		}
		parsed, _ := parseKimiOutput(t, out)
		if got, _ := parsed["name"].(string); got != "from-file" {
			t.Errorf("name = %q, want %q", got, "from-file")
		}
	})
}

func TestRewriteClaudeAgentRefs(t *testing.T) {
	tests := map[string]struct{ body, want string }{
		"an agent file reference is repointed": {
			"read ~/.claude/agents/gen-docs.md and follow it",
			"read " + kimiAgentsDir + "/gen-docs.md and follow it",
		},
		"several on one line are all repointed": {
			"~/.claude/agents/a.md and ~/.claude/agents/b-c.md",
			kimiAgentsDir + "/a.md and " + kimiAgentsDir + "/b-c.md",
		},
		// Both CLIs share the memory directory deliberately (the atlas, the
		// per-project notes), so it is not a Claude-only path. The markdown
		// cases matter most: only the `agents/` segment may be rewritten, and
		// a rule keyed on "any directory under ~/.claude" would take these too.
		"agent memory is left alone": {
			"at ~/.claude/agent-memory/codebase-navigator/",
			"at ~/.claude/agent-memory/codebase-navigator/",
		},
		"a markdown file directly under agent memory is left alone": {
			"see ~/.claude/agent-memory/notes.md for the history",
			"see ~/.claude/agent-memory/notes.md for the history",
		},
		"a markdown file under a nested memory directory is left alone": {
			"see ~/.claude/agent-memory/dev-agent/devexp-toolkit.md for the notes",
			"see ~/.claude/agent-memory/dev-agent/devexp-toolkit.md for the notes",
		},
		"a markdown file under another ~/.claude directory is left alone": {
			"~/.claude/skills/notes.md",
			"~/.claude/skills/notes.md",
		},
		"the projects directory is left alone": {
			"~/.claude/projects/foo/memory/notes.md",
			"~/.claude/projects/foo/memory/notes.md",
		},
		"a bare agents directory with no filename is prose, not a reference": {
			"deployed to ~/.claude/agents/",
			"deployed to ~/.claude/agents/",
		},
		"a non-markdown path is left alone": {
			"~/.claude/agents/notes.txt",
			"~/.claude/agents/notes.txt",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := rewriteClaudeAgentRefs(tt.body, kimiAgentsDir); got != tt.want {
				t.Errorf("rewriteClaudeAgentRefs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKimiSubagents(t *testing.T) {
	got := kimiSubagents(map[string]bool{
		"dev-agent":  true, // delegates
		"root-cause": true, // delegates
		"security":   false,
		"changelog":  false,
	})
	want := []string{"changelog", "security", "explore", "plan"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("kimiSubagents() = %v, want %v", got, want)
	}
	// An unbounded allowlist would let dev-agent → backend-senior-dev →
	// dev-agent run as deep as the model takes it.
	for _, name := range got {
		if name == "dev-agent" || name == "root-cause" {
			t.Errorf("kimiSubagents() offered the delegating agent %q, which allows an unbounded chain", name)
		}
	}
}

func TestInstallKimi(t *testing.T) {
	agent := func(name, tools string) string {
		fm := []string{"name: " + name, `description: "d for ` + name + `"`, "color: cyan", "memory: user"}
		if tools != "" {
			fm = append(fm, "tools: "+tools)
		}
		return kimiAgent(fm...)
	}
	files := map[string]string{
		"orchestrate.md": agent("orchestrate", "Read, Bash, Agent"),
		"helper.md":      agent("helper", "Read, WebFetch"),
		"other.md":       agent("other", "Grep"),
		"README.md":      "# readme, never installed",
		"notes.txt":      "not an agent",
	}

	t.Run("installs every agent, skipping README and non-markdown files", func(t *testing.T) {
		src, target := t.TempDir(), filepath.Join(t.TempDir(), "agents")
		writeAgentFiles(t, src, files)
		var got []string
		var err error
		out := captureStdout(t, func() { got, err = InstallKimi(src, target, kimiAgentsDir, nil, false) })
		if err != nil {
			t.Fatalf("InstallKimi() error = %v\n%s", err, out)
		}
		sort.Strings(got)
		if want := []string{"helper.md", "orchestrate.md", "other.md"}; !reflect.DeepEqual(got, want) {
			t.Errorf("InstallKimi() = %v, want %v", got, want)
		}
		// WebFetch has an equivalent, so nothing should be reported dropped.
		if strings.Contains(out, "no equivalent") {
			t.Errorf("reported a dropped tool for agents that have none:\n%s", out)
		}
	})

	t.Run("the subagents list holds only agents this run installed that cannot themselves delegate", func(t *testing.T) {
		src, target := t.TempDir(), filepath.Join(t.TempDir(), "agents")
		writeAgentFiles(t, src, files)
		captureStdout(t, func() { InstallKimi(src, target, kimiAgentsDir, []string{"other"}, false) }) //nolint:errcheck
		parsed, _ := parseKimiOutput(t, readFile(t, filepath.Join(target, "orchestrate.md")))
		var got []string
		for _, v := range parsed["subagents"].([]any) {
			got = append(got, v.(string))
		}
		// "other" is disabled this run, and "orchestrate" delegates itself.
		if want := []string{"helper", "explore", "plan"}; !reflect.DeepEqual(got, want) {
			t.Errorf("subagents = %v, want %v", got, want)
		}
	})

	t.Run("a disabled agent is neither written nor returned", func(t *testing.T) {
		src, target := t.TempDir(), filepath.Join(t.TempDir(), "agents")
		writeAgentFiles(t, src, files)
		var got []string
		out := captureStdout(t, func() { got, _ = InstallKimi(src, target, kimiAgentsDir, []string{"helper"}, false) })
		sort.Strings(got)
		if want := []string{"orchestrate.md", "other.md"}; !reflect.DeepEqual(got, want) {
			t.Errorf("InstallKimi() = %v, want %v", got, want)
		}
		if !strings.Contains(out, "[skip] helper.md") {
			t.Errorf("no skip notice for the disabled agent:\n%s", out)
		}
		if exists(filepath.Join(target, "helper.md")) {
			t.Error("a disabled agent was written")
		}
	})

	t.Run("a dry run writes nothing and still reports what it would write", func(t *testing.T) {
		src, target := t.TempDir(), filepath.Join(t.TempDir(), "agents")
		writeAgentFiles(t, src, files)
		var got []string
		out := captureStdout(t, func() { got, _ = InstallKimi(src, target, kimiAgentsDir, nil, true) })
		if len(got) != 3 {
			t.Errorf("InstallKimi() = %v, want three entries so the manifest still tracks them", got)
		}
		if exists(target) {
			t.Error("a dry run created the target directory")
		}
		for _, name := range []string{"orchestrate.md", "helper.md", "other.md"} {
			if !strings.Contains(out, filepath.Join(target, name)) {
				t.Errorf("the dry run does not name %s:\n%s", name, out)
			}
		}
	})

	t.Run("an agent Kimi would refuse warns and is skipped while the rest install", func(t *testing.T) {
		src, target := t.TempDir(), filepath.Join(t.TempDir(), "agents")
		bad := map[string]string{
			"no-desc.md":  kimiAgent("name: no-desc", "tools: Read"),
			"bad-yaml.md": kimiAgent("name: bad-yaml", "description: Release: shipped"),
			"inert.md":    kimiAgent("name: inert", `description: "d"`, "tools: NotebookEdit"),
			"good.md":     agent("good", "Read"),
		}
		writeAgentFiles(t, src, bad)
		var got []string
		out := captureStdout(t, func() { got, _ = InstallKimi(src, target, kimiAgentsDir, nil, false) })
		if want := []string{"good.md"}; !reflect.DeepEqual(got, want) {
			t.Errorf("InstallKimi() = %v, want %v", got, want)
		}
		for _, name := range []string{"no-desc.md", "bad-yaml.md", "inert.md"} {
			if !strings.Contains(out, "transform "+name) {
				t.Errorf("no warning naming %s:\n%s", name, out)
			}
		}
		// A refused agent must not be offered to a parent either.
		if exists(filepath.Join(target, "inert.md")) {
			t.Error("an agent with no usable tools was installed")
		}
	})

	t.Run("a dropped tool is reported, because Kimi drops it without saying so", func(t *testing.T) {
		src, target := t.TempDir(), filepath.Join(t.TempDir(), "agents")
		writeAgentFiles(t, src, map[string]string{"a.md": agent("a", "Read, NotebookEdit")})
		out := captureStdout(t, func() { InstallKimi(src, target, kimiAgentsDir, nil, false) }) //nolint:errcheck
		if !strings.Contains(out, "NotebookEdit") || !strings.Contains(out, "no equivalent") {
			t.Errorf("the dropped tool is not named:\n%s", out)
		}
	})

	t.Run("a second run writes the same bytes", func(t *testing.T) {
		src, target := t.TempDir(), filepath.Join(t.TempDir(), "agents")
		writeAgentFiles(t, src, files)
		captureStdout(t, func() { InstallKimi(src, target, kimiAgentsDir, nil, false) }) //nolint:errcheck
		first := readFile(t, filepath.Join(target, "orchestrate.md"))
		captureStdout(t, func() { InstallKimi(src, target, kimiAgentsDir, nil, false) }) //nolint:errcheck
		if second := readFile(t, filepath.Join(target, "orchestrate.md")); second != first {
			t.Errorf("a re-install changed the file:\nfirst:\n%s\nsecond:\n%s", first, second)
		}
	})
}

// TestRepoAgentsKimiTransform runs the real agents/ tree through the transform.
// The unit tests above use fixtures; this is the only check that what devexp
// actually ships survives it.
func TestRepoAgentsKimiTransform(t *testing.T) {
	src := filepath.Join("..", "..", "..", "agents")
	target := filepath.Join(t.TempDir(), "agents")
	var installed []string
	var err error
	out := captureStdout(t, func() { installed, err = InstallKimi(src, target, kimiAgentsDir, nil, false) })
	if err != nil {
		t.Fatalf("InstallKimi() error = %v\n%s", err, out)
	}
	if strings.Contains(out, "transform ") {
		t.Errorf("a shipped agent failed to transform:\n%s", out)
	}
	repo := repoAgentFiles(t)
	if len(installed) != len(repo) {
		t.Errorf("installed %d agents, want all %d shipped ones", len(installed), len(repo))
	}

	known := map[string]bool{}
	for _, v := range kimiToolMap {
		known[v] = true
	}
	var delegating []string
	for _, file := range installed {
		content := readFile(t, filepath.Join(target, file))
		parsed, body := parseKimiOutput(t, content)
		for _, k := range []string{"color", "memory", "model"} {
			if _, present := parsed[k]; present {
				t.Errorf("%s kept %s", file, k)
			}
		}
		tools, ok := parsed["tools"].([]any)
		if !ok {
			t.Errorf("%s has no tools list; every shipped agent declares tools", file)
		}
		for _, v := range tools {
			name := v.(string)
			if !known[name] && !strings.HasPrefix(name, "mcp__") {
				t.Errorf("%s declares %q, which is not a Kimi tool", file, name)
			}
			if name == "WebFetch" {
				t.Errorf("%s still declares WebFetch; Kimi calls it FetchURL and drops the other silently", file)
			}
		}
		if _, present := parsed["subagents"]; present {
			delegating = append(delegating, strings.TrimSuffix(file, ".md"))
		}
		if strings.Contains(body, "~/.claude/agents/") {
			t.Errorf("%s still points at Claude Code's agent directory", file)
		}
		if n := strings.Count(body, "${base_prompt}"); n != 1 {
			t.Errorf("%s has %d base-prompt markers, want 1", file, n)
		}
	}

	// The nine agents that hold the Agent tool, and no others.
	sort.Strings(delegating)
	want := []string{"backend-senior-dev", "data-flow", "dev-agent", "frontend-senior-dev",
		"grooming-agent", "impact-analysis", "onboarding", "root-cause", "tech-debt"}
	if !reflect.DeepEqual(delegating, want) {
		t.Errorf("agents with subagents = %v, want %v", delegating, want)
	}
	delegates := map[string]bool{}
	for _, n := range delegating {
		delegates[n] = true
	}
	parsed, _ := parseKimiOutput(t, readFile(t, filepath.Join(target, "dev-agent.md")))
	for _, v := range parsed["subagents"].([]any) {
		if delegates[v.(string)] {
			t.Errorf("dev-agent may spawn %q, which can delegate further", v)
		}
	}
}
