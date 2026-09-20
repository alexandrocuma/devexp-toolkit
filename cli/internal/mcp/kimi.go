package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"devexp/internal/fsutil"
	"devexp/internal/ui"
)

// ── Kimi Code CLI ─────────────────────────────────────────────────────────────
//
// Kimi has no `kimi mcp add`: its MCP servers are the "mcpServers" object of
// $KIMI_CODE_HOME/mcp.json, which devexp merges into. Three facts about that
// file decide everything below, each verified by running Kimi Code CLI 2.0.1
// against a scratch KIMI_CODE_HOME and reading its log:
//
//   - One entry Kimi rejects costs the user every MCP server in the file, not
//     just that one ("mcp config initial load failed"), and the failure is only
//     logged — in the session it just looks like no MCP server exists. So every
//     entry devexp writes is validated against Kimi's schema first, the whole
//     rendered file is re-checked before it replaces anything, and a file
//     devexp cannot parse is refused rather than overwritten.
//   - Kimi expands nothing: no ${VAR}, no $VAR, no ~ ("spawn ${SHELL} ENOENT").
//     Every value is resolved at install time (resolve.go).
//   - mcp.json is strict JSON, not JSONC — a comment is a hard parse failure
//     for Kimi too, which is why refusing to touch a file devexp can't parse
//     costs the user nothing they had.
//
// The output format — two-space indent, trailing newline, every unrelated
// top-level key kept as it was — is the format Kimi's own writer produces.
//
// Every path this file prints or puts in an error is quoted: it is derived
// from $KIMI_CODE_HOME, and unquoted, an embedded newline forges a line of
// devexp output and an escape sequence reaches the terminal.

