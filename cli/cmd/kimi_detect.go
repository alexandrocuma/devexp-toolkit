package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ── Kimi Code CLI detection ───────────────────────────────────────────────────
//
// A binary named `kimi` on PATH is ambiguous. Kimi Code CLI (moonshotai/
// kimi-code, config under $KIMI_CODE_HOME) and the legacy Python kimi-cli
// (config under ~/.kimi) both install one, and their version numbers overlap,
// so the number cannot tell them apart — only the shape of the output can.
// Kimi Code prints a bare semver; the legacy CLI prints "kimi, version <x>".
// Anything else is some third binary by that name, and devexp skips it rather
// than guessing.

// kimiMinVersion is the oldest Kimi Code CLI devexp installs for.
const kimiMinVersion = "0.31.0"

// kimiProbeTimeout bounds the version probe. `kimi` on PATH is whatever binary
// carries that name; one that blocks reading stdin would otherwise hang every
// install, and no other exec in the installer is bounded. A var so a test can
// shorten it and assert that the bound is real.
var kimiProbeTimeout = 5 * time.Second

// kimiRawLimit caps how much of the probe's output a notice repeats. cmd.Output
// caps nothing, so without this a stub printing megabytes prints them to the
// user's terminal.
const kimiRawLimit = 120

type kimiStatus int

const (
	kimiAbsent  kimiStatus = iota // nothing named `kimi` on PATH
	kimiOK                        // Kimi Code CLI, at or above kimiMinVersion
	kimiTooOld                    // Kimi Code CLI, but older than that
	kimiLegacy                    // the Python kimi-cli, which devexp does not support
	kimiUnknown                   // named `kimi`, but not one devexp recognises
)

// kimiDetection is what a PATH lookup plus one version probe found. It is a
// plain value, so every selection rule that depends on it stays pure.
type kimiDetection struct {
	status  kimiStatus
	version string // the major.minor.patch that was parsed, when there was one
	raw     string // what the probe printed, trimmed
	err     error  // why the probe failed, when it did
}

var (
	// Prerelease and build metadata are accepted and compared as the release
	// they are built towards: a 0.31.0-rc is close enough to 0.31.0 to try.
	kimiSemverRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.+-]*)?$`)
	kimiLegacyRe = regexp.MustCompile(`^kimi, version `)
)

// classifyKimi turns one version probe into a detection. It is pure: the probe
// is the caller's job, so every output shape is a table row rather than a
// binary on PATH.
func classifyKimi(out string, runErr error, min string) kimiDetection {
	raw := strings.TrimSpace(out)
	// A probe that failed is never trusted, however good its output looked.
	// The error is kept beside that output rather than replacing it: a binary
	// that prints a fine version and exits 3 has identifiable output and a
	// failed command, and saying so is the difference between a usable message
	// and a baffling one.
	if runErr != nil {
		return kimiDetection{status: kimiUnknown, raw: raw, err: runErr}
	}
	// Format before number, always: the legacy CLI's versions overlap Kimi
	// Code's, so comparing the number first would accept it as new enough.
	if kimiLegacyRe.MatchString(raw) {
		return kimiDetection{status: kimiLegacy, raw: raw}
	}
	m := kimiSemverRe.FindStringSubmatch(raw)
	if m == nil {
		return kimiDetection{status: kimiUnknown, raw: raw}
	}
	version := m[1] + "." + m[2] + "." + m[3]
	if compareVersion(version, min) < 0 {
		return kimiDetection{status: kimiTooOld, version: version, raw: raw}
	}
	return kimiDetection{status: kimiOK, version: version, raw: raw}
}

// compareVersion orders two major.minor.patch strings, returning -1, 0 or 1.
func compareVersion(a, b string) int {
	av, bv := versionParts(a), versionParts(b)
	for i := range av {
		switch {
		case av[i] < bv[i]:
			return -1
		case av[i] > bv[i]:
			return 1
		}
	}
	return 0
}

// versionParts splits major.minor.patch into numbers. A part that does not
// parse counts as 0 rather than panicking: the string may have come from a
// foreign binary, and a wrong answer here must still be a safe one.
func versionParts(v string) [3]int {
	var out [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		n, _ := strconv.Atoi(part)
		out[i] = n
	}
	return out
}

// notice explains a `kimi` that was found but cannot be used, and returns ""
// when there is nothing to say — no binary at all, or a usable one.
func (k kimiDetection) notice() string {
	switch k.status {
	case kimiTooOld:
		return fmt.Sprintf("Kimi Code CLI %s found — devexp needs %s or newer; skipping Kimi", k.version, kimiMinVersion)
	case kimiLegacy:
		return fmt.Sprintf("`kimi` on PATH is the legacy kimi-cli (%s), not Kimi Code CLI — skipping Kimi", k.describeRaw())
	case kimiUnknown:
		if k.err != nil {
			return fmt.Sprintf("`kimi --version` failed (%v)%s — skipping Kimi", k.err, k.describeOutput())
		}
		return fmt.Sprintf("could not identify `kimi --version` output (%s) — skipping Kimi", k.describeRaw())
	}
	return ""
}

// describeRaw renders what the probe printed for a notice: quoted and capped,
// because it is whatever some binary on PATH chose to write. Unquoted, an
// embedded newline forges a line of devexp output and an escape sequence
// reaches the terminal — the same reason nothing from the manifest is printed
// raw (backup.go).
func (k kimiDetection) describeRaw() string {
	if k.raw == "" {
		return "no output"
	}
	raw := k.raw
	if len(raw) > kimiRawLimit {
		raw = raw[:kimiRawLimit] + "…"
	}
	return strconv.Quote(raw)
}

// describeOutput adds what the probe managed to print to a failure notice, and
// nothing at all when it printed nothing.
func (k kimiDetection) describeOutput() string {
	if k.raw == "" {
		return ""
	}
	return ", output " + k.describeRaw()
}

// runKimiVersion is a package var so tests can swap the probe for a fixed
// answer instead of putting a binary on PATH.
var runKimiVersion = func() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kimiProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kimi", "--version")
	cmd.Stdin = nil // a probe that waits for input is a hang, not an answer
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	return string(out), err
}

// detectKimi reports what the `kimi` on PATH actually is. It probes only when
// there is something to probe, so a run with no Kimi installed execs nothing.
func detectKimi() kimiDetection {
	if !commandExists("kimi") {
		return kimiDetection{status: kimiAbsent}
	}
	out, err := runKimiVersion()
	return classifyKimi(out, err, kimiMinVersion)
}
