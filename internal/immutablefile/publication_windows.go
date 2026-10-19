//go:build windows

package immutablefile

import (
	"os"
	"path/filepath"
	"syscall"
)

const PublicationSupported = true
const ReadFlags = 0

func RenameNoReplace(_ *os.File, dir, stage, leaf string) error {
	from, err := syscall.UTF16PtrFromString(filepath.Join(dir, stage))
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(filepath.Join(dir, leaf))
	if err != nil {
		return err
	}
	return syscall.MoveFile(from, to) // Deliberately no REPLACE_EXISTING.
}