// kimiEntry is one mcpServers member as devexp writes it.
//
// transport is always written out even though Kimi infers it from command or
// url, so an entry devexp wrote cannot change meaning if that inference does.
// (It infers "http" from a url, never "sse", so sse has to be explicit anyway.)
type kimiEntry struct {
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

func kimiEntryFor(r resolved) kimiEntry {
	e := kimiEntry{Transport: r.transport}
	if r.transport == "http" || r.transport == "sse" {
		e.URL = r.url
		e.Headers = r.headers
		return e
	}
	// Anything else is written as it stands and rejected by validateKimiEntry
	// below, rather than quietly turned into a stdio entry that runs something
	// the registry never asked for.
	e.Command = r.command
	e.Args = r.args
	e.Env = r.env
	return e
}

// InstallKimi merges mcps into the "mcpServers" object of Kimi's mcp.json.
//
// owned maps an MCP name to the fingerprint of the entry devexp last wrote for
// it (manifest.Manifest.MCPs); the returned map replaces it. That is what
// tells devexp's own entries from the user's: an entry devexp does not own is
// never rewritten and never removed, however it is named.
//
// Nothing is written when nothing changed — not even a re-encoding of the same
// content — so a second install reports "already configured" and leaves the
// file alone.
func InstallKimi(mcps []MCP, env map[string]string, path string, owned map[string]string, dryRun, reinstall bool) (map[string]string, error) {
	doc, servers, nullServers, err := loadKimiConfig(path)
	if err != nil {
		names := make([]string, 0, len(mcps))
		for _, m := range mcps {
			names = append(names, m.Name)
		}
		return owned, &ConfigRefusedError{Path: path, Reason: err, Servers: names}
	}

	warnInvalidEntries(path, servers, owned, nullServers)

	// Every name this run installs, whether or not it could be written. It is
	// what "no longer installed" means for pruning: a deselection or a
	// registry change, never a step that merely could not run.
	//
	// A project-scoped MCP is left out deliberately. Unlike the skips below,
	// which mean "not this run", it means devexp will never write this one to
	// Kimi's user file on any run — so an entry left from before the registry
	// changed its scope is stale, and is pruned like any other MCP devexp no
	// longer installs there.
	selected := make(map[string]bool, len(mcps))
	for _, m := range mcps {
		if m.scope() == "project" {
			continue
		}
		selected[m.Name] = true
	}

	newOwned := make(map[string]string, len(mcps))
	// keepOwned carries an MCP devexp could not configure this run forward as
	// still devexp's. "I cannot write this right now" — an unset required_env
	// in a fresh clone, in CI, in another shell — is not "this is no longer
	// installed", and pruning treats anything missing from newOwned as the
	// latter. Without this, re-running without mcps/.env deletes a working
	// entry the user never asked to lose.
	keepOwned := func(name string) {
		if h := owned[name]; h != "" {
			newOwned[name] = h
		}
	}
	var changes []kimiChange

	for _, m := range mcps {
		if m.scope() == "project" {
			// No keepOwned: see `selected` above — this one is not installed
			// here at all, so an entry from an earlier scope is pruned.
			ui.Skipped(m.Name, "project-scoped MCPs are not installed for Kimi")
			continue
		}
		r := resolveMCP(m, env)
		if len(r.missing) > 0 {
			printRequired(m, r.missing)
			keepOwned(m.Name)
			continue
		}
		raw, err := encodeJSON(kimiEntryFor(r))
		if err != nil {
			ui.Warn(fmt.Sprintf("%s — could not be encoded for mcp.json (%v); skipping it", m.Name, err))
			keepOwned(m.Name)
			continue
		}
		if err := validateKimiEntry(raw); err != nil {
			// A registry entry devexp itself cannot write validly. Skipping it
			// keeps the file loadable; writing it would cost the user every
			// other MCP server in it.
			ui.Warn(fmt.Sprintf("%s — devexp would write an entry Kimi rejects (%v); skipping it", m.Name, err))
			keepOwned(m.Name)
			continue
		}
		hash := entryFingerprint(raw)

		existing, exists := servers.get(m.Name)
		switch {
		case exists && entryFingerprint(existing) == hash:
			// Already exactly what devexp installs, so there is nothing to
			// write — but ownership is only carried forward, never claimed
			// here. An entry that merely happens to match is the user's, and
			// adopting it would let a later run, with this MCP deselected,
			// delete something they wrote. The cost is that a lost manifest
			// leaves devexp's own entry unmanaged until --reinstall-mcps.
			ui.Skipped(m.Name, "already configured")
			if reinstall {
				// The one way to say "this one is devexp's" about an entry
				// devexp cannot prove it wrote — and the escape hatch the
				// comment above promises, for a manifest that was lost.
				newOwned[m.Name] = hash
			} else {
				keepOwned(m.Name)
			}
			continue
		case exists && !ownsEntry(existing, owned[m.Name]) && !reinstall:
			ui.Skipped(m.Name, "a user-defined entry with this name exists — left untouched (--reinstall-mcps replaces it)")
			continue
		case !exists:
			changes = append(changes, kimiChange{verb: "add", name: m.Name, transport: r.transport})
		default:
			changes = append(changes, kimiChange{verb: "update", name: m.Name, transport: r.transport})
		}
		servers.set(m.Name, raw)
		newOwned[m.Name] = hash
	}

	changes = append(changes, pruneKimiEntries(servers, owned, selected)...)

	if dryRun {
		for _, c := range changes {
			ui.DryRun(c.describe(path))
		}
		return owned, nil
	}
	if len(changes) == 0 {
		return newOwned, nil
	}

	data, err := renderKimiConfig(doc, servers)
	if err != nil {
		return owned, fmt.Errorf("mcp: %w", err)
	}
	if err := verifyKimiConfig(data, newOwned); err != nil {
		return owned, fmt.Errorf("mcp: %q was left untouched: the file devexp was about to write %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return owned, err
	}
	// 0600 for a new file: the resolved env values and headers written above
	// are whatever mcps/.env holds, tokens included. An existing file keeps
	// its own mode, and a symlinked mcp.json keeps the link (fsutil).
	if err := fsutil.WriteFileAtomic(path, data, 0o600); err != nil {
		return owned, fmt.Errorf("mcp: %w", err)
	}
	// Only now: a "+ context7" printed before a write that then failed is the
	// line a skimming reader keeps, and it would be a lie.
	for _, c := range changes {
		c.report()
	}
	fmt.Printf("  Saved: %q\n", path)
	return newOwned, nil
}

// kimiChange is one edit to mcpServers, held back until it is on disk.
type kimiChange struct {
	verb      string // add | update | remove
	name      string
	transport string // empty for a removal
}

func (c kimiChange) describe(path string) string {
	switch c.verb {
	case "remove":
		return fmt.Sprintf("remove mcpServers.%s from %q", c.name, path)
	case "update":
		return fmt.Sprintf("update mcpServers.%s (%s) in %q", c.name, c.transport, path)
	}
	return fmt.Sprintf("add mcpServers.%s (%s) to %q", c.name, c.transport, path)
}

func (c kimiChange) report() {
	switch c.verb {
	case "remove":
		ui.Removed(c.name)
	case "update":
		ui.Updated(c.name)
	default:
		ui.Added(c.name)
	}
}

// pruneKimiEntries removes the entries devexp wrote on an earlier run and does
// not install now — an MCP deselected in the wizard, or dropped from the
// registry — the same way a stale agent or skill file is removed. Only an
// entry still identical to what devexp wrote goes; one the user has edited
// since stays, and stops being devexp's.
func pruneKimiEntries(servers *jsonObject, owned map[string]string, selected map[string]bool) []kimiChange {
	var changes []kimiChange
	for _, name := range sortedKeys(owned) {
		// Whatever happened to a selected MCP above — written, left alone as
		// the user's, or not configurable this run — it is still installed,
		// and it has already had its own line. Saying "no longer installed by
		// devexp" about it as well would be both a second message and a
		// false one.
		if selected[name] {
			continue
		}
		existing, exists := servers.get(name)
		if !exists {
			continue
		}
		if !ownsEntry(existing, owned[name]) {
			ui.Skipped(name, "no longer installed by devexp, but the entry has been edited — left in place and no longer tracked")
			continue
		}
		servers.del(name)
		changes = append(changes, kimiChange{verb: "remove", name: name})
	}
	return changes
}

// ownsEntry reports whether existing is the entry devexp recorded under this
// name. An empty record never matches, and neither does an entry that cannot
// be fingerprinted — both fingerprint to "", and comparing them equal would
// let devexp rewrite or delete an entry it never wrote.
func ownsEntry(existing json.RawMessage, recorded string) bool {
	if recorded == "" {
		return false
	}
	return entryFingerprint(existing) == recorded
}

// warnInvalidEntries reports entries that are already in the file and already
// unloadable. devexp leaves them alone — writing does not make them worse —
// but the user has to be told, because while one of them is there Kimi loads
// no MCP server at all, devexp's included, and says so only in its log.
func warnInvalidEntries(path string, servers *jsonObject, owned map[string]string, nullServers bool) {
	// "mcpServers": null is not "no servers" — Kimi requires an object and
	// rejects the whole file without one. Writing below repairs it; saying so
	// matters for the run that writes nothing.
	if nullServers {
		ui.Warn(fmt.Sprintf(`%q has "mcpServers": null, which Kimi rejects — while it is there Kimi loads no MCP servers at all. devexp repairs it if it writes anything below; if it writes nothing, replace it with {} by hand.`, path))
	}
	for _, name := range servers.keys {
		raw, _ := servers.get(name)
		if ownsEntry(raw, owned[name]) {
			continue // devexp's own, and about to be revalidated or replaced
		}
		if err := validateKimiEntry(raw); err != nil {
			ui.Warn(fmt.Sprintf("%q: the %q entry is one Kimi rejects (%v) — while it is there Kimi loads no MCP servers at all, devexp's included. devexp left it alone; fix or remove it.", path, name, err))
		}
	}
}

// renderKimiConfig puts servers back into doc and formats the whole file the
// way Kimi's own writer does: two-space indent, trailing newline.
func renderKimiConfig(doc, servers *jsonObject) ([]byte, error) {
	raw, err := servers.MarshalJSON()
	if err != nil {
		return nil, err
	}
	doc.set("mcpServers", raw)
	compact, err := doc.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// verifyKimiConfig re-reads the bytes about to be written and re-validates
// every entry devexp wrote. Only devexp's own: an entry that was already in
// the file and already invalid has been warned about, and refusing to write
// because of it would leave the user with no way to install anything.
func verifyKimiConfig(data []byte, wrote map[string]string) error {
	doc, err := decodeObject(data)
	if err != nil {
		return fmt.Errorf("would not have been readable (%v)", err)
	}
	raw, ok := doc.get("mcpServers")
	if !ok {
		return errors.New(`would have had no "mcpServers"`)
	}
	servers, err := decodeObject(raw)
	if err != nil {
		return fmt.Errorf(`would have had an "mcpServers" that is not an object (%v)`, err)
	}
	for _, name := range sortedKeys(wrote) {
		raw, ok := servers.get(name)
		if !ok {
			return fmt.Errorf("would not have contained the %q entry", name)
		}
		if err := validateKimiEntry(raw); err != nil {
			return fmt.Errorf("would have held a %q entry Kimi rejects: %w", name, err)
		}
	}
	return nil
}

// loadKimiConfig reads mcp.json for merging into. A missing or blank file is
// an empty document. Anything devexp cannot merge into without losing what is
// there is an error, and the caller writes nothing: unreadable, not strict
// JSON (Kimi parses mcp.json with JSON.parse — unlike opencode's config.json
// it is not JSONC), a top level that is not an object, or an "mcpServers" that
// is not one. Returned are the whole document and its servers object; every
// value devexp does not touch passes through as the bytes it was read as.
func loadKimiConfig(path string) (doc, servers *jsonObject, nullServers bool, err error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return newJSONObject(), newJSONObject(), false, nil
	}
	if err != nil {
		return nil, nil, false, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return newJSONObject(), newJSONObject(), false, nil
	}
	return parseKimiConfig(data, path)
}

// parseKimiConfig also reports an "mcpServers" that is present and null. Kimi
// rejects the whole file for it, so it is not the same as an absent key, but
// merging into an empty object and writing a real one repairs the file — so
// it is a notice, not a refusal.
func parseKimiConfig(data []byte, path string) (*jsonObject, *jsonObject, bool, error) {
	doc, err := decodeObject(data)
	if err != nil {
		return nil, nil, false, fmt.Errorf("%q is not a JSON object devexp can merge into (%v)", path, err)
	}
	raw, ok := doc.get("mcpServers")
	if !ok {
		return doc, newJSONObject(), false, nil
	}
	if string(bytes.TrimSpace(raw)) == "null" {
		return doc, newJSONObject(), true, nil
	}
	servers, err := decodeObject(raw)
	if err != nil {
		return nil, nil, false, fmt.Errorf(`%q: "mcpServers" is not a JSON object (%v)`, path, err)
	}
	return doc, servers, false, nil
}

// ── Kimi's entry schema ───────────────────────────────────────────────────────
//
// Mirrors the zod schema Kimi 2.0.1 parses mcp.json with
// (packages/agent-core-v2/src/mcpCore/config-schema.ts in its bundle): a union
// discriminated on transport, with stdio/http/sse arms and a set of fields
// common to all three. Unknown keys are accepted, because Kimi accepts them
// (it drops them on read). Every rule below was confirmed by feeding the real
// CLI a file that breaks it.

const maxKimiTimeoutMs = 2147483647

func validateKimiEntry(raw json.RawMessage) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return errors.New("is not a JSON object")
	}
	transport, err := entryTransport(obj)
	if err != nil {
		return err
	}
	checks := []func() error{
		func() error { return boolField(obj, "enabled") },
		func() error { return boolField(obj, "deferred") },
		func() error { return intField(obj, "startupTimeoutMs", 1, maxKimiTimeoutMs) },
		func() error { return intField(obj, "toolTimeoutMs", 1, maxKimiTimeoutMs) },
		func() error { return stringsField(obj, "enabledTools") },
		func() error { return stringsField(obj, "disabledTools") },
	}
	switch transport {
	case "stdio":
		checks = append(checks,
			func() error { return requiredStringField(obj, "command") },
			func() error { return stringsField(obj, "args") },
			func() error { return stringMapField(obj, "env") },
			func() error { return stringField(obj, "cwd") },
			func() error { return enumField(obj, "executor", "local", "kaos") },
			func() error { return requiredIfPresent(obj, "runtime_id") },
		)
	default:
		checks = append(checks,
			func() error { return urlField(obj, "url") },
			func() error { return stringMapField(obj, "headers") },
			func() error { return enumField(obj, "auth", "oauth") },
			func() error { return requiredIfPresent(obj, "bearerTokenEnvVar") },
		)
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// entryTransport returns the transport Kimi would use for the entry. An absent
// transport is inferred from what is there — a string command means stdio, a
// string url means http, and sse is never inferred.
func entryTransport(obj map[string]json.RawMessage) (string, error) {
	if raw, ok := obj["transport"]; ok {
		if err := notNull(raw, "transport"); err != nil {
			return "", err
		}
		var t string
		if err := json.Unmarshal(raw, &t); err != nil {
			return "", errors.New(`"transport" is not a string`)
		}
		switch t {
		case "stdio", "http", "sse":
			return t, nil
		}
		return "", fmt.Errorf("%q is not one of Kimi's transports (stdio, http, sse)", t)
	}
	if isJSONString(obj["command"]) {
		return "stdio", nil
	}
	if isJSONString(obj["url"]) {
		return "http", nil
	}
	return "", errors.New(`has no "transport", and no "command" or "url" to infer one from`)
}

// isJSONString reports whether raw is a JSON string. A null is not one: Kimi
// infers the transport with `typeof obj.command === "string"`, so an entry
// with a null command and a real url is an http entry to Kimi, not a broken
// stdio one — and encoding/json would decode that null into a string without
// complaining.
func isJSONString(raw json.RawMessage) bool {
	if isJSONNull(raw) {
		return false
	}
	var s string
	return len(raw) > 0 && json.Unmarshal(raw, &s) == nil
}

func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// notNull rejects an explicit JSON null in a declared field. encoding/json
// decodes null into a slice, map, bool or string without complaining, but
// zod's .optional() means absent: a null fails the arm, the discriminated
// union throws, and Kimi rejects the entire file — which is every MCP server
// the user has, not just this entry. Verified against 2.0.1 for args, env,
// cwd, enabled and headers.
func notNull(raw json.RawMessage, key string) error {
	if isJSONNull(raw) {
		return fmt.Errorf("%q is null, which Kimi does not accept (leave it out instead)", key)
	}
	return nil
}

func stringField(obj map[string]json.RawMessage, key string) error {
	raw, ok := obj[key]
	if !ok {
		return nil
	}
	if err := notNull(raw, key); err != nil {
		return err
	}
	if !isJSONString(raw) {
		return fmt.Errorf("%q is not a string", key)
	}
	return nil
}

func requiredStringField(obj map[string]json.RawMessage, key string) error {
	raw, ok := obj[key]
	if !ok {
		return fmt.Errorf("has no %q", key)
	}
	if err := notNull(raw, key); err != nil {
		return err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("%q is not a string", key)
	}
	if s == "" {
		return fmt.Errorf("%q is empty", key)
	}
	return nil
}

// requiredIfPresent is requiredStringField for an optional field that Kimi
// still refuses to accept empty.
func requiredIfPresent(obj map[string]json.RawMessage, key string) error {
	if _, ok := obj[key]; !ok {
		return nil
	}
	return requiredStringField(obj, key)
}

// urlField mirrors what Kimi's url() actually checks, which is `new URL` on
// the trimmed value: a scheme, and — for the schemes the URL standard calls
// special — a host. It deliberately insists on nothing else, because this
// feeds a warning that tells the user an entry is costing them every MCP
// server in the file. Requiring http/https would fire it on a file:// entry
// that loads.
//
// Go's parser is stricter than `new URL` in exactly three places — percent
// escapes, userinfo, and stray control bytes other than NUL — and those are
// tolerated. It is *not* stricter about ports or hosts, so every other parse
// failure is a URL Kimi rejects too, and is reported rather than waved
// through.
func urlField(obj map[string]json.RawMessage, key string) error {
	if err := requiredStringField(obj, key); err != nil {
		return err
	}
	var v string
	json.Unmarshal(obj[key], &v) //nolint:errcheck // requiredStringField checked it
	v = strings.TrimSpace(v)     // zod trims before parsing
	if !urlSchemeRe.MatchString(v) {
		return fmt.Errorf("%q is not a URL (it has no scheme)", key)
	}
	u, err := url.Parse(v)
	if err != nil {
		if toleratedURLError(err, v) {
			return nil
		}
		return fmt.Errorf("%q is not a URL (%v)", key, err)
	}
	// Hostname(), not Host: for "http://user@:80" the host is ":80" — not
	// empty, and not a host either.
	host := u.Hostname()
	if specialURLScheme[strings.ToLower(u.Scheme)] && host == "" {
		return fmt.Errorf("%q is not a URL (%s: needs a host)", key, u.Scheme)
	}
	// A bracketed IPv6 literal is colon-separated once the brackets come off,
	// so its colons are not the ones this is looking for. Anywhere else a
	// colon in the hostname means a second port, as in "e.com:8080:9090".
	forbidden := " :[]"
	if strings.HasPrefix(u.Host, "[") {
		forbidden = " []"
	}
	if strings.ContainsAny(host, forbidden) {
		return fmt.Errorf("%q has a host Kimi's URL parser rejects", key)
	}
	if port := u.Port(); port != "" {
		// Port 0 is a valid port to the URL standard and loads in Kimi; only
		// the upper bound is real. Atoi of a digits-only string is never
		// negative, and fails only on something far past 65535 anyway.
		n, convErr := strconv.Atoi(port)
		if convErr != nil || n > 65535 {
			return fmt.Errorf("%q has a port Kimi's URL parser rejects", key)
		}
	}
	return nil
}

// toleratedURLError reports whether err is one of the three failures where Go
// is stricter than the URL constructor, so the value is a URL Kimi loads.
// A NUL byte is the exception inside the third: Go and Kimi both reject it.
func toleratedURLError(err error, v string) bool {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "invalid URL escape"):
		return true
	case strings.Contains(msg, "invalid userinfo"):
		return true
	case strings.Contains(msg, "invalid control character"):
		return !strings.ContainsRune(v, 0)
	}
	return false
}

