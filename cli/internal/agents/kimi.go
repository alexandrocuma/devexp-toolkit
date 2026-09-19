package agents

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
// Kimi reads an agent's front matter as strict YAML (js-yaml), so unlike
// transformForOpencode — which splits lines on the first colon and works by
// luck on the twelve multi-line quoted descriptions — this rebuilds the
// mapping through a YAML parser and encoder. A description is copied as a
// node and re-emitted, so its value survives byte for byte whatever quoting
// the source used.
//
// What Kimi does with what it reads, all measured against the 2.0.1 binary:
//
//   - `description` is required and the body must be non-empty; a file that
//     fails either is skipped, and the only notice is a line in Kimi's own log.
//   - `name` is optional (it falls back to the filename) but must be
//     kebab-case, and may not be one of Kimi's built-in profiles without
//     `override: true`, which devexp never emits.
//   - `color`, `memory` and `model` are read and ignored. They are dropped
//     anyway, so nobody reads an installed file and assumes they work.
//   - An unknown tool name is dropped silently, and nothing reaches Kimi's
//     log — the warning goes to its in-session event dispatcher. So devexp
//     reports dropped names at install time, which is the only moment anyone
//     will see them.

// kimiToolMap maps Claude Code tool names onto Kimi Code CLI's. A name absent
// from the map is dropped and reported.
//
// Every name is listed, including the ones that are the same on both sides,
// because passing through anything Kimi happens to accept is how this goes
// wrong: Kimi has its own TaskList, which lists background tasks rather than
// todos. Passing Claude's todo TaskList through would compile, install and
// silently hand the agent an unrelated capability.
var kimiToolMap = map[string]string{
	"Read":  "Read",
	"Write": "Write",
	"Edit":  "Edit",
	"Bash":  "Bash",
	"Glob":  "Glob",
	"Grep":  "Grep",
	"Agent": "Agent",
	"Skill": "Skill",
	// WebSearch exists in Kimi's tool set but needs a host-provided search
	// service; where there is none it is a warning when the profile activates,
	// not an install-time problem.
	"WebSearch": "WebSearch",
	"WebFetch":  "FetchURL",
	// Claude's four task tools and TodoWrite are all todo management, which
	// Kimi does with the single TodoList tool. kimiTools dedupes, so an agent
	// declaring all four gets one entry.
	"TaskCreate": "TodoList",
	"TaskGet":    "TodoList",
	"TaskList":   "TodoList",
	"TaskUpdate": "TodoList",
	"TodoWrite":  "TodoList",
}

// kimiBuiltinProfiles are Kimi's own agent profiles.
var kimiBuiltinProfiles = map[string]bool{"agent": true, "coder": true, "explore": true, "plan": true}

// kimiDelegationBuiltins are the built-in profiles a delegating devexp agent
// may spawn, appended to every subagents list. `coder` is left out: a devexp
// agent that wants code written says so itself.
var kimiDelegationBuiltins = []string{"explore", "plan"}

var kimiNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// kimiClaudeAgentRefRe matches a reference to an installed Claude Code agent
// file in a body. Only this form is rewritten: `~/.claude/agent-memory` and
// `~/.claude/projects` stay as they are, because both CLIs share that memory
// deliberately, and a bare `~/.claude/agents/` with no filename is prose.
var kimiClaudeAgentRefRe = regexp.MustCompile(`~/\.claude/agents/([a-z0-9][a-z0-9-]*\.md)`)

// kimiBasePromptFooter opts the ported body into Kimi's own system prompt.
//
// This is the difference between a working port and a crippled one. A custom
// agent body in Kimi is a template that *replaces* the whole system prompt,
// where Claude Code appends to it. Measured on 2.0.1: the same body with this
// footer renders a 9,144-character system prompt and without it renders 104 —
// the body and nothing else. What the footer restores is the tool-use
// guidance, the coding and risky-action rules, the environment section, the
// AGENTS.md injection, the working-directory listing and the entire skills
// catalog.
//
// Nothing visibly fails without it: the agent still gets every tool, it just
// has no idea what any of them are for and cannot see a single skill. That is
// exactly why it is added automatically rather than left to each agent's
// author, who is writing for Claude Code and has no reason to think about it.
//
// It goes last, and Kimi splices it in textually at this position, so the
// devexp instructions are read first and Kimi's defaults follow.
const kimiBasePromptFooter = "\n\n---\n\n${base_prompt}\n"

