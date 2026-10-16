//go:build darwin

package tracecatalog

import "golang.org/x/sys/unix"

// Same held directory and generated leaves; RENAME_EXCL also rejects targets
// created by external processes that do not follow the cooperating lock.
func exportRenameNoReplace(fd uintptr, stage, target string) error {
	return unix.RenameatxNp(int(fd), stage, int(fd), target, unix.RENAME_EXCL)
}
