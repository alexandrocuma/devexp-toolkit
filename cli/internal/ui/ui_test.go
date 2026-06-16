package ui

import (
	"os"
	"strings"
	"testing"
)

func TestMultiSelectLogic(t *testing.T) {
	items := []string{"alpha", "beta", "gamma"}

	allTrue := func() []bool { return []bool{true, true, true} }

	tests := map[string]struct {
		selected []bool
		choice   int  // menu index passed to applyMultiSelectChoice
		applyOne bool // whether to call applyMultiSelectChoice at all
		wantDone bool // expected done flag (only when applyOne)
		want     []string
	}{
		"initial state all selected": {
			selected: allTrue(),
			applyOne: false,
			want:     []string{"alpha", "beta", "gamma"},
		},
		"single toggle deselects correct item": {
			// idx 3 -> selected[1] (beta) toggled off; alpha & gamma remain.
			selected: allTrue(),
			choice:   3,
			applyOne: true,
			wantDone: false,
			want:     []string{"alpha", "gamma"},
		},
		"toggle-all off when all selected": {
			selected: allTrue(),
			choice:   1,
			applyOne: true,
			wantDone: false,
			want:     nil,
		},
		"toggle-all on when some deselected": {
			selected: []bool{true, false, true},
			choice:   1,
			applyOne: true,
			wantDone: false,
			want:     []string{"alpha", "beta", "gamma"},
		},
		"done returns current set": {
			selected: []bool{true, false, true},
			choice:   0,
			applyOne: true,
			wantDone: true,
			want:     []string{"alpha", "gamma"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if tt.applyOne {
				done := applyMultiSelectChoice(tt.selected, tt.choice)
				if done != tt.wantDone {
					t.Errorf("done = %v, want %v", done, tt.wantDone)
				}
			}
			got := collectSelected(items, tt.selected)
			if !equalStrings(got, tt.want) {
				t.Errorf("collectSelected = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildMultiSelectDisplay(t *testing.T) {
	items := []string{"alpha", "beta", "gamma"}

	tests := map[string]struct {
		selected []bool
		wantHead string   // expected row 0 (Done line)
		wantRows []string // expected item rows (index 2..)
	}{
		"all selected shows full count and ticks": {
			selected: []bool{true, true, true},
			wantHead: "✔  Done  (3 / 3 selected)",
			wantRows: []string{"✓  alpha", "✓  beta", "✓  gamma"},
		},
		"some deselected shows partial count and circles": {
			selected: []bool{true, false, true},
			wantHead: "✔  Done  (2 / 3 selected)",
			wantRows: []string{"✓  alpha", "○  beta", "✓  gamma"},
		},
		"none selected shows zero count": {
			selected: []bool{false, false, false},
			wantHead: "✔  Done  (0 / 3 selected)",
			wantRows: []string{"○  alpha", "○  beta", "○  gamma"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := buildMultiSelectDisplay(items, tt.selected)
			if len(got) != len(items)+2 {
				t.Fatalf("len = %d, want %d", len(got), len(items)+2)
			}
			if got[0] != tt.wantHead {
				t.Errorf("head = %q, want %q", got[0], tt.wantHead)
			}
			if got[1] != "◎  Toggle all" {
				t.Errorf("row 1 = %q, want toggle-all", got[1])
			}
			if !equalStrings(got[2:], tt.wantRows) {
				t.Errorf("item rows = %v, want %v", got[2:], tt.wantRows)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// captureOutput swaps os.Stdout and os.Stderr for pipes, runs fn, and returns
// (stdout, stderr).
func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	outCh := drain(rOut)
	errCh := drain(rErr)

	fn()

	_ = wOut.Close()
	_ = wErr.Close()
	return <-outCh, <-errCh
}

func drain(r *os.File) <-chan string {
	ch := make(chan string)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		ch <- sb.String()
	}()
	return ch
}

func TestOutputFormatters(t *testing.T) {
	tests := map[string]struct {
		fn       func()
		onStderr bool
		contains []string
	}{
		"Info":    {fn: func() { Info("hello-info") }, contains: []string{"hello-info", "[devexp]"}},
		"Success": {fn: func() { Success("hello-success") }, contains: []string{"hello-success", "[devexp]"}},
		"Warn":    {fn: func() { Warn("hello-warn") }, contains: []string{"hello-warn", "[devexp]"}},
		"Error":   {fn: func() { Error("hello-error") }, onStderr: true, contains: []string{"hello-error", "[devexp] ERROR:"}},
		"Added":   {fn: func() { Added("item-add") }, contains: []string{"item-add", "+"}},
		"Removed": {fn: func() { Removed("item-rm") }, contains: []string{"item-rm", "-"}},
		"Updated": {fn: func() { Updated("item-up") }, contains: []string{"item-up", "~", "updated"}},
		"Skipped": {fn: func() { Skipped("item-skip", "because") }, contains: []string{"item-skip", "[skip]", "because"}},
		"DryRun":  {fn: func() { DryRun("would-do-thing") }, contains: []string{"would-do-thing", "[dry-run]"}},
		"Required": {
			fn:       func() { Required("svc", []string{"TOK"}) },
			contains: []string{"svc", "[REQUIRED]", "TOK"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			stdout, stderr := captureOutput(t, tt.fn)
			out := stdout
			if tt.onStderr {
				if stdout != "" {
					t.Errorf("expected nothing on stdout, got %q", stdout)
				}
				out = stderr
			}
			for _, want := range tt.contains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\ngot: %s", want, out)
				}
			}
		})
	}
}
