package hitraceconv

import (
	"bufio"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const ownedPerfTraceStagingPattern = ".codrax-perftrace-output-*"

// ownedPerfTraceWriteSpec is the closed authorization for the four owned
// perftrace writers. In particular, zero rows are derived from a validated
// raw record census; no caller-controlled boolean can open that lane.
type ownedPerfTraceWriteSpec struct {
	Profile                ownedTracePerfProfile
	ExpectedRows           int
	RawCaptureCompleteness *RawPerfCaptureCompleteness
	RawCaptureResidual     *RawPerfCaptureResidual
	RawSampleAdmission     *RawPerfSampleAdmission
}

func (spec ownedPerfTraceWriteSpec) normalize() (ownedPerfTraceWriteSpec, string, string, error) {
	requiredSource, requiredClock, validProfile := spec.Profile.sourceClock()
	if !validProfile || spec.ExpectedRows < 0 {
		return ownedPerfTraceWriteSpec{}, "", "", errors.New("perftrace write profile is incomplete")
	}
	if spec.Profile != ownedTracePerfRaw {
		if spec.ExpectedRows <= 0 || spec.RawCaptureCompleteness != nil || spec.RawCaptureResidual != nil ||
			spec.RawSampleAdmission != nil {
			return ownedPerfTraceWriteSpec{}, "", "", errors.New("nonraw perftrace write profile cannot carry raw inventory semantics")
		}
		return spec, requiredSource, requiredClock, nil
	}
	if spec.RawCaptureCompleteness == nil || spec.RawCaptureResidual == nil || spec.RawSampleAdmission == nil {
		return ownedPerfTraceWriteSpec{}, "", "", errors.New("raw perftrace write profile requires capture completeness, residual, and sample admission")
	}
	capture := *spec.RawCaptureCompleteness
	if reason := validateRawPerfCaptureCompleteness(capture); reason != "" {
		return ownedPerfTraceWriteSpec{}, "", "", errors.New("raw perftrace capture completeness is invalid: " + reason)
	}
	admission := *spec.RawSampleAdmission
	if reason := validateRawPerfSampleAdmission(admission); reason != "" {
		return ownedPerfTraceWriteSpec{}, "", "", errors.New("raw perftrace sample admission is invalid: " + reason)
	}
	if admission.Candidates != capture.SampleRecords.Accepted || admission.QueryRows > uint64(math.MaxInt) ||
		int(admission.QueryRows) != spec.ExpectedRows {
		return ownedPerfTraceWriteSpec{}, "", "", errors.New("raw perftrace sample admission does not match capture completeness or output rows")
	}
	if spec.ExpectedRows == 0 {
		hasIssue, err := rawPerfCaptureHasPublicationIssue(capture)
		if err != nil || !hasIssue && !rawPerfSampleAdmissionHasIssue(admission) {
			return ownedPerfTraceWriteSpec{}, "", "", errors.New("raw perftrace zero-row inventory has no deterministic publication issue")
		}
	}
	residual := *spec.RawCaptureResidual
	if reason := validateRawPerfCaptureResidual(residual); reason != "" {
		return ownedPerfTraceWriteSpec{}, "", "", errors.New("raw perftrace capture residual is invalid: " + reason)
	}
	spec.RawCaptureCompleteness = &capture
	spec.RawCaptureResidual = &residual
	spec.RawSampleAdmission = &admission
	return spec, requiredSource, requiredClock, nil
}

// ownedTracePublicationError identifies failures after converter data has
// entered Codrax's output-publication boundary. Provider fallback may recover
// from an unsupported external report, but it must never hide our own staging,
// validation, generation, receipt, or publication failure.
type ownedTracePublicationError struct {
	Stage      string
	PublicPath string
	Cause      error
}

func (failure *ownedTracePublicationError) Error() string {
	if failure == nil {
		return "validated owned trace publication failed"
	}
	message := "validated owned trace publication failed"
	if strings.TrimSpace(failure.Stage) != "" {
		message += " at " + failure.Stage
	}
	if strings.TrimSpace(failure.PublicPath) != "" {
		message += " for " + failure.PublicPath
	}
	return message
}

func (failure *ownedTracePublicationError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

func newOwnedTracePublicationError(stage, publicPath string, cause ...error) error {
	return &ownedTracePublicationError{
		Stage: strings.TrimSpace(stage), PublicPath: strings.TrimSpace(publicPath),
		Cause: traceDBJoinPreservingSingle(nil, cause...),
	}
}

func ownedTraceOutputHardFailure(err error) bool {
	if err == nil {
		return false
	}
	var invariant *ownedTraceOutputInvariantError
	var publication *ownedTracePublicationError
	return errors.As(err, &invariant) || errors.As(err, &publication) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// writeValidatedOwnedPerfTraceWithLedger is the sole four-provider perftrace
// output throat. Bytes are written only to a private sibling, hashed at the
// same write point, synced and closed, then parsed from one held generation.
// The public name becomes visible only after semantic validation and exact
// snapshot publication; the returned claim is read back from the ledger and
// binds the receipt to the exact public generation.
func writeValidatedOwnedPerfTraceWithLedger(
	ctx context.Context,
	spec ownedPerfTraceWriteSpec,
	outputPath string,
	ledger *conversionFileLedger,
	write func(io.Writer) error,
) (published publishedOwnedTraceValidation, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalizedSpec, requiredSource, requiredClock, specErr := spec.normalize()
	if specErr != nil {
		return published, newOwnedTracePublicationError("contract", outputPath, specErr)
	}
	if strings.TrimSpace(outputPath) == "" || ledger == nil || write == nil {
		return published, newOwnedTracePublicationError("contract", outputPath, errors.New("perftrace output contract is incomplete"))
	}
	spec = normalizedSpec
	if err := ctx.Err(); err != nil {
		return published, err
	}
	target, err := prepareSealedConversionPublicationTarget(outputPath, ownedPerfTraceStagingPattern)
	if err != nil {
		return published, newOwnedTracePublicationError("prepare", outputPath, err)
	}
	privateStagingRoot := target.stagingDir.Path()
	targetCleanup := target.Cleanup
	defer func() {
		if targetCleanup != nil {
			cleanupErr := targetCleanup()
			targetCleanup = nil
			if cleanupErr != nil {
				resultErr = traceDBJoinPreservingSingle(resultErr, newOwnedTracePublicationError("cleanup_private", outputPath, cleanupErr))
			}
		}
		resultErr = redactTraceStreamerPrivateError(resultErr, privateStagingRoot)
	}()

	out, err := os.OpenFile(target.StagingPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return published, newOwnedTracePublicationError("create_private", outputPath, err)
	}
	wireHasher := newOwnedTraceWireHasher()
	buffered := bufio.NewWriter(io.MultiWriter(out, wireHasher))
	writeErr := write(buffered)
	flushErr := buffered.Flush()
	syncErr := out.Sync()
	closeErr := out.Close()
	if writeErr != nil || flushErr != nil || syncErr != nil || closeErr != nil {
		return published, newOwnedTracePublicationError("finish_private", outputPath, writeErr, flushErr, syncErr, closeErr)
	}
	expectedWire := wireHasher.finish()
	if !expectedWire.Valid {
		return published, newOwnedTracePublicationError("wire_digest", outputPath, errors.New("perftrace wire digest is unavailable"))
	}

	sealedOutput, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		return published, newOwnedTracePublicationError("adopt_private", outputPath, err)
	}
	defer func() {
		if closeErr := sealedOutput.Close(); closeErr != nil {
			resultErr = traceDBJoinPreservingSingle(resultErr, newOwnedTracePublicationError("close_held", outputPath, closeErr))
		}
	}()

	profile := ownedTraceValidationProfile{
		Kind:                 ownedTraceValidationPerf,
		PerfProfile:          spec.Profile,
		ExpectedRows:         spec.ExpectedRows,
		ExpectedKnown:        spec.ExpectedRows,
		ExpectedWire:         expectedWire,
		RequiredEventType:    tracequery.EventPerfSample,
		RequiredPerfSource:   requiredSource,
		RequiredPerfClock:    requiredClock,
		RequirePerfIntegrity: true,
		AllowZeroRows:        spec.ExpectedRows == 0,
	}
	profile.CoverageTable, _ = spec.Profile.coverageTable()
	if spec.RawCaptureCompleteness != nil {
		profile.RawCaptureCompleteness = *spec.RawCaptureCompleteness
		profile.HasRawCaptureCompleteness = true
	}
	if spec.RawCaptureResidual != nil {
		profile.RawCaptureResidual = *spec.RawCaptureResidual
		profile.HasRawCaptureResidual = true
	}
	if spec.RawSampleAdmission != nil {
		profile.RawSampleAdmission = *spec.RawSampleAdmission
		profile.HasRawSampleAdmission = true
	}
	validatedReceipt, _, err := validateOwnedTraceOutput(ctx, sealedOutput, target.finalBindingPath, profile)
	if err != nil {
		return published, err
	}
	if err := publishValidatedOwnedTraceOutputNoReplace(ctx, target, sealedOutput, validatedReceipt, ledger); err != nil {
		return published, newOwnedTracePublicationError("publish", outputPath, err)
	}
	cleanupErr := targetCleanup()
	targetCleanup = nil
	if cleanupErr != nil {
		rollbackErr := ledger.removeOwnedPath(target.finalBindingPath)
		return published, newOwnedTracePublicationError("cleanup_private", outputPath, cleanupErr, rollbackErr)
	}
	published, ok := ledger.ownedTraceValidation(target.finalBindingPath)
	if !ok {
		rollbackErr := ledger.removeOwnedPath(target.finalBindingPath)
		return published, newOwnedTracePublicationError("read_public_receipt", outputPath, errors.New("published perftrace receipt is unavailable"), rollbackErr)
	}
	return published, nil
}
