package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"devexp/internal/ui"
)

// ── opencode plugin install ───────────────────────────────────────────────────
//
// opencode auto-loads every *.js/*.ts file directly inside its plugins/
// directory (the glob is non-recursive) and calls every export of each one as
// a plugin. So devexp installs exactly one top-level file, the entry, and puts
// everything it composes one level down where the loader never looks:
//
//	<plugins>/devexp.js            hooks/opencode/devexp-plugin.js
//	<plugins>/devexp/utils.js      shared helpers the modules import
//	<plugins>/devexp/package.json  {"type":"module"} for the modules
//	<plugins>/devexp/<module>.js   one per selected hook
//	<plugins>/devexp/hooks.json    the selection the entry reads
//
// Files are copied, not referenced: a binary install's repo dir is a cache
// that is wiped whenever a new version is extracted. They are copied by the
// registry list, never by glob, so *.test.js never ships. And nothing is ever
// written to <plugins>/package.json: opencode reads a package.json next to a
// plugin file and its main/exports can redirect the entry.

const (
	opencodeEntry     = "devexp.js"
	opencodeDir       = "devexp"
	opencodeSelection = "hooks.json"
	opencodeSrcDir    = "hooks/opencode"
	opencodeEntrySrc  = "devexp-plugin.js"
	legacyEntry       = "devexp-plugin.js"
)

// legacyNames is the closed set of files the pre-46ca772 bash installer copied
// flat into plugins/ (git ls-tree 46ca772^ hooks/opencode/). Nothing outside
// this set is ever considered legacy.
var legacyNames = []string{
	"devexp-plugin.js",
	"utils.js",
	"secret-guard.js",
	"secret-in-write-guard.js",
	"dangerous-cmd-guard.js",
	"large-file-guard.js",
	"lint-on-save.js",
	"format-on-save.js",
	"test-on-save.js",
}

// legacyPackageJSON is the exact content of the legacy flat package.json.
const legacyPackageJSON = `{ "type": "module" }`

