package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"devexp/internal/fsutil"
	"devexp/internal/ui"
)

// ── Kimi's config.toml: devexp's managed hooks block ──────────────────────────
//
// Kimi has no `kimi hooks add`: its hooks are the `hooks` array of
// $KIMI_CODE_HOME/config.toml, one `[[hooks]]` table per hook. Everything
// below follows from how Kimi Code CLI 2.0.1 reads that array — read out of
// the shipped bundle rather than guessed, because the file also holds the
// user's providers, models and API keys and one bad edit costs them all of it:
//
//   - The section is registered as the config domain "hooks"
//     (registerConfigSection(HOOKS_SECTION, HooksConfigSchema) in
//     agent-core-v2/src/features/externalHooks/configSection.ts) and validated
//     as `array(HookDefSchema)`, where HookDefSchema is
//     `object({event: enum(HOOK_EVENT_TYPES), matcher: string().optional(),
//     command: string().min(1), timeout: number().int().min(1).max(600)
//     .optional()}).strict()`.
//   - `.strict()` means no other key. `name`, `description`, `type`, `id` —
//     anything devexp might have liked to mark ownership with inside the table
//     — makes the entry invalid. That is why the block below is delimited by
//     TOML comments and not by a key.
//   - One invalid entry costs the WHOLE section, the user's own hooks
//     included: buildValidated (agent-core-v2/src/app/config/configService)
//     validates a domain as a unit and, on a failure, drops it and pushes the
//     warning "Ignored invalid config section 'hooks': …". A warning in a
//     diagnostics pane is not something anyone reads — in the session it just
//     looks like no hook exists. So every entry devexp writes is checked
//     against that schema first, and the rendered file is re-parsed and
//     re-checked before it replaces anything.
//   - Kimi's own writer (node-sdk writeConfigFile → configToTomlData →
//     stringify) validates the whole config against a SECOND copy of the
//     schema, node-sdk/src/config/schema.ts, whose event enum is four events
//     shorter than the runtime one (no UserPromptQueued, TurnStarted,
//     SessionHeartbeat, TaskStarted). An entry using one of those four loads
//     at runtime but makes `kimi` throw CONFIG_INVALID the next time it writes
//     its own config — on login, say. kimiHookEvents (kimi.go) is the runtime
//     list; every event the registry actually uses is PreToolUse, which is in
//     both, and a new one should be checked against both before it is added.
//   - `hooks` is a top-level array of tables, so the block is appended at the
//     END of the file. A `[[hooks]]` header ends whatever table preceded it,
//     and any bare key written after it would belong to devexp's last hook —
//     which is exactly the extra key `.strict()` rejects. verifyKimiHooks
//     catches that, and a file it cannot vouch for is left alone.
//
// Every path this file prints or puts in an error is quoted: it comes from
// $KIMI_CODE_HOME, and unquoted, an embedded newline forges a line of devexp
// output and an escape sequence reaches the terminal (#111).

// The lines that delimit devexp's block. They are deliberately terse and
// contentless: they are matched literally on every later install, so the
// wording can never change without orphaning the blocks already on disk. What
// the block is and who wrote it goes in the comment lines inside it.
const (
	kimiBlockBegin = "# devexp:hooks:begin"
	kimiBlockEnd   = "# devexp:hooks:end"
)

// kimiMinTimeout and kimiMaxTimeout are HookDefSchema's
// `.int().min(1).max(600)`.
const (
	kimiMinTimeout = 1
	kimiMaxTimeout = 600
)

// ErrKimiMarkers is what a config.toml holding a devexp block devexp cannot
// identify is refused with: no begin without an end, one pair and no more, and
// the begin before the end. Anything else and devexp cannot tell which bytes
// are its own, so it writes none of them.
var ErrKimiMarkers = errors.New("devexp's " + kimiBlockBegin + " / " + kimiBlockEnd + " markers are unbalanced, so devexp cannot tell which lines are its own")

// KimiCommand is the command Kimi is given for one selected hook: bash, the
// adapter, and the Claude Code guard the adapter runs, both paths absolute and
// single-quoted.
//
// Kimi spawns the command with `shell: true` and no arguments of its own
// (runHook), so the guard's path has to travel inside the command line, and
// every path in it has to survive the shell. hooksDir is the installed hooks
// root — the one holding kimi/ and claude-code/ — and h.Script is
// registry-relative ("hooks/claude-code/secret-guard.sh"), so the registry's
// own "hooks/" prefix comes off.
func KimiCommand(hooksDir string, h KimiHook) string {
	script := filepath.Join(hooksDir, filepath.FromSlash(strings.TrimPrefix(h.Script, "hooks/")))
	adapter := filepath.Join(hooksDir, "kimi", "adapter.sh")
	return "bash " + shellSingleQuote(adapter) + " " + shellSingleQuote(script)
}

