//go:build linux

package immutablefile

import (
	"golang.org/x/sys/unix"
	"os"
)

const PublicationSupported = true
const ReadFlags = unix.O_NONBLOCK | unix.O_NOFOLLOW

func RenameNoReplace(parent *os.File, _ string, stage, leaf string) error {
	return unix.Renameat2(int(parent.Fd()), stage, int(parent.Fd()), leaf, unix.RENAME_NOREPLACE)
}
