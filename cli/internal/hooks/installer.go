package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"devexp/internal/ui"
)

type Registry []Hook

// Target ids — the sibling block keys in hooks/registry.json.
const (
	TargetClaudeCode = "claude_code"
	TargetOpencode   = "opencode"
)

// TargetSpec is one install target's view of a hook. Each target uses the
// fields it needs (Claude Code: Event/Matcher/Script; opencode: Event/Module/
// Export/FailClosed). A new target adds a registry block, not a new Go type.
type TargetSpec struct {
	Event      string `json:"event,omitempty"`
	Matcher    string `json:"matcher,omitempty"`
	Script     string `json:"script,omitempty"`
	Module     string `json:"module,omitempty"`
	Export     string `json:"export,omitempty"`
	FailClosed bool   `json:"fail_closed,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"` // nil = follow Hook.Enabled
}

type Hook struct {
	Name        string
	Description string
	Enabled     bool
	Targets     map[string]TargetSpec // keyed by target id
}

// UnmarshalJSON reads the common fields (name, description, enabled) and
// decodes every other key whose value is a JSON object as a per-target block,
// so adding a target to the registry needs no change to this type.
func (h *Hook) UnmarshalJSON(b []byte) error {
	var common struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
	}
	if err := json.Unmarshal(b, &common); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	out := Hook{Name: common.Name, Description: common.Description, Enabled: common.Enabled}
	for key, val := range raw {
		if key == "name" || key == "description" || key == "enabled" || !isJSONObject(val) {
			continue
		}
		var spec TargetSpec
		if err := json.Unmarshal(val, &spec); err != nil {
			return fmt.Errorf("hook %q: target %q: %w", out.Name, key, err)
		}
		if out.Targets == nil {
			out.Targets = map[string]TargetSpec{}
		}
		out.Targets[key] = spec
	}
	*h = out
	return nil
}

// isJSONObject reports whether raw holds a JSON object, ignoring leading
// whitespace. Non-object extras in a registry entry are not target blocks.
func isJSONObject(raw json.RawMessage) bool {
	for _, c := range raw {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		}
		return c == '{'
	}
	return false
}

// Target returns the hook's block for target id, and whether it has one.
func (h Hook) Target(id string) (TargetSpec, bool) {
	spec, ok := h.Targets[id]
	return spec, ok
}

// EnabledFor reports whether the hook is enabled for target id: false without
// a block for it, else the block's own enabled override, else Hook.Enabled.
func (h Hook) EnabledFor(id string) bool {
	spec, ok := h.Targets[id]
	if !ok {
		return false
	}
	if spec.Enabled != nil {
		return *spec.Enabled
	}
	return h.Enabled
}

type hookEntry struct {
	Matcher string    `json:"matcher"`
	Hooks   []hookCmd `json:"hooks"`
}

type hookCmd struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func LoadRegistry(path string) (Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseRegistry(data)
}

// ParseRegistry decodes hooks/registry.json content, for callers that read it
// from somewhere other than a file on disk (the binary's embedded assets).
func ParseRegistry(data []byte) (Registry, error) {
	var r Registry
	return r, json.Unmarshal(data, &r)
}