// shellSingleQuote wraps s so a POSIX shell reads it as one literal word. A
// single quote is closed, escaped and reopened, which is the only thing that
// cannot appear inside single quotes.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// kimiEntry is one [[hooks]] table as devexp writes it: HookDefSchema's four
// keys and nothing else, because the schema is strict.
type kimiEntry struct {
	event   string
	matcher string // omitted when empty; Kimi reads an empty pattern as "matches everything"
	command string
	timeout int
}

// validate checks the entry against HookDefSchema before it is rendered. A
// hook that fails is left out of the block rather than written: written, it
// would cost the user every hook in the file, devexp's and their own.
func (e kimiEntry) validate() error {
	switch {
	case !kimiHookEvents[e.event]:
		return fmt.Errorf("%q is not an event Kimi knows", e.event)
	case e.command == "":
		return errors.New("the command is empty")
	case strings.ContainsAny(e.command, "\n\r"):
		// TOML would escape it and the shell would then run two commands.
		return errors.New("the command spans more than one line")
	case strings.ContainsAny(e.matcher, "\n\r"):
		return errors.New("the matcher spans more than one line")
	case e.timeout != 0 && (e.timeout < kimiMinTimeout || e.timeout > kimiMaxTimeout):
		return fmt.Errorf("the timeout %ds is outside Kimi's %d..%d", e.timeout, kimiMinTimeout, kimiMaxTimeout)
	}
	return nil
}

