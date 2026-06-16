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
		p := promptui.Select{
			Label: label,
			Items: buildMultiSelectDisplay(items, selected),
			Size:  14,
		}
		idx, _, err := p.Run()
		if err != nil {
			return nil, err
		}

		if applyMultiSelectChoice(selected, idx) {
			return collectSelected(items, selected), nil
		}
	}
}

// buildMultiSelectDisplay renders the checklist menu rows for the current
// selection: row 0 is the "Done" line with the live count, row 1 is "Toggle
// all", and the remaining rows mirror items with a ✓/○ marker per selected flag.
func buildMultiSelectDisplay(items []string, selected []bool) []string {
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
	return display
}

// applyMultiSelectChoice mutates selected in response to a menu choice and
// reports whether the user is done. idx 0 = Done, idx 1 = toggle-all
// (deselect everything if all are currently selected, otherwise select all),
// idx >= 2 toggles the item at idx-2.
func applyMultiSelectChoice(selected []bool, idx int) (done bool) {
	switch idx {
	case 0: // Done
		return true
	case 1: // Toggle all
		count := 0
		for _, s := range selected {
			if s {
				count++
			}
		}
		allSelected := count == len(selected)
		for i := range selected {
			selected[i] = !allSelected
		}
	default: // Toggle individual item
		selected[idx-2] = !selected[idx-2]
	}
	return false
}

// collectSelected returns the items whose selected flag is set, preserving order.
func collectSelected(items []string, selected []bool) []string {
	var result []string
	for i, s := range selected {
		if s {
			result = append(result, items[i])
		}
	}
	return result
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
