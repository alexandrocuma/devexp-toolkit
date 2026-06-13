package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	return string(data)
}

const orchestratorFixture = `---
name: orchestrator
description: "Orchestrates a swarm of specialist subagents."
model: sonnet
mode: primary
permission:
  task:
    "*": allow
---

# Agent Orchestrator
`

func TestTransformForOpencode(t *testing.T) {
	basicAgent := readTestdata(t, "basic-agent.md")
	allToolsAgent := readTestdata(t, "all-tools-agent.md")
	noFrontmatter := readTestdata(t, "no-frontmatter.md")

	customModelAgent := `---
name: custom-agent
description: "Agent with a custom model id."
model: some-custom-model-id
tools: Read
color: green
memory: project
---

# Custom Agent
`

	tests := []struct {
		name          string
		input         string
		selectedModel string
		modelOnly     bool
		want          string
	}{
		{
			name:          "modelOnly with override replaces model line",
			input:         orchestratorFixture,
			selectedModel: "opus",
			modelOnly:     true,
			want:          strings.Replace(orchestratorFixture, "model: sonnet", "model: anthropic/claude-opus-4-6", 1),
		},
		{
			name:          "modelOnly without override leaves content unchanged",
			input:         orchestratorFixture,
			selectedModel: "",
			modelOnly:     true,
			want:          orchestratorFixture,
		},
		{
			name:          "full transform strips name/color/memory, remaps model, adds disabled tools and mode",
			input:         basicAgent,
			selectedModel: "",
			modelOnly:     false,
			want: `---
description: "A minimal test agent used for installer transform tests."
model: anthropic/claude-sonnet-4-6
tools:
  edit: false
  glob: false
  grep: false
  webfetch: false
  websearch: false
mode: subagent
---

# Basic Agent

This is a short body used to verify that the body content is preserved
unchanged after the frontmatter transform.
`,
		},
		{
			name:          "model override remaps model regardless of original alias",
			input:         basicAgent,
			selectedModel: "haiku",
			modelOnly:     false,
			want: `---
description: "A minimal test agent used for installer transform tests."
model: anthropic/claude-haiku-4-5-20251001
tools:
  edit: false
  glob: false
  grep: false
  webfetch: false
  websearch: false
mode: subagent
---

# Basic Agent

This is a short body used to verify that the body content is preserved
unchanged after the frontmatter transform.
`,
		},
		{
			name:          "all opencode tools enabled emits no tools block",
			input:         allToolsAgent,
			selectedModel: "",
			modelOnly:     false,
			want: `---
description: "A test agent with every opencode-supported tool enabled."
model: anthropic/claude-sonnet-4-6
mode: subagent
---

# All Tools Agent

This agent has every opencode-supported capability enabled, so the
transform should not emit a disabled-tools section in the frontmatter.
`,
		},
		{
			name:          "content with fewer than 3 frontmatter delimiters is returned unchanged",
			input:         noFrontmatter,
			selectedModel: "",
			modelOnly:     false,
			want:          noFrontmatter,
		},
		{
			name:          "unresolved model alias passes through resolveModel fallthrough",
			input:         customModelAgent,
			selectedModel: "",
			modelOnly:     false,
			want: `---
description: "Agent with a custom model id."
model: some-custom-model-id
tools:
  bash: false
  edit: false
  glob: false
  grep: false
  webfetch: false
  websearch: false
  write: false
mode: subagent
---

# Custom Agent
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := transformForOpencode(tt.input, tt.selectedModel, tt.modelOnly)
			if err != nil {
				t.Fatalf("transformForOpencode() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("transformForOpencode() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
