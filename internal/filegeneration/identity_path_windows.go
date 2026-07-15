//go:build windows

package filegeneration

import (
	"fmt"
	"os"
)

func openPathForIdentity(path string) (*os.File, error) {
	if IsWindowsNamedPipePath(path) {
		return nil, fmt.Errorf("named-pipe paths cannot provide a regular-file generation identity")
	}
	return os.Open(path)
}
