package hitraceconv

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const (
	hiperfGzipDecodedSnapshotLeaf   = "hiperf_decoded.perf.data"
	hiperfGzipMaxCompressedBytes    = int64(64 << 30)
	hiperfGzipMaxDecodedBytes       = int64(64 << 30)
	hiperfGzipMaxCompressionRatio   = int64(1000)
	hiperfGzipMaxOptionalFieldBytes = int64(64 << 10)
	hiperfGzipMaxHeaderBytes        = int64(3*(64<<10) + 32)
	perfInputTransformGzipV1        = "gzip_perf_data_v1"
)

const (
	hiperfGzipCodeInvalidHeader = "gzip_invalid_header"
	hiperfGzipCodeIntegrity     = "gzip_integrity_failed"
	hiperfGzipCodeResourceLimit = "gzip_resource_limit"
	hiperfGzipCodeTrailingData  = "gzip_trailing_data"
	hiperfGzipCodeDecodedFormat = "gzip_decoded_format_invalid"
)

// HiperfGzipError is a data-local verdict. It never hides context, input
// generation, private-directory, I/O, sync, or cleanup failures, which remain
// hard conversion errors.
type HiperfGzipError struct {
	Code  string
	Cause error
}

func (err *HiperfGzipError) Error() string {
	if err == nil {
		return "HIPERF gzip input rejected"
	}
	message := "HIPERF gzip input rejected: code=" + firstNonEmpty(err.Code, hiperfGzipCodeIntegrity)
	if err.Cause != nil {
		message += ": " + err.Cause.Error()
	}
	return message
}

func (err *HiperfGzipError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func hiperfGzipFailure(code string, cause error) error {
	return &HiperfGzipError{Code: code, Cause: cause}
}

// snapshotInputView borrows a held snapshot without acquiring close or path
// authority. upstream is revalidated on both sides of every stage so the
// derived generation cannot outlive or relabel its exact source generation.
type snapshotInputView struct {
	snapshot *externalToolInputSnapshot
	upstream conversionInputView
	display  string
}

func snapshotInputViewFromLease(lease *externalToolInputLease, display string) (*snapshotInputView, error) {
	if lease == nil {
		return nil, conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageExternalTool, display, errors.New("HIPERF gzip source lease is nil"))
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.closed || lease.transport != externalToolInputTransportSnapshot || lease.snapshot == nil || lease.source == nil {
		return nil, conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageExternalTool, display, errors.New("HIPERF gzip source lease is not snapshot-backed"))
	}
	view := &snapshotInputView{snapshot: lease.snapshot, upstream: lease.source, display: display}
	if err := view.Validate(conversionInputStageExternalTool); err != nil {
		return nil, err
	}
	return view, nil
}

func (view *snapshotInputView) Size() int64 {
	if view == nil || view.snapshot == nil {
		return 0
	}
	return view.snapshot.size
}

func (view *snapshotInputView) DisplayPath() string {
	if view == nil {
		return ""
	}
	return view.display
}

func (view *snapshotInputView) ReadAt(buffer []byte, offset int64) (int, error) {
	if view == nil || view.snapshot == nil {
		return 0, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageExternalTool, view.DisplayPath(), errors.New("HIPERF snapshot view is closed"))
	}
	view.snapshot.mu.RLock()
	defer view.snapshot.mu.RUnlock()
	if view.snapshot.closed || view.snapshot.file == nil {
		return 0, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageExternalTool, view.display, errors.New("HIPERF snapshot view is closed"))
	}
	if offset < 0 {
		return 0, conversionInputFailure(ConversionInputCodeInvalidRange, conversionInputStageExternalTool, view.display, nil)
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	if offset >= view.snapshot.size {
		return 0, io.EOF
	}
	remaining := view.snapshot.size - offset
	limited := buffer
	truncated := int64(len(buffer)) > remaining
	if truncated {
		limited = buffer[:int(remaining)]
	}
	n, err := view.snapshot.file.ReadAt(limited, offset)
	if err == nil && truncated {
		err = io.EOF
	}
	return n, err
}

func (view *snapshotInputView) Validate(stage conversionInputStage) error {
	if view == nil || view.snapshot == nil || view.upstream == nil || !stage.valid() || strings.TrimSpace(view.display) == "" {
		return conversionInputFailure(ConversionInputCodeInternalContract, stage, view.DisplayPath(), errors.New("HIPERF snapshot view contract is incomplete"))
	}
	if err := view.upstream.Validate(stage); err != nil {
		return err
	}
	if err := view.snapshot.Validate(); err != nil {
		return err
	}
	return view.upstream.Validate(stage)
}

