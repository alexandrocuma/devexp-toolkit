package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"devexp/internal/ui"
)

var modelRe = regexp.MustCompile(`(?m)^model:.*$`)

var modelMap = map[string]string{
	"sonnet":      "anthropic/claude-sonnet-4-6",
	"opus":        "anthropic/claude-opus-4-6",
	"haiku":       "anthropic/claude-haiku-4-5-20251001",
	"gpt4":        "openai/gpt-4.1-2025-04-14",
	"gpt4o":       "openai/gpt-4o",
	"o3":          "openai/o3-2025-04-16",
	"o4mini":      "openai/o4-mini-2025-04-16",
	"deepseek":    "deepseek/deepseek-chat",
	"deepseek-r1": "deepseek/deepseek-reasoner",
	"kimi":        "moonshot/kimi-k2.5",
	"kimi-turbo":  "moonshot/kimi-k2-turbo-preview",
}

var skipFrontmatterKeys = map[string]bool{"name": true, "color": true, "memory": true}

var opencodeTools = map[string]bool{
	"read": true, "write": true, "edit": true, "bash": true,
	"glob": true, "grep": true, "webfetch": true, "websearch": true,
}

var claudeToOC = map[string]string{
	"read": "read", "write": "write", "edit": "edit", "bash": "bash",
	"glob": "glob", "grep": "grep", "webfetch": "webfetch", "websearch": "websearch",
}

func resolveModel(alias string) string {
	if resolved, ok := modelMap[alias]; ok {
		return resolved
	}
	return alias
}

// InstallClaude copies agent .md files into targetDir with an optional model override.
func InstallClaude(srcDir, targetDir, model string, disabled []string, dryRun bool) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := entry.Name()
		if name == "README.md" {
			continue
		}
		agentName := strings.TrimSuffix(name, ".md")
		if isDisabled(agentName, disabled) {
			ui.Skipped(name, "disabled in devexp.config.json")
			continue
		}
		dest := filepath.Join(targetDir, name)
		if dryRun {
			ui.DryRun(fmt.Sprintf("write %s", dest))
			count++
			continue
		}
		content, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			return count, err
		}
		if model != "" {
			resolved := resolveModel(model)
			content = modelRe.ReplaceAll(content, []byte("model: "+resolved))
		}
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return count, err
		}
		if err := os.WriteFile(dest, content, 0644); err != nil {
			return count, err
		}
		ui.Added(name)
		count++
	}
	return count, nil
}

// InstallOpencode transforms agent files for opencode and writes them to targetDir.
func InstallOpencode(srcDir, targetDir, model string, disabled []string, dryRun bool) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := entry.Name()
		if name == "README.md" {
			continue
		}
		agentName := strings.TrimSuffix(name, ".md")
		if isDisabled(agentName, disabled) {
			ui.Skipped(name, "disabled in devexp.config.json")
			continue
		}
		content, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			return count, err
		}
		transformed, err := transformForOpencode(string(content), model, false)
		if err != nil {
			ui.Warn(fmt.Sprintf("transform %s: %v", name, err))
			continue
		}
		dest := filepath.Join(targetDir, name)
		if dryRun {
			ui.DryRun(fmt.Sprintf("transform + write %s", dest))
			count++
			continue
		}
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return count, err
		}
		if err := os.WriteFile(dest, []byte(transformed), 0644); err != nil {
			return count, err
		}
		ui.Added(name)
		count++
	}
	return count, nil
}

// InstallOpencodeExclusive copies opencode-exclusive agents (agents/opencode/) with model-only substitution.
func InstallOpencodeExclusive(srcDir, targetDir, model string, dryRun bool) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := entry.Name()
		content, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			return count, err
		}
		transformed, err := transformForOpencode(string(content), model, true)
		if err != nil {
			ui.Warn(fmt.Sprintf("transform %s: %v", name, err))
			continue
		}
		dest := filepath.Join(targetDir, name)
		if dryRun {
			ui.DryRun(fmt.Sprintf("write %s (opencode-exclusive)", dest))
			count++
			continue
		}
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return count, err
		}
		if err := os.WriteFile(dest, []byte(transformed), 0644); err != nil {
			return count, err
		}
		ui.Added(fmt.Sprintf("%s (opencode-exclusive)", name))
		count++
	}
	return count, nil
}

// transformForOpencode ports transform_agent.py to Go.
// modelOnly=true performs model-line substitution only (for opencode-exclusive agents).
func transformForOpencode(content, selectedModel string, modelOnly bool) (string, error) {
	if modelOnly {
		if selectedModel != "" {
			return modelRe.ReplaceAllString(content, "model: "+resolveModel(selectedModel)), nil
		}
		return content, nil
	}

	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return content, nil
	}
	fm := parts[1]
	body := parts[2]

	var newLines []string
	for _, line := range strings.Split(strings.TrimSpace(fm), "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			newLines = append(newLines, line)
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		val := strings.TrimSpace(line[idx+1:])

		if skipFrontmatterKeys[key] {
			continue
		}
		switch key {
		case "model":
			alias := val
			if selectedModel != "" {
				alias = selectedModel
			}
			newLines = append(newLines, "model: "+resolveModel(alias))
		case "tools":
			enabled := map[string]bool{}
			for _, t := range strings.Split(val, ",") {
				t = strings.TrimSpace(strings.ToLower(t))
				if oc, ok := claudeToOC[t]; ok {
					enabled[oc] = true
				}
			}
			var disabled []string
			for t := range opencodeTools {
				if !enabled[t] {
					disabled = append(disabled, t)
				}
			}
			sort.Strings(disabled)
			if len(disabled) > 0 {
				newLines = append(newLines, "tools:")
				for _, t := range disabled {
					newLines = append(newLines, "  "+t+": false")
				}
			}
		default:
			newLines = append(newLines, line)
		}
	}
	newLines = append(newLines, "mode: subagent")

	return "---\n" + strings.Join(newLines, "\n") + "\n---" + body, nil
}

func isDisabled(name string, disabled []string) bool {
	for _, d := range disabled {
		if d == name {
			return true
		}
	}
	return false
}
