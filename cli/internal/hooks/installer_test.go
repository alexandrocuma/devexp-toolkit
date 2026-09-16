package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isForeignDevexpHook(tt.cmd, managed, repoDir); got != tt.want {
				t.Errorf("isForeignDevexpHook(%q) = %v, want %v", tt.cmd, got, tt.want)
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
