package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const (
	GzipTextTransportProfile = "gzip_trace_text_v1"
	// MaxGzipTraceInputBytes lets callers reject over-budget physical inputs
	// before any full-source hashing at a surrounding preparation boundary.
	MaxGzipTraceInputBytes = hiperfGzipMaxCompressedBytes
)

const (
	GzipTextCodeInvalidHeader = "gzip_invalid_header"
	GzipTextCodeIntegrity     = "gzip_integrity_failed"
	GzipTextCodeResourceLimit = "gzip_resource_limit"
	GzipTextCodeTrailingData  = "gzip_trailing_data"
	GzipTextCodeDecodedBinary = "gzip_decoded_binary"
	GzipTextCodeDecodedText   = "gzip_decoded_text_invalid"
	GzipTextCodeDecodedEmpty  = "gzip_decoded_empty"
)

// GzipTextTransportResult attests only byte transport and complete text
// admission. It grants no producer, event, clock or causal-analysis authority.
type GzipTextTransportResult struct {
	Profile           string `json:"profile"`
	SourcePath        string `json:"source_path"`
	SourceBytes       int64  `json:"source_bytes"`
	SourceSHA256      string `json:"source_sha256"`
	SourceGeneration  string `json:"source_generation"`
	DecodedPath       string `json:"decoded_path"`
	DecodedBytes      int64  `json:"decoded_bytes"`
	DecodedSHA256     string `json:"decoded_sha256"`
	DecodedGeneration string `json:"decoded_generation"`
}

// GzipTextTransportError is a data-local rejection. Only DecodedBinary, with
// its complete source receipt, permits a caller to consider another existing
// decoder. Filesystem, cancellation, source-generation and cleanup failures
// never retain this error type as an unwrap-able fallback authorization.
type GzipTextTransportError struct {
	Code             string
	DecodedFormat    string
	SourcePath       string
	SourceBytes      int64
	SourceSHA256     string
	SourceGeneration string
	Cause            error
}

func (err *GzipTextTransportError) Error() string {
	if err == nil {
		return "gzip trace transport rejected"
	}
	message := "gzip trace transport rejected: code=" + err.Code
	if err.DecodedFormat != "" {
		message += " decoded_format=" + err.DecodedFormat
	}
	if err.Cause != nil {
		message += ": " + err.Cause.Error()
	}
	return message
}

