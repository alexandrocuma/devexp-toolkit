package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── config.toml after Kimi has rewritten it ───────────────────────────────────
//
// Kimi round-trips config.toml through parse -> stringify whenever it writes
// its own config — on login above all — and stringify emits no comments. So
// devexp's markers vanish while the [[hooks]] tables between them stay. Every
// test here is about that file: the next install must recognise its own
// entries by their command and re-mark them, not append a second copy of every
// hook.

// kimiRewritten is what a config.toml looks like after Kimi has rewritten one
// devexp installed into: devexp's two hooks, no markers, no comments.
func kimiRewritten(hooksDir string) string {
	var b strings.Builder
	b.WriteString("model = \"k2\"\n\n")
	for _, h := range kimiTestHooks()[:2] {
		b.WriteString("[[hooks]]\n")
		b.WriteString("event = " + tomlString(h.Event) + "\n")
		b.WriteString("matcher = " + tomlString(h.Matcher) + "\n")
		b.WriteString("command = " + tomlString(KimiCommand(hooksDir, h)) + "\n")
		b.WriteString("timeout = 45\n\n")
	}
	return b.String()
}

func TestWriteKimiHooksAdoptsItsOwnUnmarkedEntries(t *testing.T) {
	_, hooksDir := kimiHome(t, "")
	configPath, _ := kimiHome(t, kimiRewritten(hooksDir))

	changed, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:2])
	if err != nil {
		t.Fatalf("WriteKimiHooks: %v\n%s", err, out)
	}
	if !changed {
		t.Fatalf("a file whose markers Kimi dropped was left as it was\n%s", out)
	}

	conf := readFile(t, configPath)
	if got := len(tomlHooks(t, conf)); got != 2 {
		t.Errorf("config.toml holds %d hooks, want 2 — devexp appended a second copy of its own:\n%s", got, conf)
	}
	if strings.Count(conf, kimiBlockBegin) != 1 || strings.Count(conf, kimiBlockEnd) != 1 {
		t.Errorf("the adopted entries were not re-marked exactly once:\n%s", conf)
	}
	// Everything of the user's survives.
	if !strings.Contains(conf, `model = "k2"`) {
		t.Errorf("the user's own config did not survive:\n%s", conf)
	}
	if !strings.Contains(out, "no devexp markers") {
		t.Errorf("the run does not explain what it recognised:\n%s", out)
	}

	// And the file is now stable again: a third install changes nothing.
	changed, out, err = writeKimi(t, configPath, hooksDir, kimiTestHooks()[:2])
	if err != nil || changed {
		t.Errorf("the re-marked file is not stable: changed = %v, err = %v\n%s", changed, err, out)
	}
}

// A user's own hooks are not devexp's, however they sit in the file.
func TestWriteKimiHooksAdoptionKeepsUserHooks(t *testing.T) {
	_, hooksDir := kimiHome(t, "")
	content := "[[hooks]]\nevent = \"Stop\"\ncommand = \"mine.sh\"\n\n" +
		kimiRewritten(hooksDir) +
		"# my own note\n[[hooks]]\nevent = \"SessionEnd\"\ncommand = \"also-mine.sh\"\n"
	configPath, _ := kimiHome(t, content)

	_, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:2])
	if err != nil {
		t.Fatalf("WriteKimiHooks: %v\n%s", err, out)
	}

	conf := readFile(t, configPath)
	got := tomlHooks(t, conf)
	if len(got) != 4 {
		t.Fatalf("config.toml holds %d hooks, want 4 (two of the user's, two of devexp's):\n%s", len(got), conf)
	}
	for _, want := range []string{"mine.sh", "also-mine.sh", "# my own note"} {
		if !strings.Contains(conf, want) {
			t.Errorf("adoption lost the user's %q:\n%s", want, conf)
		}
	}
}

