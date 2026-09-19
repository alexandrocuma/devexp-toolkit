package hooks

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"devexp/internal/fsutil"
	"devexp/internal/ui"
)

// ── Installing the Kimi hooks: the scripts, then the registration ─────────────
//
// Claude Code and opencode both run devexp's hooks out of the repo checkout;
// Kimi does not. Its config.toml holds a plain shell command, the user may
// move or delete the checkout, and a hook whose command cannot be run is read
// by Kimi's runner as an ALLOW (runHook, see kimi.go). So the guards are
// COPIED into the Kimi root and registered from there.
//
// Copy first, register second, and never the other way round: between the two
// writes there is a config.toml naming scripts that do not exist yet, and
// that window has to be the short one at the end of a successful install
// rather than the permanent state of a failed one.
//
// What is copied is not only the selected guards:
//
//   - kimi/adapter.sh, which every registered command invokes;
//   - claude-code/scan-budget.sh, which each guard sources as
//     "${BASH_SOURCE[0]%/*}/scan-budget.sh" — it has to sit NEXT TO the
//     guards, and a guard that cannot read it exits 1, which Kimi reads as an
//     allow. It is copied whenever any guard is.
//
// Everything is tracked in the manifest, so a guard devexp stops shipping is
// removed rather than left behind for a stale command to find.

// kimiAdapterFile is the adapter's path below the installed hooks directory.
const kimiAdapterFile = "kimi/adapter.sh"

// kimiScanBudgetFile is the helper every guard sources from its own directory.
const kimiScanBudgetFile = "claude-code/scan-budget.sh"

// kimiSupportFiles are copied alongside the selected guards, in this order:
// the adapter the command line names, and the scan budget the guards source.
var kimiSupportFiles = []string{kimiAdapterFile, kimiScanBudgetFile}

// InstallKimi copies the hooks Kimi is getting into hooksDir and registers
// them in configPath, and returns the paths it now owns below hooksDir — the
// manifest's record, which is also what a later run prunes against.
//
// recorded is what the previous run owned. Anything in it this run does not
// install is removed, and what could not be removed is returned so the record
// survives and a later run can finish the job.
//
// On an error the paths written SO FAR come back with it. A half-finished
// install has real files on disk, and the caller has to record them: they are
// the only trace devexp keeps of what it put in the Kimi root.
func InstallKimi(registry Registry, repoDir, home, hooksDir, configPath string, disabled, recorded []string, dryRun bool) ([]string, error) {
	if !filepath.IsAbs(hooksDir) {
		return nil, fmt.Errorf("hooks: the installed hooks directory %q is not absolute, so Kimi — which runs a hook from whatever directory it is in — could not find the scripts; refusing to install them", hooksDir)
	}

	selected, skipped := SelectKimi(registry, disabled)
	// In registry order, not map order, so two runs read the same.
	for _, h := range registry {
		if reason, ok := skipped[h.Name]; ok {
			ui.Skipped(h.Name, reason)
		}
	}

	if len(selected) == 0 {
		// Nothing to register. The block goes, and so do the scripts: a
		// command in a config.toml devexp no longer writes would still be run.
		if _, err := RemoveKimiHooks(configPath, hooksDir, dryRun); err != nil {
			return recorded, err
		}
		kept := removeKimiFiles(home, hooksDir, recorded, dryRun)
		ui.Skipped("Kimi hooks", "every hook devexp could give Kimi is disabled — nothing to install")
		return kept, nil
	}

	wanted := kimiInstallFiles(selected)
	written := make([]string, 0, len(wanted))
	for _, rel := range wanted {
		src := filepath.Join(repoDir, "hooks", filepath.FromSlash(rel))
		changed, err := copyKimiFile(src, hooksDir, rel, dryRun)
		if err != nil {
			// Registering now would name scripts that are not all there, and
			// a missing script is an allow. Hand back what is already on disk
			// so the manifest records it.
			return append(written, staleNew(written, recorded)...), fmt.Errorf("hooks: %w", err)
		}
		written = append(written, rel)
		switch {
		case dryRun:
			ui.DryRun("write " + filepath.Join(hooksDir, filepath.FromSlash(rel)))
		case changed:
			ui.Added(rel)
		default:
			ui.Skipped(rel, "already up to date")
		}
	}

	if _, err := WriteKimiHooks(configPath, hooksDir, selected, dryRun); err != nil {
		// The scripts are on disk and unregistered, which is inert; record
		// them so the next run — or UninstallKimi — can clean up.
		return append(written, staleNew(written, recorded)...), err
	}

	kept := removeKimiFiles(home, hooksDir, staleNew(written, recorded), dryRun)
	ui.Success(fmt.Sprintf("Kimi hooks (%d): %s", len(selected), KimiNames(selected)))
	return append(written, kept...), nil
}

