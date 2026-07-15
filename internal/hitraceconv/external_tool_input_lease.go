package hitraceconv

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// externalToolInputProfile is a closed transport authority. A caller may ask
// for the Linux inherited-FD optimization only when a typed resolver has
// proved the exact tool artifact compatible; display strings and filenames
// must never choose this hard gate.
type externalToolInputProfile uint8

const (
	externalToolInputSnapshotOnly externalToolInputProfile = iota + 1
	externalToolInputVerifiedLinuxFD
)

func (profile externalToolInputProfile) valid() bool {
	return profile == externalToolInputSnapshotOnly || profile == externalToolInputVerifiedLinuxFD
}

type externalToolInputTransport uint8

const (
	externalToolInputTransportSnapshot externalToolInputTransport = iota + 1
	externalToolInputTransportLinuxFD
)

func (transport externalToolInputTransport) valid() bool {
	return transport == externalToolInputTransportSnapshot || transport == externalToolInputTransportLinuxFD
}

// externalToolInputFileSource is deliberately narrower than exposing an FD.
// Platform code can borrow the already-held generation only inside a callback;
// ownership and close authority stay with the source.
type externalToolInputFileSource interface {
	withOpenFile(func(*os.File) error) error
}

// externalToolWholeFileSource is a separate hard capability. Only a source
// whose held file is byte-for-byte the complete logical input may enter the
// Linux inherited-FD transport; bounded payload views deliberately lack it.
type externalToolWholeFileSource interface {
	externalToolInputFileSource
	externalToolWholeFileSource()
}

// externalToolInputSnapshot holds the exact private snapshot generation from
// creation through child completion. path is the private argv binding, while
// display is the public diagnostic identity; validation always starts from
// this handle plus the held parent authority.
type externalToolInputSnapshot struct {
	mu       sync.RWMutex
	dir      *privateConversionDir
	name     string
	path     string
	display  string
	file     *os.File
	identity filegeneration.Identity
	size     int64
	closed   bool
}

// externalToolInputLease is the sole producer of an external command's main
// input argument. It owns either an exact private snapshot or a duplicated
// Linux input FD and keeps that authority until the command boundary is fully
// validated.
type externalToolInputLease struct {
	mu                sync.Mutex
	source            conversionInputView
	transport         externalToolInputTransport
	snapshot          *externalToolInputSnapshot
	inheritedFile     *os.File
	inheritedIdentity filegeneration.Identity
	commandBuilt      bool
	closed            bool
}

type externalToolInputProgress func(done, total int64)

// newExternalToolInputLeaseWithPublicProgress centralizes snapshot-copy
// progress for providers. Only public diagnostic identities are emitted; the
// private snapshot path never escapes through progress events.
func newExternalToolInputLeaseWithPublicProgress(
	ctx context.Context,
	opts Options,
	source conversionInputView,
	staging *privateConversionDir,
	snapshotLeaf string,
	profile externalToolInputProfile,
	stage, provider, publicInput, publicOutput string,
) (*externalToolInputLease, error) {
	if profile != externalToolInputSnapshotOnly {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			publicInput,
			fmt.Errorf("progress-reporting perf input lease requires the snapshot-only profile"),
		)
	}
	start := progressStarted(
		opts,
		stage,
		"preparing immutable "+provider+" input",
		publicInput,
		publicOutput,
	)
	lastProgress := start
	lease, err := newExternalToolInputLeaseWithProgress(
		ctx,
		source,
		staging,
		snapshotLeaf,
		profile,
		func(done, total int64) {
			now := time.Now()
			if done != total && now.Sub(lastProgress) < progressHeartbeatInterval {
				return
			}
			lastProgress = now
			emitProgress(opts, ProgressEvent{
				Stage:      stage,
				Status:     ProgressStatusProgress,
				Message:    "copying immutable " + provider + " input",
				Path:       publicInput,
				OutputPath: publicOutput,
				BytesDone:  done,
				BytesTotal: total,
				Elapsed:    now.Sub(start),
			})
		},
	)
	if err != nil {
		progressFinished(opts, stage, provider+" input snapshot failed", publicInput, publicOutput, start, ProgressStatusFailed)
		return nil, err
	}
	progressFinished(opts, stage, "prepared immutable "+provider+" input", publicInput, publicOutput, start, ProgressStatusComplete)
	return lease, nil
}

