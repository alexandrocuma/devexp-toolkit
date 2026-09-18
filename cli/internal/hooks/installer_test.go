package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type hooksMapT = map[string][]hookEntry

func createScript(t *testing.T, repoDir, relPath string) string {
	t.Helper()
	abs := filepath.Join(repoDir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(abs, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	return abs
}

func writeSettingsHooks(t *testing.T, settingsPath string, hooks hooksMapT) {
	t.Helper()
	hooksBytes, err := json.Marshal(hooks)
	if err != nil {
		t.Fatalf("Marshal hooks error = %v", err)
	}
	raw := map[string]json.RawMessage{"hooks": hooksBytes}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}

// ccHook builds a registry hook that has only a claude_code target block.
func ccHook(name string, enabled bool, event, matcher, script string) Hook {
	return Hook{
		Name: name, Enabled: enabled,
		Targets: map[string]TargetSpec{
			TargetClaudeCode: {Event: event, Matcher: matcher, Script: script},
		},
	}
}

func readHooks(t *testing.T, settingsPath string) hooksMapT {
	t.Helper()
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", settingsPath, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal settings error = %v", err)
	}
	var hooks hooksMapT
	if hooksRaw, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
			t.Fatalf("Unmarshal hooks error = %v", err)
		}
	}
	return withoutFields(hooks)
}

// withoutFields drops the raw members hooks were read with, so they compare
// equal to entries built in a test. Tests of the members themselves read the
// file's bytes instead.
func withoutFields(hooks hooksMapT) hooksMapT {
	for _, entries := range hooks {
		for i := range entries {
			entries[i].fields = nil
			for j := range entries[i].Hooks {
				entries[i].Hooks[j].fields = nil
			}
		}
	}
	return hooks
}

func TestIsStaleDevexpHook(t *testing.T) {
	repoDir := t.TempDir()
	existingScript := createScript(t, repoDir, "hooks/claude-code/exists.sh")
	missingScript := filepath.Join(repoDir, "hooks", "claude-code", "missing.sh")

	outsideDir := t.TempDir()
	outsideExisting := filepath.Join(outsideDir, "user-hook.sh")
	if err := os.WriteFile(outsideExisting, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	outsideMissing := filepath.Join(outsideDir, "missing-user-hook.sh")

	tests := map[string]struct {
		cmd  string
		want bool
	}{
		"existing script under repoDir is not stale": {
			cmd:  existingScript,
			want: false,
		},
		"missing script under repoDir is stale": {
			cmd:  missingScript,
			want: true,
		},
		"existing script outside repoDir is not stale": {
			cmd:  outsideExisting,
			want: false,
		},
		"missing script outside repoDir is not stale (user hook)": {
			cmd:  outsideMissing,
			want: false,
		},
		"cmd equal to repoDir itself is not stale": {
			cmd:  repoDir,
			want: false,
		},
		"missing script under repoDir, single-quoted, is stale": {
			cmd:  shellQuote(missingScript),
			want: true,
		},
		// #138: only devexp's own form under repoDir is judged. Everything
		// below names nothing on disk, and is the user's.
		"user command under repoDir with an argument": {
			cmd: filepath.Join(repoDir, "hooks", "x.sh") + " --flag",
		},
		"missing script under hooks/claude-code with an argument": {
			cmd: missingScript + " --flag",
		},
		"quoted missing script with an argument": {
			cmd: shellQuote(missingScript) + " --flag",
		},
		"missing script chained with another command": {
			cmd: missingScript + ";true",
		},
		"missing script run through a wrapper": {
			cmd: "bash " + missingScript,
		},
		"double-quoted missing script": {
			cmd: `"` + missingScript + `"`,
		},
		"missing file in another repoDir directory": {
			cmd: filepath.Join(repoDir, "bin", "tool"),
		},
		"missing file directly under hooks/": {
			cmd: filepath.Join(repoDir, "hooks", "x.sh"),
		},
		"missing file nested below hooks/claude-code/": {
			cmd: filepath.Join(repoDir, "hooks", "claude-code", "sub", "x.sh"),
		},
		"missing file under my-hooks/claude-code/": {
			cmd: filepath.Join(repoDir, "my-hooks", "claude-code", "missing.sh"),
		},
		"missing script by an unclean path": {
			cmd: repoDir + "/hooks/../hooks/claude-code/missing.sh",
		},
		"missing script through a variable": {
			cmd: "$DEVEXP_DIR/hooks/claude-code/missing.sh",
		},
		"missing script by a relative path": {
			cmd: "hooks/claude-code/missing.sh",
		},
		"missing script named by a glob": {
			cmd: filepath.Join(repoDir, "hooks", "claude-code", "*.sh"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := isStaleDevexpHook(tt.cmd, repoDir)
			if got != tt.want {
				t.Errorf("isStaleDevexpHook(%q, %q) = %v, want %v", tt.cmd, repoDir, got, tt.want)
			}
		})
	}
}

// TestInstallClaude_PrunesOnlyDevexpStaleHooks (#138): a removed script's
// registration is pruned in every form devexp wrote it — plain, single-quoted,
// or the bare path an install from a repo dir needing quotes wrote before
// #135. A user's command under the repo dir that names nothing on disk (with
// arguments, in another directory, with shell syntax) stays.
func TestInstallClaude_PrunesOnlyDevexpStaleHooks(t *testing.T) {
	for name, dir := range map[string]string{
		"plain repo dir":          "repo",
		"repo dir needing quotes": "My Proj/it's $x",
	} {
		t.Run(name, func(t *testing.T) {
			repoDir := filepath.Join(t.TempDir(), dir)
			registry := Registry{ccHook("secret-guard", true, "PreToolUse", "Read", "hooks/claude-code/secret-guard.sh")}
			createScript(t, repoDir, "hooks/claude-code/secret-guard.sh")
			removed := filepath.Join(repoDir, "hooks", "claude-code", "removed.sh")
			stale := []string{hookCommand(removed), shellQuote(removed), removed}
			user := []string{
				filepath.Join(repoDir, "hooks", "x.sh") + " --flag",
				shellQuote(removed) + " --flag",
				shellQuote(removed) + "; true",
				`"` + removed + `"`,
				shellQuote(filepath.Join(repoDir, "bin", "tool")),
				shellQuote(filepath.Join(repoDir, "hooks", "claude-code", "sub", "x.sh")),
				"$DEVEXP_DIR/hooks/claude-code/removed.sh",
			}
			var cmds []hookCmd
			for _, c := range append(append([]string(nil), user...), stale...) {
				cmds = append(cmds, hookCmd{Type: "command", Command: c})
			}
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			writeSettingsHooks(t, settingsPath, hooksMapT{"Stop": {{Hooks: cmds}}})

			var err error
			out := captureOutput(t, func() { err = InstallClaude(registry, repoDir, settingsPath, nil, false) })
			if err != nil {
				t.Fatalf("InstallClaude() error = %v\n%s", err, out)
			}
			var got []string
			for _, h := range readHooks(t, settingsPath)["Stop"][0].Hooks {
				got = append(got, h.Command)
			}
			if !reflect.DeepEqual(got, user) {
				t.Errorf("Stop commands =\n%q\nwant the user's only\n%q\noutput:\n%s", got, user, out)
			}
		})
	}
}

func TestPruneStaleHooks(t *testing.T) {
	repoDir := t.TempDir()
	existingScript := createScript(t, repoDir, "hooks/claude-code/exists.sh")
	missingScript := filepath.Join(repoDir, "hooks", "claude-code", "missing.sh")
	userScript := filepath.Join(t.TempDir(), "user-hook.sh")

	tests := map[string]struct {
		hooksMap   hooksMapT
		wantHooks  hooksMapT
		wantPruned bool
	}{
		"prunes individual stale hook but keeps others in the same entry": {
			hooksMap: hooksMapT{
				"PreToolUse": {
					{Matcher: "Bash", Hooks: []hookCmd{
						{Type: "command", Command: existingScript},
						{Type: "command", Command: missingScript},
					}},
				},
			},
			wantHooks: hooksMapT{
				"PreToolUse": {
					{Matcher: "Bash", Hooks: []hookCmd{
						{Type: "command", Command: existingScript},
					}},
				},
			},
			wantPruned: true,
		},
		"removes the event key entirely when all its hooks are stale": {
			hooksMap: hooksMapT{
				"PreToolUse": {
					{Matcher: "Bash", Hooks: []hookCmd{
						{Type: "command", Command: missingScript},
					}},
				},
			},
			wantHooks:  hooksMapT{},
			wantPruned: true,
		},
		"leaves user hooks and valid devexp hooks untouched": {
			hooksMap: hooksMapT{
				"PreToolUse": {
					{Matcher: "Bash", Hooks: []hookCmd{
						{Type: "command", Command: existingScript},
						{Type: "command", Command: userScript},
					}},
				},
			},
			wantHooks: hooksMapT{
				"PreToolUse": {
					{Matcher: "Bash", Hooks: []hookCmd{
						{Type: "command", Command: existingScript},
						{Type: "command", Command: userScript},
					}},
				},
			},
			wantPruned: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := pruneStaleHooks(tt.hooksMap, repoDir, false)
			if got != tt.wantPruned {
				t.Errorf("pruneStaleHooks() pruned = %v, want %v", got, tt.wantPruned)
			}
			if !reflect.DeepEqual(tt.hooksMap, tt.wantHooks) {
				t.Errorf("pruneStaleHooks() hooksMap = %+v, want %+v", tt.hooksMap, tt.wantHooks)
			}
		})
	}
}

