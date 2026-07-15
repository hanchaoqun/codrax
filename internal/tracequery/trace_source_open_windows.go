//go:build windows

package tracequery

import (
	"fmt"
	"os"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func openTraceSourcePath(path string) (*os.File, error) {
	if filegeneration.IsWindowsNamedPipePath(path) {
		return nil, fmt.Errorf("trace source is not a regular file: named-pipe path=%q", path)
	}
	return os.Open(path)
}
