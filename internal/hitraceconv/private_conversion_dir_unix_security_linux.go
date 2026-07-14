//go:build linux

package hitraceconv

// Linux POSIX ACLs are constrained by the mode's ACL mask. Fchmod(0700)
// therefore removes all effective group/other grants, including inherited
// named-user/group entries. The held-inode mode and owner checks remain the
// authoritative validation on this platform.
func securePrivateConversionDirUnixPlatform(int) error { return nil }

func validatePrivateConversionDirUnixSecurityPlatform(int) error { return nil }
