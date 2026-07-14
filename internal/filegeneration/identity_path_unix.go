//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package filegeneration

import (
	"os"
	"syscall"
)

// O_NONBLOCK prevents a path-binding check from hanging when an attacker
// swaps the regular source path to a FIFO. It has no effect on regular files.
func openPathForIdentity(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
