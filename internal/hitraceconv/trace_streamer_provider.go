package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	traceStreamerConverter         = converterVersion + "+trace-streamer-db"
	traceStreamerPrivateDirPattern = "ts-*"
)

type traceStreamerExportResult struct {
	Artifact          Artifact
	SystraceArtifact  Artifact
	Decision          TraceProviderDecision
	Coverage          []TraceDBCoverage
	TraceCoverage     []TraceDBCoverage
	Caveats           []string
	Ran               bool
	EventsWritten     int
	OutputBytes       int64
	FirstTimestampSec float64
	LastTimestampSec  float64
	FailureStage      string
	FailureCode       string
	Cause             error
}

func convertTraceStreamerOnly(ctx context.Context, opts Options, plan traceProviderPlan, inventory standaloneSegmentInventory, output string, ledger *conversionFileLedger) (Result, error) {
	input := inventory.input
	if input == nil {
		return Result{}, conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageStandaloneScan, "", fmt.Errorf("standalone inventory has no input authority"))
	}
	inputPath := input.DisplayPath()
	inputBytes := input.Size()
	if err := ensureOutputDoesNotExist(output); err != nil {
		return Result{}, err
	}
	retainDB := opts.KeepTraceDB || strings.TrimSpace(opts.TraceDBOutputPath) != ""
	export, err := runTraceStreamerExport(ctx, opts, plan.TraceStreamer, input, output, opts.KeepTraceDB, ledger)
	if err != nil {
		return Result{}, err
	}
	if retainDB && (export.Artifact.Path == "" || !artifactPathExists(export.Artifact.Path)) {
		if export.Decision.Caveat != "" {
			return Result{}, traceStreamerExportFailureError(export, plan.TraceStreamer)
		}
		return Result{}, fmt.Errorf("trace_streamer did not produce a trace DB artifact")
	}
	if export.SystraceArtifact.Path == "" && !retainDB {
		if export.Decision.Caveat != "" {
			return Result{}, traceStreamerExportFailureError(export, plan.TraceStreamer)
		}
		return Result{}, fmt.Errorf("trace_streamer did not produce query-ready systrace rows")
	}
	standaloneExtractOpts := standaloneExtractOptions{GeneratePerfTrace: true}
	if export.SystraceArtifact.Path != "" && traceDBCoverageHasPerfSamples(export.Coverage) {
		standaloneExtractOpts.GeneratePerfTrace = false
		standaloneExtractOpts.PrimaryPerfSource = "trace_streamer DB perf_sample rows in systrace"
	}
	standaloneArtifacts, standaloneCaveats, standaloneDecisions, err := extractStandaloneArtifactsWithOptionsAndLedger(ctx, opts, inventory, output, standaloneExtractOpts, ledger)
	if err != nil {
		if export.SystraceArtifact.Path != "" {
			err = traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(export.SystraceArtifact.Path))
		}
		return Result{}, err
	}
	artifacts := append([]Artifact(nil), standaloneArtifacts...)
	if export.SystraceArtifact.Path != "" {
		artifacts = append([]Artifact{export.SystraceArtifact}, artifacts...)
	}
	if retainDB {
		artifacts = append(artifacts, export.Artifact)
	}
	caveats := append([]string(nil), export.Caveats...)
	caveats = append(caveats, standaloneCaveats...)
	result := Result{
		InputPath:          inputPath,
		InputBytes:         inputBytes,
		OutputPath:         export.SystraceArtifact.Path,
		OutputBytes:        export.OutputBytes,
		Artifacts:          artifacts,
		ProviderDecisions:  append([]PerfProviderDecision(nil), standaloneDecisions...),
		TraceDecisions:     []TraceProviderDecision{export.Decision},
		TraceDBCoverage:    append([]TraceDBCoverage(nil), export.Coverage...),
		TraceCoverage:      append([]TraceDBCoverage(nil), export.TraceCoverage...),
		Caveats:            caveats,
		EventsWritten:      export.EventsWritten,
		FirstTimestampSec:  export.FirstTimestampSec,
		LastTimestampSec:   export.LastTimestampSec,
		MissingFormatCount: 0,
		UnknownEventCount:  0,
	}
	layoutCaveats, layoutCoverage := standaloneLayoutRejectionEvidence(inventory)
	result.Caveats = append(result.Caveats, layoutCaveats...)
	result.TraceCoverage = append(result.TraceCoverage, layoutCoverage...)
	if err := finalizeResultTraceBundleWithLedger(ctx, inputPath, result.OutputPath, &result, ledger); err != nil {
		return Result{}, err
	}
	return result, nil
}