func newExternalToolInputLease(
	ctx context.Context,
	source conversionInputView,
	staging *privateConversionDir,
	snapshotLeaf string,
	profile externalToolInputProfile,
) (*externalToolInputLease, error) {
	return newExternalToolInputLeaseWithProgress(ctx, source, staging, snapshotLeaf, profile, nil)
}

func newExternalToolInputLeaseWithProgress(
	ctx context.Context,
	source conversionInputView,
	staging *privateConversionDir,
	snapshotLeaf string,
	profile externalToolInputProfile,
	progress externalToolInputProgress,
) (*externalToolInputLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || !profile.valid() {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			"",
			fmt.Errorf("external tool input lease contract is invalid"),
		)
	}
	if err := completeConversionInputStage(ctx, source, conversionInputStageExternalTool, nil); err != nil {
		return nil, err
	}

	inherited, inheritedIdentity, used, err := tryExternalToolInheritedInputPlatform(source, profile)
	if err != nil {
		return nil, err
	}
	if used {
		lease := &externalToolInputLease{
			source: source, transport: externalToolInputTransportLinuxFD,
			inheritedFile: inherited, inheritedIdentity: inheritedIdentity,
		}
		if err := completeConversionInputStage(ctx, source, conversionInputStageExternalTool, nil); err != nil {
			return nil, traceDBJoinPreservingSingle(err, lease.Close())
		}
		if err := lease.Validate(); err != nil {
			return nil, traceDBJoinPreservingSingle(err, lease.Close())
		}
		return lease, nil
	}

	if staging == nil {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			source.DisplayPath(),
			fmt.Errorf("private snapshot directory authority is missing"),
		)
	}
	snapshot, err := createExternalToolInputSnapshot(ctx, source, staging, snapshotLeaf, progress)
	if err != nil {
		return nil, err
	}
	lease := &externalToolInputLease{
		source: source, transport: externalToolInputTransportSnapshot, snapshot: snapshot,
	}
	if err := lease.Validate(); err != nil {
		return nil, traceDBJoinPreservingSingle(err, lease.Close())
	}
	return lease, nil
}

func createExternalToolInputSnapshot(
	ctx context.Context,
	source conversionInputView,
	dir *privateConversionDir,
	name string,
	progress externalToolInputProgress,
) (result *externalToolInputSnapshot, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || dir == nil {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			"",
			fmt.Errorf("external tool snapshot source is incomplete"),
		)
	}
	expectedSize := source.Size()
	if expectedSize < 0 {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			source.DisplayPath(),
			fmt.Errorf("external tool snapshot source size is invalid: %d", expectedSize),
		)
	}
	if err := completeConversionInputStage(ctx, source, conversionInputStageExternalTool, nil); err != nil {
		return nil, err
	}
	defer func() {
		validated := completeConversionInputStage(ctx, source, conversionInputStageExternalTool, resultErr)
		if validated != nil && result != nil {
			validated = traceDBJoinPreservingSingle(validated, result.Close())
			result = nil
		}
		resultErr = validated
	}()
	path, err := dir.ChildPath(name)
	if err != nil {
		return nil, err
	}
	if err := validateExternalToolInputSourcePlatform(source); err != nil {
		return nil, err
	}
	writer, err := createExternalToolInputSnapshotFile(dir, name)
	if err != nil {
		return nil, err
	}
	writerOwned := true
	defer func() {
		if writerOwned && writer != nil {
			resultErr = traceDBJoinPreservingSingle(resultErr, writer.Close())
		}
	}()

	written, copyErr := copyExternalToolInputSnapshot(
		ctx,
		writer,
		io.NewSectionReader(source, 0, expectedSize),
		expectedSize,
		progress,
	)
	if copyErr != nil {
		return nil, copyErr
	}
	if written != expectedSize {
		return nil, fmt.Errorf("external tool input snapshot size mismatch: wrote=%d want=%d", written, expectedSize)
	}
	if err := writer.Sync(); err != nil {
		return nil, fmt.Errorf("sync external tool input snapshot: %w", err)
	}
	createdInfo, err := writer.Stat()
	if err != nil || !createdInfo.Mode().IsRegular() || createdInfo.Size() != expectedSize {
		if err == nil {
			err = fmt.Errorf("mode=%s size=%d want=%d", createdInfo.Mode(), createdInfo.Size(), expectedSize)
		}
		return nil, fmt.Errorf("validate created external tool input snapshot: %w", err)
	}
	createdIdentity, err := filegeneration.FromFile(writer)
	if err != nil {
		return nil, fmt.Errorf("capture external tool input snapshot generation: %w", err)
	}
	if !createdIdentity.Strong() {
		return nil, conversionInputFailure(
			ConversionInputCodeStrongIdentityUnavailable,
			conversionInputStageExternalTool,
			source.DisplayPath(),
			fmt.Errorf("external tool snapshot has no strong identity"),
		)
	}
	held, heldInfo, err := freezeExternalToolInputSnapshotFile(dir, name, writer, createdInfo)
	writerOwned = false // the platform freeze consumes writer on every return
	writer = nil
	if err != nil {
		return nil, err
	}
	closeHeld := true
	defer func() {
		if closeHeld {
			resultErr = traceDBJoinPreservingSingle(resultErr, held.Close())
		}
	}()
	heldIdentity, err := filegeneration.FromFile(held)
	if err != nil || !heldIdentity.Strong() || !createdIdentity.SameVersion(heldIdentity) ||
		heldInfo == nil || !heldInfo.Mode().IsRegular() || heldInfo.Size() != expectedSize {
		if err == nil {
			err = fmt.Errorf("external tool snapshot generation changed while freezing")
		}
		return nil, conversionInputFailure(
			ConversionInputCodeGenerationChanged,
			conversionInputStageExternalTool,
			source.DisplayPath(),
			err,
		)
	}
	snapshot := &externalToolInputSnapshot{
		dir: dir, name: name, path: path, display: source.DisplayPath(), file: held,
		identity: heldIdentity, size: heldIdentity.Size(),
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	closeHeld = false
	return snapshot, nil
}

