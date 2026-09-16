package hooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
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
