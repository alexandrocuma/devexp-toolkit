package cmd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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
// at whatever directory the command happens to run in.

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
// two targets its paths do not hang off $HOME. They are resolved and refused
// here so a misconfigured $KIMI_CODE_HOME is a refusal now rather than a
// surprise removal later.

// kimiPaths holds every destination the Kimi Code CLI install writes to.
type kimiPaths struct {
	// home is what the removal guard resolves symlinks from. It is the Kimi
	// root's parent, not $HOME: removeStale requires the directory it removes
	// from not to escape home (backup.go, internal/removeguard), and
	// $KIMI_CODE_HOME may point outside $HOME entirely, which would leave
	// every removal unguardable. With the default root this is exactly $HOME,
	// as for Claude Code; with a custom root it keeps the Kimi root itself
	// inside the chain the guard checks.
	home string
	root string // $KIMI_CODE_HOME when set, else <home>/.kimi-code
	// agents is where agent files are written; agentsRef is how they are
	// named inside the installed bodies, which is the tilde form when the root
	// is the default one (kimiAgentsRef).
	agents    string
	agentsRef string
	skills    string
	// hooks is the installed hooks root: the adapter and the guard scripts
	// are copied below it, and the command each [[hooks]] entry in
	// config.toml runs names them from here. Kimi runs a hook from whatever
	// directory the session is in, so it must be absolute — which it is,
	// being built from the resolved root.
	hooks    string
	mcp      string
	config   string
	manifest string
	backup   string
}

