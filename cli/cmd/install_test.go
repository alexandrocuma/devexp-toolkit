package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"devexp/internal/config"
	"devexp/internal/hooks"
	"devexp/internal/mcp"
	"devexp/internal/repo"
)

func TestFilterMCPs(t *testing.T) {
	registry := []mcp.MCP{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}

	tests := map[string]struct {
		selected []string
		want     []string
	}{
		"nil selected returns full registry": {
			selected: nil,
			want:     []string{"alpha", "beta", "gamma"},
		},
		"selected filters to matching names": {
			selected: []string{"beta"},
			want:     []string{"beta"},
		},
		"selected with no matches returns empty": {
			selected: []string{"nonexistent"},
			want:     nil,
		},
		"empty selected slice returns empty": {
			selected: []string{},
			want:     nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := filterMCPs(registry, tt.selected)
			var gotNames []string
			for _, m := range got {
				gotNames = append(gotNames, m.Name)
			}
			if !reflect.DeepEqual(gotNames, tt.want) {
				t.Errorf("filterMCPs() = %v, want %v", gotNames, tt.want)
			}
		})
	}
}

func TestBuildEnv(t *testing.T) {
	tests := map[string]struct {
		osEnv   map[string]string
		dotenv  map[string]string
		repoDir string
		want    map[string]string
	}{
		"sets DEVEXP_DIR to repoDir": {
			dotenv:  map[string]string{},
			repoDir: "/some/repo",
			want:    map[string]string{"DEVEXP_DIR": "/some/repo"},
		},
		"dotenv adds new keys": {
			dotenv:  map[string]string{"ONLY_IN_DOTENV": "dotenv-value"},
			repoDir: "/repo",
			want:    map[string]string{"ONLY_IN_DOTENV": "dotenv-value", "DEVEXP_DIR": "/repo"},
		},
		"dotenv overrides OS env": {
			osEnv:   map[string]string{"DEVEXP_TEST_VAR": "from-os"},
			dotenv:  map[string]string{"DEVEXP_TEST_VAR": "from-dotenv"},
			repoDir: "/repo",
			want:    map[string]string{"DEVEXP_TEST_VAR": "from-dotenv"},
		},
		"preserves OS env not in dotenv": {
			osEnv:   map[string]string{"DEVEXP_TEST_VAR": "from-os"},
			dotenv:  map[string]string{},
			repoDir: "/repo",
			want:    map[string]string{"DEVEXP_TEST_VAR": "from-os"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for k, v := range tt.osEnv {
				t.Setenv(k, v)
			}

			got := buildEnv(tt.dotenv, tt.repoDir)

			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("buildEnv()[%q] = %q, want %q", k, got[k], want)
				}
			}
		})
	}
}

