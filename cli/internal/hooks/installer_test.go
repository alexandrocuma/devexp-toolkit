package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
				return Registry{{
					Name: "foo", Enabled: true,
					ClaudeCode: HookCC{Event: "PreToolUse", Matcher: "Bash", Script: "hooks/claude-code/foo.sh"},
				}}, nil
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
				return Registry{{
					Name: "foo", Enabled: true,
					ClaudeCode: HookCC{Event: "PreToolUse", Matcher: "Bash", Script: "hooks/claude-code/foo.sh"},
				}}, nil
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
				return Registry{{
					Name: "foo", Enabled: true,
					ClaudeCode: HookCC{Event: "PreToolUse", Matcher: "Bash", Script: "hooks/claude-code/foo.sh"},
				}}, []string{"foo"}
			},
			check: func(t *testing.T, repoDir, settingsPath string) {
				if got := readHooks(t, settingsPath); got != nil {
					t.Errorf("hooks = %+v, want settings.json to not exist", got)
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
				return Registry{{
					Name: "foo", Enabled: true,
					ClaudeCode: HookCC{Event: "PreToolUse", Matcher: "Bash", Script: "hooks/claude-code/foo.sh"},
				}}, nil
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
		{Name: "secret-guard", Enabled: true, ClaudeCode: HookCC{
			Event: "PreToolUse", Matcher: "Read|Bash", Script: "hooks/claude-code/secret-guard.sh"}},
		{Name: "dangerous-cmd-guard", Enabled: true, ClaudeCode: HookCC{
			Event: "PreToolUse", Matcher: "Bash", Script: "hooks/claude-code/dangerous-cmd-guard.sh"}},
		// Disabled on purpose: a foreign copy of a disabled hook still runs,
		// so it must still be pruned.
		{Name: "graphify-read-guard", Enabled: false, ClaudeCode: HookCC{
			Event: "PreToolUse", Matcher: "Read|Glob", Script: "hooks/claude-code/graphify-read-guard.sh"}},
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
