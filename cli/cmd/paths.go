package cmd

import (
	"fmt"
	"path/filepath"
	"time"
)

// ── Install target paths ──────────────────────────────────────────────────────
//
// Both install paths derived every destination inline from $HOME, which fused
// trivially checkable string assembly into functions that also talk to the
// filesystem. These are pure: given a home directory (and, for the backup
// directory, a timestamp) they return where things go, and touch nothing.
//
// They refuse a home that is empty or relative rather than return a relative
// path: HOME unset, empty or relative would otherwise point every destination
// at whatever directory the command happens to run in (#126).

// targetHome refuses a HOME that is unset, empty or relative, and returns it
// cleaned. Every install and uninstall target is built from it, so both
// commands check it before touching anything, and the path helpers below check
// it again so no caller can get a relative target path.
func targetHome(home string) (string, error) {
	if home == "" || !filepath.IsAbs(home) {
		return "", fmt.Errorf("HOME is %q, not an absolute path", home)
	}
	return filepath.Clean(home), nil
}

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
func claudeTargetPaths(home string, now time.Time) (claudePaths, error) {
	home, err := targetHome(home)
	if err != nil {
		return claudePaths{}, err
	}
	return claudePaths{
		agents:   filepath.Join(home, ".claude", "agents"),
		skills:   filepath.Join(home, ".claude", "skills"),
		settings: filepath.Join(home, ".claude", "settings.json"),
		manifest: filepath.Join(home, ".claude", ".devexp-manifest.json"),
		backup:   filepath.Join(home, ".claude", ".devexp-backup-"+now.Format("20060102T150405")),
	}, nil
}

// opencodePaths holds every destination the opencode install writes to.
type opencodePaths struct {
	agents   string
	skills   string
	plugins  string
	config   string
	manifest string
}

// opencodeTargetPaths resolves the opencode destinations under home. Skills
// land in commands/, which is why the field and the directory differ.
func opencodeTargetPaths(home string) (opencodePaths, error) {
	home, err := targetHome(home)
	if err != nil {
		return opencodePaths{}, err
	}
	return opencodePaths{
		agents:   filepath.Join(home, ".config", "opencode", "agents"),
		skills:   filepath.Join(home, ".config", "opencode", "commands"),
		plugins:  filepath.Join(home, ".config", "opencode", "plugins"),
		config:   filepath.Join(home, ".config", "opencode", "config.json"),
		manifest: filepath.Join(home, ".config", "opencode", ".devexp-manifest.json"),
	}, nil
}
