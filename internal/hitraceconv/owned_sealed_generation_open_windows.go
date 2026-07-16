//go:build windows

package hitraceconv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// openOwnedSealedGenerationFile reopens a converter-owned public generation
// without weakening the original held publication handle. Windows share
// checks are bidirectional: that held handle owns DELETE/WRITE_ATTRIBUTES, so
// every read-only verifier must share READ, WRITE and DELETE even though the
// verifier itself receives no mutation access.
func openOwnedSealedGenerationFile(path string) (*os.File, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("owned sealed generation path is empty")
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	leaf := filepath.Base(abs)
	if err := validatePrivateConversionDirChildNamePlatform(leaf); err != nil {
		return nil, fmt.Errorf("owned sealed generation leaf is invalid: %w", err)
	}
	canonicalParent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return nil, fmt.Errorf("resolve owned sealed generation parent: %w", err)
	}
	parent, err := openPrivateConversionDirWindowsParent(canonicalParent)
	if err != nil {
		return nil, err
	}
	file, _, openErr := openPublishedConversionRegularChildWindowsWithAccess(
		parent,
		leaf,
		sealedConversionPublicationOutput,
		windows.FILE_READ_DATA|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
	)
	closeErr := windows.CloseHandle(parent)
	if openErr != nil || closeErr != nil {
		if file != nil {
			closeErr = traceDBJoinPreservingSingle(closeErr, file.Close())
		}
		return nil, traceDBJoinPreservingSingle(openErr, closeErr)
	}
	return file, nil
}
