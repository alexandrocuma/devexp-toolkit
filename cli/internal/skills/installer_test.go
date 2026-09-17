package skills

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

// TestInstallClaude_SymlinkInsideRealSkillDir (#157 review): a symlinked
// SKILL.md or subdirectory inside a real skill directory is previewed in a dry
// run, and a real run doesn't report a skipped SKILL.md as added.
func TestInstallClaude_SymlinkInsideRealSkillDir(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		t.Run(fmt.Sprintf("dryRun=%v", dryRun), func(t *testing.T) {
			src, target, outside := t.TempDir(), t.TempDir(), t.TempDir()
			writeSkillDir(t, src, "beta", map[string]string{"SKILL.md": sampleSkillMD, "references/notes.md": "new\n"})
			writeSkillDir(t, src, "gamma", map[string]string{"SKILL.md": sampleSkillMD})
			os.WriteFile(filepath.Join(outside, "mine.md"), []byte("mine\n"), 0644)                  //nolint:errcheck
			os.MkdirAll(filepath.Join(outside, "refs"), 0755)                                        //nolint:errcheck
			os.MkdirAll(filepath.Join(target, "beta"), 0755)                                         //nolint:errcheck
			os.Symlink(filepath.Join(outside, "mine.md"), filepath.Join(target, "beta", "SKILL.md")) //nolint:errcheck
			os.Symlink(filepath.Join(outside, "refs"), filepath.Join(target, "beta", "references"))  //nolint:errcheck
			var err error
			out := captureStdout(t, func() { _, err = InstallClaude(src, target, nil, dryRun) })
			if err != nil {
				t.Fatalf("InstallClaude() error = %v\n%s", err, out)
			}
			for _, p := range []string{"beta/SKILL.md", "beta/references"} {
				if !strings.Contains(out, fmt.Sprintf("%q is a symlink, so it was left untouched", filepath.Join(target, p))) {
					t.Errorf("no warning for %s:\n%s", p, out)
				}
			}
			if strings.Contains(out, "beta/SKILL.md\n") {
				t.Errorf("the skipped beta/SKILL.md is reported as added:\n%s", out)
			}
			if !dryRun && !strings.Contains(out, "gamma/SKILL.md\n") {
				t.Errorf("gamma/SKILL.md not reported as added:\n%s", out)
			}
			if got, _ := os.ReadFile(filepath.Join(outside, "mine.md")); string(got) != "mine\n" {
				t.Errorf("written through the SKILL.md link: %q", got)
			}
			if entries, _ := os.ReadDir(filepath.Join(outside, "refs")); len(entries) != 0 {
				t.Errorf("written into the symlinked references/")
			}
			if _, statErr := os.Stat(filepath.Join(target, "gamma")); dryRun != os.IsNotExist(statErr) {
				t.Errorf("gamma on disk = %v with dryRun=%v", statErr == nil, dryRun)
			}
		})
	}
}

// TestInstall_SkillFilesNeverWrittenInPlace (#157 review): skill and command
// files are replaced through a temp file and a rename. In a directory where no
// temp file can be created, the write fails and the old bytes stay, where an
// in-place os.WriteFile would have succeeded.
func TestInstall_SkillFilesNeverWrittenInPlace(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	readOnly := func(t *testing.T, dir string) {
		t.Helper()
		os.Chmod(dir, 0555)                       //nolint:errcheck
		t.Cleanup(func() { os.Chmod(dir, 0755) }) //nolint:errcheck
	}
	t.Run("CopyDir", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("new\n"), 0644) //nolint:errcheck
		os.WriteFile(filepath.Join(dst, "SKILL.md"), []byte("old\n"), 0644) //nolint:errcheck
		readOnly(t, dst)
		if err := CopyDir(src, dst); err == nil {
			t.Errorf("CopyDir() = nil, want the temp-file error")
		}
		if got, _ := os.ReadFile(filepath.Join(dst, "SKILL.md")); string(got) != "old\n" {
			t.Errorf("SKILL.md = %q, written in place", got)
		}
	})
	t.Run("InstallOpencode", func(t *testing.T) {
		src, target := t.TempDir(), t.TempDir()
		writeSkillDir(t, src, "alpha", map[string]string{"SKILL.md": sampleSkillMD})
		os.WriteFile(filepath.Join(target, "alpha.md"), []byte("old\n"), 0644) //nolint:errcheck
		readOnly(t, target)
		var err error
		captureStdout(t, func() { _, err = InstallOpencode(src, target, nil, false) })
		if got, _ := os.ReadFile(filepath.Join(target, "alpha.md")); err == nil || string(got) != "old\n" {
			t.Errorf("InstallOpencode() error = %v, alpha.md = %q; want an error and the old bytes", err, got)
		}
	})
}

// TestInstallClaude_UnreadableSkillFailsInDryRunToo: the dry-run walk reports
// what a real run would hit, such as a source directory it can't read.
func TestInstallClaude_UnreadableSkillFailsInDryRunToo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	for _, dryRun := range []bool{true, false} {
		t.Run(fmt.Sprintf("dryRun=%v", dryRun), func(t *testing.T) {
			src, target := t.TempDir(), t.TempDir()
			writeSkillDir(t, src, "alpha", map[string]string{"SKILL.md": sampleSkillMD, "references/notes.md": "x\n"})
			refs := filepath.Join(src, "alpha", "references")
			os.Chmod(refs, 0o000)                       //nolint:errcheck
			t.Cleanup(func() { os.Chmod(refs, 0o755) }) //nolint:errcheck
			var err error
			captureStdout(t, func() { _, err = InstallClaude(src, target, nil, dryRun) })
			if err == nil {
				t.Errorf("InstallClaude(dryRun=%v) = nil, want the unreadable-directory error", dryRun)
			}
		})
	}
}
