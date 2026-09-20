package hitraceconv

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

// PrepareFile is the default file-attachment policy over the same conversion
// transaction. Only the decoded semantic route decides DB retention; explicit
// ConvertFile callers retain their own option choices and conflict checks.
func PrepareFile(ctx context.Context, opts Options) (Result, error) {
	opts.prepareInputDefaults = true
	return ConvertFile(ctx, opts)
}

func validateGzipTextOptions(opts Options) error {
	if requestedTraceEngineMode(opts.TraceEngine) == traceEngineTraceStreamer || opts.KeepTraceDB ||
		strings.TrimSpace(opts.TraceDBOutputPath) != "" || strings.TrimSpace(opts.TraceStreamerPath) != "" ||
		len(opts.TraceStreamerSoDirs) > 0 {
		return fmt.Errorf("gzip contains text, not a binary trace body: database and explicit trace_streamer options cannot be applied to byte-preserving text transport")
	}
	return nil
}

func prepareTraceGzipInput(ctx context.Context, opts Options, route *traceConversionInput) (_ *traceConversionInput, err error) {
	defer func() {
		err = completeConversionInputStage(ctx, route.archive, conversionInputStageArchiveIntake, err)
	}()
	output := strings.TrimSpace(opts.OutputPath)
	if output == "" {
		output = DefaultOutputPath(route.archive.DisplayPath())
	}
	staging, err := newRuntimePrivateConversionDir(opts.RuntimeAnchor, "."+filepath.Base(output)+".*.gzip")
	if err != nil {
		return route, err
	}
	route.staging = staging
	input, receipt, err := prepareGzipBinaryInput(ctx, opts, route.archive, staging)
	if err != nil {
		return route, err
	}
	route.gzip, route.input, route.gzipReceipt = input, input, receipt
	if receipt.DecodedBytes == 0 {
		return route, gzipTextFailure(GzipTextCodeDecodedEmpty, fmt.Errorf("decoded capture is empty"))
	}
	var prefix [32]byte
	n, err := input.ReadAt(prefix[:], 0)
	if err != nil && err != io.EOF {
		return route, err
	}
	format := gzipTextDecodedBinaryFormat(prefix[:n])
	if format == "" {
		// Complete text admission is a transport contract, not a keyword or
		// event-count test. Nested/unknown binary containers remain rejected.
		if err := attachment.ValidateTextReaderAtFull(ctx, attachment.KindTrace, route.archive.DisplayPath(), input, input.Size(), attachment.TracePhysicalLineMaxBytes); err != nil {
			return route, gzipTextFailure(GzipTextCodeDecodedText, err)
		}
		route.gzipText = true
		return route, nil
	}
	route.gzipProvenance = &tracebundle.GzipInputProvenance{
		Profile: tracebundle.GzipInputProfileV1, SourceBytes: receipt.SourceBytes, SourceSHA256: receipt.SourceSHA256,
		SourceGeneration: route.archive.identity.CacheToken(), DecodedFormat: format,
		DecodedBytes: receipt.DecodedBytes, DecodedSHA256: receipt.DecodedSHA256,
		DecodedGeneration: receipt.DecodedGeneration,
	}
	if err := tracebundle.ValidateGzipInputProvenance(route.gzipProvenance); err != nil {
		return route, err
	}
	// A fixed transport coordinate cannot borrow gzip Name/MTIME or private
	// staging paths as source identity or trace-clock authority.
	route.namespace = route.archive.CanonicalPath() + "!/gzip"
	return route, nil
}
