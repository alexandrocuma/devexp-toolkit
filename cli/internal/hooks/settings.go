package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
)

// member is one member of a JSON object as read: its key and the bytes of its
// value.
type member struct {
	key   string
	value json.RawMessage
}

// rawObject is a JSON object kept as its members, in document order, each value
// as the bytes it was read as. A hook in settings.json carries fields devexp
// never interprets — timeout, args, shell, async, if, statusMessage, an http
// hook's url and headers, a prompt hook's prompt — and Claude Code adds more
// over time. Holding hooks this way writes every one of them back as it was
// (#137).
type rawObject []member

func parseRawObject(b []byte) (rawObject, error) {
	if !isJSONObject(b) {
		return nil, errors.New("not a JSON object")
	}
	c, err := scanJSONContainer(b, 0)
	if err != nil {
		return nil, err
	}
	obj := make(rawObject, 0, len(c.members))
	for _, m := range c.members {
		obj = append(obj, member{key: m.key, value: m.raw})
	}
	return obj, nil
}

// value returns the last member named key: the one encoding/json, and so
// Claude Code, reads when a key is repeated.
func (o rawObject) value(key string) (json.RawMessage, bool) {
	for i := len(o) - 1; i >= 0; i-- {
		if o[i].key == key {
			return o[i].value, true
		}
	}
	return nil, false
}

func (o rawObject) has(key string) bool {
	_, ok := o.value(key)
	return ok
}

// stringField returns key's value as a string, or "" when key is missing or
// its value isn't a string.
func (o rawObject) stringField(key string) string {
	v, ok := o.value(key)
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return ""
	}
	return s
}

// withString returns o with key set to the string s. When key already reads
// as s (as stringField reads it, so a missing key reads as ""), o is returned
// as it is and its bytes, escapes included, stay. Otherwise every member named
// key takes the new value in place, or one is appended.
func (o rawObject) withString(key, s string) (rawObject, error) {
	if o.stringField(key) == s {
		return o, nil
	}
	v, err := encodeJSON(s)
	if err != nil {
		return nil, err
	}
	return o.with(key, v), nil
}

// with returns a copy of o with every member named key set to v, or with v
// appended when there is none.
func (o rawObject) with(key string, v json.RawMessage) rawObject {
	out := append(rawObject(nil), o...)
	found := false
	for i := range out {
		if out[i].key == key {
			out[i].value = v
			found = true
		}
	}
	if !found {
		out = append(out, member{key: key, value: v})
	}
	return out
}

