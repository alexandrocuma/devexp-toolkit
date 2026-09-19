//go:build unix

package cmd

import "syscall"

// mkfifoForTest makes a named pipe, for the "a recorded script name is not a
// regular file" case. Unix-only, and the caller skips where it is unavailable.
func mkfifoForTest(path string) error { return syscall.Mkfifo(path, 0o600) }