func InstallClaude(registry Registry, repoDir, settingsPath string, disabled []string, dryRun bool) error {
	// Every command is registered as repoDir/<script> and run later from
	// whatever directory Claude Code is in, so it must be absolute (#126).
	if !filepath.IsAbs(repoDir) {
		return fmt.Errorf("hooks: repo dir %q is not an absolute path, so hook commands would be relative — refusing to register them", repoDir)
	}

	// Load existing settings as a raw map to preserve unknown fields
	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &raw) //nolint:errcheck
	}

	hooksMap := map[string][]hookEntry{}
	if hooksRaw, ok := raw["hooks"]; ok {
		json.Unmarshal(hooksRaw, &hooksMap) //nolint:errcheck
	}

	pruned := pruneStaleHooks(hooksMap, repoDir, dryRun)
	pruned = pruneForeignDevexpHooks(hooksMap, registry, repoDir, dryRun) || pruned

	isDisabled := func(name string) bool {
		for _, d := range disabled {
			if d == name {
				return true
			}
		}
		return false
	}

	changed := false
	for _, hook := range registry {
		if !hook.Enabled {
			continue
		}
		if isDisabled(hook.Name) {
			ui.Skipped(hook.Name, "disabled in devexp.config.json")
			continue
		}
		cc := hook.Targets[TargetClaudeCode]
		if cc.Event == "" || cc.Script == "" {
			continue
		}
		scriptAbs := filepath.Join(repoDir, cc.Script)

		if dryRun {
			ui.DryRun(fmt.Sprintf("add %s hook: %s", cc.Event, filepath.Base(cc.Script)))
			continue
		}

		// Skip if already registered
		alreadyIn := false
		for _, e := range hooksMap[cc.Event] {
			for _, h := range e.Hooks {
				if h.Command == scriptAbs {
					alreadyIn = true
					break
				}
			}
		}
		if alreadyIn {
			ui.Skipped(fmt.Sprintf("%s: %s", cc.Event, filepath.Base(cc.Script)), "already registered")
			continue
		}

		hooksMap[cc.Event] = append(hooksMap[cc.Event], hookEntry{
			Matcher: cc.Matcher,
			Hooks:   []hookCmd{{Type: "command", Command: scriptAbs}},
		})
		changed = true
		fmt.Printf("  \033[0;32m+\033[0m %s: %s\n", cc.Event, filepath.Base(cc.Script))
	}

	if dryRun {
		return nil
	}
	if !changed && !pruned {
		return nil
	}

	hooksBytes, err := json.Marshal(hooksMap)
	if err != nil {
		return err
	}
	raw["hooks"] = json.RawMessage(hooksBytes)

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, out, 0644); err != nil {
		return err
	}
	fmt.Printf("  Saved: %s\n", settingsPath)
	return nil
}

// pruneStaleHooks removes registered hook commands that live under repoDir
// (devexp-managed) but whose backing script no longer exists on disk — i.e.
// the hook was removed from this version's registry. User-authored hooks
// pointing elsewhere are left untouched. Returns whether anything was pruned.
func pruneStaleHooks(hooksMap map[string][]hookEntry, repoDir string, dryRun bool) bool {
	pruned := false
	for event, entries := range hooksMap {
		var kept []hookEntry
		for _, e := range entries {
			var keptCmds []hookCmd
			for _, h := range e.Hooks {
				if isStaleDevexpHook(h.Command, repoDir) {
					if dryRun {
						ui.DryRun(fmt.Sprintf("remove %s hook: %s (script no longer exists)", event, filepath.Base(h.Command)))
					} else {
						ui.Removed(fmt.Sprintf("%s: %s (script no longer exists)", event, filepath.Base(h.Command)))
					}
					pruned = true
					continue
				}
				keptCmds = append(keptCmds, h)
			}
			if len(keptCmds) == 0 {
				continue
			}
			e.Hooks = keptCmds
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(hooksMap, event)
		} else {
			hooksMap[event] = kept
		}
	}
	return pruned
}

// isStaleDevexpHook reports whether cmd is a devexp-managed command (lives
// under repoDir) whose script no longer exists on disk.
func isStaleDevexpHook(cmd, repoDir string) bool {
	rel, err := filepath.Rel(repoDir, cmd)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return false
	}
	_, statErr := os.Stat(cmd)
	return os.IsNotExist(statErr)
}

// scriptDir is the one directory every claude_code.script in the registry
// lives under. Requiring it in a command path keeps the basename match below
// from ever catching a user hook that merely shares a filename.
const scriptDir = "hooks/claude-code/"

// shellSyntax holds the characters that make a hook command more than a plain
// path: whitespace (arguments, env assignments, wrappers like `bash …`),
// expansions ($, ~, backticks), quoting and other shell operators.
const shellSyntax = " \t\n\r$~'\"`\\;&|<>()*?[]{}!#"

