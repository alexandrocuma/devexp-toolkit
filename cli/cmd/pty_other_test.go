//go:build !linux && !darwin

package cmd

import (
	"errors"
	"os"
)

// openPTYForTest has no implementation outside Linux and Darwin; the tests
// that want one skip on this error rather than failing.
func openPTYForTest() (master, slave *os.File, err error) {
	return nil, nil, errors.New("no pty helper on this platform")
}
