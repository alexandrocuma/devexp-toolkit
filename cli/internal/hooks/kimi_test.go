package hooks

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// TestSelectKimi covers the per-target enabled logic on synthetic registries:
// a block's own override beats the hook's top-level flag, a hook with no kimi
// block is absent rather than silently on, and every hook that is not selected
// leaves a reason behind.
func TestSelectKimi(t *testing.T) {
	guard := TargetSpec{
		Event:      "PreToolUse",
		Matcher:    "^Bash$",
		Script:     "hooks/claude-code/dangerous-cmd-guard.sh",
		FailClosed: true,
		Timeout:    45,
	}

	tests := map[string]struct {
		registry    Registry
		disabled    []string
		want        []KimiHook
		wantSkipped map[string]string
	}{
		"an enabled block is selected": {
			registry: Registry{{Name: "dangerous-cmd-guard", Enabled: true, Targets: map[string]TargetSpec{TargetKimi: guard}}},
			want: []KimiHook{{
				Name: "dangerous-cmd-guard", Event: "PreToolUse", Matcher: "^Bash$",
				Script: "hooks/claude-code/dangerous-cmd-guard.sh", FailClosed: true, Timeout: 45,
			}},
			wantSkipped: map[string]string{},
		},
		"no kimi block means absent, not enabled": {
			registry: Registry{{Name: "lonely", Enabled: true, Targets: map[string]TargetSpec{
				TargetClaudeCode: {Event: "PreToolUse", Script: "hooks/claude-code/secret-guard.sh"},
			}}},
			wantSkipped: map[string]string{"lonely": "no Kimi mapping in hooks/registry.json"},
		},
		"a block with no script cannot be installed": {
			registry:    Registry{{Name: "scriptless", Enabled: true, Targets: map[string]TargetSpec{TargetKimi: {Event: "PreToolUse"}}}},
			wantSkipped: map[string]string{"scriptless": "no Kimi mapping in hooks/registry.json"},
		},
		"a block with no event cannot be installed": {
			registry:    Registry{{Name: "eventless", Enabled: true, Targets: map[string]TargetSpec{TargetKimi: {Script: "hooks/claude-code/secret-guard.sh"}}}},
			wantSkipped: map[string]string{"eventless": "no Kimi mapping in hooks/registry.json"},
		},
		// The large-file-guard / on-save shape: on for the other targets, off
		// here, and the run says why rather than quietly omitting it.
		"enabled:false reports the block's own reason": {
			registry: Registry{{Name: "large-file-guard", Enabled: true, Targets: map[string]TargetSpec{
				TargetKimi: {Event: "PreToolUse", Script: "hooks/claude-code/large-file-guard.sh", Enabled: boolPtr(false), Reason: "Kimi runs an ask as an allow"},
			}}},
			wantSkipped: map[string]string{"large-file-guard": "Kimi runs an ask as an allow"},
		},
		"enabled:false with no reason still reports something": {
			registry: Registry{{Name: "mute", Enabled: true, Targets: map[string]TargetSpec{
				TargetKimi: {Event: "PreToolUse", Script: "hooks/claude-code/large-file-guard.sh", Enabled: boolPtr(false)},
			}}},
			wantSkipped: map[string]string{"mute": "off for Kimi"},
		},
		// The graphify shape: ships off, but a block may turn it back on for
		// one target. Kimi's blocks never do; the logic still has to.
		"a block override beats the top-level flag": {
			registry: Registry{
				{Name: "off-everywhere", Enabled: false, Targets: map[string]TargetSpec{
					TargetKimi: {Event: "PreToolUse", Script: "hooks/claude-code/graphify-read-guard.sh"},
				}},
				{Name: "on-for-kimi", Enabled: false, Targets: map[string]TargetSpec{
					TargetKimi: {Event: "PreToolUse", Matcher: "^Read$", Script: "hooks/claude-code/graphify-read-guard.sh", Enabled: boolPtr(true)},
				}},
			},
			want: []KimiHook{{
				Name: "on-for-kimi", Event: "PreToolUse", Matcher: "^Read$",
				Script: "hooks/claude-code/graphify-read-guard.sh", Timeout: kimiDefaultTimeout,
			}},
			wantSkipped: map[string]string{"off-everywhere": "off for Kimi"},
		},
		"the user's --disable wins over an enabled block": {
			registry:    Registry{{Name: "dangerous-cmd-guard", Enabled: true, Targets: map[string]TargetSpec{TargetKimi: guard}}},
			disabled:    []string{"dangerous-cmd-guard"},
			wantSkipped: map[string]string{"dangerous-cmd-guard": "disabled"},
		},
		// A block that omits the timeout must not be registered without one:
		// Kimi's own default is 30 s, under the guards' 44 s scan budget
		// ceiling, and a hook Kimi kills is read as an allow.
		"an omitted timeout becomes the default": {
			registry: Registry{{Name: "budgeted", Enabled: true, Targets: map[string]TargetSpec{
				TargetKimi: {Event: "PreToolUse", Matcher: "^Bash$", Script: "hooks/claude-code/secret-guard.sh", FailClosed: true},
			}}},
			want: []KimiHook{{
				Name: "budgeted", Event: "PreToolUse", Matcher: "^Bash$",
				Script: "hooks/claude-code/secret-guard.sh", FailClosed: true, Timeout: kimiDefaultTimeout,
			}},
			wantSkipped: map[string]string{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			selected, skipped := SelectKimi(tt.registry, tt.disabled)
			if !reflect.DeepEqual(selected, tt.want) {
				t.Errorf("SelectKimi() selected = %+v, want %+v", selected, tt.want)
			}
			if !reflect.DeepEqual(skipped, tt.wantSkipped) {
				t.Errorf("SelectKimi() skipped = %v, want %v", skipped, tt.wantSkipped)
			}
		})
	}
}

