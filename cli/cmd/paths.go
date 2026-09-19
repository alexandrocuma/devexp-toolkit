package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
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
	home     string // cleaned; stale removal resolves symlinks from here
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
		home:     home,
		agents:   filepath.Join(home, ".claude", "agents"),
		skills:   filepath.Join(home, ".claude", "skills"),
		settings: filepath.Join(home, ".claude", "settings.json"),
		manifest: filepath.Join(home, ".claude", ".devexp-manifest.json"),
		backup:   filepath.Join(home, ".claude", ".devexp-backup-"+now.Format("20060102T150405")),
	}, nil
}

// opencodePaths holds every destination the opencode install writes to.
type opencodePaths struct {
	home     string // cleaned; stale removal resolves symlinks from here
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
		home:     home,
		agents:   filepath.Join(home, ".config", "opencode", "agents"),
		skills:   filepath.Join(home, ".config", "opencode", "commands"),
		plugins:  filepath.Join(home, ".config", "opencode", "plugins"),
		config:   filepath.Join(home, ".config", "opencode", "config.json"),
		manifest: filepath.Join(home, ".config", "opencode", ".devexp-manifest.json"),
	}, nil
}

// ── Kimi Code CLI paths ───────────────────────────────────────────────────────
//
// Kimi Code keeps everything under one configurable root, so unlike the other
// two targets its paths do not hang off $HOME. Nothing writes to them yet —
// #112-#114 fill that in — but they are resolved and refused here so a
// misconfigured $KIMI_CODE_HOME is a refusal now rather than a surprise
// removal later.

// kimiPaths holds every destination the Kimi Code CLI install will write to.
type kimiPaths struct {
	// home is what the removal guard resolves symlinks from. It is the Kimi
	// root's parent, not $HOME: removeStale requires the directory it removes
	// from not to escape home (backup.go, internal/removeguard), and
	// $KIMI_CODE_HOME may point outside $HOME entirely, which would leave
	// every removal unguardable. With the default root this is exactly $HOME,
	// as for Claude Code; with a custom root it keeps the Kimi root itself
	// inside the chain the guard checks.
	home     string
	root     string // $KIMI_CODE_HOME when set, else <home>/.kimi-code
	agents   string
	skills   string
	mcp      string
	config   string
	manifest string
	backup   string
}

// resolveKimiHome mirrors Kimi Code's own rule — $KIMI_CODE_HOME when set,
// else ~/.kimi-code — and refuses the values that would aim an install, and
// later a removal, somewhere it must never point. Kimi resolves a relative
// value against whatever directory it happens to run in; devexp refuses one
// outright, for the same reason targetHome refuses a relative HOME (#126).
// Kimi treats an empty value as unset, so devexp does too.
func resolveKimiHome(kimiCodeHome, home string) (string, error) {
	home, err := targetHome(home)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(kimiCodeHome) == "" {
		return filepath.Join(home, ".kimi-code"), nil
	}
	if !filepath.IsAbs(kimiCodeHome) {
		return "", fmt.Errorf("KIMI_CODE_HOME is %q, not an absolute path", kimiCodeHome)
	}
	root := filepath.Clean(kimiCodeHome)
	// The root gets a manifest, a backup directory and, from #113, removals,
	// so it may not be the filesystem root, the home directory, or anything
	// containing the home directory (/Users, $HOME/.., …). An unrelated
	// absolute path such as /opt/kimi is deliberately allowed: pointing
	// $KIMI_CODE_HOME somewhere outside $HOME is the whole reason it exists,
	// and removals stay guarded there because kimiPaths.home is the root's
	// parent, not $HOME.
	//
	// This is the only gate on where a Kimi install may land. For the other
	// two targets the removal guard doubles as an "is it under $HOME?" check;
	// for Kimi it cannot, by design.
	if root == string(filepath.Separator) || root == home || strings.HasPrefix(home, root+string(filepath.Separator)) {
		return "", fmt.Errorf("KIMI_CODE_HOME is %q, which devexp will not install into", root)
	}
	return root, nil
}

// kimiTargetPaths resolves the Kimi Code destinations under kimiCodeHome, or
// under home when it is unset. now is passed in rather than read from the
// clock so the backup directory's name is assertable.
func kimiTargetPaths(kimiCodeHome, home string, now time.Time) (kimiPaths, error) {
	root, err := resolveKimiHome(kimiCodeHome, home)
	if err != nil {
		return kimiPaths{}, err
	}
	return kimiPaths{
		home:     filepath.Dir(root),
		root:     root,
		agents:   filepath.Join(root, "agents"),
		skills:   filepath.Join(root, "skills"),
		mcp:      filepath.Join(root, "mcp.json"),
		config:   filepath.Join(root, "config.toml"),
		manifest: filepath.Join(root, ".devexp-manifest.json"),
		backup:   filepath.Join(root, ".devexp-backup-"+now.Format("20060102T150405")),
	}, nil
}
