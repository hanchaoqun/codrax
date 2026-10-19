//go:build !darwin

package traceinput

import (
	"fmt"
	"os"
)

func preparationNativeEntryName(_ string, _ os.FileInfo) (string, error) {
	return "", fmt.Errorf("native search-only entry identity is unavailable on this platform")
}
