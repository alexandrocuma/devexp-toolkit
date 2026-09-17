package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installSettings writes content (with REPO and FOREIGN replaced) to a fresh
// settings.json, runs InstallClaude with testRegistry, and returns the file's
// bytes and the output.
func installSettings(t *testing.T, repoDir, settingsPath string, dryRun bool) (string, string, error) {
	t.Helper()
	var err error
	out := captureOutput(t, func() { err = InstallClaude(testRegistry(), repoDir, settingsPath, nil, dryRun) })
	data, readErr := os.ReadFile(settingsPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return string(data), out, err
}

func testRepo(t *testing.T, dir string) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), dir)
	for _, s := range []string{"secret-guard.sh", "dangerous-cmd-guard.sh", "graphify-read-guard.sh"} {
		createScript(t, repoDir, "hooks/claude-code/"+s)
	}
	return repoDir
}

// userSettings is a settings.json as Claude Code and a user leave it: top-level
// keys devexp never reads, written inline, and hooks carrying every kind of
// field Claude Code documents plus one it doesn't (yet).
const userSettings = `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {"allow": ["Bash(git diff:*)"], "deny": []},
  "env": {"GREETING": "caf\u00e9 & <tea>", "RAW": "café"},
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "cd /tmp && make lint > /dev/null",
            "timeout": 30,
            "async": true,
            "asyncRewake": false,
            "shell": "bash",
            "if": "Bash(git *)",
            "statusMessage": "Linting\u2026",
            "x-future": {
              "nested": [
                1,
                2.50,
                null
              ]
            }
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "REPO/hooks/claude-code/secret-guard.sh",
            "args": [
              "--strict"
            ],
            "timeout": 5
          }
        ]
      },
      {
        "matcher": "Read|Bash",
        "x-note": "mine",
        "hooks": [
          {
            "type": "command",
            "command": "FOREIGN/hooks/claude-code/secret-guard.sh"
          },
          {
            "command": "/usr/local/bin/audit-log",
            "type": "command",
            "timeout": 10
          }
        ]
      },
      {
        "matcher": "Write",
        "hooks": []
      },
      {
        "matcher": "Edit",
        "hooks": [
          {
            "type": "command",
            "command": "REPO/hooks/claude-code/removed.sh"
          }
        ]
      }
    ],
    "Stop": [],
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "http",
            "url": "http://localhost:8080/hooks",
            "headers": {
              "X-Trace": "on"
            },
            "allowedEnvVars": [
              "TRACE_ID"
            ],
            "timeout": 30
          },
          {
            "type": "prompt",
            "prompt": "Check $ARGUMENTS",
            "model": "claude-haiku"
          },
          {
            "type": "mcp_tool",
            "server": "audit",
            "tool": "log",
            "input": {
              "path": "${tool_input.file_path}"
            }
          }
        ]
      }
    ]
  },
  "statusLine": {"type": "command", "command": "~/.claude/statusline.sh"},
  "cleanupPeriodDays": 30.0
}
`

// userSettingsInstalled is userSettings after one install: the foreign-root
// copy and the stale removed.sh entry are gone, secret-guard and
// dangerous-cmd-guard are appended exactly as earlier releases wrote them, and
// every other byte is as it was. The exec-form secret-guard (args) is the
// user's, so it neither counts as registered nor is touched.
const userSettingsInstalled = `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {"allow": ["Bash(git diff:*)"], "deny": []},
  "env": {"GREETING": "caf\u00e9 & <tea>", "RAW": "café"},
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "cd /tmp && make lint > /dev/null",
            "timeout": 30,
            "async": true,
            "asyncRewake": false,
            "shell": "bash",
            "if": "Bash(git *)",
            "statusMessage": "Linting\u2026",
            "x-future": {
              "nested": [
                1,
                2.50,
                null
              ]
            }
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "REPO/hooks/claude-code/secret-guard.sh",
            "args": [
              "--strict"
            ],
            "timeout": 5
          }
        ]
      },
      {
        "matcher": "Read|Bash",
        "x-note": "mine",
        "hooks": [
          {
            "command": "/usr/local/bin/audit-log",
            "type": "command",
            "timeout": 10
          }
        ]
      },
      {
        "matcher": "Write",
        "hooks": []
      },
      {
        "matcher": "Read|Bash",
        "hooks": [
          {
            "type": "command",
            "command": "REPO/hooks/claude-code/secret-guard.sh"
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "REPO/hooks/claude-code/dangerous-cmd-guard.sh"
          }
        ]
      }
    ],
    "Stop": [],
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "http",
            "url": "http://localhost:8080/hooks",
            "headers": {
              "X-Trace": "on"
            },
            "allowedEnvVars": [
              "TRACE_ID"
            ],
            "timeout": 30
          },
          {
            "type": "prompt",
            "prompt": "Check $ARGUMENTS",
            "model": "claude-haiku"
          },
          {
            "type": "mcp_tool",
            "server": "audit",
            "tool": "log",
            "input": {
              "path": "${tool_input.file_path}"
            }
          }
        ]
      }
    ]
  },
  "statusLine": {"type": "command", "command": "~/.claude/statusline.sh"},
  "cleanupPeriodDays": 30.0
}
`

