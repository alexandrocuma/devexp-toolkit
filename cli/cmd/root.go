// Package cmd is devexp's cobra command tree and the orchestration each
// command runs: resolving where the assets are, deciding what a run installs,
// and driving the per-asset installers in internal/.
//
// Everything here is unexported apart from Execute. The command tree is wired
// through package-level vars and init(), so there is no constructor to export
// and nothing outside main has a reason to reach in.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is overwritten at build time via:
//
//	-ldflags "-X devexp/cmd.version=v1.2.3"
//
// GoReleaser injects the tagged version; local `go build` leaves it as "dev".
var version = "dev"

var rootCmd = &cobra.Command{
	Use:     "devexp",
	Short:   "DevExp Framework — agents, skills, hooks, and MCPs for Claude Code, opencode & Kimi Code CLI",
	Version: version,
}

// Execute runs the command tree and is the only thing main calls. It exits
// the process on error rather than returning one: cobra has already printed
// the usage or the error by this point, so returning would only make main
// print it a second time.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