func (view *snapshotInputView) withOpenFile(callback func(*os.File) error) error {
	if view == nil || view.snapshot == nil || callback == nil {
		return conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageExternalTool, view.DisplayPath(), errors.New("HIPERF snapshot callback is incomplete"))
	}
	view.snapshot.mu.RLock()
	defer view.snapshot.mu.RUnlock()
	if view.snapshot.closed || view.snapshot.file == nil {
		return conversionInputFailure(ConversionInputCodeClosed, conversionInputStageExternalTool, view.display, errors.New("HIPERF snapshot view is closed"))
	}
	return callback(view.snapshot.file)
}

func newExternalToolInputLeaseFromSnapshot(ctx context.Context, source conversionInputView, snapshot *externalToolInputSnapshot) (*externalToolInputLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || snapshot == nil {
		return nil, conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageExternalTool, "", errors.New("decoded HIPERF lease contract is incomplete"))
	}
	if err := completeConversionInputStage(ctx, source, conversionInputStageExternalTool, nil); err != nil {
		return nil, err
	}
	lease := &externalToolInputLease{source: source, transport: externalToolInputTransportSnapshot, snapshot: snapshot}
	if err := lease.Validate(); err != nil {
		return nil, traceDBJoinPreservingSingle(err, lease.Close())
	}
	if err := completeConversionInputStage(ctx, source, conversionInputStageExternalTool, nil); err != nil {
		return nil, traceDBJoinPreservingSingle(err, lease.Close())
	}
	return lease, nil
}