// kimiEntries turns the selected hooks into entries, dropping — loudly — any
// that devexp cannot write validly.
func kimiEntries(hooksDir string, selected []KimiHook) []kimiEntry {
	entries := make([]kimiEntry, 0, len(selected))
	for _, h := range selected {
		e := kimiEntry{
			event:   h.Event,
			matcher: h.Matcher,
			command: KimiCommand(hooksDir, h),
			timeout: h.Timeout,
		}
		if err := e.validate(); err != nil {
			ui.Warn(fmt.Sprintf("%s — devexp would write a hook Kimi rejects (%v), which would cost you every hook in config.toml; skipping it", h.Name, err))
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// renderKimiBlock renders the marker-delimited block, LF-terminated. No
// entries means no block: the markers are not left behind empty, so a config
// devexp installs nothing into looks like one it never touched.
func renderKimiBlock(entries []kimiEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(kimiBlockBegin + "\n")
	b.WriteString("# Written by devexp. Everything between these two markers is replaced on\n")
	b.WriteString("# the next install, so edit hooks/registry.json rather than this block —\n")
	b.WriteString("# and keep hooks of your own outside it, where they will be left alone.\n")
	for _, e := range entries {
		b.WriteString("\n[[hooks]]\n")
		b.WriteString("event = " + tomlString(e.event) + "\n")
		if e.matcher != "" {
			b.WriteString("matcher = " + tomlString(e.matcher) + "\n")
		}
		b.WriteString("command = " + tomlString(e.command) + "\n")
		if e.timeout != 0 {
			b.WriteString(fmt.Sprintf("timeout = %d\n", e.timeout))
		}
	}
	b.WriteString(kimiBlockEnd + "\n")
	return b.String()
}

// tomlString renders s as a TOML basic string. Only the escapes TOML defines
// are used, so a backslash in a Windows path or a regex stays one backslash in
// the value Kimi reads.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// WriteKimiHooks puts devexp's hooks into Kimi's config.toml as one
// marker-delimited block, and returns whether the file changed.
//
// What it guarantees:
//
//   - Everything outside the block survives byte for byte — the user's
//     providers, models, keys, their own [[hooks]], their comments, their key
//     order and their formatting. Only the lines between the markers are
//     rewritten, and they are rewritten whole, so a hook that stops being
//     selected leaves the file with them.
//   - A block already in the file is replaced where it stands; a file without
//     one gets the block appended, which is the only place a top-level
//     `[[hooks]]` can go without capturing keys that follow it.
//   - The rendered file is re-parsed and re-checked before it replaces
//     anything: it must still be valid TOML, devexp's hooks must read back
//     exactly as written, and no hook that was there before may have been lost
//     or altered.
//   - Nothing is written when nothing changed, so a second install leaves the
//     bytes and the mtime as they were.
//
// What it refuses, leaving the file exactly as it was and saying so:
//
//   - a config.toml devexp cannot read, or that is not valid TOML;
//   - one whose markers are unbalanced, where devexp cannot tell which lines
//     are its own (ErrKimiMarkers);
//   - one where the file devexp was about to write would not parse, or would
//     not read back as intended — a `hooks` that is not an array of tables, a
//     bare key sitting after the block, a user hook the edit would disturb;
//   - one it cannot replace safely: a dangling symlink, a path that is not a
//     regular file, a file this user may not write (fsutil).
//
// A hook devexp itself cannot render validly is skipped with a warning rather
// than written, for the same reason: one entry Kimi rejects drops every hook
// in the file.
func WriteKimiHooks(configPath, hooksDir string, selected []KimiHook, dryRun bool) (bool, error) {
	if !filepath.IsAbs(hooksDir) {
		return false, fmt.Errorf("hooks: the installed hooks directory %q is not absolute, so Kimi — which runs a hook from whatever directory it is in — could not find the scripts; refusing to register them", hooksDir)
	}
	return editKimiHooks(configPath, renderKimiBlock(kimiEntries(hooksDir, selected)), dryRun)
}

// RemoveKimiHooks takes devexp's block back out of Kimi's config.toml and
// leaves every other byte as it was, including the user's own hooks. A file
// with no block, and a file that is not there at all, are not changes.
func RemoveKimiHooks(configPath string, dryRun bool) (bool, error) {
	return editKimiHooks(configPath, "", dryRun)
}

// editKimiHooks replaces devexp's block with block ("" removes it).
func editKimiHooks(configPath, block string, dryRun bool) (bool, error) {
	old, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("hooks: %q could not be read (%w), so it was left untouched and no hook was registered for Kimi", configPath, err)
	}
	if os.IsNotExist(err) {
		old = nil
		if block == "" {
			return false, nil
		}
	}

	// Refusing a file devexp cannot parse costs the user nothing they had:
	// Kimi cannot read it either, so nothing in it is working now, and
	// rewriting it would only lose whatever they were part way through.
	if len(bytes.TrimSpace(old)) > 0 {
		if err := toml.Unmarshal(old, &map[string]any{}); err != nil {
			return false, fmt.Errorf("hooks: %q is not valid TOML (%w) — Kimi cannot read it either; it was left untouched, so fix it and re-run", configPath, err)
		}
	}

	eol := lineEnding(old)
	// What is on disk, kept whole: the no-change test and the wording below
	// both compare against the file as the user has it, not against the
	// adopted copy.
	orig := old
	adopted := 0
	if begins, ends := findKimiMarkers(old); len(begins) == 0 && len(ends) == 0 {
		old, adopted = adoptUnmarkedKimiHooks(old)
	}

	data, err := spliceKimiBlock(old, withKimiEOL(block, eol), eol)
	if err != nil {
		return false, fmt.Errorf("hooks: %q was left untouched: %w", configPath, err)
	}
	if bytes.Equal(data, orig) {
		return false, nil
	}
	if err := verifyKimiHooks(data, block); err != nil {
		return false, fmt.Errorf("hooks: %q was left untouched: the file devexp was about to write %w", configPath, err)
	}
	warnForeignKimiHooks(configPath, old)
	if adopted > 0 {
		ui.Warn(fmt.Sprintf("%q holds %d devexp hook entr%s with no devexp markers around them — Kimi rewrites config.toml on login and drops every comment, markers included. devexp has recognised them by their command and is rewriting them as one marked block, rather than appending a second copy of every hook.",
			configPath, adopted, map[bool]string{true: "y", false: "ies"}[adopted == 1]))
	}

	verb := "update"
	switch {
	case block == "":
		verb = "remove"
	case len(bytes.TrimSpace(orig)) == 0:
		verb = "add"
	}
	if dryRun {
		ui.DryRun(fmt.Sprintf("%s devexp's hooks block in %q", verb, configPath))
		return true, nil
	}
	// 0700, as Kimi creates its own home with (writeConfigFile's
	// `mkdir(dirname, {mode: 448})`): what lands in there is the user's
	// credentials.
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return false, fmt.Errorf("hooks: %w", err)
	}
	// 0600 for a new config.toml: it is the file Kimi writes provider API keys
	// into. An existing file keeps its own mode, and a symlinked config.toml
	// keeps the link (fsutil).
	if err := fsutil.WriteFileAtomic(configPath, data, 0o600); err != nil {
		return false, fmt.Errorf("hooks: %w", err)
	}
	return true, nil
}

// withKimiEOL gives block the file's own line ending. devexp renders LF and
// the file may be CRLF; a block that mixed the two would be valid TOML and
// would still look wrong in the user's editor.
func withKimiEOL(block, eol string) string {
	if eol == "\n" || block == "" {
		return block
	}
	return strings.ReplaceAll(block, "\n", eol)
}

// spliceKimiBlock puts block where devexp's block is, or at the end of the
// file when there is none. block is "" to remove it.
//
// A marker is a line that, trimmed of spaces and of the CR of a CRLF file, is
// exactly the marker — so a marker quoted inside a string or mentioned in a
// comment is not one.
func spliceKimiBlock(content []byte, block, eol string) ([]byte, error) {
	begins, ends := findKimiMarkers(content)
	switch {
	case len(begins) == 0 && len(ends) == 0:
		if block == "" {
			return content, nil
		}
		return appendKimiBlock(content, block, eol), nil
	case len(begins) != 1 || len(ends) != 1 || ends[0] < begins[0]:
		return nil, ErrKimiMarkers
	}

	before, after := content[:begins[0]], content[ends[0]:]
	if block == "" {
		// Give back the blank line appendKimiBlock put in, so installing and
		// then removing leaves the file it started as.
		sep := []byte(eol + eol)
		if len(after) == 0 && bytes.HasSuffix(before, sep) {
			before = before[:len(before)-len(eol)]
		}
		return concatBytes(before, after), nil
	}
	return concatBytes(before, []byte(block), after), nil
}

// findKimiMarkers returns the offset of the start of each begin-marker line
// and the offset just past the end of each end-marker line (its line ending
// included).
func findKimiMarkers(content []byte) (begins, ends []int) {
	for off := 0; off < len(content); {
		end := bytes.IndexByte(content[off:], '\n')
		next := len(content)
		if end >= 0 {
			next = off + end + 1
		}
		switch strings.TrimSpace(string(content[off:next])) {
		case kimiBlockBegin:
			begins = append(begins, off)
		case kimiBlockEnd:
			ends = append(ends, next)
		}
		off = next
	}
	return begins, ends
}

// ── Recovering from a config.toml Kimi rewrote ────────────────────────────────
//
// Kimi's own writer round-trips config.toml through parse -> stringify, and
// stringify emits no comments. So the moment the user logs in — or changes any
// setting from inside Kimi — devexp's two marker lines are gone while the
// [[hooks]] tables between them stay. Left alone, the next install finds no
// markers, appends its block, and the user has every guard registered TWICE:
// each tool call scanned twice, each block reason printed twice, and no way to
// tell which copy is which.
//
// The entries themselves survive that rewrite intact, and they are
// recognisable: their command is devexp's adapter invocation, which nothing
// else has a reason to be. So when there are no markers at all, an entry that
// runs devexp's adapter is adopted as devexp's — lifted out of the file here
// and written back inside fresh markers by the same splice that would have
// appended them.
//
// Only when there are NO markers. A file that still has them is the
// authoritative answer to "which lines are devexp's", and an entry a user
// deliberately wrote outside the block to run the adapter their own way is
// theirs to keep.

// kimiAdapterRe recognises devexp's adapter in a rendered command. It matches
// the path tail rather than the whole command, because the install root moves:
// a user who repoints $KIMI_CODE_HOME, or a devexp that changes how it quotes,
// must still recognise the entries the previous install wrote.
var kimiAdapterRe = regexp.MustCompile(`(^|[^\w.-])kimi[/\\]adapter\.sh($|[^\w.-])`)

// kimiCommandRe matches the `command = "..."` line of a [[hooks]] table. Only
// a basic string: that is what renderKimiBlock writes, and an entry devexp did
// not write is not devexp's to adopt.
var kimiCommandRe = regexp.MustCompile(`^\s*command\s*=\s*(".*")\s*$`)

// adoptUnmarkedKimiHooks removes the [[hooks]] tables whose command runs
// devexp's adapter, and reports how many it took out. The caller then appends
// the current block, so the net effect is that the stripped entries come back
// with their markers.
//
// It works on lines rather than on the parsed document because everything
// outside devexp's own entries has to survive byte for byte, and a TOML
// round-trip would not keep the user's formatting, key order or comments.
// verifyKimiHooks re-reads the result either way.
func adoptUnmarkedKimiHooks(content []byte) ([]byte, int) {
	lines, offsets := kimiLines(content)
	var out []byte
	kept, adopted := 0, 0
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "[[hooks]]" {
			continue
		}
		end := kimiTableEnd(lines, i)
		if !kimiTableIsDevexp(lines[i+1 : end]) {
			i = end - 1
			continue
		}
		// Blank lines immediately before the table go with it, so adopting
		// does not leave a gap growing by one line per install. A comment is
		// never absorbed: it may be the user's, about what follows.
		start := i
		for start > 0 && strings.TrimSpace(lines[start-1]) == "" {
			start--
		}
		out = append(out, content[kept:offsets[start]]...)
		kept = offsets[end]
		adopted++
		i = end - 1
	}
	if adopted == 0 {
		return content, 0
	}
	return append(out, content[kept:]...), adopted
}

