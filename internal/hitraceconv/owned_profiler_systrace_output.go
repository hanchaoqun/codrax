package hitraceconv

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const ownedProfilerSystraceStagingPattern = ".codrax-profiler-systrace-*"

const (
	profilerOwnedRowProvenanceInvalid = "profiler_owned_row_provenance_invalid"
	profilerOwnedRowClassInvalid      = "profiler_owned_row_class_invalid"
)

type profilerSystracePublication struct {
	Artifact      Artifact
	TraceCoverage TraceDBCoverage
	Stats         traceDBRowSortStats
}

// profilerOwnedRowProfileBuilder consumes only authenticated, source-order
// verified rows selected by the sorter's final publication loop. It never
// observes staged or withheld rows, so every exceptional-row digest binds the
// exact physical coordinate used by the private generation.
type profilerOwnedRowProfileBuilder struct {
	rows    int
	known   int
	unknown ownedTraceRowDigestBuilder
}

func (builder *profilerOwnedRowProfileBuilder) observe(observation traceDBFinalRowObservation) error {
	if builder == nil || observation.LineNo <= 0 ||
		!traceDBSinglePhysicalLine(observation.Row.line, false) {
		return &ownedTraceOutputInvariantError{Reason: profilerOwnedRowProvenanceInvalid}
	}
	provenance := observation.Row.profilerProvenance()
	if provenance.PublisherSlot == profilerPairPublisherNone ||
		provenance.TraceClass == profilerTraceClassNone || !provenance.valid() {
		return &ownedTraceOutputInvariantError{Reason: profilerOwnedRowProvenanceInvalid}
	}
	if builder.rows == math.MaxInt {
		return &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationCountMismatch}
	}
	switch provenance.TraceClass {
	case profilerTraceClassStructuredKnown, profilerTraceClassTextKnown:
		if builder.known == math.MaxInt {
			return &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationCountMismatch}
		}
		builder.known++
	case profilerTraceClassTextIntentionalUnknown:
		builder.unknown.add(observation.LineNo, observation.Row.line)
	default:
		return &ownedTraceOutputInvariantError{Reason: profilerOwnedRowClassInvalid}
	}
	builder.rows++
	return nil
}

func (builder *profilerOwnedRowProfileBuilder) finish(
	stats traceDBRowSortStats,
	expectedRows int,
	wire ownedTraceWireDigest,
) (ownedTraceValidationProfile, error) {
	profile := ownedTraceValidationProfile{
		Kind:          ownedTraceValidationProfiler,
		CoverageTable: tracebundle.SystraceReceiptTableProfiler,
		ExpectedWire:  wire,
	}
	if builder == nil || expectedRows <= 0 || stats.RowsWritten != expectedRows ||
		builder.rows != expectedRows || builder.known < 0 || builder.known > builder.rows {
		return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationCountMismatch}
	}
	unknown := builder.unknown.finish()
	if !unknown.Valid || unknown.Rows != builder.rows-builder.known {
		return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationCountMismatch}
	}
	profile.ExpectedRows = builder.rows
	profile.ExpectedKnown = builder.known
	profile.ExpectedUnknown = unknown
	if reason := profile.validate(); reason != "" {
		return profile, &ownedTraceOutputInvariantError{Reason: reason}
	}
	return profile, nil
}