func maybeRunTraceStreamerAuto(ctx context.Context, opts Options, plan traceProviderPlan, input conversionInputView, output string, hasTracePerfSidecar bool, ledger *conversionFileLedger) (traceStreamerExportResult, error) {
	if input == nil {
		return traceStreamerExportResult{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			"",
			fmt.Errorf("trace_streamer input authority is missing"),
		)
	}
	inputPath := input.DisplayPath()
	if !plan.includesEngine(traceEngineTraceStreamer) {
		return traceStreamerExportResult{}, nil
	}
	if !plan.TraceStreamer.Available {
		caveat := "trace_streamer was not discovered; selected SQL trace conversion cannot produce systrace"
		if isAutoTraceEngineMode(opts.TraceEngine) {
			if hasTracePerfSidecar {
				caveat = "trace_streamer was not discovered; auto trace+perf conversion will use built-in raw trace parsing and standalone perf fallback"
			} else {
				caveat = "trace_streamer was not discovered; auto trace conversion will use the built-in raw trace parser"
			}
		}
		decision := traceProviderSkipped(
			newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameTraceStreamer), opts, inputPath, output),
			false,
			"trace_streamer_unavailable",
			caveat,
		)
		cause := fmt.Errorf("trace_streamer provider is unavailable from source %q path %q", plan.TraceStreamer.Source, plan.TraceStreamer.Path)
		return traceStreamerExportResult{
			Decision:     decision,
			Caveats:      traceStreamerFailureCaveats(plan.TraceStreamer, caveat),
			FailureStage: "trace_streamer_discovery",
			FailureCode:  "trace_streamer_unavailable",
			Cause:        cause,
		}, nil
	}
	return runTraceStreamerExport(ctx, opts, plan.TraceStreamer, input, output, opts.KeepTraceDB, ledger)
}