// kimiTableEnd returns the index just past the last line of the table opened
// at lines[start]: the next table header, or the end of the file, with any
// trailing blank and comment lines given back so they stay with what follows
// them rather than being adopted along with the table.
func kimiTableEnd(lines []string, start int) int {
	end := len(lines)
	for j := start + 1; j < len(lines); j++ {
		if strings.HasPrefix(strings.TrimSpace(lines[j]), "[") {
			end = j
			break
		}
	}
	for end > start+1 {
		t := strings.TrimSpace(lines[end-1])
		if t != "" && !strings.HasPrefix(t, "#") {
			break
		}
		end--
	}
	return end
}

// kimiTableIsDevexp reports whether a [[hooks]] table's body holds a command
// that runs devexp's adapter.
func kimiTableIsDevexp(body []string) bool {
	for _, line := range body {
		m := kimiCommandRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var doc struct {
			C string `toml:"c"`
		}
		if err := toml.Unmarshal([]byte("c = "+m[1]), &doc); err != nil {
			return false
		}
		return kimiAdapterRe.MatchString(doc.C)
	}
	return false
}

// kimiLines splits content into lines (each keeping its own line ending) and
// the byte offset each one starts at, plus a final offset for the end of the
// content, so a run of lines maps straight back to a byte range.
func kimiLines(content []byte) ([]string, []int) {
	var lines []string
	offsets := []int{}
	for off := 0; off < len(content); {
		next := len(content)
		if end := bytes.IndexByte(content[off:], '\n'); end >= 0 {
			next = off + end + 1
		}
		lines = append(lines, string(content[off:next]))
		offsets = append(offsets, off)
		off = next
	}
	return lines, append(offsets, len(content))
}