func copyExternalToolInputSnapshot(
	ctx context.Context,
	dst io.Writer,
	src io.Reader,
	total int64,
	progress externalToolInputProgress,
) (int64, error) {
	if progress != nil {
		progress(0, total)
	}
	return copyCancellableRange(ctx, dst, src, func(written int64) {
		if progress != nil {
			progress(written, total)
		}
	})
}

func createExternalToolInputSnapshotFile(dir *privateConversionDir, name string) (*os.File, error) {
	if dir == nil {
		return nil, fmt.Errorf("private snapshot directory authority is missing")
	}
	if _, err := dir.ChildPath(name); err != nil {
		return nil, err
	}
	dir.mu.Lock()
	defer dir.mu.Unlock()
	if dir.terminal {
		return nil, traceDBJoinPreservingSingle(
			fmt.Errorf("private conversion directory authority is terminal: %s", dir.path), dir.terminalErr,
		)
	}
	if err := dir.validateIdentityLocked(true); err != nil {
		return nil, err
	}
	if err := validatePrivateConversionDirSecurityPlatform(dir.path, dir.identity, &dir.platform); err != nil {
		return nil, fmt.Errorf("%w: path=%s: %v", errPrivateConversionDirSecurityInvalid, dir.path, err)
	}
	if err := validateExternalToolInputSnapshotDirPlatform(&dir.platform); err != nil {
		return nil, err
	}
	file, err := createExternalToolInputSnapshotFilePlatform(&dir.platform, name)
	if err != nil {
		return nil, fmt.Errorf("create private external tool input snapshot %q: %w", name, err)
	}
	return file, nil
}

// freezeExternalToolInputSnapshotFile consumes writer on every return. Windows
// uses a handle-preserving access downgrade; Unix keeps the creator handle.
func freezeExternalToolInputSnapshotFile(
	dir *privateConversionDir,
	name string,
	writer *os.File,
	created os.FileInfo,
) (*os.File, os.FileInfo, error) {
	if dir == nil || writer == nil || created == nil {
		if writer != nil {
			_ = writer.Close()
		}
		return nil, nil, fmt.Errorf("external tool snapshot freeze authority is incomplete")
	}
	dir.mu.Lock()
	defer dir.mu.Unlock()
	if dir.terminal {
		return nil, nil, traceDBJoinPreservingSingle(
			fmt.Errorf("private conversion directory authority is terminal: %s", dir.path), writer.Close(), dir.terminalErr,
		)
	}
	if err := dir.validateIdentityLocked(true); err != nil {
		return nil, nil, traceDBJoinPreservingSingle(err, writer.Close())
	}
	file, info, err := freezeExternalToolInputSnapshotFilePlatform(&dir.platform, name, writer, created)
	if err != nil {
		return nil, nil, err
	}
	if err := validatePrivateConversionRegularChildPlatform(&dir.platform, name, file, info); err != nil {
		return nil, nil, traceDBJoinPreservingSingle(
			fmt.Errorf("bind frozen external tool input snapshot %q: %w", name, err), file.Close(),
		)
	}
	return file, info, nil
}