func TestCommandExists(t *testing.T) {
	tests := map[string]struct {
		name string
		want bool
	}{
		"existing command": {
			name: "go",
			want: true,
		},
		"nonexistent command": {
			name: "devexp-definitely-not-a-real-command-xyz",
			want: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := commandExists(tt.name); got != tt.want {
				t.Errorf("commandExists(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestListAgentNames(t *testing.T) {
	tests := map[string]struct {
		files []string
		dirs  []string
		want  []string
	}{
		"returns sorted .md names without extension": {
			files: []string{"zeta.md", "alpha.md", "beta.md"},
			want:  []string{"alpha", "beta", "zeta"},
		},
		"excludes README": {
			files: []string{"alpha.md", "README.md"},
			want:  []string{"alpha"},
		},
		"ignores non-md files": {
			files: []string{"alpha.md", "notes.txt"},
			want:  []string{"alpha"},
		},
		"directories are skipped": {
			files: []string{"alpha.md"},
			dirs:  []string{"opencode"},
			want:  []string{"alpha"},
		},
		"empty agents dir returns nil": {
			files: nil,
			want:  nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			repoDir := t.TempDir()
			agentsDir := filepath.Join(repoDir, "agents")
			if err := os.MkdirAll(agentsDir, 0755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			for _, f := range tt.files {
				if err := os.WriteFile(filepath.Join(agentsDir, f), []byte("content"), 0644); err != nil {
					t.Fatalf("WriteFile(%s) error = %v", f, err)
				}
			}
			for _, d := range tt.dirs {
				if err := os.Mkdir(filepath.Join(agentsDir, d), 0755); err != nil {
					t.Fatalf("Mkdir(%s) error = %v", d, err)
				}
			}

			got := listAgentNames(repoDir)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("listAgentNames() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("missing agents dir returns nil", func(t *testing.T) {
		repoDir := t.TempDir()
		if got := listAgentNames(repoDir); got != nil {
			t.Errorf("listAgentNames() = %v, want nil", got)
		}
	})
}

func TestListHookNames(t *testing.T) {
	tests := map[string]struct {
		registry string
		want     []string
	}{
		"returns only enabled hooks": {
			registry: `[
				{"name": "alpha", "enabled": true},
				{"name": "beta", "enabled": false},
				{"name": "gamma", "enabled": true}
			]`,
			want: []string{"alpha", "gamma"},
		},
		"empty registry returns nil": {
			registry: `[]`,
			want:     nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			repoDir := t.TempDir()
			hooksDir := filepath.Join(repoDir, "hooks")
			if err := os.MkdirAll(hooksDir, 0755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			if err := os.WriteFile(filepath.Join(hooksDir, "registry.json"), []byte(tt.registry), 0644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			got := listHookNames(repoDir)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("listHookNames() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("missing registry returns nil", func(t *testing.T) {
		repoDir := t.TempDir()
		if got := listHookNames(repoDir); got != nil {
			t.Errorf("listHookNames() = %v, want nil", got)
		}
	})
}

func TestResolveHookDisabled(t *testing.T) {
	registry := hooks.Registry{
		{Name: "alpha", Enabled: true},
		{Name: "beta", Enabled: true},
		{Name: "gamma", Enabled: false},
	}

	tests := map[string]struct {
		selected    []string
		cfgDisabled []string
		want        []string
	}{
		"nil selected falls back to cfgDisabled": {
			selected:    nil,
			cfgDisabled: []string{"beta"},
			want:        []string{"beta"},
		},
		"selected disables enabled hooks not chosen": {
			selected:    []string{"alpha"},
			cfgDisabled: nil,
			want:        []string{"beta"},
		},
		"selected includes all enabled hooks disables none": {
			selected:    []string{"alpha", "beta"},
			cfgDisabled: nil,
			want:        nil,
		},
		"already-disabled hooks are not surfaced again": {
			selected:    []string{"alpha", "beta", "gamma"},
			cfgDisabled: nil,
			want:        nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := resolveHookDisabled(registry, tt.selected, tt.cfgDisabled)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveHookDisabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveAgentDisabled(t *testing.T) {
	tests := map[string]struct {
		agentFiles  []string
		selected    []string
		cfgDisabled []string
		want        []string
	}{
		"nil selected falls back to cfgDisabled": {
			agentFiles:  []string{"alpha.md", "beta.md"},
			selected:    nil,
			cfgDisabled: []string{"beta"},
			want:        []string{"beta"},
		},
		"selected disables all agents not chosen": {
			agentFiles:  []string{"alpha.md", "beta.md", "gamma.md"},
			selected:    []string{"alpha"},
			cfgDisabled: nil,
			want:        []string{"beta", "gamma"},
		},
		"selected includes all agents disables none": {
			agentFiles:  []string{"alpha.md", "beta.md"},
			selected:    []string{"alpha", "beta"},
			cfgDisabled: nil,
			want:        nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			repoDir := t.TempDir()
			agentsDir := filepath.Join(repoDir, "agents")
			if err := os.MkdirAll(agentsDir, 0755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			for _, f := range tt.agentFiles {
				if err := os.WriteFile(filepath.Join(agentsDir, f), []byte("content"), 0644); err != nil {
					t.Fatalf("WriteFile(%s) error = %v", f, err)
				}
			}

			got := resolveAgentDisabled(repoDir, tt.selected, tt.cfgDisabled)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveAgentDisabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBackupExisting(t *testing.T) {
	t.Run("copies matching files to backupDir", func(t *testing.T) {
		dir := t.TempDir()
		backupDir := filepath.Join(t.TempDir(), "backup")

		if err := os.WriteFile(filepath.Join(dir, "keep.md"), []byte("agent content"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("not matched"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		backupExisting(dir, "*.md", backupDir, false)

		got, err := os.ReadFile(filepath.Join(backupDir, "keep.md"))
		if err != nil {
			t.Fatalf("ReadFile(backup) error = %v", err)
		}
		if string(got) != "agent content" {
			t.Errorf("backed up content = %q, want %q", got, "agent content")
		}
		if _, err := os.Stat(filepath.Join(backupDir, "ignore.txt")); !os.IsNotExist(err) {
			t.Errorf("ignore.txt should not have been backed up, stat err = %v", err)
		}
	})

	t.Run("dry run does not create backupDir", func(t *testing.T) {
		dir := t.TempDir()
		backupDir := filepath.Join(t.TempDir(), "backup")

		if err := os.WriteFile(filepath.Join(dir, "keep.md"), []byte("agent content"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		backupExisting(dir, "*.md", backupDir, true)

		if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
			t.Errorf("backupDir should not exist in dry run, stat err = %v", err)
		}
	})

	t.Run("no matches does not create backupDir", func(t *testing.T) {
		dir := t.TempDir()
		backupDir := filepath.Join(t.TempDir(), "backup")

		backupExisting(dir, "*.md", backupDir, false)

		if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
			t.Errorf("backupDir should not exist when no files match, stat err = %v", err)
		}
	})
}

func TestBackupExistingDirs(t *testing.T) {
	t.Run("copies skill dirs containing SKILL.md to backupDir", func(t *testing.T) {
		dir := t.TempDir()
		backupDir := filepath.Join(t.TempDir(), "backup")

		alpha := filepath.Join(dir, "alpha")
		if err := os.MkdirAll(filepath.Join(alpha, "references"), 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(alpha, "SKILL.md"), []byte("alpha skill"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(alpha, "references", "notes.md"), []byte("notes"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		notASkill := filepath.Join(dir, "not-a-skill")
		if err := os.MkdirAll(notASkill, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(notASkill, "notes.txt"), []byte("no SKILL.md here"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		backupExistingDirs(dir, backupDir, false)

		got, err := os.ReadFile(filepath.Join(backupDir, "alpha", "SKILL.md"))
		if err != nil {
			t.Fatalf("ReadFile(backup alpha/SKILL.md) error = %v", err)
		}
		if string(got) != "alpha skill" {
			t.Errorf("backed up SKILL.md content = %q, want %q", got, "alpha skill")
		}
		if _, err := os.Stat(filepath.Join(backupDir, "alpha", "references", "notes.md")); err != nil {
			t.Errorf("nested reference file not backed up: %v", err)
		}
		if _, err := os.Stat(filepath.Join(backupDir, "not-a-skill")); !os.IsNotExist(err) {
			t.Errorf("not-a-skill should not have been backed up, stat err = %v", err)
		}
	})

	t.Run("dry run does not create backupDir", func(t *testing.T) {
		dir := t.TempDir()
		backupDir := filepath.Join(t.TempDir(), "backup")

		alpha := filepath.Join(dir, "alpha")
		if err := os.MkdirAll(alpha, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(alpha, "SKILL.md"), []byte("alpha skill"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		backupExistingDirs(dir, backupDir, true)

		if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
			t.Errorf("backupDir should not exist in dry run, stat err = %v", err)
		}
	})

	t.Run("no skill dirs does not create backupDir", func(t *testing.T) {
		dir := t.TempDir()
		backupDir := filepath.Join(t.TempDir(), "backup")

		backupExistingDirs(dir, backupDir, false)

		if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
			t.Errorf("backupDir should not exist when no skill dirs found, stat err = %v", err)
		}
	})
}

func TestRemoveStale(t *testing.T) {
	t.Run("dry run reports without removing", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "stale.md")
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		out := captureStdout(t, func() {
			removeStale(filepath.Dir(dir), dir, []string{"stale.md"}, nil, staleFile, (*os.Root).Remove, true)
		})

		if _, err := os.Stat(path); err != nil {
			t.Errorf("dry run should not remove %s, stat err = %v", path, err)
		}
		if want := fmt.Sprintf("remove %q (no longer in this release)", path); !strings.Contains(out, want) {
			t.Errorf("no preview %q:\n%s", want, out)
		}
	})

	t.Run("removes entries via removeFn", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "stale.md")
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		removeStale(filepath.Dir(dir), dir, []string{"stale.md"}, nil, staleFile, (*os.Root).Remove, false)

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, stat err = %v", path, err)
		}
	})

	t.Run("removes directories via os.RemoveAll", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "old-skill")
		if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("old"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		removeStale(filepath.Dir(dir), dir, []string{"old-skill"}, nil, staleDir, (*os.Root).RemoveAll, false)

		if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, stat err = %v", skillDir, err)
		}
	})

	t.Run("tolerates already-missing entries", func(t *testing.T) {
		dir := t.TempDir()

		out := captureStdout(t, func() {
			removeStale(filepath.Dir(dir), dir, []string{"already-gone.md"}, nil, staleFile, (*os.Root).Remove, false)
		})
		if out != "" {
			t.Errorf("an entry already gone needs nothing, printed:\n%s", out)
		}
	})
}

func TestIsInstalledName(t *testing.T) {
	tests := map[string]struct {
		name  string
		shape staleShape
		want  bool
	}{
		"an agent file":                             {name: "dev-agent.md", shape: staleFile, want: true},
		"a skill directory":                         {name: "graphify", shape: staleDir, want: true},
		"a file without .md":                        {name: "dev-agent", shape: staleFile},
		"a bare .md":                                {name: ".md", shape: staleFile},
		"an empty file name":                        {name: "", shape: staleFile},
		"an empty directory name":                   {name: "", shape: staleDir},
		"the target directory itself":               {name: ".", shape: staleDir},
		"the parent directory":                      {name: "..", shape: staleDir},
		"a parent-relative file":                    {name: "../precious.md", shape: staleFile},
		"a parent-relative directory":               {name: "../precious", shape: staleDir},
		"a dot-dot hidden in a file name":           {name: "..md", shape: staleFile},
		"a nested file":                             {name: "sub/agent.md", shape: staleFile},
		"a path that cleans back into the target":   {name: "sub/../agent.md", shape: staleFile},
		"a trailing separator":                      {name: "graphify/", shape: staleDir},
		"an absolute file path":                     {name: "/etc/agent.md", shape: staleFile},
		"an absolute directory path":                {name: "/etc", shape: staleDir},
		"an opencode command, recorded without .md": {name: "deliver", shape: staleCommand, want: true},
		"an opencode command recorded with .md":     {name: "deliver.md", shape: staleCommand, want: true},
		"an empty opencode command":                 {name: "", shape: staleCommand},
		"a parent-relative opencode command":        {name: "../precious", shape: staleCommand},
		"an opencode command that gains a dot-dot":  {name: "x.", shape: staleCommand},
		"an absolute opencode command":              {name: "/etc/precious", shape: staleCommand},
		"a backslash in an agent file":              {name: `sub\agent.md`, shape: staleFile},
		"a backslash in a skill directory":          {name: `sub\skill`, shape: staleDir},
		"a newline in an agent file":                {name: "evil\n  - everything.md", shape: staleFile},
		"an escape sequence in an agent file":       {name: "\x1b[2J\x1b[31mFAKE.md", shape: staleFile},
		"a NUL in an agent file":                    {name: "a\x00.md", shape: staleFile},
		"a carriage return in a skill directory":    {name: "skill\r", shape: staleDir},
		"an escape sequence in an opencode command": {name: "de\x1bliver", shape: staleCommand},
		"a backslash separator":                     {name: `..\agent.md`, shape: staleFile},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isInstalledName(tt.name, tt.shape); got != tt.want {
				t.Errorf("isInstalledName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestRemoveStale_TamperedEntries: a manifest entry that isn't a bare name
// devexp installs is never removed, in a real run or a dry run, and each one
// is warned about. Everything lives under root, so any removal outside the
// target directory (or of the directory itself) shows up in the snapshot.
func TestRemoveStale_TamperedEntries(t *testing.T) {
	type layout struct{ root, target string }
	tests := map[string]struct {
		shape    staleShape
		removeFn func(*os.Root, string) error
		entries  func(l layout) []string
	}{
		"agent files": {shape: staleFile, removeFn: (*os.Root).Remove, entries: func(l layout) []string {
			return []string{"../precious.md", filepath.Join(l.root, "precious.md"), "sub/../mine.md", "..", ".md", "mine",
				"evil\n  - everything.md", "\x1b[2J\x1b[31mFAKE.md"}
		}},
		"skill directories": {shape: staleDir, removeFn: (*os.Root).RemoveAll, entries: func(l layout) []string {
			return []string{"", ".", "..", "../precious", filepath.Join(l.root, "precious"), "mine/", "mine\n  - everything"}
		}},
		"opencode commands": {shape: staleCommand, removeFn: (*os.Root).Remove, entries: func(l layout) []string {
			return []string{"", ".", "../precious", filepath.Join(l.root, "precious"), "sub/../mine", "x.", "mine\x1b[2J"}
		}},
	}
	for name, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s dryRun=%v", name, dryRun), func(t *testing.T) {
				root := t.TempDir()
				l := layout{root: root, target: filepath.Join(root, "target")}
				for p, c := range map[string]string{
					filepath.Join(root, "precious.md"):          "keep\n",
					filepath.Join(root, "precious", "SKILL.md"): "keep\n",
					filepath.Join(l.target, "mine.md"):          "keep\n",
					filepath.Join(l.target, "mine", "SKILL.md"): "keep\n",
				} {
					os.MkdirAll(filepath.Dir(p), 0o755) //nolint:errcheck
					os.WriteFile(p, []byte(c), 0o644)   //nolint:errcheck
				}
				before := treeBytes(t, root)
				entries := tt.entries(l)

				out := captureStdout(t, func() { removeStale(filepath.Dir(l.target), l.target, entries, nil, tt.shape, tt.removeFn, dryRun) })

				if after := treeBytes(t, root); !reflect.DeepEqual(after, before) {
					t.Errorf("tree changed:\n got %v\nwant %v\n%s", after, before, out)
				}
				for _, e := range entries {
					if !strings.Contains(out, fmt.Sprintf("%q left untouched: listed in the manifest but not a name devexp installs in %q\n", e, l.target)) {
						t.Errorf("no warning naming %q:\n%s", e, out)
					}
				}
				if strings.Contains(out, "[dry-run]") {
					t.Errorf("a rejected entry was previewed as a removal:\n%s", out)
				}
				if strings.Contains(out, "\x1b[2J") || strings.Contains(out, "\n  - everything") {
					t.Errorf("a manifest entry reached the terminal raw:\n%q", out)
				}
			})
		}
	}
}

// TestRemoveStale_EntryShape: a valid name is removed only when the entry on
// disk has the shape devexp installs. A symlink is never removed (nor what it
// points at), as for opencode plugin files; a valid entry next to them still is.
func TestRemoveStale_EntryShape(t *testing.T) {
	tests := map[string]struct {
		shape    staleShape
		removeFn func(*os.Root, string) error
		setup    func(t *testing.T, root, target string) string // returns the kept entry name
		wantWarn string
	}{
		"a symlinked agent file": {shape: staleFile, removeFn: (*os.Root).Remove, wantWarn: "a symlink", setup: func(t *testing.T, root, target string) string {
			mustSymlink(t, filepath.Join(root, "outside.md"), filepath.Join(target, "linked.md"))
			return "linked.md"
		}},
		"a symlinked skill directory": {shape: staleDir, removeFn: (*os.Root).RemoveAll, wantWarn: "a symlink", setup: func(t *testing.T, root, target string) string {
			mustSymlink(t, filepath.Join(root, "outside"), filepath.Join(target, "linked"))
			return "linked"
		}},
		"a directory where an agent file was": {shape: staleFile, removeFn: (*os.Root).Remove, wantWarn: "a directory", setup: func(t *testing.T, root, target string) string {
			os.MkdirAll(filepath.Join(target, "empty.md"), 0o755) //nolint:errcheck
			return "empty.md"
		}},
		"a symlinked opencode command": {shape: staleCommand, removeFn: (*os.Root).Remove, wantWarn: "a symlink", setup: func(t *testing.T, root, target string) string {
			mustSymlink(t, filepath.Join(root, "outside.md"), filepath.Join(target, "linked.md"))
			return "linked"
		}},
		"a file where a skill directory was": {shape: staleDir, removeFn: (*os.Root).RemoveAll, wantWarn: "a file", setup: func(t *testing.T, root, target string) string {
			os.WriteFile(filepath.Join(target, "loose"), []byte("keep\n"), 0o644) //nolint:errcheck
			return "loose"
		}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "target")
			os.MkdirAll(filepath.Join(root, "outside"), 0o755)                                //nolint:errcheck
			os.WriteFile(filepath.Join(root, "outside", "SKILL.md"), []byte("keep\n"), 0o644) //nolint:errcheck
			os.WriteFile(filepath.Join(root, "outside.md"), []byte("keep\n"), 0o644)          //nolint:errcheck
			os.MkdirAll(target, 0o755)                                                        //nolint:errcheck
			kept := tt.setup(t, root, target)
			// onDisk is the path an entry names: opencode commands are
			// recorded without the .md their file has.
			onDisk := func(entry string) string {
				if tt.shape == staleCommand {
					entry += ".md"
				}
				return filepath.Join(target, entry)
			}
			valid := "old.md"
			switch tt.shape {
			case staleDir:
				valid = "old-skill"
				os.MkdirAll(onDisk(valid), 0o755) //nolint:errcheck
			case staleCommand:
				valid = "old-cmd"
				os.WriteFile(onDisk(valid), []byte("old\n"), 0o644) //nolint:errcheck
			default:
				os.WriteFile(onDisk(valid), []byte("old\n"), 0o644) //nolint:errcheck
			}

			out := captureStdout(t, func() {
				removeStale(filepath.Dir(target), target, []string{kept, valid}, nil, tt.shape, tt.removeFn, false)
			})

			if _, err := os.Lstat(onDisk(kept)); err != nil {
				t.Errorf("%s removed, want kept: %v\n%s", kept, err, out)
			}
			for _, p := range []string{filepath.Join(root, "outside.md"), filepath.Join(root, "outside", "SKILL.md")} {
				if got, _ := os.ReadFile(p); string(got) != "keep\n" {
					t.Errorf("%s = %q, want untouched", p, got)
				}
			}
			if !strings.Contains(out, fmt.Sprintf("%q left untouched", onDisk(kept))) || !strings.Contains(out, tt.wantWarn) {
				t.Errorf("no warning naming %s as %s:\n%s", kept, tt.wantWarn, out)
			}
			if _, err := os.Lstat(onDisk(valid)); !os.IsNotExist(err) {
				t.Errorf("valid stale entry %s not removed, stat err = %v", valid, err)
			}
			if want := fmt.Sprintf("-\x1b[0m %q\n", filepath.Base(onDisk(valid))); !strings.Contains(out, want) {
				t.Errorf("no removal line %q:\n%q", want, out)
			}
		})
	}
}

// TestRemoveStale_InstalledTwin: a stale entry that is one of this run's
// installed items under another name is never removed. On a case-insensitive
// filesystem (the macOS default) "DEV-AGENT.md" is the dev-agent.md just
// installed; the case check catches that on any filesystem and in a dry run.
// A hard link stands in for any other variant (Unicode normalization) the
// filesystem resolves to the same file, so os.SameFile is exercised on
// case-sensitive filesystems too.
func TestRemoveStale_InstalledTwin(t *testing.T) {
	tests := map[string]struct {
		shape            staleShape
		stale, installed string
		setup            func(t *testing.T, target string)
		wantWarn         string
	}{
		"a Claude Code agent differing in case": {shape: staleFile, stale: "DEV-AGENT.md", installed: "dev-agent.md",
			wantWarn: `"DEV-AGENT.md" left untouched: the same name as "dev-agent.md", installed by this run, apart from case`},
		"a Claude Code skill differing in case": {shape: staleDir, stale: "GRAPHIFY", installed: "graphify",
			wantWarn: `"GRAPHIFY" left untouched: the same name as "graphify", installed by this run, apart from case`},
		"an opencode agent differing in case": {shape: staleFile, stale: "Deliver-Agent.md", installed: "deliver-agent.md",
			wantWarn: `"Deliver-Agent.md" left untouched: the same name as "deliver-agent.md", installed by this run, apart from case`},
		"an opencode command differing in case": {shape: staleCommand, stale: "DELIVER", installed: "deliver",
			wantWarn: `"DELIVER" left untouched: the same name as "deliver", installed by this run, apart from case`},
		"an agent file that is the installed file": {shape: staleFile, stale: "old.md", installed: "new.md",
			setup: func(t *testing.T, target string) {
				if err := os.Link(filepath.Join(target, "new.md"), filepath.Join(target, "old.md")); err != nil {
					t.Fatalf("Link() error = %v", err)
				}
			},
			wantWarn: `old.md" left untouched: the same file as "new.md", installed by this run`},
		"an opencode command that is the installed file": {shape: staleCommand, stale: "old", installed: "new",
			setup: func(t *testing.T, target string) {
				if err := os.Link(filepath.Join(target, "new.md"), filepath.Join(target, "old.md")); err != nil {
					t.Fatalf("Link() error = %v", err)
				}
			},
			wantWarn: `old.md" left untouched: the same file as "new", installed by this run`},
	}
	for name, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s dryRun=%v", name, dryRun), func(t *testing.T) {
				target := t.TempDir()
				installed := filepath.Join(target, onDisk(tt.installed, tt.shape))
				if tt.shape == staleDir {
					os.MkdirAll(installed, 0o755)                                              //nolint:errcheck
					os.WriteFile(filepath.Join(installed, "SKILL.md"), []byte("new\n"), 0o644) //nolint:errcheck
				} else {
					os.WriteFile(installed, []byte("new\n"), 0o644) //nolint:errcheck
				}
				if tt.setup != nil {
					tt.setup(t, target)
				}
				var removed []string
				removeFn := func(root *os.Root, name string) error {
					removed = append(removed, name)
					return root.RemoveAll(name)
				}

				out := captureStdout(t, func() {
					removeStale(filepath.Dir(target), target, []string{tt.stale, tt.installed}, []string{tt.installed}, tt.shape, removeFn, dryRun)
				})

				if len(removed) > 0 {
					t.Errorf("removed %v, want nothing\n%s", removed, out)
				}
				if _, err := os.Lstat(installed); err != nil {
					t.Errorf("installed %s gone: %v\n%s", installed, err, out)
				}
				if !strings.Contains(out, tt.wantWarn) {
					t.Errorf("no warning %q:\n%s", tt.wantWarn, out)
				}
				if strings.Contains(out, "[dry-run]") {
					t.Errorf("an installed item was previewed as a removal:\n%s", out)
				}
			})
		}
	}
}

// TestRemoveStale_LstatError: when an entry listed in the directory can't be
// checked, it is kept with a warning; one gone by the time it is checked needs
// nothing.
func TestRemoveStale_LstatError(t *testing.T) {
	tests := map[string]struct {
		shape    staleShape
		entry    string
		lstatErr error
		wantWarn string
	}{
		"an agent file that can't be checked": {shape: staleFile, entry: "old.md", lstatErr: fs.ErrPermission,
			wantWarn: "old.md\" left untouched: permission denied"},
		"a skill directory that can't be checked": {shape: staleDir, entry: "old-skill", lstatErr: fs.ErrPermission,
			wantWarn: "old-skill\" left untouched: permission denied"},
		"an opencode command that can't be checked": {shape: staleCommand, entry: "old", lstatErr: fs.ErrPermission,
			wantWarn: "old.md\" left untouched: permission denied"},
		"an entry already gone": {shape: staleDir, entry: "old-skill", lstatErr: fs.ErrNotExist},
	}
	for name, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s dryRun=%v", name, dryRun), func(t *testing.T) {
				dir := t.TempDir()
				if tt.shape == staleDir {
					writeTestFile(t, filepath.Join(dir, onDisk(tt.entry, tt.shape), "SKILL.md"), "old\n")
				} else {
					writeTestFile(t, filepath.Join(dir, onDisk(tt.entry, tt.shape)), "old\n")
				}
				orig := rootLstat
				rootLstat = func(_ *os.Root, name string) (os.FileInfo, error) {
					return nil, &os.PathError{Op: "lstat", Path: name, Err: tt.lstatErr}
				}
				t.Cleanup(func() { rootLstat = orig })
				var removed []string
				removeFn := func(_ *os.Root, name string) error {
					removed = append(removed, name)
					return nil
				}

				out := captureStdout(t, func() { removeStale(filepath.Dir(dir), dir, []string{tt.entry}, nil, tt.shape, removeFn, dryRun) })

				if len(removed) > 0 || strings.Contains(out, "[dry-run]") {
					t.Errorf("removal attempted or previewed (removed %v):\n%s", removed, out)
				}
				if tt.wantWarn == "" {
					if out != "" {
						t.Errorf("printed %q, want nothing", out)
					}
				} else if !strings.Contains(out, tt.wantWarn) {
					t.Errorf("no warning %q:\n%s", tt.wantWarn, out)
				}
			})
		}
	}
}

// TestDoInstall_CaseVariantManifest: a previous manifest listing an agent or
// skill under another case (a case-only rename between releases) must not
// remove what this run installs. On a case-sensitive filesystem the variant
// simply isn't on disk; the warning is asserted on every filesystem.
func TestDoInstall_CaseVariantManifest(t *testing.T) {
	repoDir := writeOpencodeHookRepo(t)
	for rel, content := range map[string]string{
		"agents/dev-agent.md":      "---\nname: dev-agent\n---\nbody\n",
		"skills/graphify/SKILL.md": "---\nname: graphify\n---\nbody\n",
	} {
		p := filepath.Join(repoDir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)     //nolint:errcheck
		os.WriteFile(p, []byte(content), 0o644) //nolint:errcheck
	}
	targets := map[string]struct {
		install func(*installOpts) error
		paths   func(home string) (manifest string, installed []string)
	}{
		"claude": {doInstallClaude, func(home string) (string, []string) {
			p := testClaudePaths(t, home)
			return p.manifest, []string{filepath.Join(p.agents, "dev-agent.md"), filepath.Join(p.skills, "graphify", "SKILL.md")}
		}},
		"opencode": {doInstallOpencode, func(home string) (string, []string) {
			p := testOpencodePaths(t, home)
			return p.manifest, []string{filepath.Join(p.agents, "dev-agent.md"), filepath.Join(p.skills, "graphify.md")}
		}},
	}
	for tname, target := range targets {
		t.Run(tname, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			fakeCLI(t)
			manifestPath, installed := target.paths(home)
			os.MkdirAll(filepath.Dir(manifestPath), 0o755)                                                 //nolint:errcheck
			os.WriteFile(manifestPath, []byte(`{"agents":["DEV-AGENT.md"],"skills":["GRAPHIFY"]}`), 0o644) //nolint:errcheck
			probe := filepath.Join(home, "case-probe")
			os.WriteFile(probe, nil, 0o644) //nolint:errcheck
			_, err := os.Lstat(filepath.Join(home, "CASE-PROBE"))
			t.Logf("case-insensitive filesystem: %v", err == nil)

			var installErr error
			out := captureStdout(t, func() {
				installErr = target.install(&installOpts{repoDir: repoDir, cfg: &config.Config{}, env: map[string]string{}})
			})
			if installErr != nil {
				t.Fatalf("install error = %v\n%s", installErr, out)
			}
			for _, p := range installed {
				if _, err := os.Stat(p); err != nil {
					t.Errorf("installed %s removed by a case-variant stale entry: %v\n%s", p, err, out)
				}
			}
			for _, w := range []string{
				`"DEV-AGENT.md" left untouched: the same name as "dev-agent.md"`,
				`"GRAPHIFY" left untouched: the same name as "graphify"`,
			} {
				if !strings.Contains(out, w) {
					t.Errorf("no warning %q:\n%s", w, out)
				}
			}
		})
	}
}

func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
}

// TestDoInstall_TamperedAgentSkillManifest drives both installs with a manifest
// whose agent and skill entries point outside the target directories, at the
// directories themselves, or at a user's own files through a cleaned path.
// Only the valid stale entry of each category may be removed; every other
// entry is named in a warning.
func TestDoInstall_TamperedAgentSkillManifest(t *testing.T) {
	repoDir := writeOpencodeHookRepo(t)
	type layout struct {
		manifest, base, agents, skills string
		validAgent, validSkill         string // stale entries that must go
		validSkillPath                 string
	}
	targets := map[string]struct {
		install func(*installOpts) error
		layout  func(home string) layout
		skills  func(l layout, home string) []string
	}{
		"claude": {
			install: doInstallClaude,
			layout: func(home string) layout {
				p := testClaudePaths(t, home)
				return layout{manifest: p.manifest, base: filepath.Dir(p.manifest), agents: p.agents, skills: p.skills,
					validAgent: "old.md", validSkill: "old-skill", validSkillPath: filepath.Join(p.skills, "old-skill", "SKILL.md")}
			},
			skills: func(l layout, home string) []string {
				return []string{"", ".", "..", relPath(t, l.skills, home), relPath(t, l.skills, filepath.Join(home, "precious")),
					filepath.Join(home, "precious"), "sub/../mine"}
			},
		},
		"opencode": {
			install: doInstallOpencode,
			layout: func(home string) layout {
				p := testOpencodePaths(t, home)
				return layout{manifest: p.manifest, base: filepath.Dir(p.manifest), agents: p.agents, skills: p.skills,
					validAgent: "old.md", validSkill: "old-cmd", validSkillPath: filepath.Join(p.skills, "old-cmd.md")}
			},
			// Command entries are recorded without .md; the file has it.
			skills: func(l layout, home string) []string {
				return []string{"", relPath(t, l.skills, filepath.Join(home, "precious")), filepath.Join(home, "precious"), "sub/../mine"}
			},
		},
	}
	for tname, target := range targets {
		t.Run(tname, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			fakeCLI(t)
			l := target.layout(home)
			precious := []string{
				filepath.Join(home, "precious.md"),
				filepath.Join(home, "precious", "SKILL.md"),
				filepath.Join(l.base, "precious.txt"),
				filepath.Join(l.agents, "mine.md"),
				filepath.Join(l.skills, "mine", "SKILL.md"),
				filepath.Join(l.skills, "mine.md"),
			}
			for _, p := range append(precious, filepath.Join(l.agents, l.validAgent), l.validSkillPath) {
				os.MkdirAll(filepath.Dir(p), 0o755)      //nolint:errcheck
				os.WriteFile(p, []byte("keep\n"), 0o644) //nolint:errcheck
			}
			badAgents := []string{"..", ".", relPath(t, l.agents, filepath.Join(home, "precious.md")),
				filepath.Join(home, "precious.md"), "sub/../mine.md"}
			badSkills := target.skills(l, home)
			data, _ := json.Marshal(map[string][]string{
				"agents": append([]string{l.validAgent}, badAgents...),
				"skills": append([]string{l.validSkill}, badSkills...),
			})
			os.WriteFile(l.manifest, data, 0o644) //nolint:errcheck

			var err error
			out := captureStdout(t, func() {
				err = target.install(&installOpts{repoDir: repoDir, cfg: &config.Config{}, env: map[string]string{}})
			})
			if err != nil {
				t.Fatalf("install error = %v\n%s", err, out)
			}
			for _, p := range precious {
				if got, _ := os.ReadFile(p); string(got) != "keep\n" {
					t.Errorf("%s = %q, want untouched\n%s", p, got, out)
				}
			}
			for _, p := range []string{filepath.Join(l.agents, l.validAgent), l.validSkillPath} {
				if _, err := os.Lstat(p); !os.IsNotExist(err) {
					t.Errorf("valid stale entry %s not removed, stat err = %v", p, err)
				}
			}
			if want, n := len(badAgents)+len(badSkills), strings.Count(out, "left untouched: listed in the manifest"); n != want {
				t.Errorf("warned about %d manifest entries, want %d:\n%s", n, want, out)
			}
			// Each warning names the entry exactly as the manifest records it.
			for dir, entries := range map[string][]string{l.agents: badAgents, l.skills: badSkills} {
				for _, e := range entries {
					want := fmt.Sprintf("[devexp]\x1b[0m %q left untouched: listed in the manifest but not a name devexp installs in %q\n", e, dir)
					if !strings.Contains(out, want) {
						t.Errorf("no warning line %q:\n%s", want, out)
					}
				}
			}
		})
	}
}

func relPath(t *testing.T, base, target string) string {
	t.Helper()
	rel, err := filepath.Rel(base, target)
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}
	return rel
}

func TestRunRemove(t *testing.T) {
	t.Run("missing uninstall.sh returns error", func(t *testing.T) {
		repoDir := t.TempDir()

		err := runRemove(repoDir)
		if err == nil {
			t.Fatal("runRemove() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "uninstall.sh not found") {
			t.Errorf("runRemove() error = %v, want it to mention uninstall.sh not found", err)
		}
	})

	t.Run("runs uninstall.sh and returns its exit status", func(t *testing.T) {
		repoDir := t.TempDir()
		script := filepath.Join(repoDir, "uninstall.sh")
		if err := os.WriteFile(script, []byte("#!/bin/bash\nexit 0\n"), 0755); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		if err := runRemove(repoDir); err != nil {
			t.Errorf("runRemove() error = %v, want nil", err)
		}
	})

	t.Run("passes this binary to uninstall.sh as DEVEXP_BIN", func(t *testing.T) {
		repoDir := t.TempDir()
		got := filepath.Join(repoDir, "devexp-bin")
		script := filepath.Join(repoDir, "uninstall.sh")
		if err := os.WriteFile(script, []byte("#!/bin/bash\nprintf '%s' \"$DEVEXP_BIN\" > \""+got+"\"\n"), 0755); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		t.Setenv("DEVEXP_BIN", "stale")

		if err := runRemove(repoDir); err != nil {
			t.Fatalf("runRemove() error = %v", err)
		}
		exe, _ := os.Executable()
		if data, _ := os.ReadFile(got); string(data) != exe {
			t.Errorf("DEVEXP_BIN = %q, want %q", data, exe)
		}
	})

	t.Run("propagates nonzero exit from uninstall.sh", func(t *testing.T) {
		repoDir := t.TempDir()
		script := filepath.Join(repoDir, "uninstall.sh")
		if err := os.WriteFile(script, []byte("#!/bin/bash\nexit 1\n"), 0755); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		if err := runRemove(repoDir); err == nil {
			t.Error("runRemove() error = nil, want non-nil for nonzero exit")
		}
	})
}

// ── Target selection ──────────────────────────────────────────────────────────

// selectTargets is the rule the flag path and the wizard both resolve targets
// with. It is pure, so every combination is checkable here — including the
// both-CLI branch, which no dry-run on a single-CLI machine can reach.
func TestSelectTargets(t *testing.T) {
	tests := map[string]struct {
		hasClaude, hasOpencode bool
		choice                 string
		wantClaude, wantOpen   bool
		wantErr                bool
	}{
		"only claude installed": {
			hasClaude: true, wantClaude: true,
		},
		"only opencode installed": {
			hasOpencode: true, wantOpen: true,
		},
		"both installed, user picks Claude Code": {
			hasClaude: true, hasOpencode: true, choice: "Claude Code",
			wantClaude: true,
		},
		"both installed, user picks opencode": {
			hasClaude: true, hasOpencode: true, choice: "opencode",
			wantOpen: true,
		},
		"both installed, user picks Both": {
			hasClaude: true, hasOpencode: true, choice: "Both",
			wantClaude: true, wantOpen: true,
		},
		"both installed, unrecognised choice selects neither": {
			hasClaude: true, hasOpencode: true, choice: "something else",
		},
		"neither installed is an error": {
			wantErr: true,
		},
		// choice only matters when both are present; a stray value must not
		// override what is actually installed.
		"choice is ignored when only one CLI is present": {
			hasClaude: true, choice: "opencode",
			wantClaude: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			claude, open, err := selectTargets(tt.hasClaude, tt.hasOpencode, tt.choice)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if claude != tt.wantClaude || open != tt.wantOpen {
				t.Errorf("= (claude=%v, opencode=%v), want (claude=%v, opencode=%v)",
					claude, open, tt.wantClaude, tt.wantOpen)
			}
		})
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written. ui.Info goes straight to stdout via fmt.Printf with no
// injectable writer, so this is the only way to assert what was announced.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		io.Copy(&b, r) //nolint:errcheck
		done <- b.String()
	}()

	fn()

	w.Close()
	os.Stdout = orig
	return <-done
}

// announceTargets is the I/O half: it decides nothing and only reports. The
// both-CLI arm is unreachable here because it calls ui.SelectPlatform, which
// needs a TTY.
//
// Asserting the announcement text matters: without it every case would assert
// the same empty choice, and the test would pass unchanged if both arms printed
// the same thing — or nothing at all.
func TestAnnounceTargets(t *testing.T) {
	tests := map[string]struct {
		hasClaude, hasOpencode bool
		wantOut                string
		wantAbsent             string
	}{
		"claude only announces Claude Code": {
			hasClaude: true,
			wantOut:   "Detected: Claude Code",
			// must not claim opencode is present
			wantAbsent: "opencode",
		},
		"opencode only announces opencode": {
			hasOpencode: true,
			wantOut:     "Detected: opencode",
			wantAbsent:  "Claude Code",
		},
		"neither present announces nothing": {
			wantOut: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var choice string
			var err error
			out := captureStdout(t, func() {
				choice, err = announceTargets(tt.hasClaude, tt.hasOpencode)
			})

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// An empty choice means no prompt was shown, which is what separates
			// these arms from the both-CLI one.
			if choice != "" {
				t.Errorf("choice = %q, want empty (no prompt should be shown)", choice)
			}
			if tt.wantOut == "" {
				if strings.TrimSpace(out) != "" {
					t.Errorf("announced %q, want nothing", out)
				}
				return
			}
			if !strings.Contains(out, tt.wantOut) {
				t.Errorf("announced %q, want it to contain %q", out, tt.wantOut)
			}
			if tt.wantAbsent != "" && strings.Contains(out, tt.wantAbsent) {
				t.Errorf("announced %q, must not mention %q", out, tt.wantAbsent)
			}
		})
	}
}

// fakeCLI puts an executable named bin on a PATH containing only that dir, so
// commandExists finds exactly what the test intends and no real CLI leaks in.
func fakeCLI(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", n, err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestDetectTargets(t *testing.T) {
	t.Run("finds claude alone", func(t *testing.T) {
		fakeCLI(t, "claude")
		claude, open, err := detectTargets()
		if err != nil || !claude || open {
			t.Errorf("= (%v, %v, %v), want (true, false, nil)", claude, open, err)
		}
	})

	t.Run("finds opencode alone", func(t *testing.T) {
		fakeCLI(t, "opencode")
		claude, open, err := detectTargets()
		if err != nil || claude || !open {
			t.Errorf("= (%v, %v, %v), want (false, true, nil)", claude, open, err)
		}
	})

	t.Run("errors when neither is on PATH", func(t *testing.T) {
		fakeCLI(t)
		if _, _, err := detectTargets(); err == nil {
			t.Errorf("expected an error when no CLI is installed")
		}
	})
}

// ── Install target paths ──────────────────────────────────────────────────────

func TestTargetHome(t *testing.T) {
	tests := map[string]struct {
		home    string
		wantErr bool
	}{
		"absolute":      {home: "/home/me"},
		"cleaned":       {home: "/home/me/"},
		"empty":         {home: "", wantErr: true},
		"relative":      {home: "home/me", wantErr: true},
		"dot":           {home: ".", wantErr: true},
		"tilde literal": {home: "~", wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := targetHome(tt.home)
			if tt.wantErr {
				if err == nil || got != "" || !strings.Contains(err.Error(), "not an absolute path") {
					t.Errorf("targetHome(%q) = %q, %v; want a refusal", tt.home, got, err)
				}
				return
			}
			if err != nil || got != "/home/me" {
				t.Errorf("targetHome(%q) = %q, %v; want /home/me", tt.home, got, err)
			}
		})
	}
}

// testClaudePaths and testOpencodePaths resolve the target paths under a home
// a test controls, which is always absolute.
func testClaudePaths(t *testing.T, home string) claudePaths {
	t.Helper()
	p, err := claudeTargetPaths(home, time.Now())
	if err != nil {
		t.Fatalf("claudeTargetPaths(%q) error = %v", home, err)
	}
	return p
}

func testOpencodePaths(t *testing.T, home string) opencodePaths {
	t.Helper()
	p, err := opencodeTargetPaths(home)
	if err != nil {
		t.Fatalf("opencodeTargetPaths(%q) error = %v", home, err)
	}
	return p
}

func TestClaudeTargetPaths(t *testing.T) {
	now := time.Date(2026, 9, 16, 4, 5, 6, 0, time.UTC)
	got, err := claudeTargetPaths("/home/u", now)

	want := claudePaths{
		home:     "/home/u",
		agents:   "/home/u/.claude/agents",
		skills:   "/home/u/.claude/skills",
		settings: "/home/u/.claude/settings.json",
		manifest: "/home/u/.claude/.devexp-manifest.json",
		// now is a parameter precisely so this is assertable rather than
		// whatever the clock said when the test ran.
		backup: "/home/u/.claude/.devexp-backup-20260916T040506",
	}
	if err != nil || got != want {
		t.Errorf("claudeTargetPaths() = %+v, %v; want %+v", got, err, want)
	}

	// A relative target path would land under the current directory.
	for _, home := range []string{"", "home/u", "."} {
		if got, err := claudeTargetPaths(home, now); err == nil || got != (claudePaths{}) {
			t.Errorf("claudeTargetPaths(%q) = %+v, %v; want no paths and an error", home, got, err)
		}
	}
}

func TestOpencodeTargetPaths(t *testing.T) {
	got, err := opencodeTargetPaths("/home/u")

	want := opencodePaths{
		home:   "/home/u",
		agents: "/home/u/.config/opencode/agents",
		// skills land in commands/, which is why the field and directory differ
		skills:   "/home/u/.config/opencode/commands",
		plugins:  "/home/u/.config/opencode/plugins",
		config:   "/home/u/.config/opencode/config.json",
		manifest: "/home/u/.config/opencode/.devexp-manifest.json",
	}
	if err != nil || got != want {
		t.Errorf("opencodeTargetPaths() = %+v, %v; want %+v", got, err, want)
	}

	for _, home := range []string{"", "home/u", "."} {
		if got, err := opencodeTargetPaths(home); err == nil || got != (opencodePaths{}) {
			t.Errorf("opencodeTargetPaths(%q) = %+v, %v; want no paths and an error", home, got, err)
		}
	}
}

// testKimiPaths resolves the Kimi target paths under a home a test controls.
func testKimiPaths(t *testing.T, kimiCodeHome, home string) kimiPaths {
	t.Helper()
	p, err := kimiTargetPaths(kimiCodeHome, home, time.Now())
	if err != nil {
		t.Fatalf("kimiTargetPaths(%q, %q) error = %v", kimiCodeHome, home, err)
	}
	return p
}

// resolveKimiHome mirrors Kimi Code's own rule, and refuses the values that
// would aim an install — and from #113 a removal — somewhere it must not.
func TestResolveKimiHome(t *testing.T) {
	tests := map[string]struct {
		kimiCodeHome string
		home         string
		want         string
		wantErr      string
	}{
		"unset falls back to ~/.kimi-code": {home: "/home/u", want: "/home/u/.kimi-code"},
		// Kimi's own CLI treats an empty value as unset, so devexp does too.
		"empty counts as unset":       {kimiCodeHome: "", home: "/home/u", want: "/home/u/.kimi-code"},
		"whitespace counts as unset":  {kimiCodeHome: "   ", home: "/home/u", want: "/home/u/.kimi-code"},
		"an absolute value wins":      {kimiCodeHome: "/opt/k", home: "/home/u", want: "/opt/k"},
		"a trailing slash is cleaned": {kimiCodeHome: "/opt/k/", home: "/home/u", want: "/opt/k"},
		"an inner dotdot is cleaned":  {kimiCodeHome: "/opt/x/../k", home: "/home/u", want: "/opt/k"},
		"a value under home is fine":  {kimiCodeHome: "/home/u/kimi", home: "/home/u", want: "/home/u/kimi"},

		// Kimi resolves a relative value against whatever directory it runs
		// in; devexp refuses rather than install somewhere it cannot name.
		"a relative value is refused": {kimiCodeHome: "rel/dir", home: "/home/u", wantErr: "not an absolute path"},
		"a bare dot is refused":       {kimiCodeHome: ".", home: "/home/u", wantErr: "not an absolute path"},
		"a tilde is refused":          {kimiCodeHome: "~/.kimi-code", home: "/home/u", wantErr: "not an absolute path"},
		// These two would put a manifest, a backup directory and later a
		// removal root at the top of the filesystem or of the user's home.
		"the filesystem root is refused":            {kimiCodeHome: "/", home: "/home/u", wantErr: "will not install into"},
		"the home directory is refused":             {kimiCodeHome: "/home/u", home: "/home/u", wantErr: "will not install into"},
		"the home directory, uncleaned, is refused": {kimiCodeHome: "/home/u/", home: "/home/u", wantErr: "will not install into"},
		// A relative HOME is refused whether or not KIMI_CODE_HOME is set.
		"a bad home is refused":                       {home: "home/u", wantErr: "not an absolute path"},
		"a bad home is refused even with an override": {kimiCodeHome: "/opt/k", home: "", wantErr: "not an absolute path"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveKimiHome(tt.kimiCodeHome, tt.home)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveKimiHome(%q, %q) = %q, %v; want an error containing %q",
						tt.kimiCodeHome, tt.home, got, err, tt.wantErr)
				}
				if got != "" {
					t.Errorf("= %q, want no path alongside the error", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("resolveKimiHome(%q, %q) = %q, %v; want %q",
					tt.kimiCodeHome, tt.home, got, err, tt.want)
			}
		})
	}
}

