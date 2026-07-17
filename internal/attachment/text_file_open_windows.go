//go:build windows

package attachment

import (
	"fmt"
	"os"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func openTextSourceFile(path string) (*os.File, error) {
	if filegeneration.IsWindowsNamedPipePath(path) {
		return nil, fmt.Errorf("attached text source is not a regular file: named-pipe path=%q", path)
	}
	return os.Open(path)
}