// UninstallKimi takes the block out of config.toml and removes the scripts
// devexp recorded, returning what is still on disk. It is the removal half of
// InstallKimi, kept here so the two cannot drift apart (#115).
func UninstallKimi(home, hooksDir, configPath string, recorded []string, dryRun bool) ([]string, error) {
	changed, err := RemoveKimiHooks(configPath, hooksDir, dryRun)
	if err != nil {
		return recorded, err
	}
	kept := removeKimiFiles(home, hooksDir, recorded, dryRun)
	if !changed && len(recorded) == 0 {
		ui.Skipped("Kimi hooks", "not installed")
	}
	return kept, nil
}

// kimiInstallFiles lists what has to be on disk for selected to work: the
// support files first, then each guard, with duplicates dropped (two hooks
// may name the same script).
func kimiInstallFiles(selected []KimiHook) []string {
	out := slices.Clone(kimiSupportFiles)
	for _, h := range selected {
		rel := path.Clean(strings.TrimPrefix(h.Script, "hooks/"))
		if !slices.Contains(out, rel) {
			out = append(out, rel)
		}
	}
	return out
}

// staleNew returns the entries of old that installed does not contain.
func staleNew(installed, old []string) []string {
	var out []string
	for _, name := range old {
		if !slices.Contains(installed, name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// copyKimiFile copies one script into hooksDir, keeping the source's mode —
// the executable bit above all, because a guard is also run directly by the
// adapter's `bash "$guard"`, and because a file the user cannot read is a
// guard that exits 1, which Kimi allows.
//
// It reports whether the destination changed, so a second install says
// "already up to date" rather than claiming work it did not do.
func copyKimiFile(src, hooksDir, rel string, dryRun bool) (bool, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return false, fmt.Errorf("hook script %q could not be read, so it was not installed for Kimi (%w) — with no script on disk Kimi's runner reads the hook as an allow, so nothing was registered either", src, err)
	}
	info, err := os.Stat(src)
	if err != nil {
		return false, fmt.Errorf("hook script %q could not be stat'd (%w)", src, err)
	}
	// 0500 at the very least: readable and executable by its owner. A source
	// checkout with a slack umask must not widen what lands in the Kimi root,
	// which sits next to the user's credentials, so the mode is masked to the
	// owner's bits.
	mode := info.Mode().Perm()&0o700 | 0o500

	dst := filepath.Join(hooksDir, filepath.FromSlash(rel))
	if old, err := os.ReadFile(dst); err == nil && string(old) == string(data) {
		if st, err := os.Stat(dst); err == nil && st.Mode().Perm() == mode {
			return false, nil
		}
	}
	if dryRun {
		return true, nil
	}
	// 0700 like the rest of the Kimi root: what lives beside it is the user's
	// credentials.
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return false, fmt.Errorf("hook script %q: %w", dst, err)
	}
	if err := fsutil.WriteFileAtomic(dst, data, mode); err != nil {
		return false, fmt.Errorf("hook script %q: %w", dst, err)
	}
	// WriteFileAtomic keeps an existing file's own mode; force ours, because
	// a guard left non-executable is a guard Kimi allows around.
	if err := os.Chmod(dst, mode); err != nil {
		return false, fmt.Errorf("hook script %q: %w", dst, err)
	}
	return true, nil
}