// Markers present are the authoritative answer to "which lines are devexp's".
// An entry the user deliberately wrote outside the block to run the adapter
// their own way is theirs, and stays.
func TestWriteKimiHooksAdoptsOnlyWhenNoMarkers(t *testing.T) {
	_, hooksDir := kimiHome(t, "")
	theirs := "[[hooks]]\nevent = \"PreToolUse\"\nmatcher = \"^Grep$\"\ncommand = " +
		tomlString(KimiCommand(hooksDir, kimiTestHooks()[0])) + "\n"
	configPath, _ := kimiHome(t, theirs)

	// First install: no markers yet, so this one IS adopted — that is the
	// documented trade, and the install after it is the one under test.
	if _, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:1]); err != nil {
		t.Fatalf("first write: %v\n%s", err, out)
	}
	// Now the user adds their own adapter entry outside the marked block.
	conf := readFile(t, configPath) + "\n" + strings.Replace(theirs, "^Grep$", "^WebFetch$", 1)
	if err := os.WriteFile(configPath, []byte(conf), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:1]); err != nil {
		t.Fatalf("second write: %v\n%s", err, out)
	}
	after := readFile(t, configPath)
	if !strings.Contains(after, "^WebFetch$") {
		t.Errorf("an entry outside an intact marked block was adopted away:\n%s", after)
	}
	if got := len(tomlHooks(t, after)); got != 2 {
		t.Errorf("config.toml holds %d hooks, want 2:\n%s", got, after)
	}
}

// After one Kimi login there are no markers, so adoption is the NORMAL path,
// not a rare one — and what it matches, it deletes. A user hook that merely
// resembles devexp's is not devexp's: a different install root, a guard from
// somewhere else, an extra argument, a wrapper of their own. Each of these
// once matched, because the recogniser looked only at the adapter's path
// tail.
func TestWriteKimiHooksAdoptionLeavesLookalikesAlone(t *testing.T) {
	_, hooksDir := kimiHome(t, "")
	adapter := filepath.Join(hooksDir, "kimi", "adapter.sh")
	guard := filepath.Join(hooksDir, "claude-code", "secret-guard.sh")

	lookalikes := map[string]string{
		"a second devexp install under another root":  "bash '/opt/other/hooks/kimi/adapter.sh' '/opt/other/hooks/claude-code/secret-guard.sh'",
		"devexp's adapter running their own script":   "bash " + shellSingleQuote(adapter) + " '/home/u/my-guards/audit.sh'",
		"devexp's command with an argument of theirs": "bash " + shellSingleQuote(adapter) + " " + shellSingleQuote(guard) + " --verbose",
		"devexp's command inside a wrapper":           "myrunner bash " + shellSingleQuote(adapter) + " " + shellSingleQuote(guard),
		"devexp's command with something after it":    "bash " + shellSingleQuote(adapter) + " " + shellSingleQuote(guard) + " ; echo done",
		"a guard of theirs beside devexp's":           "bash '/home/u/kimi/adapter.sh' " + shellSingleQuote(guard),
		// A quoted extra argument: the command still ends in a quote, so only
		// the quote inside the extracted guard argument tells it from ours.
		"devexp's command with a quoted argument": "bash " + shellSingleQuote(adapter) + " " + shellSingleQuote(guard) + " " + shellSingleQuote("--verbose"),
	}

	for name, command := range lookalikes {
		t.Run(name, func(t *testing.T) {
			theirs := "[[hooks]]\nevent = \"PreToolUse\"\nmatcher = \"^Bash$\"\ncommand = " +
				tomlString(command) + "\ntimeout = 12\n"
			configPath, _ := kimiHome(t, theirs)

			_, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:1])
			if err != nil {
				t.Fatalf("WriteKimiHooks: %v\n%s", err, out)
			}
			after := readFile(t, configPath)
			if !strings.Contains(after, tomlString(command)) {
				t.Errorf("a hook that is not devexp's was taken over:\n%s", after)
			}
			if !strings.Contains(after, "timeout = 12") {
				t.Errorf("their own timeout went with it:\n%s", after)
			}
			if got := len(tomlHooks(t, after)); got != 2 {
				t.Errorf("config.toml holds %d hooks, want 2 (theirs and devexp's):\n%s", got, after)
			}
			if strings.Contains(stripANSI(out), "TAKEN THEM OVER") {
				t.Errorf("the run claims it took over a hook it left alone:\n%s", out)
			}
		})
	}
}

