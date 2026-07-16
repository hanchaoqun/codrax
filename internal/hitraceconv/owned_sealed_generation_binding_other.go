//go:build !windows

package hitraceconv

import (
	"fmt"
	"os"
)

func validateOwnedSealedGenerationPathBinding(path string, opened os.FileInfo) error {
	if opened == nil {
		return fmt.Errorf("owned sealed generation descriptor identity is missing")
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return fmt.Errorf("owned sealed generation path is not a plain regular binding")
	}
	return nil
}