func decompressStandaloneHiperfGzip(
	ctx context.Context,
	opts Options,
	compressed *snapshotInputView,
	staging *privateConversionDir,
	sourceArtifactPath string,
) (_ *snapshotInputView, _ *externalToolInputSnapshot, _ *PerfInputTransform, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if compressed == nil || staging == nil || strings.TrimSpace(sourceArtifactPath) == "" {
		return nil, nil, nil, conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageExternalTool, sourceArtifactPath, errors.New("HIPERF gzip decoder contract is incomplete"))
	}
	if err := completeConversionInputStage(ctx, compressed, conversionInputStageExternalTool, nil); err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		resultErr = completeConversionInputStage(ctx, compressed, conversionInputStageExternalTool, resultErr)
	}()
	if err := preflightHiperfGzipHeader(compressed); err != nil {
		return nil, nil, nil, err
	}

	started := progressStarted(opts, "hiperf_gzip_decompress", "decoding receipt-bound HIPERF gzip input", sourceArtifactPath, sourceArtifactPath)
	status := ProgressStatusFailed
	defer func() {
		progressFinished(opts, "hiperf_gzip_decompress", "HIPERF gzip decode "+string(status), sourceArtifactPath, sourceArtifactPath, started, status)
	}()

	compressedHash := sha256.New()
	counted := &countingWriter{writer: compressedHash}
	section := io.NewSectionReader(compressed, 0, compressed.Size())
	buffered := bufio.NewReaderSize(io.TeeReader(section, counted), 64<<10)
	reader, err := gzip.NewReader(buffered)
	if err != nil {
		return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeInvalidHeader, err)
	}
	reader.Multistream(false)
	readerClosed := false
	defer func() {
		if !readerClosed {
			resultErr = traceDBJoinPreservingSingle(resultErr, reader.Close())
		}
	}()

	prefix := make([]byte, len(perfMagic2))
	if _, err := io.ReadFull(reader, prefix); err != nil {
		return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeIntegrity, err)
	}
	if string(prefix) != perfMagic2 {
		return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeDecodedFormat, fmt.Errorf("decoded payload does not begin with %s", perfMagic2))
	}
	decodedLimit := hiperfGzipMaxDecodedBytes
	if ratioLimit := compressed.Size() * hiperfGzipMaxCompressionRatio; ratioLimit < decodedLimit {
		decodedLimit = ratioLimit
	}
	if int64(len(prefix)) > decodedLimit {
		return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeResourceLimit, errors.New("decoded HIPERF payload exceeds its size/ratio budget"))
	}

	writer, err := createExternalToolInputSnapshotFile(staging, hiperfGzipDecodedSnapshotLeaf)
	if err != nil {
		return nil, nil, nil, err
	}
	writerOwned := true
	defer func() {
		if writerOwned && writer != nil {
			resultErr = traceDBJoinPreservingSingle(resultErr, writer.Close())
		}
	}()
	decodedHash := sha256.New()
	destination := io.MultiWriter(writer, decodedHash)
	if n, err := destination.Write(prefix); err != nil || n != len(prefix) {
		if err == nil {
			err = io.ErrShortWrite
		}
		return nil, nil, nil, err
	}
	decodedBytes := int64(len(prefix))
	lastProgress := started
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		n, readErr := reader.Read(buffer)
		if n > 0 {
			if decodedBytes > decodedLimit-int64(n) {
				return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeResourceLimit, fmt.Errorf("decoded HIPERF payload exceeds %d-byte size/ratio budget", decodedLimit))
			}
			written, writeErr := destination.Write(buffer[:n])
			decodedBytes += int64(written)
			if writeErr != nil {
				return nil, nil, nil, writeErr
			}
			if written != n {
				return nil, nil, nil, io.ErrShortWrite
			}
			now := time.Now()
			if now.Sub(lastProgress) >= progressHeartbeatInterval {
				lastProgress = now
				emitProgress(opts, ProgressEvent{Stage: "hiperf_gzip_decompress", Status: ProgressStatusProgress, Message: "decoding HIPERF gzip input", Path: sourceArtifactPath, OutputPath: sourceArtifactPath, BytesDone: decodedBytes})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeIntegrity, readErr)
		}
		if n == 0 {
			return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeIntegrity, io.ErrNoProgress)
		}
	}
	if err := reader.Close(); err != nil {
		readerClosed = true
		return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeIntegrity, err)
	}
	readerClosed = true
	if _, err := buffered.Peek(1); err != io.EOF {
		if err == nil {
			err = errors.New("concatenated gzip member or trailing bytes")
		}
		return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeTrailingData, err)
	}
	if counted.count != compressed.Size() {
		return nil, nil, nil, hiperfGzipFailure(hiperfGzipCodeIntegrity, fmt.Errorf("compressed read size mismatch: got=%d want=%d", counted.count, compressed.Size()))
	}
	if err := writer.Sync(); err != nil {
		return nil, nil, nil, fmt.Errorf("sync decoded HIPERF snapshot: %w", err)
	}
	created, err := writer.Stat()
	if err != nil || !created.Mode().IsRegular() || created.Size() != decodedBytes {
		if err == nil {
			err = errors.New("decoded HIPERF snapshot mode/size mismatch")
		}
		return nil, nil, nil, err
	}
	createdIdentity, err := filegeneration.FromFile(writer)
	if err != nil || !createdIdentity.Strong() {
		return nil, nil, nil, firstNonNilError(err, errors.New("decoded HIPERF snapshot has no strong identity"))
	}
	held, heldInfo, err := freezeExternalToolInputSnapshotFile(staging, hiperfGzipDecodedSnapshotLeaf, writer, created)
	writerOwned = false
	writer = nil
	if err != nil {
		return nil, nil, nil, err
	}
	closeHeld := true
	defer func() {
		if closeHeld {
			resultErr = traceDBJoinPreservingSingle(resultErr, held.Close())
		}
	}()
	heldIdentity, err := filegeneration.FromFile(held)
	if err != nil || !heldIdentity.Strong() || !createdIdentity.SameVersion(heldIdentity) || heldInfo == nil || heldInfo.Size() != decodedBytes {
		return nil, nil, nil, firstNonNilError(err, errors.New("decoded HIPERF snapshot generation changed while freezing"))
	}
	privatePath, err := staging.ChildPath(hiperfGzipDecodedSnapshotLeaf)
	if err != nil {
		return nil, nil, nil, err
	}
	snapshot := &externalToolInputSnapshot{dir: staging, name: hiperfGzipDecodedSnapshotLeaf, path: privatePath, display: sourceArtifactPath, file: held, identity: heldIdentity, size: heldIdentity.Size()}
	decoded := &snapshotInputView{snapshot: snapshot, upstream: compressed, display: sourceArtifactPath}
	if err := decoded.Validate(conversionInputStageExternalTool); err != nil {
		return nil, nil, nil, err
	}
	transform := &PerfInputTransform{
		Profile: perfInputTransformGzipV1, SourceArtifactPath: sourceArtifactPath,
		SourceFormat: string(perfInputGzipPerfData), SourceBytes: compressed.Size(), SourceSHA256: hex.EncodeToString(compressedHash.Sum(nil)),
		DecodedFormat: string(perfInputLinuxPerfData), DecodedBytes: decodedBytes, DecodedSHA256: hex.EncodeToString(decodedHash.Sum(nil)),
	}
	status = ProgressStatusComplete
	closeHeld = false
	return decoded, snapshot, transform, nil
}

