package agents

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

	tests := map[string]struct {
		input         string
		selectedModel string
		modelOnly     bool
		want          string
	}{
		"modelOnly with override replaces model line": {
			input:         orchestratorFixture,
			selectedModel: "opus",
			modelOnly:     true,
			want:          strings.Replace(orchestratorFixture, "model: sonnet", "model: anthropic/claude-opus-4-6", 1),
		},
		"modelOnly without override leaves content unchanged": {
			input:         orchestratorFixture,
			selectedModel: "",
			modelOnly:     true,
			want:          orchestratorFixture,
		},
		"full transform strips name/color/memory, remaps model, adds disabled tools and mode": {
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
		"model override remaps model regardless of original alias": {
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
		"all opencode tools enabled emits no tools block": {
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
		"content with fewer than 3 frontmatter delimiters is returned unchanged": {
			input:         noFrontmatter,
			selectedModel: "",
			modelOnly:     false,
			want:          noFrontmatter,
		},
		"unresolved model alias passes through resolveModel fallthrough": {
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

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
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

func writeAgentFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
}

func TestInstallClaude(t *testing.T) {
	agentContent := `---
name: sample-agent
description: "A sample agent"
model: sonnet
---

# Sample Agent
`

	tests := map[string]struct {
		files         map[string]string
		model         string
		disabled      []string
		dryRun        bool
		wantInstalled []string
	}{
		"installs all md files except README": {
			files: map[string]string{
				"agent-a.md": agentContent,
				"agent-b.md": agentContent,
				"README.md":  "# readme",
				"notes.txt":  "not an agent",
			},
			wantInstalled: []string{"agent-a.md", "agent-b.md"},
		},
		"skips disabled agents and excludes from installed list": {
			files: map[string]string{
				"agent-a.md": agentContent,
				"agent-b.md": agentContent,
			},
			disabled:      []string{"agent-b"},
			wantInstalled: []string{"agent-a.md"},
		},
		"dry run returns would-be list without writing files": {
			files: map[string]string{
				"agent-a.md": agentContent,
			},
			dryRun:        true,
			wantInstalled: []string{"agent-a.md"},
		},
		"applies model override to model line": {
			files: map[string]string{
				"agent-a.md": agentContent,
			},
			model:         "haiku",
			wantInstalled: []string{"agent-a.md"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srcDir := t.TempDir()
			targetDir := filepath.Join(t.TempDir(), "target")
			writeAgentFiles(t, srcDir, tt.files)

			got, err := InstallClaude(srcDir, targetDir, tt.model, tt.disabled, tt.dryRun)
			if err != nil {
				t.Fatalf("InstallClaude() error = %v", err)
			}

			sort.Strings(got)
			sort.Strings(tt.wantInstalled)
			if !reflect.DeepEqual(got, tt.wantInstalled) {
				t.Errorf("InstallClaude() = %v, want %v", got, tt.wantInstalled)
			}

			if tt.dryRun {
				if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
					t.Errorf("dry run should not create targetDir")
				}
				return
			}

			if tt.model != "" {
				data, err := os.ReadFile(filepath.Join(targetDir, "agent-a.md"))
				if err != nil {
					t.Fatalf("ReadFile(agent-a.md) error = %v", err)
				}
				if !strings.Contains(string(data), "model: anthropic/claude-haiku-4-5-20251001") {
					t.Errorf("installed file does not contain overridden model line:\n%s", data)
				}
			}
		})
	}
}

func TestInstallOpencode(t *testing.T) {
	agentContent := `---
name: sample-agent
description: "A sample agent"
model: sonnet
tools: Read, Edit
---

# Sample Agent
`

	tests := map[string]struct {
		files         map[string]string
		disabled      []string
		dryRun        bool
		wantInstalled []string
	}{
		"installs all md files except README": {
			files: map[string]string{
				"agent-a.md": agentContent,
				"agent-b.md": agentContent,
				"README.md":  "# readme",
			},
			wantInstalled: []string{"agent-a.md", "agent-b.md"},
		},
		"skips disabled agents and excludes from installed list": {
			files: map[string]string{
				"agent-a.md": agentContent,
				"agent-b.md": agentContent,
			},
			disabled:      []string{"agent-b"},
			wantInstalled: []string{"agent-a.md"},
		},
		"dry run returns would-be list without writing files": {
			files: map[string]string{
				"agent-a.md": agentContent,
			},
			dryRun:        true,
			wantInstalled: []string{"agent-a.md"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srcDir := t.TempDir()
			targetDir := filepath.Join(t.TempDir(), "target")
			writeAgentFiles(t, srcDir, tt.files)

			got, err := InstallOpencode(srcDir, targetDir, "", tt.disabled, tt.dryRun)
			if err != nil {
				t.Fatalf("InstallOpencode() error = %v", err)
			}

			sort.Strings(got)
			sort.Strings(tt.wantInstalled)
			if !reflect.DeepEqual(got, tt.wantInstalled) {
				t.Errorf("InstallOpencode() = %v, want %v", got, tt.wantInstalled)
			}

			if tt.dryRun {
				if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
					t.Errorf("dry run should not create targetDir")
				}
			}
		})
	}
}

func TestInstallOpencodeExclusive(t *testing.T) {
	exclusiveContent := `---
name: opencode-only-agent
description: "An opencode-exclusive agent"
model: sonnet
---

# Opencode Only Agent
`

	tests := map[string]struct {
		missingSrcDir bool
		files         map[string]string
		dryRun        bool
		wantInstalled []string
	}{
		"missing srcDir returns nil installed and nil error": {
			missingSrcDir: true,
		},
		"installs all md files in srcDir": {
			files: map[string]string{
				"opencode-agent-a.md": exclusiveContent,
				"opencode-agent-b.md": exclusiveContent,
			},
			wantInstalled: []string{"opencode-agent-a.md", "opencode-agent-b.md"},
		},
		"dry run returns would-be list without writing files": {
			files: map[string]string{
				"opencode-agent-a.md": exclusiveContent,
			},
			dryRun:        true,
			wantInstalled: []string{"opencode-agent-a.md"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var srcDir string
			if tt.missingSrcDir {
				srcDir = filepath.Join(t.TempDir(), "does-not-exist")
			} else {
				srcDir = t.TempDir()
				writeAgentFiles(t, srcDir, tt.files)
			}
			targetDir := filepath.Join(t.TempDir(), "target")

			got, err := InstallOpencodeExclusive(srcDir, targetDir, "", tt.dryRun)
			if err != nil {
				t.Fatalf("InstallOpencodeExclusive() error = %v", err)
			}

			sort.Strings(got)
			sort.Strings(tt.wantInstalled)
			if !reflect.DeepEqual(got, tt.wantInstalled) {
				t.Errorf("InstallOpencodeExclusive() = %v, want %v", got, tt.wantInstalled)
			}
		})
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stdout = orig }()
	fn()
	w.Close()
	return <-done
}

// TestInstall_SymlinkedAgentEntry (#124): an installed agent file that is a
// symlink — to a customised copy in dotfiles, or to the toolkit's own source —
// is never written through or replaced, for every installer and in dry runs.
// It stays in the install set, so the manifest keeps tracking it. A regular
// file next to it is replaced atomically.
func TestInstall_SymlinkedAgentEntry(t *testing.T) {
	// A description and a body: the other three installers do not need them,
	// but Kimi refuses an agent without either, and the point of this test is
	// the symlink rule rather than the transform.
	agent := "---\nname: a\ndescription: \"a test agent\"\nmodel: sonnet\n---\n\n# A\n"
	installers := map[string]func(src, target string, dryRun bool) ([]string, error){
		"InstallKimi": func(src, target string, dryRun bool) ([]string, error) {
			return InstallKimi(src, target, "/home/u/.kimi-code/agents", nil, dryRun)
		},
		"InstallClaude": func(src, target string, dryRun bool) ([]string, error) {
			return InstallClaude(src, target, "opus", nil, dryRun)
		},
		"InstallOpencode": func(src, target string, dryRun bool) ([]string, error) {
			return InstallOpencode(src, target, "opus", nil, dryRun)
		},
		"InstallOpencodeExclusive": func(src, target string, dryRun bool) ([]string, error) {
			return InstallOpencodeExclusive(src, target, "opus", dryRun)
		},
	}
	for name, install := range installers {
		for _, dangling := range []bool{false, true} {
			for _, dryRun := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s dangling=%v dryRun=%v", name, dangling, dryRun), func(t *testing.T) {
					src, target, dotfiles := t.TempDir(), t.TempDir(), t.TempDir()
					writeAgentFiles(t, src, map[string]string{"linked.md": agent, "plain.md": agent})
					mine := filepath.Join(dotfiles, "linked.md")
					if !dangling {
						os.WriteFile(mine, []byte("my own agent\n"), 0644) //nolint:errcheck
					}
					if err := os.Symlink(mine, filepath.Join(target, "linked.md")); err != nil {
						t.Fatal(err)
					}
					os.WriteFile(filepath.Join(target, "plain.md"), []byte("old\n"), 0644) //nolint:errcheck

					var installed []string
					var err error
					out := captureStdout(t, func() { installed, err = install(src, target, dryRun) })
					if err != nil {
						t.Fatalf("error = %v\n%s", err, out)
					}
					sort.Strings(installed)
					if want := []string{"linked.md", "plain.md"}; !reflect.DeepEqual(installed, want) {
						t.Errorf("installed = %v, want %v", installed, want)
					}
					if fi, err := os.Lstat(filepath.Join(target, "linked.md")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
						t.Errorf("symlinked agent replaced")
					}
					got, statErr := os.ReadFile(mine)
					if dangling && !os.IsNotExist(statErr) {
						t.Errorf("a file was created at the dangling link's destination")
					}
					if !dangling && string(got) != "my own agent\n" {
						t.Errorf("link target = %q, written through", got)
					}
					if !strings.Contains(out, "is a symlink, so it was left untouched") || strings.Contains(out, "m linked.md") || strings.Contains(out, "/linked.md\n") {
						t.Errorf("no warning for the symlinked agent (or it was reported as written):\n%s", out)
					}
					plain, _ := os.ReadFile(filepath.Join(target, "plain.md"))
					if dryRun != (string(plain) == "old\n") {
						t.Errorf("plain.md = %q (dryRun=%v)", plain, dryRun)
					}
					if entries, _ := os.ReadDir(target); len(entries) != 2 {
						t.Errorf("target holds %d entries, want 2 (no temp files)", len(entries))
					}
				})
			}
		}
	}
}

