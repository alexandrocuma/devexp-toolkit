package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"devexp/internal/manifest"
	"devexp/internal/mcp"
	"devexp/internal/ui"
)

// ── Kimi Code CLI ─────────────────────────────────────────────────────────────
//
// Kimi installs MCP servers and, so far, nothing else: agents and skills are
// #113, hooks #114. Every run therefore says plainly what it did not install
// as well as what it did (notYetSupported in install.go), and a run that could
// only have installed the parts Kimi does not have yet still exits non-zero
// rather than being congratulated.
//
// Resolving the paths first is also where a $KIMI_CODE_HOME devexp must not
// write to is caught, before anything is read or written.

func doInstallKimi(opts *installOpts) error {
	p, err := kimiTargetPaths(os.Getenv("KIMI_CODE_HOME"), os.Getenv("HOME"), time.Now())
	if err != nil {
		return err
	}
	ui.Info("Installing for Kimi Code CLI...")
	fmt.Println()

	if opts.agentsOnly || opts.skillsOnly {
		ui.Warn("Nothing to install for Kimi Code CLI: it installs MCP servers only so far, and this run asked for agents or skills alone (#113).")
		fmt.Println()
		return nil
	}

	old := loadOldManifest(p.manifest)
	// The whole struct, so a field this target does not write — and a field a
	// later version adds — is carried forward rather than dropped (#108).
	newManifest := *old

	owned, err := installMCPsKimi(opts, p, old.MCPs)
	if err != nil {
		// An mcp.json devexp cannot merge into without losing what is there
		// costs only the MCP step; it is not a reason to fail the run. Today
		// that leaves nothing else to do, but #113 and #114 will.
		var refused *mcp.ConfigRefusedError
		if !errors.As(err, &refused) {
			return err
		}
		warnMCPsSkipped(refused, "mcpServers")
		fmt.Println()
	}
	newManifest.MCPs = owned

	if !opts.dryRun {
		if err := manifest.Save(p.manifest, &newManifest); err != nil {
			ui.Warn(fmt.Sprintf("save manifest: %v", err))
		}
	}

	ui.Success("Kimi Code CLI installation complete.")
	// Quoted: the path comes from $KIMI_CODE_HOME (#111).
	fmt.Printf("  MCPs   : %q\n", p.mcp)
	fmt.Println()
	ui.Info("Restart Kimi Code CLI to load the MCP servers.")
	fmt.Println()
	return nil
}

// installMCPsKimi merges the selected registry MCPs into Kimi's mcp.json and
// returns what devexp now owns there, for the manifest. On a refusal it
// returns what was owned before, so a file devexp could not read never loses
// devexp its record of what it put there.
func installMCPsKimi(opts *installOpts, p kimiPaths, owned map[string]string) (map[string]string, error) {
	registry, err := loadFullRegistry(opts.repoDir, opts.cfg)
	if err != nil {
		return owned, err
	}
	registry = filterMCPs(registry, opts.selectedMCPs)

	ui.Info(fmt.Sprintf("Installing MCP servers (Kimi → %q)...", p.mcp))
	newOwned, err := mcp.InstallKimi(registry, opts.env, p.mcp, owned, opts.dryRun, opts.reinstallMCPs)
	fmt.Println()
	return newOwned, err
}
