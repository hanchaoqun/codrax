//go:build !unix && !windows

package hitraceconv

import (
	"context"
	"fmt"
	"os"
)

type publishedConversionFilePlatformState struct{}

func duplicatePublishedConversionParentPlatform(*privateConversionDir) (publishedConversionFilePlatformState, error) {
	return publishedConversionFilePlatformState{}, fmt.Errorf("retained trace DB publication is unsupported on this platform")
}

func validatePublishedConversionFilePlatform(*publishedConversionFilePlatformState, string, *os.File, os.FileInfo) error {
	return fmt.Errorf("retained trace DB publication is unsupported on this platform")
}

func removePublishedConversionFilePlatform(*publishedConversionFilePlatformState, string, *os.File) error {
	return fmt.Errorf("retained trace DB publication is unsupported on this platform")
}

func closePublishedConversionFilePlatform(*publishedConversionFilePlatformState) error { return nil }

func publishSealedConversionFilePlatform(
	context.Context,
	*sealedConversionFile,
	*privateConversionDir,
	string,
	string,
	string,
) (*retainedTraceDBPublication, error) {
	return nil, fmt.Errorf("retained trace DB publication is unsupported on this platform")
}
