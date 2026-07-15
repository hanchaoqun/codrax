//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package tracebundle

import "os"

func openManifestFile(path string) (*os.File, error) {
	return os.Open(path)
}

func preflightManifestPath(string) error { return nil }
