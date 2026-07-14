//go:build unix && !darwin && !linux

package hitraceconv

import "fmt"

// Other Unix ACL models (notably NFSv4 ACLs) are not proven equivalent to a
// POSIX mode mask. Fail closed instead of labelling 0700 alone as exact
// private-directory security.
func securePrivateConversionDirUnixPlatform(int) error {
	return fmt.Errorf("private conversion directory ACL security is unsupported on this Unix platform")
}

func validatePrivateConversionDirUnixSecurityPlatform(int) error {
	return fmt.Errorf("private conversion directory ACL security is unsupported on this Unix platform")
}
