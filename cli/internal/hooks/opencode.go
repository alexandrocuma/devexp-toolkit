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
	"syscall"

	"devexp/internal/removeguard"
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

// InstallOpencode brings pluginsDir in line with the hooks selected from
// registry: it writes the plugin for the selection, removes the plugin files
// devexp no longer installs, and returns the paths (relative to pluginsDir,
// entry first) the manifest should record. recorded is the previous manifest's
// plugins list — nil on a first install or when the manifest was unreadable.
//
// Nothing changes on disk until every check has passed:
//   - plugins/ and plugins/devexp/ must be real directories when they exist.
//     Writing or removing through a symlinked one would reach files outside
//     plugins/, such as a source checkout.
//   - every source must be readable, and no destination may be a symlink or a
//     directory, or a devexp.js devexp doesn't own.
//
// Files are written atomically (temp file + rename).
//
// The files devexp may remove are the recorded list plus the devexp files it
// recognises on disk (ownedOnDisk), so a lost manifest can't leave a plugin
// installed for good. Removing the whole plugin is all-or-nothing: when
// devexp.js has to stay, devexp/ stays with it, because an entry without its
// hooks.json blocks every opencode tool call.
func InstallOpencode(registry Registry, repoDir, pluginsDir string, disabled, recorded []string, dryRun bool) ([]string, error) {
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

	var files []pluginFile
	var contents [][]byte
	if len(selected) > 0 {
		selection, err := opencodeSelectionJSON(selected)
		if err != nil {
			return nil, err
		}
		files = opencodePluginFiles(selected)
		contents = make([][]byte, len(files))
		for i, f := range files {
			if f.Src == "" {
				contents[i] = selection
			} else if contents[i], err = os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(f.Src))); err != nil {
				return nil, fmt.Errorf("opencode plugin source: %w", err)
			}
		}
	}
	dests := make([]string, len(files))
	for i, f := range files {
		dests[i] = f.Dest
	}
	stale := stalePlugins(pluginsDir, registry, recorded, dests)

	if len(files) == 0 && len(stale) == 0 {
		ui.Skipped("opencode plugin", "every hook is disabled — nothing to install")
		return nil, nil
	}
	if _, err := checkPluginRoots(pluginsDir); err != nil {
		return nil, err
	}
	for _, f := range files {
		if err := checkPluginDest(pluginsDir, f.Dest, recorded); err != nil {
			return nil, err
		}
	}

	if len(files) == 0 {
		// Only removals are left. Uninstall removes the plugin through the same
		// helper, so the two cannot drift apart.
		if entryKeepReason(pluginsDir, stale) == "" {
			ui.Skipped("opencode plugin", "every hook is disabled — nothing to install")
		}
		if kept := removeWholeOpencodePlugin(pluginsDir, stale, "no longer installed", dryRun); len(kept) > 0 {
			return kept, nil
		}
		return nil, nil
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

	kept := removePluginFiles(pluginsDir, stale, "no longer installed", dryRun)
	ui.Success(fmt.Sprintf("opencode hooks (%d): %s", len(names), strings.Join(names, ", ")))
	return append(dests, kept...), nil
}

// removeWholeOpencodePlugin removes stale, the files of a plugin that is no
// longer installed at all, once the roots have been checked. It is all-or-
// nothing on the entry: if a devexp.js on disk has to stay, everything stays,
// because an entry without its hooks.json blocks every opencode tool call.
// devexp/ is pruned once nothing was kept. It returns the paths it kept, which
// the manifest should go on recording. reason labels the dry-run lines.
func removeWholeOpencodePlugin(pluginsDir string, stale []string, reason string, dryRun bool) []string {
	if why := entryKeepReason(pluginsDir, stale); why != "" {
		ui.Warn(fmt.Sprintf("opencode plugin left installed and its hooks remain active: %s is %s, so devexp.js and devexp/ are both kept (removing only devexp/ would block every tool call). Move devexp.js aside and re-run to remove the plugin.",
			filepath.Join(pluginsDir, opencodeEntry), why))
		return stale
	}
	kept := removePluginFiles(pluginsDir, stale, reason, dryRun)
	if len(kept) == 0 {
		pruneOpencodeDir(pluginsDir, dryRun)
	}
	return kept
}

// UninstallOpencode removes devexp's opencode plugin by install's rules: the
// recorded paths devexp owns plus the devexp files recognised on disk, never
// anything through a symlinked plugins/, and all-or-nothing on the entry. It
// returns the plugin paths still on disk, which the manifest should keep
// recording. An error means the roots were refused and nothing was removed.
//
// registry may be nil: ownership by path needs none, and without it only the
// modules a registry would name go unrecognised on disk.
func UninstallOpencode(registry Registry, pluginsDir string, recorded []string, dryRun bool) ([]string, error) {
	stale := stalePlugins(pluginsDir, registry, recorded, nil)
	if len(stale) == 0 {
		ui.Skipped("opencode plugin", "not installed")
		return nil, nil
	}
	if _, err := checkPluginRoots(pluginsDir); err != nil {
		return stale, err
	}
	if kept := removeWholeOpencodePlugin(pluginsDir, stale, "uninstall", dryRun); len(kept) > 0 {
		return kept, nil
	}
	return nil, nil
}

