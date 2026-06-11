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

func Info(msg string)    { fmt.Printf("%s[devexp]%s %s\n", colorBlue, colorReset, msg) }
func Success(msg string) { fmt.Printf("%s[devexp]%s %s\n", colorGreen, colorReset, msg) }
func Warn(msg string)    { fmt.Printf("%s[devexp]%s %s\n", colorYellow, colorReset, msg) }
func Error(msg string)   { fmt.Fprintf(os.Stderr, "%s[devexp] ERROR:%s %s\n", colorRed, colorReset, msg) }

func Added(name string)           { fmt.Printf("  %s+%s %s\n", colorGreen, colorReset, name) }
func Removed(name string)         { fmt.Printf("  %s-%s %s\n", colorRed, colorReset, name) }
func Updated(name string)         { fmt.Printf("  %s~%s %s — updated\n", colorYellow, colorReset, name) }
func Skipped(name, reason string) { fmt.Printf("  [skip] %s — %s\n", name, reason) }
func DryRun(msg string)           { fmt.Printf("  %s[dry-run]%s %s\n", colorYellow, colorReset, msg) }
func Required(name string, keys []string) {
	fmt.Printf("\n  %s[REQUIRED]%s %s — missing required env vars:\n", colorRed, colorReset, name)
	for _, k := range keys {
		fmt.Printf("    %s=<your-value>\n", k)
	}
}
