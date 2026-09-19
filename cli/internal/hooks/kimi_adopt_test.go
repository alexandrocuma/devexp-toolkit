package hooks

import (
	"os"
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

// Removal has to reach the unmarked entries too, or #115's uninstall would
// leave a live guard behind on any machine where the user has logged in.
func TestRemoveKimiHooksTakesUnmarkedEntries(t *testing.T) {
	_, hooksDir := kimiHome(t, "")
	configPath, _ := kimiHome(t, kimiRewritten(hooksDir)+"[[hooks]]\nevent = \"Stop\"\ncommand = \"mine.sh\"\n")

	var changed bool
	var err error
	out := captureOutput(t, func() { changed, err = RemoveKimiHooks(configPath, false) })
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

// The recogniser matches the adapter's path tail, so a root that has moved is
// still recognised — and a command that merely mentions something similar is
// not.
func TestKimiAdapterRecogniser(t *testing.T) {
	yes := []string{
		"bash '/home/u/.kimi-code/hooks/kimi/adapter.sh' '/home/u/.kimi-code/hooks/claude-code/secret-guard.sh'",
		"bash '/opt/k/hooks/kimi/adapter.sh' '/opt/k/hooks/claude-code/secret-guard.sh'",
		`bash "C:\devexp\hooks\kimi\adapter.sh" "C:\devexp\hooks\claude-code\secret-guard.sh"`,
	}
	no := []string{
		"bash '/home/u/hooks/kimi/adapter.shell.sh'",
		"bash '/home/u/hooks/my-kimi/adapter.sh.bak'",
		"echo 'kimi adapter.sh'",
		"bash '/home/u/hooks/claude-code/secret-guard.sh'",
	}
	for _, c := range yes {
		if !kimiAdapterRe.MatchString(c) {
			t.Errorf("devexp's own command was not recognised: %q", c)
		}
	}
	for _, c := range no {
		if kimiAdapterRe.MatchString(c) {
			t.Errorf("a command that is not devexp's was claimed: %q", c)
		}
	}
}
