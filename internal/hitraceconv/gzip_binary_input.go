package hitraceconv

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const gzipBinaryInputSnapshotLeaf = "gzip_decoded_input.sys"

// gzipInputReceipt describes transport bytes only. DecodedGeneration is the
// private creator generation, not a public artifact or semantic capability.
type gzipInputReceipt struct {
	SourceBytes       int64
	SourceSHA256      string
	DecodedBytes      int64
	DecodedSHA256     string
	DecodedGeneration string
}

// gzipBinaryInput retains any decoded payload, including text. Its caller is
// the sole route-selection authority. It owns only snapshot's held file; the
// caller retains source and the private directory through final conversion.
type gzipBinaryInput struct {
	source   conversionInputView
	snapshot *externalToolInputSnapshot
	receipt  gzipInputReceipt
}

func (input *gzipBinaryInput) Size() int64 {
	if input == nil || input.snapshot == nil {
		return 0
	}
	return input.snapshot.size
}

func (input *gzipBinaryInput) DisplayPath() string {
	if input == nil || input.source == nil {
		return ""
	}
	return input.source.DisplayPath()
}

func (input *gzipBinaryInput) ReadAt(buffer []byte, offset int64) (int, error) {
	if input == nil || input.snapshot == nil {
		return 0, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageRoute, input.DisplayPath(), errors.New("gzip input is closed"))
	}
	input.snapshot.mu.RLock()
	defer input.snapshot.mu.RUnlock()
	if input.snapshot.closed || input.snapshot.file == nil {
		return 0, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageRoute, input.DisplayPath(), errors.New("gzip input is closed"))
	}
	if offset < 0 {
		return 0, conversionInputFailure(ConversionInputCodeInvalidRange, conversionInputStageRoute, input.DisplayPath(), nil)
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	if offset >= input.snapshot.size {
		return 0, io.EOF
	}
	remaining := input.snapshot.size - offset
	limited := buffer
	truncated := int64(len(buffer)) > remaining
	if truncated {
		limited = buffer[:int(remaining)]
	}
	n, err := input.snapshot.file.ReadAt(limited, offset)
	if err == nil && truncated {
		err = io.EOF
	}
	return n, err
}

func (input *gzipBinaryInput) Validate(stage conversionInputStage) error {
	if input == nil || input.source == nil || input.snapshot == nil || !stage.valid() {
		return conversionInputFailure(ConversionInputCodeInternalContract, stage, input.DisplayPath(), errors.New("gzip input view contract is incomplete"))
	}
	if err := input.source.Validate(stage); err != nil {
		return err
	}
	if err := input.snapshot.Validate(); err != nil {
		return conversionInputFailure(ConversionInputCodeGenerationChanged, stage, input.DisplayPath(), err)
	}
	if input.receipt.SourceBytes != input.source.Size() || input.receipt.DecodedBytes != input.snapshot.size ||
		input.receipt.DecodedGeneration != input.snapshot.identity.CacheToken() {
		return conversionInputFailure(ConversionInputCodeInternalContract, stage, input.DisplayPath(), errors.New("gzip input receipt differs from its bound generations"))
	}
	return input.source.Validate(stage)
}

func (input *gzipBinaryInput) withOpenFile(fn func(*os.File) error) error {
	if input == nil || input.snapshot == nil || fn == nil {
		return conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageExternalTool, input.DisplayPath(), errors.New("gzip input file callback is incomplete"))
	}
	input.snapshot.mu.RLock()
	defer input.snapshot.mu.RUnlock()
	if input.snapshot.closed || input.snapshot.file == nil {
		return conversionInputFailure(ConversionInputCodeClosed, conversionInputStageExternalTool, input.DisplayPath(), errors.New("gzip input is closed"))
	}
	return fn(input.snapshot.file)
}

func (*gzipBinaryInput) externalToolWholeFileSource()      {}
func (*gzipBinaryInput) traceStreamerSnapshotLeaf() string { return gzipBinaryInputSnapshotLeaf }
func (input *gzipBinaryInput) Close() error {
	if input == nil || input.snapshot == nil {
		return nil
	}
	return input.snapshot.Close()
}

