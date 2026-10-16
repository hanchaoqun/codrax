//go:build linux

package tracecatalog

import "golang.org/x/sys/unix"

func exportRenameNoReplace(fd uintptr, stage, target string) error {
	return unix.Renameat2(int(fd), stage, int(fd), target, unix.RENAME_NOREPLACE)
}