// TestInstall_AgentFilesNeverWrittenInPlace (#157 review): agent files are
// replaced through a temp file and a rename, for every installer. In a
// directory where no temp file can be created, the write fails and the old
// bytes stay, where an in-place os.WriteFile would have succeeded.
func TestInstall_AgentFilesNeverWrittenInPlace(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	installers := map[string]func(src, target string) ([]string, error){
		"InstallClaude":            func(src, target string) ([]string, error) { return InstallClaude(src, target, "", nil, false) },
		"InstallOpencode":          func(src, target string) ([]string, error) { return InstallOpencode(src, target, "", nil, false) },
		"InstallOpencodeExclusive": func(src, target string) ([]string, error) { return InstallOpencodeExclusive(src, target, "", false) },
	}
	for name, install := range installers {
		t.Run(name, func(t *testing.T) {
			src, target := t.TempDir(), t.TempDir()
			writeAgentFiles(t, src, map[string]string{"a.md": "---\nname: a\n---\n# A\n"})
			os.WriteFile(filepath.Join(target, "a.md"), []byte("old\n"), 0644) //nolint:errcheck
			os.Chmod(target, 0555)                                             //nolint:errcheck
			t.Cleanup(func() { os.Chmod(target, 0755) })                       //nolint:errcheck
			var err error
			captureStdout(t, func() { _, err = install(src, target) })
			if got, _ := os.ReadFile(filepath.Join(target, "a.md")); err == nil || string(got) != "old\n" {
				t.Errorf("error = %v, a.md = %q; want an error and the old bytes", err, got)
			}
		})
	}
}
