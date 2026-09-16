package cmd

import (
	"path/filepath"
	"time"
)

// ── Install target paths ──────────────────────────────────────────────────────
//
// Both install paths derived every destination inline from $HOME, which fused
// trivially checkable string assembly into functions that also talk to the
// filesystem. These are pure: given a home directory (and, for the backup
// directory, a timestamp) they return where things go, and touch nothing.

// claudePaths holds every destination the Claude Code install writes to.
type claudePaths struct {
	agents   string
	skills   string
	settings string
	manifest string
	backup   string
}

// claudeTargetPaths resolves the Claude Code destinations under home. now is
// passed in rather than read from the clock so the backup directory's name is
// assertable.
func claudeTargetPaths(home string, now time.Time) claudePaths {
	return claudePaths{
		agents:   filepath.Join(home, ".claude", "agents"),
		skills:   filepath.Join(home, ".claude", "skills"),
		settings: filepath.Join(home, ".claude", "settings.json"),
		manifest: filepath.Join(home, ".claude", ".devexp-manifest.json"),
		backup:   filepath.Join(home, ".claude", ".devexp-backup-"+now.Format("20060102T150405")),
	}
}

// opencodePaths holds every destination the opencode install writes to.
type opencodePaths struct {
	agents   string
	skills   string
	config   string
	manifest string
}

// opencodeTargetPaths resolves the opencode destinations under home. Skills
// land in commands/, which is why the field and the directory differ.
func opencodeTargetPaths(home string) opencodePaths {
	return opencodePaths{
		agents:   filepath.Join(home, ".config", "opencode", "agents"),
		skills:   filepath.Join(home, ".config", "opencode", "commands"),
		config:   filepath.Join(home, ".config", "opencode", "config.json"),
		manifest: filepath.Join(home, ".config", "opencode", ".devexp-manifest.json"),
	}
}
