//go:build !unix

package repo

import "os"

// statOwner can't tell who owns a file on this platform, so a dev build's
// source checkout is never verified and the bundled assets are used.
func statOwner(os.FileInfo) (uint32, bool) { return 0, false }
