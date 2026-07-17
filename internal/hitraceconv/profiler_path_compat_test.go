package hitraceconv

import (
	"context"
	"fmt"
)

// profilerPathTestBinding preserves the historical path-shaped test helpers
// without leaving a second path-opening parser in production. Every adapter
// opens one local authority and delegates to the same ReaderAt cores used by
// ConvertFile.
func profilerPathTestBinding(path string) (*conversionInputAuthority, *profilerInputBinding, error) {
	authority, err := openConversionInputAuthority(path)
	if err != nil {
		return nil, nil, err
	}
	binding, err := newProfilerInputBinding(authority, authority.CanonicalPath())
	if err != nil {
		_ = authority.Close()
		return nil, nil, err
	}
	return authority, binding, nil
}

func tryConvertProfilerContainer(ctx context.Context, opts Options, inputSize int64, output string, standaloneArtifacts []Artifact, standaloneCaveats []string, standaloneDecisions []PerfProviderDecision, initialTraceDecisions []TraceProviderDecision, initialTraceDBCoverage []TraceDBCoverage) (result Result, detected bool, err error) {
	authority, _, err := profilerPathTestBinding(opts.InputPath)
	if err != nil {
		return Result{}, false, err
	}
	defer func() {
		if closeErr := authority.Close(); closeErr != nil {
			result = Result{}
			detected = false
			err = traceDBJoinPreservingSingle(err, closeErr)
		}
	}()
	if inputSize != authority.Size() {
		return Result{}, false, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageProfilerHeader,
			authority.DisplayPath(),
			fmt.Errorf("profiler test input size %d does not match authority size %d", inputSize, authority.Size()),
		)
	}
	ledger, err := newConversionFileLedgerForAuthority(authority)
	if err != nil {
		return Result{}, false, err
	}
	committed := false
	defer func() {
		if !committed {
			err = joinConversionCleanupError(err, ledger)
		}
	}()
	standaloneInventory, inventoryErr := findStandaloneSegmentsFromInput(ctx, authority)
	if inventoryErr != nil {
		return Result{}, false, inventoryErr
	}
	result, detected, err = tryConvertProfilerContainerWithLedger(ctx, opts, authority, output,
		standaloneInventory, standaloneArtifacts, standaloneCaveats, standaloneDecisions,
		initialTraceDecisions, initialTraceDBCoverage, ledger)
	if err == nil {
		if validateErr := authority.Validate(conversionInputStagePreCommit); validateErr != nil {
			err = validateErr
		} else if closeErr := authority.Close(); closeErr != nil {
			err = traceDBJoinPreservingSingle(ctx.Err(), closeErr)
		} else if validateErr := ledger.validateOwnedPaths(); validateErr != nil {
			err = validateErr
		} else if releaseErr := ledger.releaseOwnedAuthorities(); releaseErr != nil {
			err = fmt.Errorf("release profiler test publication authorities: %w", releaseErr)
		} else {
			committed = true
		}
		if err != nil {
			result = Result{}
		}
	}
	return result, detected, err
}

func extractProfilerContainerSystraceRows(ctx context.Context, path string, inputSize int64, sink *traceDBRowSink) (profilerContainerExtraction, error) {
	return extractProfilerContainerSystraceRowsWithSessionLimit(ctx, path, inputSize, inputSize, sink)
}

func extractProfilerContainerSystraceRowsWithSessionLimit(ctx context.Context, path string,
	inputSize, sessionInputSize int64, sink *traceDBRowSink,
) (extracted profilerContainerExtraction, err error) {
	authority, binding, err := profilerPathTestBinding(path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, authority.Close())
		if err != nil {
			extracted = profilerContainerExtraction{}
		}
	}()
	if inputSize != binding.inputSize {
		return profilerContainerExtraction{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageProfilerHeader,
			binding.input.DisplayPath(),
			fmt.Errorf("profiler test input size %d does not match authority size %d", inputSize, binding.inputSize),
		)
	}
	inventory, err := findStandaloneSegmentsFromInput(ctx, authority)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	return extractProfilerContainerSystraceRowsWithSessionLimitFromInput(
		ctx, binding, sessionInputSize, inventory.rootProof, sink)
}

func extractProfilerTraceFile(ctx context.Context, path string, inputSize int64, header profilerTraceHeader, sink *traceDBRowSink) (profilerContainerExtraction, error) {
	return extractProfilerTraceFileWithFrameLimit(ctx, path, inputSize, header, sink, maxProfilerPluginFrameBytes)
}

func extractProfilerTraceFileWithFrameLimit(ctx context.Context, path string, inputSize int64,
	header profilerTraceHeader, sink *traceDBRowSink, maxFrameBytes uint64,
) (extracted profilerContainerExtraction, err error) {
	authority, binding, err := profilerPathTestBinding(path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, authority.Close())
		if err != nil {
			extracted = profilerContainerExtraction{}
		}
	}()
	if inputSize != binding.inputSize {
		return profilerContainerExtraction{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageProfilerBody,
			binding.input.DisplayPath(),
			fmt.Errorf("profiler test input size %d does not match authority size %d", inputSize, binding.inputSize),
		)
	}
	rootProofValue, err := validateProfilerRootProfileEnvelope(
		ctx, authority, header, inputSize, maxFrameBytes)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	rootProof := &rootProofValue
	return extractProfilerTraceFileFromInput(
		ctx, binding, inputSize, header, rootProof, sink, maxFrameBytes)
}

func extractProfilerSessionPackage(ctx context.Context, path string, inputSize int64,
	sink *traceDBRowSink,
) (profilerContainerExtraction, error) {
	return extractProfilerSessionPackageWithLineLimit(ctx, path, inputSize, sink, maxProfilerTextLineBytes)
}

func extractProfilerSessionPackageWithLineLimit(ctx context.Context, path string, inputSize int64,
	sink *traceDBRowSink, maxLineBytes int,
) (extracted profilerContainerExtraction, err error) {
	authority, binding, err := profilerPathTestBinding(path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, authority.Close())
		if err != nil {
			extracted = profilerContainerExtraction{}
		}
	}()
	return extractProfilerSessionPackageFromInput(ctx, binding, inputSize, sink, maxLineBytes)
}

func readProfilerTraceHeaderAtPath(path string, off int64, fileSize int64) (header profilerTraceHeader, ok bool, err error) {
	authority, binding, err := profilerPathTestBinding(path)
	if err != nil {
		return profilerTraceHeader{}, false, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, authority.Close())
		if err != nil {
			header = profilerTraceHeader{}
			ok = false
		}
	}()
	if off != 0 || fileSize != binding.inputSize {
		return profilerTraceHeader{}, false, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageProfilerHeader,
			binding.input.DisplayPath(),
			fmt.Errorf("profiler test header range offset=%d size=%d authority_size=%d", off, fileSize, binding.inputSize),
		)
	}
	return readProfilerTraceHeaderFromInput(context.Background(), binding)
}