// moduleFile is the file-name rule the entry enforces on hooks.json `module`
// values (hooks/opencode/devexp-plugin.js). The installer applies it before
// writing so a bad registry entry fails the install instead of the guard.
var moduleFile = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.js$`)

// SelectOpencode returns, in registry order, the hooks to install for
// opencode: those with an opencode module, enabled for opencode (the block's
// own enabled override, else the top-level flag) and not named in disabled.
func SelectOpencode(registry Registry, disabled []string) []Hook {
	off := make(map[string]bool, len(disabled))
	for _, d := range disabled {
		off[d] = true
	}
	var selected []Hook
	for _, h := range registry {
		spec, ok := h.Target(TargetOpencode)
		if !ok || spec.Module == "" || !h.EnabledFor(TargetOpencode) || off[h.Name] {
			continue
		}
		selected = append(selected, h)
	}
	return selected
}

// selectionEntry is one element of devexp/hooks.json. The key names are the
// contract the entry reads — exactly name, module, export and failClosed
// (camelCase, unlike the registry's fail_closed) — so none is omitempty.
type selectionEntry struct {
	Name       string `json:"name"`
	Module     string `json:"module"`
	Export     string `json:"export"`
	FailClosed bool   `json:"failClosed"`
}

// opencodeModuleBase validates a hook's opencode module path and returns the
// bare file name it is installed under. The module must sit directly in
// hooks/opencode/, pass the entry's file-name rule, and not be a test file.
func opencodeModuleBase(h Hook) (string, error) {
	spec, _ := h.Target(TargetOpencode)
	base := path.Base(spec.Module)
	switch {
	case path.Dir(spec.Module) != opencodeSrcDir:
		return "", fmt.Errorf("hook %q: opencode.module %q is not directly under %s/", h.Name, spec.Module, opencodeSrcDir)
	case !moduleFile.MatchString(base):
		return "", fmt.Errorf("hook %q: opencode.module %q is not a plain .js file name", h.Name, spec.Module)
	case strings.HasSuffix(base, ".test.js"):
		return "", fmt.Errorf("hook %q: opencode.module %q is a test file", h.Name, spec.Module)
	case base == "utils.js":
		return "", fmt.Errorf("hook %q: opencode.module %q collides with the shared utils.js", h.Name, spec.Module)
	case spec.Export == "":
		return "", fmt.Errorf("hook %q: opencode.export is empty", h.Name)
	}
	return base, nil
}

// opencodeSelectionJSON renders devexp/hooks.json for selected. An empty
// selection is an error: the entry treats [] as a broken install and blocks
// every tool call, so with nothing selected no plugin is installed at all.
func opencodeSelectionJSON(selected []Hook) ([]byte, error) {
	if len(selected) == 0 {
		return nil, errors.New("opencode selection is empty")
	}
	entries := make([]selectionEntry, 0, len(selected))
	for _, h := range selected {
		base, err := opencodeModuleBase(h)
		if err != nil {
			return nil, err
		}
		spec, _ := h.Target(TargetOpencode)
		entries = append(entries, selectionEntry{Name: h.Name, Module: base, Export: spec.Export, FailClosed: spec.FailClosed})
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// pluginFile is one installed file: Dest is relative to plugins/ (slash
// separated, as recorded in the manifest), Src relative to the repo root.
// Src is empty for the generated hooks.json.
type pluginFile struct {
	Dest string
	Src  string
}

// opencodePluginFiles lists every file the plugin consists of for selected,
// entry first. utils.js and package.json always ship with a non-empty
// selection: every module may import ./utils.js (the only relative import the
// entry's tests allow), and the modules need the ESM marker.
func opencodePluginFiles(selected []Hook) []pluginFile {
	if len(selected) == 0 {
		return nil
	}
	files := []pluginFile{
		{Dest: opencodeEntry, Src: path.Join(opencodeSrcDir, opencodeEntrySrc)},
		{Dest: path.Join(opencodeDir, "utils.js"), Src: path.Join(opencodeSrcDir, "utils.js")},
		{Dest: path.Join(opencodeDir, "package.json"), Src: path.Join(opencodeSrcDir, "package.json")},
	}
	for _, h := range selected {
		spec, _ := h.Target(TargetOpencode)
		files = append(files, pluginFile{Dest: path.Join(opencodeDir, path.Base(spec.Module)), Src: spec.Module})
	}
	return append(files, pluginFile{Dest: path.Join(opencodeDir, opencodeSelection)})
}

// hasDevexpHeader reports whether content opens with the comment header every
// devexp opencode file has carried since the first flat install:
//
//	/**
//	 * <name> — …
func hasDevexpHeader(name string, content []byte) bool {
	lines := strings.SplitN(string(content), "\n", 3)
	if len(lines) < 3 {
		return false
	}
	return strings.TrimSuffix(lines[0], "\r") == "/**" &&
		strings.HasPrefix(lines[1], " * "+name+" — ")
}

// isLegacyDevexpFile reports whether a file found directly in plugins/ was
// left there by the legacy flat install: its name is in the closed legacy set
// AND its content carries that file's devexp header. Both must hold, so a
// user's own utils.js is never taken for devexp's.
func isLegacyDevexpFile(name string, content []byte) bool {
	for _, n := range legacyNames {
		if n == name {
			return hasDevexpHeader(name, content)
		}
	}
	return false
}

// isDevexpEntry reports whether content is a devexp plugin entry. The entry is
// installed as devexp.js but keeps its source header (devexp-plugin.js).
func isDevexpEntry(content []byte) bool {
	return hasDevexpHeader(opencodeEntrySrc, content)
}

// InstallOpencode installs the devexp plugin for the hooks selected from
// registry into pluginsDir and returns the installed paths (relative to
// pluginsDir, entry first) for the manifest. With nothing selected it installs
// nothing and returns nil, so the caller's stale-file pass removes a previous
// plugin. It never writes outside devexp.js and devexp/, refuses to write
// through a symlink, and refuses to replace a devexp.js that is not a devexp
// entry.
func InstallOpencode(registry Registry, repoDir, pluginsDir string, disabled []string, dryRun bool) ([]string, error) {
	selected := SelectOpencode(registry, disabled)
	chosen := make(map[string]bool, len(selected))
	var names []string
	for _, h := range selected {
		chosen[h.Name] = true
		names = append(names, h.Name)
	}
	for _, h := range registry {
		if spec, ok := h.Target(TargetOpencode); ok && spec.Module != "" && !chosen[h.Name] {
			ui.Skipped(h.Name, "disabled")
		}
	}
	if len(selected) == 0 {
		ui.Skipped("opencode plugin", "every hook is disabled — nothing to install")
		return nil, nil
	}

	selection, err := opencodeSelectionJSON(selected)
	if err != nil {
		return nil, err
	}

	// Read every source and check every destination before writing anything,
	// so a missing source or a conflict leaves the existing plugin as it was.
	files := opencodePluginFiles(selected)
	contents := make([][]byte, len(files))
	for i, f := range files {
		if f.Src == "" {
			contents[i] = selection
		} else if contents[i], err = os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(f.Src))); err != nil {
			return nil, fmt.Errorf("opencode plugin source: %w", err)
		}
		if err := checkPluginDest(pluginsDir, f.Dest); err != nil {
			return nil, err
		}
	}

	dests := make([]string, len(files))
	for i, f := range files {
		dests[i] = f.Dest
	}

	if dryRun {
		for _, d := range dests {
			ui.DryRun("write " + filepath.Join(pluginsDir, filepath.FromSlash(d)))
		}
	} else {
		if err := os.MkdirAll(filepath.Join(pluginsDir, opencodeDir), 0755); err != nil {
			return nil, err
		}
		// Modules and hooks.json first, the entry last: the entry never points
		// at a selection whose modules aren't on disk yet.
		for i := 1; i <= len(files); i++ {
			idx := i % len(files)
			if err := writeIfChanged(pluginsDir, files[idx].Dest, contents[idx]); err != nil {
				return nil, err
			}
		}
	}

	ui.Success(fmt.Sprintf("opencode hooks (%d): %s", len(names), strings.Join(names, ", ")))
	return dests, nil
}

// checkPluginDest refuses destinations that are not plain files devexp may
// own: a symlink (writing would follow it out of plugins/), a directory, or a
// devexp.js that is not a devexp entry (it is someone else's plugin).
func checkPluginDest(pluginsDir, dest string) error {
	dir := filepath.Join(pluginsDir, opencodeDir)
	if fi, err := os.Lstat(dir); err == nil && !fi.IsDir() {
		return fmt.Errorf("%s exists and is not a directory — move it aside and re-run", dir)
	}
	p := filepath.Join(pluginsDir, filepath.FromSlash(dest))
	fi, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s exists and is not a regular file — move it aside and re-run", p)
	}
	if dest == opencodeEntry {
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !isDevexpEntry(content) {
			return fmt.Errorf("%s exists and is not a devexp plugin entry — move it aside and re-run", p)
		}
	}
	return nil
}

// writeIfChanged writes content to pluginsDir/dest unless it already holds
// exactly those bytes, reporting added or updated files.
func writeIfChanged(pluginsDir, dest string, content []byte) error {
	p := filepath.Join(pluginsDir, filepath.FromSlash(dest))
	existing, err := os.ReadFile(p)
	switch {
	case err == nil && bytes.Equal(existing, content):
		return nil
	case err == nil:
		if err := os.WriteFile(p, content, 0644); err != nil {
			return err
		}
		ui.Updated(dest)
	case os.IsNotExist(err):
		if err := os.WriteFile(p, content, 0644); err != nil {
			return err
		}
		ui.Added(dest)
	default:
		return err
	}
	return nil
}

// OwnedStalePlugins filters manifest-recorded stale plugin paths down to the
// ones devexp may delete: devexp.js (only while it still is a devexp entry) or
// a file directly in devexp/ with a name devexp installs. Anything else — a
// hand-edited manifest with ../, a nested path, a replaced entry — is kept and
// reported, never removed.
func OwnedStalePlugins(pluginsDir string, stale []string) []string {
	var owned []string
	for _, rel := range stale {
		if isOwnedPluginPath(rel) && (rel != opencodeEntry || entryStillDevexp(pluginsDir)) {
			owned = append(owned, rel)
			continue
		}
		ui.Warn(fmt.Sprintf("%s left untouched: listed in the manifest but not a devexp plugin file", filepath.Join(pluginsDir, filepath.FromSlash(rel))))
	}
	return owned
}

func isOwnedPluginPath(rel string) bool {
	if rel == opencodeEntry {
		return true
	}
	dir, base, ok := strings.Cut(rel, "/")
	if !ok || dir != opencodeDir {
		return false
	}
	return base == opencodeSelection || base == "package.json" || moduleFile.MatchString(base)
}

// entryStillDevexp reports whether devexp.js may be removed: it is absent
// (removal is a no-op) or still a devexp entry.
func entryStillDevexp(pluginsDir string) bool {
	p := filepath.Join(pluginsDir, opencodeEntry)
	fi, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	content, err := os.ReadFile(p)
	return err == nil && isDevexpEntry(content)
}

// PruneOpencodeDir removes pluginsDir/devexp once it is empty. os.Remove
// refuses a non-empty directory, so anything left in it survives.
func PruneOpencodeDir(pluginsDir string, dryRun bool) {
	if dryRun {
		return
	}
	os.Remove(filepath.Join(pluginsDir, opencodeDir)) //nolint:errcheck
}

// ── Legacy flat install cleanup ───────────────────────────────────────────────

// CleanLegacyOpencode removes what the pre-46ca772 bash installer left behind:
// hook files copied flat into plugins/ (each would now load as a second
// plugin), its package.json, and the config.json `plugin` entry it registered
// (opencode logs a load failure for it once the file is gone).
//
// A file is removed only when its name is in the closed legacy set AND its
// content carries that file's devexp header; a same-named file without it is
// kept with a warning. package.json is removed only alongside a legacy match
// and only with the exact legacy content. The config entry is removed only on
// an exact string match, and every other byte of config.json is preserved.
func CleanLegacyOpencode(pluginsDir, configPath string, dryRun bool) error {
	matched := 0
	for _, name := range legacyNames {
		p := filepath.Join(pluginsDir, name)
		fi, err := os.Lstat(p)
		if err != nil {
			continue
		}
		var content []byte
		if fi.Mode().IsRegular() {
			content, _ = os.ReadFile(p)
		}
		if !fi.Mode().IsRegular() || !isLegacyDevexpFile(name, content) {
			ui.Warn(fmt.Sprintf("%s left untouched: same name as a legacy devexp file but not devexp content", p))
			continue
		}
		matched++
		if err := removeLegacy(p, name+" (legacy flat install)", dryRun); err != nil {
			return err
		}
	}

	if matched > 0 {
		p := filepath.Join(pluginsDir, "package.json")
		if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() {
			if content, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(content)) == legacyPackageJSON {
				if err := removeLegacy(p, "package.json (legacy flat install)", dryRun); err != nil {
					return err
				}
			}
		}
	}

	return removeLegacyConfigEntry(configPath, filepath.Join(pluginsDir, legacyEntry), dryRun)
}

func removeLegacy(p, label string, dryRun bool) error {
	if dryRun {
		ui.DryRun("remove " + p)
		return nil
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	ui.Removed(label)
	return nil
}

// removeLegacyConfigEntry drops every element of config.json's top-level
// `plugin` array that is exactly the string entry, and the key itself if the
// array ends up empty. The edit is spliced into the original bytes, so
// formatting, key order and every other value stay as they were. A missing,
// malformed or unexpected config is left untouched.
func removeLegacyConfigEntry(configPath, entry string, dryRun bool) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	var before map[string]any
	if err := json.Unmarshal(data, &before); err != nil {
		return nil
	}

	out, removed, err := spliceOutPluginEntry(data, entry)
	if err != nil || removed == 0 {
		return nil
	}

	// The splice must equal the semantic edit exactly, or nothing is written.
	var after map[string]any
	if err := json.Unmarshal(out, &after); err != nil {
		return fmt.Errorf("legacy plugin entry removal produced invalid JSON; %s left untouched", configPath)
	}
	var kept []any
	for _, v := range before["plugin"].([]any) {
		if s, ok := v.(string); !ok || s != entry {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		delete(before, "plugin")
	} else {
		before["plugin"] = kept
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("legacy plugin entry removal changed more than the entry; %s left untouched", configPath)
	}

	msg := fmt.Sprintf("plugin entry %s from %s", entry, configPath)
	if dryRun {
		ui.DryRun("remove " + msg)
		return nil
	}
	fi, err := os.Stat(configPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, out, fi.Mode().Perm()); err != nil {
		return err
	}
	ui.Removed(msg)
	return nil
}

// jsonMember is one member of a JSON object or array, as byte offsets into the
// scanned document: [start, end) covers the key (objects) through the value.
type jsonMember struct {
	key        string
	start, end int
	raw        json.RawMessage
}

// jsonContainer is a scanned object or array: its members plus the offsets
// just after the opening delimiter and at the closing one.
type jsonContainer struct {
	members        []jsonMember
	openEnd, close int
}

// scanJSONContainer scans the object or array that starts at data[offset]
// (after optional whitespace). Offsets in the result index into data.
func scanJSONContainer(data []byte, offset int) (jsonContainer, error) {
	var c jsonContainer
	dec := json.NewDecoder(bytes.NewReader(data[offset:]))
	tok, err := dec.Token()
	if err != nil {
		return c, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || (delim != '{' && delim != '[') {
		return c, errors.New("not a JSON object or array")
	}
	c.openEnd = offset + int(dec.InputOffset())
	for dec.More() {
		start := offset + int(dec.InputOffset())
		for start < len(data) && (data[start] == ',' || isJSONSpace(data[start])) {
			start++
		}
		var m jsonMember
		if delim == '{' {
			keyTok, err := dec.Token()
			if err != nil {
				return c, err
			}
			m.key, _ = keyTok.(string)
		}
		if err := dec.Decode(&m.raw); err != nil {
			return c, err
		}
		m.start, m.end = start, offset+int(dec.InputOffset())
		c.members = append(c.members, m)
	}
	c.close = offset + int(dec.InputOffset())
	if _, err := dec.Token(); err != nil {
		return c, err
	}
	return c, nil
}

func isJSONSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

// cutMember returns data with member i of c removed together with exactly one
// adjacent separator, leaving every other byte in place.
func cutMember(data []byte, c jsonContainer, i int) []byte {
	var from, to int
	switch {
	case len(c.members) == 1:
		from, to = c.openEnd, c.close
	case i > 0:
		from, to = c.members[i-1].end, c.members[i].end
	default:
		from, to = c.members[0].start, c.members[1].start
	}
	out := make([]byte, 0, len(data)-(to-from))
	out = append(out, data[:from]...)
	return append(out, data[to:]...)
}

// spliceOutPluginEntry removes, one at a time, each element of the top-level
// `plugin` array that is exactly the string entry — and the `plugin` member
// once its array is empty. It returns the edited bytes and how many elements
// it removed.
func spliceOutPluginEntry(data []byte, entry string) ([]byte, int, error) {
	removed := 0
	for {
		top, err := scanJSONContainer(data, 0)
		if err != nil {
			return nil, 0, err
		}
		pi := -1
		for i, m := range top.members {
			if m.key == "plugin" {
				pi = i
			}
		}
		if pi < 0 {
			return data, removed, nil
		}
		plugin := top.members[pi]
		valueStart := plugin.end - len(plugin.raw)
		arr, err := scanJSONContainer(data, valueStart)
		if err != nil {
			return data, removed, nil // `plugin` is not an array: not ours to edit
		}
		hit := -1
		for i, m := range arr.members {
			var s string
			if json.Unmarshal(m.raw, &s) == nil && s == entry {
				hit = i
				break
			}
		}
		if hit < 0 {
			return data, removed, nil
		}
		removed++
		if len(arr.members) == 1 {
			return cutMember(data, top, pi), removed, nil
		}
		data = cutMember(data, arr, hit)
	}
}
