package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"devexp/internal/hooks"
	"devexp/internal/mcp"
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
