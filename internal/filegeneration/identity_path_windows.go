//go:build windows

package filegeneration

import (
	"fmt"
	"os"
	"strings"
)

func openPathForIdentity(path string) (*os.File, error) {
	normalized := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	if strings.HasPrefix(normalized, `\\.\pipe\`) {
		return nil, fmt.Errorf("named-pipe paths cannot provide a regular-file generation identity")
	}
	return os.Open(path)
}