func prepareGzipBinaryInput(ctx context.Context, opts Options, source conversionInputView, staging *privateConversionDir) (result *gzipBinaryInput, receipt gzipInputReceipt, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || staging == nil {
		return nil, receipt, conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageRoute, "", errors.New("gzip input preparation contract is incomplete"))
	}
	if err := completeConversionInputStage(ctx, source, conversionInputStageRoute, nil); err != nil {
		return nil, receipt, err
	}
	defer func() {
		hard := errors.Join(ctx.Err(), source.Validate(conversionInputStageRoute), staging.Validate())
		if result != nil {
			hard = errors.Join(hard, result.Validate(conversionInputStageRoute))
		}
		resultErr = gzipTextHardFailure(resultErr, hard)
		if resultErr != nil {
			if result != nil {
				resultErr = gzipTextHardFailure(resultErr, result.Close())
			}
			result, receipt = nil, gzipInputReceipt{}
		}
	}()
	checked := &gzipTextCheckedInput{conversionInputView: source}
	if err := preflightGzipInput(checked); err != nil {
		return nil, receipt, err
	}
	started := progressStarted(opts, "gzip_input_decompress", "decoding complete gzip capture", source.DisplayPath(), opts.OutputPath)
	defer func() {
		status := ProgressStatusComplete
		if resultErr != nil {
			status = ProgressStatusFailed
		}
		progressFinished(opts, "gzip_input_decompress", "gzip capture decode "+status, source.DisplayPath(), opts.OutputPath, started, status)
	}()
	writer, err := createExternalToolInputSnapshotFile(staging, gzipBinaryInputSnapshotLeaf)
	if err != nil {
		return nil, receipt, err
	}
	writerOwned := true
	defer func() {
		if writerOwned {
			resultErr = gzipTextHardFailure(resultErr, writer.Close())
		}
	}()
	receipt, err = inflateGzipToWriter(ctx, opts, checked, writer, "gzip_input_decompress", started)
	if err != nil {
		return nil, gzipInputReceipt{}, err
	}
	if err := writer.Sync(); err != nil {
		return nil, gzipInputReceipt{}, err
	}
	created, err := writer.Stat()
	if err != nil {
		return nil, gzipInputReceipt{}, err
	}
	createdGeneration, err := filegeneration.FromFile(writer)
	if err != nil {
		return nil, gzipInputReceipt{}, err
	}
	if !created.Mode().IsRegular() || created.Size() != receipt.DecodedBytes || !createdGeneration.Strong() || createdGeneration.Size() != receipt.DecodedBytes {
		return nil, gzipInputReceipt{}, errors.New("gzip decoded writer generation is unavailable or changed")
	}
	held, info, err := freezeExternalToolInputSnapshotFile(staging, gzipBinaryInputSnapshotLeaf, writer, created)
	writerOwned = false // freeze consumes the creator handle on every return.
	if err != nil {
		return nil, gzipInputReceipt{}, err
	}
	keepHeld := false
	defer func() {
		if !keepHeld {
			resultErr = gzipTextHardFailure(resultErr, held.Close())
		}
	}()
	generation, err := filegeneration.FromFile(held)
	if err != nil {
		return nil, gzipInputReceipt{}, err
	}
	if !generation.Strong() || !createdGeneration.SameVersion(generation) || info == nil || !info.Mode().IsRegular() || info.Size() != receipt.DecodedBytes {
		return nil, gzipInputReceipt{}, errors.New("gzip decoded generation changed while freezing")
	}
	// The write-point digest must describe the frozen bytes, not merely the
	// buffers sent to a writable creator handle. Bind that digest before a
	// semantic decoder can consume this private input.
	count, digest, measured, err := tracebundle.MeasureFile(ctx, held)
	if err != nil {
		return nil, gzipInputReceipt{}, err
	}
	if count != receipt.DecodedBytes || digest != receipt.DecodedSHA256 || !generation.SameVersion(measured) {
		return nil, gzipInputReceipt{}, errors.New("frozen gzip decoded bytes differ from the inflate receipt")
	}
	privatePath, err := staging.ChildPath(gzipBinaryInputSnapshotLeaf)
	if err != nil {
		return nil, gzipInputReceipt{}, err
	}
	receipt.DecodedGeneration = generation.CacheToken()
	result = &gzipBinaryInput{source: source, receipt: receipt, snapshot: &externalToolInputSnapshot{
		dir: staging, name: gzipBinaryInputSnapshotLeaf, path: privatePath, display: source.DisplayPath(),
		file: held, identity: generation, size: generation.Size(),
	}}
	if err := result.Validate(conversionInputStageRoute); err != nil {
		result = nil
		return nil, gzipInputReceipt{}, err
	}
	keepHeld = true
	return result, receipt, nil
}

func preflightGzipInput(input *gzipTextCheckedInput) error {
	if err := preflightHiperfGzipHeader(input); err != nil {
		if input.readErr != nil {
			return input.readErr
		}
		var rejected *HiperfGzipError
		if errors.As(err, &rejected) {
			return gzipTextFailure(rejected.Code, rejected.Cause)
		}
		return err
	}
	return nil
}