// kimiUnrenderableRe matches what Kimi substitutes in the text devexp writes
// the Kimi root into. Two different expanders run over installed files, and a
// root is written into both kinds:
//
//   - agent bodies go through renderPrompt, which replaces ${name} for the
//     identifier-shaped names in its variable table;
//   - skill bodies go through expandSkillParameters, which rewrites
//     $ARGUMENTS and $0-$9.
//
// The shipped sources are held to the same rule from the other side, by
// kimiPromptVars in internal/agents and kimiSkillParameterRe in
// internal/skills. This is the same guard for the one piece of that text that
// does not come from the repo.
var kimiUnrenderableRe = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}|\$ARGUMENTS|\$[0-9]`)

// resolveKimiHome mirrors Kimi Code's own rule — $KIMI_CODE_HOME when set,
// else ~/.kimi-code — and refuses the values that would aim an install, and
// later a removal, somewhere it must never point. Kimi resolves a relative
// value against whatever directory it happens to run in; devexp refuses one
// outright, for the same reason targetHome refuses a relative HOME.
// Kimi treats an empty value as unset, so devexp does too.
//
// The two branches converge on one root and one set of content checks before
// a single return. They were two returns, and the content checks sat only on
// the explicit branch: with $KIMI_CODE_HOME unset — the path almost every user
// takes — a $HOME holding a newline installed 34 agents and 8 skills with
// attacker-chosen lines spliced into six of their bodies.
// Anything that rejects a root has to be reachable from both branches, so
// there is only one place to add the next one.
func resolveKimiHome(kimiCodeHome, home string) (string, error) {
	home, err := targetHome(home)
	if err != nil {
		return "", err
	}

	// named is how a refusal refers to the root, because the two branches
	// derive it from different places and telling the user to fix
	// $KIMI_CODE_HOME when they never set it is useless advice.
	root, named := filepath.Join(home, ".kimi-code"), "the Kimi root ~/.kimi-code"
	if strings.TrimSpace(kimiCodeHome) != "" {
		if !filepath.IsAbs(kimiCodeHome) {
			return "", fmt.Errorf("KIMI_CODE_HOME is %q, not an absolute path", kimiCodeHome)
		}
		root, named = filepath.Clean(kimiCodeHome), "KIMI_CODE_HOME"
		// The root gets a manifest, a backup directory and removals, so it
		// may not be the filesystem root, the home directory,
		// or anything containing the home directory (/Users, $HOME/.., …). An
		// unrelated absolute path such as /opt/kimi is deliberately allowed:
		// pointing $KIMI_CODE_HOME somewhere outside $HOME is the whole reason
		// it exists, and removals stay guarded there because kimiPaths.home is
		// the root's parent, not $HOME.
		//
		// This is the only gate on where a Kimi install may land. For the
		// other two targets the removal guard doubles as an "is it under
		// $HOME?" check; for Kimi it cannot, by design. The default root needs
		// no such check: it is always a directory under $HOME.
		if root == string(filepath.Separator) || root == home || strings.HasPrefix(home, root+string(filepath.Separator)) {
			return "", fmt.Errorf("KIMI_CODE_HOME is %q, which devexp will not install into", root)
		}
	}

	// This path is written into the installed agent and skill
	// bodies, which are prompts a model reads, so what it may contain is no
	// longer only devexp's problem. It reaches here from $KIMI_CODE_HOME or
	// from $HOME, and both are attacker-controlled in the cases that matter:
	//
	//   - A control character — a newline above all — puts attacker-chosen
	//     lines inside every installed agent's instructions, at column 1.
	//     Nothing downstream can tell those from the agent's own text.
	//   - A ${…}, $ARGUMENTS or $<n> form is rewritten by Kimi itself when it
	//     renders the body, so the repointed "read <path>/x.md" line becomes
	//     something else at the moment it is used.
	//   - Bytes that are not valid UTF-8 cannot survive the round trip. Kimi
	//     reads these files as UTF-8 and would substitute replacement
	//     characters, so the rewritten reference would point at a path that
	//     does not exist, silently. settings.json is exposed to the same
	//     class of failure.
	//     macOS refuses such a directory name outright; Linux does not.
	//
	// All three are refused here rather than escaped at each use, because this
	// is the one gate every Kimi destination is built from.
	if strings.ContainsFunc(root, unicode.IsControl) {
		return "", fmt.Errorf("%s is %q, which contains a control character devexp will not write into an agent file", named, root)
	}
	if m := kimiUnrenderableRe.FindString(root); m != "" {
		return "", fmt.Errorf("%s is %q, which contains %q — Kimi substitutes that when it renders an agent or skill body, so devexp will not write it into one", named, root, m)
	}
	if !utf8.ValidString(root) {
		return "", fmt.Errorf("%s is %q, which is not valid UTF-8 and cannot be written into an agent file unchanged", named, root)
	}
	return root, nil
}

// kimiAgentsRef is how an installed agent file is *named inside a prompt*, as
// opposed to where it is written. Agent and skill bodies say "read
// ~/.claude/agents/<name>.md and follow it", and the Kimi install repoints
// those at this directory. The destination on disk is always kimiPaths.agents,
// an absolute path; only the text written into the installed bodies differs.
//
// With the default root it is the tilde form, `~/.kimi-code/agents`. With a
// custom $KIMI_CODE_HOME it is the absolute path, because there is no tilde
// form of a root that does not sit at ~/.kimi-code.
//
// The tilde form is preferred where it exists because it puts *no
// environment-derived string into a prompt at all*. That is the surface
// removed, not merely one way of exploiting it: resolveKimiHome still refuses
// a root that could not be written into a prompt, and still has to, because
// the custom-root branch below writes one. The two are defence in depth.
//
// It is safe because `~` genuinely expands. Measured against 2.0.1 by driving
// real tool calls: Read, Write, Edit, Glob and Grep all expand `~/` against
// the process's HOME before resolving the path. Two further details from that
// measurement matter here:
//
//   - Expansion happens *before* the absolute-path test, so an expanded `~/`
//     also clears the guard that refuses a relative path to a file outside the
//     workspace. `~/.kimi-code/agents/x.md` is readable from a project
//     directory where `../.kimi-code/agents/x.md` is refused — which is
//     exactly devexp's case, since an agent file is always outside the
//     workspace.
//   - It expands against HOME as Kimi sees it at run time, not as devexp saw
//     it at install time. If those differ, so does Kimi's own default root, so
//     the tilde form follows the root it is actually naming.
//
// (One measured version of a closed-source bundle, not a documented contract.
// Nothing depends on it holding: a wrong path makes an agent reference fail
// loudly when followed, and the gate covers the injection concern separately.)
func kimiAgentsRef(root, home string) string {
	if root == filepath.Join(home, ".kimi-code") {
		return "~/.kimi-code/agents"
	}
	return filepath.Join(root, "agents")
}

// kimiTargetPaths resolves the Kimi Code destinations under kimiCodeHome, or
// under home when it is unset. now is passed in rather than read from the
// clock so the backup directory's name is assertable.
func kimiTargetPaths(kimiCodeHome, home string, now time.Time) (kimiPaths, error) {
	root, err := resolveKimiHome(kimiCodeHome, home)
	if err != nil {
		return kimiPaths{}, err
	}
	// home is cleaned by targetHome inside resolveKimiHome; kimiAgentsRef
	// compares against it, so it must be cleaned here the same way.
	cleanHome, err := targetHome(home)
	if err != nil {
		return kimiPaths{}, err
	}
	return kimiPaths{
		home:      filepath.Dir(root),
		root:      root,
		agents:    filepath.Join(root, "agents"),
		agentsRef: kimiAgentsRef(root, cleanHome),
		skills:    filepath.Join(root, "skills"),
		hooks:     filepath.Join(root, "hooks"),
		mcp:       filepath.Join(root, "mcp.json"),
		config:    filepath.Join(root, "config.toml"),
		manifest:  filepath.Join(root, ".devexp-manifest.json"),
		backup:    filepath.Join(root, ".devexp-backup-"+now.Format("20060102T150405")),
	}, nil
}
