package cmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// classifyKimi is the whole of the legacy-vs-Kimi-Code decision, and it is
// pure, so every output shape a `kimi` on PATH could produce is a row here
// rather than a binary someone has to install.
func TestClassifyKimi(t *testing.T) {
	tests := map[string]struct {
		out        string
		runErr     error
		wantStatus kimiStatus
		wantVer    string
	}{
		"the installed CLI's exact output": {out: "0.42.0\n", wantStatus: kimiOK, wantVer: "0.42.0"},
		"no trailing newline":              {out: "0.43.1", wantStatus: kimiOK, wantVer: "0.43.1"},
		"surrounding whitespace":           {out: "  0.42.0  \n", wantStatus: kimiOK, wantVer: "0.42.0"},
		"exactly the minimum":              {out: kimiMinVersion, wantStatus: kimiOK, wantVer: kimiMinVersion},
		"a major version above":            {out: "1.0.0", wantStatus: kimiOK, wantVer: "1.0.0"},
		"double-digit minor":               {out: "0.100.0", wantStatus: kimiOK, wantVer: "0.100.0"},
		// A prerelease counts as the release it is built towards.
		"a prerelease of a new enough version": {out: "0.44.0-beta.1", wantStatus: kimiOK, wantVer: "0.44.0"},
		"build metadata":                       {out: "0.42.0+sha.abc", wantStatus: kimiOK, wantVer: "0.42.0"},

		"one patch below the minimum": {out: "0.30.9", wantStatus: kimiTooOld, wantVer: "0.30.9"},
		"an ancient version":          {out: "0.1.0", wantStatus: kimiTooOld, wantVer: "0.1.0"},
		// A prerelease of the minimum is not the minimum.
		"a prerelease below the minimum": {out: "0.30.0-rc.1", wantStatus: kimiTooOld, wantVer: "0.30.0"},

		// The legacy Python kimi-cli ships a binary by the same name, and its
		// version numbers overlap Kimi Code's, so only the format decides.
		"legacy kimi-cli, a high version":         {out: "kimi, version 1.50.0\n", wantStatus: kimiLegacy},
		"legacy kimi-cli, an overlapping version": {out: "kimi, version 0.42.0", wantStatus: kimiLegacy},

		"no output at all":      {out: "", wantStatus: kimiUnknown},
		"whitespace only":       {out: "  \n ", wantStatus: kimiUnknown},
		"a usage message":       {out: "Usage: kimi [options]", wantStatus: kimiUnknown},
		"a two-part version":    {out: "0.42", wantStatus: kimiUnknown},
		"a v-prefixed version":  {out: "v0.42.0", wantStatus: kimiUnknown},
		"a version plus prose":  {out: "kimi 0.42.0", wantStatus: kimiUnknown},
		"a non-numeric version": {out: "x.y.z", wantStatus: kimiUnknown},
		// A binary that fails or times out is never guessed at.
		"the probe failed":    {out: "", runErr: errors.New("exit status 1"), wantStatus: kimiUnknown},
		"the probe timed out": {out: "", runErr: context.DeadlineExceeded, wantStatus: kimiUnknown},
		// Output that looks fine but came with an error is still an error —
		// and the notice must say the command failed rather than blame the
		// output, which was perfectly identifiable.
		"output alongside an error": {out: "0.42.0", runErr: errors.New("exit status 3"), wantStatus: kimiUnknown},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := classifyKimi(tt.out, tt.runErr, kimiMinVersion)
			if got.status != tt.wantStatus {
				t.Errorf("classifyKimi(%q, %v).status = %v, want %v", tt.out, tt.runErr, got.status, tt.wantStatus)
			}
			if got.version != tt.wantVer {
				t.Errorf("classifyKimi(%q, %v).version = %q, want %q", tt.out, tt.runErr, got.version, tt.wantVer)
			}

			// Anything devexp cannot use must say why, and a too-old one must
			// name the minimum so the user knows what to upgrade to.
			notice := got.notice()
			switch tt.wantStatus {
			case kimiOK:
				if notice != "" {
					t.Errorf("notice = %q, want none for a usable CLI", notice)
				}
			default:
				if notice == "" {
					t.Errorf("notice is empty for status %v; a skipped CLI must say why", tt.wantStatus)
				}
				if tt.wantStatus == kimiTooOld && !strings.Contains(notice, kimiMinVersion) {
					t.Errorf("notice = %q, want it to name the minimum %q", notice, kimiMinVersion)
				}
				if tt.wantStatus == kimiLegacy && !strings.Contains(notice, "legacy kimi-cli") {
					t.Errorf("notice = %q, want it to name the legacy CLI", notice)
				}
				// A failed probe is reported as a failure, with its reason —
				// never as output devexp could not parse.
				if tt.runErr != nil {
					if !strings.Contains(notice, tt.runErr.Error()) {
						t.Errorf("notice = %q, want it to carry the probe's error %q", notice, tt.runErr)
					}
					if strings.Contains(notice, "could not identify") {
						t.Errorf("notice = %q blames the output for a command that failed", notice)
					}
				}
			}
		})
	}
}

