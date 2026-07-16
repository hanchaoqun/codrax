//go:build !windows

package hitraceconv

import "os"

func openOwnedSealedGenerationFile(path string) (*os.File, error) {
	return openConversionInputFile(path)
}