func TestInstallClaude(t *testing.T) {
	tests := map[string]struct {
		setup  func(t *testing.T, repoDir, settingsPath string) (Registry, []string)
		dryRun bool
		check  func(t *testing.T, repoDir, settingsPath string)
	}{
		"adds a new hook not yet registered": {
			setup: func(t *testing.T, repoDir, settingsPath string) (Registry, []string) {
				createScript(t, repoDir, "hooks/claude-code/foo.sh")
				return Registry{ccHook("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh")}, nil
			},
			check: func(t *testing.T, repoDir, settingsPath string) {
				got := readHooks(t, settingsPath)
				want := hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{
							{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/foo.sh")},
						}},
					},
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("hooks = %+v, want %+v", got, want)
				}
			},
		},
		"skips a hook already registered": {
			setup: func(t *testing.T, repoDir, settingsPath string) (Registry, []string) {
				scriptAbs := createScript(t, repoDir, "hooks/claude-code/foo.sh")
				writeSettingsHooks(t, settingsPath, hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{
							{Type: "command", Command: scriptAbs},
						}},
					},
				})
				return Registry{ccHook("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh")}, nil
			},
			check: func(t *testing.T, repoDir, settingsPath string) {
				got := readHooks(t, settingsPath)
				want := hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{
							{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/foo.sh")},
						}},
					},
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("hooks = %+v, want %+v", got, want)
				}
			},
		},
		"skips a disabled hook, leaving settings.json unwritten": {
			setup: func(t *testing.T, repoDir, settingsPath string) (Registry, []string) {
				createScript(t, repoDir, "hooks/claude-code/foo.sh")
				return Registry{ccHook("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh")}, []string{"foo"}
			},
			check: func(t *testing.T, repoDir, settingsPath string) {
				if got := readHooks(t, settingsPath); got != nil {
					t.Errorf("hooks = %+v, want settings.json to not exist", got)
				}
			},
		},
		// Wiring: the cases above exercise pruneForeignDevexpHooks directly.
		// These go through InstallClaude, so dropping or misordering the call
		// inside it fails here even though the unit tests still pass.
		"removes a registration from another install root, keeping this one": {
			setup: func(t *testing.T, repoDir, settingsPath string) (Registry, []string) {
				scriptAbs := createScript(t, repoDir, "hooks/claude-code/foo.sh")
				foreign := filepath.Join(t.TempDir(), "hooks/claude-code/foo.sh")
				writeSettingsHooks(t, settingsPath, hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: foreign}}},
						{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: scriptAbs}}},
					},
				})
				return Registry{ccHook("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh")}, nil
			},
			check: func(t *testing.T, repoDir, settingsPath string) {
				got := readHooks(t, settingsPath)
				want := hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{
							{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/foo.sh")},
						}},
					},
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("hooks = %+v, want %+v", got, want)
				}
			},
		},
		// The foreign entry is the *only* one, so the hook is also added. If the
		// prune call were removed, `changed` would still be true and the file
		// would still be written -- only this assertion catches the leftover.
		"replaces a foreign-root registration with this root's": {
			setup: func(t *testing.T, repoDir, settingsPath string) (Registry, []string) {
				createScript(t, repoDir, "hooks/claude-code/foo.sh")
				foreign := filepath.Join(t.TempDir(), "hooks/claude-code/foo.sh")
				writeSettingsHooks(t, settingsPath, hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: foreign}}},
					},
				})
				return Registry{ccHook("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh")}, nil
			},
			check: func(t *testing.T, repoDir, settingsPath string) {
				got := readHooks(t, settingsPath)
				want := hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{
							{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/foo.sh")},
						}},
					},
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("hooks = %+v, want %+v", got, want)
				}
			},
		},
		"prune-only run rewrites settings.json to drop a hook whose script no longer exists": {
			setup: func(t *testing.T, repoDir, settingsPath string) (Registry, []string) {
				writeSettingsHooks(t, settingsPath, hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{
							{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/removed.sh")},
						}},
					},
				})
				return Registry{}, nil
			},
			check: func(t *testing.T, repoDir, settingsPath string) {
				got := readHooks(t, settingsPath)
				want := hooksMapT{}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("hooks = %+v, want %+v", got, want)
				}
			},
		},
		"dry run reports additions and prunes without writing settings.json": {
			setup: func(t *testing.T, repoDir, settingsPath string) (Registry, []string) {
				createScript(t, repoDir, "hooks/claude-code/foo.sh")
				writeSettingsHooks(t, settingsPath, hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{
							{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/removed.sh")},
						}},
					},
				})
				return Registry{ccHook("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh")}, nil
			},
			dryRun: true,
			check: func(t *testing.T, repoDir, settingsPath string) {
				got := readHooks(t, settingsPath)
				want := hooksMapT{
					"PreToolUse": {
						{Matcher: "Bash", Hooks: []hookCmd{
							{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/removed.sh")},
						}},
					},
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("hooks = %+v, want %+v (dry run must not write)", got, want)
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			repoDir := t.TempDir()
			settingsPath := filepath.Join(t.TempDir(), "settings.json")

			registry, disabled := tt.setup(t, repoDir, settingsPath)

			if err := InstallClaude(registry, repoDir, settingsPath, disabled, tt.dryRun); err != nil {
				t.Fatalf("InstallClaude() error = %v", err)
			}

			tt.check(t, repoDir, settingsPath)
		})
	}
}

// TestInstallClaude_RelativeHookCommands: every registered command is absolute.
// An earlier install from a relative repo dir left relative devexp commands;
// they are replaced by the absolute registration, and a relative command that
// isn't devexp's is left alone (#126).
func TestInstallClaude_RelativeHookCommands(t *testing.T) {
	repoDir := t.TempDir()
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	scriptAbs := createScript(t, repoDir, "hooks/claude-code/secret-guard.sh")
	const userRel = "my-hooks/format-check.sh"
	writeSettingsHooks(t, settingsPath, hooksMapT{
		"PreToolUse": {
			{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: "hooks/claude-code/secret-guard.sh"}}},
			{Matcher: "Read|Glob", Hooks: []hookCmd{{Type: "command", Command: "./hooks/claude-code/graphify-read-guard.sh"}}},
			{Matcher: "Write", Hooks: []hookCmd{{Type: "command", Command: userRel}}},
		},
	})

	var err error
	out := captureOutput(t, func() { err = InstallClaude(testRegistry(), repoDir, settingsPath, nil, false) })
	if err != nil {
		t.Fatalf("InstallClaude() error = %v\n%s", err, out)
	}
	want := hooksMapT{"PreToolUse": {
		{Matcher: "Write", Hooks: []hookCmd{{Type: "command", Command: userRel}}},
		{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: scriptAbs}}},
		{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/dangerous-cmd-guard.sh")}}},
	}}
	if got := readHooks(t, settingsPath); !reflect.DeepEqual(got, want) {
		t.Errorf("hooks = %+v, want %+v", got, want)
	}
	for _, entries := range readHooks(t, settingsPath) {
		for _, e := range entries {
			for _, h := range e.Hooks {
				if h.Command != userRel && !filepath.IsAbs(h.Command) {
					t.Errorf("registered a relative devexp command %q", h.Command)
				}
			}
		}
	}
}

// TestInstallClaude_RelativeRepoDir: a relative repo dir would register
// relative commands, so InstallClaude refuses before touching settings.json.
func TestInstallClaude_RelativeRepoDir(t *testing.T) {
	for _, repoDir := range []string{"", ".", "repo", "sub/../repo"} {
		t.Run(repoDir, func(t *testing.T) {
			cwd := t.TempDir()
			t.Chdir(cwd)
			createScript(t, cwd, "repo/hooks/claude-code/secret-guard.sh")
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			writeSettingsHooks(t, settingsPath, hooksMapT{})
			before, _ := os.ReadFile(settingsPath)

			err := InstallClaude(testRegistry(), repoDir, settingsPath, nil, false)
			if err == nil || !strings.Contains(err.Error(), "not an absolute path") {
				t.Errorf("InstallClaude(repoDir=%q) error = %v, want a refusal", repoDir, err)
			}
			if after, _ := os.ReadFile(settingsPath); string(after) != string(before) {
				t.Errorf("settings.json changed:\n%s", after)
			}
		})
	}
}

func testRegistry() Registry {
	return Registry{
		ccHook("secret-guard", true, "PreToolUse", "Read|Bash", "hooks/claude-code/secret-guard.sh"),
		ccHook("dangerous-cmd-guard", true, "PreToolUse", "Bash", "hooks/claude-code/dangerous-cmd-guard.sh"),
		// Disabled on purpose: a foreign copy of a disabled hook still runs,
		// so it must still be pruned.
		ccHook("graphify-read-guard", false, "PreToolUse", "Read|Glob", "hooks/claude-code/graphify-read-guard.sh"),
	}
}

