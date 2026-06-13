// Package manifest tracks which agent and skill files devexp installed on a
// prior run, so a later run can detect and remove files that are no longer
// shipped by the toolkit (stale files left over from an older version).
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Manifest records the filenames/dirnames devexp installed for one target
// (Claude or opencode) on the most recent run of each category. Entries are
// the same names returned by the agents/skills installers (e.g.
// "code-reviewer.md" for an agent, "graphify" for a skill directory).
type Manifest struct {
	Agents []string `json:"agents"`
	Skills []string `json:"skills"`
}

// Load reads a manifest from path. A missing file is not an error — it
// returns an empty Manifest, which is the expected state on a first install
// or when upgrading from a devexp version that predates manifests. A
// malformed file is tolerated the same way, so a corrupt cache file never
// blocks install.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return &Manifest{}, nil
	}
	return &m, nil
}

// Save writes m to path as indented JSON, creating parent directories as
// needed.
func Save(path string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Stale returns entries present in old but not in newList — i.e. files that
// were installed previously but are not part of this run's install set, and
// should be removed from the target directory.
func Stale(old, newList []string) []string {
	keep := make(map[string]bool, len(newList))
	for _, n := range newList {
		keep[n] = true
	}
	var stale []string
	for _, n := range old {
		if !keep[n] {
			stale = append(stale, n)
		}
	}
	return stale
}
