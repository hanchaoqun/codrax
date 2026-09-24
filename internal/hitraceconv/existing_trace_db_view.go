package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// prepareExistingTraceDBFromView shares the sealed SQLite reader/exporter with
// transported input. The caller owns the source authority and transaction;
// this function must not reopen DisplayPath or start another input ledger.
func prepareExistingTraceDBFromView(
	ctx context.Context, opts Options, source conversionInputView, output, retainedPath string,
	ledger *conversionFileLedger, receipt ExistingTraceDBSource, validateBoundary func() error,
) (result Result, resultErr error) {
	defer func() {
		if resultErr != nil {
			result = Result{}
		}
	}()
	if source == nil || ledger == nil || validateBoundary == nil || receipt.Path != source.DisplayPath() || receipt.Bytes != source.Size() || receipt.Generation == "" {
		return Result{}, fmt.Errorf("existing trace DB view binding is incomplete")
	}
	if err := validateBoundary(); err != nil {
		return Result{}, err
	}
	var staging *privateConversionDir
	var target sealedConversionPublicationTarget
	leaf := sealedTraceDBVirtualName
	if retainedPath != "" {
		var err error
		target, err = prepareSealedConversionPublicationTargetWithLedger(retainedPath, "existing-db-*", ledger)
		if err != nil {
			return Result{}, err
		}
		staging, leaf = target.stagingDir, target.finalLeaf
		defer func() { resultErr = traceDBJoinPreservingSingle(resultErr, target.Cleanup()) }()
	} else {
		var err error
		staging, err = newRuntimePrivateConversionDir(ledger.stagingRoot, "existing-db-*")
		if err != nil {
			return Result{}, err
		}
		defer func() { resultErr = traceDBJoinPreservingSingle(resultErr, staging.FinalizeCleanup()) }()
	}
	start := progressStarted(opts, "existing_trace_db_snapshot", "preparing closed SQLite trace snapshot", source.DisplayPath(), output)
	lease, err := newExternalToolInputLeaseWithProgress(ctx, source, staging, leaf, externalToolInputSnapshotOnly, nil)
	if err != nil {
		return Result{}, err
	}
	sealed, err := sealExternalToolInputSnapshot(ctx, lease, staging)
	if err != nil {
		return Result{}, err
	}
	defer func() { resultErr = traceDBJoinPreservingSingle(resultErr, sealed.Close()) }()
	progressFinished(opts, "existing_trace_db_snapshot", "closed SQLite trace snapshot prepared", source.DisplayPath(), output, start, ProgressStatusComplete)
	if err := validateBoundary(); err != nil {
		return Result{}, err
	}
	hasher := sha256.New()
	if _, err := copyCancellableRange(ctx, hasher, io.NewSectionReader(sealed, 0, sealed.Size()), nil); err != nil {
		return Result{}, err
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	if sealed.Size() != receipt.Bytes || receipt.SHA256 != "" && digest != receipt.SHA256 {
		return Result{}, fmt.Errorf("existing trace DB sealed snapshot differs from the bound payload")
	}
	receipt.SHA256 = digest
	start = progressStarted(opts, "existing_trace_db_export", "reading closed SQLite trace snapshot", source.DisplayPath(), output)
	exported, err := exportTraceDBToSystraceFromSealedWithLedger(ctx, sealed, source.DisplayPath(), output, ledger)
	if err != nil {
		return Result{}, err
	}
	if exported.Artifact.Trace == nil || !exported.Artifact.Trace.TraceQueryReady {
		return Result{}, fmt.Errorf("existing SQLite DB contains no query-ready trace rows under the supported schema, owner and clock contracts")
	}
	decision := newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameTraceStreamer), Options{TraceEngine: traceEngineTraceStreamer}, source.DisplayPath(), output)
	decision, err = traceProviderPublished(decision, exported.Artifact, ledger)
	if err != nil {
		return Result{}, err
	}
	decision.Caveat = "existing closed SQLite database normalized read-only; trace_streamer executable was not invoked"
	if ledger.gzip != nil {
		decision.Caveat = "decoded self-contained SQLite snapshot normalized read-only; trace_streamer executable was not invoked; pre-compression database lifecycle is not established"
	}
	result = Result{
		InputPath: source.DisplayPath(), InputBytes: source.Size(), OutputPath: exported.Artifact.Path,
		OutputBytes: exported.OutputBytes, EventsWritten: exported.EventsWritten,
		FirstTimestampSec: exported.FirstTimestampSec, LastTimestampSec: exported.LastTimestampSec,
		Artifacts: []Artifact{exported.Artifact}, TraceDecisions: []TraceProviderDecision{decision},
		TraceDBCoverage: exported.Coverage, TraceCoverage: exported.TraceCoverage,
		Caveats:               append([]string{decision.Caveat}, traceDBSemanticQualityCaveats(exported.Coverage)...),
		ExistingTraceDBSource: &receipt,
	}
	if retainedPath != "" {
		// Publish the same sealed generation read by SQLite, not a path reopened
		// after export and not a second converter output.
		if err := publishSealedConversionFileNoReplace(ctx, target, sealed, ledger); err != nil {
			return Result{}, err
		}
		result.Artifacts = append(result.Artifacts, Artifact{
			Type: ArtifactTraceDB, Path: retainedPath, Bytes: receipt.Bytes, SHA256: receipt.SHA256,
			Converter: traceStreamerConverter,
			Caveats:   []string{"SQLite payload retained unchanged from gzip input; no converter invoked"},
		})
	}
	if err := finalizeResultTraceBundleWithLedger(ctx, source.DisplayPath(), output, &result, ledger); err != nil {
		return Result{}, err
	}
	progressFinished(opts, "existing_trace_db_export", "closed SQLite trace exported", source.DisplayPath(), output, start, ProgressStatusComplete)
	if err := sealed.Validate(); err != nil {
		return Result{}, err
	}
	if err := validateBoundary(); err != nil {
		return Result{}, err
	}
	return result, nil
}
