//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package filegeneration

import "os"

func openPathForIdentity(path string) (*os.File, error) {
	return os.Open(path)
}
