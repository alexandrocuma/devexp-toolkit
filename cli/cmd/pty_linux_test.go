//go:build linux

package cmd

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openPTYForTest opens a pseudo-terminal pair so a test can prove that
// stdinIsTerminal says yes to a real one. Unix-only and per-OS, like
// mkfifoForTest; callers skip where it is unavailable.
//
// The Linux dance is unlock (TIOCSPTLCK 0), ask for the number (TIOCGPTN),
// then open /dev/pts/<n>. Done by hand rather than with a pty module because
// the CLI takes no dependency it does not ship a feature for, and a test
// helper is a poor reason to start.
func openPTYForTest() (master, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("unlock pty: %w", err)
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("pty number: %w", err)
	}
	name := fmt.Sprintf("/dev/pts/%d", n)
	s, err := os.OpenFile(name, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("open %s: %w", name, err)
	}
	return m, s, nil
}