// splitFrontmatter applies Kimi's front-matter rule: the first line must trim
// to `---`, and the first later line that trims to `---` closes the block. ok
// is false when there is no such block, which for Kimi means the file is not
// an agent at all.
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

// kimiTools maps Claude tool names onto Kimi's, preserving the order they were
// declared in and dropping duplicates (the four Task* names collapse onto one
// TodoList). dropped lists the names with no Kimi equivalent, in order, so the
// caller can name them.
//
// An `mcp__…` name passes through: Kimi matches those against a glob, so which
// ones exist depends on the user's MCP servers rather than on anything devexp
// can check here.
func kimiTools(claude []string) (tools, dropped []string) {
	seen := map[string]bool{}
	for _, name := range claude {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		mapped, ok := kimiToolMap[name]
		if !ok && strings.HasPrefix(name, "mcp__") {
			mapped, ok = name, true
		}
		if !ok {
			dropped = append(dropped, name)
			continue
		}
		if seen[mapped] {
			continue
		}
		seen[mapped] = true
		tools = append(tools, mapped)
	}
	return tools, dropped
}

// rewriteClaudeAgentRefs points the body's "read ~/.claude/agents/<n>.md and
// follow it" instructions at where devexp actually installed them for Kimi.
//
// agentsDir is absolute. Not because `~` would fail — measured against 2.0.1,
// Read, Write, Edit, Glob and Grep all expand `~/` against the process's HOME
// before resolving, and an expanded `~/` even counts as absolute for the
// outside-workspace guard, where a relative path to the same file is refused.
// It is absolute because the Kimi root is only `~/.kimi-code` when
// $KIMI_CODE_HOME is unset; with it set to, say, /opt/kimi there is no tilde
// form of the destination at all. One shape that is always right beats two
// that depend on the environment.
//
// That is also why the `~/.claude/agent-memory` paths a few lines down stay as
// they are rather than being made absolute: they are not this install's
// destination but a directory both CLIs share on purpose, and `~` resolves
// there correctly in either CLI. The two forms in one installed file are the
// same decision seen from two sides, not an inconsistency.
//
// (Measured on one version of a closed-source bundle, not a documented
// contract. resolveKimiHome refuses a root that cannot be written into a
// prompt either way, so nothing here depends on that measurement holding.)
//
// It is duplicated in internal/skills rather than shared, because internal
// packages do not import one another (docs/development/conventions.md) — the
// same reason isDisabled is duplicated there.
func rewriteClaudeAgentRefs(body, agentsDir string) string {
	return kimiClaudeAgentRefRe.ReplaceAllString(body, filepath.ToSlash(agentsDir)+"/$1")
}

// parseStringList reads a `tools:`/`subagents:` value the way Kimi does:
// either a comma-separated string or a YAML sequence of strings.
func parseStringList(n *yaml.Node) ([]string, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		var out []string
		for _, part := range strings.Split(n.Value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out, nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, item := range n.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("list entry is not a string")
			}
			if v := strings.TrimSpace(item.Value); v != "" {
				out = append(out, v)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("value is neither a comma-separated string nor a list")
}

// stringSeq builds a YAML sequence node from names.
func stringSeq(names []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, name := range names {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name})
	}
	return n
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// kimiFrontmatter is what one pass over a source agent's front matter yields.
type kimiFrontmatter struct {
	name      string
	tools     []string // as the source declares them, before mapping
	delegates bool     // the source tools include Agent
	mapping   *yaml.Node
	body      string
}