func TestIsForeignDevexpHook(t *testing.T) {
	repoDir := t.TempDir()
	otherRoot := t.TempDir()
	managed := managedScriptNames(testRegistry())

	tests := map[string]struct {
		cmd  string
		want bool
	}{
		"devexp hook registered from another install root is foreign": {
			cmd:  filepath.Join(otherRoot, "hooks", "claude-code", "secret-guard.sh"),
			want: true,
		},
		"the same hook under repoDir is not foreign": {
			cmd:  filepath.Join(repoDir, "hooks", "claude-code", "secret-guard.sh"),
			want: false,
		},
		"a disabled hook from another root is still foreign": {
			cmd:  filepath.Join(otherRoot, "hooks", "claude-code", "graphify-read-guard.sh"),
			want: true,
		},
		"user hook sharing a basename but outside the script dir is untouched": {
			cmd:  filepath.Join(otherRoot, "my-hooks", "secret-guard.sh"),
			want: false,
		},
		"unknown script inside the script dir is untouched": {
			cmd:  filepath.Join(otherRoot, "hooks", "claude-code", "my-own-hook.sh"),
			want: false,
		},
		// A sibling directory shares a string prefix with repoDir without being
		// nested in it — a naive strings.HasPrefix would wrongly call this ours.
		"sibling root sharing a path prefix is foreign": {
			cmd:  repoDir + "-old/hooks/claude-code/secret-guard.sh",
			want: true,
		},
		"empty command is not foreign": {
			cmd:  "",
			want: false,
		},
		"a relative command is not foreign (isRelativeDevexpHook handles it)": {
			cmd:  "hooks/claude-code/secret-guard.sh",
			want: false,
		},
		"script dir inside another dir name is untouched": {
			cmd:  filepath.Join(otherRoot, "my-hooks", "claude-code", "secret-guard.sh"),
			want: false,
		},
		"script dir not directly above the script is untouched": {
			cmd:  filepath.Join(otherRoot, "hooks", "claude-code", "sub", "secret-guard.sh"),
			want: false,
		},
		"env assignment before an absolute path is untouched": {
			cmd:  "FOO=1 " + filepath.Join(otherRoot, "hooks", "claude-code", "secret-guard.sh"),
			want: false,
		},
		"interpreter wrapper around an absolute path is untouched": {
			cmd:  "bash " + filepath.Join(otherRoot, "hooks", "claude-code", "secret-guard.sh"),
			want: false,
		},
		"quoted absolute path is untouched": {
			cmd:  `"` + filepath.Join(otherRoot, "hooks", "claude-code", "secret-guard.sh") + `"`,
			want: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isForeignDevexpHook(tt.cmd, managed, repoDir); got != tt.want {
				t.Errorf("isForeignDevexpHook(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestIsRelativeDevexpHook(t *testing.T) {
	managed := managedScriptNames(testRegistry())
	tests := map[string]struct {
		cmd  string
		want bool
	}{
		"relative devexp hook":                      {cmd: "hooks/claude-code/secret-guard.sh", want: true},
		"relative devexp hook with ./":              {cmd: "./hooks/claude-code/secret-guard.sh", want: true},
		"relative devexp hook under a subdir":       {cmd: "repo/hooks/claude-code/secret-guard.sh", want: true},
		"relative copy of a disabled hook":          {cmd: "hooks/claude-code/graphify-read-guard.sh", want: true},
		"absolute devexp hook":                      {cmd: "/opt/devexp/hooks/claude-code/secret-guard.sh", want: false},
		"relative user hook sharing a basename":     {cmd: "my-hooks/secret-guard.sh", want: false},
		"relative unknown script in the script dir": {cmd: "hooks/claude-code/my-own-hook.sh", want: false},
		"bare basename":                             {cmd: "secret-guard.sh", want: false},
		"empty":                                     {cmd: "", want: false},
		// Not plain paths: devexp never registered these, so they are the user's.
		"project-dir variable":        {cmd: "$CLAUDE_PROJECT_DIR/hooks/claude-code/secret-guard.sh", want: false},
		"quoted project-dir variable": {cmd: `"$CLAUDE_PROJECT_DIR"/hooks/claude-code/secret-guard.sh`, want: false},
		"braced project-dir variable": {cmd: "${CLAUDE_PROJECT_DIR}/hooks/claude-code/secret-guard.sh", want: false},
		"tilde":                       {cmd: "~/vendor/hooks/claude-code/secret-guard.sh", want: false},
		"HOME variable":               {cmd: "$HOME/vendor/hooks/claude-code/secret-guard.sh", want: false},
		"env assignment prefix":       {cmd: "FOO=1 hooks/claude-code/secret-guard.sh", want: false},
		"interpreter wrapper":         {cmd: "bash hooks/claude-code/secret-guard.sh", want: false},
		"single-quoted":               {cmd: "'hooks/claude-code/secret-guard.sh'", want: false},
		"backticks":                   {cmd: "`pwd`/hooks/claude-code/secret-guard.sh", want: false},
		"trailing argument":           {cmd: "hooks/claude-code/secret-guard.sh --strict", want: false},
		"chained command":             {cmd: "hooks/claude-code/secret-guard.sh;true", want: false},
		// hooks/claude-code/ must start at a path-segment boundary, right above the script.
		"script dir inside another dir name": {cmd: "my-hooks/claude-code/secret-guard.sh", want: false},
		"script dir not directly above":      {cmd: "hooks/claude-code/sub/secret-guard.sh", want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isRelativeDevexpHook(tt.cmd, managed); got != tt.want {
				t.Errorf("isRelativeDevexpHook(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestPruneForeignDevexpHooks(t *testing.T) {
	repoDir := t.TempDir()
	otherRoot := t.TempDir()
	registry := testRegistry()

	mine := filepath.Join(repoDir, "hooks", "claude-code", "secret-guard.sh")
	foreign := filepath.Join(otherRoot, "hooks", "claude-code", "secret-guard.sh")
	foreignDisabled := filepath.Join(otherRoot, "hooks", "claude-code", "graphify-read-guard.sh")
	userHook := filepath.Join(otherRoot, "my-hooks", "secret-guard.sh")

	tests := map[string]struct {
		hooksMap   hooksMapT
		wantHooks  hooksMapT
		wantPruned bool
	}{
		"removes the foreign duplicate and keeps ours": {
			hooksMap: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: foreign}}},
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: mine}}},
			}},
			wantHooks: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: mine}}},
			}},
			wantPruned: true,
		},
		"prunes a foreign copy of a disabled hook": {
			hooksMap: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Glob", Hooks: []hookCmd{{Type: "command", Command: foreignDisabled}}},
			}},
			wantHooks:  hooksMapT{},
			wantPruned: true,
		},
		"keeps a user hook that merely shares a basename": {
			hooksMap: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: userHook}}},
			}},
			wantHooks: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: userHook}}},
			}},
			wantPruned: false,
		},
		"an entry holding both keeps only the user command": {
			hooksMap: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{
					{Type: "command", Command: foreign},
					{Type: "command", Command: userHook},
				}},
			}},
			wantHooks: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: userHook}}},
			}},
			wantPruned: true,
		},
		"prunes a relative devexp hook": {
			hooksMap: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: "hooks/claude-code/secret-guard.sh"}}},
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: mine}}},
			}},
			wantHooks: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: mine}}},
			}},
			wantPruned: true,
		},
		"keeps a relative user hook": {
			hooksMap: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: "my-hooks/secret-guard.sh"}}},
			}},
			wantHooks: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: "my-hooks/secret-guard.sh"}}},
			}},
			wantPruned: false,
		},
		"keeps every hook that isn't a plain devexp script path": {
			hooksMap: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{
					{Type: "command", Command: "$CLAUDE_PROJECT_DIR/hooks/claude-code/secret-guard.sh"},
					{Type: "command", Command: `"$CLAUDE_PROJECT_DIR"/hooks/claude-code/secret-guard.sh`},
					{Type: "command", Command: "~/vendor/hooks/claude-code/secret-guard.sh"},
					{Type: "command", Command: "$HOME/vendor/hooks/claude-code/secret-guard.sh"},
					{Type: "command", Command: "FOO=1 " + foreign},
					{Type: "command", Command: "bash " + foreign},
					{Type: "command", Command: "my-hooks/claude-code/secret-guard.sh"},
					{Type: "command", Command: filepath.Join(otherRoot, "my-hooks", "claude-code", "secret-guard.sh")},
				}},
			}},
			wantHooks: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{
					{Type: "command", Command: "$CLAUDE_PROJECT_DIR/hooks/claude-code/secret-guard.sh"},
					{Type: "command", Command: `"$CLAUDE_PROJECT_DIR"/hooks/claude-code/secret-guard.sh`},
					{Type: "command", Command: "~/vendor/hooks/claude-code/secret-guard.sh"},
					{Type: "command", Command: "$HOME/vendor/hooks/claude-code/secret-guard.sh"},
					{Type: "command", Command: "FOO=1 " + foreign},
					{Type: "command", Command: "bash " + foreign},
					{Type: "command", Command: "my-hooks/claude-code/secret-guard.sh"},
					{Type: "command", Command: filepath.Join(otherRoot, "my-hooks", "claude-code", "secret-guard.sh")},
				}},
			}},
			wantPruned: false,
		},
		"nothing foreign means nothing pruned": {
			hooksMap: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: mine}}},
			}},
			wantHooks: hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: mine}}},
			}},
			wantPruned: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := pruneForeignDevexpHooks(tt.hooksMap, registry, repoDir, false)
			if got != tt.wantPruned {
				t.Errorf("pruneForeignDevexpHooks() pruned = %v, want %v", got, tt.wantPruned)
			}
			if !reflect.DeepEqual(tt.hooksMap, tt.wantHooks) {
				t.Errorf("hooksMap = %+v, want %+v", tt.hooksMap, tt.wantHooks)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

func TestHookUnmarshalJSON(t *testing.T) {
	tests := map[string]struct {
		json        string
		wantHook    Hook
		wantMissing []string
		wantErr     string
	}{
		"claude_code block lands in Targets": {
			json: `{
				"name": "secret-guard",
				"description": "d",
				"claude_code": {"event": "PreToolUse", "matcher": "Read|Bash", "script": "hooks/claude-code/secret-guard.sh"},
				"enabled": true
			}`,
			wantHook: Hook{
				Name: "secret-guard", Description: "d", Enabled: true,
				Targets: map[string]TargetSpec{
					TargetClaudeCode: {Event: "PreToolUse", Matcher: "Read|Bash", Script: "hooks/claude-code/secret-guard.sh"},
				},
			},
			wantMissing: []string{TargetOpencode},
		},
		"opencode block parses module, export, fail_closed and enabled": {
			json: `{
				"name": "graphify-read-guard",
				"claude_code": {"event": "PreToolUse", "matcher": "Read|Glob", "script": "hooks/claude-code/graphify-read-guard.sh"},
				"opencode": {"event": "tool.execute.before", "module": "hooks/opencode/graphify-read-guard.js", "export": "graphifyReadGuard", "fail_closed": true, "enabled": true},
				"enabled": false
			}`,
			wantHook: Hook{
				Name: "graphify-read-guard", Enabled: false,
				Targets: map[string]TargetSpec{
					TargetClaudeCode: {Event: "PreToolUse", Matcher: "Read|Glob", Script: "hooks/claude-code/graphify-read-guard.sh"},
					TargetOpencode: {
						Event: "tool.execute.before", Module: "hooks/opencode/graphify-read-guard.js",
						Export: "graphifyReadGuard", FailClosed: true, Enabled: boolPtr(true),
					},
				},
			},
		},
		// The target-generic criterion: a block under a key the code has never
		// heard of parses with no change to the type or the decoder.
		"an arbitrary new sibling block parses without code changes": {
			json: `{
				"name": "foo",
				"claude_code": {"event": "PreToolUse", "script": "hooks/claude-code/foo.sh"},
				"example_target": {"event": "X", "script": "s.sh"},
				"enabled": true
			}`,
			wantHook: Hook{
				Name: "foo", Enabled: true,
				Targets: map[string]TargetSpec{
					TargetClaudeCode: {Event: "PreToolUse", Script: "hooks/claude-code/foo.sh"},
					"example_target": {Event: "X", Script: "s.sh"},
				},
			},
		},
		"non-object extras are not targets": {
			json: `{"name": "foo", "tags": ["a"], "note": "x", "claude_code": {"script": "s.sh"}, "enabled": true}`,
			wantHook: Hook{
				Name: "foo", Enabled: true,
				Targets: map[string]TargetSpec{TargetClaudeCode: {Script: "s.sh"}},
			},
		},
		"no target blocks means no targets": {
			json:        `{"name": "foo", "enabled": true}`,
			wantHook:    Hook{Name: "foo", Enabled: true},
			wantMissing: []string{TargetClaudeCode, TargetOpencode},
		},
		"a malformed target block is an error naming the hook and target": {
			json:    `{"name": "broken-hook", "opencode": {"fail_closed": "yes"}, "enabled": true}`,
			wantErr: `hook "broken-hook": target "opencode"`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got Hook
			err := json.Unmarshal([]byte(tt.json), &got)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Unmarshal() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.wantHook) {
				t.Errorf("Unmarshal() = %+v, want %+v", got, tt.wantHook)
			}
			for id, want := range tt.wantHook.Targets {
				if spec, ok := got.Target(id); !ok || !reflect.DeepEqual(spec, want) {
					t.Errorf("Target(%q) = %+v, %v, want %+v, true", id, spec, ok, want)
				}
			}
			for _, id := range tt.wantMissing {
				if _, ok := got.Target(id); ok {
					t.Errorf("Target(%q) ok = true, want false", id)
				}
			}
		})
	}
}

func TestHookEnabledFor(t *testing.T) {
	tests := map[string]struct {
		hook Hook
		want bool
	}{
		"missing block is not enabled": {
			hook: Hook{Enabled: true, Targets: map[string]TargetSpec{TargetClaudeCode: {}}},
			want: false,
		},
		"nil override follows top-level true": {
			hook: Hook{Enabled: true, Targets: map[string]TargetSpec{TargetOpencode: {}}},
			want: true,
		},
		"nil override follows top-level false": {
			hook: Hook{Enabled: false, Targets: map[string]TargetSpec{TargetOpencode: {}}},
			want: false,
		},
		// The graphify case: off for Claude Code, on for opencode.
		"explicit true overrides top-level false": {
			hook: Hook{Enabled: false, Targets: map[string]TargetSpec{TargetOpencode: {Enabled: boolPtr(true)}}},
			want: true,
		},
		"explicit false overrides top-level true": {
			hook: Hook{Enabled: true, Targets: map[string]TargetSpec{TargetOpencode: {Enabled: boolPtr(false)}}},
			want: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tt.hook.EnabledFor(TargetOpencode); got != tt.want {
				t.Errorf("EnabledFor(%q) = %v, want %v", TargetOpencode, got, tt.want)
			}
		})
	}
}

