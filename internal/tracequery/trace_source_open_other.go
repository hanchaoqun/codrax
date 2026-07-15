//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package tracequery

import "os"

func openTraceSourcePath(path string) (*os.File, error) {
	return os.Open(path)
}
