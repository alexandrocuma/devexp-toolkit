//go:build !unix

package repo

import "os"

// statIDs can't tell who owns a file on this platform, so a dev build's
// source checkout is never verified and the bundled assets are used.
func statIDs(os.FileInfo) (uid, gid uint32, ok bool) { return 0, 0, false }