func (err *GzipTextTransportError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func gzipTextFailure(code string, cause error) error {
	return &GzipTextTransportError{Code: code, Cause: cause}
}

// gzipTextHardFailure deliberately does not wrap a provisional data verdict:
// otherwise errors.As could still authorize fallback after a cleanup failure.
func gzipTextHardFailure(verdict, hard error) error {
	if hard == nil {
		return verdict
	}
	if verdict == nil {
		return hard
	}
	return fmt.Errorf("gzip text transport aborted (provisional verdict: %v): %w", verdict, hard)
}

// PrepareGzipTraceText decodes exactly one gzip member, validates every decoded
// byte, then publishes a no-replace text generation. Input and gzip metadata
// are never rewritten or used to infer clocks. The only consumed Options are
// InputPath, OutputPath, RuntimeAnchor, RuntimeAnchorFallback and Progress.
func PrepareGzipTraceText(ctx context.Context, opts Options) (result GzipTextTransportResult, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := attachment.ValidateSourceLabel(opts.InputPath); err != nil {
		return result, err
	}
	if strings.TrimSpace(opts.OutputPath) == "" {
		return result, errors.New("gzip text output path is required")
	}
	if err := attachment.ValidateSourceLabel(opts.OutputPath); err != nil {
		return result, err
	}
	input, err := openConversionInputAuthority(opts.InputPath)
	if err != nil {
		return result, err
	}
	var ledger *conversionFileLedger
	var target sealedConversionPublicationTarget
	var sealed *sealedConversionFile
	published := false
	defer func() {
		// All hard gates complete before publication authorities are released.
		hard := errors.Join(ctx.Err(), input.Validate(conversionInputStagePreCommit))
		if sealed != nil && !published {
			hard = errors.Join(hard, sealed.Validate())
		}
		if sealed != nil {
			hard = errors.Join(hard, sealed.Close())
		}
		if target.Cleanup != nil {
			hard = errors.Join(hard, target.Cleanup())
		}
		hard = errors.Join(hard, input.Validate(conversionInputStagePreCommit), input.Close(), ctx.Err())
		resultErr = gzipTextHardFailure(resultErr, hard)
		if resultErr == nil && ledger != nil {
			resultErr = ledger.validateOwnedPaths()
		}
		if resultErr == nil {
			resultErr = ctx.Err()
		}
		if resultErr != nil {
			if ledger != nil {
				resultErr = gzipTextHardFailure(resultErr, ledger.cleanup())
			}
			result = GzipTextTransportResult{}
			return
		}
		if ledger != nil {
			resultErr = ledger.releaseOwnedAuthorities()
		}
		if resultErr != nil {
			result = GzipTextTransportResult{}
		}
	}()
	ledger, err = newConversionFileLedgerForAuthority(input)
	if err != nil {
		return result, err
	}
	anchor, err := resolveConversionRuntimeAnchor(opts.RuntimeAnchor, opts.OutputPath)
	if err != nil {
		return result, err
	}
	ledger.stagingRoot, err = selectSecureConversionRuntimeAnchor(anchor, opts.RuntimeAnchorFallback)
	if err != nil {
		return result, err
	}
	checked := &gzipTextCheckedInput{conversionInputView: input}
	if err := preflightGzipInput(checked); err != nil {
		return result, err
	}
	target, err = prepareSealedConversionPublicationTargetWithLedger(opts.OutputPath, ".codrax-gzip-text-*", ledger)
	if err != nil {
		return result, err
	}
	started := progressStarted(opts, "gzip_text_decompress", "decoding complete gzip trace text", input.DisplayPath(), opts.OutputPath)
	defer func() {
		status := ProgressStatusComplete
		if resultErr != nil {
			status = ProgressStatusFailed
		}
		progressFinished(opts, "gzip_text_decompress", "gzip trace text decode "+string(status), input.DisplayPath(), opts.OutputPath, started, status)
	}()
	var decodedGeneration filegeneration.Identity
	result, decodedGeneration, err = inflateGzipTraceText(ctx, opts, checked, target, started)
	if err != nil {
		return GzipTextTransportResult{}, err
	}
	result.SourceGeneration = input.identity.CacheToken()
	sealed, err = target.stagingDir.AdoptRegularChild(target.finalLeaf, false)
	if err != nil {
		return GzipTextTransportResult{}, err
	}
	if err := validateGzipTextSealed(ctx, sealed, decodedGeneration, result.DecodedPath, result.DecodedBytes, result.DecodedSHA256); err != nil {
		return GzipTextTransportResult{}, err
	}
	if result.DecodedBytes == 0 {
		return GzipTextTransportResult{}, gzipTextFailure(GzipTextCodeDecodedEmpty, errors.New("decoded capture is empty; provide a non-empty capture"))
	}
	var prefix [32]byte
	n, err := sealed.ReadAt(prefix[:], 0)
	if err != nil && err != io.EOF {
		return GzipTextTransportResult{}, err
	}
	if format := gzipTextDecodedBinaryFormat(prefix[:n]); format != "" {
		return GzipTextTransportResult{}, &GzipTextTransportError{
			Code: GzipTextCodeDecodedBinary, DecodedFormat: format,
			SourcePath: result.SourcePath, SourceBytes: result.SourceBytes,
			SourceSHA256: result.SourceSHA256, SourceGeneration: result.SourceGeneration,
			Cause: errors.New("decoded capture is binary, not trace text; use its supported binary decoder"),
		}
	}
	if err := attachment.ValidateTextReaderAtFull(ctx, attachment.KindTrace, result.SourcePath, sealed, sealed.Size(), attachment.TracePhysicalLineMaxBytes); err != nil {
		var issue attachment.TextIssue
		if errors.As(err, &issue) {
			return GzipTextTransportResult{}, gzipTextFailure(GzipTextCodeDecodedText, err)
		}
		return GzipTextTransportResult{}, err
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStagePreCommit, nil); err != nil {
		return GzipTextTransportResult{}, err
	}
	if err := publishSealedConversionFileNoReplaceWithValidation(ctx, target, sealed, ledger, func(published *retainedTraceDBPublication) error {
		_, digest, generation, err := tracebundle.MeasureFile(ctx, published.file)
		if err != nil {
			return err
		}
		if published.size != result.DecodedBytes || digest != result.DecodedSHA256 || !published.identity.SameVersion(generation) {
			return errors.New("published gzip text generation differs from decoded bytes")
		}
		result.DecodedGeneration = generation.CacheToken()
		return nil
	}); err != nil {
		return GzipTextTransportResult{}, err
	}
	published = true
	return result, nil
}

// Only these exact capture magics are eligible for another semantic decoder.
// Nested containers and SQLite deliberately remain terminal text rejections.
func gzipTextDecodedBinaryFormat(prefix []byte) string {
	switch format := attachment.KnownBinaryTraceFormat(prefix); format {
	case attachment.BinaryTraceFormatHarmonyRMQ, attachment.BinaryTraceFormatOHOSProfile, attachment.BinaryTraceFormatLinuxPerf:
		return string(format)
	}
	if hasPrefixBytes(prefix, []byte("SIMPLEPERF")) {
		return string(perfInputSimpleperfReportProto)
	}
	if hasPrefixBytes(prefix, []byte{0x49, 0xdf}) {
		return "openharmony_raw"
	}
	return ""
}

type gzipTextCheckedInput struct {
	conversionInputView
	readErr error
}

func (input *gzipTextCheckedInput) ReadAt(buffer []byte, offset int64) (int, error) {
	n, err := input.conversionInputView.ReadAt(buffer, offset)
	if err != nil && err != io.EOF {
		input.readErr = err
	}
	return n, err
}

func inflateGzipTraceText(ctx context.Context, opts Options, input *gzipTextCheckedInput, target sealedConversionPublicationTarget, started time.Time) (result GzipTextTransportResult, generation filegeneration.Identity, resultErr error) {
	if err := preflightGzipInput(input); err != nil {
		return result, generation, err
	}
	out, err := createExternalToolInputSnapshotFile(target.stagingDir, target.finalLeaf)
	if err != nil {
		return result, generation, err
	}
	defer func() { resultErr = gzipTextHardFailure(resultErr, out.Close()) }()
	receipt, err := inflateGzipToWriter(ctx, opts, input, out, "gzip_text_decompress", started)
	if err != nil {
		return result, generation, err
	}
	if err := out.Sync(); err != nil {
		return result, generation, err
	}
	generation, err = filegeneration.FromFile(out)
	if err != nil {
		return result, generation, err
	}
	if !generation.Strong() || generation.Size() != receipt.DecodedBytes {
		return result, generation, errors.New("decoded gzip writer generation is unavailable or changed")
	}
	return GzipTextTransportResult{
		Profile: GzipTextTransportProfile, SourcePath: input.DisplayPath(), SourceBytes: receipt.SourceBytes, SourceSHA256: receipt.SourceSHA256,
		DecodedPath: target.finalBindingPath, DecodedBytes: receipt.DecodedBytes, DecodedSHA256: receipt.DecodedSHA256,
	}, generation, nil
}

func validateGzipTextSealed(ctx context.Context, sealed *sealedConversionFile, generation filegeneration.Identity, path string, size int64, digest string) error {
	if err := sealed.Validate(); err != nil {
		return err
	}
	if !generation.Strong() || !generation.SameVersion(sealed.identity) {
		return errors.New("decoded gzip private generation changed before adoption")
	}
	if sealed.Size() != size {
		return errors.New("private gzip text size differs from decoded size")
	}
	err := sealed.withOpenFile(func(file *os.File) error {
		count, actual, _, err := tracebundle.MeasureFile(ctx, file)
		if err != nil {
			return err
		}
		if count != size || actual != digest {
			return fmt.Errorf("decoded gzip text bytes changed before validation: %s", path)
		}
		return nil
	})
	return errors.Join(err, sealed.Validate())
}
