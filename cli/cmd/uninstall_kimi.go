package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"devexp/internal/hooks"
	"devexp/internal/manifest"
	"devexp/internal/mcp"
	"devexp/internal/ui"
)

// ── Uninstall (Kimi Code CLI) ─────────────────────────────────────────────────
//
// Unlike Claude Code and opencode, whose agents uninstall.sh matches by repo
// filename, nothing on disk in the Kimi root says which files are devexp's.
// The manifest is the entire ownership record, so the whole removal — agents,
// skills, MCP entries, hooks and the manifest itself — happens here, in the
// same package as the install, by the same rules.
//
// Every step reuses the function the install already goes through rather than
// growing a second set of rules that could drift from it:
//
//   - hooks:  hooks.UninstallKimi, written alongside InstallKimi in #114;
//   - skills
//     and agents: removeStale with installed = nil, which makes every
//     recorded name stale. It already carries the #128 removal rules — an
//     os.Root handle, names read once, an exact byte-for-byte name match, no
//     symlinked entry, nothing removed through a symlinked or linked parent
//     directory — and it hands back what had to stay;
//   - MCPs:   mcp.InstallKimi with no registry at all, which selects nothing,
//     so pruneKimiEntries removes every entry devexp still owns and leaves
//     one the user has edited since.
//
// Nothing here fails the run on its own: what could not be removed stays in
// the manifest so a later run can finish. Only a refusal to resolve the Kimi
// root is an error, and then nothing has been touched.

// doUninstallKimi removes what the manifest in the Kimi root records. home is
// the user's home directory, already validated by runUninstall; the Kimi root
// itself comes from $KIMI_CODE_HOME, exactly as it does on install.
func doUninstallKimi(home string, dryRun bool) error {
	p, err := kimiTargetPaths(os.Getenv("KIMI_CODE_HOME"), home, time.Now())
	if err != nil {
		return err
	}

	// Only a regular file counts as a manifest to act on, for the same reason
	// doUninstallOpencode requires one: a symlink is never written through,
	// and its contents describe files somewhere else.
	manifestInfo, statErr := os.Lstat(p.manifest)
	if os.IsNotExist(statErr) {
		ui.Info(fmt.Sprintf("No devexp install recorded for Kimi Code CLI (%q) — nothing to remove.", p.manifest))
		return nil
	}

	// An unreadable or malformed manifest yields an empty one, and an empty
	// manifest owns nothing. That is the whole point: with no other record of
	// what devexp wrote, guessing would mean deleting the user's files.
	old := loadOldManifest(p.manifest)
	kept := *old

	ui.Info(fmt.Sprintf("Removing devexp from Kimi Code CLI (%q)...", p.root))
	fmt.Println()

	// Hooks first, and inside UninstallKimi the registration goes before the
	// scripts: a registered command whose script is missing is a silent allow
	// under Kimi's runner, so the config entry must never outlive its script.
	keptHooks, err := hooks.UninstallKimi(p.hooks, p.config, old.Hooks, dryRun)
	if err != nil {
		// config.toml could not be rewritten. The scripts stay on disk with
		// it, because removing them under a live registration is the one
		// ordering that opens a hole.
		ui.Warn(fmt.Sprintf("Kimi hooks left registered (%v) — %q was not changed; re-run once it is readable.", err, p.config))
	}
	kept.Hooks = keptHooks

	// Skills before agents, the reverse of the install order, so a run
	// interrupted part-way never leaves a skill whose agent reference is gone.
	kept.Skills = removeStale(p.home, p.skills, old.Skills, nil, staleDir, (*os.Root).RemoveAll, dryRun)
	kept.Agents = removeStale(p.home, p.agents, old.Agents, nil, staleFile, (*os.Root).Remove, dryRun)

	// No registry, so nothing is selected and every owned entry is pruned —
	// each one still guarded by the fingerprint the manifest recorded, so an
	// entry the user changed since is left in place and stops being devexp's.
	if len(old.MCPs) > 0 {
		ui.Info(fmt.Sprintf("Removing MCP servers (Kimi → %q)...", p.mcp))
		remaining, err := mcp.InstallKimi(nil, nil, p.mcp, old.MCPs, dryRun, false)
		var refused *mcp.ConfigRefusedError
		if errors.As(err, &refused) {
			// The file could not be read without losing what is in it. Keep
			// the ownership record: dropping it would make devexp read its own
			// entries as the user's on the next install and never touch them.
			ui.Warn(fmt.Sprintf("%q could not be read (%v) — its MCP entries were left in place and are still recorded as devexp's.", p.mcp, refused.Reason))
		} else if err != nil {
			ui.Warn(fmt.Sprintf("MCP servers: %v", err))
		}
		kept.MCPs = remaining
		fmt.Println()
	}

	if dryRun {
		ui.DryRun("remove " + p.manifest)
		fmt.Println()
		ui.Info("Dry run — nothing above was removed.")
		return nil
	}

	saveKimiUninstallRecord(p, old, &kept, manifestInfo, statErr)
	reportKimiLeftovers(p, &kept)
	return nil
}