func (o rawObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := encodeJSON(m.key)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(m.value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// encodeJSON is json.Marshal without HTML escaping, so a string holding &, <
// or > is written as it reads rather than as a \u escape.
func encodeJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// hookEntry is one element of an event's array in settings.json.
type hookEntry struct {
	Matcher string
	Hooks   []hookCmd
	// fields is the entry as read from settings.json; nil for one devexp builds.
	fields rawObject
}

// hookCmd is one handler in a hookEntry's hooks array.
type hookCmd struct {
	Type    string
	Command string
	// fields is the handler as read from settings.json; nil for one devexp
	// builds.
	fields rawObject
}

func (e *hookEntry) UnmarshalJSON(b []byte) error {
	obj, err := parseRawObject(b)
	if err != nil {
		return fmt.Errorf("hook entry: %w", err)
	}
	out := hookEntry{Matcher: obj.stringField("matcher"), fields: obj}
	if v, ok := obj.value("hooks"); ok {
		if err := json.Unmarshal(v, &out.Hooks); err != nil {
			return fmt.Errorf("hook entry: hooks: %w", err)
		}
	}
	*e = out
	return nil
}

// MarshalJSON writes an entry devexp built exactly as earlier releases did
// ({"matcher", "hooks"}), and an entry read from settings.json as its own
// members in their order, with only its hooks array re-encoded.
func (e hookEntry) MarshalJSON() ([]byte, error) {
	if e.fields == nil {
		return encodeJSON(struct {
			Matcher string    `json:"matcher"`
			Hooks   []hookCmd `json:"hooks"`
		}{e.Matcher, e.Hooks})
	}
	obj, err := e.fields.withString("matcher", e.Matcher)
	if err != nil {
		return nil, err
	}
	if obj.has("hooks") || len(e.Hooks) > 0 {
		hooks, err := encodeJSON(e.Hooks)
		if err != nil {
			return nil, err
		}
		obj = obj.with("hooks", hooks)
	}
	return obj.MarshalJSON()
}

func (h *hookCmd) UnmarshalJSON(b []byte) error {
	obj, err := parseRawObject(b)
	if err != nil {
		return fmt.Errorf("hook handler: %w", err)
	}
	*h = hookCmd{Type: obj.stringField("type"), Command: obj.stringField("command"), fields: obj}
	return nil
}

// MarshalJSON writes a handler devexp built exactly as earlier releases did
// ({"type", "command"}), and a handler read from settings.json as its own
// members in their order, changing only a command devexp rewrote.
func (h hookCmd) MarshalJSON() ([]byte, error) {
	if h.fields == nil {
		return encodeJSON(struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		}{h.Type, h.Command})
	}
	obj, err := h.fields.withString("type", h.Type)
	if err != nil {
		return nil, err
	}
	if obj, err = obj.withString("command", h.Command); err != nil {
		return nil, err
	}
	return obj.MarshalJSON()
}

// devexpForm reports whether h has the shape of a handler devexp registers: a
// command hook, run through the shell. With args, Claude Code spawns command
// directly with no shell (exec form), so it isn't a shell word devexp wrote:
// such a handler is the user's whatever path it names, and is never matched,
// re-quoted or removed.
func (h hookCmd) devexpForm() bool {
	return h.Type == "command" && !h.fields.has("args")
}

// settingsDoc is settings.json as read, kept so that a rewrite changes only the
// value of its "hooks" key and every other byte stays as it was.
type settingsDoc struct {
	data    []byte
	top     jsonContainer
	hooksAt int      // index of the "hooks" member in top.members, or -1
	events  []string // hooks' event keys, in document order
	eol     string   // the file's line ending, for the lines devexp writes
}

// loadSettings parses settings.json content (empty for a missing file) and
// decodes its hooks for editing. Content devexp can't edit without losing
// something is refused: invalid JSON, a top level that isn't an object, hooks
// that isn't an object, an event that isn't an array, or an entry or handler
// that isn't an object.
func loadSettings(data []byte) (*settingsDoc, map[string][]hookEntry, error) {
	doc := &settingsDoc{data: data, hooksAt: -1, eol: lineEnding(data)}
	hooksMap := map[string][]hookEntry{}
	if len(bytes.TrimSpace(data)) == 0 {
		return doc, hooksMap, nil
	}
	if !json.Valid(data) {
		return nil, nil, errors.New("not valid JSON")
	}
	if !isJSONObject(data) {
		return nil, nil, errors.New("not a JSON object")
	}
	top, err := scanJSONContainer(data, 0)
	if err != nil {
		return nil, nil, err
	}
	doc.top = top
	for i, m := range top.members {
		if m.key == "hooks" {
			doc.hooksAt = i
		}
	}
	if doc.hooksAt < 0 {
		return doc, hooksMap, nil
	}
	raw := top.members[doc.hooksAt].raw
	if string(raw) == "null" {
		return doc, hooksMap, nil
	}
	events, err := parseRawObject(raw)
	if err != nil {
		return nil, nil, fmt.Errorf(`"hooks" is %w`, err)
	}
	for _, ev := range events {
		var entries []hookEntry
		if err := json.Unmarshal(ev.value, &entries); err != nil {
			return nil, nil, fmt.Errorf("hooks.%s: %w", ev.key, err)
		}
		if _, seen := hooksMap[ev.key]; !seen {
			doc.events = append(doc.events, ev.key)
		}
		hooksMap[ev.key] = entries
	}
	return doc, hooksMap, nil
}

// render returns settings.json with its hooks replaced by hooksMap. Events keep
// their order; new ones follow, sorted. Only the hooks value is written anew,
// indented like the line its key is on (or compact, in a compact file) and with
// the file's own line ending; a file without hooks gains the key after its last
// member, and a new file is written as earlier releases wrote it. The result
// must decode to the original document with only hooks changed, or nothing is
// returned.
func (d *settingsDoc) render(hooksMap map[string][]hookEntry) ([]byte, error) {
	order := make([]string, 0, len(hooksMap))
	listed := map[string]bool{}
	for _, ev := range d.events {
		if _, ok := hooksMap[ev]; ok {
			order = append(order, ev)
			listed[ev] = true
		}
	}
	var added []string
	for ev := range hooksMap {
		if !listed[ev] {
			added = append(added, ev)
		}
	}
	sort.Strings(added)
	order = append(order, added...)

	events := make(rawObject, 0, len(order))
	for _, ev := range order {
		v, err := encodeJSON(hooksMap[ev])
		if err != nil {
			return nil, err
		}
		events = append(events, member{key: ev, value: v})
	}
	hooks, err := encodeJSON(events)
	if err != nil {
		return nil, err
	}

	data := d.data
	var out []byte
	switch {
	case len(bytes.TrimSpace(data)) == 0:
		out = concat([]byte("{\n  \"hooks\": "), indentJSON(hooks, "  ", "  "), []byte("\n}"))
	case d.hooksAt >= 0:
		m := d.top.members[d.hooksAt]
		valueStart := m.end - len(m.raw)
		out = concat(data[:valueStart], d.withEOL(layoutAt(data, m.start).format(hooks)), data[m.end:])
	case len(d.top.members) == 0:
		out = concat(data[:d.top.openEnd], d.withEOL(concat([]byte("\n  \"hooks\": "), indentJSON(hooks, "  ", "  "), []byte("\n"))), data[d.top.close:])
	default:
		last := d.top.members[len(d.top.members)-1]
		l := layoutAt(data, last.start)
		var insert []byte
		if l.indented {
			insert = concat([]byte(",\n"+l.prefix+`"hooks": `), l.format(hooks))
		} else {
			insert = concat([]byte(`,"hooks":`), hooks)
		}
		out = concat(data[:last.end], d.withEOL(insert), data[last.end:])
	}

	// By construction out is data with only the hooks value replaced by hooks:
	// every other byte is copied. This check can't fail unless the splice above
	// has a bug, so it is a guard, not a code path a test can reach. Numbers
	// are compared as their text (json.Number): a valid number no float64 holds,
	// such as 1e400, must not stop the install.
	want := map[string]any{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := decodeJSON(data, &want); err != nil {
			return nil, err
		}
	}
	var hooksValue any
	if err := decodeJSON(hooks, &hooksValue); err != nil {
		return nil, err
	}
	want["hooks"] = hooksValue
	var got map[string]any
	if err := decodeJSON(out, &got); err != nil || !reflect.DeepEqual(got, want) {
		return nil, errors.New("rewriting it would change more than its hooks")
	}
	return out, nil
}

// decodeJSON decodes the single JSON value in b into v, keeping numbers as
// json.Number.
func decodeJSON(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("trailing data after the JSON value")
	}
	return nil
}

