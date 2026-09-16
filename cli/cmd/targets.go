package cmd

import (
	"fmt"
	"os/exec"

	"devexp/internal/ui"
)

// ── CLI auto-detect (flag path) ───────────────────────────────────────────────

func detectTargets() (claude, opencode bool, err error) {
	hasClaude := commandExists("claude")
	hasOpencode := commandExists("opencode")

	switch {
	case hasClaude && hasOpencode:
		ui.Info("Detected: Claude Code and opencode")
		fmt.Println()
		choice, err := ui.SelectPlatform()
		if err != nil {
			return false, false, err
		}
		return choice == "Claude Code" || choice == "Both",
			choice == "opencode" || choice == "Both",
			nil
	case hasClaude:
		ui.Info("Detected: Claude Code")
		return true, false, nil
	case hasOpencode:
		ui.Info("Detected: opencode")
		return false, true, nil
	default:
		return false, false, fmt.Errorf("no supported CLI detected (claude or opencode)")
	}
}

// commandExists reports whether name is on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
