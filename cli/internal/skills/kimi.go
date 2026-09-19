package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"devexp/internal/fsutil"
	"devexp/internal/ui"
)

// ── Kimi Code CLI ─────────────────────────────────────────────────────────────
//
// A Kimi skill is a directory holding SKILL.md, the same shape Claude Code
// uses — no index, no manifest, and supporting subdirectories such as
// graphify's references/ are copied along without being mistaken for skills of
// their own. So the install is InstallClaude's copy with SKILL.md rewritten on
// the way through.
//
// What Kimi requires of that file, measured against the 2.0.1 binary:
//
//   - The front matter is strict YAML, and for a directory skill both `name`
//     and `description` are required. A file that fails either is dropped from
//     the catalog with a line in Kimi's own log.
//   - `type`, if present, must be prompt, inline or flow. Any other value
//     drops the skill with no warning at all, anywhere.
//   - Unknown keys (devexp ships `trigger` and `argument-hint`) are ignored.
//
// devexp keeps its skill sources valid for all of that, and a repo-level test
// holds them to it (frontmatter_repo_test.go). This validates anyway rather
// than trusting it, because a skill that silently does not exist is the one
// failure a user cannot diagnose.

// kimiSkillTypes are the only values Kimi accepts for a skill's `type`.
var kimiSkillTypes = map[string]bool{"prompt": true, "inline": true, "flow": true}

// kimiClaudeAgentRefRe matches a reference to an installed Claude Code agent
// file. Duplicated from internal/agents rather than shared: internal packages
// do not import one another (docs/development/conventions.md), the same reason
// isDisabled is duplicated there.
var kimiClaudeAgentRefRe = regexp.MustCompile(`~/\.claude/agents/([a-z0-9][a-z0-9-]*\.md)`)

// splitFrontmatter applies Kimi's front-matter rule: the first line must trim
// to `---`, and the first later line that trims to `---` closes the block.
func splitFrontmatter(content string) (fm, body string, ok bool) {
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

// transformSkillForKimi validates a SKILL.md the way Kimi does and repoints
// its references to Claude Code's installed agents at the Kimi copies.
//
// It returns an error rather than guessing a repair: a skill devexp cannot
// make valid is one Kimi would skip, and saying so at install time is the only
// way anyone finds out.
func transformSkillForKimi(content, name, agentsDir string) (string, error) {
	fm, body, ok := splitFrontmatter(content)
	if !ok {
		return "", fmt.Errorf("no front matter block — Kimi requires one, with a name and a description")
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(fm), &parsed); err != nil {
		return "", fmt.Errorf("front matter is not valid YAML: %w", err)
	}
	if got, _ := parsed["name"].(string); strings.TrimSpace(got) == "" {
		return "", fmt.Errorf("no name — Kimi requires one for a directory skill")
	} else if got != name {
		return "", fmt.Errorf("name %q does not match the directory %q", got, name)
	}
	if desc, _ := parsed["description"].(string); strings.TrimSpace(desc) == "" {
		return "", fmt.Errorf("no description — Kimi requires one for a directory skill")
	}
	if typ, present := parsed["type"]; present {
		s, _ := typ.(string)
		if !kimiSkillTypes[s] {
			return "", fmt.Errorf("type %q is not one Kimi supports (prompt, inline or flow), and it would drop the skill without saying so", typ)
		}
	}
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("body is empty")
	}
	// The front matter is copied byte for byte — Kimi already parses it, and
	// re-encoding would churn quoting for no gain. Only the body is rewritten.
	return "---\n" + fm + "\n---\n" + kimiClaudeAgentRefRe.ReplaceAllString(body, filepath.ToSlash(agentsDir)+"/$1"), nil
}

// InstallKimi copies each skill directory into targetDir/<name>/ with its
// SKILL.md rewritten for Kimi Code CLI. agentsDir is the absolute directory
// the agents were installed to, which body references to Claude Code's copies
// are repointed at. Returns the installed skill dirnames (including in dryRun
// mode), so callers can diff against a manifest to detect stale skills.
func InstallKimi(srcDir, targetDir, agentsDir string, disabled []string, dryRun bool) ([]string, error) {
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
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}
		if isDisabled(name, disabled) {
			ui.Skipped(name, "disabled in devexp.config.json")
			continue
		}
		content, err := os.ReadFile(skillFile)
		if err != nil {
			return installed, err
		}
		// Transformed before anything is copied, so a skill Kimi would refuse
		// never leaves a half-written directory behind.
		transformed, err := transformSkillForKimi(string(content), name, agentsDir)
		if err != nil {
			ui.Warn(fmt.Sprintf("skill %s: %v — Kimi would skip it, so it was not installed", name, err))
			continue
		}

		destDir := filepath.Join(targetDir, name)
		if fsutil.IsSymlink(destDir) {
			warnSymlinked(destDir)
			installed = append(installed, name)
			continue
		}
		subst := func(rel string, original []byte) ([]byte, error) {
			if rel == "SKILL.md" {
				return []byte(transformed), nil
			}
			return original, nil
		}
		kept, err := copyDir(skillDir, destDir, dryRun, subst)
		if err != nil {
			// copyDir creates the destination before it writes into it, so a
			// failure part-way leaves a directory that is devexp's. The name
			// goes back with the error, or the caller records a tree it owns
			// but cannot account for. Checked rather than assumed: a failure
			// in MkdirAll itself leaves nothing behind.
			if _, statErr := os.Lstat(destDir); statErr == nil {
				installed = append(installed, name)
			}
			return installed, err
		}
		if dryRun {
			ui.DryRun(fmt.Sprintf("transform + write %s/", destDir))
		} else if !slices.Contains(kept, filepath.Join(destDir, "SKILL.md")) {
			ui.Added(fmt.Sprintf("%s/SKILL.md", name))
		}
		installed = append(installed, name)
	}
	return installed, nil
}
