package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devexp/internal/ui"
)

// InstallClaude copies each skill directory (SKILL.md plus any supplementary
// files, e.g. references/) into ~/.claude/skills/<name>/. Returns the
// installed skill dirnames (including in dryRun mode), so callers can diff
// against a manifest to detect stale skills from prior runs.
func InstallClaude(srcDir, targetDir string, disabled []string, dryRun bool) ([]string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, err
	}
	var installed []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		skillDir := filepath.Join(srcDir, name)
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			continue
		}
		if isDisabled(name, disabled) {
			ui.Skipped(name, "disabled in devexp.config.json")
			continue
		}
		destDir := filepath.Join(targetDir, name)
		if dryRun {
			ui.DryRun(fmt.Sprintf("write %s/", destDir))
			installed = append(installed, name)
			continue
		}
		if err := CopyDir(skillDir, destDir); err != nil {
			return installed, err
		}
		ui.Added(fmt.Sprintf("%s/SKILL.md", name))
		installed = append(installed, name)
	}
	return installed, nil
}

// CopyDir recursively copies all files and subdirectories from src to dst.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, content, 0644)
	})
}

// InstallOpencode copies skills as <name>.md into ~/.config/opencode/commands/
// (strips the `name:` frontmatter line, which opencode derives from the
// filename). Returns the installed skill dirnames, bare (no .md suffix), for
// consistency with InstallClaude's manifest entries — callers append .md when
// removing stale opencode skill files. Includes dryRun mode.
func InstallOpencode(srcDir, targetDir string, disabled []string, dryRun bool) ([]string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, err
	}
	var installed []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		skillFile := filepath.Join(srcDir, name, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}
		if isDisabled(name, disabled) {
			ui.Skipped(name, "disabled in devexp.config.json")
			continue
		}
		dest := filepath.Join(targetDir, name+".md")
		if dryRun {
			ui.DryRun(fmt.Sprintf("write %s", dest))
			installed = append(installed, name)
			continue
		}
		content, err := os.ReadFile(skillFile)
		if err != nil {
			return installed, err
		}
		// Strip `name:` line — opencode derives name from filename
		var lines []string
		for _, line := range strings.Split(string(content), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "name:") {
				lines = append(lines, line)
			}
		}
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return installed, err
		}
		if err := os.WriteFile(dest, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return installed, err
		}
		ui.Added(fmt.Sprintf("%s.md", name))
		installed = append(installed, name)
	}
	return installed, nil
}

func isDisabled(name string, disabled []string) bool {
	for _, d := range disabled {
		if d == name {
			return true
		}
	}
	return false
}