var (
	// urlSchemeRe is RFC 3986's scheme, which is what `new URL` insists on.
	urlSchemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	// specialURLScheme lists the schemes the URL standard requires a host
	// for. file is special too, but its host may be empty.
	specialURLScheme = map[string]bool{"http": true, "https": true, "ws": true, "wss": true, "ftp": true}
)

func stringsField(obj map[string]json.RawMessage, key string) error {
	raw, ok := obj[key]
	if !ok {
		return nil
	}
	if err := notNull(raw, key); err != nil {
		return err
	}
	// Decoded into []string, a null member arrives as "" with no error — the
	// same hole notNull closes one level up. array(string()) rejects it, and
	// with it the whole file.
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("%q is not a list of strings", key)
	}
	for i, item := range items {
		if isJSONNull(item) {
			return fmt.Errorf("%q has a null at position %d, which Kimi does not accept", key, i)
		}
		if !isJSONString(item) {
			return fmt.Errorf("%q is not a list of strings", key)
		}
	}
	return nil
}

func stringMapField(obj map[string]json.RawMessage, key string) error {
	raw, ok := obj[key]
	if !ok {
		return nil
	}
	if err := notNull(raw, key); err != nil {
		return err
	}
	// Same hole as stringsField: record(string(), string()) rejects a null
	// value, and decoding into map[string]string would turn it into "".
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("%q is not an object of strings", key)
	}
	for _, name := range sortedRawKeys(m) {
		if isJSONNull(m[name]) {
			return fmt.Errorf("%q has a null for %q, which Kimi does not accept", key, name)
		}
		if !isJSONString(m[name]) {
			return fmt.Errorf("%q is not an object of strings", key)
		}
	}
	return nil
}