// What adoption does take over, it says plainly — and names, because a user
// who wrote one of those lines by hand has no other way to get it back.
func TestWriteKimiHooksAdoptionWarnsPlainly(t *testing.T) {
	_, hooksDir := kimiHome(t, "")
	configPath, _ := kimiHome(t, kimiRewritten(hooksDir))

	_, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:2])
	if err != nil {
		t.Fatalf("WriteKimiHooks: %v\n%s", err, out)
	}
	for _, want := range []string{"TAKEN THEM OVER", "put it back", KimiCommand(hooksDir, kimiTestHooks()[0])} {
		if !strings.Contains(out, want) {
			t.Errorf("the warning does not say %q:\n%s", want, out)
		}
	}
}

// The recogniser itself, close up.
func TestIsKimiOwnCommand(t *testing.T) {
	hooksDir := filepath.Join("/home", "u", ".kimi-code", "hooks")
	adapter := filepath.Join(hooksDir, "kimi", "adapter.sh")
	guard := filepath.Join(hooksDir, "claude-code", "secret-guard.sh")
	own := "bash " + shellSingleQuote(adapter) + " " + shellSingleQuote(guard)

	if !isKimiOwnCommand(hooksDir, own) {
		t.Errorf("devexp's own rendered command was not recognised: %q", own)
	}
	// Exactly what KimiCommand renders, so the two cannot drift apart.
	if got := KimiCommand(hooksDir, kimiTestHooks()[0]); !isKimiOwnCommand(hooksDir, got) {
		t.Errorf("KimiCommand renders something the recogniser rejects: %q", got)
	}
	notOwn := map[string]string{
		"nothing at all":                   "",
		"a trailing space":                 own + " ",
		"another interpreter":              "sh " + shellSingleQuote(adapter) + " " + shellSingleQuote(guard),
		"the adapter with no guard":        "bash " + shellSingleQuote(adapter),
		"an adapter under another root":    "bash " + shellSingleQuote(filepath.Join("/other/hooks", "kimi", "adapter.sh")) + " " + shellSingleQuote(guard),
		"a guard from the wrong directory": "bash " + shellSingleQuote(adapter) + " " + shellSingleQuote(filepath.Join(hooksDir, "kimi", "secret-guard.sh")),
		"a guard without the .sh suffix":   "bash " + shellSingleQuote(adapter) + " " + shellSingleQuote(filepath.Join(hooksDir, "claude-code", "secret-guard")),
		// A second argument that is QUOTED still ends in a quote, so the
		// bracketing check passes and only the quote INSIDE the extracted
		// argument catches it. An unquoted `--verbose` is caught by the
		// bracketing alone, so it pins nothing here.
		"a quoted second argument": own + " " + shellSingleQuote("--verbose"),
		"a quoted third argument":  own + " " + shellSingleQuote("--a") + " " + shellSingleQuote("--b"),
		// Same shape, with the guard argument itself carrying a quote.
		"a guard path holding a quote": "bash " + shellSingleQuote(adapter) + " " + shellSingleQuote(filepath.Join(hooksDir, "claude-code", "it's.sh")),
	}
	for why, c := range notOwn {
		if isKimiOwnCommand(hooksDir, c) {
			t.Errorf("%s was claimed as devexp's: %q", why, c)
		}
	}

	// Without a hooks root there is nothing to compare against, so nothing is
	// ever devexp's — adoption is off rather than guessing. The command that
	// matters here is a RELATIVE one: filepath.Join("", "kimi", "adapter.sh")
	// is "kimi/adapter.sh", so without the guard this would match, and an
	// entry of the user's running a relative adapter would be deleted.
	relative := "bash 'kimi/adapter.sh' 'claude-code/secret-guard.sh'"
	if isKimiOwnCommand("", relative) {
		t.Errorf("an unknown hooks root claimed a relative command: %q", relative)
	}
	if isKimiOwnCommand("", own) {
		t.Errorf("an unknown hooks root claimed an absolute command anyway")
	}
}

