package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := map[string]struct {
		setup func(t *testing.T, dir string) string // returns manifest path
		want  *Manifest
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
		"malformed JSON returns empty manifest": {
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "bad.json")
				if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			want: &Manifest{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setup(t, dir)

			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
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