// writeValidatedOwnedProfilerSystraceWithLedger is the sole Profiler
// systrace publication throat. The public path remains absent until the final
// authenticated sorter rows, terminal ledger, complete wire, held tracequery
// verdict and exact-generation receipt all agree.
func writeValidatedOwnedProfilerSystraceWithLedger(
	ctx context.Context,
	outputPath string,
	sink *traceDBRowSink,
	extraction profilerContainerExtraction,
	terminal profilerTerminalPublicationLedger,
	ledger *conversionFileLedger,
) (publication profilerSystracePublication, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	expectedRows, countErr := profilerTerminalCountToInt(
		terminal.rows.published, "profiler_terminal_publication_rows_overflow",
	)
	if countErr != nil {
		return publication, newOwnedTracePublicationError("contract", outputPath, countErr)
	}
	if strings.TrimSpace(outputPath) == "" || sink == nil || ledger == nil ||
		expectedRows <= 0 || sink.captureLifecycle != profilerCaptureSealed ||
		!sink.profilerTraceClassification {
		return publication, newOwnedTracePublicationError(
			"contract", outputPath, errors.New("profiler systrace output contract is incomplete"),
		)
	}
	if err := ctx.Err(); err != nil {
		return publication, err
	}
	target, err := prepareSealedConversionPublicationTarget(outputPath, ownedProfilerSystraceStagingPattern)
	if err != nil {
		return publication, newOwnedTracePublicationError("prepare", outputPath, err)
	}
	privateStagingRoot := target.stagingDir.Path()
	targetCleanup := target.Cleanup
	defer func() {
		if targetCleanup != nil {
			cleanupErr := targetCleanup()
			targetCleanup = nil
			if cleanupErr != nil {
				resultErr = traceDBJoinPreservingSingle(
					resultErr,
					newOwnedTracePublicationError("cleanup_private", outputPath, cleanupErr),
				)
			}
		}
		resultErr = redactTraceStreamerPrivateError(resultErr, privateStagingRoot)
	}()

	out, err := os.OpenFile(target.StagingPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return publication, newOwnedTracePublicationError("create_private", outputPath, err)
	}
	wireHasher := newOwnedTraceWireHasher()
	profileBuilder := &profilerOwnedRowProfileBuilder{}
	stats, writeErr := sink.writeTo(ctx, io.MultiWriter(out, wireHasher), profileBuilder.observe)
	syncErr := out.Sync()
	closeErr := out.Close()
	publication.Stats = sink.stats
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return publication, newOwnedTracePublicationError(
			"finish_private", outputPath, writeErr, syncErr, closeErr,
		)
	}
	if stats != publication.Stats {
		return publication, newOwnedTracePublicationError(
			"sorter_stats", outputPath,
			&ownedTraceOutputInvariantError{Reason: traceDBPostvalidationCountMismatch},
		)
	}
	if err := validateProfilerTerminalWrittenProjection(extraction, terminal, sink); err != nil {
		return publication, newOwnedTracePublicationError("terminal_projection", outputPath, err)
	}
	wire := wireHasher.finish()
	if !wire.Valid {
		return publication, newOwnedTracePublicationError(
			"wire_digest", outputPath, errors.New("profiler systrace wire digest is unavailable"),
		)
	}
	profile, err := profileBuilder.finish(publication.Stats, expectedRows, wire)
	if err != nil {
		return publication, newOwnedTracePublicationError("expected_profile", outputPath, err)
	}

	sealedOutput, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		return publication, newOwnedTracePublicationError("adopt_private", outputPath, err)
	}
	sealedOpen := true
	defer func() {
		if sealedOpen {
			if closeErr := sealedOutput.Close(); closeErr != nil {
				resultErr = traceDBJoinPreservingSingle(
					resultErr,
					newOwnedTracePublicationError("close_held", outputPath, closeErr),
				)
			}
		}
	}()

	validatedReceipt, traceCoverage, err := validateOwnedTraceOutput(
		ctx, sealedOutput, target.finalBindingPath, profile,
	)
	publication.TraceCoverage = traceCoverage
	if err != nil {
		publication.TraceCoverage.ArtifactPath = ""
		return publication, err
	}
	if err := publishValidatedOwnedTraceOutputNoReplace(
		ctx, target, sealedOutput, validatedReceipt, ledger,
	); err != nil {
		return publication, newOwnedTracePublicationError("publish", outputPath, err)
	}
	rollbackPublished := func(stage string, cause error) error {
		return newOwnedTracePublicationError(
			stage, outputPath, cause, ledger.removeOwnedPath(target.finalBindingPath),
		)
	}
	if err := ctx.Err(); err != nil {
		return publication, rollbackPublished("post_publish_context", err)
	}
	cleanupErr := targetCleanup()
	targetCleanup = nil
	if cleanupErr != nil {
		return publication, rollbackPublished("cleanup_private", cleanupErr)
	}
	if err := ctx.Err(); err != nil {
		return publication, rollbackPublished("post_cleanup_context", err)
	}
	publication.Artifact, err = newValidatedSystraceArtifact(
		ledger,
		target.finalBindingPath,
		ownedTraceValidationProfiler,
		[]string{"generated from OpenHarmony profiler/session plugin payloads"},
	)
	if err != nil {
		return publication, rollbackPublished("read_public_receipt", err)
	}
	if err := ctx.Err(); err != nil {
		publication.Artifact = Artifact{}
		return publication, rollbackPublished("post_receipt_context", err)
	}
	if err := sealedOutput.Close(); err != nil {
		publication.Artifact = Artifact{}
		return publication, rollbackPublished("close_held", err)
	}
	sealedOpen = false
	publication.TraceCoverage.ArtifactPath = publication.Artifact.Path
	return publication, nil
}
