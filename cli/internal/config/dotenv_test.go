package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDotenv(t *testing.T) {
	tests := map[string]struct {
		content string
		want    map[string]string
	}{
		"basic key=value pairs": {
			content: "FOO=bar\nBAZ=qux",
			want:    map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		"comments and blank lines skipped": {
			content: "# c\n\nFOO=bar\n  \n# c2",
			want:    map[string]string{"FOO": "bar"},
		},
		"whitespace trimmed from key and value": {
			content: "  FOO = bar  ",
			want:    map[string]string{"FOO": "bar"},
		},
		"line with no equals sign is skipped": {
			content: "FOO=bar\nNOEQUALS\nBAZ=qux",
			want:    map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		"value containing equals sign": {
			content: "URL=https://x.com?a=b",
			want:    map[string]string{"URL": "https://x.com?a=b"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".env")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			got, err := LoadDotenv(path)
			if err != nil {
				t.Fatalf("LoadDotenv() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadDotenv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadDotenv_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.env")

	got, err := LoadDotenv(path)
	if err == nil {
		t.Fatal("LoadDotenv() error = nil, want non-nil error")
	}
	if got != nil {
		t.Errorf("LoadDotenv() = %v, want nil", got)
	}
}