func TestKimiTargetPaths(t *testing.T) {
	now := time.Date(2026, 9, 18, 4, 5, 6, 0, time.UTC)

	t.Run("under the default root", func(t *testing.T) {
		got, err := kimiTargetPaths("", "/home/u", now)
		want := kimiPaths{
			// home is the Kimi root's parent, which for the default root is
			// exactly $HOME, as for Claude Code.
			home:     "/home/u",
			root:     "/home/u/.kimi-code",
			agents:   "/home/u/.kimi-code/agents",
			skills:   "/home/u/.kimi-code/skills",
			mcp:      "/home/u/.kimi-code/mcp.json",
			config:   "/home/u/.kimi-code/config.toml",
			manifest: "/home/u/.kimi-code/.devexp-manifest.json",
			// now is a parameter precisely so this is assertable rather than
			// whatever the clock said when the test ran.
			backup: "/home/u/.kimi-code/.devexp-backup-20260918T040506",
		}
		if err != nil || got != want {
			t.Errorf("kimiTargetPaths() = %+v, %v; want %+v", got, err, want)
		}
	})

	t.Run("under a KIMI_CODE_HOME outside the user home", func(t *testing.T) {
		got, err := kimiTargetPaths("/opt/k", "/home/u", now)
		want := kimiPaths{
			home:     "/opt",
			root:     "/opt/k",
			agents:   "/opt/k/agents",
			skills:   "/opt/k/skills",
			mcp:      "/opt/k/mcp.json",
			config:   "/opt/k/config.toml",
			manifest: "/opt/k/.devexp-manifest.json",
			backup:   "/opt/k/.devexp-backup-20260918T040506",
		}
		if err != nil || got != want {
			t.Errorf("kimiTargetPaths() = %+v, %v; want %+v", got, err, want)
		}
	})

	// home is what removeStale hands the removal guard, which refuses to check
	// — and so refuses to remove from — a directory that escapes it. Were home
	// $HOME, a KIMI_CODE_HOME outside $HOME would leave every Kimi removal
	// unguardable, which is the bug this assertion exists to catch.
	t.Run("every target directory stays inside home", func(t *testing.T) {
		for _, kimiCodeHome := range []string{"", "/opt/k", "/home/u/kimi"} {
			p := testKimiPaths(t, kimiCodeHome, "/home/u")
			for _, dir := range []string{p.root, p.agents, p.skills, p.backup} {
				rel, err := filepath.Rel(p.home, dir)
				if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
					t.Errorf("KIMI_CODE_HOME=%q: %q escapes home %q (rel %q, %v)", kimiCodeHome, dir, p.home, rel, err)
				}
			}
		}
	})

	t.Run("a refused root yields no paths at all", func(t *testing.T) {
		for _, bad := range []string{"rel/dir", ".", "/", "/home/u"} {
			if got, err := kimiTargetPaths(bad, "/home/u", now); err == nil || got != (kimiPaths{}) {
				t.Errorf("kimiTargetPaths(%q) = %+v, %v; want no paths and an error", bad, got, err)
			}
		}
		for _, badHome := range []string{"", "home/u", "."} {
			if got, err := kimiTargetPaths("", badHome, now); err == nil || got != (kimiPaths{}) {
				t.Errorf("kimiTargetPaths(home=%q) = %+v, %v; want no paths and an error", badHome, got, err)
			}
		}
	})
}

