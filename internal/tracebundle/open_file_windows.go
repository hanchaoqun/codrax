//go:build windows

package tracebundle

import (
	"fmt"
	"os"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func openManifestFile(path string) (*os.File, error) {
	return os.Open(path)
}

// Avoid handing the Win32 named-pipe namespace to os.Stat/os.Open: both are
// allowed to wait for a server and therefore cannot be a bounded manifest
// intake operation.
func preflightManifestPath(path string) error {
	if filegeneration.IsWindowsNamedPipePath(path) {
		return fmt.Errorf("%w: named-pipe path=%q", ErrNotRegular, path)
	}
	return nil
}
