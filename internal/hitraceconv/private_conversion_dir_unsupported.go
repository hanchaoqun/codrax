//go:build !unix && !windows

package hitraceconv

import (
	"fmt"
	"os"
)

type privateConversionDirPlatformState struct{}

func validatePrivateConversionDirChildNamePlatform(string) error { return nil }

func validatePrivateConversionDirPublicBindingPlatform(string, os.FileInfo, *privateConversionDirPlatformState) error {
	return fmt.Errorf("private conversion directories are unsupported on this platform")
}

func openPrivateConversionDirRootPlatform(string, os.FileInfo, *privateConversionDirPlatformState) (*os.Root, error) {
	return nil, fmt.Errorf("private conversion directories are unsupported on this platform")
}

func removePrivateConversionDirChildrenPlatform(string, os.FileInfo, *privateConversionDirPlatformState) error {
	return fmt.Errorf("private conversion directories are unsupported on this platform")
}

func createPrivateConversionDirPlatform(_, _ string) (string, os.FileInfo, privateConversionDirPlatformState, error) {
	return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("private conversion directories are unsupported on this platform")
}

func validatePrivateConversionDirIdentityPlatform(string, os.FileInfo, *privateConversionDirPlatformState) error {
	return fmt.Errorf("private conversion directories are unsupported on this platform")
}

func validatePrivateConversionDirSecurityPlatform(string, os.FileInfo, *privateConversionDirPlatformState) error {
	return fmt.Errorf("private conversion directories are unsupported on this platform")
}

func preparePrivateConversionDirCleanupPlatform(string, os.FileInfo, *os.Root, *privateConversionDirPlatformState) error {
	return fmt.Errorf("private conversion directories are unsupported on this platform")
}

func removePrivateConversionDirRootPlatform(string, os.FileInfo, *privateConversionDirPlatformState) error {
	return fmt.Errorf("private conversion directories are unsupported on this platform")
}

func closePrivateConversionDirPlatform(*privateConversionDirPlatformState) error { return nil }