// isManagedScriptPath reports whether cmd is a plain path to a registry script:
// no shell syntax, a managed basename, and scriptDir directly above it starting
// at a path-segment boundary (the start of cmd or right after a "/"). devexp
// only ever registers such paths; a command that expands variables, takes
// arguments or lives in some other */hooks/claude-code/ directory (e.g.
// my-hooks/claude-code/) is the user's, and is never pruned.
func isManagedScriptPath(cmd string, managed map[string]bool) bool {
	if cmd == "" || strings.ContainsAny(cmd, shellSyntax) {
		return false
	}
	p := filepath.ToSlash(cmd)
	base := path.Base(p)
	if !managed[base] {
		return false
	}
	head := strings.TrimSuffix(p, base)
	return head == scriptDir || strings.HasSuffix(head, "/"+scriptDir)
}

// managedScriptNames collects the basename of every script the registry knows
// about — including disabled hooks, whose foreign-root copies would otherwise
// keep running after the hook was turned off.
func managedScriptNames(registry Registry) map[string]bool {
	names := map[string]bool{}
	for _, h := range registry {
		if s := h.Targets[TargetClaudeCode].Script; s != "" {
			names[filepath.Base(s)] = true
		}
	}
	return names
}

// isForeignDevexpHook reports whether cmd is a devexp-managed hook registered
// from a *different* install root than repoDir — the copy a release-binary
// install leaves behind when the same machine is later installed from a clone.
//
// The registry admits exactly one script per hook, so a second registration of
// the same script under another root is a duplicate by definition: both fire,
// and the one outside repoDir is never refreshed again. It is identified by
// registry basename plus the registry's own script directory, so a user hook
// that happens to share a filename is untouched.
func isForeignDevexpHook(cmd string, managed map[string]bool, repoDir string) bool {
	if !filepath.IsAbs(cmd) || !isManagedScriptPath(cmd, managed) {
		return false
	}
	rel, err := filepath.Rel(repoDir, cmd)
	if err != nil {
		return false
	}
	// Under repoDir: it is the copy being installed, not a foreign duplicate.
	return strings.HasPrefix(rel, "..")
}

// isRelativeDevexpHook reports whether cmd is a devexp-managed hook registered
// with a relative path — what an install from a relative DEVEXP_DIR wrote
// before repo dirs were made absolute (#126). It is matched like
// isForeignDevexpHook, by isManagedScriptPath, and replaced by the absolute
// registration on the same run.
func isRelativeDevexpHook(cmd string, managed map[string]bool) bool {
	return !filepath.IsAbs(cmd) && isManagedScriptPath(cmd, managed)
}

// pruneForeignDevexpHooks removes devexp-managed registrations that point at an
// install root other than repoDir, or that are relative paths. Without this, a
// clone install can never clean up a release-binary install's entries: they sit
// outside repoDir, so pruneStaleHooks refuses to touch them, and they go stale
// permanently while still executing.
func pruneForeignDevexpHooks(hooksMap map[string][]hookEntry, registry Registry, repoDir string, dryRun bool) bool {
	managed := managedScriptNames(registry)
	pruned := false
	for event, entries := range hooksMap {
		var kept []hookEntry
		for _, e := range entries {
			var keptCmds []hookCmd
			for _, h := range e.Hooks {
				reason := ""
				switch {
				case isRelativeDevexpHook(h.Command, managed):
					reason = "relative path"
				case isForeignDevexpHook(h.Command, managed, repoDir):
					reason = "duplicate from another install root"
				}
				if reason != "" {
					msg := fmt.Sprintf("%s: %s (%s)", event, filepath.Base(h.Command), reason)
					if dryRun {
						ui.DryRun("remove " + msg)
					} else {
						ui.Removed(msg)
					}
					pruned = true
					continue
				}
				keptCmds = append(keptCmds, h)
			}
			if len(keptCmds) == 0 {
				continue
			}
			e.Hooks = keptCmds
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(hooksMap, event)
		} else {
			hooksMap[event] = kept
		}
	}
	return pruned
}
