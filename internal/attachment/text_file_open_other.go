//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package attachment

import "os"

func openTextSourceFile(path string) (*os.File, error) {
	return os.Open(path)
}
