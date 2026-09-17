package skills

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const sampleSkillMD = `---
name: alpha
description: "A sample skill"
---

# Alpha Skill
`

// writeSkillDir creates srcDir/name/ and writes each file (keyed by relative
// path within the skill dir) with its content.
func writeSkillDir(t *testing.T, srcDir, name string, files map[string]string) {
	t.Helper()
	skillDir := filepath.Join(srcDir, name)
	for rel, content := range files {
		path := filepath.Join(skillDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
}

func TestInstallClaude(t *testing.T) {
	tests := map[string]struct {
		skills        map[string]map[string]string // skill dirname -> relative file -> content
		disabled      []string
		dryRun        bool
		wantInstalled []string
		wantNestedRef bool
	}{
		"installs skill dirs containing SKILL.md, skips dirs without it": {
			skills: map[string]map[string]string{
				"alpha": {
					"SKILL.md":            sampleSkillMD,
					"references/notes.md": "# Notes\n",
				},
				"beta": {
					"SKILL.md": sampleSkillMD,
				},
				"not-a-skill": {
					"notes.txt": "no SKILL.md here",
				},
			},
			wantInstalled: []string{"alpha", "beta"},
			wantNestedRef: true,
		},
		"skips disabled skills and excludes from installed list": {
			skills: map[string]map[string]string{
				"alpha": {"SKILL.md": sampleSkillMD},
				"beta":  {"SKILL.md": sampleSkillMD},
			},
			disabled:      []string{"beta"},
			wantInstalled: []string{"alpha"},
		},
		"dry run returns would-be list without writing files": {
			skills: map[string]map[string]string{
				"alpha": {"SKILL.md": sampleSkillMD},
			},
			dryRun:        true,
			wantInstalled: []string{"alpha"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srcDir := t.TempDir()
			targetDir := filepath.Join(t.TempDir(), "target")
			for skillName, files := range tt.skills {
				writeSkillDir(t, srcDir, skillName, files)
			}

			got, err := InstallClaude(srcDir, targetDir, tt.disabled, tt.dryRun)
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

			if tt.wantNestedRef {
				data, err := os.ReadFile(filepath.Join(targetDir, "alpha", "references", "notes.md"))
				if err != nil {
					t.Fatalf("nested reference file not copied: %v", err)
				}
				if string(data) != "# Notes\n" {
					t.Errorf("nested reference file content = %q, want %q", data, "# Notes\n")
				}
			}
		})
	}
}

func TestInstallOpencode(t *testing.T) {
	// A skill whose body legitimately contains `name:` lines: a fenced YAML
	// example and a table row. Both used to vanish from the opencode copy.
	bodyNameMD := "---\n" +
		"name: alpha\n" +
		"description: \"A sample skill\"\n" +
		"---\n\n" +
		"# Alpha Skill\n\n" +
		"```yaml\n" +
		"agent:\n" +
		"  name: reviewer\n" +
		"```\n\n" +
		"| key | meaning |\n" +
		"|---|---|\n" +
		"| name: the agent id | required |\n"

	// An indented `name:` inside the front matter is a nested key belonging to
	// another value, not the skill's own name.
	nestedNameMD := "---\n" +
		"name: alpha\n" +
		"contract:\n" +
		"  name: inner\n" +
		"---\n\n" +
		"# Alpha Skill\n"

	noFrontMatterMD := "# Alpha Skill\n\nname: not metadata\n"

	unterminatedMD := "---\n" +
		"name: alpha\n" +
		"description: never closed\n\n" +
		"# Alpha Skill\n"

	tests := map[string]struct {
		skills          map[string]map[string]string
		disabled        []string
		dryRun          bool
		wantInstalled   []string
		wantContains    []string
		wantNotContains []string
	}{
		"strips the top-level name: from front matter": {
			skills:          map[string]map[string]string{"alpha": {"SKILL.md": sampleSkillMD}},
			wantInstalled:   []string{"alpha"},
			wantNotContains: []string{"name: alpha"},
			wantContains:    []string{"description: \"A sample skill\"", "# Alpha Skill"},
		},
		"keeps name: lines in the body": {
			skills:          map[string]map[string]string{"alpha": {"SKILL.md": bodyNameMD}},
			wantInstalled:   []string{"alpha"},
			wantNotContains: []string{"name: alpha"},
			wantContains:    []string{"  name: reviewer", "| name: the agent id | required |"},
		},
		"keeps an indented name: nested inside front matter": {
			skills:          map[string]map[string]string{"alpha": {"SKILL.md": nestedNameMD}},
			wantInstalled:   []string{"alpha"},
			wantNotContains: []string{"name: alpha"},
			wantContains:    []string{"  name: inner"},
		},
		"copies a skill without front matter unchanged": {
			skills:        map[string]map[string]string{"alpha": {"SKILL.md": noFrontMatterMD}},
			wantInstalled: []string{"alpha"},
			wantContains:  []string{"name: not metadata"},
		},
		"leaves unterminated front matter untouched": {
			skills:        map[string]map[string]string{"alpha": {"SKILL.md": unterminatedMD}},
			wantInstalled: []string{"alpha"},
			wantContains:  []string{"name: alpha"},
		},
		"skips disabled skills and excludes from installed list": {
			skills: map[string]map[string]string{
				"alpha": {"SKILL.md": sampleSkillMD},
				"beta":  {"SKILL.md": sampleSkillMD},
			},
			disabled:      []string{"beta"},
			wantInstalled: []string{"alpha"},
		},
		"dry run returns would-be list without writing files": {
			skills:        map[string]map[string]string{"alpha": {"SKILL.md": sampleSkillMD}},
			dryRun:        true,
			wantInstalled: []string{"alpha"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srcDir := t.TempDir()
			targetDir := filepath.Join(t.TempDir(), "target")
			for skillName, files := range tt.skills {
				writeSkillDir(t, srcDir, skillName, files)
			}

			got, err := InstallOpencode(srcDir, targetDir, tt.disabled, tt.dryRun)
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
				return
			}

			if len(tt.wantContains) == 0 && len(tt.wantNotContains) == 0 {
				return
			}
			data, err := os.ReadFile(filepath.Join(targetDir, "alpha.md"))
			if err != nil {
				t.Fatalf("ReadFile(alpha.md) error = %v", err)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(string(data), want) {
					t.Errorf("installed file is missing %q:\n%s", want, data)
				}
			}
			for _, unwanted := range tt.wantNotContains {
				if strings.Contains(string(data), unwanted) {
					t.Errorf("installed file should not contain %q:\n%s", unwanted, data)
				}
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

// TestInstall_SymlinkedSkillEntries (#124): a skill directory, a file or a
// subdirectory inside one, or an opencode command that is a symlink is never
// written through or replaced; everything else is still installed.
func TestInstall_SymlinkedSkillEntries(t *testing.T) {
	isLink := func(p string) bool {
		fi, err := os.Lstat(p)
		return err == nil && fi.Mode()&os.ModeSymlink != 0
	}
	setup := func(t *testing.T) (src, target, outside string) {
		src, target, outside = t.TempDir(), t.TempDir(), t.TempDir()
		for _, name := range []string{"alpha", "beta", "gamma"} {
			writeSkillDir(t, src, name, map[string]string{"SKILL.md": sampleSkillMD, "references/notes.md": "new notes\n"})
		}
		os.WriteFile(filepath.Join(outside, "mine.md"), []byte("mine\n"), 0644)               //nolint:errcheck
		os.MkdirAll(filepath.Join(outside, "refs"), 0755)                                     //nolint:errcheck
		os.WriteFile(filepath.Join(outside, "refs", "notes.md"), []byte("my notes\n"), 0644)  //nolint:errcheck
		os.MkdirAll(filepath.Join(outside, "skill"), 0755)                                    //nolint:errcheck
		os.WriteFile(filepath.Join(outside, "skill", "SKILL.md"), []byte("my skill\n"), 0644) //nolint:errcheck
		return src, target, outside
	}
	unchanged := func(t *testing.T, outside string) {
		t.Helper()
		for p, want := range map[string]string{"mine.md": "mine\n", "refs/notes.md": "my notes\n", "skill/SKILL.md": "my skill\n"} {
			if got, _ := os.ReadFile(filepath.Join(outside, p)); string(got) != want {
				t.Errorf("%s = %q, written through a symlink", p, got)
			}
		}
		if entries, _ := os.ReadDir(filepath.Join(outside, "skill")); len(entries) != 1 {
			t.Errorf("files written into a symlinked skill directory: %d entries", len(entries))
		}
	}

	t.Run("InstallClaude", func(t *testing.T) {
		src, target, outside := setup(t)
		os.Symlink(filepath.Join(outside, "skill"), filepath.Join(target, "alpha"))              //nolint:errcheck
		os.MkdirAll(filepath.Join(target, "beta"), 0755)                                         //nolint:errcheck
		os.Symlink(filepath.Join(outside, "mine.md"), filepath.Join(target, "beta", "SKILL.md")) //nolint:errcheck
		os.Symlink(filepath.Join(outside, "refs"), filepath.Join(target, "beta", "references"))  //nolint:errcheck
		var installed []string
		var err error
		out := captureStdout(t, func() { installed, err = InstallClaude(src, target, nil, false) })
		if err != nil {
			t.Fatalf("InstallClaude() error = %v\n%s", err, out)
		}
		sort.Strings(installed)
		if want := []string{"alpha", "beta", "gamma"}; !reflect.DeepEqual(installed, want) {
			t.Errorf("installed = %v, want %v", installed, want)
		}
		unchanged(t, outside)
		for _, p := range []string{"alpha", "beta/SKILL.md", "beta/references"} {
			if !isLink(filepath.Join(target, p)) {
				t.Errorf("%s is no longer a symlink", p)
			}
		}
		if got, _ := os.ReadFile(filepath.Join(target, "gamma", "references", "notes.md")); string(got) != "new notes\n" {
			t.Errorf("gamma not installed: %q", got)
		}
		if n := strings.Count(out, "is a symlink, so it was left untouched"); n != 3 {
			t.Errorf("%d symlink warnings, want 3:\n%s", n, out)
		}
	})

	t.Run("InstallClaude dry run", func(t *testing.T) {
		src, target, outside := setup(t)
		os.Symlink(filepath.Join(outside, "skill"), filepath.Join(target, "alpha")) //nolint:errcheck
		out := captureStdout(t, func() { InstallClaude(src, target, nil, true) })   //nolint:errcheck
		if !strings.Contains(out, "is a symlink, so it was left untouched") || strings.Contains(out, "alpha/\n") {
			t.Errorf("dry run doesn't preview the symlinked skill as kept:\n%s", out)
		}
		unchanged(t, outside)
	})

	t.Run("InstallOpencode", func(t *testing.T) {
		src, target, outside := setup(t)
		os.Symlink(filepath.Join(outside, "mine.md"), filepath.Join(target, "alpha.md"))   //nolint:errcheck
		os.Symlink(filepath.Join(outside, "missing.md"), filepath.Join(target, "beta.md")) //nolint:errcheck
		var installed []string
		var err error
		out := captureStdout(t, func() { installed, err = InstallOpencode(src, target, nil, false) })
		if err != nil {
			t.Fatalf("InstallOpencode() error = %v\n%s", err, out)
		}
		sort.Strings(installed)
		if want := []string{"alpha", "beta", "gamma"}; !reflect.DeepEqual(installed, want) {
			t.Errorf("installed = %v, want %v", installed, want)
		}
		unchanged(t, outside)
		if _, err := os.Lstat(filepath.Join(outside, "missing.md")); !os.IsNotExist(err) {
			t.Errorf("a file was created at a dangling link's destination")
		}
		if !isLink(filepath.Join(target, "alpha.md")) || !isLink(filepath.Join(target, "beta.md")) {
			t.Errorf("symlinked command replaced")
		}
		if got, _ := os.ReadFile(filepath.Join(target, "gamma.md")); !strings.Contains(string(got), "# Alpha Skill") {
			t.Errorf("gamma.md not installed: %q", got)
		}
	})
}
