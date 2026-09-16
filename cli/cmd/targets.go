package cmd

import (
	"fmt"
	"os/exec"

	"devexp/internal/ui"
)

// ── CLI target selection ──────────────────────────────────────────────────────

// selectTargets maps CLI availability, plus the user's answer when both are
// present, onto the two install flags.
//
// It is deliberately pure: no PATH lookup, no printing, no prompt. Both the
// flag path (detectTargets) and the interactive path (runWizard) resolved
// targets with their own copy of this switch, so the rule lived in two places
// and could drift. choice is ignored unless both CLIs are present, and comes
// from ui.SelectPlatform.
func selectTargets(hasClaude, hasOpencode bool, choice string) (claude, opencode bool, err error) {
	switch {
	case hasClaude && hasOpencode:
		return choice == "Claude Code" || choice == "Both",
			choice == "opencode" || choice == "Both",
			nil
	case hasClaude:
		return true, false, nil
	case hasOpencode:
		return false, true, nil
	default:
		return false, false, fmt.Errorf("no supported CLI detected (claude or opencode)")
	}
}

// announceTargets reports what was detected and, when both CLIs are present,
// asks which to install for. It returns the raw choice for selectTargets;
// an empty string means no prompt was shown.
//
// This is the I/O half: everything here touches the terminal, and nothing here
// decides anything.
func announceTargets(hasClaude, hasOpencode bool) (choice string, err error) {
	switch {
	case hasClaude && hasOpencode:
		ui.Info("Detected: Claude Code and opencode")
		fmt.Println()
		return ui.SelectPlatform()
	case hasClaude:
		ui.Info("Detected: Claude Code")
	case hasOpencode:
		ui.Info("Detected: opencode")
	}
	return "", nil
}

// detectTargets is the flag path's shell: look up what's installed, announce
// it, then decide. Note it prints no trailing blank line — runInstall does.
func detectTargets() (claude, opencode bool, err error) {
	hasClaude := commandExists("claude")
	hasOpencode := commandExists("opencode")

	choice, err := announceTargets(hasClaude, hasOpencode)
	if err != nil {
		return false, false, err
	}
	return selectTargets(hasClaude, hasOpencode, choice)
}

// commandExists reports whether name is on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