// lineEnding is the line ending of data's first line: CRLF or LF (also for a
// file with a single line or none).
func lineEnding(data []byte) string {
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// withEOL returns text devexp wrote, whose lines end in LF, with the file's own
// line ending. JSON strings can't hold a raw newline, so every LF is a line
// break.
func (d *settingsDoc) withEOL(b []byte) []byte {
	if d.eol == "\n" {
		return b
	}
	return bytes.ReplaceAll(b, []byte("\n"), []byte(d.eol))
}

// layout is how a member sits in its object: on its own line after prefix
// (indented), or run together with what precedes it (compact).
type layout struct {
	indented bool
	prefix   string
}

// layoutAt reads the layout of the member starting at data[start]: indented
// when only spaces and tabs precede it on its line.
func layoutAt(data []byte, start int) layout {
	lineStart := bytes.LastIndexByte(data[:start], '\n') + 1
	lead := data[lineStart:start]
	if len(bytes.Trim(lead, " \t")) != 0 {
		return layout{}
	}
	return layout{indented: true, prefix: string(lead)}
}

// format renders a compact JSON value for this layout: indented one level per
// nesting step, by the member's own indentation (two spaces when it has none).
func (l layout) format(compact []byte) []byte {
	if !l.indented {
		return compact
	}
	unit := l.prefix
	if unit == "" {
		unit = "  "
	}
	return indentJSON(compact, l.prefix, unit)
}

func indentJSON(compact []byte, prefix, indent string) []byte {
	var b bytes.Buffer
	if err := json.Indent(&b, compact, prefix, indent); err != nil {
		return compact
	}
	return b.Bytes()
}

func concat(parts ...[]byte) []byte {
	var n int
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