func fill(s, repoDir, foreign string) string {
	return strings.NewReplacer("REPO", repoDir, "FOREIGN", foreign).Replace(s)
}

// TestInstallClaude_KeepsUserHookFields (#137): install rewrites only what it
// owns. Every field of a user's hook — timeout, async, shell, if,
// statusMessage, args, an http, prompt or mcp_tool handler's own fields, an
// unknown field — survives install and re-install byte for byte, as do an
// omitted matcher, an empty entry and event, and everything outside hooks.
func TestInstallClaude_KeepsUserHookFields(t *testing.T) {
	repoDir := testRepo(t, "repo")
	foreign := filepath.Join(t.TempDir(), "cache")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte(fill(userSettings, repoDir, foreign)), 0o644); err != nil {
		t.Fatal(err)
	}

	got, out, err := installSettings(t, repoDir, settingsPath, false)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if want := fill(userSettingsInstalled, repoDir, foreign); got != want {
		t.Errorf("install wrote:\n%s\nwant:\n%s", got, want)
	}

	again, out, err := installSettings(t, repoDir, settingsPath, false)
	if err != nil {
		t.Fatalf("re-install: %v\n%s", err, out)
	}
	if again != got {
		t.Errorf("re-install changed settings.json:\n%s\nwant:\n%s", again, got)
	}
	if strings.Contains(out, "Saved:") {
		t.Errorf("re-install wrote settings.json with nothing to change:\n%s", out)
	}
}