// TestSelectKimi_RepoRegistry pins what the real registry actually sends to
// Kimi: the three fail-closed guards and nothing else. Every hook is accounted
// for either way, so a hook added later cannot reach Kimi — or fail to — by
// accident.
func TestSelectKimi_RepoRegistry(t *testing.T) {
	registry, err := LoadRegistry(filepath.Join("..", "..", "..", "hooks", "registry.json"))
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}

	selected, skipped := SelectKimi(registry, nil)

	var got []string
	for _, h := range selected {
		got = append(got, h.Name)
		if h.Event != "PreToolUse" {
			t.Errorf("%s: event = %q; only PreToolUse reaches the agent under Kimi", h.Name, h.Event)
		}
		if !h.FailClosed {
			t.Errorf("%s: selected for Kimi but not fail-closed", h.Name)
		}
	}
	want := []string{"secret-guard", "dangerous-cmd-guard", "secret-in-write-guard"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SelectKimi() selected = %v, want %v (registry order)", got, want)
	}

	// The two the ticket names explicitly, plus everything else that is off.
	for _, name := range []string{
		"format-on-save", "graphify-grep-nudge",
		"large-file-guard", "lint-on-save", "test-on-save",
		"graphify-read-guard", "graphify-session-sentinel",
	} {
		reason, ok := skipped[name]
		if !ok {
			t.Errorf("%s: not skipped for Kimi, but it must not be installed there", name)
			continue
		}
		if reason == "" || reason == "off for Kimi" {
			t.Errorf("%s: skipped with %q; the registry block owes a reason the installer can print", name, reason)
		}
	}

	if len(selected)+len(skipped) != len(registry) {
		names := make([]string, 0, len(skipped))
		for n := range skipped {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Errorf("SelectKimi() accounted for %d selected + %d skipped (%v), want all %d hooks",
			len(selected), len(skipped), names, len(registry))
	}
}