// appendKimiBlock puts block at the end of the file, after a blank line. The
// end is the only place a top-level [[hooks]] is safe: it ends whatever table
// came before it, and nothing follows it to be captured.
func appendKimiBlock(content []byte, block, eol string) []byte {
	if len(content) == 0 {
		return []byte(block)
	}
	sep := ""
	if !bytes.HasSuffix(content, []byte(eol)) {
		sep = eol
	}
	return concatBytes(content, []byte(sep+eol), []byte(block))
}

func concatBytes(parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// verifyKimiHooks re-reads the bytes about to be written, the way Kimi will.
// It is the last check before the user's config is replaced, and it is what
// makes "everything else survives" a guarantee rather than an intention:
//
//   - the file still parses as TOML;
//   - devexp's hooks read back as the entries devexp meant to write, in order
//     and contiguous — so a bare key that ended up inside the last one, which
//     is what an appended block risks, is caught;
//   - the file holds exactly the hooks it held outside devexp's block, plus
//     devexp's — so nothing of the user's was swallowed or dropped.
func verifyKimiHooks(data []byte, block string) error {
	got, err := kimiHooksIn(data)
	if err != nil {
		return err
	}
	// The same bytes with devexp's block taken out: the hooks that are the
	// user's, whether this run added the block or replaced one.
	bare, err := spliceKimiBlock(data, "", lineEnding(data))
	if err != nil {
		return fmt.Errorf("cannot be read back without devexp's own block: %w", err)
	}
	theirs, err := kimiHooksIn(bare)
	if err != nil {
		return fmt.Errorf("would not parse without devexp's own block: %w", err)
	}
	want := kimiWantedIn(block)
	if len(got) != len(theirs)+len(want) {
		return fmt.Errorf("would hold %d hooks where it should hold %d — %d of them the ones already in the file; devexp's block would have disturbed them", len(got), len(theirs)+len(want), len(theirs))
	}
	if len(want) == 0 {
		return nil
	}
	for off := 0; off+len(want) <= len(got); off++ {
		if reflect.DeepEqual(got[off:off+len(want)], want) {
			return nil
		}
	}
	return errors.New("would not read back as the hooks devexp meant to write — a line outside devexp's block has run into it")
}

// kimiHooksIn parses data and returns its hooks as Kimi's schema will see
// them. An absent hooks key is no hooks; anything there that is not an array
// of tables is what Kimi rejects the whole section for, and is an error here.
func kimiHooksIn(data []byte) ([]map[string]any, error) {
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("would not be valid TOML: %w", err)
	}
	raw, ok := doc["hooks"]
	if !ok || raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("would hold a `hooks` that is not an array of tables (%T), which Kimi rejects the whole hooks section for", raw)
	}
	out := make([]map[string]any, 0, len(arr))
	for i, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("would hold a `hooks` entry at index %d that is not a table (%T), which Kimi rejects the whole hooks section for", i, e)
		}
		out = append(out, m)
	}
	return out, nil
}