// sortedRawKeys keeps the message about a bad member the same on every run:
// map iteration order is not.
func sortedRawKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func boolField(obj map[string]json.RawMessage, key string) error {
	raw, ok := obj[key]
	if !ok {
		return nil
	}
	if err := notNull(raw, key); err != nil {
		return err
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return fmt.Errorf("%q is not true or false", key)
	}
	return nil
}

func intField(obj map[string]json.RawMessage, key string, lo, hi int64) error {
	raw, ok := obj[key]
	if !ok {
		return nil
	}
	if err := notNull(raw, key); err != nil {
		return err
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("%q is not a number", key)
	}
	v, err := n.Int64()
	if err != nil || v < lo || v > hi {
		return fmt.Errorf("%q is not a whole number between %d and %d", key, lo, hi)
	}
	return nil
}

func enumField(obj map[string]json.RawMessage, key string, allowed ...string) error {
	raw, ok := obj[key]
	if !ok {
		return nil
	}
	if err := notNull(raw, key); err != nil {
		return err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("%q is not a string", key)
	}
	for _, a := range allowed {
		if s == a {
			return nil
		}
	}
	return fmt.Errorf("%q is %q, which Kimi does not accept", key, s)
}

// ── JSON that keeps what it was given ─────────────────────────────────────────

