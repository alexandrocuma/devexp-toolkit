package ui

import (
	"fmt"

	"github.com/manifoldco/promptui"
)

// ── Action ────────────────────────────────────────────────────────────────────

// Action is the index of the chosen entry in SelectAction's menu, so the
// constants below and that menu's Items must stay in the same order.
type Action int

// The actions, in the order SelectAction lists them. iota ties each value to
// its menu position, so reordering these without reordering that menu
// silently changes what every answer means.
const (
	ActionInstall Action = iota
	ActionReinstallMCPs
	ActionRemove
	ActionDryRun
)

// SelectAction asks what the run should do. The returned Action is the menu
// index, valid only when err is nil — on a cancelled prompt promptui returns
// index 0, which is ActionInstall and would otherwise read as a choice.
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

// Scope is the index of the chosen entry in SelectScope's menu, under the
// same ordering constraint as Action.
type Scope int

// The scopes, in the order SelectScope lists them, under the same constraint
// as the actions above.
const (
	ScopeFull Scope = iota
	ScopeMCPsOnly
	ScopeAgentsOnly
	ScopeSkillsOnly
)

// SelectScope asks which kinds of asset to install. The returned Scope is
// the menu index and, as with SelectAction, means nothing unless err is nil.
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

// ── Multi-select ──────────────────────────────────────────────────────────────

// MultiSelect shows a toggleable checklist; all items start selected.
// "Done" confirms, "Toggle all" flips every item. Returns the selected names,
// always as a non-nil slice: callers distinguish "was not asked" (nil) from
// "was asked and picked nothing" (empty), and reading the second as the first
// would install everything to someone who deliberately unticked it all.
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
	case 0:
		return true
	case 1:
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
	default:
		selected[idx-2] = !selected[idx-2]
	}
	return false
}

// collectSelected returns the items whose selected flag is set, preserving
// order. The result is never nil — see MultiSelect on why an empty pick has to
// stay tellable apart from no pick at all.
func collectSelected(items []string, selected []bool) []string {
	result := []string{}
	for i, s := range selected {
		if s {
			result = append(result, items[i])
		}
	}
	return result
}

// ── Confirm ───────────────────────────────────────────────────────────────────

// Confirm asks a yes/no question, reading a declined answer as (false, nil)
// rather than an error: promptui reports a "no" the same way it reports a
// broken terminal, and only the second is a failure the caller should report.
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
