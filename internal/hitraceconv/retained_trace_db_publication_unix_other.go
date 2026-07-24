//go:build unix && !linux && !darwin

package hitraceconv

import (
	"context"
	"fmt"
)

func publishSealedConversionFilePlatform(
	_ context.Context,
	_ *sealedConversionFile,
	_ *privateConversionDir,
	_ *publishedConversionFilePlatformState,
	_ string,
	_ string,
	_ string,
	kind sealedConversionPublicationKind,
) (*retainedTraceDBPublication, error) {
	return nil, fmt.Errorf("%s exact-generation publication is unsupported on this Unix platform", kind.diagnosticName())
}