func (snapshot *externalToolInputSnapshot) Validate() error {
	if snapshot == nil {
		return fmt.Errorf("external tool input snapshot is nil")
	}
	snapshot.mu.RLock()
	defer snapshot.mu.RUnlock()
	if snapshot.closed || snapshot.file == nil || snapshot.dir == nil || !snapshot.identity.Initialized() {
		return conversionInputFailure(
			ConversionInputCodeClosed,
			conversionInputStageExternalTool,
			snapshot.display,
			fmt.Errorf("external tool input snapshot is closed or incomplete: %s", snapshot.name),
		)
	}
	snapshot.dir.mu.Lock()
	defer snapshot.dir.mu.Unlock()
	if snapshot.dir.terminal {
		return traceDBJoinPreservingSingle(
			fmt.Errorf("private conversion directory authority is terminal: %s", snapshot.dir.path), snapshot.dir.terminalErr,
		)
	}
	if err := snapshot.dir.validateIdentityLocked(true); err != nil {
		return err
	}
	if err := validatePrivateConversionDirSecurityPlatform(snapshot.dir.path, snapshot.dir.identity, &snapshot.dir.platform); err != nil {
		return fmt.Errorf("%w: path=%s: %v", errPrivateConversionDirSecurityInvalid, snapshot.dir.path, err)
	}
	current, err := filegeneration.FromFile(snapshot.file)
	if err != nil || !current.Strong() || !snapshot.identity.SameVersion(current) || current.Size() != snapshot.size {
		return conversionInputFailure(
			ConversionInputCodeGenerationChanged,
			conversionInputStageExternalTool,
			snapshot.display,
			traceDBJoinPreservingSingle(fmt.Errorf("external tool input snapshot generation changed: %s", snapshot.name), err),
		)
	}
	info, err := snapshot.file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != snapshot.size {
		return conversionInputFailure(
			ConversionInputCodeGenerationChanged,
			conversionInputStageExternalTool,
			snapshot.display,
			traceDBJoinPreservingSingle(fmt.Errorf("external tool input snapshot identity changed: %s", snapshot.name), err),
		)
	}
	if err := validatePrivateConversionRegularChildPlatform(&snapshot.dir.platform, snapshot.name, snapshot.file, info); err != nil {
		return conversionInputFailure(
			ConversionInputCodeGenerationChanged,
			conversionInputStageExternalTool,
			snapshot.display,
			fmt.Errorf("external tool input snapshot binding changed: %s: %w", snapshot.name, err),
		)
	}
	return nil
}

func (snapshot *externalToolInputSnapshot) Close() error {
	if snapshot == nil {
		return nil
	}
	snapshot.mu.Lock()
	defer snapshot.mu.Unlock()
	if snapshot.closed {
		return nil
	}
	snapshot.closed = true
	if snapshot.file == nil {
		return nil
	}
	err := snapshot.file.Close()
	snapshot.file = nil
	return err
}