// jsonObject is a JSON object that remembers the order its keys came in and
// keeps every value it was not asked to change as the exact bytes it was read
// as. Decoding into map[string]any and back would reorder the user's file and
// round every number through float64; this keeps both.
type jsonObject struct {
	keys []string
	vals map[string]json.RawMessage
}

func newJSONObject() *jsonObject {
	return &jsonObject{vals: map[string]json.RawMessage{}}
}

func (o *jsonObject) get(key string) (json.RawMessage, bool) {
	raw, ok := o.vals[key]
	return raw, ok
}

func (o *jsonObject) set(key string, raw json.RawMessage) {
	if _, exists := o.vals[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = raw
}

func (o *jsonObject) del(key string) {
	if _, exists := o.vals[key]; !exists {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *jsonObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := encodeJSON(k)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(o.vals[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// decodeObject reads one JSON object, in order, and refuses anything else —
// including a second value after it, which encoding/json would otherwise
// ignore.
func decodeObject(data []byte) (*jsonObject, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("is not a JSON object")
	}
	o := newJSONObject()
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected key %v", keyTok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		o.set(key, raw)
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("has trailing data after the top-level value")
	}
	return o, nil
}

// entryFingerprint identifies one mcpServers entry, so a later run can tell
// the entry devexp wrote from one the user has edited since. It hashes a
// canonical form — keys sorted, numbers as written — so re-indenting or
// reordering a file never reads as an edit. An entry that cannot be decoded
// has no fingerprint and so is never mistaken for devexp's.
func entryFingerprint(raw json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return ""
	}
	data, err := encodeJSON(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// encodeJSON marshals v without escaping <, > and &. encoding/json escapes
// them by default, which would rewrite a URL's query string or a header value
// that devexp is only copying.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