// TestLoadRegistry_RepoRegistry checks the real hooks/registry.json: every
// hook maps to both targets, and graphify stays on for opencode only.
func TestLoadRegistry_RepoRegistry(t *testing.T) {
	registry, err := LoadRegistry(filepath.Join("..", "..", "..", "hooks", "registry.json"))
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	if len(registry) != 10 {
		t.Fatalf("LoadRegistry() = %d hooks, want 10", len(registry))
	}

	graphify := map[string]bool{
		"graphify-read-guard":       true,
		"graphify-session-sentinel": true,
		"graphify-grep-nudge":       true,
	}
	for _, h := range registry {
		cc, ok := h.Target(TargetClaudeCode)
		if !ok || cc.Script == "" {
			t.Errorf("%s: claude_code.script missing (block present = %v)", h.Name, ok)
		}
		oc, ok := h.Target(TargetOpencode)
		if !ok || oc.Module == "" || oc.Export == "" {
			t.Errorf("%s: opencode.module/export missing, got %+v (block present = %v)", h.Name, oc, ok)
		}
		if graphify[h.Name] {
			if h.Enabled {
				t.Errorf("%s: Enabled = true, want false (Claude Code ships it off)", h.Name)
			}
			if !h.EnabledFor(TargetOpencode) {
				t.Errorf("%s: EnabledFor(%q) = false, want true", h.Name, TargetOpencode)
			}
		}
	}
}

// ── Paths that need shell quoting (#135) ─────────────────────────────────────

// needsQuoting are repo dir names with each kind of character Claude Code's
// shell (sh -c) would split on or interpret.
var needsQuoting = map[string]string{
	"space":      "My Proj",
	"quote":      "it's",
	"dollar":     "a$b",
	"semicolon":  "a;b",
	"ampersand":  "a&b",
	"everything": "My Proj/it's $x;&`y`(z)",
}

// decoys are where an unquoted command from each needsQuoting dir would land,
// relative to the dir's parent: cut at the space or operator, or with $b
// expanded to nothing. ("it's" is an unterminated quote: nothing runs at all.)
var decoys = map[string][]string{
	"space":      {"My"},
	"dollar":     {"a/hooks/claude-code/secret-guard.sh", "a/hooks/claude-code/dangerous-cmd-guard.sh"},
	"semicolon":  {"a"},
	"ampersand":  {"a"},
	"everything": {"My"},
}

func TestHookCommand(t *testing.T) {
	tests := map[string]struct {
		path, want string
	}{
		"plain path stays plain":      {path: "/opt/devexp/hooks/claude-code/secret-guard.sh", want: "/opt/devexp/hooks/claude-code/secret-guard.sh"},
		"space is quoted":             {path: "/My Proj/hooks/claude-code/secret-guard.sh", want: "'/My Proj/hooks/claude-code/secret-guard.sh'"},
		"single quote is escaped":     {path: "/it's/hooks/claude-code/secret-guard.sh", want: `'/it'\''s/hooks/claude-code/secret-guard.sh'`},
		"dollar is quoted":            {path: "/a$b/hooks/claude-code/secret-guard.sh", want: "'/a$b/hooks/claude-code/secret-guard.sh'"},
		"semicolon is quoted":         {path: "/a;b/hooks/claude-code/secret-guard.sh", want: "'/a;b/hooks/claude-code/secret-guard.sh'"},
		"ampersand is quoted":         {path: "/a&b/hooks/claude-code/secret-guard.sh", want: "'/a&b/hooks/claude-code/secret-guard.sh'"},
		"two quotes, both escaped":    {path: "/''/x.sh", want: `'/'\'''\''/x.sh'`},
		"backslash needs no escaping": {path: `/a\b/x.sh`, want: `'/a\b/x.sh'`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := hookCommand(tt.path); got != tt.want {
				t.Errorf("hookCommand(%q) = %q, want %q", tt.path, got, tt.want)
			}
			if got, ok := commandPath(tt.want); !ok || got != tt.path {
				t.Errorf("commandPath(%q) = %q, %v; want %q, true", tt.want, got, ok, tt.path)
			}
		})
	}
}

func TestCommandPath(t *testing.T) {
	const abs = "/My Proj/hooks/claude-code/secret-guard.sh"
	tests := map[string]struct {
		cmd    string
		want   string
		wantOK bool
	}{
		"plain absolute path":                   {cmd: "/opt/x/hooks/claude-code/secret-guard.sh", want: "/opt/x/hooks/claude-code/secret-guard.sh", wantOK: true},
		"plain relative path":                   {cmd: "hooks/claude-code/secret-guard.sh", want: "hooks/claude-code/secret-guard.sh", wantOK: true},
		"single-quoted absolute path":           {cmd: "'" + abs + "'", want: abs, wantOK: true},
		"single-quoted path needing no quoting": {cmd: "'/opt/x/secret-guard.sh'", want: "/opt/x/secret-guard.sh", wantOK: true},
		"escaped single quote":                  {cmd: `'/it'\''s/x.sh'`, want: "/it's/x.sh", wantOK: true},
		"empty":                                 {cmd: ""},
		"lone quote":                            {cmd: "'"},
		"empty quotes":                          {cmd: "''"},
		"unquoted space":                        {cmd: abs},
		"single-quoted relative path":           {cmd: "'hooks/claude-code/secret-guard.sh'"},
		"double-quoted":                         {cmd: `"` + abs + `"`},
		"quoted then argument":                  {cmd: "'" + abs + "' --flag"},
		"quoted then chained command":           {cmd: "'" + abs + "';true"},
		"quoted word ending in a quote after ;": {cmd: "'/a';'" + abs + "'"},
		"concatenated quoted and bare words":    {cmd: "'/My Proj/hooks/'claude-code/secret-guard.sh"},
		"bare word then quoted word":            {cmd: "/x'/My Proj/secret-guard.sh'"},
		"unbalanced quote inside":               {cmd: "'/it's/x.sh'"},
		"other escape spelling of a quote":      {cmd: `'/it'"'"'s/x.sh'`},
		"wrapper around a quoted path":          {cmd: "bash '" + abs + "'"},
		"variable":                              {cmd: "$CLAUDE_PROJECT_DIR/hooks/claude-code/secret-guard.sh"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := commandPath(tt.cmd)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("commandPath(%q) = %q, %v; want %q, %v", tt.cmd, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// TestInstallClaude_PathsNeedingQuotes registers hooks from repo dirs that need
// quoting, then runs every registered command the way Claude Code does — through
// sh -c — from an unrelated directory. Only the intended script may run: a decoy
// sits wherever an unquoted command would land (see decoys).
func TestInstallClaude_PathsNeedingQuotes(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	for name, dir := range needsQuoting {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			repoDir := filepath.Join(base, dir)
			log := filepath.Join(t.TempDir(), "ran.log")
			writeExec := func(p, body string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, s := range []string{"secret-guard.sh", "dangerous-cmd-guard.sh"} {
				writeExec(filepath.Join(repoDir, "hooks", "claude-code", s), `echo "intended `+s+`" >> "$LOG"`)
			}
			decoy := `echo "DECOY $0" >> "$LOG"`
			for _, p := range decoys[name] {
				writeExec(filepath.Join(base, p), decoy)
			}
			settingsPath := filepath.Join(t.TempDir(), "settings.json")

			var err error
			out := captureOutput(t, func() { err = InstallClaude(testRegistry(), repoDir, settingsPath, nil, false) })
			if err != nil {
				t.Fatalf("InstallClaude() error = %v\n%s", err, out)
			}

			cwd := t.TempDir()
			ran := 0
			for _, entries := range readHooks(t, settingsPath) {
				for _, e := range entries {
					for _, h := range e.Hooks {
						if h.Command == filepath.Join(repoDir, "hooks", "claude-code", filepath.Base(h.Command)) {
							t.Errorf("registered the unquoted path %q", h.Command)
						}
						c := exec.Command("sh", "-c", h.Command)
						c.Dir = cwd
						c.Env = append(os.Environ(), "LOG="+log, "b=", "x=", "y=", "z=")
						if combined, err := c.CombinedOutput(); err != nil {
							t.Errorf("sh -c %q: %v\n%s", h.Command, err, combined)
						}
						ran++
					}
				}
			}
			if ran != 2 {
				t.Fatalf("ran %d registered commands, want 2 (the enabled hooks)", ran)
			}
			got, _ := os.ReadFile(log)
			lines := strings.Split(strings.TrimSpace(string(got)), "\n")
			sort.Strings(lines)
			want := []string{"intended dangerous-cmd-guard.sh", "intended secret-guard.sh"}
			if !reflect.DeepEqual(lines, want) {
				t.Errorf("what ran = %q, want %q", lines, want)
			}
		})
	}
}

// TestInstallClaude_QuotingOnlyWhenNeeded: a path with no shell syntax is
// registered exactly as earlier releases wrote it, so existing settings don't
// change and older matchers still recognise the command.
func TestInstallClaude_QuotingOnlyWhenNeeded(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "plain-repo_1.0")
	scriptAbs := createScript(t, repoDir, "hooks/claude-code/secret-guard.sh")
	createScript(t, repoDir, "hooks/claude-code/dangerous-cmd-guard.sh")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	captureOutput(t, func() {
		if err := InstallClaude(testRegistry(), repoDir, settingsPath, nil, false); err != nil {
			t.Error(err)
		}
	})
	got := readHooks(t, settingsPath)["PreToolUse"]
	if len(got) == 0 || got[0].Hooks[0].Command != scriptAbs {
		t.Errorf("hooks = %+v, want the plain path %q first", got, scriptAbs)
	}
}

// TestInstallClaude_MigratesUnquotedCommands: an earlier install from a repo
// dir needing quotes wrote the bare path, which the shell splits. It is
// rewritten in place — disabled hooks included — and the user's own commands,
// however they quote a devexp path, are left exactly as they were.
func TestInstallClaude_MigratesUnquotedCommands(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "My Proj", "it's $x;&clone")
	otherRoot := filepath.Join(base, "Other Root", "cache")
	for _, s := range []string{"secret-guard.sh", "dangerous-cmd-guard.sh", "graphify-read-guard.sh"} {
		createScript(t, repoDir, "hooks/claude-code/"+s)
	}
	mine := func(s string) string { return filepath.Join(repoDir, "hooks", "claude-code", s) }
	foreign := filepath.Join(otherRoot, "hooks", "claude-code", "secret-guard.sh")
	userCmds := []string{
		`"` + mine("secret-guard.sh") + `"`,
		shellQuote(mine("secret-guard.sh")) + " --strict",
		"bash " + shellQuote(foreign),
		shellQuote(filepath.Join(otherRoot, "hooks", "claude-code")+"/") + "secret-guard.sh",
		shellQuote("hooks/claude-code/secret-guard.sh"),
		shellQuote(filepath.Join(otherRoot, "my-hooks", "claude-code", "secret-guard.sh")),
		foreign, // another root's unquoted path: indistinguishable from a command with arguments
	}
	var userHooks []hookCmd
	for _, c := range userCmds {
		userHooks = append(userHooks, hookCmd{Type: "command", Command: c})
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	writeSettingsHooks(t, settingsPath, hooksMapT{
		"PreToolUse": {
			{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: mine("secret-guard.sh")}}},
			{Matcher: "Read|Glob", Hooks: []hookCmd{{Type: "command", Command: mine("graphify-read-guard.sh")}}}, // disabled
			{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: shellQuote(filepath.Join(otherRoot, "hooks", "claude-code", "dangerous-cmd-guard.sh"))}}},
			{Matcher: "Write", Hooks: userHooks},
		},
	})

	var err error
	out := captureOutput(t, func() { err = InstallClaude(testRegistry(), repoDir, settingsPath, nil, false) })
	if err != nil {
		t.Fatalf("InstallClaude() error = %v\n%s", err, out)
	}
	want := hooksMapT{"PreToolUse": {
		{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: shellQuote(mine("secret-guard.sh"))}}},
		{Matcher: "Read|Glob", Hooks: []hookCmd{{Type: "command", Command: shellQuote(mine("graphify-read-guard.sh"))}}},
		{Matcher: "Write", Hooks: userHooks},
		{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: shellQuote(mine("dangerous-cmd-guard.sh"))}}},
	}}
	if got := readHooks(t, settingsPath); !reflect.DeepEqual(got, want) {
		t.Errorf("hooks =\n%+v\nwant\n%+v\noutput:\n%s", got, want, out)
	}

	// A second run finds everything registered and writes nothing.
	before, _ := os.ReadFile(settingsPath)
	out = captureOutput(t, func() { err = InstallClaude(testRegistry(), repoDir, settingsPath, nil, false) })
	if err != nil {
		t.Fatalf("second InstallClaude() error = %v", err)
	}
	if after, _ := os.ReadFile(settingsPath); string(after) != string(before) || strings.Contains(out, "Saved:") {
		t.Errorf("second install changed settings.json:\n%s", out)
	}
}