func preflightHiperfGzipHeader(input conversionInputView) error {
	if input == nil || input.Size() < 18 || input.Size() > hiperfGzipMaxCompressedBytes {
		return hiperfGzipFailure(hiperfGzipCodeResourceLimit, fmt.Errorf("compressed HIPERF size=%d outside [18,%d]", input.Size(), hiperfGzipMaxCompressedBytes))
	}
	var fixed [10]byte
	if err := readHiperfGzipAt(input, fixed[:], 0); err != nil {
		return err
	}
	if fixed[0] != 0x1f || fixed[1] != 0x8b || fixed[2] != 8 || fixed[3]&0xe0 != 0 {
		return hiperfGzipFailure(hiperfGzipCodeInvalidHeader, errors.New("gzip magic/method/reserved flags are invalid"))
	}
	flags := fixed[3]
	position := int64(len(fixed))
	if flags&0x04 != 0 {
		var size [2]byte
		if err := readHiperfGzipAt(input, size[:], position); err != nil {
			return err
		}
		position += 2 + int64(binary.LittleEndian.Uint16(size[:]))
		if position > input.Size()-8 || position > hiperfGzipMaxHeaderBytes {
			return hiperfGzipFailure(hiperfGzipCodeResourceLimit, errors.New("gzip extra/header exceeds budget"))
		}
	}
	for _, flag := range []byte{0x08, 0x10} {
		if flags&flag == 0 {
			continue
		}
		var err error
		position, err = scanHiperfGzipCString(input, position)
		if err != nil {
			return err
		}
	}
	if flags&0x02 != 0 {
		position += 2
	}
	if position > input.Size()-8 || position > hiperfGzipMaxHeaderBytes {
		return hiperfGzipFailure(hiperfGzipCodeInvalidHeader, errors.New("gzip header overlaps its trailer"))
	}
	return nil
}

func scanHiperfGzipCString(input conversionInputView, position int64) (int64, error) {
	start := position
	buffer := make([]byte, 4096)
	for position < input.Size()-8 && position-start <= hiperfGzipMaxOptionalFieldBytes {
		want := int64(len(buffer))
		if remaining := input.Size() - 8 - position; remaining < want {
			want = remaining
		}
		if remaining := hiperfGzipMaxOptionalFieldBytes + 1 - (position - start); remaining < want {
			want = remaining
		}
		if want <= 0 {
			break
		}
		if err := readHiperfGzipAt(input, buffer[:int(want)], position); err != nil {
			return 0, err
		}
		for index, value := range buffer[:int(want)] {
			if value == 0 {
				next := position + int64(index) + 1
				if next > hiperfGzipMaxHeaderBytes {
					return 0, hiperfGzipFailure(hiperfGzipCodeResourceLimit, errors.New("gzip optional header exceeds total budget"))
				}
				return next, nil
			}
		}
		position += want
	}
	return 0, hiperfGzipFailure(hiperfGzipCodeResourceLimit, errors.New("gzip filename/comment is unterminated or exceeds budget"))
}

func readHiperfGzipAt(input conversionInputView, buffer []byte, offset int64) error {
	n, err := input.ReadAt(buffer, offset)
	if n == len(buffer) && (err == nil || err == io.EOF) {
		return nil
	}
	if err == nil {
		err = io.ErrUnexpectedEOF
	}
	var inputErr *ConversionInputError
	if errors.As(err, &inputErr) {
		return err
	}
	return hiperfGzipFailure(hiperfGzipCodeInvalidHeader, err)
}

type countingWriter struct {
	writer io.Writer
	count  int64
}

func (writer *countingWriter) Write(buffer []byte) (int, error) {
	n, err := writer.writer.Write(buffer)
	writer.count += int64(n)
	return n, err
}

var _ conversionInputView = (*snapshotInputView)(nil)
var _ externalToolInputFileSource = (*snapshotInputView)(nil)