// parseKimiFrontmatter reads and validates a source agent against what Kimi
// requires, without deciding anything about the output. fallbackName is the
// filename's stem, which Kimi itself would fall back to.
func parseKimiFrontmatter(content, fallbackName string) (*kimiFrontmatter, error) {
	fm, body, ok := splitFrontmatter(content)
	if !ok {
		return nil, fmt.Errorf("no front matter block — Kimi requires one, with a description")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return nil, fmt.Errorf("front matter is not valid YAML: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("front matter is not a mapping")
	}
	m := doc.Content[0]

	out := &kimiFrontmatter{name: fallbackName, mapping: m, body: body}
	var hasDescription bool
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, val := m.Content[i], m.Content[i+1]
		switch key.Value {
		case "name":
			out.name = strings.TrimSpace(val.Value)
		case "description":
			if val.Kind != yaml.ScalarNode || strings.TrimSpace(val.Value) == "" {
				return nil, fmt.Errorf("description is empty — Kimi requires a non-empty one")
			}
			hasDescription = true
		case "tools":
			tools, err := parseStringList(val)
			if err != nil {
				return nil, fmt.Errorf("tools: %w", err)
			}
			out.tools = tools
			out.delegates = slices.Contains(tools, "Agent")
		}
	}
	if !hasDescription {
		return nil, fmt.Errorf("no description — Kimi requires one and would skip this agent")
	}
	if !kimiNameRe.MatchString(out.name) {
		return nil, fmt.Errorf("name %q is not kebab-case — Kimi refuses it", out.name)
	}
	if kimiBuiltinProfiles[out.name] {
		return nil, fmt.Errorf("name %q is one of Kimi's built-in profiles, which needs override: true", out.name)
	}
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("body is empty — Kimi requires one")
	}
	return out, nil
}

// transformForKimi rewrites one agent file for Kimi Code CLI and returns it
// with the Claude tool names that had no equivalent. subagents is the
// allowlist to emit, used only when the source declares the Agent tool.
func transformForKimi(content, fallbackName, agentsDir string, subagents []string) (string, []string, error) {
	parsed, err := parseKimiFrontmatter(content, fallbackName)
	if err != nil {
		return "", nil, err
	}

	// The output mapping is built key by key rather than by deleting from the
	// input. Every key devexp has an opinion about gets a case below; anything
	// else is carried through, because Kimi ignores what it does not know and
	// silently dropping a key an author added is worse than passing it on.
	//
	// The keys devexp writes itself — name, tools, subagents — are never
	// carried through, whatever the source says. A source that gained its own
	// `subagents` would otherwise have the key emitted twice, and js-yaml
	// rejects a duplicate mapping key: the agent would vanish from Kimi with
	// nothing but a line in its log, which is the failure mode the rest of
	// this file exists to avoid.
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	out.Content = append(out.Content, scalar("name"), scalar(parsed.name))

	var dropped []string
	for i := 0; i+1 < len(parsed.mapping.Content); i += 2 {
		key, val := parsed.mapping.Content[i], parsed.mapping.Content[i+1]
		switch key.Value {
		case "name", "subagents":
			// Both are devexp's to write: name was emitted above (from the
			// parsed value, so a missing one is filled in), and subagents is
			// emitted with the tools below, computed from what this run
			// installed rather than from what the source claims.
		case "color", "memory", "model":
			// Read and ignored by Kimi. Dropped so an installed file does not
			// suggest they do something.
		case "description":
			// Copied as a node: the encoder re-quotes it safely, and the value
			// survives whatever quoting the source used.
			out.Content = append(out.Content, scalar("description"), val)
		case "tools":
			var tools []string
			tools, dropped = kimiTools(parsed.tools)
			if len(tools) == 0 {
				// Kimi would load this agent, list it, and offer it to a parent
				// as a delegation target — with no tools at all, no error and
				// nothing in its log. An agent that can do nothing is worse than
				// one that is absent, because only the absence is visible.
				return "", dropped, fmt.Errorf("none of its tools (%s) has a Kimi equivalent, which would install an agent that can do nothing", strings.Join(parsed.tools, ", "))
			}
			out.Content = append(out.Content, scalar("tools"), stringSeq(tools))
			if parsed.delegates && len(subagents) > 0 {
				out.Content = append(out.Content, scalar("subagents"), stringSeq(subagents))
			}
		default:
			out.Content = append(out.Content, key, val)
		}
	}

	encoded, err := yaml.Marshal(out)
	if err != nil {
		return "", dropped, fmt.Errorf("encode front matter: %w", err)
	}
	body := rewriteClaudeAgentRefs(parsed.body, agentsDir) + kimiBasePromptFooter
	// The closing fence carries its own newline. splitFrontmatter returns the
	// body from the line after the fence, so that newline is not part of it:
	// concatenating "---" and the body directly would glue the two together
	// ("---# Body") and the file would stop being an agent at all.
	return "---\n" + string(encoded) + "---\n" + body, dropped, nil
}

