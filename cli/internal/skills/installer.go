package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"devexp/internal/fsutil"
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
		if fsutil.IsSymlink(destDir) {
			warnSymlinked(destDir)
			installed = append(installed, name)
			continue
		}
		if dryRun {
			ui.DryRun(fmt.Sprintf("write %s/", destDir))
			// Preview the symlinks inside the skill a real run leaves alone.
			if _, err := copyDir(skillDir, destDir, true, nil); err != nil {
				return installed, err
			}
			installed = append(installed, name)
			continue
		}
		kept, err := copyDir(skillDir, destDir, false, nil)
		if err != nil {
			return installed, err
		}
		// A SKILL.md left untouched as a symlink was not added; its warning
		// already said so.
		if !slices.Contains(kept, filepath.Join(destDir, "SKILL.md")) {
			ui.Added(fmt.Sprintf("%s/SKILL.md", name))
		}
		installed = append(installed, name)
	}
	return installed, nil
}

// CopyDir recursively copies all files and subdirectories from src to dst.
// Each file is written atomically. A file or directory in dst that is a
// symlink is left untouched, with a warning, and nothing is written through it
// (#124): it may point at a user's own copy or at the toolkit's source.
func CopyDir(src, dst string) error {
	_, err := copyDir(src, dst, false, nil)
	return err
}

// copyDir is CopyDir, returning the destination paths it left untouched as
// symlinks. With dryRun it writes nothing and only warns about those paths, so
// a preview names what a real run keeps.
//
// transform, when non-nil, gets each file's slash-separated path relative to
// src and its contents, and returns what to write instead. It is how the Kimi
// install substitutes a rewritten SKILL.md: copying the directory and then
// overwriting that one file would write through it when it is a symlink, which
// is the whole thing #124 forbids.
//
// The symlink check and the write are separate steps: a link created in
// between is followed (fsutil.WriteFileAtomic replaces its target). Only a
// writer running as the same user can do that.
func copyDir(src, dst string, dryRun bool, transform func(rel string, content []byte) ([]byte, error)) (kept []string, err error) {
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(dst, rel)
		if fsutil.IsSymlink(dest) {
			warnSymlinked(dest)
			kept = append(kept, dest)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if dryRun {
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if transform != nil {
			content, err = transform(filepath.ToSlash(rel), content)
			if err != nil {
				return err
			}
		}
		return fsutil.WriteFileAtomic(dest, content, 0644)
	})
	return kept, err
}

// warnSymlinked reports a skill destination left untouched because it is a
// symlink.
func warnSymlinked(dest string) {
	ui.Warn(fmt.Sprintf("%q is a symlink, so it was left untouched — devexp never writes through or replaces a symlinked skill file or directory; replace the link to install this release's copy", dest))
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
		if fsutil.IsSymlink(dest) {
			warnSymlinked(dest)
			installed = append(installed, name)
			continue
		}
		if dryRun {
			ui.DryRun(fmt.Sprintf("write %s", dest))
			installed = append(installed, name)
			continue
		}
		content, err := os.ReadFile(skillFile)
		if err != nil {
			return installed, err
		}
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return installed, err
		}
		if err := fsutil.WriteFileAtomic(dest, []byte(stripFrontMatterName(string(content))), 0644); err != nil {
			return installed, err
		}
		ui.Added(fmt.Sprintf("%s.md", name))
		installed = append(installed, name)
	}
	return installed, nil
}

// stripFrontMatterName removes the top-level `name:` key from the leading
// front matter block, which opencode derives from the filename instead.
//
// Only that one key is removed. The body is copied verbatim: a `name:` inside
// a fenced example, a config snippet or a table row is content, not metadata,
// and dropping it silently corrupted the opencode copy of the skill. An
// indented `name:` is left alone even inside the front matter, since it is a
// key nested under something else rather than the skill's own name.
//
// Content without a properly delimited front matter block is returned
// unchanged — an unterminated `---` is malformed, and guessing where the
// metadata ends would risk deleting body lines.
func stripFrontMatterName(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content
	}
	closing := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closing = i
			break
		}
	}
	if closing == -1 {
		return content
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if i > 0 && i < closing && strings.HasPrefix(line, "name:") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func isDisabled(name string, disabled []string) bool {
	for _, d := range disabled {
		if d == name {
			return true
		}
	}
	return false
}