// TestInstallClaude_RequotesQuotedPlainPath: a quoted form of a path that
// needs no quoting runs the same script, so it is devexp's and is rewritten to
// the plain form rather than registered twice.
func TestInstallClaude_RequotesQuotedPlainPath(t *testing.T) {
	repoDir := t.TempDir()
	scriptAbs := createScript(t, repoDir, "hooks/claude-code/secret-guard.sh")
	createScript(t, repoDir, "hooks/claude-code/dangerous-cmd-guard.sh")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	writeSettingsHooks(t, settingsPath, hooksMapT{"PreToolUse": {
		{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: "'" + scriptAbs + "'"}}},
	}})
	captureOutput(t, func() {
		if err := InstallClaude(testRegistry(), repoDir, settingsPath, nil, false); err != nil {
			t.Error(err)
		}
	})
	got := readHooks(t, settingsPath)["PreToolUse"]
	if len(got) != 2 || got[0].Hooks[0].Command != scriptAbs {
		t.Errorf("hooks = %+v, want %q rewritten plain and one dangerous-cmd-guard entry", got, scriptAbs)
	}
}

func TestQuotedDevexpHookMatchers(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "My Proj")
	otherRoot := filepath.Join(base, "it's $other;&")
	createScript(t, repoDir, "hooks/claude-code/secret-guard.sh")
	managed := managedScriptNames(testRegistry())
	sd := func(root, s string) string { return filepath.Join(root, "hooks", "claude-code", s) }

	tests := map[string]struct {
		cmd                      string
		foreign, relative, stale bool
	}{
		"quoted own hook":                        {cmd: shellQuote(sd(repoDir, "secret-guard.sh"))},
		"quoted own hook, script removed":        {cmd: shellQuote(sd(repoDir, "removed.sh")), stale: true},
		"quoted foreign hook":                    {cmd: shellQuote(sd(otherRoot, "secret-guard.sh")), foreign: true},
		"quoted foreign copy of a disabled hook": {cmd: shellQuote(sd(otherRoot, "graphify-read-guard.sh")), foreign: true},
		"quoted foreign plain-path hook":         {cmd: "'/opt/devexp/hooks/claude-code/secret-guard.sh'", foreign: true},
		"plain foreign hook":                     {cmd: "/opt/devexp/hooks/claude-code/secret-guard.sh", foreign: true},
		"plain relative hook":                    {cmd: "hooks/claude-code/secret-guard.sh", relative: true},
		"quoted relative path":                   {cmd: "'hooks/claude-code/secret-guard.sh'"},
		"quoted unknown script":                  {cmd: shellQuote(sd(otherRoot, "mine.sh"))},
		"quoted my-hooks/claude-code":            {cmd: shellQuote(filepath.Join(otherRoot, "my-hooks", "claude-code", "secret-guard.sh"))},
		"double-quoted foreign hook":             {cmd: `"` + sd(otherRoot, "secret-guard.sh") + `"`},
		"quoted foreign hook with argument":      {cmd: shellQuote(sd(otherRoot, "secret-guard.sh")) + " --x"},
		"quoted foreign hook, chained":           {cmd: shellQuote(sd(otherRoot, "secret-guard.sh")) + ";true"},
		"wrapper around quoted foreign hook":     {cmd: "bash " + shellQuote(sd(otherRoot, "secret-guard.sh"))},
		"concatenated words":                     {cmd: shellQuote(filepath.Join(otherRoot, "hooks")) + "/claude-code/secret-guard.sh"},
		"two quoted words chained":               {cmd: shellQuote(filepath.Join(otherRoot, "setup")) + ";" + shellQuote(sd(otherRoot, "secret-guard.sh"))},
		"quote escaped another way":              {cmd: `'/it'"'"'s/hooks/claude-code/secret-guard.sh'`},
		"unquoted foreign path needing quotes":   {cmd: sd(otherRoot, "secret-guard.sh")},
		"quoted variable":                        {cmd: `'$CLAUDE_PROJECT_DIR/hooks/claude-code/secret-guard.sh'`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isForeignDevexpHook(tt.cmd, managed, repoDir); got != tt.foreign {
				t.Errorf("isForeignDevexpHook(%q) = %v, want %v", tt.cmd, got, tt.foreign)
			}
			if got := isRelativeDevexpHook(tt.cmd, managed); got != tt.relative {
				t.Errorf("isRelativeDevexpHook(%q) = %v, want %v", tt.cmd, got, tt.relative)
			}
			if got := isStaleDevexpHook(tt.cmd, repoDir); got != tt.stale {
				t.Errorf("isStaleDevexpHook(%q) = %v, want %v", tt.cmd, got, tt.stale)
			}
		})
	}
}

// runHooks runs every command registered in settingsPath through sh -c, the way
// Claude Code does, with LOG set, from an unrelated directory.
func runHooks(t *testing.T, settingsPath, log string) {
	t.Helper()
	cwd := t.TempDir()
	for _, entries := range readHooks(t, settingsPath) {
		for _, e := range entries {
			for _, h := range e.Hooks {
				c := exec.Command("sh", "-c", h.Command)
				c.Dir = cwd
				c.Env = append(os.Environ(), "LOG="+log)
				// A user's command may legitimately fail here; only what ran matters.
				c.Run() //nolint:errcheck
			}
		}
	}
}

// loggedLines returns the sorted lines of log (none if it doesn't exist).
func loggedLines(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	sort.Strings(lines)
	return lines
}

// loggingScript writes an executable script at repoDir/relPath that appends
// "<name>" to $LOG each time it runs.
func loggingScript(t *testing.T, repoDir, relPath string) string {
	t.Helper()
	abs := filepath.Join(repoDir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\necho " + filepath.Base(relPath) + ` >> "$LOG"` + "\n"
	if err := os.WriteFile(abs, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return abs
}

func commands(hooks hooksMapT) []string {
	var out []string
	for _, entries := range hooks {
		for _, e := range entries {
			for _, h := range e.Hooks {
				out = append(out, h.Command)
			}
		}
	}
	sort.Strings(out)
	return out
}

// TestInstallClaude_DoubleQuotedHandFix: "<repo>/hooks/claude-code/<script>" is
// the natural hand fix for an unquoted path. When the path has no $, backquote,
// \ or ", those double quotes are literal, so it is this repo's script: it is
// rewritten to the registered form instead of gaining a second copy that runs
// alongside it.
func TestInstallClaude_DoubleQuotedHandFix(t *testing.T) {
	for name, dir := range map[string]string{
		"needs quoting":    "My Proj/it's (x) & y",
		"needs no quoting": "plain-repo",
	} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			repoDir := filepath.Join(base, dir)
			for _, s := range []string{"secret-guard.sh", "dangerous-cmd-guard.sh", "graphify-read-guard.sh"} {
				loggingScript(t, repoDir, "hooks/claude-code/"+s)
			}
			mine := func(s string) string { return filepath.Join(repoDir, "hooks", "claude-code", s) }
			foreign := loggingScript(t, filepath.Join(base, "Other Root"), "hooks/claude-code/secret-guard.sh")
			userCmds := []hookCmd{
				{Type: "command", Command: `"` + mine("secret-guard.sh") + `" --strict`}, // arguments
				{Type: "command", Command: `"` + foreign + `"`},                          // another root
				{Type: "command", Command: `"` + filepath.Join(repoDir, "hooks", "claude-code", "mine.sh") + `"`},
			}
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			writeSettingsHooks(t, settingsPath, hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: `"` + mine("secret-guard.sh") + `"`}}},
				{Matcher: "Read|Glob", Hooks: []hookCmd{{Type: "command", Command: `"` + mine("graphify-read-guard.sh") + `"`}}}, // disabled
				{Matcher: "Write", Hooks: userCmds},
			}})

			var err error
			out := captureOutput(t, func() { err = InstallClaude(testRegistry(), repoDir, settingsPath, nil, false) })
			if err != nil {
				t.Fatalf("InstallClaude() error = %v\n%s", err, out)
			}
			want := hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: hookCommand(mine("secret-guard.sh"))}}},
				{Matcher: "Read|Glob", Hooks: []hookCmd{{Type: "command", Command: hookCommand(mine("graphify-read-guard.sh"))}}},
				{Matcher: "Write", Hooks: userCmds},
				{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: hookCommand(mine("dangerous-cmd-guard.sh"))}}},
			}}
			if got := readHooks(t, settingsPath); !reflect.DeepEqual(got, want) {
				t.Fatalf("hooks =\n%+v\nwant\n%+v\noutput:\n%s", got, want, out)
			}

			before, _ := os.ReadFile(settingsPath)
			out = captureOutput(t, func() { err = InstallClaude(testRegistry(), repoDir, settingsPath, nil, false) })
			if after, _ := os.ReadFile(settingsPath); err != nil || string(after) != string(before) {
				t.Errorf("second install changed settings.json (err %v):\n%s", err, out)
			}

			// Each of this repo's scripts runs once. The user's "…" --strict
			// runs it once more, and the foreign root's copy runs as its own.
			log := filepath.Join(t.TempDir(), "ran.log")
			runHooks(t, settingsPath, log)
			wantRan := []string{"dangerous-cmd-guard.sh", "graphify-read-guard.sh", "secret-guard.sh", "secret-guard.sh", "secret-guard.sh"}
			if got := loggedLines(t, log); !reflect.DeepEqual(got, wantRan) {
				t.Errorf("ran %q, want %q", got, wantRan)
			}
		})
	}
}