func runTraceStreamerExport(ctx context.Context, opts Options, lane traceProviderLanePlan, input conversionInputView, output string, keepDB bool, ledger *conversionFileLedger) (result traceStreamerExportResult, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil {
		return traceStreamerExportResult{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			"",
			fmt.Errorf("trace_streamer input authority is missing"),
		)
	}
	inputPath := input.DisplayPath()
	decision := newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameTraceStreamer), opts, inputPath, output)
	decision.Selected = true
	decision.Attempted = true
	if !lane.Available || strings.TrimSpace(lane.Path) == "" {
		cause := fmt.Errorf("trace_streamer is not available; run codrax trace convert --trace-tools-status or pass --trace-streamer /path/to/trace_streamer")
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_streamer_unavailable", "trace_streamer engine was selected but trace_streamer is not available"),
			Caveats:  traceStreamerFailureCaveats(lane, "trace_streamer engine was selected but trace_streamer is not available"),
			Ran:      false, FailureStage: "trace_streamer_discovery", FailureCode: "trace_streamer_unavailable", Cause: cause,
		}, nil
	}
	dbTarget, err := prepareTraceStreamerDBTarget(opts, inputPath, output, keepDB)
	if err != nil {
		return traceStreamerExportResult{}, fmt.Errorf("prepare trace_streamer DB staging path: %w", err)
	}
	privateStagingPath := dbTarget.stagingDir.Path()
	defer func() {
		redactTraceStreamerExportResult(&result, privateStagingPath)
		resultErr = redactTraceStreamerPrivateError(resultErr, privateStagingPath)
	}()
	cleanup := dbTarget.Cleanup
	dbPath := dbTarget.StagingPath
	if err := dbTarget.validateStaging(); err != nil {
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(err, cleanupErr)
	}
	snapshotLeaf, err := traceStreamerInputSnapshotLeafForView(input)
	if err != nil {
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(err, cleanupErr)
	}
	snapshotLeaf, snapshotLeafCompacted := traceStreamerInputSnapshotLeafForPlatform(
		privateStagingPath, snapshotLeaf, runtime.GOOS,
	)
	snapshotStart := progressStarted(
		opts,
		"trace_streamer_input_snapshot",
		"preparing immutable trace_streamer input",
		inputPath,
		output,
	)
	lastSnapshotProgress := snapshotStart
	inputLease, err := newExternalToolInputLeaseWithProgress(
		ctx,
		input,
		dbTarget.stagingDir,
		snapshotLeaf,
		lane.ExternalInputProfile,
		func(done, total int64) {
			now := time.Now()
			if done != total && now.Sub(lastSnapshotProgress) < progressHeartbeatInterval {
				return
			}
			lastSnapshotProgress = now
			emitProgress(opts, ProgressEvent{
				Stage:      "trace_streamer_input_snapshot",
				Status:     ProgressStatusProgress,
				Message:    "copying immutable trace_streamer input",
				Path:       inputPath,
				OutputPath: output,
				BytesDone:  done,
				BytesTotal: total,
				Elapsed:    now.Sub(snapshotStart),
			})
		},
	)
	if err != nil {
		progressFinished(opts, "trace_streamer_input_snapshot", "trace_streamer input snapshot failed", inputPath, output, snapshotStart, ProgressStatusFailed)
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(err, cleanupErr)
	}
	progressFinished(opts, "trace_streamer_input_snapshot", "prepared immutable trace_streamer input", inputPath, output, snapshotStart, ProgressStatusComplete)
	cmd, err := inputLease.Command(ctx, lane.Path, nil, traceStreamerExportArguments(dbPath, opts.TraceStreamerSoDirs))
	if err != nil {
		boundaryErr := finishExternalToolCommand(ctx, inputLease, dbTarget.stagingDir, nil)
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(err, boundaryErr, cleanupErr)
	}
	if lane.EmbeddedLinuxRuntime {
		// The integrity-verified embedded child and its loader preflight use
		// one deterministic runtime closure. Caller-selected providers retain
		// their caller-owned environments.
		cmd.setEnvironment(embeddedTraceStreamerRuntimeEnvironment(os.Environ()))
	}
	combined, runErr, commandStart, commandStarted := runCommandWithProgressUntilExit(opts, cmd, "trace_streamer_export", "running trace_streamer SQLite DB export")
	if err := finishExternalToolCommand(ctx, inputLease, dbTarget.stagingDir, runErr); err != nil {
		progressFinished(opts, "trace_streamer_export", "trace_streamer command boundary rejected", lane.Path, "", commandStart, ProgressStatusFailed)
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(err, cleanupErr)
	}
	commandStatus := ProgressStatusComplete
	if runErr != nil {
		commandStatus = ProgressStatusFailed
	}
	commandMessage := terminalProgressMessage("running trace_streamer SQLite DB export", commandStatus)
	if !commandStarted {
		commandMessage = "external command failed to start"
	}
	progressFinished(opts, "trace_streamer_export", commandMessage, lane.Path, "", commandStart, commandStatus)
	if runErr != nil {
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		if ctxErr := ctx.Err(); ctxErr != nil {
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(ctxErr, runErr, cleanupErr)
		}
		caveat := fmt.Sprintf("trace_streamer DB export failed (%s)%s%s", runErr,
			boundedTraceStreamerCommandOutput(combined),
			traceStreamerEmptyChildDiagnostic(runtime.GOOS, combined, lane.Path,
				filepath.Join(privateStagingPath, snapshotLeaf), dbPath, snapshotLeafCompacted))
		if cleanupErr != nil {
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(traceStreamerProviderAttemptError("trace_streamer_export", "trace_streamer_failed", caveat, runErr), cleanupErr)
		}
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_streamer_failed", caveat),
			Caveats:  traceStreamerFailureCaveats(lane, caveat),
			Ran:      true, FailureStage: "trace_streamer_export", FailureCode: "trace_streamer_failed", Cause: runErr,
		}, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(ctxErr, cleanupErr)
	}
	sealedOutputs, adoptionErr := adoptTraceStreamerDBOutputs(dbTarget.stagingDir)
	if adoptionErr != nil {
		failureCode, recoverable := traceStreamerDBOutputValidationCode(adoptionErr)
		caveat := fmt.Sprintf("trace_streamer DB export completed but its output set could not be sealed: %v", adoptionErr)
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		if !recoverable {
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(
				traceStreamerProviderAttemptError("trace_db_validate", failureCode, caveat, adoptionErr), cleanupErr,
			)
		}
		if cleanupErr != nil {
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(
				traceStreamerProviderAttemptError("trace_db_validate", failureCode, caveat, adoptionErr), cleanupErr,
			)
		}
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, failureCode, caveat),
			Caveats:  traceStreamerFailureCaveats(lane, caveat),
			Ran:      true, FailureStage: "trace_db_validate", FailureCode: failureCode, Cause: adoptionErr,
		}, nil
	}
	dbBytes := sealedOutputs.Size()
	companionPresent := sealedOutputs.CompanionPresent()
	sourceNames, sourceNameErr := scanTraceDBSourceNameInventory(ctx, input)
	if sourceNameErr != nil {
		closeErr := sealedOutputs.close()
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(sourceNameErr, closeErr, cleanupErr)
	}
	normalizeDisplayPath := inputPath
	if dbTarget.Retained {
		normalizeDisplayPath = dbTarget.FinalPath
	}
	normalizeStart := progressStarted(opts, "trace_db_normalize", "normalizing trace_streamer SQLite DB to systrace", normalizeDisplayPath, output)
	systraceExport, systraceErr := exportTraceDBToSystraceFromSealedWithSourceNamesAndLedger(
		ctx, sealedOutputs.main, normalizeDisplayPath, output, sourceNames, ledger)
	integrityErr := sealedOutputs.validate()
	if integrityErr != nil {
		progressFinished(opts, "trace_db_normalize", "trace_streamer SQLite DB generation changed during normalization", normalizeDisplayPath, output, normalizeStart, ProgressStatusFailed)
		closeErr := sealedOutputs.close()
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(
			fmt.Errorf("trace_streamer DB output integrity failed: %w", integrityErr), systraceErr, closeErr, cleanupErr,
		)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		closeErr := sealedOutputs.close()
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(ctxErr, systraceErr, closeErr, cleanupErr)
	}
	if sealedTraceDBNormalizationFailureIsFatal(systraceErr) {
		progressFinished(opts, "trace_db_normalize", "sealed trace_streamer SQLite authority failed", normalizeDisplayPath, output, normalizeStart, ProgressStatusFailed)
		closeErr := sealedOutputs.close()
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(
			fmt.Errorf("sealed trace_streamer DB normalization failed closed: %w", systraceErr), closeErr, cleanupErr,
		)
	}
	if dbTarget.Retained {
		if err := publishRetainedTraceDBOutputs(ctx, dbTarget, sealedOutputs, ledger); err != nil {
			closeErr := sealedOutputs.close()
			cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
			cleanup = nil
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(
				fmt.Errorf("publish retained trace_streamer DB: %w", err), closeErr, cleanupErr,
			)
		}
		dbPath = dbTarget.FinalPath
	}
	if closeErr := sealedOutputs.close(); closeErr != nil {
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(
			fmt.Errorf("close sealed trace_streamer DB outputs: %w", closeErr), cleanupErr,
		)
	}
	if dbTarget.Retained {
		if err := cleanupTraceStreamerDBTarget(cleanup); err != nil {
			return traceStreamerExportResult{}, fmt.Errorf("cleanup trace_streamer DB staging after publish: %w", err)
		}
		cleanup = nil
	}
	dbArtifact := Artifact{
		Type:      ArtifactTraceDB,
		Path:      dbPath,
		Bytes:     dbBytes,
		Converter: traceStreamerConverter,
		Caveats:   []string{"trace_streamer SQLite DB preserved as conversion provenance"},
	}
	if companionPresent {
		dbArtifact.Caveats = append(dbArtifact.Caveats, "timestamp_companion="+dbPath+".ohos.ts")
	}
	if systraceErr != nil {
		progressFinished(opts, "trace_db_normalize", "trace_streamer SQLite DB normalization failed", normalizeDisplayPath, output, normalizeStart, ProgressStatusFailed)
		if cleanup != nil {
			if cleanupErr := cleanupTraceStreamerDBTarget(cleanup); cleanupErr != nil {
				caveat := fmt.Sprintf("trace_streamer DB export succeeded, but Codrax could not normalize the DB to systrace: %v", systraceErr)
				return traceStreamerExportResult{}, traceDBJoinPreservingSingle(traceStreamerProviderAttemptError("trace_db_normalize", "trace_db_normalize_failed", caveat, systraceErr), cleanupErr)
			}
			cleanup = nil
			dbArtifact = Artifact{}
		}
		caveat := fmt.Sprintf("trace_streamer DB export succeeded, but Codrax could not normalize the DB to systrace: %v", systraceErr)
		failureDecision := traceProviderFailure(
			decision,
			"trace_db_normalize_failed",
			caveat,
		)
		if dbTarget.Retained {
			failureDecision.DBPath = dbPath
		}
		return traceStreamerExportResult{
			Artifact:      dbArtifact,
			Decision:      failureDecision,
			Coverage:      systraceExport.Coverage,
			TraceCoverage: systraceExport.TraceCoverage,
			Caveats:       traceStreamerFailureCaveats(lane, caveat),
			Ran:           true, FailureStage: "trace_db_normalize", FailureCode: "trace_db_normalize_failed", Cause: systraceErr,
		}, nil
	}
	if systraceExport.Artifact.Path == "" {
		progressFinished(opts, "trace_db_normalize", "trace_streamer SQLite DB normalization produced no systrace rows", normalizeDisplayPath, output, normalizeStart, ProgressStatusFailed)
		if cleanup != nil {
			if cleanupErr := cleanupTraceStreamerDBTarget(cleanup); cleanupErr != nil {
				caveat := "trace_streamer DB export succeeded, but no systrace-compatible rows were emitted; inspect trace_db_coverage for missing or empty tables"
				return traceStreamerExportResult{}, traceDBJoinPreservingSingle(traceStreamerProviderAttemptError("trace_db_normalize", "trace_db_no_rows", caveat, errors.New(caveat)), cleanupErr)
			}
			cleanup = nil
			dbArtifact = Artifact{}
		}
		caveat := "trace_streamer DB export succeeded, but no systrace-compatible rows were emitted; inspect trace_db_coverage for missing or empty tables"
		failureDecision := traceProviderFailure(
			decision,
			"trace_db_no_rows",
			caveat,
		)
		if dbTarget.Retained {
			failureDecision.DBPath = dbPath
		}
		return traceStreamerExportResult{
			Artifact:      dbArtifact,
			Decision:      failureDecision,
			Coverage:      systraceExport.Coverage,
			TraceCoverage: systraceExport.TraceCoverage,
			Caveats:       traceStreamerFailureCaveats(lane, caveat),
			Ran:           true, FailureStage: "trace_db_normalize", FailureCode: "trace_db_no_rows", Cause: errors.New(caveat),
		}, nil
	}
	progressFinished(opts, "trace_db_normalize", "normalized trace_streamer SQLite DB to systrace", normalizeDisplayPath, output, normalizeStart, ProgressStatusComplete)
	if cleanup != nil {
		if err := cleanupTraceStreamerDBTarget(cleanup); err != nil {
			return traceStreamerExportResult{}, err
		}
		cleanup = nil
		dbArtifact = Artifact{}
	}
	success, err := traceProviderPublished(decision, systraceExport.Artifact, ledger)
	if err != nil {
		return traceStreamerExportResult{}, err
	}
	if caveats := dedupeStrings(lane.Caveats); len(caveats) > 0 {
		success.Caveat = strings.Join(caveats, " | ")
	}
	if keepDB || strings.TrimSpace(opts.TraceDBOutputPath) != "" {
		success.DBPath = dbPath
	}
	qualityCaveats := traceDBSemanticQualityCaveats(systraceExport.Coverage)
	return traceStreamerExportResult{
		Artifact:         dbArtifact,
		SystraceArtifact: systraceExport.Artifact,
		Decision:         success,
		Coverage:         systraceExport.Coverage,
		TraceCoverage:    systraceExport.TraceCoverage,
		Caveats: dedupeStrings(append(
			append(append([]string(nil), lane.Caveats...), "trace_streamer DB export succeeded and was normalized to systrace for trace_query"),
			qualityCaveats...)),
		Ran:               true,
		EventsWritten:     systraceExport.EventsWritten,
		OutputBytes:       systraceExport.OutputBytes,
		FirstTimestampSec: systraceExport.FirstTimestampSec,
		LastTimestampSec:  systraceExport.LastTimestampSec,
	}, nil
}