// ── Registry loading ──────────────────────────────────────────────────────────

func writeRegistry(t *testing.T, repoDir, contents string) {
	t.Helper()
	dir := filepath.Join(repoDir, "mcps")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}

func TestLoadFullRegistry(t *testing.T) {
	t.Run("loads the registry", func(t *testing.T) {
		repo := t.TempDir()
		writeRegistry(t, repo, `[{"name":"context7"},{"name":"other"}]`)

		got, err := loadFullRegistry(repo, &config.Config{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0].Name != "context7" {
			t.Errorf("got %+v, want the two registry entries", got)
		}
	})

	t.Run("appends extra MCPs from config", func(t *testing.T) {
		repo := t.TempDir()
		writeRegistry(t, repo, `[{"name":"context7"}]`)

		got, err := loadFullRegistry(repo, &config.Config{
			ExtraMCPs: []byte(`[{"name":"org-internal"}]`),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[1].Name != "org-internal" {
			t.Errorf("got %+v, want the extra MCP appended", got)
		}
	})

	t.Run("malformed extra MCPs warn but do not fail the load", func(t *testing.T) {
		repo := t.TempDir()
		writeRegistry(t, repo, `[{"name":"context7"}]`)

		got, err := loadFullRegistry(repo, &config.Config{ExtraMCPs: []byte(`not json`)})
		if err != nil {
			t.Fatalf("a bad extra MCP should not fail the whole load: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %+v, want just the registry entry", got)
		}
	})

	t.Run("a missing registry is an error", func(t *testing.T) {
		if _, err := loadFullRegistry(t.TempDir(), &config.Config{}); err == nil {
			t.Errorf("expected an error when mcps/registry.json is absent")
		}
	})
}

// ── Backup and stale-removal error paths ──────────────────────────────────────

// blockedDir returns a path whose parent is a regular file, so MkdirAll on it
// must fail — the branch both backup helpers take when the backup directory
// cannot be created.
func blockedDir(t *testing.T) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	return filepath.Join(f, "backup")
}

func TestBackupExisting_ErrorPaths(t *testing.T) {
	t.Run("returns quietly when the backup dir cannot be created", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("hi"), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
		backupExisting(dir, "*.md", blockedDir(t), false)
	})

	t.Run("skips a match that cannot be read", func(t *testing.T) {
		dir := t.TempDir()
		// A directory matching the glob: ReadFile fails, so the loop continues
		// rather than aborting the whole backup.
		if err := os.Mkdir(filepath.Join(dir, "dir.md"), 0o755); err != nil {
			t.Fatalf("Mkdir error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "real.md"), []byte("hi"), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
		backupDir := filepath.Join(t.TempDir(), "backup")

		backupExisting(dir, "*.md", backupDir, false)

		if _, err := os.Stat(filepath.Join(backupDir, "real.md")); err != nil {
			t.Errorf("the readable match should still be backed up: %v", err)
		}
	})
}

func TestBackupExistingDirs_ErrorPaths(t *testing.T) {
	t.Run("returns quietly when the source dir does not exist", func(t *testing.T) {
		backupExistingDirs(filepath.Join(t.TempDir(), "absent"), t.TempDir(), false)
	})

	t.Run("ignores plain files alongside skill dirs", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "loose.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
		skill := filepath.Join(dir, "alpha")
		if err := os.MkdirAll(skill, 0o755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# a"), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
		backupDir := filepath.Join(t.TempDir(), "backup")

		backupExistingDirs(dir, backupDir, false)

		if _, err := os.Stat(filepath.Join(backupDir, "alpha", "SKILL.md")); err != nil {
			t.Errorf("the skill dir should be backed up: %v", err)
		}
		if _, err := os.Stat(filepath.Join(backupDir, "loose.txt")); !os.IsNotExist(err) {
			t.Errorf("a loose file should not be backed up, stat err = %v", err)
		}
	})

	t.Run("returns quietly when the backup dir cannot be created", func(t *testing.T) {
		dir := t.TempDir()
		skill := filepath.Join(dir, "alpha")
		if err := os.MkdirAll(skill, 0o755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# a"), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
		backupExistingDirs(dir, blockedDir(t), false)
	})
}

func TestRemoveStale_RemoveError(t *testing.T) {
	// A real failure (not "already gone") is warned about and does not stop the
	// remaining removals.
	var attempted []string
	removeFn := func(_ *os.Root, name string) error {
		attempted = append(attempted, name)
		if strings.HasPrefix(name, "boom") {
			return errors.New("permission denied")
		}
		return nil
	}

	dir := t.TempDir()
	for _, n := range []string{"boom-one.md", "fine.md", "boom-two.md"} {
		os.WriteFile(filepath.Join(dir, n), []byte("old\n"), 0o644) //nolint:errcheck
	}
	out := captureStdout(t, func() {
		removeStale(filepath.Dir(dir), dir, []string{"boom-one.md", "fine.md", "boom-two.md"}, nil, staleFile, removeFn, false)
	})
	if want := fmt.Sprintf("remove %q: permission denied", filepath.Join(dir, "boom-two.md")); !strings.Contains(out, want) {
		t.Errorf("no warning %q:\n%s", want, out)
	}

	want := []string{"boom-one.md", "fine.md", "boom-two.md"}
	if !reflect.DeepEqual(attempted, want) {
		t.Errorf("attempted = %v, want %v — a failure must not abort the rest", attempted, want)
	}
}

// ── opencode hook plugin ──────────────────────────────────────────────────────

// writeOpencodeHookRepo builds a repo with no agents, skills or MCPs and three
// opencode hooks: two enabled, and graphify-read-guard, which is off for
// Claude Code but on for opencode. Every module has a *.test.js sibling that
// must never be installed.
func writeOpencodeHookRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	files := map[string]string{
		".devexp-toolkit":    "devexp-toolkit\n",
		"mcps/registry.json": "[]",
		"hooks/registry.json": `[
  {"name": "secret-guard", "enabled": true,
   "claude_code": {"event": "PreToolUse", "matcher": "Read", "script": "hooks/claude-code/secret-guard.sh"},
   "opencode": {"event": "tool.execute.before", "module": "hooks/opencode/secret-guard.js", "export": "secretGuard", "fail_closed": true}},
  {"name": "lint-on-save", "enabled": true,
   "opencode": {"event": "file.edited", "module": "hooks/opencode/lint-on-save.js", "export": "lintOnSave"}},
  {"name": "graphify-read-guard", "enabled": false,
   "opencode": {"event": "tool.execute.before", "module": "hooks/opencode/graphify-read-guard.js", "export": "graphifyReadGuard", "enabled": true}}
]`,
		"hooks/opencode/devexp-plugin.js": "/**\n * devexp-plugin.js — entry point for devexp opencode hooks\n */\n",
		"hooks/opencode/utils.js":         "// utils\n",
		"hooks/opencode/package.json":     "{ \"type\": \"module\" }\n",
	}
	for _, n := range []string{"secret-guard", "lint-on-save", "graphify-read-guard"} {
		files["hooks/opencode/"+n+".js"] = "// " + n + "\n"
		files["hooks/opencode/"+n+".test.js"] = "// test " + n + "\n"
	}
	for rel, content := range files {
		p := filepath.Join(repoDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
	}
	for _, d := range []string{"agents/opencode", "skills"} {
		if err := os.MkdirAll(filepath.Join(repoDir, d), 0o755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
	}
	return repoDir
}

func pluginTree(t *testing.T, pluginsDir string) []string {
	t.Helper()
	var out []string
	filepath.Walk(pluginsDir, func(p string, info os.FileInfo, err error) error { //nolint:errcheck
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(pluginsDir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func loadManifestPlugins(t *testing.T, home string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".config", "opencode", ".devexp-manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m struct {
		Plugins []string `json:"plugins"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m.Plugins
}

// TestDoInstallOpencode_Hooks drives the opencode install end to end in a temp
// HOME, one run after another as a user would.
func TestDoInstallOpencode_Hooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoDir := writeOpencodeHookRepo(t)
	pluginsDir := filepath.Join(home, ".config", "opencode", "plugins")
	run := func(t *testing.T, opts *installOpts) string {
		t.Helper()
		opts.repoDir = repoDir
		if opts.cfg == nil {
			opts.cfg = &config.Config{}
		}
		opts.env = map[string]string{}
		var err error
		out := captureStdout(t, func() { err = doInstallOpencode(opts) })
		if err != nil {
			t.Fatalf("doInstallOpencode() error = %v\n%s", err, out)
		}
		return out
	}
	full := []string{
		"devexp.js", "devexp/graphify-read-guard.js", "devexp/hooks.json", "devexp/lint-on-save.js",
		"devexp/package.json", "devexp/secret-guard.js", "devexp/utils.js",
	}

	t.Run("1 full install writes the plugin and records it entry first", func(t *testing.T) {
		out := run(t, &installOpts{})
		if got := pluginTree(t, pluginsDir); !reflect.DeepEqual(got, full) {
			t.Errorf("plugins tree = %v, want %v", got, full)
		}
		if got := loadManifestPlugins(t, home); len(got) != len(full) || got[0] != "devexp.js" {
			t.Errorf("manifest plugins = %v, want %d entries with devexp.js first", got, len(full))
		}
		if !strings.Contains(out, "opencode hooks (3): secret-guard, lint-on-save, graphify-read-guard") ||
			!strings.Contains(out, "Hooks  : "+pluginsDir) {
			t.Errorf("output does not list the opencode hooks and their path:\n%s", out)
		}
	})

	t.Run("2 a hook disabled in config is removed on re-install", func(t *testing.T) {
		out := run(t, &installOpts{cfg: &config.Config{DisabledHooks: []string{"lint-on-save"}}})
		if _, err := os.Stat(filepath.Join(pluginsDir, "devexp", "lint-on-save.js")); !os.IsNotExist(err) {
			t.Errorf("devexp/lint-on-save.js still installed")
		}
		for _, p := range loadManifestPlugins(t, home) {
			if p == "devexp/lint-on-save.js" {
				t.Errorf("manifest still lists devexp/lint-on-save.js")
			}
		}
		if !strings.Contains(out, "devexp/lint-on-save.js") {
			t.Errorf("output does not report the removal:\n%s", out)
		}
	})

	t.Run("3 --agents-only keeps the recorded plugins", func(t *testing.T) {
		before := loadManifestPlugins(t, home)
		run(t, &installOpts{agentsOnly: true, cfg: &config.Config{DisabledHooks: []string{"lint-on-save"}}})
		if got := loadManifestPlugins(t, home); !reflect.DeepEqual(got, before) || len(got) == 0 {
			t.Errorf("manifest plugins after --agents-only = %v, want %v", got, before)
		}
		run(t, &installOpts{skillsOnly: true})
		if got := loadManifestPlugins(t, home); !reflect.DeepEqual(got, before) {
			t.Errorf("manifest plugins after --skills-only = %v, want %v", got, before)
		}
	})

	t.Run("4 a hook deselected in the wizard is not installed", func(t *testing.T) {
		run(t, &installOpts{selectedHooks: []string{"lint-on-save"}})
		if _, err := os.Stat(filepath.Join(pluginsDir, "devexp", "secret-guard.js")); !os.IsNotExist(err) {
			t.Errorf("devexp/secret-guard.js installed though deselected")
		}
		if _, err := os.Stat(filepath.Join(pluginsDir, "devexp", "lint-on-save.js")); err != nil {
			t.Errorf("devexp/lint-on-save.js not installed though selected: %v", err)
		}
	})

	t.Run("5 every hook disabled removes the plugin and says why", func(t *testing.T) {
		foreign := filepath.Join(pluginsDir, "my-plugin.js")
		os.WriteFile(foreign, []byte("export const mine = 1\n"), 0o644) //nolint:errcheck
		out := run(t, &installOpts{cfg: &config.Config{DisabledHooks: []string{"secret-guard", "lint-on-save", "graphify-read-guard"}}})
		if got := pluginTree(t, pluginsDir); !reflect.DeepEqual(got, []string{"my-plugin.js"}) {
			t.Errorf("plugins tree = %v, want only the foreign my-plugin.js", got)
		}
		if _, err := os.Stat(filepath.Join(pluginsDir, "devexp")); !os.IsNotExist(err) {
			t.Errorf("devexp/ directory left behind")
		}
		if got := loadManifestPlugins(t, home); len(got) != 0 {
			t.Errorf("manifest plugins = %v, want none", got)
		}
		if !strings.Contains(out, "every hook is disabled") {
			t.Errorf("output does not say why:\n%s", out)
		}
	})

	t.Run("6 dry-run on a clean HOME writes nothing", func(t *testing.T) {
		clean := t.TempDir()
		t.Setenv("HOME", clean)
		out := run(t, &installOpts{dryRun: true})
		if _, err := os.Stat(filepath.Join(clean, ".config")); !os.IsNotExist(err) {
			t.Errorf("dry-run wrote under HOME: %v", pluginTree(t, clean))
		}
		if n := strings.Count(out, "write "+filepath.Join(clean, ".config", "opencode", "plugins")); n != len(full) {
			t.Errorf("dry-run listed %d plugin files, want %d:\n%s", n, len(full), out)
		}
	})

	t.Run("7 a legacy flat install is cleaned up, foreign content kept", func(t *testing.T) {
		clean := t.TempDir()
		t.Setenv("HOME", clean)
		plugins := filepath.Join(clean, ".config", "opencode", "plugins")
		os.MkdirAll(plugins, 0o755) //nolint:errcheck
		legacy := "/**\n * secret-guard.js — blocks accidental reads of .env and private key files\n */\n"
		os.WriteFile(filepath.Join(plugins, "secret-guard.js"), []byte(legacy), 0o644)              //nolint:errcheck
		os.WriteFile(filepath.Join(plugins, "utils.js"), []byte("export const mine = 1\n"), 0o644)  //nolint:errcheck
		os.WriteFile(filepath.Join(plugins, "package.json"), []byte(`{ "type": "module" }`), 0o644) //nolint:errcheck
		configPath := filepath.Join(clean, ".config", "opencode", "config.json")
		config := `{"theme":"x","plugin":["` + filepath.Join(plugins, "devexp-plugin.js") + `","npm-plugin"]}`
		os.WriteFile(configPath, []byte(config), 0o644) //nolint:errcheck

		run(t, &installOpts{})
		for _, gone := range []string{"secret-guard.js", "package.json"} {
			if _, err := os.Stat(filepath.Join(plugins, gone)); !os.IsNotExist(err) {
				t.Errorf("legacy %s not removed", gone)
			}
		}
		if got, _ := os.ReadFile(filepath.Join(plugins, "utils.js")); string(got) != "export const mine = 1\n" {
			t.Errorf("user utils.js = %q, changed", got)
		}
		got, _ := os.ReadFile(configPath)
		var cfg map[string]any
		json.Unmarshal(got, &cfg) //nolint:errcheck
		if !reflect.DeepEqual(cfg["plugin"], []any{"npm-plugin"}) || cfg["theme"] != "x" {
			t.Errorf("config.json = %s, want the legacy entry dropped and the rest kept", got)
		}
	})
}

// TestDoInstallOpencode_TamperedManifest: stale removal trusts the manifest
// only for paths devexp installs, so a hand-edited plugins list can never
// delete a file outside devexp.js and devexp/.
func TestDoInstallOpencode_TamperedManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoDir := writeOpencodeHookRepo(t)
	base := filepath.Join(home, ".config", "opencode")
	plugins := filepath.Join(base, "plugins")
	os.MkdirAll(plugins, 0o755) //nolint:errcheck
	precious := map[string]string{
		filepath.Join(base, "precious.txt"):      "keep me\n",
		filepath.Join(plugins, "my-plugin.js"):   "export const mine = 1\n",
		filepath.Join(plugins, "devexp", "x.md"): "mine\n",
	}
	for p, c := range precious {
		os.MkdirAll(filepath.Dir(p), 0o755) //nolint:errcheck
		os.WriteFile(p, []byte(c), 0o644)   //nolint:errcheck
	}
	manifestJSON := `{"agents":[],"skills":[],"plugins":["devexp.js","../precious.txt","my-plugin.js","devexp/x.md"]}`
	os.WriteFile(filepath.Join(base, ".devexp-manifest.json"), []byte(manifestJSON), 0o644) //nolint:errcheck

	var err error
	out := captureStdout(t, func() {
		err = doInstallOpencode(&installOpts{repoDir: repoDir, cfg: &config.Config{}, env: map[string]string{}})
	})
	if err != nil {
		t.Fatalf("doInstallOpencode() error = %v\n%s", err, out)
	}
	for p, c := range precious {
		if got, _ := os.ReadFile(p); string(got) != c {
			t.Errorf("%s = %q, want %q untouched", p, got, c)
		}
	}
	if n := strings.Count(out, "left untouched: listed in the manifest"); n != 3 {
		t.Errorf("warned about %d manifest paths, want 3:\n%s", n, out)
	}
}

// TestDoInstall_PartialRunsPrintNoHooks: --agents-only and --skills-only
// don't install hooks, so neither target may print anything about hooks.
func TestDoInstall_PartialRunsPrintNoHooks(t *testing.T) {
	repoDir := writeOpencodeHookRepo(t)
	targets := map[string]func(*installOpts) error{"claude": doInstallClaude, "opencode": doInstallOpencode}
	scopes := map[string]installOpts{
		"agents-only": {agentsOnly: true},
		"skills-only": {skillsOnly: true},
	}
	for tname, install := range targets {
		for sname, scope := range scopes {
			t.Run(tname+" "+sname, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				fakeCLI(t)
				opts := scope
				opts.repoDir, opts.cfg, opts.env = repoDir, &config.Config{}, map[string]string{}
				var err error
				out := captureStdout(t, func() { err = install(&opts) })
				if err != nil {
					t.Fatalf("install error = %v\n%s", err, out)
				}
				// Temp paths carry this test's name, which contains "Hooks".
				plain := strings.NewReplacer(home, "<home>", repoDir, "<repo>").Replace(out)
				if strings.Contains(strings.ToLower(plain), "hook") {
					t.Errorf("%s run printed hook output:\n%s", sname, plain)
				}
			})
		}
	}
}

// TestDoInstall_UnreadableManifest: a manifest that can't be read or parsed
// must not panic, must be reported, must not make anything stale, and the
// install must finish and rewrite the manifest when it can.
func TestDoInstall_UnreadableManifest(t *testing.T) {
	repoDir := writeOpencodeHookRepo(t)
	// Lists files that exist on disk and that this repo doesn't ship: a
	// trusted manifest would mark them stale. skills is a type mismatch, so
	// json still fills agents and plugins before reporting the error.
	const partial = `{"agents":["mine.md"],"skills":{"x":1},"plugins":["devexp/old.js"]}`
	type layout struct{ manifest, agents, plugins string }
	targets := map[string]struct {
		install func(*installOpts) error
		paths   func(t *testing.T, home string) layout
	}{
		"claude": {doInstallClaude, func(t *testing.T, home string) layout {
			p := testClaudePaths(t, home)
			return layout{p.manifest, p.agents, ""}
		}},
		"opencode": {doInstallOpencode, func(t *testing.T, home string) layout {
			p := testOpencodePaths(t, home)
			return layout{p.manifest, p.agents, p.plugins}
		}},
	}
	corruptions := map[string]struct {
		write       func(t *testing.T, path string)
		wantRewrite bool
	}{
		"a directory at the manifest path": {write: func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		"invalid JSON": {wantRewrite: true, write: func(t *testing.T, path string) {
			os.WriteFile(path, []byte("{not json"), 0o644) //nolint:errcheck
		}},
		"partially decodable JSON": {wantRewrite: true, write: func(t *testing.T, path string) {
			os.WriteFile(path, []byte(partial), 0o644) //nolint:errcheck
		}},
	}
	for tname, target := range targets {
		for cname, c := range corruptions {
			t.Run(tname+" "+cname, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				fakeCLI(t)
				l := target.paths(t, home)
				precious := []string{filepath.Join(l.agents, "mine.md")}
				if l.plugins != "" {
					precious = append(precious, filepath.Join(l.plugins, "devexp", "old.js"))
				}
				for _, p := range precious {
					os.MkdirAll(filepath.Dir(p), 0o755)      //nolint:errcheck
					os.WriteFile(p, []byte("keep\n"), 0o644) //nolint:errcheck
				}
				os.MkdirAll(filepath.Dir(l.manifest), 0o755) //nolint:errcheck
				c.write(t, l.manifest)

				var err error
				out := captureStdout(t, func() {
					err = target.install(&installOpts{repoDir: repoDir, cfg: &config.Config{}, env: map[string]string{}})
				})
				if err != nil {
					t.Fatalf("install error = %v\n%s", err, out)
				}
				if !strings.Contains(out, "manifest "+l.manifest+" is unreadable") {
					t.Errorf("no warning naming the manifest:\n%s", out)
				}
				for _, p := range precious {
					if got, _ := os.ReadFile(p); string(got) != "keep\n" {
						t.Errorf("%s removed or changed on a run with an unreadable manifest", p)
					}
				}
				if !strings.Contains(out, "installation complete") {
					t.Errorf("install did not complete:\n%s", out)
				}
				if c.wantRewrite {
					data, _ := os.ReadFile(l.manifest)
					var m map[string]any
					if json.Unmarshal(data, &m) != nil {
						t.Errorf("manifest not rewritten as valid JSON: %q", data)
					}
				}
			})
		}
	}
}

// runOpencode runs doInstallOpencode in the current HOME and returns its
// output and error.
func runOpencode(t *testing.T, repoDir string, cfg *config.Config) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() {
		err = doInstallOpencode(&installOpts{repoDir: repoDir, cfg: cfg, env: map[string]string{}})
	})
	return out, err
}

func allHooksDisabled() *config.Config {
	return &config.Config{DisabledHooks: []string{"secret-guard", "lint-on-save", "graphify-read-guard"}}
}

func treeBytes(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error { //nolint:errcheck
		if err == nil && !info.IsDir() {
			data, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			snap[rel] = string(data)
		}
		return nil
	})
	return snap
}

// TestDoInstallOpencode_RefusalKeepsLegacyInstall: legacy cleanup runs only
// after the new plugin is installed, so a refused install leaves the working
// legacy plugin and its config entry exactly as they were.
func TestDoInstallOpencode_RefusalKeepsLegacyInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeCLI(t)
	repoDir := writeOpencodeHookRepo(t)
	p := testOpencodePaths(t, home)
	os.MkdirAll(p.plugins, 0o755) //nolint:errcheck
	fixtures := filepath.Join("..", "internal", "hooks", "testdata", "legacy-opencode")
	entries, err := os.ReadDir(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(fixtures, e.Name()))
		os.WriteFile(filepath.Join(p.plugins, e.Name()), data, 0o644) //nolint:errcheck
	}
	os.WriteFile(filepath.Join(p.plugins, "devexp.js"), []byte("export const Mine = async () => ({})\n"), 0o644) //nolint:errcheck
	cfgJSON := `{"plugin":["` + filepath.Join(p.plugins, "devexp-plugin.js") + `"]}`
	os.WriteFile(p.config, []byte(cfgJSON), 0o644) //nolint:errcheck
	before := treeBytes(t, p.plugins)

	out, err := runOpencode(t, repoDir, &config.Config{})
	if err == nil || !strings.Contains(err.Error(), "not a devexp plugin entry") {
		t.Fatalf("doInstallOpencode() error = %v, want the devexp.js refusal\n%s", err, out)
	}
	if after := treeBytes(t, p.plugins); !reflect.DeepEqual(before, after) {
		t.Errorf("plugins/ changed on a refused install")
	}
	if got, _ := os.ReadFile(p.config); string(got) != cfgJSON {
		t.Errorf("config.json = %s, want the legacy entry kept", got)
	}
}

// TestDoInstallOpencode_LostManifest: a corrupt manifest no longer leaves the
// plugin installed for good.
func TestDoInstallOpencode_LostManifest(t *testing.T) {
	setup := func(t *testing.T) (repoDir string, p opencodePaths) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		fakeCLI(t)
		repoDir = writeOpencodeHookRepo(t)
		if out, err := runOpencode(t, repoDir, &config.Config{}); err != nil {
			t.Fatalf("install: %v\n%s", err, out)
		}
		p = testOpencodePaths(t, home)
		os.WriteFile(p.manifest, []byte(`{"agents": "oops"`), 0o644) //nolint:errcheck
		return repoDir, p
	}

	t.Run("every hook disabled removes the plugin", func(t *testing.T) {
		repoDir, p := setup(t)
		for i := 0; i < 2; i++ {
			if out, err := runOpencode(t, repoDir, allHooksDisabled()); err != nil {
				t.Fatalf("install: %v\n%s", err, out)
			}
		}
		for _, gone := range []string{"devexp.js", "devexp"} {
			if _, err := os.Lstat(filepath.Join(p.plugins, gone)); !os.IsNotExist(err) {
				t.Errorf("plugins/%s still installed", gone)
			}
		}
	})

	t.Run("a normal install rewrites the plugins list", func(t *testing.T) {
		repoDir, p := setup(t)
		if out, err := runOpencode(t, repoDir, &config.Config{}); err != nil {
			t.Fatalf("install: %v\n%s", err, out)
		}
		home := filepath.Dir(filepath.Dir(filepath.Dir(p.plugins)))
		if got := loadManifestPlugins(t, home); len(got) != 7 || got[0] != "devexp.js" {
			t.Errorf("manifest plugins = %v, want 7 entries with devexp.js first", got)
		}
	})
}

// TestDoInstallOpencode_SymlinkedDevexpAllDisabled is the review repro: a
// devexp/ linked to a checkout, every hook disabled. Nothing behind the link
// may be deleted, and the link itself stays.
func TestDoInstallOpencode_SymlinkedDevexpAllDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeCLI(t)
	repoDir := writeOpencodeHookRepo(t)
	if out, err := runOpencode(t, repoDir, &config.Config{}); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	p := testOpencodePaths(t, home)
	checkout := filepath.Join(t.TempDir(), "checkout")
	if err := os.Rename(filepath.Join(repoDir, "hooks", "opencode"), checkout); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(p.plugins, "devexp"))                  //nolint:errcheck
	os.Symlink(checkout, filepath.Join(p.plugins, "devexp"))          //nolint:errcheck
	os.Symlink(checkout, filepath.Join(repoDir, "hooks", "opencode")) //nolint:errcheck
	before := treeBytes(t, checkout)

	out, err := runOpencode(t, repoDir, allHooksDisabled())
	if err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Errorf("doInstallOpencode() error = %v, want a symlink refusal\n%s", err, out)
	}
	if after := treeBytes(t, checkout); !reflect.DeepEqual(before, after) {
		t.Errorf("files in the linked checkout were deleted: %d before, %d after", len(before), len(after))
	}
	if fi, err := os.Lstat(filepath.Join(p.plugins, "devexp")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the devexp symlink was removed")
	}
}

// TestDoInstallOpencode_SymlinkedPluginsDir: owner decision "write, never
// remove". Through a symlinked plugins/ the plugin is installed, but neither
// the legacy flat files nor a later all-disabled run removes anything; the
// files left behind are listed and stay recorded.
func TestDoInstallOpencode_SymlinkedPluginsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeCLI(t)
	repoDir := writeOpencodeHookRepo(t)
	p := testOpencodePaths(t, home)
	dotfiles := filepath.Join(t.TempDir(), "dotfiles-plugins")
	os.MkdirAll(dotfiles, 0o755)                //nolint:errcheck
	os.MkdirAll(filepath.Dir(p.plugins), 0o755) //nolint:errcheck
	if err := os.Symlink(dotfiles, p.plugins); err != nil {
		t.Fatal(err)
	}
	legacy := "/**\n * secret-guard.js — blocks accidental reads of .env and private key files\n */\n"
	os.WriteFile(filepath.Join(dotfiles, "secret-guard.js"), []byte(legacy), 0o644) //nolint:errcheck

	out, err := runOpencode(t, repoDir, &config.Config{})
	if err != nil {
		t.Fatalf("install through a symlinked plugins/: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dotfiles, "devexp", "hooks.json")); err != nil {
		t.Errorf("plugin not installed inside the link target: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dotfiles, "secret-guard.js")); string(got) != legacy {
		t.Errorf("legacy file removed through the symlink")
	}
	if !strings.Contains(out, "never removes files through it; remove these by hand") {
		t.Errorf("no warning listing the legacy file left behind:\n%s", out)
	}

	before := treeBytes(t, dotfiles)
	out, err = runOpencode(t, repoDir, allHooksDisabled())
	if err != nil {
		t.Fatalf("all-disabled run: %v\n%s", err, out)
	}
	if after := treeBytes(t, dotfiles); !reflect.DeepEqual(before, after) {
		t.Errorf("files removed through the plugins symlink on an all-disabled run")
	}
	if got := loadManifestPlugins(t, home); len(got) != 7 || got[0] != "devexp.js" {
		t.Errorf("manifest plugins = %v, want the 7 files left behind still recorded", got)
	}
	if fi, err := os.Lstat(p.plugins); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("plugins symlink removed or replaced")
	}
}

// ── HOME refusal (#126) ───────────────────────────────────────────────────────

// badHomes are the HOME values that would turn every target path into one
// under the current directory: unset, empty and relative.
var badHomes = map[string]func(t *testing.T){
	"unset":    func(t *testing.T) { t.Setenv("HOME", ""); os.Unsetenv("HOME") }, //nolint:errcheck
	"empty":    func(t *testing.T) { t.Setenv("HOME", "") },
	"relative": func(t *testing.T) { t.Setenv("HOME", "home") },
}

// writeDotfilesTree fills dir with what a bad HOME would make devexp read,
// write or remove there, at the top (HOME empty) and under home/ (HOME=home):
// an existing Claude Code and opencode install, each with a stale agent the
// manifest lists, and an embedded-asset cache from another version in the
// user cache dir (darwin and linux) holding a file of its own, which
// repo.Resolve would wipe before re-extracting.
func writeDotfilesTree(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{}
	for rel, content := range map[string]string{
		"Library/Caches/devexp/assets/.devexp-version":    "some-other",
		"Library/Caches/devexp/assets/precious.txt":       "keep\n",
		".cache/devexp/assets/.devexp-version":            "some-other",
		".cache/devexp/assets/precious.txt":               "keep\n",
		".claude/agents/mine.md":                          "mine\n",
		".claude/agents/old.md":                           "old\n",
		".claude/skills/mine/SKILL.md":                    "mine\n",
		".claude/settings.json":                           "{\"hooks\": {}}\n",
		".claude/.devexp-manifest.json":                   `{"agents":["old.md"]}`,
		".config/opencode/agents/old.md":                  "old\n",
		".config/opencode/config.json":                    "{\"mcp\": {}}\n",
		".config/opencode/.devexp-manifest.json":          `{"agents":["old.md"],"plugins":["devexp.js"]}`,
		".config/opencode/plugins/devexp.js":              "/**\n * devexp-plugin.js — entry point for devexp opencode hooks\n */\n",
		".config/opencode/plugins/devexp/secret-guard.js": "// secret-guard\n",
	} {
		files[rel] = content
		files["home/"+rel] = content
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// refusalRepo is writeOpencodeHookRepo with one stdio MCP in the registry, so
// an install that got as far as MCP registration would call the CLI.
func refusalRepo(t *testing.T) string {
	t.Helper()
	repoDir := writeOpencodeHookRepo(t)
	registry := `[{"name": "probe", "command": "echo", "args": ["hi"], "scope": "user"}]`
	if err := os.WriteFile(filepath.Join(repoDir, "mcps", "registry.json"), []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
	return repoDir
}

// loggingCLI is fakeCLI whose executables append every invocation to a log
// file and succeed. The log's absolute path is written into each script, so no
// HOME, cwd or environment change can send a call anywhere else. It returns
// the log path; the file exists only once something was called.
func loggingCLI(t *testing.T, names ...string) (calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(t.TempDir(), "calls")
	for _, n := range names {
		script := "#!/bin/sh\necho \"$0 $*\" >> '" + calls + "'\n"
		if err := os.WriteFile(filepath.Join(dir, n), []byte(script), 0o755); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", n, err)
		}
	}
	t.Setenv("PATH", dir)
	return calls
}

// noCalls fails the test if any logged CLI was invoked.
func noCalls(t *testing.T, calls string) {
	t.Helper()
	if data, err := os.ReadFile(calls); err == nil {
		t.Errorf("CLI called despite the HOME refusal:\n%s", data)
	}
}

// treeState is treeBytes plus every directory, so a run that only creates an
// empty directory (a backup dir, a plugins dir) still shows up as a change.
func treeState(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := treeBytes(t, root)
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error { //nolint:errcheck
		if err == nil && info.IsDir() {
			rel, _ := filepath.Rel(root, p)
			snap[rel+"/"] = "dir"
		}
		return nil
	})
	return snap
}

// TestInstallCmd_RefusesBadHome: `devexp install` refuses an unset, empty or
// relative HOME before it does anything — before resolving the assets (a
// standalone binary extracts them under the user cache dir, which a relative
// HOME puts under the current directory), before the wizard, and before MCP
// registration — so a dotfiles tree in the current directory is left byte for
// byte as it was.
func TestInstallCmd_RefusesBadHome(t *testing.T) {
	runs := map[string]struct {
		clis       []string
		args       []string
		standalone bool // no DEVEXP_DIR: assets come from the binary
	}{
		"flags, claude":     {clis: []string{"claude"}, args: []string{"install", "--reinstall-mcps"}},
		"flags, opencode":   {clis: []string{"opencode"}, args: []string{"install", "--reinstall-mcps"}},
		"flags, mcps only":  {clis: []string{"claude"}, args: []string{"install", "--mcps-only"}},
		"flags, standalone": {clis: []string{"opencode"}, args: []string{"install", "--agents-only"}, standalone: true},
		"wizard":            {clis: []string{"claude"}, args: []string{"install"}},
	}
	for rname, run := range runs {
		for hname, setHome := range badHomes {
			t.Run(rname+", HOME "+hname, func(t *testing.T) {
				cwd := t.TempDir()
				writeDotfilesTree(t, cwd)
				t.Chdir(cwd)
				tmp := t.TempDir() // where the embedded assets go when there is no user cache dir
				t.Setenv("TMPDIR", tmp)
				t.Setenv("XDG_CACHE_HOME", "")
				calls := loggingCLI(t, run.clis...)
				if run.standalone {
					t.Setenv("DEVEXP_DIR", "")
				} else {
					t.Setenv("DEVEXP_DIR", refusalRepo(t))
				}
				before := treeState(t, cwd)
				setHome(t)

				out, err := executeRoot(t, run.args...)
				if err == nil || !strings.Contains(err.Error(), "HOME is") || !strings.Contains(err.Error(), "refusing to install anything") {
					t.Errorf("install error = %v, want a HOME refusal\n%s", err, out)
				}
				if after := treeState(t, cwd); !reflect.DeepEqual(before, after) {
					t.Errorf("files under the current directory changed:\nbefore %v\nafter  %v", before, after)
				}
				if got := treeState(t, tmp); len(got) != 1 {
					t.Errorf("wrote under TMPDIR: %v", got)
				}
				noCalls(t, calls)
			})
		}
	}
}

// TestDoInstall_RefusesBadHome: the per-target installers resolve their paths
// through the same check, so even called directly they refuse before MCP
// registration or any other write.
func TestDoInstall_RefusesBadHome(t *testing.T) {
	targets := map[string]func(*installOpts) error{
		"claude":   doInstallClaude,
		"opencode": doInstallOpencode,
	}
	for tname, install := range targets {
		for hname, setHome := range badHomes {
			t.Run(tname+", HOME "+hname, func(t *testing.T) {
				cwd := t.TempDir()
				writeDotfilesTree(t, cwd)
				t.Chdir(cwd)
				calls := loggingCLI(t, "claude", "opencode")
				repoDir := refusalRepo(t)
				before := treeState(t, cwd)
				setHome(t)

				var err error
				out := captureStdout(t, func() {
					err = install(&installOpts{repoDir: repoDir, cfg: &config.Config{}, env: map[string]string{}})
				})
				if err == nil || !strings.Contains(err.Error(), "not an absolute path") {
					t.Errorf("install error = %v, want a HOME refusal\n%s", err, out)
				}
				if after := treeState(t, cwd); !reflect.DeepEqual(before, after) {
					t.Errorf("files under the current directory changed:\nbefore %v\nafter  %v", before, after)
				}
				noCalls(t, calls)
			})
		}
	}
}

// ── DEVEXP_DIR resolution (#126) ──────────────────────────────────────────────

// TestInstallCmd_RelativeDevexpDir: a relative DEVEXP_DIR is resolved to an
// absolute path, so every hook command written to settings.json is absolute.
func TestInstallCmd_RelativeDevexpDir(t *testing.T) {
	for name, devexpDir := range map[string]func(repoDir string) (cwd, dir string){
		"dot": func(repoDir string) (string, string) { return repoDir, "." },
		"relative through ..": func(repoDir string) (string, string) {
			return filepath.Dir(repoDir), "sub/../" + filepath.Base(repoDir)
		},
	} {
		t.Run(name, func(t *testing.T) {
			repoDir := writeOpencodeHookRepo(t)
			if err := os.MkdirAll(filepath.Join(repoDir, "hooks", "claude-code"), 0o755); err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(repoDir, "hooks", "claude-code", "secret-guard.sh"), []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck
			home := t.TempDir()
			t.Setenv("HOME", home)
			loggingCLI(t, "claude")
			cwd, dir := devexpDir(repoDir)
			t.Chdir(cwd)
			t.Setenv("DEVEXP_DIR", dir)

			out, err := executeRoot(t, "install", "--reinstall-mcps")
			if err != nil {
				t.Fatalf("install error = %v\n%s", err, out)
			}
			commands := hookCommands(t, home)
			if len(commands) != 1 {
				t.Fatalf("hook commands = %v, want the one secret-guard registration", commands)
			}
			for _, c := range commands {
				if !filepath.IsAbs(c) || !strings.HasSuffix(c, "/hooks/claude-code/secret-guard.sh") {
					t.Errorf("hook command %q, want an absolute path to secret-guard.sh", c)
				}
				if _, err := os.Stat(c); err != nil {
					t.Errorf("hook command %q does not point at the script: %v", c, err)
				}
			}
		})
	}
}

// hookCommands returns every hook command in HOME's Claude Code settings.json,
// failing the test when the file was not written.
func hookCommands(t *testing.T, home string) []string {
	t.Helper()
	data, err := os.ReadFile(testClaudePaths(t, home).settings)
	if err != nil {
		t.Fatalf("settings.json not written: %v", err)
	}
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string } `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, entries := range settings.Hooks {
		for _, e := range entries {
			for _, h := range e.Hooks {
				commands = append(commands, h.Command)
			}
		}
	}
	return commands
}

// TestInstallCmd_DevexpDirNotARepo: a DEVEXP_DIR that isn't a devexp-toolkit checkout is an
// error before anything is installed — no fallback to another repo or to the
// embedded assets, no CLI call, nothing under HOME.
func TestInstallCmd_DevexpDirNotARepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	calls := loggingCLI(t, "claude")
	cwd := t.TempDir()
	t.Chdir(cwd)
	os.MkdirAll(filepath.Join(cwd, "not-a-repo", "agents"), 0o755) //nolint:errcheck
	t.Setenv("DEVEXP_DIR", "not-a-repo")

	out, err := executeRoot(t, "install", "--reinstall-mcps")
	if err == nil || !strings.Contains(err.Error(), "not a devexp-toolkit checkout") {
		t.Errorf("install error = %v, want a not-a-checkout error\n%s", err, out)
	}
	if got := treeState(t, home); len(got) != 1 {
		t.Errorf("wrote under HOME: %v", got)
	}
	if got := treeState(t, cwd); len(got) != 3 {
		t.Errorf("wrote under the cwd: %v", got)
	}
	noCalls(t, calls)
}

// ── Asset root detection (#134) ───────────────────────────────────────────────

// TestInstallCmd_AssetRoot: without DEVEXP_DIR, `devexp install` run inside
// a directory tree never installs from that tree, marked as a devexp-toolkit
// checkout or not: a dev build uses the checkout it was built from (for
// `go test`, this repository), a tagged build its bundled assets. Nothing from
// the tree is registered or copied, the tree is left as it was, and the asset
// root is printed before anything is installed.
func TestInstallCmd_AssetRoot(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// <root>/cli/cmd/install_test.go; under -trimpath the path is
	// module-relative, no checkout is recorded and dev builds use the bundled
	// assets too.
	sourceCheckout := ""
	if filepath.IsAbs(thisFile) {
		sourceCheckout = filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	}
	tests := map[string]struct {
		version string
		marker  bool
	}{
		"dev build, same shape without the marker":    {version: "dev"},
		"dev build, a devexp-toolkit checkout":        {version: "dev", marker: true},
		"tagged build, same shape without the marker": {version: "v9.9.9"},
		"tagged build, a devexp-toolkit checkout":     {version: "v9.9.9", marker: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			origVersion := version
			version = tt.version
			t.Cleanup(func() { version = origVersion })

			tree, err := filepath.EvalSymlinks(refusalRepo(t))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(tree, "hooks", "claude-code"), 0o755); err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(tree, "hooks", "claude-code", "secret-guard.sh"), []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck
			if !tt.marker {
				if err := os.Remove(filepath.Join(tree, ".devexp-toolkit")); err != nil {
					t.Fatal(err)
				}
			}
			sub := filepath.Join(tree, "hooks", "opencode")
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CACHE_HOME", "")
			t.Setenv("DEVEXP_DIR", "")
			calls := loggingCLI(t, "claude")
			t.Chdir(sub)
			before := treeState(t, tree)

			out, err := executeRoot(t, "install", "--reinstall-mcps")
			if err != nil {
				t.Fatalf("install error = %v\n%s", err, out)
			}

			wantRoot, wantOrigin := sourceCheckout, "the devexp-toolkit checkout this binary was built from"
			// A checkout that isn't verifiably the user's (e.g. cloned with a
			// group-writable umask) is skipped with a warning.
			unverified := strings.Contains(out, "can't be verified as yours")
			if unverified {
				t.Logf("this checkout can't be verified as the user's; expecting the bundled assets:\n%s", out)
			}
			if tt.version != "dev" || sourceCheckout == "" || unverified {
				if tt.version != "dev" && unverified {
					t.Errorf("a tagged build checked a source checkout:\n%s", out)
				}
				cache, err := os.UserCacheDir()
				if err != nil {
					t.Fatal(err)
				}
				dir := "assets"
				if tt.version == "dev" {
					dir = "assets-dev"
				}
				wantRoot, wantOrigin = filepath.Join(cache, "devexp", dir), "assets bundled in this binary, extracted to the user cache"
			}
			announce := "Asset root: " + wantRoot + " (" + wantOrigin + ")"
			at := strings.Index(out, announce)
			if at < 0 {
				t.Fatalf("output does not announce %q:\n%s", announce, out)
			}
			if detected := strings.Index(out, "Detected:"); detected < at {
				t.Errorf("asset root announced after the install started (at %d, first install output at %d):\n%s", at, detected, out)
			}

			commands := hookCommands(t, home)
			if len(commands) == 0 {
				t.Fatalf("no hooks registered\n%s", out)
			}
			for _, c := range commands {
				if !strings.HasPrefix(c, wantRoot+string(filepath.Separator)) {
					t.Errorf("hook command %q is not under the asset root %q", c, wantRoot)
				}
			}
			if logged, _ := os.ReadFile(calls); strings.Contains(string(logged), "probe") {
				t.Errorf("the tree's MCP was registered; CLI calls:\n%s", logged)
			}
			if after := treeState(t, tree); !reflect.DeepEqual(before, after) {
				t.Errorf("files in the tree changed:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

// TestAnnounceAssetRoot: a warning about a skipped source checkout is shown,
// before the asset root line.
func TestAnnounceAssetRoot(t *testing.T) {
	tests := map[string]struct {
		src   repo.Source
		want  []string // in order
		avoid []string
	}{
		"checkout": {
			src:   repo.Source{RepoDir: "/src/toolkit", Origin: repo.OriginSourceDir},
			want:  []string{"Asset root: /src/toolkit (" + repo.OriginSourceDir + ")"},
			avoid: []string{"standalone", "skipped"},
		},
		"bundled, with a warning": {
			src:  repo.Source{RepoDir: "/cache/devexp/assets", Embedded: true, Origin: repo.OriginEmbedded, Warning: "checkout skipped: no marker"},
			want: []string{"checkout skipped: no marker", "Running standalone", "Asset root: /cache/devexp/assets (" + repo.OriginEmbedded + ")"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			out := captureStdout(t, func() { announceAssetRoot(tt.src) })
			at := 0
			for _, w := range tt.want {
				i := strings.Index(out[at:], w)
				if i < 0 {
					t.Fatalf("output lacks %q after offset %d:\n%s", w, at, out)
				}
				at += i + len(w)
			}
			for _, a := range tt.avoid {
				if strings.Contains(out, a) {
					t.Errorf("output contains %q:\n%s", a, out)
				}
			}
		})
	}
}

// TestDoInstallOpencode_UnmergeableConfig (#157 review): opencode reads
// config.json as JSONC, so a commented one is valid. The MCP merge still can't
// edit it safely: it is left byte for byte, the servers to add by hand are
// named, and agents, skills and hooks install anyway, in a dry run too. With
// --mcps-only there is nothing else to do, so it stays an error.
func TestDoInstallOpencode_UnmergeableConfig(t *testing.T) {
	const jsonc = "{\n  // my model\n  \"model\": \"x\",\n}\n"
	setup := func(t *testing.T) (repoDir, home, configPath string) {
		home = t.TempDir()
		t.Setenv("HOME", home)
		repoDir = writeOpencodeHookRepo(t)
		os.WriteFile(filepath.Join(repoDir, "mcps", "registry.json"), []byte(`[{"name": "context7", "command": "npx"}, {"name": "remote", "transport": "http", "url": "https://example.com"}]`), 0o644) //nolint:errcheck
		os.MkdirAll(filepath.Join(repoDir, "agents"), 0o755)                                                                                                                                            //nolint:errcheck
		os.WriteFile(filepath.Join(repoDir, "agents", "helper.md"), []byte("---\nname: helper\n---\n# H\n"), 0o644)                                                                                     //nolint:errcheck
		configPath = filepath.Join(home, ".config", "opencode", "config.json")
		os.MkdirAll(filepath.Dir(configPath), 0o755)   //nolint:errcheck
		os.WriteFile(configPath, []byte(jsonc), 0o644) //nolint:errcheck
		return repoDir, home, configPath
	}

	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("full install dryRun=%v", dryRun), func(t *testing.T) {
			repoDir, home, configPath := setup(t)
			var err error
			out := captureStdout(t, func() {
				err = doInstallOpencode(&installOpts{repoDir: repoDir, cfg: &config.Config{}, env: map[string]string{}, dryRun: dryRun})
			})
			if err != nil {
				t.Fatalf("doInstallOpencode() error = %v\n%s", err, out)
			}
			if got, _ := os.ReadFile(configPath); string(got) != jsonc {
				t.Errorf("config.json changed:\n%s", got)
			}
			for _, want := range []string{"MCP servers skipped", "not valid JSON", "left untouched", "context7, remote", "opencode installation complete"} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			agent := filepath.Join(home, ".config", "opencode", "agents", "helper.md")
			plugin := filepath.Join(home, ".config", "opencode", "plugins", "devexp.js")
			for _, p := range []string{agent, plugin} {
				if _, statErr := os.Stat(p); dryRun != os.IsNotExist(statErr) {
					t.Errorf("%s exists = %v, want %v", p, statErr == nil, !dryRun)
				}
			}
			if dryRun && !strings.Contains(out, "helper.md") {
				t.Errorf("dry run doesn't preview the agent:\n%s", out)
			}
		})
	}

	t.Run("--mcps-only returns the error", func(t *testing.T) {
		repoDir, _, configPath := setup(t)
		var err error
		out := captureStdout(t, func() {
			err = doInstallOpencode(&installOpts{repoDir: repoDir, cfg: &config.Config{}, env: map[string]string{}, mcpsOnly: true})
		})
		var refused *mcp.ConfigRefusedError
		if !errors.As(err, &refused) || !strings.Contains(err.Error(), "left untouched") {
			t.Errorf("doInstallOpencode() error = %v, want the config refusal\n%s", err, out)
		}
		if got, _ := os.ReadFile(configPath); string(got) != jsonc {
			t.Errorf("config.json changed")
		}
	})

	t.Run("other MCP errors still stop the install", func(t *testing.T) {
		repoDir, _, _ := setup(t)
		os.WriteFile(filepath.Join(repoDir, "mcps", "registry.json"), []byte(`{not a registry`), 0o644) //nolint:errcheck
		var err error
		captureStdout(t, func() {
			err = doInstallOpencode(&installOpts{repoDir: repoDir, cfg: &config.Config{}, env: map[string]string{}})
		})
		if err == nil {
			t.Errorf("doInstallOpencode() = nil, want the registry error")
		}
	})
}
