package skills

import (
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
	tests := map[string]struct {
		skills        map[string]map[string]string
		disabled      []string
		dryRun        bool
		wantInstalled []string
	}{
		"writes <name>.md stripping name: frontmatter line": {
			skills: map[string]map[string]string{
				"alpha": {"SKILL.md": sampleSkillMD},
			},
			wantInstalled: []string{"alpha"},
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

			if _, ok := tt.skills["alpha"]; ok {
				data, err := os.ReadFile(filepath.Join(targetDir, "alpha.md"))
				if err != nil {
					t.Fatalf("ReadFile(alpha.md) error = %v", err)
				}
				if strings.Contains(string(data), "name:") {
					t.Errorf("installed file should not contain name: line:\n%s", data)
				}
			}
		})
	}
}
