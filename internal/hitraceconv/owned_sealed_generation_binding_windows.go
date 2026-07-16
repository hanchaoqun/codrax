//go:build windows

package hitraceconv

import "os"

// The Windows opener is already parent-relative, opens the final component
// with FILE_OPEN_REPARSE_POINT and rejects reparse/directory attributes.
func validateOwnedSealedGenerationPathBinding(string, os.FileInfo) error {
	return nil
}
