package hitraceconv

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const ownedBuiltinSystraceStagingPattern = ".codrax-builtin-systrace-*"

type builtinSystracePublication struct {
	Artifact      Artifact
	TraceCoverage TraceDBCoverage
}

// writeValidatedOwnedBuiltinSystraceWithLedger is the sole built-in RMQ
// systrace publication throat. The public path remains absent until bytes,
// producer provenance and the held tracequery verdict all close on one private
// generation. The returned Artifact is read back from that public receipt.
func writeValidatedOwnedBuiltinSystraceWithLedger(
	ctx context.Context,
	outputPath string,
	rows []renderedRow,
	ledger *conversionFileLedger,
) (publication builtinSystracePublication, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(outputPath) == "" || ledger == nil {
		return publication, newOwnedTracePublicationError(
			"contract", outputPath, errors.New("builtin systrace output contract is incomplete"),
		)
	}
	if err := ctx.Err(); err != nil {
		return publication, err
	}
	target, err := prepareSealedConversionPublicationTarget(outputPath, ownedBuiltinSystraceStagingPattern)
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
	buffered := bufio.NewWriterSize(io.MultiWriter(out, wireHasher), 256*1024)
	profile, writeErr := writeOwnedBuiltinSystraceRows(ctx, buffered, rows)
	flushErr := buffered.Flush()
	syncErr := out.Sync()
	closeErr := out.Close()
	if writeErr != nil || flushErr != nil || syncErr != nil || closeErr != nil {
		return publication, newOwnedTracePublicationError(
			"finish_private", outputPath, writeErr, flushErr, syncErr, closeErr,
		)
	}
	profile.ExpectedWire = wireHasher.finish()
	if !profile.ExpectedWire.Valid {
		return publication, newOwnedTracePublicationError(
			"wire_digest", outputPath, errors.New("builtin systrace wire digest is unavailable"),
		)
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
		ledger, target.finalBindingPath, ownedTraceValidationBuiltin, nil,
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

// writeOwnedBuiltinSystraceRows writes the fixed header and the final ordered
// body through one byte stream while building only bounded aggregate digests.
// The returned profile cannot be published until the caller attaches the wire
// digest from that same write point.
func writeOwnedBuiltinSystraceRows(
	ctx context.Context,
	w io.Writer,
	rows []renderedRow,
) (ownedTraceValidationProfile, error) {
	profile := ownedTraceValidationProfile{
		Kind:          ownedTraceValidationBuiltin,
		CoverageTable: tracebundle.SystraceReceiptTableBuiltin,
		ExpectedRows:  len(rows),
		AllowZeroRows: true,
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if w == nil {
		return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationGenerationInvalid}
	}
	if err := ctx.Err(); err != nil {
		return profile, err
	}
	if err := writeSystraceHeader(w); err != nil {
		return profile, err
	}
	headerLines := strings.Count(systraceHeader, "\n")
	if len(rows) > math.MaxInt-headerLines {
		return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationCountMismatch}
	}
	var advisoryDigest ownedTraceRowDigestBuilder
	var unknownDigest ownedTraceRowDigestBuilder
	var unparsedDigest ownedTraceRowDigestBuilder
	for index, row := range rows {
		if err := ctx.Err(); err != nil {
			return profile, err
		}
		if !row.builtinProvenance.valid() || !traceDBSinglePhysicalLine(row.line, false) {
			return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationUnparsedOwnedRow}
		}
		lineNo := headerLines + index + 1
		event, parsed, parseErr := parseOwnedBuiltinRow(lineNo, row.line)
		if parseErr != nil {
			return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationParsePanic, Cause: parseErr}
		}
		switch row.builtinProvenance {
		case builtinRowProvenanceNone:
			if !parsed {
				return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationUnparsedOwnedRow}
			}
			if event.Type == tracequery.EventUnknown || ownedBuiltinAdvisoryEvent(event.Name, event.Type) {
				return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationUnknownOwnedRow}
			}
			profile.ExpectedKnown++
		case builtinRowProvenanceOpaqueMarkerAdvisory:
			if !parsed || !ownedBuiltinAdvisoryEvent(event.Name, event.Type) {
				return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationUnknownOwnedRow}
			}
			advisoryDigest.add(lineNo, row.line)
			if event.Type == tracequery.EventUnknown {
				unknownDigest.add(lineNo, row.line)
			} else {
				profile.ExpectedKnown++
			}
		case builtinRowProvenanceIntentionalHeaderOnly:
			if parsed || !canonicalOwnedBuiltinHeaderOnlyLine(lineNo, row.line) {
				return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationUnparsedOwnedRow}
			}
			unparsedDigest.add(lineNo, row.line)
		default:
			return profile, &ownedTraceOutputInvariantError{Reason: traceDBPostvalidationCountMismatch}
		}
		if _, err := io.WriteString(w, row.line); err != nil {
			return profile, err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return profile, err
		}
	}
	profile.ExpectedAdvisory = advisoryDigest.finish()
	profile.ExpectedUnknown = unknownDigest.finish()
	profile.ExpectedUnparsed = unparsedDigest.finish()
	// ExpectedWire is intentionally attached only after flush/sync/close. Use a
	// temporary valid sentinel to prove every other profile invariant now.
	profile.ExpectedWire.Valid = true
	reason := profile.validate()
	profile.ExpectedWire = ownedTraceWireDigest{}
	if reason != "" {
		return profile, &ownedTraceOutputInvariantError{Reason: reason}
	}
	return profile, nil
}

func parseOwnedBuiltinRow(lineNo int, line string) (event tracequery.Event, parsed bool, resultErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			resultErr = fmt.Errorf("tracequery parser panic: %v", recovered)
			event = tracequery.Event{}
			parsed = false
		}
	}()
	event, parsed = tracequery.ParseLine(lineNo, line, nil)
	return event, parsed, nil
}

func canonicalOwnedBuiltinHeaderOnlyLine(lineNo int, line string) bool {
	if !strings.HasSuffix(line, ": ") {
		return false
	}
	probe, parsed, err := parseOwnedBuiltinRow(lineNo, line+"print: I|0|codrax_header_probe")
	return err == nil && parsed && probe.Name == "print" && probe.Type == tracequery.EventTraceMark
}
