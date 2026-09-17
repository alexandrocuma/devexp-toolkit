//go:build unix

package repo

import (
	"os"
	"syscall"
)

// statIDs returns the uid and gid that own the file fi describes.
func statIDs(fi os.FileInfo) (uid, gid uint32, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return st.Uid, st.Gid, true
}