// kimiWantedIn re-reads devexp's own block for what it should decode to, so
// what is verified is the text that is actually going into the file and not a
// second rendering of the same intent.
func kimiWantedIn(block string) []map[string]any {
	if block == "" {
		return nil
	}
	want, err := kimiHooksIn([]byte(block))
	if err != nil {
		// Unreachable: the block came from renderKimiBlock. The caller's
		// count check then fails and the file is left alone.
		return nil
	}
	return want
}

// warnForeignKimiHooks reports a hook that was already in the file and that
// Kimi already rejects. devexp leaves it alone — it is the user's, and writing
// does not make it worse — but they have to be told, because while it is there
// Kimi runs NO hook from this file, devexp's guards included, and says so only
// as a diagnostic nobody reads.
func warnForeignKimiHooks(path string, old []byte) {
	bare, err := spliceKimiBlock(old, "", lineEnding(old))
	if err != nil {
		return
	}
	theirs, err := kimiHooksIn(bare)
	if err != nil {
		ui.Warn(fmt.Sprintf("%q: %v — while that is so, Kimi runs no hook at all from this file, devexp's guards included", path, err))
		return
	}
	for i, h := range theirs {
		if err := validateForeignKimiHook(h); err != nil {
			ui.Warn(fmt.Sprintf("%q: the hook already at index %d is one Kimi rejects (%v) — while it is there Kimi runs no hook at all from this file, devexp's guards included; devexp has left it as it is", path, i, err))
		}
	}
}

// validateForeignKimiHook checks one already-present entry against
// HookDefSchema — the strict key set, the event enum, a non-empty command and
// a timeout inside 1..600.
func validateForeignKimiHook(h map[string]any) error {
	for k := range h {
		switch k {
		case "event", "matcher", "command", "timeout":
		default:
			return fmt.Errorf("it has a %q key, and Kimi's hook schema is strict", k)
		}
	}
	event, ok := h["event"].(string)
	switch {
	case !ok:
		return errors.New("it has no `event` string")
	case !kimiHookEvents[event]:
		return fmt.Errorf("%q is not an event Kimi knows", event)
	}
	cmd, ok := h["command"].(string)
	switch {
	case !ok:
		return errors.New("it has no `command` string")
	case cmd == "":
		return errors.New("its `command` is empty")
	}
	if m, ok := h["matcher"]; ok {
		if _, ok := m.(string); !ok {
			return errors.New("its `matcher` is not a string")
		}
	}
	if t, ok := h["timeout"]; ok {
		n, ok := t.(int64)
		switch {
		case !ok:
			return errors.New("its `timeout` is not a whole number")
		case n < kimiMinTimeout || n > kimiMaxTimeout:
			return fmt.Errorf("its `timeout` of %d is outside Kimi's %d..%d", n, kimiMinTimeout, kimiMaxTimeout)
		}
	}
	return nil
}