// inflateGzipToWriter is the shared byte-only inflater. It does not own or
// close destination, choose a decoded format, or publish any output. Header
// preflight is bounded; compressed hashing happens in the one inflate pass.
func inflateGzipToWriter(ctx context.Context, opts Options, input *gzipTextCheckedInput, destination io.Writer, progressStage string, started time.Time) (receipt gzipInputReceipt, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil || input.conversionInputView == nil || destination == nil {
		return receipt, errors.New("gzip stream contract is incomplete")
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageRoute, nil); err != nil {
		return receipt, err
	}
	defer func() {
		resultErr = gzipTextHardFailure(resultErr, errors.Join(ctx.Err(), input.Validate(conversionInputStageRoute)))
		if resultErr != nil {
			receipt = gzipInputReceipt{}
		}
	}()
	if err := preflightGzipInput(input); err != nil {
		return receipt, err
	}
	sourceHash := sha256.New()
	counted := &countingWriter{writer: sourceHash}
	buffered := bufio.NewReaderSize(io.TeeReader(io.NewSectionReader(input, 0, input.Size()), counted), 64<<10)
	reader, err := gzip.NewReader(buffered)
	if err != nil {
		if input.readErr != nil {
			return receipt, input.readErr
		}
		return receipt, gzipTextFailure(GzipTextCodeInvalidHeader, err)
	}
	reader.Multistream(false)
	defer func() { resultErr = gzipTextHardFailure(resultErr, reader.Close()) }()
	decodedHash := sha256.New()
	dest := io.MultiWriter(destination, decodedHash)
	limit := hiperfGzipMaxDecodedBytes
	if ratio := input.Size() * hiperfGzipMaxCompressionRatio; ratio < limit {
		limit = ratio
	}
	var decodedBytes int64
	lastProgress := started
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return receipt, err
		}
		n, readErr := reader.Read(buffer)
		if input.readErr != nil {
			return receipt, input.readErr
		}
		if n > 0 {
			if int64(n) > limit-decodedBytes {
				return receipt, gzipTextFailure(GzipTextCodeResourceLimit, fmt.Errorf("decoded capture exceeds %d-byte size/ratio budget", limit))
			}
			written, writeErr := dest.Write(buffer[:n])
			if writeErr != nil {
				return receipt, writeErr
			}
			if written != n {
				return receipt, io.ErrShortWrite
			}
			decodedBytes += int64(n)
			if now := time.Now(); now.Sub(lastProgress) >= progressHeartbeatInterval {
				lastProgress = now
				emitProgress(opts, ProgressEvent{Stage: progressStage, Status: ProgressStatusProgress, Message: "decoding complete gzip capture", Path: input.DisplayPath(), OutputPath: opts.OutputPath, BytesDone: decodedBytes})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return receipt, gzipTextFailure(GzipTextCodeIntegrity, readErr)
		}
		if n == 0 {
			return receipt, gzipTextFailure(GzipTextCodeIntegrity, io.ErrNoProgress)
		}
	}
	if _, err := buffered.Peek(1); err != io.EOF {
		if input.readErr != nil {
			return receipt, input.readErr
		}
		if err == nil {
			err = errors.New("concatenated gzip member or trailing bytes are unsupported")
		}
		return receipt, gzipTextFailure(GzipTextCodeTrailingData, err)
	}
	if counted.count != input.Size() {
		return receipt, errors.New("gzip compressed byte count differs from held source size")
	}
	return gzipInputReceipt{SourceBytes: input.Size(), SourceSHA256: hex.EncodeToString(sourceHash.Sum(nil)), DecodedBytes: decodedBytes, DecodedSHA256: hex.EncodeToString(decodedHash.Sum(nil))}, nil
}

// publishGzipInputText preserves the decoded input through route finalization.
// A bounded copy (not another inflate) gets its own sealed publication
// authority. The caller has already admitted the complete held text, owns the
// ledger and source, and fills SourceGeneration from its outer authority.
func publishGzipInputText(ctx context.Context, opts Options, input *gzipBinaryInput, receipt gzipInputReceipt, ledger *conversionFileLedger) (result GzipTextTransportResult, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil || ledger == nil || receipt != input.receipt || receipt.DecodedBytes <= 0 || strings.TrimSpace(opts.OutputPath) == "" {
		return result, errors.New("gzip text publication contract is incomplete")
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStagePreCommit, nil); err != nil {
		return result, err
	}
	defer func() {
		resultErr = gzipTextHardFailure(resultErr, errors.Join(ctx.Err(), input.Validate(conversionInputStagePreCommit)))
		if resultErr != nil {
			result = GzipTextTransportResult{}
		}
	}()
	target, err := prepareSealedConversionPublicationTargetWithLedger(opts.OutputPath, ".codrax-gzip-text-*", ledger)
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
	if err := validateGzipTextSealed(ctx, sealed, sealed.identity, opts.OutputPath, receipt.DecodedBytes, receipt.DecodedSHA256); err != nil {
		return result, err
	}
	result = GzipTextTransportResult{
		Profile: GzipTextTransportProfile, SourcePath: input.DisplayPath(), SourceBytes: receipt.SourceBytes, SourceSHA256: receipt.SourceSHA256,
		DecodedPath: target.finalBindingPath, DecodedBytes: receipt.DecodedBytes, DecodedSHA256: receipt.DecodedSHA256,
	}
	if err := publishSealedConversionFileNoReplaceWithValidation(ctx, target, sealed, ledger, func(published *retainedTraceDBPublication) error {
		count, digest, generation, err := tracebundle.MeasureFile(ctx, published.file)
		if err != nil {
			return err
		}
		if count != receipt.DecodedBytes || digest != receipt.DecodedSHA256 || !published.identity.SameVersion(generation) {
			return errors.New("published gzip text differs from its held decoded input")
		}
		result.DecodedGeneration = generation.CacheToken()
		return nil
	}); err != nil {
		return GzipTextTransportResult{}, err
	}
	return result, nil
}

var _ conversionInputView = (*gzipBinaryInput)(nil)
var _ externalToolWholeFileSource = (*gzipBinaryInput)(nil)
