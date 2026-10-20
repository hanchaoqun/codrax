package hitraceconv

import (
	"context"
	"errors"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

type heldTextPublication struct {
	path, sha256, generation string
	bytes                    int64
}

// Shared by complete container transports. This copies an already held,
// fully admitted member, never inflates another time or publishes its preview.
// Callers must first check their private transport receipt against that input.
func publishHeldTraceText(ctx context.Context, opts Options, input conversionInputView, size int64, digest string, ledger *conversionFileLedger) (result heldTextPublication, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil || ledger == nil || size <= 0 || input.Size() != size || digest == "" || strings.TrimSpace(opts.OutputPath) == "" {
		return result, errors.New("held text publication contract is incomplete")
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStagePreCommit, nil); err != nil {
		return result, err
	}
	defer func() {
		// Preserve the existing transport fallback boundary: a cleanup,
		// cancellation or generation failure must dominate a data verdict.
		resultErr = gzipTextHardFailure(resultErr, errors.Join(ctx.Err(), input.Validate(conversionInputStagePreCommit)))
		if resultErr != nil {
			result = heldTextPublication{}
		}
	}()
	target, err := prepareSealedConversionPublicationTargetWithLedger(opts.OutputPath, ".codrax-trace-text-*", ledger)
	if err != nil {
		return result, err
	}
	defer func() { resultErr = gzipTextHardFailure(resultErr, target.Cleanup()) }()
	copy, err := createExternalToolInputSnapshot(ctx, input, target.stagingDir, target.finalLeaf, nil)
	if err != nil {
		return result, err
	}
	defer func() { resultErr = gzipTextHardFailure(resultErr, copy.Close()) }()
	sealed, err := copy.detachSealed()
	if err != nil {
		return result, err
	}
	defer func() { resultErr = gzipTextHardFailure(resultErr, sealed.Close()) }()
	if err := validateGzipTextSealed(ctx, sealed, sealed.identity, opts.OutputPath, size, digest); err != nil {
		return result, err
	}
	result = heldTextPublication{path: target.finalBindingPath, bytes: size, sha256: digest}
	if err := publishSealedConversionFileNoReplaceWithValidation(ctx, target, sealed, ledger, func(published *retainedTraceDBPublication) error {
		count, actual, generation, err := tracebundle.MeasureFile(ctx, published.file)
		if err != nil {
			return err
		}
		if count != size || actual != digest || !published.identity.SameVersion(generation) {
			return errors.New("published text differs from its held complete input")
		}
		result.generation = generation.CacheToken()
		return nil
	}); err != nil {
		return heldTextPublication{}, err
	}
	return result, nil
}
