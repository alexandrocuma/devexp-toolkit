//go:build unix

package repo

import (
	"os"
	"syscall"
)

// statOwner returns the uid that owns the file fi describes.
func statOwner(fi os.FileInfo) (uint32, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Uid, true
}