// detachSealed transfers the exact held snapshot generation without closing
// and reopening its pathname. The returned authority is suitable for the
// sealed no-replace publisher; ownership leaves snapshot on success.
func (snapshot *externalToolInputSnapshot) detachSealed() (*sealedConversionFile, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("external tool input snapshot is nil")
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	snapshot.mu.Lock()
	defer snapshot.mu.Unlock()
	if snapshot.closed || snapshot.file == nil || snapshot.dir == nil || !snapshot.identity.Initialized() {
		return nil, conversionInputFailure(
			ConversionInputCodeClosed,
			conversionInputStageExternalTool,
			snapshot.display,
			fmt.Errorf("external tool input snapshot is closed or incomplete: %s", snapshot.name),
		)
	}
	prepared, err := prepareExternalToolInputSnapshotForSealedTransfer(snapshot.file, snapshot.name)
	if err != nil {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			snapshot.display,
			fmt.Errorf("prepare external tool input snapshot for sealed transfer: %w", err),
		)
	}
	snapshot.file = prepared
	current, err := filegeneration.FromFile(snapshot.file)
	if err != nil || !current.Strong() || !snapshot.identity.SameVersion(current) || current.Size() != snapshot.size {
		return nil, conversionInputFailure(
			ConversionInputCodeGenerationChanged,
			conversionInputStageExternalTool,
			snapshot.display,
			traceDBJoinPreservingSingle(fmt.Errorf("external tool input snapshot changed before sealed transfer: %s", snapshot.name), err),
		)
	}
	sealed := &sealedConversionFile{
		dir: snapshot.dir, name: snapshot.name, file: snapshot.file,
		identity: current, size: snapshot.size,
	}
	snapshot.file = nil
	snapshot.closed = true
	return sealed, nil
}

