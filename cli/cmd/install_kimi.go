package cmd

import (
	"fmt"
	"os"
	"time"

	"devexp/internal/ui"
)

// ── Kimi Code CLI ─────────────────────────────────────────────────────────────
//
// Kimi is selectable but installs nothing yet: #112 adds MCPs, #113 agents and
// skills, #114 hooks. Until those land this must be impossible to mistake for
// a successful install, so it resolves the paths it would use — which is also
// where a broken $KIMI_CODE_HOME is caught — says plainly that it wrote
// nothing, and leaves runInstall to refuse "All done." for a run that got no
// further than here.

func doInstallKimi(opts *installOpts) error {
	p, err := kimiTargetPaths(os.Getenv("KIMI_CODE_HOME"), os.Getenv("HOME"), time.Now())
	if err != nil {
		return err
	}
	ui.Info("Installing for Kimi Code CLI...")
	fmt.Println()
	ui.Warn("Kimi Code CLI is not a supported install target yet (#110) — nothing was installed.")
	fmt.Printf("  Nothing was written to %s\n", p.root)
	fmt.Println("  Agents, skills, MCPs and hooks arrive in #112-#114.")
	fmt.Println()
	return nil
}