// TestInstallClaude_RequoteKeepsHandlerFields: re-quoting a devexp command
// changes that command only — the handler keeps its other fields and their
// order. An exec-form handler (args) naming the same kind of path is the
// user's: never re-quoted, and not counted as the registration.
func TestInstallClaude_RequoteKeepsHandlerFields(t *testing.T) {
	repoDir := testRepo(t, "My Proj")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	in := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Read|Bash",
        "hooks": [
          {
            "timeout": 7,
            "type": "command",
            "command": "REPO/hooks/claude-code/secret-guard.sh",
            "statusMessage": "guarding"
          },
          {
            "type": "command",
            "command": "REPO/hooks/claude-code/dangerous-cmd-guard.sh",
            "args": []
          }
        ]
      }
    ]
  }
}`
	want := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Read|Bash",
        "hooks": [
          {
            "timeout": 7,
            "type": "command",
            "command": "'REPO/hooks/claude-code/secret-guard.sh'",
            "statusMessage": "guarding"
          },
          {
            "type": "command",
            "command": "REPO/hooks/claude-code/dangerous-cmd-guard.sh",
            "args": []
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "'REPO/hooks/claude-code/dangerous-cmd-guard.sh'"
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(fill(in, repoDir, "")), 0o644); err != nil {
		t.Fatal(err)
	}
	got, out, err := installSettings(t, repoDir, settingsPath, false)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if want := fill(want, repoDir, ""); got != want {
		t.Errorf("install wrote:\n%s\nwant:\n%s", got, want)
	}
}

// TestInstallClaude_SettingsLayout: the hooks value is written in the layout
// of the file around it, and everything outside it keeps its bytes.
func TestInstallClaude_SettingsLayout(t *testing.T) {
	registry := Registry{ccHook("secret-guard", true, "PreToolUse", "Read", "hooks/claude-code/secret-guard.sh")}
	const hooks2 = "{\n    \"PreToolUse\": [\n      {\n        \"matcher\": \"Read\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"CMD\"\n          }\n        ]\n      }\n    ]\n  }"
	tests := map[string]struct{ in, want string }{
		// Byte for byte what v0.9.0 wrote (json.MarshalIndent) for a new file.
		"no settings.json": {
			in:   "",
			want: "{\n  \"hooks\": " + hooks2 + "\n}",
		},
		"empty object": {
			in:   "{}\n",
			want: "{\n  \"hooks\": " + hooks2 + "\n}\n",
		},
		"indented, no hooks key": {
			in:   "{\n  \"model\": \"opus\",\n  \"env\": {\"A\": \"<&>\"}\n}\n",
			want: "{\n  \"model\": \"opus\",\n  \"env\": {\"A\": \"<&>\"},\n  \"hooks\": " + hooks2 + "\n}\n",
		},
		"compact, no hooks key": {
			in:   `{"model":"opus"}`,
			want: `{"model":"opus","hooks":{"PreToolUse":[{"matcher":"Read","hooks":[{"type":"command","command":"CMD"}]}]}}`,
		},
		"compact, hooks key": {
			in:   `{"hooks":{},"model":"opus"}`,
			want: `{"hooks":{"PreToolUse":[{"matcher":"Read","hooks":[{"type":"command","command":"CMD"}]}]},"model":"opus"}`,
		},
		"hooks null, tab-indented": {
			in:   "{\n\t\"hooks\": null,\n\t\"model\": \"opus\"\n}",
			want: "{\n\t\"hooks\": {\n\t\t\"PreToolUse\": [\n\t\t\t{\n\t\t\t\t\"matcher\": \"Read\",\n\t\t\t\t\"hooks\": [\n\t\t\t\t\t{\n\t\t\t\t\t\t\"type\": \"command\",\n\t\t\t\t\t\t\"command\": \"CMD\"\n\t\t\t\t\t}\n\t\t\t\t]\n\t\t\t}\n\t\t]\n\t},\n\t\"model\": \"opus\"\n}",
		},
		"four-space indented, hooks key": {
			in:   "{\n    \"hooks\": {\"Stop\": []}\n}",
			want: "{\n    \"hooks\": {\n        \"Stop\": [],\n        \"PreToolUse\": [\n            {\n                \"matcher\": \"Read\",\n                \"hooks\": [\n                    {\n                        \"type\": \"command\",\n                        \"command\": \"CMD\"\n                    }\n                ]\n            }\n        ]\n    }\n}",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			repoDir := filepath.Join(t.TempDir(), "repo")
			cmd := createScript(t, repoDir, "hooks/claude-code/secret-guard.sh")
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			if tt.in != "" {
				if err := os.WriteFile(settingsPath, []byte(tt.in), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			out := captureOutput(t, func() { err = InstallClaude(registry, repoDir, settingsPath, nil, false) })
			if err != nil {
				t.Fatalf("InstallClaude: %v\n%s", err, out)
			}
			got, _ := os.ReadFile(settingsPath)
			if want := strings.ReplaceAll(tt.want, "CMD", cmd); string(got) != want {
				t.Errorf("settings.json =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// TestInstallClaude_RefusesUneditableSettings: a settings.json devexp can't
// edit without losing something is left byte for byte as it was, dry run or
// not, and the install fails saying which file.
func TestInstallClaude_RefusesUneditableSettings(t *testing.T) {
	tests := map[string]string{
		"invalid JSON":             `{"model": "opus",}`,
		"trailing content":         `{} {}`,
		"top level not an object":  `[]`,
		"hooks not an object":      `{"hooks": []}`,
		"event not an array":       `{"hooks": {"PreToolUse": {}}}`,
		"entry not an object":      `{"hooks": {"PreToolUse": ["x"]}}`,
		"entry hooks not an array": `{"hooks": {"PreToolUse": [{"hooks": {}}]}}`,
		"handler not an object":    `{"hooks": {"PreToolUse": [{"hooks": [null]}]}}`,
	}
	for name, content := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(name, func(t *testing.T) {
				repoDir := testRepo(t, "repo")
				settingsPath := filepath.Join(t.TempDir(), "settings.json")
				if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				got, out, err := installSettings(t, repoDir, settingsPath, dryRun)
				if err == nil || !strings.Contains(err.Error(), settingsPath) || !strings.Contains(err.Error(), "left untouched") {
					t.Errorf("dryRun=%v: error = %v, want a refusal naming %s\n%s", dryRun, err, settingsPath, out)
				}
				if got != content {
					t.Errorf("dryRun=%v: settings.json changed to:\n%s", dryRun, got)
				}
			})
		}
	}
}

// TestHookCmdMarshal: a handler devexp builds is written as earlier releases
// wrote it; one read from settings.json keeps its members, and a changed
// command replaces only that member's value.
func TestHookCmdMarshal(t *testing.T) {
	built, err := encodeJSON(hookEntry{Matcher: "Read", Hooks: []hookCmd{{Type: "command", Command: "/a&b"}}})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"matcher":"Read","hooks":[{"type":"command","command":"/a&b"}]}`; string(built) != want {
		t.Errorf("built entry = %s, want %s", built, want)
	}

	var h hookCmd
	if err := h.UnmarshalJSON([]byte(`{"x":1,"command":"/first","type":"command","command":"\u002fold","args":[]}`)); err != nil {
		t.Fatal(err)
	}
	if h.Command != "/old" || h.devexpForm() {
		t.Errorf("read handler = %+v (devexpForm %v), want command /old, not devexp form", h, h.devexpForm())
	}
	same, _ := h.MarshalJSON()
	if want := `{"x":1,"command":"/first","type":"command","command":"\u002fold","args":[]}`; string(same) != want {
		t.Errorf("unchanged handler = %s, want %s", same, want)
	}
	h.Command = "'/new'"
	changed, _ := h.MarshalJSON()
	if want := `{"x":1,"command":"'/new'","type":"command","command":"'/new'","args":[]}`; string(changed) != want {
		t.Errorf("changed handler = %s, want %s", changed, want)
	}
}