// TestInstallClaude_DoubleQuotedWithShellSpecials: inside double quotes $,
// backquote and \ are still special, so "<path>" is not that path and stays
// the user's.
func TestInstallClaude_DoubleQuotedWithShellSpecials(t *testing.T) {
	for name, dir := range map[string]string{"dollar": "a$b", "backquote": "a`b`", "backslash": `a\b`} {
		t.Run(name, func(t *testing.T) {
			repoDir := filepath.Join(t.TempDir(), dir)
			createScript(t, repoDir, "hooks/claude-code/secret-guard.sh")
			createScript(t, repoDir, "hooks/claude-code/dangerous-cmd-guard.sh")
			scriptAbs := filepath.Join(repoDir, "hooks", "claude-code", "secret-guard.sh")
			userCmd := `"` + scriptAbs + `"`
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			writeSettingsHooks(t, settingsPath, hooksMapT{"PreToolUse": {
				{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: userCmd}}},
			}})
			captureOutput(t, func() {
				if err := InstallClaude(testRegistry(), repoDir, settingsPath, nil, false); err != nil {
					t.Error(err)
				}
			})
			got := commands(readHooks(t, settingsPath))
			wantCmds := []string{userCmd, shellQuote(filepath.Join(repoDir, "hooks", "claude-code", "dangerous-cmd-guard.sh")), shellQuote(scriptAbs)}
			sort.Strings(wantCmds)
			if !reflect.DeepEqual(got, wantCmds) {
				t.Errorf("commands = %q, want %q", got, wantCmds)
			}
		})
	}
}

// oldInstallClaude does what InstallClaude did before #135 for one hook: append
// the bare path unless that exact string is already registered. It stands in
// for an older devexp run against the same settings.json.
func oldInstallClaude(t *testing.T, settingsPath, event, matcher, scriptAbs string) {
	t.Helper()
	hooks := readHooks(t, settingsPath)
	for _, e := range hooks[event] {
		for _, h := range e.Hooks {
			if h.Command == scriptAbs {
				return
			}
		}
	}
	if hooks == nil {
		hooks = hooksMapT{}
	}
	hooks[event] = append(hooks[event], hookEntry{Matcher: matcher, Hooks: []hookCmd{{Type: "command", Command: scriptAbs}}})
	writeSettingsHooks(t, settingsPath, hooks)
}

// TestInstallClaude_CollapsesLegacyDuplicates: installing with this release,
// then an older one (which appends the bare path it can't match), then this
// release again must leave one registration per script, not a legacy entry
// rewritten into an exact duplicate.
func TestInstallClaude_CollapsesLegacyDuplicates(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "My Proj")
	registry := testRegistry()
	for _, h := range registry {
		loggingScript(t, repoDir, h.Targets[TargetClaudeCode].Script)
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	install := func() {
		t.Helper()
		var err error
		out := captureOutput(t, func() { err = InstallClaude(registry, repoDir, settingsPath, nil, false) })
		if err != nil {
			t.Fatalf("InstallClaude() error = %v\n%s", err, out)
		}
	}

	install()                    // new
	for _, h := range registry { // old
		cc := h.Targets[TargetClaudeCode]
		if h.Enabled {
			oldInstallClaude(t, settingsPath, cc.Event, cc.Matcher, filepath.Join(repoDir, cc.Script))
		}
	}
	// A hand fix in an entry that also holds a user command: only the
	// duplicate goes, the user command and the entry's matcher stay.
	hooks := readHooks(t, settingsPath)
	hooks["PreToolUse"] = append(hooks["PreToolUse"], hookEntry{Matcher: "Read", Hooks: []hookCmd{
		{Type: "command", Command: "/usr/local/bin/my-hook"},
		{Type: "command", Command: `"` + filepath.Join(repoDir, "hooks", "claude-code", "secret-guard.sh") + `"`},
	}})
	writeSettingsHooks(t, settingsPath, hooks)
	install() // new
	install() // new

	want := []string{
		"/usr/local/bin/my-hook",
		shellQuote(filepath.Join(repoDir, "hooks", "claude-code", "dangerous-cmd-guard.sh")),
		shellQuote(filepath.Join(repoDir, "hooks", "claude-code", "secret-guard.sh")),
	}
	sort.Strings(want)
	got := readHooks(t, settingsPath)
	if cmds := commands(got); !reflect.DeepEqual(cmds, want) {
		t.Fatalf("commands = %q, want %q", cmds, want)
	}
	for event, entries := range got {
		for _, e := range entries {
			if len(e.Hooks) == 0 {
				t.Errorf("%s: entry %+v left with no commands", event, e)
			}
		}
	}
	var userEntry *hookEntry
	for i, e := range got["PreToolUse"] {
		if e.Matcher == "Read" {
			userEntry = &got["PreToolUse"][i]
		}
	}
	if userEntry == nil || len(userEntry.Hooks) != 1 || userEntry.Hooks[0].Command != "/usr/local/bin/my-hook" {
		t.Errorf("user entry = %+v, want matcher Read holding only /usr/local/bin/my-hook", userEntry)
	}

	log := filepath.Join(t.TempDir(), "ran.log")
	runHooks(t, settingsPath, log)
	if lines := loggedLines(t, log); !reflect.DeepEqual(lines, []string{"dangerous-cmd-guard.sh", "secret-guard.sh"}) {
		t.Errorf("ran %q, want each enabled script once", lines)
	}
}

// shellSyntaxChars lists, independently of shellSyntax, every character
// Claude Code's shell would split on or interpret in a hook command.
var shellSyntaxChars = []string{
	" ", "\t", "\n", "\r", "$", "~", "'", `"`, "`", `\`, ";", "&", "|",
	"<", ">", "(", ")", "*", "?", "[", "]", "{", "}", "!", "#",
}

func TestShellSyntax_IsExactlyTheListedCharacters(t *testing.T) {
	got := strings.Split(shellSyntax, "")
	want := append([]string(nil), shellSyntaxChars...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shellSyntax = %q, want %q", got, want)
	}
}

// TestInstallClaude_EveryShellSyntaxCharacter: a repo dir holding any one of
// these characters is registered quoted, and sh -c runs exactly that script.
// Characters the shell treats literally keep the plain form.
func TestInstallClaude_EveryShellSyntaxCharacter(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	registry := Registry{ccHook("secret-guard", true, "PreToolUse", "Read", "hooks/claude-code/secret-guard.sh")}
	check := func(t *testing.T, char string, wantQuoted bool) {
		repoDir := filepath.Join(t.TempDir(), "a"+char+"b")
		scriptAbs := loggingScript(t, repoDir, "hooks/claude-code/secret-guard.sh")
		settingsPath := filepath.Join(t.TempDir(), "settings.json")
		var err error
		out := captureOutput(t, func() { err = InstallClaude(registry, repoDir, settingsPath, nil, false) })
		if err != nil {
			t.Fatalf("InstallClaude() error = %v\n%s", err, out)
		}
		cmds := commands(readHooks(t, settingsPath))
		want := scriptAbs
		if wantQuoted {
			want = shellQuote(scriptAbs)
		}
		if len(cmds) != 1 || cmds[0] != want {
			t.Fatalf("commands = %q, want [%q]", cmds, want)
		}
		log := filepath.Join(t.TempDir(), "ran.log")
		runHooks(t, settingsPath, log)
		if lines := loggedLines(t, log); !reflect.DeepEqual(lines, []string{"secret-guard.sh"}) {
			t.Errorf("sh -c %q ran %q, want the script once", cmds[0], lines)
		}
	}
	for _, c := range shellSyntaxChars {
		t.Run(fmt.Sprintf("quoted %q", c), func(t *testing.T) { check(t, c, true) })
	}
	for _, c := range []string{"-", "_", ".", "+", "@", "%", ",", ":", "=", "é", "ß"} {
		t.Run(fmt.Sprintf("plain %q", c), func(t *testing.T) { check(t, c, false) })
	}
}

// TestShellSyntax_MatchesUninstallScript: uninstall.sh keeps its own copy of
// the set; if the two drift, one of them misjudges which commands are devexp's.
func TestShellSyntax_MatchesUninstallScript(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "uninstall.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^SHELL_SYNTAX = set\((.*)\)$`).FindSubmatch(src)
	if m == nil {
		t.Fatal("no SHELL_SYNTAX = set(...) line in uninstall.sh")
	}
	out, err := exec.Command(py, "-c", "import ast, json, sys; print(json.dumps(sorted(ast.literal_eval(sys.argv[1]))))", string(m[1])).Output()
	if err != nil {
		t.Fatalf("evaluating %s: %v", m[1], err)
	}
	var got []string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := strings.Split(shellSyntax, "")
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("uninstall.sh SHELL_SYNTAX = %q, want shellSyntax %q", got, want)
	}
}

// TestRequoteDevexpHooks_DropsEmptiedEntry: removing a duplicate spelling must
// not leave an entry with no commands behind, and must keep the event's other
// entries and commands as they were.
func TestRequoteDevexpHooks_DropsEmptiedEntry(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "My Proj")
	scriptAbs := filepath.Join(repoDir, "hooks", "claude-code", "secret-guard.sh")
	hooks := hooksMapT{
		"PreToolUse": {
			{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: shellQuote(scriptAbs)}}},
			{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: scriptAbs}}},
			{Matcher: "Read", Hooks: []hookCmd{{Type: "command", Command: `"` + scriptAbs + `"`}, {Type: "command", Command: "/usr/local/bin/my-hook"}}},
		},
		"PostToolUse": {
			{Matcher: "Write", Hooks: []hookCmd{{Type: "command", Command: scriptAbs}}},
		},
	}
	var changed bool
	captureOutput(t, func() { changed = requoteDevexpHooks(hooks, testRegistry(), repoDir, false) })
	want := hooksMapT{
		"PreToolUse": {
			{Matcher: "Read|Bash", Hooks: []hookCmd{{Type: "command", Command: shellQuote(scriptAbs)}}},
			{Matcher: "Read", Hooks: []hookCmd{{Type: "command", Command: "/usr/local/bin/my-hook"}}},
		},
		// Another event is a separate registration: rewritten, not dropped.
		"PostToolUse": {
			{Matcher: "Write", Hooks: []hookCmd{{Type: "command", Command: shellQuote(scriptAbs)}}},
		},
	}
	if !changed || !reflect.DeepEqual(hooks, want) {
		t.Errorf("changed = %v, hooks =\n%+v\nwant\n%+v", changed, hooks, want)
	}
}