// A notice repeats what some binary on PATH printed, so it quotes and caps it:
// unquoted, an embedded newline forges a line of devexp output and an escape
// sequence reaches the terminal. Same rule as backup.go's.
func TestKimiNoticeQuotesWhatItSaw(t *testing.T) {
	if n := classifyKimi("Usage: kimi", nil, kimiMinVersion).notice(); !strings.Contains(n, `"Usage: kimi"`) {
		t.Errorf("notice = %q, want it to quote the output it could not parse", n)
	}
	// Nothing at all must not render as an empty pair of brackets.
	if n := classifyKimi("", nil, kimiMinVersion).notice(); !strings.Contains(n, "no output") {
		t.Errorf("notice = %q, want it to say there was no output", n)
	}

	t.Run("control characters cannot forge output or reach the terminal", func(t *testing.T) {
		for _, out := range []string{
			"0.1\n[devexp] Installed 34 agent(s).",
			"\x1b[2J\x1b[31mFAKE 0.42.0",
			"kimi, version 1.0\n[devexp] all good",
		} {
			n := classifyKimi(out, nil, kimiMinVersion).notice()
			if n == "" {
				t.Fatalf("classifyKimi(%q) produced no notice", out)
			}
			if strings.ContainsAny(n, "\n\r\x1b") {
				t.Errorf("notice %q carries a raw control character from %q", n, out)
			}
		}
	})

	t.Run("a flood of output is capped", func(t *testing.T) {
		n := classifyKimi(strings.Repeat("x", 20000), nil, kimiMinVersion).notice()
		if len(n) > kimiRawLimit+200 {
			t.Errorf("notice is %d bytes for 20000 bytes of output; it is not capped", len(n))
		}
		if !strings.Contains(n, "…") {
			t.Errorf("notice = %q, want it to show the output was truncated", n)
		}
	})
}

