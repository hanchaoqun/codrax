//go:build !unix && !windows

package hitraceconv

import (
	"context"
	"fmt"
	"os"
)

type publishedConversionFilePlatformState struct{}

func duplicatePublishedConversionParentPlatform(_ *privateConversionDir, kind sealedConversionPublicationKind) (publishedConversionFilePlatformState, error) {
	return publishedConversionFilePlatformState{}, fmt.Errorf("%s publication is unsupported on this platform", kind.diagnosticName())
}

func validatePublishedConversionFilePlatform(_ *publishedConversionFilePlatformState, _ string, _ *os.File, _ os.FileInfo, kind sealedConversionPublicationKind) error {
	return fmt.Errorf("%s publication is unsupported on this platform", kind.diagnosticName())
}

func removePublishedConversionFilePlatform(_ *publishedConversionFilePlatformState, _ string, _ *os.File, kind sealedConversionPublicationKind) error {
	return fmt.Errorf("%s publication is unsupported on this platform", kind.diagnosticName())
}

func closePublishedConversionFilePlatform(*publishedConversionFilePlatformState) error { return nil }

func publishSealedConversionFilePlatform(
	_ context.Context,
	_ *sealedConversionFile,
	_ *privateConversionDir,
	_ string,
	_ string,
	_ string,
	kind sealedConversionPublicationKind,
) (*retainedTraceDBPublication, error) {
	return nil, fmt.Errorf("%s publication is unsupported on this platform", kind.diagnosticName())
}