// checkPluginRoots checks the two directories every plugin write and removal
// happens in, and reports whether plugins/ is a symlink or behind one.
//
//   - plugins/ may be a symlink to a directory (a dotfiles setup), or sit under
//     a symlinked ~/.config or ~/.config/opencode (removeguard.BehindSymlink,
//     resolved from pluginsHome). devexp writes through it, but never removes
//     anything through it (linked=true; the removal paths check this). One
//     that can't be resolved counts as linked too.
//   - plugins/ that is a dangling link, a link to a non-directory, or not a
//     directory at all is refused.
//   - plugins/devexp/ must be a real directory when it exists. A symlink is
//     refused outright: it may point at a source checkout, and even writing
//     there would overwrite its files.
func checkPluginRoots(pluginsDir string) (linked bool, err error) {
	fi, err := os.Lstat(pluginsDir)
	switch {
	case os.IsNotExist(err):
		return false, nil
	case err != nil:
		return false, err
	case fi.Mode()&os.ModeSymlink != 0:
		target, err := os.Stat(pluginsDir)
		if err != nil || !target.IsDir() {
			return false, fmt.Errorf("%s is a symlink that doesn't point at a directory — fix or remove the link and re-run", pluginsDir)
		}
		linked = true
	case !fi.IsDir():
		return false, fmt.Errorf("%s exists and is not a directory — move it aside and re-run", pluginsDir)
	default:
		_, behind, err := behindSymlink(pluginsHome(pluginsDir), pluginsDir)
		linked = behind || err != nil
	}

	dir := filepath.Join(pluginsDir, opencodeDir)
	fi, err = os.Lstat(dir)
	switch {
	case os.IsNotExist(err):
		return linked, nil
	case err != nil:
		return false, err
	case fi.Mode()&os.ModeSymlink != 0:
		return false, fmt.Errorf("%s is a symlink — devexp writes and removes opencode plugin files only in a real directory; replace the link with a directory and re-run", dir)
	case !fi.IsDir():
		return false, fmt.Errorf("%s exists and is not a directory — move it aside and re-run", dir)
	}
	return linked, nil
}

// behindSymlink is removeguard.BehindSymlink; tests swap it to inject errors.
var behindSymlink = removeguard.BehindSymlink

// pluginsHome is the home directory pluginsDir is resolved from when checking
// whether it is behind a symlink. devexp installs plugins only at
// <home>/.config/opencode/plugins (cli/cmd/paths.go); for any other path only
// plugins/ itself is checked.
func pluginsHome(pluginsDir string) string {
	suffix := string(filepath.Separator) + filepath.Join(".config", "opencode", "plugins")
	if home, ok := strings.CutSuffix(filepath.Clean(pluginsDir), suffix); ok && home != "" {
		return home
	}
	return filepath.Dir(pluginsDir)
}

// warnLeftBehind reports files devexp would have removed but didn't, because
// plugins/ is a symlink or behind one, and nothing is ever removed through one.
func warnLeftBehind(pluginsDir string, paths []string) {
	why := "is a symlink"
	if !removeguard.IsSymlink(pluginsDir) {
		why = "is behind a symlink"
		if resolved, err := filepath.EvalSymlinks(pluginsDir); err == nil {
			why += fmt.Sprintf(" (it resolves to %s)", resolved)
		}
	}
	ui.Warn(fmt.Sprintf("%s %s — devexp never removes files through it; remove these by hand: %s", pluginsDir, why, strings.Join(paths, ", ")))
}