func TestCompareVersion(t *testing.T) {
	tests := map[string]struct {
		a, b string
		want int
	}{
		"equal":                   {a: "0.42.0", b: "0.42.0", want: 0},
		"patch lower":             {a: "0.42.0", b: "0.42.1", want: -1},
		"patch higher":            {a: "0.42.2", b: "0.42.1", want: 1},
		"minor beats patch":       {a: "0.43.0", b: "0.42.99", want: 1},
		"major beats minor":       {a: "1.0.0", b: "0.99.99", want: 1},
		"numeric, not lexical":    {a: "0.9.0", b: "0.10.0", want: -1},
		"double digits":           {a: "0.100.0", b: "0.31.0", want: 1},
		"a missing part counts 0": {a: "1", b: "1.0.0", want: 0},
		"a junk part counts 0":    {a: "x.y.z", b: "0.0.0", want: 0},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := compareVersion(tt.a, tt.b); got != tt.want {
				t.Errorf("compareVersion(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// shortenKimiProbeTimeout makes the probe's deadline small enough to assert
// against, and restores it afterwards.
func shortenKimiProbeTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := kimiProbeTimeout
	t.Cleanup(func() { kimiProbeTimeout = orig })
	kimiProbeTimeout = d
}

// swapKimiProbe replaces the version probe for one test.
func swapKimiProbe(t *testing.T, out string, err error) *int {
	t.Helper()
	orig := runKimiVersion
	t.Cleanup(func() { runKimiVersion = orig })
	calls := 0
	runKimiVersion = func() (string, error) {
		calls++
		return out, err
	}
	return &calls
}

// absPath resolves a helper binary before any test clears PATH. fakeCLI
// replaces PATH wholesale, so a stub script that says `sleep` finds nothing
// and exits 127 instantly — which would make every timing assertion below
// pass without the probe ever being bounded.
func absPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not on PATH: %v", name, err)
	}
	return p
}

func TestDetectKimi(t *testing.T) {
	t.Run("no binary on PATH is absent, and is never probed", func(t *testing.T) {
		fakeCLI(t)
		calls := swapKimiProbe(t, "0.42.0\n", nil)
		got := detectKimi()
		if got.status != kimiAbsent {
			t.Errorf("= %+v, want kimiAbsent", got)
		}
		if *calls != 0 {
			t.Errorf("probed %d time(s) with nothing on PATH", *calls)
		}
	})

	t.Run("a binary on PATH is probed exactly once", func(t *testing.T) {
		fakeCLI(t, "kimi")
		calls := swapKimiProbe(t, "0.42.0\n", nil)
		if got := detectKimi(); got.status != kimiOK || got.version != "0.42.0" {
			t.Errorf("= %+v, want a usable 0.42.0", got)
		}
		if *calls != 1 {
			t.Errorf("probed %d time(s), want 1", *calls)
		}
	})

	t.Run("a probe that times out is unknown, never assumed usable", func(t *testing.T) {
		fakeCLI(t, "kimi")
		swapKimiProbe(t, "", context.DeadlineExceeded)
		got := detectKimi()
		if got.status != kimiUnknown {
			t.Errorf("= %+v, want kimiUnknown", got)
		}
		if !strings.Contains(got.notice(), "skipping Kimi") {
			t.Errorf("notice = %q, want it to say Kimi was skipped", got.notice())
		}
	})

	t.Run("a legacy kimi-cli on PATH is not Kimi Code", func(t *testing.T) {
		fakeCLI(t, "kimi")
		swapKimiProbe(t, "kimi, version 1.50.0\n", nil)
		if got := detectKimi(); got.status != kimiLegacy {
			t.Errorf("= %+v, want kimiLegacy", got)
		}
	})

	// The real probe, not the seam: a binary that reads stdin must not be able
	// to hang the installer waiting for input that will never come.
	t.Run("the real probe never waits on stdin", func(t *testing.T) {
		cat := absPath(t, "cat")
		fakeCLI(t)
		fakeCLIScript(t, "kimi", cat)
		if got := detectKimi(); got.status != kimiUnknown {
			t.Errorf("= %+v, want kimiUnknown from a binary that prints nothing", got)
		}
	})

	// The two defences that keep a hostile or broken `kimi` from hanging every
	// install. Each stub outlives the shortened deadline by far, so a probe
	// that is not bounded does not fail the assertion — it hangs the test,
	// which is the same signal.
	bounded := map[string]string{
		// The deadline: a binary that simply never returns.
		"a binary that never exits": "%s 30",
		// WaitDelay: the child exits at once, but a grandchild keeps the
		// stdout pipe open, so cmd.Output reads from it until the grandchild
		// dies. The context does not cover this — it kills a process that has
		// already exited. Measured without WaitDelay: 30s.
		"a binary whose grandchild holds the pipe open": "%s 30 & exit 0",
	}
	for name, body := range bounded {
		t.Run("the real probe is bounded: "+name, func(t *testing.T) {
			sleepBin := absPath(t, "sleep")
			fakeCLI(t)
			fakeCLIScript(t, "kimi", fmt.Sprintf(body, sleepBin))
			shortenKimiProbeTimeout(t, 200*time.Millisecond)

			start := time.Now()
			got := detectKimi()
			elapsed := time.Since(start)

			if got.status != kimiUnknown {
				t.Errorf("= %+v, want kimiUnknown", got)
			}
			// Generous: the point is "bounded", not "fast". The stubs sleep
			// 30s, so anything in this range can only mean the bound held.
			if elapsed > 10*time.Second {
				t.Errorf("the probe took %v; it is not bounded", elapsed)
			}
			// The reason has to survive into the notice, or a hung `kimi`
			// looks the same as one that printed something odd.
			if n := got.notice(); !strings.Contains(n, "failed") {
				t.Errorf("notice = %q, want it to report the probe failure", n)
			}
		})
	}

	// The real probe against the shape the installed CLI actually prints.
	t.Run("the real probe reads a bare semver", func(t *testing.T) {
		fakeCLI(t)
		fakeCLIScript(t, "kimi", "printf '0.42.0\\n'")
		if got := detectKimi(); got.status != kimiOK || got.version != "0.42.0" {
			t.Errorf("= %+v, want a usable 0.42.0", got)
		}
	})
}
