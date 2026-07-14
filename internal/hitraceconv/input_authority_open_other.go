//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris

package hitraceconv

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

func openConversionInputFile(path string) (*os.File, error) {
	if runtime.GOOS == "windows" {
		normalized := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
		if strings.HasPrefix(normalized, `\\.\pipe\`) {
			return nil, fmt.Errorf("named-pipe input cannot be converted as a regular trace file")
		}
	}
	return os.Open(path)
}
