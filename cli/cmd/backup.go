package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"devexp/internal/manifest"
	"devexp/internal/skills"
	"devexp/internal/ui"
)

// backupExisting copies pre-existing files matching glob into backupDir.
func backupExisting(dir, pattern, backupDir string, dryRun bool) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 || dryRun {
		return
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return
	}
	for _, src := range matches {
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		os.WriteFile(filepath.Join(backupDir, filepath.Base(src)), data, 0644) //nolint:errcheck
	}
}

// backupExistingDirs copies pre-existing skill directories (immediate
// subdirectories of dir containing a SKILL.md) into backupDir/<name>/.
func backupExistingDirs(dir, backupDir string, dryRun bool) {
	if dryRun {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		srcDir := filepath.Join(dir, name)
		if _, err := os.Stat(filepath.Join(srcDir, "SKILL.md")); err != nil {
			continue
		}
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return
		}
		skills.CopyDir(srcDir, filepath.Join(backupDir, name)) //nolint:errcheck
	}
}

// loadOldManifest reads the previous run's manifest. An unreadable or corrupt
// manifest is reported and treated as empty: the install carries on, nothing
// counts as stale on this run (an empty old list marks nothing for removal),
// and the manifest is rewritten at the end as usual.
func loadOldManifest(path string) *manifest.Manifest {
	old, err := manifest.Load(path)
	if err != nil {
		ui.Warn(fmt.Sprintf("manifest %s is unreadable, so no stale files are removed this run: %v", path, err))
	}
	return old
}

// removeStale removes each entry in stale from dir via removeFn (file or
// dir), reporting via ui. In dry-run mode it only reports what would be
// removed.
func removeStale(dir string, stale []string, removeFn func(path string) error, dryRun bool) {
	for _, name := range stale {
		path := filepath.Join(dir, name)
		if dryRun {
			ui.DryRun(fmt.Sprintf("remove %s (no longer in this release)", path))
			continue
		}
		if err := removeFn(path); err != nil && !os.IsNotExist(err) {
			ui.Warn(fmt.Sprintf("remove %s: %v", path, err))
			continue
		}
		ui.Removed(name)
	}
}
