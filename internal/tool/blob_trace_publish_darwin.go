//go:build darwin

package tool

import (
	"os"

	"golang.org/x/sys/unix"
)

const immutableBlobPublicationSupported = true
const immutableBlobReadFlags = unix.O_NONBLOCK | unix.O_NOFOLLOW

func immutableBlobRenameNoReplace(parent *os.File, _ string, stage, leaf string) error {
	return unix.RenameatxNp(int(parent.Fd()), stage, int(parent.Fd()), leaf, unix.RENAME_EXCL)
}