func traceStreamerExportFailureError(export traceStreamerExportResult, lane traceProviderLanePlan) error {
	cause := export.Cause
	if cause == nil && strings.TrimSpace(export.Decision.Caveat) == "" {
		cause = errors.New("trace_streamer conversion failed")
	}
	return &TraceProviderFailureError{
		Decision:     export.Decision,
		Source:       lane.Source,
		Path:         lane.Path,
		Stage:        export.FailureStage,
		Code:         export.FailureCode,
		Caveats:      append([]string(nil), export.Caveats...),
		Cause:        cause,
		RolledBackDB: strings.TrimSpace(export.Decision.DBPath),
	}
}

func traceStreamerProviderAttemptError(stage, code, caveat string, cause error) error {
	message := fmt.Sprintf("trace_streamer provider failed: stage=%s code=%s caveat=%s", stage, code, strconv.Quote(caveat))
	if cause == nil {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func traceStreamerDBOutputValidationCode(err error) (code string, recoverable bool) {
	// A producer-shape error is safe to fall back from only when it is the
	// sole failure. errors.Join means adoption/validation also lost a close or
	// authority invariant; the precise shape sentinel must not mask that.
	if traceDBErrorHasJoinedFailures(err) {
		return "trace_db_unsealed", false
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "trace_db_missing", true
	case errors.Is(err, errSealedConversionFileEmpty):
		return "trace_db_empty", true
	case errors.Is(err, errSealedConversionFileNotRegular):
		return "trace_db_not_regular", true
	case errors.Is(err, errTraceStreamerDBAuxiliaryState):
		return "trace_db_auxiliary_state", true
	default:
		return "trace_db_unsealed", false
	}
}

func sealedTraceDBNormalizationFailureIsFatal(err error) bool {
	return err != nil && (errors.Is(err, errSealedTraceDBAuthority) ||
		errors.Is(err, errTraceDBSQLiteHeapBudgetExceeded) ||
		errors.Is(err, errTraceDBSQLiteBudgetAuthority) ||
		ownedTraceOutputHardFailure(err) || traceDBErrorHasJoinedFailures(err))
}

func traceDBErrorHasJoinedFailures(err error) bool {
	for current := err; current != nil; current = errors.Unwrap(current) {
		if _, joined := current.(interface{ Unwrap() []error }); joined {
			return true
		}
	}
	return false
}

func traceStreamerFailureCaveats(lane traceProviderLanePlan, primary string) []string {
	caveats := append([]string(nil), lane.Caveats...)
	caveats = append(caveats, fmt.Sprintf("trace_streamer provider resolution: source=%s path=%s available=%t",
		firstNonEmpty(strings.TrimSpace(lane.Source), "unknown"), firstNonEmpty(strings.TrimSpace(lane.Path), "none"), lane.Available))
	caveats = append(caveats, primary)
	return dedupeStrings(caveats)
}

type traceStreamerDBTarget struct {
	StagingPath          string
	FinalPath            string
	Retained             bool
	Cleanup              func() error
	stagingDir           *privateConversionDir
	outputParent         *publishedConversionFilePlatformState
	finalLeaf            string
	finalBindingPath     string
	finalAuthorityParent string
}

func (target traceStreamerDBTarget) validateStaging() error {
	if target.stagingDir == nil {
		return fmt.Errorf("trace_streamer staging directory authority is missing")
	}
	return target.stagingDir.Validate()
}

func prepareTraceStreamerDBTarget(opts Options, input, output string, keepDB bool) (traceStreamerDBTarget, error) {
	finalPath := strings.TrimSpace(opts.TraceDBOutputPath)
	if finalPath == "" && keepDB {
		finalPath = traceSidecarBase(input, output) + ".trace.db"
	}
	parent := ""
	finalLeaf := ""
	finalBindingPath := ""
	finalAuthorityParent := ""
	var outputParent *publishedConversionFilePlatformState
	if finalPath != "" {
		absoluteFinal, err := filepath.Abs(filepath.Clean(finalPath))
		if err != nil {
			return traceStreamerDBTarget{}, fmt.Errorf("resolve trace DB output path %s: %w", finalPath, err)
		}
		finalLeaf = filepath.Base(absoluteFinal)
		if finalLeaf == "" || finalLeaf == "." || finalLeaf == ".." || filepath.Base(finalLeaf) != finalLeaf {
			return traceStreamerDBTarget{}, fmt.Errorf("trace DB output file name is invalid: %s", finalPath)
		}
		if err := validatePrivateConversionDirChildNamePlatform(finalLeaf); err != nil {
			return traceStreamerDBTarget{}, fmt.Errorf("trace DB output file name is invalid: %s: %w", finalPath, err)
		}
		if err := validatePrivateConversionDirChildNamePlatform(finalLeaf + ".ohos.ts"); err != nil {
			return traceStreamerDBTarget{}, fmt.Errorf("trace DB companion output file name is invalid: %s: %w", finalPath+".ohos.ts", err)
		}
		finalBindingPath = absoluteFinal
		if _, err := os.Lstat(finalPath); err == nil {
			return traceStreamerDBTarget{}, fmt.Errorf("output file already exists: %s (delete it first or specify a different trace DB output path)", finalPath)
		} else if !os.IsNotExist(err) {
			return traceStreamerDBTarget{}, fmt.Errorf("check trace DB output path %s: %w", finalPath, err)
		}
		companionPath := finalPath + ".ohos.ts"
		if _, err := os.Lstat(companionPath); err == nil {
			return traceStreamerDBTarget{}, fmt.Errorf("trace DB companion output already exists: %s", companionPath)
		} else if !os.IsNotExist(err) {
			return traceStreamerDBTarget{}, fmt.Errorf("check trace DB companion output path %s: %w", companionPath, err)
		}
		parent = filepath.Dir(absoluteFinal)
		parentInfo, err := os.Stat(parent)
		if err != nil {
			return traceStreamerDBTarget{}, fmt.Errorf("inspect trace DB output directory %s: %w", parent, err)
		}
		if !parentInfo.IsDir() {
			return traceStreamerDBTarget{}, fmt.Errorf("trace DB output parent is not a directory: %s", parent)
		}
		finalAuthorityParent, err = filepath.EvalSymlinks(parent)
		if err != nil {
			return traceStreamerDBTarget{}, fmt.Errorf("resolve trace DB output parent %s: %w", parent, err)
		}
		outputParent, err = openPublishedConversionParentPlatform(
			finalAuthorityParent,
			sealedConversionPublicationRetainedTraceDB,
		)
		if err != nil {
			return traceStreamerDBTarget{}, err
		}
	}
	// Keep the 128-bit random leaf but minimize the fixed prefix. Official
	// Windows trace_streamer builds still encounter legacy MAX_PATH handling;
	// preserving a long customer basename inside the former 22-byte prefix
	// pushed otherwise ordinary inputs past that boundary before parsing.
	pattern := traceStreamerPrivateDirPattern
	if parent != "" {
		pattern = ".codrax-trace-db-*"
	}
	stagingRoot, err := resolveConversionRuntimeAnchor(opts.RuntimeAnchor, output)
	if err != nil {
		return traceStreamerDBTarget{}, traceDBJoinPreservingSingle(
			err,
			closePublishedConversionFilePlatform(outputParent),
		)
	}
	stagingDir, err := newRuntimePrivateConversionDir(stagingRoot, pattern)
	if err != nil {
		return traceStreamerDBTarget{}, traceDBJoinPreservingSingle(
			err,
			closePublishedConversionFilePlatform(outputParent),
		)
	}
	cleanup := func() error {
		return traceDBJoinPreservingSingle(
			stagingDir.FinalizeCleanup(),
			closePublishedConversionFilePlatform(outputParent),
		)
	}
	stagingPath, err := stagingDir.ChildPath("trace_streamer_export.db")
	if err != nil {
		return traceStreamerDBTarget{}, traceDBJoinPreservingSingle(err, cleanup())
	}
	return traceStreamerDBTarget{
		StagingPath:          stagingPath,
		FinalPath:            finalPath,
		Retained:             finalPath != "",
		Cleanup:              cleanup,
		stagingDir:           stagingDir,
		outputParent:         outputParent,
		finalLeaf:            finalLeaf,
		finalBindingPath:     finalBindingPath,
		finalAuthorityParent: finalAuthorityParent,
	}, nil
}

func cleanupTraceStreamerDBTarget(cleanup func() error) error {
	if cleanup == nil {
		return nil
	}
	return cleanup()
}

func traceStreamerExportArguments(dbPath string, soDirs []string) []string {
	afterInput := []string{"-e", dbPath}
	for _, dir := range soDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		afterInput = append(afterInput, "--So_dir", dir)
	}
	return afterInput
}

func traceStreamerInputSnapshotLeaf(inputPath string) (string, error) {
	leaf := filepath.Base(strings.TrimSpace(inputPath))
	if leaf == "" || leaf == "." || leaf == string(filepath.Separator) {
		return "", conversionInputFailure(
			ConversionInputCodeInvalidPath,
			conversionInputStageExternalTool,
			inputPath,
			fmt.Errorf("trace_streamer input has no usable basename"),
		)
	}
	for _, reserved := range []string{
		"trace_streamer_export.db",
		"trace_streamer_export.db.ohos.ts",
		"trace_streamer_export.db-journal",
		"trace_streamer_export.db-wal",
		"trace_streamer_export.db-shm",
		"ts_tmp",
		"ts_tmp.perf.data",
		"ts_tmp.jsmemory_timeline.heapsnapshot",
		"ts_tmp.jsmemory_snapshot",
	} {
		if strings.EqualFold(leaf, reserved) {
			return "", conversionInputFailure(
				ConversionInputCodeInvalidPath,
				conversionInputStageExternalTool,
				inputPath,
				fmt.Errorf("trace_streamer input basename %q conflicts with a reserved staging artifact; rename the input file", leaf),
			)
		}
	}
	return leaf, nil
}

type traceStreamerSnapshotLeafSource interface {
	traceStreamerSnapshotLeaf() string
}

func traceStreamerInputSnapshotLeafForView(input conversionInputView) (string, error) {
	if source, ok := input.(traceStreamerSnapshotLeafSource); ok {
		leaf := strings.TrimSpace(source.traceStreamerSnapshotLeaf())
		if leaf == "" || filepath.Base(leaf) != leaf {
			return "", conversionInputFailure(
				ConversionInputCodeInvalidPath,
				conversionInputStageExternalTool,
				input.DisplayPath(),
				fmt.Errorf("trace_streamer input view supplied an invalid snapshot basename"),
			)
		}
		return traceStreamerInputSnapshotLeaf(leaf)
	}
	if input == nil {
		return "", conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			"",
			fmt.Errorf("trace_streamer input authority is missing"),
		)
	}
	return traceStreamerInputSnapshotLeaf(input.DisplayPath())
}

