//go:build unix && !linux && !darwin

package hitraceconv

import (
	"context"
	"fmt"
)

func publishSealedConversionFilePlatform(
	context.Context,
	*sealedConversionFile,
	*privateConversionDir,
	string,
	string,
	string,
) (*retainedTraceDBPublication, error) {
	return nil, fmt.Errorf("retained trace DB exact-generation publication is unsupported on this Unix platform")
}