// kimiSubagents is the allowlist emitted for every delegating agent: each
// installed agent that does not itself delegate, plus Kimi's explore and plan.
//
// An explicit list rather than Kimi's `"*"` escape, which removes the
// allowlist entirely. The bodies describe delegation cycles — dev-agent asks
// for backend-senior-dev, which asks for dev-agent — so an unbounded allowlist
// lets a chain run as deep as the model decides to take it. Excluding the
// delegating agents keeps every chain one level deep, and building the list
// from what this run installed means a disabled agent never appears in it.
func kimiSubagents(installed map[string]bool) []string {
	names := make([]string, 0, len(installed)+len(kimiDelegationBuiltins))
	for name, delegates := range installed {
		if !delegates {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return append(names, kimiDelegationBuiltins...)
}

// InstallKimi transforms agent files for Kimi Code CLI and writes them to
// targetDir. agentsDir is the absolute directory the installed agents end up
// in, which body references to Claude Code's copies are rewritten to point at.
// Returns the filenames installed (including in dryRun mode), so callers can
// diff against a manifest to detect stale files from prior runs.
//
// It reads every source agent before writing any of them, because the
// subagents allowlist depends on which agents this run installs — an agent
// disabled in devexp.config.json must not be offered as a delegation target.
func InstallKimi(srcDir, targetDir, agentsDir string, disabled []string, dryRun bool) ([]string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, err
	}

	type source struct {
		file    string
		name    string
		content string
	}
	var sources []source
	installed := map[string]bool{} // name → delegates
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		file := entry.Name()
		if file == "README.md" {
			continue
		}
		agentName := strings.TrimSuffix(file, ".md")
		if isDisabled(agentName, disabled) {
			ui.Skipped(file, "disabled in devexp.config.json")
			continue
		}
		content, err := os.ReadFile(filepath.Join(srcDir, file))
		if err != nil {
			return nil, err
		}
		parsed, err := parseKimiFrontmatter(string(content), agentName)
		if err != nil {
			ui.Warn(fmt.Sprintf("transform %s: %v", file, err))
			continue
		}
		sources = append(sources, source{file: file, name: parsed.name, content: string(content)})
		installed[parsed.name] = parsed.delegates
	}

	subagents := kimiSubagents(installed)

	var out []string
	for _, src := range sources {
		transformed, dropped, err := transformForKimi(src.content, src.name, agentsDir, subagents)
		if err != nil {
			ui.Warn(fmt.Sprintf("transform %s: %v", src.file, err))
			continue
		}
		if len(dropped) > 0 {
			ui.Warn(fmt.Sprintf("%s: Kimi Code CLI has no equivalent for %s, so %s dropped from its tools — Kimi would drop them without saying so",
				src.file, strings.Join(dropped, ", "), pluralWere(len(dropped))))
		}
		dest := filepath.Join(targetDir, src.file)
		if keepSymlinkedEntry(dest) {
			out = append(out, src.file)
			continue
		}
		if dryRun {
			ui.DryRun(fmt.Sprintf("transform + write %s", dest))
			out = append(out, src.file)
			continue
		}
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return out, err
		}
		if err := fsutil.WriteFileAtomic(dest, []byte(transformed), 0644); err != nil {
			return out, err
		}
		ui.Added(src.file)
		out = append(out, src.file)
	}
	return out, nil
}

func pluralWere(n int) string {
	if n == 1 {
		return "it was"
	}
	return "they were"
}
