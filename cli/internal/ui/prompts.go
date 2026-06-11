package ui

import (
	"fmt"

	"github.com/manifoldco/promptui"
)

// ── Action ────────────────────────────────────────────────────────────────────

type Action int

const (
	ActionInstall Action = iota
	ActionReinstallMCPs
	ActionRemove
	ActionDryRun
)

func SelectAction() (Action, error) {
	p := promptui.Select{
		Label: "Action",
		Items: []string{
			"Install / Update",
			"Reinstall MCPs  (remove → re-add)",
			"Remove devexp",
			"Dry-run  (preview only, no changes)",
		},
	}
	idx, _, err := p.Run()
	return Action(idx), err
}

// ── Scope ─────────────────────────────────────────────────────────────────────

type Scope int

const (
	ScopeFull Scope = iota
	ScopeMCPsOnly
	ScopeAgentsOnly
	ScopeSkillsOnly
)

func SelectScope() (Scope, error) {
	p := promptui.Select{
		Label: "Scope",
		Items: []string{
			"Everything  (MCPs + Agents + Skills + Hooks)",
			"MCPs only",
			"Agents only",
			"Skills only",
		},
	}
	idx, _, err := p.Run()
	return Scope(idx), err
}

// ── Platform ──────────────────────────────────────────────────────────────────

func SelectPlatform() (string, error) {
	p := promptui.Select{
		Label: "Platform",
		Items: []string{"Claude Code", "opencode", "Both"},
	}
	_, result, err := p.Run()
	return result, err
}

// SelectCLI is an alias kept for backward compatibility.
func SelectCLI() (string, error) { return SelectPlatform() }

// ── Multi-select ──────────────────────────────────────────────────────────────

// MultiSelect shows a toggleable checklist. All items start selected.
// The user navigates with arrows and presses Enter to toggle items.
// Selecting "Done" or "Toggle all" are special actions.
// Returns the names of all selected items.
func MultiSelect(label string, items []string) ([]string, error) {
	selected := make([]bool, len(items))
	for i := range selected {
		selected[i] = true
	}

	for {
		count := 0
		for _, s := range selected {
			if s {
				count++
			}
		}

		display := make([]string, len(items)+2)
		display[0] = fmt.Sprintf("✔  Done  (%d / %d selected)", count, len(items))
		display[1] = "◎  Toggle all"
		for i, item := range items {
			if selected[i] {
				display[i+2] = "✓  " + item
			} else {
				display[i+2] = "○  " + item
			}
		}

		p := promptui.Select{
			Label: label,
			Items: display,
			Size:  14,
		}
		idx, _, err := p.Run()
		if err != nil {
			return nil, err
		}

		switch idx {
		case 0: // Done
			var result []string
			for i, s := range selected {
				if s {
					result = append(result, items[i])
				}
			}
			return result, nil
		case 1: // Toggle all
			allSelected := count == len(items)
			for i := range selected {
				selected[i] = !allSelected
			}
		default: // Toggle individual item
			selected[idx-2] = !selected[idx-2]
		}
	}
}

// ── Confirm ───────────────────────────────────────────────────────────────────

func Confirm(label string) (bool, error) {
	p := promptui.Prompt{
		Label:     label,
		IsConfirm: true,
	}
	_, err := p.Run()
	if err == promptui.ErrAbort {
		return false, nil
	}
	return err == nil, err
}
