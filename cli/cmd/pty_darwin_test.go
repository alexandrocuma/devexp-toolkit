//go:build darwin

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTYForTest opens a pseudo-terminal pair so a test can prove that
// stdinIsTerminal says yes to a real one. Unix-only and per-OS, like
// mkfifoForTest; callers skip where it is unavailable.
//
// The Darwin dance is grant (TIOCPTYGRANT), unlock (TIOCPTYUNLK), then read
// the slave's path out of TIOCPTYGNAME. That last one writes into a 128-byte
// buffer and has no typed helper in x/sys/unix, hence the raw ioctl. Done by
// hand rather than with a pty module because the CLI takes no dependency it
// does not ship a feature for, and a test helper is a poor reason to start.
func openPTYForTest() (master, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}
	if err := unix.IoctlSetInt(int(m.Fd()), unix.TIOCPTYGRANT, 0); err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("grant pty: %w", err)
	}
	if err := unix.IoctlSetInt(int(m.Fd()), unix.TIOCPTYUNLK, 0); err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("unlock pty: %w", err)
	}
	var buf [128]byte
	if _, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		m.Fd(),
		uintptr(unix.TIOCPTYGNAME),
		uintptr(unsafe.Pointer(&buf[0])),
	); errno != 0 {
		m.Close()
		return nil, nil, fmt.Errorf("pty name: %w", errno)
	}
	end := bytes.IndexByte(buf[:], 0)
	if end < 0 {
		// Cannot happen for a kernel that terminated the name, but a panic
		// here would read as a test bug rather than a pty one.
		m.Close()
		return nil, nil, fmt.Errorf("pty name: no terminator in %d bytes", len(buf))
	}
	name := string(buf[:end])
	s, err := os.OpenFile(name, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("open %s: %w", name, err)
	}
	return m, s, nil
}
