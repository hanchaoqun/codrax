//go:build !unix && !windows

package hitraceconv

import (
	"fmt"
	"os"
)

func validateExternalToolInputSourcePlatform(conversionInputView) error {
	return fmt.Errorf("external tool input snapshots are unsupported on this platform")
}

func validateExternalToolInputSnapshotDirPlatform(*privateConversionDirPlatformState) error {
	return fmt.Errorf("external tool input snapshots are unsupported on this platform")
}

func createExternalToolInputSnapshotFilePlatform(*privateConversionDirPlatformState, string) (*os.File, error) {
	return nil, fmt.Errorf("external tool input snapshots are unsupported on this platform")
}

func freezeExternalToolInputSnapshotFilePlatform(
	_ *privateConversionDirPlatformState,
	_ string,
	file *os.File,
	_ os.FileInfo,
) (*os.File, os.FileInfo, error) {
	var closeErr error
	if file != nil {
		closeErr = file.Close()
	}
	return nil, nil, traceDBJoinPreservingSingle(
		fmt.Errorf("external tool input snapshots are unsupported on this platform"), closeErr,
	)
}
