package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
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

	// Only the hooks value is rewritten; every other byte of settings.json, and
	// every field of a hook devexp doesn't own, is written back as read (#137).
	data, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("hooks: read %s: %w", settingsPath, err)
	}
	doc, hooksMap, err := loadSettings(data)
	if err != nil {
		return fmt.Errorf("hooks: %s: %w — it was left untouched and no hooks were registered; fix it and re-run", settingsPath, err)
	}

	pruned := requoteDevexpHooks(hooksMap, registry, repoDir, dryRun)
	pruned = pruneStaleHooks(hooksMap, repoDir, dryRun) || pruned
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
		command := hookCommand(filepath.Join(repoDir, cc.Script))

		if dryRun {
			ui.DryRun(fmt.Sprintf("add %s hook: %s", cc.Event, filepath.Base(cc.Script)))
			continue
		}

		name := fmt.Sprintf("%s: %s", cc.Event, filepath.Base(cc.Script))
		switch registerHook(hooksMap, cc.Event, cc.Matcher, command) {
		case hookRegistered:
			ui.Skipped(name, "already registered")
			continue
		case hookMatcherUpdated:
			fmt.Printf("  \033[0;33m~\033[0m %s (matcher now %q, as in the registry)\n", name, cc.Matcher)
		case hookAdded:
			fmt.Printf("  \033[0;32m+\033[0m %s\n", name)
		}
		changed = true
	}

	if dryRun {
		return nil
	}
	if !changed && !pruned {
		return nil
	}

	out, err := doc.render(hooksMap)
	if err != nil {
		return fmt.Errorf("hooks: %s: %w — it was left untouched", settingsPath, err)
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

// What registerHook did.
const (
	hookRegistered     = iota // already registered under the registry's matcher
	hookMatcherUpdated        // registered before, now under the registry's matcher
	hookAdded                 // newly registered
)

// registerHook makes devexp's command run for event under matcher, the
// registry's current matcher. A command already registered under matcher is
// left as it is. One registered only under other matchers — the registry
// changed its matcher since the install that wrote it — is brought to matcher:
// an entry holding nothing but that command takes matcher in place, keeping
// its other fields; from an entry shared with other commands (a user's, say)
// the handler moves out, fields and all, into an entry of its own, so no one
// else's matcher changes. A command registered nowhere gets a new entry.
func registerHook(hooksMap map[string][]hookEntry, event, matcher, command string) int {
	entries := hooksMap[event]
	ours := func(h hookCmd) bool { return h.devexpForm() && h.Command == command }
	for _, e := range entries {
		if e.Matcher == matcher && slices.ContainsFunc(e.Hooks, ours) {
			return hookRegistered
		}
	}

	var moved *hookCmd
	updated := false
	for i := range entries {
		e := &entries[i]
		switch n := countFunc(e.Hooks, ours); {
		case n == 0:
		case n == len(e.Hooks):
			e.Matcher = matcher
			updated = true
		default:
			var kept []hookCmd
			for _, h := range e.Hooks {
				if !ours(h) {
					kept = append(kept, h)
				} else if moved == nil {
					moved = &h
				}
			}
			e.Hooks = kept
		}
	}
	switch {
	case updated:
		return hookMatcherUpdated
	case moved != nil:
		hooksMap[event] = append(entries, hookEntry{Matcher: matcher, Hooks: []hookCmd{*moved}})
		return hookMatcherUpdated
	}
	hooksMap[event] = append(entries, hookEntry{
		Matcher: matcher,
		Hooks:   []hookCmd{{Type: "command", Command: command}},
	})
	return hookAdded
}

func countFunc[T any](s []T, f func(T) bool) int {
	n := 0
	for _, v := range s {
		if f(v) {
			n++
		}
	}
	return n
}

// removeHookCmds calls drop for every handler, in place so drop may rewrite
// it, and removes those it returns true for. An entry left with no handlers is
// removed, and so is an event left with no entries; an entry or event that
// had none to begin with is the user's and stays.
func removeHookCmds(hooksMap map[string][]hookEntry, drop func(event string, h *hookCmd) bool) {
	for event, entries := range hooksMap {
		if len(entries) == 0 {
			continue
		}
		var kept []hookEntry
		for _, e := range entries {
			if len(e.Hooks) == 0 {
				kept = append(kept, e)
				continue
			}
			var keptCmds []hookCmd
			for _, h := range e.Hooks {
				if drop(event, &h) {
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
}

// pruneStaleHooks removes registered hook commands that live under repoDir
// (devexp-managed) but whose backing script no longer exists on disk — i.e.
// the hook was removed from this version's registry. User-authored hooks
// pointing elsewhere are left untouched. Returns whether anything was pruned.
func pruneStaleHooks(hooksMap map[string][]hookEntry, repoDir string, dryRun bool) bool {
	pruned := false
	removeHookCmds(hooksMap, func(event string, h *hookCmd) bool {
		if !h.devexpForm() || !isStaleDevexpHook(h.Command, repoDir) {
			return false
		}
		name := commandBase(h.Command)
		if dryRun {
			ui.DryRun(fmt.Sprintf("remove %s hook: %s (script no longer exists)", event, name))
		} else {
			ui.Removed(fmt.Sprintf("%s: %s (script no longer exists)", event, name))
		}
		pruned = true
		return true
	})
	return pruned
}

// isStaleDevexpHook reports whether cmd is devexp's registration of a script
// under repoDir that no longer exists on disk. Only the forms devexp writes
// count (#138): a plain or single-quoted command (commandPath, so a quoted one
// is judged by the path it runs, #135), or this repo's legacy bare spelling
// (legacyScriptPath), naming the clean absolute path
// repoDir/hooks/claude-code/<script>. The registry no longer lists a removed
// script, so its name can't be checked; its directory can. Any other command
// under repoDir — with arguments, in another directory, with shell syntax — is
// the user's and never stale.
func isStaleDevexpHook(cmd, repoDir string) bool {
	p, ok := commandPath(cmd)
	if !ok {
		p, ok = legacyScriptPath(cmd, repoDir)
	}
	if !ok || filepath.Clean(p) != p {
		return false
	}
	rel, err := filepath.Rel(repoDir, p)
	if err != nil {
		return false
	}
	script, ok := strings.CutPrefix(filepath.ToSlash(rel), scriptDir)
	if !ok || script == "" || strings.Contains(script, "/") {
		return false
	}
	_, statErr := os.Stat(p)
	return os.IsNotExist(statErr)
}

// legacyScriptPath returns cmd when it may be the bare path an install from
// repoDir wrote before paths needing quotes were quoted (#135): repoDir, then a
// rest with no shell syntax, which isStaleDevexpHook requires to be
// hooks/claude-code/<script>. Only repoDir may hold shell syntax; a rest
// holding some, such as "hooks/claude-code/x.sh --flag", is a command with
// arguments. Double quotes stay the user's here: requoteDevexpHooks takes them
// as devexp's only for scripts the registry still lists.
func legacyScriptPath(cmd, repoDir string) (string, bool) {
	rest, ok := strings.CutPrefix(cmd, filepath.Clean(repoDir))
	if !ok || strings.ContainsAny(rest, shellSyntax) {
		return "", false
	}
	return cmd, true
}

// scriptDir is the one directory every claude_code.script in the registry
// lives under. Requiring it in a command path keeps the basename match below
// from ever catching a user hook that merely shares a filename.
const scriptDir = "hooks/claude-code/"

// shellSyntax holds the characters that make a hook command more than a plain
// path: whitespace (arguments, env assignments, wrappers like `bash …`),
// expansions ($, ~, backticks), quoting and other shell operators.
const shellSyntax = " \t\n\r$~'\"`\\;&|<>()*?[]{}!#"

// hookCommand is the command devexp registers for the script at p. Claude Code
// runs a command hook through a shell (sh -c), so a path with shell syntax — a
// space in "My Projects", a quote, a $ — would be split or expanded: the hook
// fails to run, which Claude Code treats as a non-blocking error, or something
// else runs (#135). Such a path is registered as one single-quoted word; any
// other path stays plain, byte-identical to what earlier releases wrote.
func hookCommand(p string) string {
	if !strings.ContainsAny(p, shellSyntax) {
		return p
	}
	return shellQuote(p)
}

// shellQuote renders s as one POSIX single-quoted shell word. Nothing inside
// single quotes is special, so each ' becomes: close quote, backslash-quote,
// reopen quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// commandPath returns the path cmd runs, if cmd is in a form devexp writes:
//   - a plain path, with no shell syntax at all, or
//   - exactly one single-quoted word holding an absolute path, as shellQuote
//     renders it (the whole command re-quotes to itself).
//
// Anything else — double quotes, a quoted word followed by arguments or ";",
// concatenated words like '/a'b, a quoted relative path — is not devexp's.
func commandPath(cmd string) (string, bool) {
	if cmd == "" {
		return "", false
	}
	if !strings.ContainsAny(cmd, shellSyntax) {
		return cmd, true
	}
	if len(cmd) < 2 || cmd[0] != '\'' || cmd[len(cmd)-1] != '\'' {
		return "", false
	}
	p := strings.ReplaceAll(cmd[1:len(cmd)-1], `'\''`, "'")
	if !filepath.IsAbs(p) || shellQuote(p) != cmd {
		return "", false
	}
	return p, true
}

// commandBase is the script name to show for cmd: the basename of the path it
// runs, or of the raw string when it isn't in a devexp form.
func commandBase(cmd string) string {
	if p, ok := commandPath(cmd); ok {
		return filepath.Base(p)
	}
	return filepath.Base(cmd)
}

// isManagedScriptPath reports whether cmd runs a registry script: it is in a
// form devexp writes (commandPath — a plain path, or one single-quoted absolute
// path), with a managed basename and scriptDir directly above it starting at a
// path-segment boundary (the start of the path or right after a "/"). devexp
// only ever registers such commands; one that expands variables, takes
// arguments or lives in some other */hooks/claude-code/ directory (e.g.
// my-hooks/claude-code/) is the user's, and is never pruned.
func isManagedScriptPath(cmd string, managed map[string]bool) bool {
	cmdPath, ok := commandPath(cmd)
	if !ok {
		return false
	}
	p := filepath.ToSlash(cmdPath)
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
	p, ok := commandPath(cmd)
	if !ok || !filepath.IsAbs(p) || !isManagedScriptPath(cmd, managed) {
		return false
	}
	rel, err := filepath.Rel(repoDir, p)
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
	p, ok := commandPath(cmd)
	return ok && !filepath.IsAbs(p) && isManagedScriptPath(cmd, managed)
}

// pruneForeignDevexpHooks removes devexp-managed registrations that point at an
// install root other than repoDir, or that are relative paths. Without this, a
// clone install can never clean up a release-binary install's entries: they sit
// outside repoDir, so pruneStaleHooks refuses to touch them, and they go stale
// permanently while still executing.
func pruneForeignDevexpHooks(hooksMap map[string][]hookEntry, registry Registry, repoDir string, dryRun bool) bool {
	managed := managedScriptNames(registry)
	pruned := false
	removeHookCmds(hooksMap, func(event string, h *hookCmd) bool {
		if !h.devexpForm() {
			return false
		}
		reason := ""
		switch {
		case isRelativeDevexpHook(h.Command, managed):
			reason = "relative path"
		case isForeignDevexpHook(h.Command, managed, repoDir):
			reason = "duplicate from another install root"
		}
		if reason == "" {
			return false
		}
		msg := fmt.Sprintf("%s: %s (%s)", event, commandBase(h.Command), reason)
		if dryRun {
			ui.DryRun("remove " + msg)
		} else {
			ui.Removed(msg)
		}
		pruned = true
		return true
	})
	return pruned
}

// requoteDevexpHooks brings each registration of a registry script under
// repoDir to the form hookCommand gives it. It recognises exactly these other
// spellings of that one path:
//   - the bare path an earlier install wrote before paths needing quotes were
//     quoted (#135), which the shell splits;
//   - the path in double quotes, the natural hand fix for that, when the path
//     has no $, backquote, \ or " (so the double quotes are literal);
//   - the single-quoted form of a path that needs no quoting.
//
// Each is rewritten in place (the entry keeps its event, matcher and position;
// disabled hooks included), or dropped when the event already holds the
// command it would become, so the script never runs twice. An entry left with
// no commands is removed. A user's command never matches: other directories,
// other scripts, arguments (in the string or as args) or any other quoting.
func requoteDevexpHooks(hooksMap map[string][]hookEntry, registry Registry, repoDir string, dryRun bool) bool {
	want := map[string]string{} // script path under repoDir -> command to register
	for _, h := range registry {
		if s := h.Targets[TargetClaudeCode].Script; s != "" {
			abs := filepath.Join(repoDir, s)
			want[abs] = hookCommand(abs)
		}
	}
	legacy := map[string]string{} // exact legacy spelling -> script path
	for abs := range want {
		legacy[abs] = abs
		if dq, ok := doubleQuoted(abs); ok {
			legacy[dq] = abs
		}
	}

	present := map[string]map[string]bool{} // event -> devexp-form commands it holds
	for event, entries := range hooksMap {
		present[event] = map[string]bool{}
		for _, e := range entries {
			for _, h := range e.Hooks {
				if h.devexpForm() {
					present[event][h.Command] = true
				}
			}
		}
	}

	changed := false
	removeHookCmds(hooksMap, func(event string, h *hookCmd) bool {
		if !h.devexpForm() {
			return false
		}
		p, ok := legacy[h.Command]
		if !ok {
			p, _ = commandPath(h.Command)
		}
		cmd, ok := want[p]
		if !ok || cmd == h.Command {
			return false
		}
		if present[event][cmd] {
			msg := fmt.Sprintf("%s: %s (duplicate of the registered command)", event, filepath.Base(p))
			if dryRun {
				ui.DryRun("remove " + msg)
				return false
			}
			ui.Removed(msg)
			changed = true
			return true
		}
		msg := fmt.Sprintf("%s: %s (command re-quoted for the shell)", event, filepath.Base(p))
		if dryRun {
			ui.DryRun("rewrite " + msg)
			return false
		}
		fmt.Printf("  \033[0;33m~\033[0m %s\n", msg)
		h.Command = cmd
		present[event][cmd] = true
		changed = true
		return false
	})
	return changed
}

// doubleQuoted returns p in double quotes when that is just p to the shell:
// p holds none of the characters double quotes leave special ($, backquote,
// \, "). Otherwise there is no such literal spelling.
func doubleQuoted(p string) (string, bool) {
	if strings.ContainsAny(p, "$`\\\"") {
		return "", false
	}
	return `"` + p + `"`, true
}
