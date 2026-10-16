//go:build windows

package tool

import (
	"os"
	"path/filepath"
	"syscall"
)

const immutableBlobPublicationSupported = true
const immutableBlobReadFlags = 0

func immutableBlobRenameNoReplace(_ *os.File, workDir, stage, leaf string) error {
	from, err := syscall.UTF16PtrFromString(filepath.Join(workDir, stage))
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(filepath.Join(workDir, leaf))
	if err != nil {
		return err
	}
	// Same-volume MoveFileW, deliberately without REPLACE_EXISTING.
	return syscall.MoveFile(from, to)
}