const traceStreamerWindowsLegacyPathBudget = 240

// traceStreamerInputSnapshotLeafForPlatform preserves the customer/member
// basename unless the native Windows argv path would exceed the conservative
// legacy budget still used by official trace_streamer builds. In that one
// case the immutable private snapshot keeps the format extension but uses a
// short leaf. The public input/output identity and all provenance remain the
// original path; only the child-private transport name changes.
func traceStreamerInputSnapshotLeafForPlatform(stagingDir, leaf, goos string) (string, bool) {
	if goos != "windows" || windowsPathUTF16Units(filepath.Join(stagingDir, leaf)) <= traceStreamerWindowsLegacyPathBudget {
		return leaf, false
	}
	ext := filepath.Ext(leaf)
	if len(ext) == 0 || len(ext) > 16 {
		ext = ".trace"
	} else {
		for index := 0; index < len(ext); index++ {
			if ext[index] > 0x7f || os.IsPathSeparator(ext[index]) {
				ext = ".trace"
				break
			}
		}
	}
	return "input" + ext, true
}

func windowsPathUTF16Units(path string) int {
	return len(utf16.Encode([]rune(path)))
}

func traceStreamerEmptyChildDiagnostic(goos string, output []byte, executable, snapshot, db string, compacted bool) string {
	if goos != "windows" || strings.TrimSpace(string(output)) != "" {
		return ""
	}
	return fmt.Sprintf("; child_output=empty windows_path_units(executable=%d,input_snapshot=%d,db=%d) legacy_path_budget=%d snapshot_leaf_compacted=%t",
		windowsPathUTF16Units(executable), windowsPathUTF16Units(snapshot), windowsPathUTF16Units(db),
		traceStreamerWindowsLegacyPathBudget, compacted)
}

