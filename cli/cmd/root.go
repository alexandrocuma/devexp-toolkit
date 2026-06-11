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
	Short:   "DevExp Framework — agents, skills, hooks, and MCPs for Claude Code & opencode",
	Version: version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