// devexpRoot makes dir look like a devexp install root: a hooks registry at
// hooks/registry.json.
func devexpRoot(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "hooks", "claude-code"), 0755); err != nil {
		t.Fatal(err)
	}
	reg := `[{"name": "secret-guard", "enabled": true, "claude_code": {"event": "PreToolUse", "script": "hooks/claude-code/secret-guard.sh"}}]`
	if err := os.WriteFile(filepath.Join(dir, "hooks", "registry.json"), []byte(reg), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestIsDevexpRoot(t *testing.T) {
	tests := map[string]struct {
		registry *string // nil: no file
		dir      bool    // hooks/registry.json is a directory
		want     bool
	}{
		"a registry":                      {registry: ptr(`[{"name": "a"}, {"name": "b", "enabled": false}]`), want: true},
		"no registry":                     {},
		"registry is a directory":         {dir: true},
		"not JSON":                        {registry: ptr(`[{"name": "a"`)},
		"an object, not an array":         {registry: ptr(`{"name": "a"}`)},
		"an empty array":                  {registry: ptr(`[]`)},
		"an element that isn't an object": {registry: ptr(`[{"name": "a"}, "b"]`)},
		"a null element":                  {registry: ptr(`[{"name": "a"}, null]`)},
		"an element without a name":       {registry: ptr(`[{"name": "a"}, {"description": "x"}]`)},
		"an empty name":                   {registry: ptr(`[{"name": ""}]`)},
		"a name that isn't a string":      {registry: ptr(`[{"name": 1}]`)},
		"a null name":                     {registry: ptr(`[{"name": null}]`)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			p := filepath.Join(root, "hooks", "registry.json")
			os.MkdirAll(filepath.Dir(p), 0755) //nolint:errcheck
			switch {
			case tt.dir:
				os.Mkdir(p, 0755) //nolint:errcheck
			case tt.registry != nil:
				os.WriteFile(p, []byte(*tt.registry), 0644) //nolint:errcheck
			}
			if got := isDevexpRoot(root); got != tt.want {
				t.Errorf("isDevexpRoot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

// TestIsOrphanedDevexpHook (#150): only devexp's own form, naming a missing
// script directly under <root>/hooks/claude-code/ of another devexp root.
func TestIsOrphanedDevexpHook(t *testing.T) {
	base := t.TempDir()
	// So the relative cases name a real devexp root (plain/) and are refused
	// for being relative, not for naming nothing.
	t.Chdir(base)
	repoDir := devexpRoot(t, filepath.Join(base, "repo"))
	other := devexpRoot(t, filepath.Join(base, "other checkout's $root"))
	userDir := filepath.Join(base, "user")
	os.MkdirAll(filepath.Join(userDir, "hooks", "claude-code"), 0755) //nolint:errcheck
	createScript(t, other, "hooks/claude-code/secret-guard.sh")
	gone := filepath.Join(other, "hooks", "claude-code", "removed.sh")
	plainOther := devexpRoot(t, filepath.Join(base, "plain"))
	plainGone := filepath.Join(plainOther, "hooks", "claude-code", "removed.sh")

	tests := map[string]struct {
		cmd  string
		want bool
	}{
		"missing script in another root, single-quoted": {cmd: shellQuote(gone), want: true},
		"missing script in another root, plain":         {cmd: plainGone, want: true},
		"missing script in another root, bare path needing quotes": {
			cmd: gone, // the shell splits it: not a form devexp writes for this path
		},
		"existing script in another root":                                  {cmd: shellQuote(filepath.Join(other, "hooks", "claude-code", "secret-guard.sh"))},
		"missing script in a root with no registry":                        {cmd: filepath.Join(userDir, "hooks", "claude-code", "removed.sh")},
		"missing script in a root that is gone":                            {cmd: filepath.Join(base, "deleted", "hooks", "claude-code", "removed.sh")},
		"missing script under repoDir (not orphaned: isStaleDevexpHook's)": {cmd: filepath.Join(repoDir, "hooks", "claude-code", "removed.sh")},
		"with an argument":                                                 {cmd: plainGone + " --flag"},
		"double-quoted":                                                    {cmd: `"` + plainGone + `"`},
		"through a wrapper":                                                {cmd: "bash " + plainGone},
		"chained":                                                          {cmd: plainGone + ";true"},
		"through a variable":                                               {cmd: "$HOME/plain/hooks/claude-code/removed.sh"},
		"relative":                                                         {cmd: "plain/hooks/claude-code/removed.sh"},
		"nested below hooks/claude-code/":                                  {cmd: filepath.Join(plainOther, "hooks", "claude-code", "sub", "removed.sh")},
		"directly under hooks/":                                            {cmd: filepath.Join(plainOther, "hooks", "removed.sh")},
		"under my-hooks/claude-code/":                                      {cmd: filepath.Join(plainOther, "my-hooks", "claude-code", "removed.sh")},
		"an unclean path":                                                  {cmd: plainOther + "/hooks/../hooks/claude-code/removed.sh"},
		"the scripts directory itself":                                     {cmd: filepath.Join(plainOther, "hooks", "claude-code") + "/"},
		"at the file-system root":                                          {cmd: "/hooks/claude-code/removed.sh"},
		"a leading // (Clean folds it, so not a clean path)":               {cmd: "/" + plainGone},
		"relative with ./":                                                 {cmd: "./plain/hooks/claude-code/removed.sh"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isOrphanedDevexpHook(tt.cmd, repoDir, map[string]bool{}); got != tt.want {
				t.Errorf("isOrphanedDevexpHook(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// TestPruneStaleHooks_RootsCachedPerAnswer (#157 review): the per-pass cache of
// isDevexpRoot holds each root's own answer. Two missing scripts under one
// user directory without a registry both stay, and two under one devexp root
// both go, whichever is judged first.
func TestPruneStaleHooks_RootsCachedPerAnswer(t *testing.T) {
	base := t.TempDir()
	repoDir := devexpRoot(t, filepath.Join(base, "repo"))
	userRoot := filepath.Join(base, "user")
	os.MkdirAll(filepath.Join(userRoot, "hooks", "claude-code"), 0755) //nolint:errcheck
	otherRoot := devexpRoot(t, filepath.Join(base, "other"))
	user1 := filepath.Join(userRoot, "hooks", "claude-code", "one.sh")
	user2 := filepath.Join(userRoot, "hooks", "claude-code", "two.sh")
	gone1 := filepath.Join(otherRoot, "hooks", "claude-code", "one.sh")
	gone2 := filepath.Join(otherRoot, "hooks", "claude-code", "two.sh")
	for name, order := range map[string][]string{
		"user root first":   {user1, user2, gone1, gone2},
		"devexp root first": {gone1, gone2, user1, user2},
	} {
		t.Run(name, func(t *testing.T) {
			var cmds []hookCmd
			for _, c := range order {
				cmds = append(cmds, hookCmd{Type: "command", Command: c})
			}
			hooksMap := hooksMapT{"Stop": {{Hooks: cmds}}}
			var pruned bool
			captureOutput(t, func() { pruned = pruneStaleHooks(hooksMap, repoDir, false) })
			var got []string
			for _, h := range hooksMap["Stop"][0].Hooks {
				got = append(got, h.Command)
			}
			if want := []string{user1, user2}; !pruned || !reflect.DeepEqual(got, want) {
				t.Errorf("pruned=%v, kept %q, want %q", pruned, got, want)
			}
		})
	}
}

// TestInstallClaude_PrunesOrphanedHooksFromOtherRoots (#150): a registration
// from another install root whose script left that root's registry is pruned;
// every user command that merely looks similar stays, fields and all.
func TestInstallClaude_PrunesOrphanedHooksFromOtherRoots(t *testing.T) {
	base := t.TempDir()
	repoDir := devexpRoot(t, filepath.Join(base, "repo"))
	createScript(t, repoDir, "hooks/claude-code/secret-guard.sh")
	registry := Registry{ccHook("secret-guard", true, "PreToolUse", "Read", "hooks/claude-code/secret-guard.sh")}
	other := devexpRoot(t, filepath.Join(base, "My Checkout"))
	gone := filepath.Join(other, "hooks", "claude-code", "pre-tool-use.sh")
	userRoot := filepath.Join(base, "dotfiles")
	os.MkdirAll(filepath.Join(userRoot, "hooks", "claude-code"), 0755) //nolint:errcheck

	user := []string{
		filepath.Join(userRoot, "hooks", "claude-code", "pre-tool-use.sh"), // no registry in that root
		shellQuote(gone) + " --flag",
		`"` + gone + `"`,
		"bash " + shellQuote(gone),
		shellQuote(filepath.Join(other, "hooks", "claude-code", "sub", "x.sh")),
	}
	settingsPath := filepath.Join(base, "settings.json")
	var cmds []hookCmd
	for _, c := range user {
		cmds = append(cmds, hookCmd{Type: "command", Command: c})
	}
	cmds = append(cmds, hookCmd{Type: "command", Command: shellQuote(gone)})
	writeSettingsHooks(t, settingsPath, hooksMapT{"Stop": {{Hooks: cmds}}})
	// An exec-form handler naming the orphaned script is the user's too.
	data, _ := os.ReadFile(settingsPath)
	data = []byte(strings.Replace(string(data), `"hooks": [`, `"hooks": [{"type": "command", "command": `+strconvQuote(shellQuote(gone))+`, "args": []},`, 1))
	os.WriteFile(settingsPath, data, 0644) //nolint:errcheck

	for _, dryRun := range []bool{true, false} {
		var err error
		out := captureOutput(t, func() { err = InstallClaude(registry, repoDir, settingsPath, nil, dryRun) })
		if err != nil {
			t.Fatalf("InstallClaude(dryRun=%v) error = %v\n%s", dryRun, err, out)
		}
		if !strings.Contains(out, "pre-tool-use.sh (script no longer exists, in another install root)") {
			t.Errorf("dryRun=%v: output doesn't report the pruned registration:\n%s", dryRun, out)
		}
		if dryRun {
			if after, _ := os.ReadFile(settingsPath); string(after) != string(data) {
				t.Errorf("dry run changed settings.json")
			}
		}
	}
	got := readHooks(t, settingsPath)["Stop"][0].Hooks
	var gotCmds []string
	for _, h := range got {
		gotCmds = append(gotCmds, h.Command)
	}
	want := append([]string{shellQuote(gone)}, user...) // the exec-form one first
	if !reflect.DeepEqual(gotCmds, want) {
		t.Errorf("Stop commands =\n%q\nwant\n%q", gotCmds, want)
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestInstallClaude_PerTargetEnabled (#150): Claude Code registration follows
// EnabledFor(TargetClaudeCode), so a hook's claude_code.enabled overrides its
// top-level enabled either way.
func TestInstallClaude_PerTargetEnabled(t *testing.T) {
	on, off := true, false
	repoDir := t.TempDir()
	for _, s := range []string{"opencode-only.sh", "cc-only.sh", "both.sh", "neither.sh"} {
		createScript(t, repoDir, "hooks/claude-code/"+s)
	}
	hook := func(name string, enabled bool, ccEnabled, ocEnabled *bool) Hook {
		return Hook{Name: name, Enabled: enabled, Targets: map[string]TargetSpec{
			TargetClaudeCode: {Event: "PreToolUse", Matcher: "Bash", Script: "hooks/claude-code/" + name + ".sh", Enabled: ccEnabled},
			TargetOpencode:   {Event: "tool.execute.before", Module: "hooks/opencode/" + name + ".js", Enabled: ocEnabled},
		}}
	}
	registry := Registry{
		hook("opencode-only", true, &off, nil), // enabled at the top, off for Claude Code
		hook("cc-only", false, &on, nil),       // off at the top, on for Claude Code
		hook("both", true, nil, nil),
		hook("neither", false, nil, &on), // the graphify-* shape: opencode only
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	var err error
	out := captureOutput(t, func() { err = InstallClaude(registry, repoDir, settingsPath, nil, false) })
	if err != nil {
		t.Fatalf("InstallClaude() error = %v\n%s", err, out)
	}
	var got []string
	for _, e := range readHooks(t, settingsPath)["PreToolUse"] {
		for _, h := range e.Hooks {
			got = append(got, filepath.Base(h.Command))
		}
	}
	sort.Strings(got)
	if want := []string{"both.sh", "cc-only.sh"}; !reflect.DeepEqual(got, want) {
		t.Errorf("registered %q, want %q", got, want)
	}
}

// TestInstallClaude_SymlinkedSettings (#124): settings.json managed as a
// symlink keeps its link; the file it points at gets the hooks, atomically and
// with its own mode. A dangling link is refused and nothing is created.
func TestInstallClaude_SymlinkedSettings(t *testing.T) {
	repoDir := t.TempDir()
	createScript(t, repoDir, "hooks/claude-code/foo.sh")
	registry := Registry{ccHook("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh")}

	t.Run("link kept, target updated", func(t *testing.T) {
		dotfiles := t.TempDir()
		target := filepath.Join(dotfiles, "settings.json")
		os.WriteFile(target, []byte("{\n  \"model\": \"opus\"\n}\n"), 0600) //nolint:errcheck
		os.Chmod(target, 0600)                                              //nolint:errcheck
		settingsPath := filepath.Join(t.TempDir(), "settings.json")
		os.Symlink(target, settingsPath) //nolint:errcheck
		var err error
		out := captureOutput(t, func() { err = InstallClaude(registry, repoDir, settingsPath, nil, false) })
		if err != nil {
			t.Fatalf("InstallClaude() error = %v\n%s", err, out)
		}
		if fi, err := os.Lstat(settingsPath); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("settings.json symlink replaced")
		}
		if got := readHooks(t, target)["PreToolUse"]; len(got) != 1 {
			t.Errorf("target hooks = %+v, want foo registered", got)
		}
		if fi, _ := os.Stat(target); fi.Mode().Perm() != 0600 {
			t.Errorf("target mode = %v, want 0600", fi.Mode().Perm())
		}
		if entries, _ := os.ReadDir(dotfiles); len(entries) != 1 {
			t.Errorf("dotfiles holds %d entries, want only settings.json", len(entries))
		}
	})

	t.Run("dangling link refused", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "settings.json")
		settingsPath := filepath.Join(t.TempDir(), "settings.json")
		os.Symlink(missing, settingsPath) //nolint:errcheck
		var err error
		out := captureOutput(t, func() { err = InstallClaude(registry, repoDir, settingsPath, nil, false) })
		if err == nil || !strings.Contains(err.Error(), "left untouched") {
			t.Errorf("InstallClaude() error = %v, want a dangling-link refusal\n%s", err, out)
		}
		if _, err := os.Lstat(missing); !os.IsNotExist(err) {
			t.Errorf("a settings.json was created at the link's destination")
		}
	})
}

// ── The scan budget's hook timeout (#162) ────────────────────────────────────

// ccHookTimeout is ccHook with a claude_code.timeout, as the fail-closed guards
// carry in the real registry.
func ccHookTimeout(name string, enabled bool, event, matcher, script string, timeout int) Hook {
	h := ccHook(name, enabled, event, matcher, script)
	spec := h.Targets[TargetClaudeCode]
	spec.Timeout = timeout
	h.Targets[TargetClaudeCode] = spec
	return h
}

// readNumber returns the integer assigned to name in the file at path, as
// `name = 1234` or `name=1234` spell it in a shell or a JS source.
func readNumber(t *testing.T, path, name string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	m := regexp.MustCompile(regexp.QuoteMeta(name) + `\s*=\s*(\d+)`).FindSubmatch(data)
	if m == nil {
		t.Fatalf("%s: no assignment to %s", path, name)
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("%s: %s = %q, not a number", path, name, m[1])
	}
	return n
}

// TestRepoRegistry_FailClosedTimeouts pins the relationship the scan budget
// depends on: a timed-out command hook does not block the tool call, so every
// guard that enforces a budget must be registered with a Claude Code timeout
// strictly above the largest budget it can run with — otherwise Claude Code
// cancels the guard before it can exit 2, and the call goes through unscanned
// (#162). It also pins the two twins to one default and one ceiling, so a
// change to either is a change to both.
func TestRepoRegistry_FailClosedTimeouts(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	registry, err := LoadRegistry(filepath.Join(root, "hooks", "registry.json"))
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}

	sh := filepath.Join(root, "hooks", "claude-code", "scan-budget.sh")
	js := filepath.Join(root, "hooks", "opencode", "utils.js")
	budgetMs := readNumber(t, sh, "DEVEXP_SCAN_BUDGET_DEFAULT_MS")
	if jsBudgetMs := readNumber(t, js, "SCAN_BUDGET_DEFAULT_MS"); budgetMs != jsBudgetMs {
		t.Errorf("scan budget = %d ms in the Claude Code twin, %d ms in the opencode twin; they must agree", budgetMs, jsBudgetMs)
	}
	// The ceiling, not the default, is the largest budget a guard can run with:
	// DEVEXP_SCAN_BUDGET_MS is clamped to it, so it is what has to clear the
	// registered timeout.
	maxMs := readNumber(t, sh, "DEVEXP_SCAN_BUDGET_MAX_MS")
	if jsMaxMs := readNumber(t, js, "SCAN_BUDGET_MAX_MS"); maxMs != jsMaxMs {
		t.Errorf("scan budget ceiling = %d ms in the Claude Code twin, %d ms in the opencode twin; they must agree", maxMs, jsMaxMs)
	}
	if budgetMs > maxMs {
		t.Errorf("default scan budget %d ms is above its own %d ms ceiling", budgetMs, maxMs)
	}

	guarded := 0
	for _, h := range registry {
		if !h.Targets[TargetOpencode].FailClosed {
			continue
		}
		guarded++
		cc := h.Targets[TargetClaudeCode]
		if cc.Timeout*1000 <= maxMs {
			t.Errorf("%s: claude_code.timeout = %ds, want more than the %d ms scan budget ceiling", h.Name, cc.Timeout, maxMs)
		}
	}
	if guarded != 3 {
		t.Errorf("found %d fail-closed guards, want 3", guarded)
	}
}

func TestInstallClaude_Timeout(t *testing.T) {
	tests := map[string]struct {
		setup func(t *testing.T, repoDir, settingsPath string) Registry
		want  func(repoDir string) []hookCmd
	}{
		"writes the registry timeout on a new registration": {
			setup: func(t *testing.T, repoDir, settingsPath string) Registry {
				createScript(t, repoDir, "hooks/claude-code/foo.sh")
				return Registry{ccHookTimeout("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh", 45)}
			},
			want: func(repoDir string) []hookCmd {
				return []hookCmd{{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/foo.sh"), Timeout: 45}}
			},
		},
		// The case that matters for every machine devexp is already installed
		// on: the registration is there, so registerHook used to stop at it and
		// the guard kept Claude Code's 600-second default for ever.
		"adds the timeout to a registration that predates it": {
			setup: func(t *testing.T, repoDir, settingsPath string) Registry {
				scriptAbs := createScript(t, repoDir, "hooks/claude-code/foo.sh")
				writeSettingsHooks(t, settingsPath, hooksMapT{
					"PreToolUse": {{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: scriptAbs}}}},
				})
				return Registry{ccHookTimeout("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh", 45)}
			},
			want: func(repoDir string) []hookCmd {
				return []hookCmd{{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/foo.sh"), Timeout: 45}}
			},
		},
		"brings a timeout that drifted back to the registry's": {
			setup: func(t *testing.T, repoDir, settingsPath string) Registry {
				scriptAbs := createScript(t, repoDir, "hooks/claude-code/foo.sh")
				writeSettingsHooks(t, settingsPath, hooksMapT{
					"PreToolUse": {{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: scriptAbs, Timeout: 5}}}},
				})
				return Registry{ccHookTimeout("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh", 45)}
			},
			want: func(repoDir string) []hookCmd {
				return []hookCmd{{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/foo.sh"), Timeout: 45}}
			},
		},
		"carries the timeout when the matcher moves": {
			setup: func(t *testing.T, repoDir, settingsPath string) Registry {
				scriptAbs := createScript(t, repoDir, "hooks/claude-code/foo.sh")
				writeSettingsHooks(t, settingsPath, hooksMapT{
					"PreToolUse": {{Matcher: "Write", Hooks: []hookCmd{{Type: "command", Command: scriptAbs}}}},
				})
				return Registry{ccHookTimeout("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh", 45)}
			},
			want: func(repoDir string) []hookCmd {
				return []hookCmd{{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/foo.sh"), Timeout: 45}}
			},
		},
		// An advisory hook declares none, and a timeout on its registration is
		// the user's: devexp never takes away a field it didn't ask for.
		"leaves a user's timeout alone when the registry declares none": {
			setup: func(t *testing.T, repoDir, settingsPath string) Registry {
				scriptAbs := createScript(t, repoDir, "hooks/claude-code/foo.sh")
				writeSettingsHooks(t, settingsPath, hooksMapT{
					"PreToolUse": {{Matcher: "Bash", Hooks: []hookCmd{{Type: "command", Command: scriptAbs, Timeout: 7}}}},
				})
				return Registry{ccHook("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh")}
			},
			want: func(repoDir string) []hookCmd {
				return []hookCmd{{Type: "command", Command: filepath.Join(repoDir, "hooks/claude-code/foo.sh"), Timeout: 7}}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			repoDir := t.TempDir()
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			registry := tt.setup(t, repoDir, settingsPath)

			if err := InstallClaude(registry, repoDir, settingsPath, nil, false); err != nil {
				t.Fatalf("InstallClaude() error = %v", err)
			}
			got := readHooks(t, settingsPath)
			want := hooksMapT{"PreToolUse": {{Matcher: "Bash", Hooks: tt.want(repoDir)}}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("hooks = %+v, want %+v", got, want)
			}

			// Installing again must change nothing: the timeout is converged,
			// so the second run has no reason to rewrite settings.json.
			before, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("ReadFile error = %v", err)
			}
			if err := InstallClaude(registry, repoDir, settingsPath, nil, false); err != nil {
				t.Fatalf("second InstallClaude() error = %v", err)
			}
			after, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("ReadFile error = %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("a second install rewrote settings.json:\n%s\nwant:\n%s", after, before)
			}
		})
	}
}

// TestInstallClaude_TimeoutKeepsOtherFields checks the timeout goes in through
// the same machinery that preserves a handler's other members (#137): the
// user's own keys, and their order, survive.
func TestInstallClaude_TimeoutKeepsOtherFields(t *testing.T) {
	repoDir := t.TempDir()
	scriptAbs := createScript(t, repoDir, "hooks/claude-code/foo.sh")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settings := `{
  "model": "opus",
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {"type": "command", "command": ` + strconv.Quote(scriptAbs) + `, "statusMessage": "scanning", "async": false}
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(settings), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	registry := Registry{ccHookTimeout("foo", true, "PreToolUse", "Bash", "hooks/claude-code/foo.sh", 45)}
	if err := InstallClaude(registry, repoDir, settingsPath, nil, false); err != nil {
		t.Fatalf("InstallClaude() error = %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	out := string(data)
	for _, want := range []string{`"model": "opus"`, `"statusMessage": "scanning"`, `"async": false`, `"timeout": 45`} {
		if !strings.Contains(out, want) {
			t.Errorf("settings.json is missing %s:\n%s", want, out)
		}
	}
}
