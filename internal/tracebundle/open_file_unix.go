//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package tracebundle

import (
	"os"
	"syscall"
)

// O_NONBLOCK closes the stat-to-open FIFO swap lane without changing regular
// file behavior.
func openManifestFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

func preflightManifestPath(string) error { return nil }