func boundedTraceStreamerCommandOutput(output []byte) string {
	if strings.TrimSpace(string(output)) == "" {
		return ""
	}
	// trace_streamer is an external process and can echo arbitrary fragments of
	// its private argv. Post-hoc path replacement cannot be made safe after the
	// shared command buffer truncates output, so customer-facing surfaces retain
	// only the typed exit status and this stable disclosure marker.
	return "\n[trace_streamer child output suppressed]"
}

type traceStreamerPrivateError struct {
	err     error
	message string
}

func (err *traceStreamerPrivateError) Error() string {
	if err == nil {
		return "trace_streamer conversion failed"
	}
	return err.message
}

func (err *traceStreamerPrivateError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.err
}

func redactTraceStreamerPrivateError(err error, stagingDir string) error {
	if err == nil {
		return nil
	}
	message := redactTraceStreamerPrivateText(err.Error(), stagingDir)
	if message == err.Error() {
		return err
	}
	return &traceStreamerPrivateError{err: err, message: message}
}

func redactTraceStreamerExportResult(result *traceStreamerExportResult, stagingDir string) {
	if result == nil || strings.TrimSpace(stagingDir) == "" {
		return
	}
	result.Decision.Caveat = redactTraceStreamerPrivateText(result.Decision.Caveat, stagingDir)
	for index := range result.Caveats {
		result.Caveats[index] = redactTraceStreamerPrivateText(result.Caveats[index], stagingDir)
	}
	for index := range result.Artifact.Caveats {
		result.Artifact.Caveats[index] = redactTraceStreamerPrivateText(result.Artifact.Caveats[index], stagingDir)
	}
	for index := range result.SystraceArtifact.Caveats {
		result.SystraceArtifact.Caveats[index] = redactTraceStreamerPrivateText(result.SystraceArtifact.Caveats[index], stagingDir)
	}
	for index := range result.Coverage {
		result.Coverage[index].Error = redactTraceStreamerPrivateText(result.Coverage[index].Error, stagingDir)
	}
	for index := range result.TraceCoverage {
		result.TraceCoverage[index].Error = redactTraceStreamerPrivateText(result.TraceCoverage[index].Error, stagingDir)
	}
	result.Cause = redactTraceStreamerPrivateError(result.Cause, stagingDir)
}

func redactTraceStreamerPrivateText(message, stagingDir string) string {
	for _, privatePrefix := range privatePathRedactionPrefixes(stagingDir) {
		message = replaceAllASCIIPathFold(message, privatePrefix, "<private_trace_staging>")
	}
	return message
}