func (lease *externalToolInputLease) Validate() error {
	if lease == nil {
		return fmt.Errorf("external tool input lease is nil")
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	return lease.validateLocked()
}

func (lease *externalToolInputLease) validateLocked() error {
	if lease.closed || lease.source == nil || !lease.transport.valid() {
		return fmt.Errorf("external tool input lease is closed or incomplete")
	}
	if err := lease.source.Validate(conversionInputStageExternalTool); err != nil {
		return err
	}
	switch lease.transport {
	case externalToolInputTransportSnapshot:
		if lease.snapshot == nil {
			return fmt.Errorf("external tool snapshot lease has no snapshot authority")
		}
		return lease.snapshot.Validate()
	case externalToolInputTransportLinuxFD:
		if lease.inheritedFile == nil || !lease.inheritedIdentity.Initialized() {
			return fmt.Errorf("external tool inherited-FD lease is incomplete")
		}
		current, err := filegeneration.FromFile(lease.inheritedFile)
		if err != nil || !current.Strong() || !lease.inheritedIdentity.SameVersion(current) {
			return conversionInputFailure(
				ConversionInputCodeGenerationChanged,
				conversionInputStageExternalTool,
				lease.source.DisplayPath(),
				err,
			)
		}
		return nil
	default:
		return fmt.Errorf("external tool input lease transport is invalid")
	}
}

// Command is the only place that materializes the child's main input argv.
// beforeInput and afterInput describe the tool grammar around that one value;
// neither may contain the original display path. Linux ExtraFiles injection is
// intentionally centralized here.
func (lease *externalToolInputLease) Command(
	ctx context.Context,
	executable string,
	beforeInput []string,
	afterInput []string,
) (*exec.Cmd, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if lease == nil || strings.TrimSpace(executable) == "" {
		return nil, fmt.Errorf("external tool command authority is incomplete")
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.commandBuilt {
		return nil, fmt.Errorf("external tool input lease already built a command")
	}
	if err := lease.validateLocked(); err != nil {
		return nil, err
	}
	for _, argument := range append(append([]string(nil), beforeInput...), afterInput...) {
		if strings.TrimSpace(argument) != "" && sameConversionCanonicalPath(argument, lease.source.DisplayPath()) {
			return nil, conversionInputFailure(
				ConversionInputCodeInternalContract,
				conversionInputStageExternalTool,
				lease.source.DisplayPath(),
				fmt.Errorf("external tool argv contains the public input path outside the lease slot"),
			)
		}
	}
	var inputArgument string
	var extraFiles []*os.File
	switch lease.transport {
	case externalToolInputTransportSnapshot:
		inputArgument = lease.snapshot.path
	case externalToolInputTransportLinuxFD:
		inputArgument = "/proc/self/fd/3"
		extraFiles = []*os.File{lease.inheritedFile}
	default:
		return nil, fmt.Errorf("external tool input lease transport is invalid")
	}
	args := make([]string, 0, len(beforeInput)+1+len(afterInput))
	args = append(args, beforeInput...)
	args = append(args, inputArgument)
	args = append(args, afterInput...)
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.ExtraFiles = extraFiles
	lease.commandBuilt = true
	return cmd, nil
}

func (lease *externalToolInputLease) Close() error {
	if lease == nil {
		return nil
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.closed {
		return nil
	}
	lease.closed = true
	var result error
	if lease.snapshot != nil {
		result = traceDBJoinPreservingSingle(result, lease.snapshot.Close())
		lease.snapshot = nil
	}
	if lease.inheritedFile != nil {
		result = traceDBJoinPreservingSingle(result, lease.inheritedFile.Close())
		lease.inheritedFile = nil
	}
	return result
}

func (lease *externalToolInputLease) detachSnapshotAsSealed() (*sealedConversionFile, conversionInputView, error) {
	if lease == nil {
		return nil, nil, fmt.Errorf("external tool input lease is nil")
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if err := lease.validateLocked(); err != nil {
		return nil, lease.source, err
	}
	if lease.transport != externalToolInputTransportSnapshot || lease.snapshot == nil || lease.inheritedFile != nil {
		return nil, lease.source, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			lease.source.DisplayPath(),
			fmt.Errorf("publishable external tool input requires the snapshot-only transport"),
		)
	}
	sealed, err := lease.snapshot.detachSealed()
	if err != nil {
		return nil, lease.source, err
	}
	source := lease.source
	lease.snapshot = nil
	lease.closed = true
	return sealed, source, nil
}

// sealExternalToolInputSnapshot is the snapshot-only terminal transition used
// when the same private generation must become a public sidecar after child
// processing. It validates both sides of the transfer and never reopens the
// private snapshot by pathname.
func sealExternalToolInputSnapshot(
	ctx context.Context,
	lease *externalToolInputLease,
	staging *privateConversionDir,
) (*sealedConversionFile, error) {
	if boundaryErr := validateExternalToolCommandBoundary(ctx, lease, staging, nil); boundaryErr != nil {
		if lease != nil {
			boundaryErr = traceDBJoinPreservingSingle(boundaryErr, lease.Close())
		}
		return nil, boundaryErr
	}
	sealed, source, err := lease.detachSnapshotAsSealed()
	if err != nil {
		return nil, traceDBJoinPreservingSingle(err, lease.Close())
	}
	finishErr := completeConversionInputStage(ctx, source, conversionInputStageExternalTool, nil)
	if staging == nil {
		finishErr = traceDBJoinPreservingSingle(finishErr, fmt.Errorf("external tool staging authority is nil"))
	} else {
		finishErr = traceDBJoinPreservingSingle(finishErr, staging.Validate())
	}
	finishErr = traceDBJoinPreservingSingle(finishErr, sealed.Validate())
	if finishErr != nil {
		return nil, traceDBJoinPreservingSingle(finishErr, sealed.Close())
	}
	return sealed, nil
}

// finishExternalToolCommand preserves existing provider fallback semantics:
// a child exit error alone remains soft for the caller to classify. Context,
// source/snapshot generation, staging security, or close failures are hard and
// retain the child error as secondary evidence.
func finishExternalToolCommand(
	ctx context.Context,
	lease *externalToolInputLease,
	staging *privateConversionDir,
	runErr error,
) error {
	boundaryErr := validateExternalToolCommandBoundary(ctx, lease, staging, runErr)
	var closeErr error
	if lease != nil {
		closeErr = lease.Close()
	}
	if boundaryErr != nil {
		return traceDBJoinPreservingSingle(boundaryErr, closeErr)
	}
	if closeErr != nil {
		return traceDBJoinPreservingSingle(closeErr, runErr)
	}
	return nil
}

// validateExternalToolCommandBoundary intentionally leaves lease open. A
// child-only error remains soft for provider fallback; context, generation, or
// staging authority failures retain the child error as secondary evidence.
func validateExternalToolCommandBoundary(
	ctx context.Context,
	lease *externalToolInputLease,
	staging *privateConversionDir,
	runErr error,
) error {
	var contextErr error
	if ctx != nil {
		contextErr = ctx.Err()
	}
	var validationErr error
	if lease == nil {
		validationErr = fmt.Errorf("external tool input lease is nil")
	} else {
		validationErr = lease.Validate()
	}
	var stagingErr error
	if staging == nil {
		stagingErr = fmt.Errorf("external tool staging authority is nil")
	} else {
		stagingErr = staging.Validate()
	}
	if contextErr != nil {
		return traceDBJoinPreservingSingle(contextErr, validationErr, stagingErr, runErr)
	}
	if validationErr != nil || stagingErr != nil {
		return traceDBJoinPreservingSingle(validationErr, stagingErr, runErr)
	}
	return nil
}
