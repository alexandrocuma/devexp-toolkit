package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/chzyer/readline"

	"devexp/internal/ui"
)

// ── CLI target selection ──────────────────────────────────────────────────────
//
// devexp installs for any combination of the CLIs it finds, so the rule is
// split three ways and each part can be checked on its own: detection reports
// what is on PATH as a plain value, selection is pure over that value, and
// announcement only prints. Both the flag path (runInstall) and the
// interactive one (runWizard) go through the same three, so the rule lives in
// exactly one place.

type target string

const (
	targetClaude   target = "claude"
	targetOpencode target = "opencode"
	targetKimi     target = "kimi"
)

// allTargets is the canonical order. Detection, announcement, selection and
// installation all work in it, so output never depends on map iteration order
// or on the order the user happened to type --target in.
var allTargets = []target{targetClaude, targetOpencode, targetKimi}

// label is how a target is named to the user.
func (t target) label() string {
	switch t {
	case targetClaude:
		return "Claude Code"
	case targetOpencode:
		return "opencode"
	case targetKimi:
		return "Kimi Code CLI"
	}
	return string(t)
}

// parseTarget maps one --target value onto a target id.
func parseTarget(s string) (target, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "claude", "claude-code":
		return targetClaude, nil
	case "opencode":
		return targetOpencode, nil
	case "kimi", "kimi-code":
		return targetKimi, nil
	}
	return "", fmt.Errorf("unknown target %q (valid: %s)", s, targetIDs())
}

// targetIDs renders the accepted --target values for help and error text.
func targetIDs() string {
	ids := make([]string, len(allTargets))
	for i, t := range allTargets {
		ids[i] = string(t)
	}
	return strings.Join(ids, ", ")
}

// detection is what a PATH lookup found. Keeping it a value is what lets
// selectTargets stay pure and still explain why a CLI that is installed is not
// on offer.
type detection struct {
	claude   bool
	opencode bool
	kimi     kimiDetection
}

// has reports whether devexp can install for t.
func (d detection) has(t target) bool {
	switch t {
	case targetClaude:
		return d.claude
	case targetOpencode:
		return d.opencode
	case targetKimi:
		return d.kimi.status == kimiOK
	}
	return false
}

// available lists the targets devexp can install for, in canonical order.
func (d detection) available() []target {
	var out []target
	for _, t := range allTargets {
		if d.has(t) {
			out = append(out, t)
		}
	}
	return out
}

// notices explains every CLI that was found but cannot be used, so a Kimi that
// is installed and too old reads as skipped rather than as absent.
func (d detection) notices() []string {
	var out []string
	if n := d.kimi.notice(); n != "" {
		out = append(out, n)
	}
	return out
}

// why explains what stands in the way of a target that is not available.
func (d detection) why(t target) string {
	if t == targetKimi {
		if n := d.kimi.notice(); n != "" {
			return n
		}
	}
	return fmt.Sprintf("%s was not found on PATH", t.label())
}

// selectTargets maps what is installed, plus what the user chose, onto the
// targets to install for. It is pure — no PATH lookup, no printing, no prompt
// — so every combination is checkable here, including ones no single machine
// can reach.
//
// chosen distinguishes three cases on purpose. nil means the user did not
// choose, which is every detected target. A non-nil empty slice means the user
// chose nothing, which is an error: an empty selection means "everything"
// everywhere else in the installer (registry.go), and installing for every CLI
// because someone deselected them all is the opposite of what they asked for.
func selectTargets(d detection, chosen []target) ([]target, error) {
	available := d.available()
	if len(available) == 0 {
		return nil, fmt.Errorf("no supported CLI detected (%s)%s", targetIDs(), suffixNotices(d.notices()))
	}
	if chosen == nil {
		return available, nil
	}
	if len(chosen) == 0 {
		return nil, fmt.Errorf("no target selected")
	}
	for _, t := range chosen {
		if !slices.Contains(allTargets, t) {
			return nil, fmt.Errorf("unknown target %q (valid: %s)", string(t), targetIDs())
		}
		// Never quietly fall back to a different CLI: someone who asked for
		// one target and got another would not find out until they looked.
		if !d.has(t) {
			return nil, fmt.Errorf("cannot install for %s: %s", t.label(), d.why(t))
		}
	}
	var out []target
	for _, t := range allTargets {
		if slices.Contains(chosen, t) {
			out = append(out, t)
		}
	}
	return out, nil
}

// suffixNotices appends the found-but-unusable explanations to an error, so
// "no supported CLI detected" never hides a Kimi that is merely too old.
func suffixNotices(notices []string) string {
	if len(notices) == 0 {
		return ""
	}
	return " — " + strings.Join(notices, "; ")
}

// announceTargets reports what was detected and what was skipped. This is the
// I/O half: everything here touches the terminal and nothing here decides
// anything. It prints no trailing blank line — its callers do.
func announceTargets(d detection) {
	if available := d.available(); len(available) > 0 {
		labels := make([]string, len(available))
		for i, t := range available {
			labels[i] = t.label()
		}
		ui.Info("Detected: " + strings.Join(labels, ", "))
	}
	for _, n := range d.notices() {
		ui.Warn(n)
	}
}

// detectTargets looks up what is installed, and only that: announcing and
// deciding are separate steps so each caller can order them itself without
// duplicating the rule.
func detectTargets() detection {
	return detection{
		claude:   commandExists("claude"),
		opencode: commandExists("opencode"),
		kimi:     detectKimi(),
	}
}

// stdinIsTerminal reports whether there is anyone there to prompt. It asks the
// library promptui itself prompts through, so the answer is exactly "can a
// prompt be shown here": a mode check would not do, because os.ModeCharDevice
// is set for /dev/null too, and an install with stdin redirected from it would
// prompt and then fail reading the answer. A package var so tests can answer
// for it.
var stdinIsTerminal = func() bool {
	return readline.IsTerminal(int(os.Stdin.Fd()))
}

// promptTargets asks which of the available targets to install for. It returns
// a non-nil empty slice when everything was deselected — ui.MultiSelect
// returns nil there, and nil is what selectTargets reads as "all".
var promptTargets = func(available []target) ([]target, error) {
	labels := make([]string, len(available))
	for i, t := range available {
		labels[i] = t.label()
	}
	picked, err := ui.MultiSelect("Install for", labels)
	if err != nil {
		return nil, err
	}
	chosen := []target{}
	for i, label := range labels {
		if slices.Contains(picked, label) {
			chosen = append(chosen, available[i])
		}
	}
	return chosen, nil
}

// resolveTargets is the shell around selectTargets: --target wins outright,
// otherwise a terminal with more than one target to choose from gets a
// checklist, and anything else installs for every detected target.
//
// That last arm is what makes a non-interactive run work at all. promptui
// needs a TTY, so until now a machine with two CLIs and no terminal — CI, a
// piped install — reached the prompt, failed reading stdin and installed
// nothing.
func resolveTargets(d detection, flagTargets []string) ([]target, error) {
	if len(flagTargets) > 0 {
		chosen := make([]target, 0, len(flagTargets))
		for _, raw := range flagTargets {
			t, err := parseTarget(raw)
			if err != nil {
				return nil, err
			}
			chosen = append(chosen, t)
		}
		return selectTargets(d, chosen)
	}
	if available := d.available(); len(available) > 1 && stdinIsTerminal() {
		chosen, err := promptTargets(available)
		if err != nil {
			return nil, err
		}
		return selectTargets(d, chosen)
	}
	return selectTargets(d, nil)
}

// commandExists reports whether name is on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