// saveKimiUninstallRecord deletes the manifest when everything it recorded is
// gone, and otherwise rewrites it with only what had to stay — so a file the
// user later puts under a name devexp once installed is never taken for
// devexp's, and a later run can still finish what this one could not.
//
// It is skipped entirely when the manifest could not be stat'd or is not a
// regular file: writing through a symlink would change a file somewhere else,
// such as a dotfiles checkout.
func saveKimiUninstallRecord(p kimiPaths, old, kept *manifest.Manifest, info os.FileInfo, statErr error) {
	if statErr != nil {
		ui.Warn(fmt.Sprintf("manifest %q could not be checked (%v), so it was left as it is", p.manifest, statErr))
		return
	}
	if !info.Mode().IsRegular() {
		ui.Warn(fmt.Sprintf("manifest %q is a symlink, so it was left untouched — it no longer matches what is on disk", p.manifest))
		return
	}
	if kimiRecordEmpty(kept) {
		if err := os.Remove(p.manifest); err != nil {
			ui.Warn(fmt.Sprintf("could not remove manifest %q: %v", p.manifest, err))
			return
		}
		ui.Removed(p.manifest)
		return
	}
	if kimiRecordSame(old, kept) {
		return
	}
	if err := manifest.Save(p.manifest, kept); err != nil {
		ui.Warn(fmt.Sprintf("save manifest: %v", err))
	}
}

// kimiRecordEmpty reports whether the manifest still records anything devexp
// owns in the Kimi root. Only the four fields a Kimi install writes count:
// Plugins is opencode's and is carried forward untouched, and a manifest that
// holds only it has nothing of Kimi's left.
func kimiRecordEmpty(m *manifest.Manifest) bool {
	return len(m.Agents) == 0 && len(m.Skills) == 0 && len(m.Hooks) == 0 && len(m.MCPs) == 0
}

// kimiRecordSame reports whether nothing about the Kimi fields changed, so an
// uninstall that removed nothing does not rewrite the file.
func kimiRecordSame(old, kept *manifest.Manifest) bool {
	if !slices.Equal(old.Agents, kept.Agents) ||
		!slices.Equal(old.Skills, kept.Skills) ||
		!slices.Equal(old.Hooks, kept.Hooks) ||
		len(old.MCPs) != len(kept.MCPs) {
		return false
	}
	for name, hash := range old.MCPs {
		if kept.MCPs[name] != hash {
			return false
		}
	}
	return true
}

// reportKimiLeftovers closes the run by saying what is still there. Two things
// survive on purpose and would otherwise look like a bug: the pre-install
// copies devexp made, which are the user's own files, and the agent memory
// directory, which Claude Code shares.
func reportKimiLeftovers(p kimiPaths, kept *manifest.Manifest) {
	fmt.Println()
	if !kimiRecordEmpty(kept) {
		ui.Warn("Some entries were left in place (see above); they stay recorded so a later run can finish.")
	} else {
		ui.Success(fmt.Sprintf("Removed devexp from Kimi Code CLI (%q).", p.root))
	}
	ui.Info(fmt.Sprintf("Kept on purpose: %q (your pre-install copies) and ~/.claude/agent-memory (shared with Claude Code).", p.root+"/.devexp-backup-*"))
	ui.Info("Restart Kimi Code CLI to finish.")
}
