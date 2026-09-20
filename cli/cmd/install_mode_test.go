package cmd

import (
	"os"
	"testing"
)

// chooseInstallMode is the fix for #171's first half: before it, no flags meant
// the wizard unconditionally, and the wizard's first prompt needs a terminal.
// The rule is pure, so all four combinations are checkable without one.
func TestChooseInstallMode(t *testing.T) {
	tests := map[string]struct {
		flagsProvided bool
		isTerminal    bool
		want          installMode
	}{
		"no flags, a terminal: ask": {
			flagsProvided: false,
			isTerminal:    true,
			want:          modeWizard,
		},
		"no flags, no terminal: install the default": {
			// The regression: this used to reach ui.SelectAction, fail, and
			// install nothing. `curl … | bash` lands exactly here.
			flagsProvided: false,
			isTerminal:    false,
			want:          modeDefault,
		},
		"flags win over a terminal": {
			flagsProvided: true,
			isTerminal:    true,
			want:          modeFlags,
		},
		"flags without a terminal": {
			flagsProvided: true,
			isTerminal:    false,
			want:          modeFlags,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := chooseInstallMode(tt.flagsProvided, tt.isTerminal); got != tt.want {
				t.Errorf("chooseInstallMode(%v, %v) = %v, want %v",
					tt.flagsProvided, tt.isTerminal, got, tt.want)
			}
		})
	}
}

// The mode is only ever as good as the terminal check feeding it, so these
// drive the real stdinIsTerminal through chooseInstallMode with stdin actually
// swapped — the end of the path a bare `devexp install` takes.
func TestInstallModeFromRealStdin(t *testing.T) {
	swapStdin := func(t *testing.T, f *os.File) {
		t.Helper()
		orig := os.Stdin
		t.Cleanup(func() { os.Stdin = orig })
		os.Stdin = f
	}

	t.Run("closed stdin installs the default, never prompts", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe() error = %v", err)
		}
		w.Close()
		r.Close()
		swapStdin(t, r)

		if got := chooseInstallMode(false, stdinIsTerminal()); got != modeDefault {
			t.Errorf("mode with closed stdin = %v, want modeDefault — a bare install would die at the first prompt", got)
		}
	})

	t.Run("a pty prompts", func(t *testing.T) {
		master, slave, err := openPTYForTest()
		if err != nil {
			t.Skipf("no pty available: %v", err)
		}
		t.Cleanup(func() { slave.Close(); master.Close() })
		swapStdin(t, slave)

		// The true direction of stdinIsTerminal, which every other test has to
		// stub. Without it a return to an os.ModeCharDevice check — or to
		// always-false — would pass the whole suite.
		if !stdinIsTerminal() {
			t.Fatal("stdinIsTerminal() = false for a pty")
		}
		if got := chooseInstallMode(false, stdinIsTerminal()); got != modeWizard {
			t.Errorf("mode on a pty = %v, want modeWizard", got)
		}
		if got := chooseInstallMode(true, stdinIsTerminal()); got != modeFlags {
			t.Errorf("mode on a pty with flags = %v, want modeFlags", got)
		}
	})
}