// TestInstallClaude_OnlyDevexpFormHandlers: devexp registers only
// {"type": "command", "command": <path>}. A handler with args (spawned without
// a shell) or of another type is the user's even when its command names a
// devexp script path: never pruned as stale, foreign or relative, and never
// re-quoted.
func TestInstallClaude_OnlyDevexpFormHandlers(t *testing.T) {
	repoDir := testRepo(t, "My Proj")
	foreign := filepath.Join(t.TempDir(), "cache")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	in := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "FOREIGN/hooks/claude-code/secret-guard.sh",
            "args": []
          },
          {
            "type": "command",
            "command": "REPO/hooks/claude-code/removed.sh",
            "args": [
              "x"
            ]
          },
          {
            "type": "agent",
            "command": "FOREIGN/hooks/claude-code/secret-guard.sh",
            "prompt": "p"
          },
          {
            "command": "REPO/hooks/claude-code/removed.sh"
          },
          {
            "type": "Command",
            "command": "hooks/claude-code/secret-guard.sh"
          },
          {
            "type": "prompt",
            "command": "REPO/hooks/claude-code/secret-guard.sh",
            "prompt": "p"
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(fill(in, repoDir, foreign)), 0o644); err != nil {
		t.Fatal(err)
	}
	got, out, err := installSettings(t, repoDir, settingsPath, false)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	stop := strings.Index(got, `"Stop": [`)
	end := strings.Index(got, "\n    ]")
	if stop < 0 || end < 0 || got[stop:end] != fill(in, repoDir, foreign)[stop:end] {
		t.Errorf("the Stop handlers changed:\n%s\nwant them as in:\n%s", got, fill(in, repoDir, foreign))
	}
}

// TestInstallClaude_NewEventsSorted: events a new settings.json gains are
// written sorted, as v0.9.0's map encoding wrote them.
func TestInstallClaude_NewEventsSorted(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	pre := createScript(t, repoDir, "hooks/claude-code/a.sh")
	post := createScript(t, repoDir, "hooks/claude-code/b.sh")
	stop := createScript(t, repoDir, "hooks/claude-code/c.sh")
	registry := Registry{
		ccHook("a", true, "PreToolUse", "Read", "hooks/claude-code/a.sh"),
		ccHook("c", true, "Stop", "", "hooks/claude-code/c.sh"),
		ccHook("b", true, "PostToolUse", "Write", "hooks/claude-code/b.sh"),
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	var err error
	out := captureOutput(t, func() { err = InstallClaude(registry, repoDir, settingsPath, nil, false) })
	if err != nil {
		t.Fatalf("InstallClaude: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(settingsPath)
	i, j, k := strings.Index(string(got), post), strings.Index(string(got), pre), strings.Index(string(got), stop)
	if !(i >= 0 && i < j && j < k) {
		t.Errorf("events not in sorted order (PostToolUse, PreToolUse, Stop):\n%s", got)
	}
}
