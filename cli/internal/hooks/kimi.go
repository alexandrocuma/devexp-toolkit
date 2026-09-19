package hooks

import "strings"

// ── Kimi Code CLI hooks ───────────────────────────────────────────────────────
//
// Kimi runs a hook the way Claude Code runs a command hook — a shell command
// per event, the tool envelope on stdin, exit 2 to block with stderr as the
// reason — so the Claude Code guard scripts are the same scripts. What differs
// is everything around them. Measured against Kimi Code CLI 2.0.1: the shipped
// bundle's agent-core-v2 hook runner — matchHooks.ts AND the runHook.ts it
// calls, because half of what matters happens in the outer frame — and
// `kimi doctor config` run over candidate files in a throwaway
// KIMI_CODE_HOME.
//
//   - The envelope on stdin is snake_case at the TOP LEVEL and nowhere else.
//     runMatchedHooks builds the payload camelCase and puts it through
//     toHookInputData — `Object.entries` + camelToSnake, so exactly one level
//     deep — before runHook spawns anything. A guard therefore receives
//     `tool_name` and `tool_input` already spelled the way it reads them, and
//     a `tool_input` whose keys are still the tool's own (`path`, not
//     `file_path`). Reading runHook alone, without its caller, is what made
//     the first adapter look for `toolName`, find nothing, and fail closed on
//     every tool call in the session.
//
//   - `permissionDecision: "ask"` is ALLOWED. runHook's structuredOutput blocks
//     only on "deny"; everything else falls through to allowResult. So a guard
//     whose only verdict is an ask cannot be installed here at all — that is
//     large-file-guard, and its registry block says so.
//   - A PostToolUse hook's result is not waited for or read. Every on-save hook
//     is therefore off for Kimi, with the reason in its registry block.
//   - A hook that times out, cannot start, or exits anything other than 0 or 2
//     is read as ALLOW (runHook returns allowResult for a spawn error, the
//     timeout kill and every other exit code). That is Kimi's design and devexp
//     cannot change it; what devexp can do is register a timeout the guards'
//     own scan budget fits inside — see kimiDefaultTimeout.
//   - The matcher is an unanchored JS regex over the tool name
//     (`new RegExp(pattern).test(value)`), and an invalid pattern silently never
//     matches. Every matcher in the registry is anchored `^(...)$` so a bare
//     `Read` cannot also fire for ReadMediaFile or for an MCP tool whose name
//     happens to contain it.
//
// This file is the selection half: which hooks go to Kimi and why the others
// do not. Rendering the command, copying the scripts and writing the
// config.toml block come with the adapter.

// kimiDefaultTimeout is the timeout, in seconds, a kimi block that omits one is
// registered with.
//
// Kimi's own default is 30 (DEFAULT_HOOK_TIMEOUT_SECONDS in its bundled
// matchHooks/runHook), and that is too low: the guards run under a scan budget
// whose ceiling is 44 seconds (hooks/claude-code/scan-budget.sh), and a hook
// Kimi kills at its timeout does not block the tool call. A guard killed
// mid-scan would therefore fail OPEN, which is the exact failure the scan
// budget exists to prevent. 45 clears the ceiling, as the Claude Code
// registrations do, and stays inside the 1..600 Kimi's schema accepts.
const kimiDefaultTimeout = 45

// kimiHookEvents is the set of events Kimi 2.0.1 accepts in a [[hooks]] entry,
// verbatim from the message `kimi doctor config` prints when it rejects one.
// An event outside it makes the entry invalid, and one invalid entry makes
// Kimi drop the WHOLE hooks section — the user's own hooks with it — behind a
// warning nobody sees. So nothing is written that has not been checked against
// this first.
var kimiHookEvents = map[string]bool{
	"PreToolUse":         true,
	"PostToolUse":        true,
	"PostToolUseFailure": true,
	"PermissionRequest":  true,
	"PermissionResult":   true,
	"UserPromptSubmit":   true,
	"UserPromptQueued":   true,
	"TurnStarted":        true,
	"Stop":               true,
	"StopFailure":        true,
	"Interrupt":          true,
	"SessionStart":       true,
	"SessionEnd":         true,
	"SessionHeartbeat":   true,
	"SubagentStart":      true,
	"SubagentStop":       true,
	"TaskStarted":        true,
	"PreCompact":         true,
	"PostCompact":        true,
	"Notification":       true,
}

// KimiHook is one hook selected for Kimi, flattened out of the registry into
// exactly what the config.toml entry and the copy step will need.
type KimiHook struct {
	Name       string
	Event      string
	Matcher    string
	Script     string // repo-relative, e.g. hooks/claude-code/secret-guard.sh
	FailClosed bool
	Timeout    int
}

// SelectKimi picks the hooks to install for Kimi, in registry order, and says
// why it left each of the others out.
//
// A hook is selected when it has a kimi block naming an event and a script,
// that block is enabled (its own override, else the hook's), and the name is
// not in disabled. skipped carries a reason for every other hook, because "not
// installed" and "installed and quiet" look identical afterwards: a hook Kimi
// cannot honour has to say so on the run that would otherwise have installed
// it.
func SelectKimi(registry Registry, disabled []string) (selected []KimiHook, skipped map[string]string) {
	isDisabled := func(name string) bool {
		for _, d := range disabled {
			if d == name {
				return true
			}
		}
		return false
	}

	skipped = map[string]string{}
	for _, h := range registry {
		spec, ok := h.Target(TargetKimi)
		switch {
		case !ok || spec.Script == "" || spec.Event == "":
			skipped[h.Name] = "no Kimi mapping in hooks/registry.json"
			continue
		case isDisabled(h.Name):
			skipped[h.Name] = "disabled"
			continue
		case !h.EnabledFor(TargetKimi):
			reason := spec.Reason
			if reason == "" {
				reason = "off for Kimi"
			}
			skipped[h.Name] = reason
			continue
		}
		timeout := spec.Timeout
		if timeout == 0 {
			timeout = kimiDefaultTimeout
		}
		selected = append(selected, KimiHook{
			Name:       h.Name,
			Event:      spec.Event,
			Matcher:    spec.Matcher,
			Script:     spec.Script,
			FailClosed: spec.FailClosed,
			Timeout:    timeout,
		})
	}
	return selected, skipped
}

// KimiNames lists the selected hooks' names, for the installer's summary line.
func KimiNames(selected []KimiHook) string {
	names := make([]string, len(selected))
	for i, h := range selected {
		names[i] = h.Name
	}
	return strings.Join(names, ", ")
}
