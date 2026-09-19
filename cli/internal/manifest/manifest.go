// Package manifest tracks what devexp installed on a prior run — agent and
// skill files, opencode plugin files, and the MCP entries it wrote into a
// config file it shares with the user — so a later run can detect and remove
// what the toolkit no longer ships, and can tell its own entries from the
// user's.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"devexp/internal/fsutil"
)

// Manifest records the filenames/dirnames devexp installed for one target
// (Claude or opencode) on the most recent run of each category. Entries are
// the same names returned by the agents/skills installers (e.g.
// "code-reviewer.md" for an agent, "graphify" for a skill directory).
//
// Plugins is opencode-only: paths relative to the plugins directory, entry
// first (e.g. "devexp.js", "devexp/hooks.json").
//
// MCPs is Kimi-only: MCP server name to a fingerprint of the entry devexp
// wrote into mcp.json. Kimi has no `mcp add` command, so devexp edits a file
// the user also edits; the fingerprint is what tells devexp's own entry from
// one the user wrote or has since changed, so it never rewrites or removes
// theirs. A fingerprint rather than a copy of the entry: the entries hold
// resolved values from mcps/.env, and a second copy of a token is a second
// place to leak it from.
//
// Both are omitted when empty, so the Claude Code manifest keeps its exact
// shape.
type Manifest struct {
	Agents  []string          `json:"agents"`
	Skills  []string          `json:"skills"`
	Plugins []string          `json:"plugins,omitempty"`
	MCPs    map[string]string `json:"mcps,omitempty"`
}

// Load reads a manifest from path. It always returns a non-nil Manifest.
//
// A missing file is not an error: it returns an empty Manifest, the expected
// state on a first install or when upgrading from a devexp version that
// predates manifests.
//
// A file that exists but can't be read or parsed returns an empty Manifest
// together with the error, so the caller can warn and still install. The
// manifest is empty rather than whatever decoded before the error (a type
// mismatch leaves earlier fields filled in): an untrustworthy manifest must
// never mark files as stale, and an empty one marks none.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Manifest{}, nil
	}
	if err != nil {
		return &Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return &Manifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return &m, nil
}

// Save writes m to path as indented JSON, creating parent directories as
// needed. The write is atomic, and a symlinked manifest keeps its link while
// the file it points at is replaced (fsutil.WriteFileAtomic).
func Save(path string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data, 0644)
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
