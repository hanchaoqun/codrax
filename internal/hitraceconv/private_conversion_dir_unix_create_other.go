//go:build unix && !darwin

package hitraceconv

import "golang.org/x/sys/unix"

func createPrivateConversionDirUnixPlatform(parentFD int, _ string, leaf string) error {
	return unix.Mkdirat(parentFD, leaf, 0o700)
}

func validatePrivateConversionDirUnixBirthSecurityPlatform(int) error { return nil }

func removePrivateConversionDirUnixCreationPlatform(parentFD int, leaf string, creatorBound bool) error {
	if !creatorBound {
		return nil
	}
	return unix.Unlinkat(parentFD, leaf, unix.AT_REMOVEDIR)
}