// checkPluginDest refuses destinations that are not plain files devexp may
// own: a symlink, a directory, or a devexp.js that is neither a devexp entry
// nor recorded in the manifest (then it is someone else's plugin). A recorded
// devexp.js is replaced even when its header is gone, so a damaged entry can
// be repaired by re-installing.
func checkPluginDest(pluginsDir, dest string, recorded []string) error {
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
	if dest == opencodeEntry && !contains(recorded, opencodeEntry) {
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

// writeIfChanged atomically writes content to pluginsDir/dest unless it
// already holds exactly those bytes, reporting added or updated files.
func writeIfChanged(pluginsDir, dest string, content []byte) error {
	p := filepath.Join(pluginsDir, filepath.FromSlash(dest))
	existing, err := os.ReadFile(p)
	switch {
	case err == nil && bytes.Equal(existing, content):
		return nil
	case err == nil:
		if err := writeFileAtomic(p, content, 0644); err != nil {
			return err
		}
		ui.Updated(dest)
	case os.IsNotExist(err):
		if err := writeFileAtomic(p, content, 0644); err != nil {
			return err
		}
		ui.Added(dest)
	default:
		return err
	}
	return nil
}

// writeFileAtomic replaces path with data through a temp file in the same
// directory and a rename. An interrupted write leaves the old file or the new
// one, never a truncated one, and the rename replaces whatever is at path
// instead of writing through it. The temp file is removed on any failure.
func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			f.Close()      //nolint:errcheck
			os.Remove(tmp) //nolint:errcheck
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Chmod(perm); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// stalePlugins lists the plugin files devexp may remove because this run
// doesn't install them: recorded paths plus ownedOnDisk, minus keep, entry
// first. A recorded path devexp never installs — a hand-edited manifest with
// ../, a nested path, a foreign name — is reported and never returned.
func stalePlugins(pluginsDir string, registry Registry, recorded, keep []string) []string {
	skip := make(map[string]bool, len(keep))
	for _, k := range keep {
		skip[k] = true
	}
	var out []string
	add := func(rel string) {
		if skip[rel] {
			return
		}
		skip[rel] = true
		if rel == opencodeEntry {
			out = append([]string{rel}, out...)
		} else {
			out = append(out, rel)
		}
	}
	for _, rel := range recorded {
		if skip[rel] {
			continue
		}
		if !isOwnedPluginPath(rel) {
			skip[rel] = true
			ui.Warn(fmt.Sprintf("%s left untouched: listed in the manifest but not a devexp plugin file", filepath.Join(pluginsDir, filepath.FromSlash(rel))))
			continue
		}
		add(rel)
	}
	for _, rel := range ownedOnDisk(pluginsDir, registry) {
		add(rel)
	}
	return out
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

// ownedOnDisk finds the devexp plugin files present in pluginsDir without the
// manifest: devexp.js while it is a devexp entry, and — only inside a real
// devexp/ directory — hooks.json, package.json, utils.js and every registry
// module name. It lets a run whose manifest was lost still remove the plugin.
func ownedOnDisk(pluginsDir string, registry Registry) []string {
	var out []string
	entry := filepath.Join(pluginsDir, opencodeEntry)
	if fi, err := os.Lstat(entry); err == nil && fi.Mode().IsRegular() {
		if content, err := os.ReadFile(entry); err == nil && isDevexpEntry(content) {
			out = append(out, opencodeEntry)
		}
	}
	dir := filepath.Join(pluginsDir, opencodeDir)
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return out
	}
	names := []string{"utils.js", "package.json"}
	for _, h := range registry {
		if spec, ok := h.Target(TargetOpencode); ok && spec.Module != "" {
			if base, err := opencodeModuleBase(h); err == nil {
				names = append(names, base)
			}
		}
	}
	names = append(names, opencodeSelection)
	for _, n := range names {
		if fi, err := os.Lstat(filepath.Join(dir, n)); err == nil && fi.Mode().IsRegular() {
			out = append(out, path.Join(opencodeDir, n))
		}
	}
	return out
}

// entryKeepReason says why the devexp.js on disk must stay, or "" when there
// is none or it may be removed with the rest of stale. Whenever it must stay,
// devexp/ stays with it: removing only devexp/ would leave an entry that
// blocks every opencode tool call.
//
//   - a symlink is someone's own setup and is never removed;
//   - a non-regular or unreadable one can't be removed safely;
//   - a regular one that isn't in stale is not recognised as devexp's: it is
//     neither recorded in the manifest nor carrying the devexp header. That is
//     almost always devexp's own entry, edited (a `// @ts-check` line, a BOM),
//     and keeping its devexp/ costs at worst some clutter.
func entryKeepReason(pluginsDir string, stale []string) string {
	fi, err := os.Lstat(filepath.Join(pluginsDir, opencodeEntry))
	switch {
	case os.IsNotExist(err):
		return ""
	case err != nil:
		return "unreadable"
	case fi.Mode()&os.ModeSymlink != 0:
		return "a symlink"
	case !fi.Mode().IsRegular():
		return "not a regular file"
	case !contains(stale, opencodeEntry):
		return "not recognised as devexp's (not recorded in the manifest, no devexp header)"
	}
	return ""
}

// removePluginFiles removes stale plugin files, entry first, and returns the
// ones it had to keep. If the entry can't be removed nothing else is, so the
// plugin never ends up as an entry without its hooks.json. Only regular files
// are removed. plugins/ and devexp/ are re-checked right before, and nothing
// is removed through a symlinked plugins/: every file is kept (and stays
// recorded, so a run after the link is replaced can clean up).
func removePluginFiles(pluginsDir string, stale []string, reason string, dryRun bool) []string {
	if len(stale) == 0 {
		return nil
	}
	linked, err := checkPluginRoots(pluginsDir)
	if err != nil {
		ui.Warn(fmt.Sprintf("opencode plugin files left untouched: %v", err))
		return stale
	}
	if linked {
		paths := make([]string, len(stale))
		for i, rel := range stale {
			paths[i] = filepath.Join(pluginsDir, filepath.FromSlash(rel))
		}
		warnLeftBehind(pluginsDir, paths)
		return stale
	}
	if dryRun {
		for _, rel := range stale {
			ui.DryRun(fmt.Sprintf("remove %s (%s)", filepath.Join(pluginsDir, filepath.FromSlash(rel)), reason))
		}
		return nil
	}
	var kept []string
	for i, rel := range stale {
		p := filepath.Join(pluginsDir, filepath.FromSlash(rel))
		fi, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err == nil && !fi.Mode().IsRegular() {
			err = errors.New("not a regular file")
		}
		if err == nil {
			err = os.Remove(p)
		}
		if err != nil && !os.IsNotExist(err) {
			if rel == opencodeEntry {
				ui.Warn(fmt.Sprintf("opencode plugin left installed and its hooks remain active: could not remove %s (%v)", p, err))
				return stale[i:]
			}
			ui.Warn(fmt.Sprintf("%s left untouched: %v", p, err))
			kept = append(kept, rel)
			continue
		}
		ui.Removed(rel)
	}
	return kept
}

// pruneOpencodeDir removes pluginsDir/devexp once it is empty, and only when
// it is a real directory: os.Remove on a symlink deletes the link whatever it
// points at, while on a directory it refuses unless the directory is empty.
func pruneOpencodeDir(pluginsDir string, dryRun bool) {
	if linked, err := checkPluginRoots(pluginsDir); err != nil || linked {
		return // never remove through a symlinked plugins/, or one behind a symlink
	}
	dir := filepath.Join(pluginsDir, opencodeDir)
	if fi, err := os.Lstat(dir); dryRun || err != nil || !fi.IsDir() {
		return
	}
	os.Remove(dir) //nolint:errcheck
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
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
	// Nothing is removed through a symlinked plugins/: it can point at a source
	// checkout whose hooks/opencode files carry exactly the legacy names and
	// headers. Matches are reported instead.
	linked, err := checkPluginRoots(pluginsDir)
	if err != nil {
		return fmt.Errorf("legacy flat install not cleaned up: %w", err)
	}
	var legacy []string
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
		legacy = append(legacy, p)
	}

	if matched > 0 {
		p := filepath.Join(pluginsDir, "package.json")
		if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() {
			if content, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(content)) == legacyPackageJSON {
				legacy = append(legacy, p)
			}
		}
	}

	if linked && len(legacy) > 0 {
		warnLeftBehind(pluginsDir, legacy)
	} else {
		for _, p := range legacy {
			if err := removeLegacy(p, filepath.Base(p)+" (legacy flat install)", dryRun); err != nil {
				return err
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

// accessWriteOK is access(2)'s W_OK: whether the current user may write a path.
const accessWriteOK = 0x2

// removeLegacyConfigEntry drops every element of config.json's top-level
// `plugin` array that is exactly the string entry, and the key itself if the
// array ends up empty. The edit is spliced into the original bytes, so
// formatting, key order and every other value stay as they were. A missing,
// malformed or unexpected config is left untouched.
func removeLegacyConfigEntry(configPath, entry string, dryRun bool) error {
	fi, err := os.Lstat(configPath)
	if err != nil {
		return nil
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		// Rewriting would either follow the link or replace it with a plain
		// file; both change someone's dotfiles setup behind their back.
		if data, err := os.ReadFile(configPath); err == nil {
			if _, removed, err := spliceOutPluginEntry(data, entry); err == nil && removed > 0 {
				ui.Warn(fmt.Sprintf("%s is a symlink, so it was left untouched — remove the plugin entry %q from it by hand", configPath, entry))
			}
		}
		return nil
	}
	if !fi.Mode().IsRegular() {
		return nil
	}
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

	// A config.json the user made read-only stays read-only. The atomic rename
	// would succeed in a writable directory and silently replace it; the
	// uninstall.sh MCP step applies the same rule.
	if syscall.Access(configPath, accessWriteOK) != nil {
		ui.Warn(fmt.Sprintf("%s is not writable, so it was left untouched — remove the plugin entry %q from it by hand", configPath, entry))
		return nil
	}

	msg := fmt.Sprintf("plugin entry %s from %s", entry, configPath)
	if dryRun {
		ui.DryRun("remove " + msg)
		return nil
	}
	if err := writeFileAtomic(configPath, out, fi.Mode().Perm()); err != nil {
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
