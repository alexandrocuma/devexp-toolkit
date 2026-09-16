package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := map[string]struct {
		setup   func(t *testing.T, dir string) string // returns manifest path
		want    *Manifest
		wantErr bool
	}{
		"missing file returns empty manifest": {
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "missing.json")
			},
			want: &Manifest{},
		},
		"valid JSON round-trips agents and skills": {
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "manifest.json")
				data := `{"agents":["a.md","b.md"],"skills":["graphify"]}`
				if err := os.WriteFile(path, []byte(data), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			want: &Manifest{Agents: []string{"a.md", "b.md"}, Skills: []string{"graphify"}},
		},
		"manifest without plugins loads with nil Plugins": {
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "manifest.json")
				data := `{"agents":["a.md"],"skills":[]}`
				if err := os.WriteFile(path, []byte(data), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			want: &Manifest{Agents: []string{"a.md"}, Skills: []string{}},
		},
		"plugins round-trip in order": {
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "manifest.json")
				data := `{"agents":null,"skills":null,"plugins":["devexp.js","devexp/hooks.json"]}`
				if err := os.WriteFile(path, []byte(data), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			want: &Manifest{Plugins: []string{"devexp.js", "devexp/hooks.json"}},
		},
		"malformed JSON returns an empty manifest and the error": {
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "bad.json")
				if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			want:    &Manifest{},
			wantErr: true,
		},
		// A type mismatch still fills the fields decoded before and after it;
		// none of that partial data may be trusted.
		"type mismatch discards partially decoded fields": {
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "partial.json")
				data := `{"agents":["mine.md"],"skills":{"x":1},"plugins":["devexp/old.js"]}`
				if err := os.WriteFile(path, []byte(data), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			want:    &Manifest{},
			wantErr: true,
		},
		"unreadable path returns an empty manifest and the error": {
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "manifest.json")
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
				return path
			},
			want:    &Manifest{},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setup(t, dir)

			got, err := Load(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Callers dereference the result even on error.
			if got == nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSave(t *testing.T) {
	tests := map[string]struct {
		manifest *Manifest
	}{
		"writes and round-trips agents and skills": {
			manifest: &Manifest{Agents: []string{"a.md", "b.md"}, Skills: []string{"graphify", "deliver"}},
		},
		"writes empty manifest": {
			manifest: &Manifest{},
		},
		"writes and round-trips plugins": {
			manifest: &Manifest{Agents: []string{"a.md"}, Skills: []string{"graphify"}, Plugins: []string{"devexp.js", "devexp/utils.js"}},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "nested", "manifest.json")

			if err := Save(path, tt.manifest); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.manifest) {
				t.Errorf("round-trip = %+v, want %+v", got, tt.manifest)
			}
		})
	}
}

// A manifest with no plugins must stay byte-for-byte what it was before the
// field existed: the Claude Code manifest never has plugins.
func TestSave_OmitsNilPlugins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := Save(path, &Manifest{Agents: []string{"a.md"}, Skills: []string{"graphify"}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := "{\n  \"agents\": [\n    \"a.md\"\n  ],\n  \"skills\": [\n    \"graphify\"\n  ]\n}"
	if string(data) != want {
		t.Errorf("Save() wrote %q, want %q", data, want)
	}
}

func TestStale(t *testing.T) {
	tests := map[string]struct {
		old     []string
		newList []string
		want    []string
	}{
		"entries in old not in new are returned": {
			old:     []string{"a.md", "b.md", "c.md"},
			newList: []string{"a.md"},
			want:    []string{"b.md", "c.md"},
		},
		"entries in both old and new are not returned": {
			old:     []string{"a.md", "b.md"},
			newList: []string{"a.md", "b.md"},
			want:    nil,
		},
		"new entries not in old do not appear in result": {
			old:     []string{"a.md"},
			newList: []string{"a.md", "b.md"},
			want:    nil,
		},
		"empty old returns nil": {
			old:     nil,
			newList: []string{"a.md"},
			want:    nil,
		},
		"empty new returns all of old": {
			old:     []string{"a.md", "b.md"},
			newList: nil,
			want:    []string{"a.md", "b.md"},
		},
		"nil inputs handled without panic": {
			old:     nil,
			newList: nil,
			want:    nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := Stale(tt.old, tt.newList)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Stale() = %v, want %v", got, tt.want)
			}
		})
	}
}
