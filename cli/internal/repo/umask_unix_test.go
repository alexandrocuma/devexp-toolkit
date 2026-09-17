//go:build unix

package repo

import "syscall"

// The ownership tests build checkouts in temp dirs and set group and other
// write bits explicitly where a case needs them. Under a umask such as 002
// every directory they create would be group-writable too, so pin the usual
// 022 for this test binary.
func init() { syscall.Umask(0o022) }
