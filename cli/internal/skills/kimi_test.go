package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"devexp/internal/ui"
)

const kimiAgentsDir = "/home/u/.kimi-code/agents"

func kimiSkill(name, extraFM, body string) string {
	fm := "name: " + name + "\ndescription: \"what " + name + " does\""
	if extraFM != "" {
		fm += "\n" + extraFM
	}
	return "---\n" + fm + "\n---\n\n" + body
}

func readSkillFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func skillExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestTransformSkillForKimi(t *testing.T) {
	t.Run("a valid skill keeps its front matter, including keys Kimi ignores", func(t *testing.T) {
		src := kimiSkill("graphify", "trigger: /graphify\nargument-hint: \"[ticket]\"", "# Body\n")
		got, err := transformSkillForKimi(src, "graphify", kimiAgentsDir)
		if err != nil {
			t.Fatalf("transformSkillForKimi() error = %v", err)
		}
		for _, want := range []string{"trigger: /graphify", `argument-hint: "[ticket]"`, "# Body"} {
			if !strings.Contains(got, want) {
				t.Errorf("output lost %q:\n%s", want, got)
			}
		}
	})

	t.Run("references to Claude Code's agents are repointed, and shared memory is not", func(t *testing.T) {
		body := "Read `~/.claude/agents/gen-docs.md` and follow it.\nAtlas at `~/.claude/agent-memory/codebase-navigator/`.\n"
		got, err := transformSkillForKimi(kimiSkill("devxp", "", body), "devxp", kimiAgentsDir)
		if err != nil {
			t.Fatalf("transformSkillForKimi() error = %v", err)
		}
		if !strings.Contains(got, kimiAgentsDir+"/gen-docs.md") {
			t.Errorf("the agent reference was not repointed:\n%s", got)
		}
		if !strings.Contains(got, "~/.claude/agent-memory/codebase-navigator/") {
			t.Errorf("the shared memory path was rewritten; both CLIs use one atlas:\n%s", got)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		tests := map[string]struct{ src, wantErr string }{
			"no front matter": {"# Just a body\n", "no front matter"},
			// The exact shape three shipped skills had: an unquoted description
			// containing ": ", which Claude Code accepts and Kimi does not.
			"an unquoted description holding a colon-space": {
				"---\nname: release\ndescription: Release phase: from merge to shipped.\n---\n\nbody\n", "not valid YAML",
			},
			"no description":                         {"---\nname: a\n---\n\nbody\n", "no description"},
			"no name":                                {"---\ndescription: \"d\"\n---\n\nbody\n", "no name"},
			"name that does not match the directory": {kimiSkill("other", "", "body\n"), "does not match the directory"},
			"an unsupported type":                    {kimiSkill("a", "type: agent", "body\n"), "not one Kimi supports"},
			"empty body":                             {"---\nname: a\ndescription: \"d\"\n---\n\n\n", "body is empty"},
		}
		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				_, err := transformSkillForKimi(tt.src, "a", kimiAgentsDir)
				if err == nil {
					t.Fatalf("transformSkillForKimi() error = nil, want one mentioning %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("transformSkillForKimi() error = %v, want it to mention %q", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("a supported type is accepted", func(t *testing.T) {
		for _, typ := range []string{"prompt", "inline", "flow"} {
			if _, err := transformSkillForKimi(kimiSkill("a", "type: "+typ, "body\n"), "a", kimiAgentsDir); err != nil {
				t.Errorf("type %q: error = %v, want nil", typ, err)
			}
		}
	})
}

func TestInstallKimi(t *testing.T) {
	writeSrc := func(t *testing.T) string {
		t.Helper()
		src := t.TempDir()
		files := map[string]string{
			"graphify/SKILL.md":            kimiSkill("graphify", "", "Read `~/.claude/agents/gen-docs.md`.\n"),
			"graphify/references/query.md": "# Query reference\n",
			"graphify/references/hooks.md": "# Hooks reference\n",
			"devxp/SKILL.md":               kimiSkill("devxp", "", "# devxp\n"),
			"not-a-skill/notes.md":         "# no SKILL.md here\n",
		}
		for rel, content := range files {
			p := filepath.Join(src, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
		return src
	}

	t.Run("installs each skill with its supporting files and a transformed SKILL.md", func(t *testing.T) {
		src, target := writeSrc(t), filepath.Join(t.TempDir(), "skills")
		var got []string
		var err error
		out := captureStdout(t, func() { got, err = InstallKimi(src, target, kimiAgentsDir, nil, false) })
		if err != nil {
			t.Fatalf("InstallKimi() error = %v\n%s", err, out)
		}
		sort.Strings(got)
		if want := []string{"devxp", "graphify"}; !reflect.DeepEqual(got, want) {
			t.Errorf("InstallKimi() = %v, want %v — a directory without SKILL.md is not a skill", got, want)
		}
		for _, rel := range []string{"graphify/references/query.md", "graphify/references/hooks.md"} {
			if !skillExists(filepath.Join(target, filepath.FromSlash(rel))) {
				t.Errorf("%s was not copied; Kimi does not scan it as a skill but the skill still needs it", rel)
			}
		}
		installed := readSkillFile(t, filepath.Join(target, "graphify", "SKILL.md"))
		if !strings.Contains(installed, kimiAgentsDir+"/gen-docs.md") {
			t.Errorf("SKILL.md was copied without being transformed:\n%s", installed)
		}
	})

	t.Run("a disabled skill is neither written nor returned", func(t *testing.T) {
		src, target := writeSrc(t), filepath.Join(t.TempDir(), "skills")
		var got []string
		out := captureStdout(t, func() { got, _ = InstallKimi(src, target, kimiAgentsDir, []string{"graphify"}, false) })
		if want := []string{"devxp"}; !reflect.DeepEqual(got, want) {
			t.Errorf("InstallKimi() = %v, want %v", got, want)
		}
		if !strings.Contains(out, "[skip] graphify") {
			t.Errorf("no skip notice:\n%s", out)
		}
		if skillExists(filepath.Join(target, "graphify")) {
			t.Error("a disabled skill was written")
		}
	})

	t.Run("a dry run writes nothing and still reports what it would write", func(t *testing.T) {
		src, target := writeSrc(t), filepath.Join(t.TempDir(), "skills")
		var got []string
		out := captureStdout(t, func() { got, _ = InstallKimi(src, target, kimiAgentsDir, nil, true) })
		if len(got) != 2 {
			t.Errorf("InstallKimi() = %v, want both skills so the manifest still tracks them", got)
		}
		if skillExists(target) {
			t.Error("a dry run created the target directory")
		}
		for _, name := range []string{"graphify", "devxp"} {
			if !strings.Contains(out, filepath.Join(target, name)) {
				t.Errorf("the dry run does not name %s:\n%s", name, out)
			}
		}
	})

	t.Run("a skill Kimi would refuse warns, installs nothing of itself, and stops nothing else", func(t *testing.T) {
		src, target := writeSrc(t), filepath.Join(t.TempDir(), "skills")
		bad := filepath.Join(src, "broken")
		os.MkdirAll(bad, 0755)                                                                                       //nolint:errcheck
		os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("---\nname: broken\ndescription: A: b\n---\nx\n"), 0644) //nolint:errcheck
		os.WriteFile(filepath.Join(bad, "extra.md"), []byte("# extra\n"), 0644)                                      //nolint:errcheck

		var got []string
		out := captureStdout(t, func() { got, _ = InstallKimi(src, target, kimiAgentsDir, nil, false) })
		sort.Strings(got)
		if want := []string{"devxp", "graphify"}; !reflect.DeepEqual(got, want) {
			t.Errorf("InstallKimi() = %v, want %v", got, want)
		}
		if !strings.Contains(out, "skill broken:") {
			t.Errorf("no warning naming the refused skill:\n%s", out)
		}
		// Validated before anything is copied, so not even a partial directory.
		if skillExists(filepath.Join(target, "broken")) {
			t.Error("a refused skill left a directory behind")
		}
	})

	t.Run("a second run writes the same bytes", func(t *testing.T) {
		src, target := writeSrc(t), filepath.Join(t.TempDir(), "skills")
		captureStdout(t, func() { InstallKimi(src, target, kimiAgentsDir, nil, false) }) //nolint:errcheck
		first := readSkillFile(t, filepath.Join(target, "graphify", "SKILL.md"))
		captureStdout(t, func() { InstallKimi(src, target, kimiAgentsDir, nil, false) }) //nolint:errcheck
		if second := readSkillFile(t, filepath.Join(target, "graphify", "SKILL.md")); second != first {
			t.Errorf("a re-install changed the file:\n%s\n%s", first, second)
		}
	})

	// #124: install never writes through a symlinked skill directory, nor
	// through a symlinked file inside a real one. The Kimi install substitutes
	// SKILL.md through copyDir rather than overwriting it after the copy, which
	// is the only way the second case holds.
	t.Run("symlinked destinations are left untouched", func(t *testing.T) {
		for _, dryRun := range []bool{false, true} {
			for _, what := range []string{"the skill directory", "SKILL.md inside it"} {
				t.Run(what, func(t *testing.T) {
					src, target, dotfiles := writeSrc(t), filepath.Join(t.TempDir(), "skills"), t.TempDir()
					mine := filepath.Join(dotfiles, "mine")
					os.MkdirAll(mine, 0755)                                                 //nolint:errcheck
					os.WriteFile(filepath.Join(mine, "SKILL.md"), []byte("my own\n"), 0644) //nolint:errcheck
					os.MkdirAll(target, 0755)                                               //nolint:errcheck

					var link string
					if what == "the skill directory" {
						link = filepath.Join(target, "graphify")
						if err := os.Symlink(mine, link); err != nil {
							t.Fatal(err)
						}
					} else {
						os.MkdirAll(filepath.Join(target, "graphify"), 0755) //nolint:errcheck
						link = filepath.Join(target, "graphify", "SKILL.md")
						if err := os.Symlink(filepath.Join(mine, "SKILL.md"), link); err != nil {
							t.Fatal(err)
						}
					}

					var got []string
					out := captureStdout(t, func() { got, _ = InstallKimi(src, target, kimiAgentsDir, nil, dryRun) })

					if !strings.Contains(out, "is a symlink, so it was left untouched") {
						t.Errorf("no symlink warning:\n%s", out)
					}
					// Warned about, and not also reported as written. The
					// added-line marker is matched rather than the bare name,
					// because the warning itself names the path. Without this
					// the run says it installed a file it deliberately did not.
					if strings.Contains(out, ui.AddedLine("graphify/SKILL.md")) {
						t.Errorf("a symlinked destination was also reported as written:\n%s", out)
					}
					// The name stays in the install set so the manifest keeps
					// tracking it and it is never reported as stale.
					sort.Strings(got)
					if want := []string{"devxp", "graphify"}; !reflect.DeepEqual(got, want) {
						t.Errorf("InstallKimi() = %v, want %v", got, want)
					}
					if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
						t.Errorf("%s was replaced (err = %v)", what, err)
					}
					if got := readSkillFile(t, filepath.Join(mine, "SKILL.md")); got != "my own\n" {
						t.Errorf("the link target was written through: %q", got)
					}
				})
			}
		}
	})
}

// TestRepoSkillsKimi runs the real skills/ tree through the transform. It is
// also what proves the shipped sources are valid for Kimi at all.
func TestRepoSkillsKimi(t *testing.T) {
	src := filepath.Join("..", "..", "..", "skills")
	target := filepath.Join(t.TempDir(), "skills")
	var installed []string
	var err error
	out := captureStdout(t, func() { installed, err = InstallKimi(src, target, kimiAgentsDir, nil, false) })
	if err != nil {
		t.Fatalf("InstallKimi() error = %v\n%s", err, out)
	}
	if strings.Contains(out, "Kimi would skip it") {
		t.Errorf("a shipped skill was refused:\n%s", out)
	}
	repo := repoSkillFiles(t)
	if len(installed) != len(repo) {
		t.Errorf("installed %d skills, want all %d shipped ones: %v", len(installed), len(repo), installed)
	}
	for _, name := range installed {
		body := readSkillFile(t, filepath.Join(target, name, "SKILL.md"))
		if strings.Contains(body, "~/.claude/agents/") {
			t.Errorf("%s still points at Claude Code's agent directory", name)
		}
	}
	// graphify's supporting files reach Claude Code and Kimi but not opencode,
	// so this is the only place the Kimi copy is checked for them.
	refs, err := os.ReadDir(filepath.Join(target, "graphify", "references"))
	if err != nil {
		t.Fatalf("graphify references were not copied: %v", err)
	}
	if len(refs) == 0 {
		t.Error("graphify/references/ is empty")
	}
	// None of the supporting files needs the rewrite today; if one gains a
	// Claude-only agent path it must be handled rather than shipped stale.
	for _, e := range refs {
		if strings.Contains(readSkillFile(t, filepath.Join(target, "graphify", "references", e.Name())), "~/.claude/agents/") {
			t.Errorf("graphify/references/%s references a Claude Code agent path, which is only rewritten in SKILL.md", e.Name())
		}
	}
}
