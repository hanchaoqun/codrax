//go:build windows

package hitraceconv

import "syscall"

// publishConversionFileNoReplace uses MoveFileW without
// MOVEFILE_REPLACE_EXISTING. Unlike os.Rename on Windows, this fails atomically
// when finalPath already exists and preserves a racing external owner's file.
// Retained DB staging is created in finalPath's parent, so this is a same-volume
// move.
func publishConversionFileNoReplace(stagingPath, finalPath string) (stagingMoved bool, err error) {
	from, err := syscall.UTF16PtrFromString(stagingPath)
	if err != nil {
		return false, err
	}
	to, err := syscall.UTF16PtrFromString(finalPath)
	if err != nil {
		return false, err
	}
	if err := syscall.MoveFile(from, to); err != nil {
		return false, err
	}
	return true, nil
}
