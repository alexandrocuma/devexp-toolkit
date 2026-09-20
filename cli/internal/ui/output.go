// Package ui is the one place devexp decides what a run looks like: two
// levels of line, and which stream each goes to.
//
// A heading line carries the "[devexp]" tag and sits flush left (Info,
// Success, Warn, Error). An item line is indented two spaces and belongs
// under the heading above it (Added, Removed, Updated, Skipped, DryRun,
// Required). Only Error writes to stderr.
//
// The colours are raw ANSI with no terminal detection, so output redirected
// to a file keeps the escapes. Tests assert on whole lines rather than
// re-deriving them — see AddedLine.
package ui

import (
	"fmt"
	"os"
)

const (
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorBlue   = "\033[0;34m"
	colorBold   = "\033[1m"
	colorReset  = "\033[0m"
)

// Info is the neutral heading: it reports what a step is about to do.
func Info(msg string) { fmt.Printf("%s[devexp]%s %s\n", colorBlue, colorReset, msg) }

// Success is the heading for a step that finished having changed something.
func Success(msg string) { fmt.Printf("%s[devexp]%s %s\n", colorGreen, colorReset, msg) }

// Warn is a heading for a degraded run, not a failed one — it never affects
// the exit status, so a caller that keeps going must say so itself.
func Warn(msg string) { fmt.Printf("%s[devexp]%s %s\n", colorYellow, colorReset, msg) }

// Error is the only line in this package that goes to stderr, so it still
// reaches the user when stdout is piped or captured. It does not exit.
func Error(msg string) { fmt.Fprintf(os.Stderr, "%s[devexp] ERROR:%s %s\n", colorRed, colorReset, msg) }

// Added is the item line for something this run created.
func Added(name string) { fmt.Print(AddedLine(name)) }

// Removed is the item line for something this run deleted.
func Removed(name string) { fmt.Printf("  %s-%s %s\n", colorRed, colorReset, name) }

// Updated is the item line for an existing entry this run overwrote.
func Updated(name string) { fmt.Printf("  %s~%s %s — updated\n", colorYellow, colorReset, name) }

// Skipped is the item line for something deliberately left alone. The reason
// is not optional: a silent skip is indistinguishable from a bug.
func Skipped(name, reason string) { fmt.Printf("  [skip] %s — %s\n", name, reason) }

// DryRun is the item line for work a preview run described but did not do.
func DryRun(msg string) { fmt.Printf("  %s[dry-run]%s %s\n", colorYellow, colorReset, msg) }

// Required lists the env vars an MCP needs before it will work, one per line
// in `KEY=<your-value>` form so the user can paste them into mcps/.env. It
// opens with a blank line because it interrupts a list of item lines.
func Required(name string, keys []string) {
	fmt.Printf("\n  %s[REQUIRED]%s %s — missing required env vars:\n", colorRed, colorReset, name)
	for _, k := range keys {
		fmt.Printf("    %s=<your-value>\n", k)
	}
}

// AddedLine is the exact line Added prints, so a test can assert that
// something was NOT reported as installed without re-deriving the escapes.
func AddedLine(name string) string {
	return fmt.Sprintf("  %s+%s %s\n", colorGreen, colorReset, name)
}
