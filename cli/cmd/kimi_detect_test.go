package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
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
		// Output that looks fine but came with an error is still an error.
		"output alongside an error": {out: "0.42.0", runErr: errors.New("signal: killed"), wantStatus: kimiUnknown},
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
			}
		})
	}
}

// An unparsable version must still produce a readable notice — the raw output
// is what tells the user which binary devexp actually found.
func TestKimiNoticeQuotesWhatItSaw(t *testing.T) {
	if n := classifyKimi("Usage: kimi", nil, kimiMinVersion).notice(); !strings.Contains(n, "Usage: kimi") {
		t.Errorf("notice = %q, want it to quote the output it could not parse", n)
	}
	// Nothing at all must not render as an empty pair of brackets.
	n := classifyKimi("", nil, kimiMinVersion).notice()
	if !strings.Contains(n, "no output") {
		t.Errorf("notice = %q, want it to say there was no output", n)
	}
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
		fakeCLI(t)
		fakeCLIScript(t, "kimi", "cat")
		if got := detectKimi(); got.status != kimiUnknown {
			t.Errorf("= %+v, want kimiUnknown from a binary that prints nothing", got)
		}
	})

	// The real probe against the shape the installed CLI actually prints.
	t.Run("the real probe reads a bare semver", func(t *testing.T) {
		fakeCLI(t)
		fakeCLIScript(t, "kimi", "printf '0.42.0\\n'")
		if got := detectKimi(); got.status != kimiOK || got.version != "0.42.0" {
			t.Errorf("= %+v, want a usable 0.42.0", got)
		}
	})
}
