//go:build windows

package hitraceconv

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// validateWindowsExactGenerationFileSystem keeps the current 64-bit
// BY_HANDLE_FILE_INFORMATION identity honest. Microsoft does not guarantee
// FileIndex uniqueness on ReFS; remote file systems also have weaker
// generation semantics. Until filegeneration carries FILE_ID_INFO's 128-bit
// identity, exact-generation authorities are NTFS-only on Windows.
func validateWindowsExactGenerationFileSystem(handle windows.Handle, purpose string) error {
	var fileSystemName [64]uint16
	if err := windows.GetVolumeInformationByHandle(
		handle,
		nil,
		0,
		nil,
		nil,
		nil,
		&fileSystemName[0],
		uint32(len(fileSystemName)),
	); err != nil {
		return fmt.Errorf("identify %s file system: %w", purpose, err)
	}
	fileSystem := windows.UTF16ToString(fileSystemName[:])
	if !strings.EqualFold(fileSystem, "NTFS") {
		return fmt.Errorf("%s exact-generation authority requires NTFS; file system=%q", purpose, fileSystem)
	}
	return nil
}