// Removal has to reach the unmarked entries too, or #115's uninstall would
// leave a live guard behind on any machine where the user has logged in.
func TestRemoveKimiHooksTakesUnmarkedEntries(t *testing.T) {
	_, hooksDir := kimiHome(t, "")
	configPath, _ := kimiHome(t, kimiRewritten(hooksDir)+"[[hooks]]\nevent = \"Stop\"\ncommand = \"mine.sh\"\n")

	var changed bool
	var err error
	out := captureOutput(t, func() { changed, err = RemoveKimiHooks(configPath, hooksDir, false) })
	if err != nil {
		t.Fatalf("RemoveKimiHooks: %v\n%s", err, stripANSI(out))
	}
	if !changed {
		t.Fatalf("unmarked devexp entries were left registered\n%s", stripANSI(out))
	}
	conf := readFile(t, configPath)
	got := tomlHooks(t, conf)
	if len(got) != 1 {
		t.Fatalf("config.toml holds %d hooks, want only the user's one:\n%s", len(got), conf)
	}
	if cmd, _ := got[0]["command"].(string); cmd != "mine.sh" {
		t.Errorf("the hook left behind is %q, not the user's:\n%s", cmd, conf)
	}
	if !strings.Contains(conf, `model = "k2"`) {
		t.Errorf("the user's own config did not survive:\n%s", conf)
	}
}

// ── the state a re-install is run to diagnose ────────────────────────────────
//
// A bare key written after devexp's end marker binds to devexp's LAST
// [[hooks]] table, because a marker is a comment and a comment ends nothing.
// Kimi's hook schema is strict, so that one extra key makes Kimi drop the
// entire hooks section — the user's own hooks and every devexp guard — behind
// a diagnostic nobody reads. In the session it just looks like no hook exists,
// and the first thing anyone does then is re-run the installer.
//
// The marked region is unchanged, so the write is a no-op. The run must still
// say what it found: this is the only code that ever looks at the file.
func TestWriteKimiHooksWarnsOnANoOpRun(t *testing.T) {
	_, hooksDir := kimiHome(t, "")
	configPath, _ := kimiHome(t, "model = \"k2\"\n")
	if _, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:2]); err != nil {
		t.Fatalf("first write: %v\n%s", err, out)
	}
	// After the end marker, so the marked region is untouched.
	broken := readFile(t, configPath) + "stray = \"key\"\n"
	if err := os.WriteFile(configPath, []byte(broken), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	changed, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:2])
	if err != nil {
		t.Fatalf("WriteKimiHooks: %v\n%s", err, out)
	}
	if changed {
		t.Errorf("a run with nothing to write reported a change")
	}
	if readFile(t, configPath) != broken {
		t.Errorf("a run with nothing to write rewrote the file")
	}
	if !strings.Contains(out, "Kimi runs no hook at all") {
		t.Errorf("the one run that could diagnose a dropped hooks section said nothing:\n%s", out)
	}
}

// The same shortcut hid the other half of that warning: a hook of the USER's
// that Kimi rejects. It is reported on a run that changes nothing, too.
func TestWriteKimiHooksWarnsAboutAForeignInvalidHookOnANoOpRun(t *testing.T) {
	_, hooksDir := kimiHome(t, "")
	configPath, _ := kimiHome(t, "[[hooks]]\nevent = \"NotAnEvent\"\ncommand = \"theirs.sh\"\n")
	if _, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:1]); err != nil {
		t.Fatalf("first write: %v\n%s", err, out)
	}

	changed, out, err := writeKimi(t, configPath, hooksDir, kimiTestHooks()[:1])
	if err != nil {
		t.Fatalf("WriteKimiHooks: %v\n%s", err, out)
	}
	if changed {
		t.Errorf("a second identical write reported a change")
	}
	if !strings.Contains(out, "NotAnEvent") {
		t.Errorf("the second run does not repeat that a hook in the file is one Kimi rejects:\n%s", out)
	}
}
