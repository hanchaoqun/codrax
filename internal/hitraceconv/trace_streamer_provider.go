package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const traceStreamerConverter = converterVersion + "+trace-streamer-db"

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
	export, err := runTraceStreamerExport(ctx, opts, plan.TraceStreamer, inputPath, output, opts.KeepTraceDB, ledger)
	if err != nil {
		return Result{}, err
	}
	if retainDB && (export.Artifact.Path == "" || !artifactPathExists(export.Artifact.Path)) {
		if export.Decision.Caveat != "" {
			return Result{}, traceStreamerExportFailureError(export)
		}
		return Result{}, fmt.Errorf("trace_streamer did not produce a trace DB artifact")
	}
	if export.SystraceArtifact.Path == "" && !retainDB {
		if export.Decision.Caveat != "" {
			return Result{}, traceStreamerExportFailureError(export)
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
	normalizeResultCollections(&result)
	if bundleArtifact, err := writeTraceBundleWithAllCoverageAndLedger(inputPath, result.OutputPath, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage, result.TraceCoverage, ledger); err != nil {
		return Result{}, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	normalizeResultCollections(&result)
	return result, nil
}

func maybeRunTraceStreamerAuto(ctx context.Context, opts Options, plan traceProviderPlan, input string, inputBytes int64, output string, hasTracePerfSidecar bool, ledger *conversionFileLedger) (traceStreamerExportResult, error) {
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
			newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameTraceStreamer), opts, input, output),
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

func runTraceStreamerExport(ctx context.Context, opts Options, lane traceProviderLanePlan, input, output string, keepDB bool, ledger *conversionFileLedger) (traceStreamerExportResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	decision := newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameTraceStreamer), opts, input, output)
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
	dbTarget, err := prepareTraceStreamerDBTarget(opts, input, output, keepDB)
	if err != nil {
		return traceStreamerExportResult{}, fmt.Errorf("prepare trace_streamer DB staging path: %w", err)
	}
	cleanup := dbTarget.Cleanup
	dbPath := dbTarget.StagingPath
	cmd := buildTraceStreamerExportCommand(ctx, lane.Path, input, dbPath, opts.TraceStreamerSoDirs)
	combined, runErr := runCommandWithProgress(opts, cmd, "trace_streamer_export", "running trace_streamer SQLite DB export")
	if runErr != nil {
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		if ctxErr := ctx.Err(); ctxErr != nil {
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(ctxErr, cleanupErr)
		}
		caveat := fmt.Sprintf("trace_streamer DB export failed (%s)%s", runErr, boundedCommandOutput(combined))
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
	info, statErr := os.Lstat(dbPath)
	if statErr != nil {
		if cleanupErr := cleanupTraceStreamerDBTarget(cleanup); cleanupErr != nil {
			caveat := fmt.Sprintf("trace_streamer DB export completed but output DB is not readable: %v", statErr)
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(traceStreamerProviderAttemptError("trace_db_validate", "trace_db_missing", caveat, statErr), cleanupErr)
		}
		cleanup = nil
		caveat := fmt.Sprintf("trace_streamer DB export completed but output DB is not readable: %v", statErr)
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_db_missing", caveat),
			Caveats:  traceStreamerFailureCaveats(lane, caveat),
			Ran:      true, FailureStage: "trace_db_validate", FailureCode: "trace_db_missing", Cause: statErr,
		}, nil
	}
	if !info.Mode().IsRegular() {
		caveat := fmt.Sprintf("trace_streamer DB export completed but staging output is not a regular file: mode=%s", info.Mode())
		if cleanupErr := cleanupTraceStreamerDBTarget(cleanup); cleanupErr != nil {
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(traceStreamerProviderAttemptError("trace_db_validate", "trace_db_not_regular", caveat, errors.New(caveat)), cleanupErr)
		}
		cleanup = nil
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_db_not_regular", caveat),
			Caveats:  traceStreamerFailureCaveats(lane, caveat),
			Ran:      true, FailureStage: "trace_db_validate", FailureCode: "trace_db_not_regular", Cause: errors.New(caveat),
		}, nil
	}
	if info.Size() == 0 {
		caveat := "trace_streamer DB export completed but output DB is empty"
		if cleanupErr := cleanupTraceStreamerDBTarget(cleanup); cleanupErr != nil {
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(traceStreamerProviderAttemptError("trace_db_validate", "trace_db_empty", caveat, errors.New(caveat)), cleanupErr)
		}
		cleanup = nil
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_db_empty", caveat),
			Caveats:  traceStreamerFailureCaveats(lane, caveat),
			Ran:      true, FailureStage: "trace_db_validate", FailureCode: "trace_db_empty", Cause: errors.New(caveat),
		}, nil
	}
	if dbTarget.Retained {
		if err := publishStagedTraceDB(dbTarget, info, ledger); err != nil {
			return traceStreamerExportResult{}, traceDBJoinPreservingSingle(fmt.Errorf("publish retained trace_streamer DB: %w", err), cleanupTraceStreamerDBTarget(cleanup))
		}
		dbPath = dbTarget.FinalPath
		if err := cleanupTraceStreamerDBTarget(cleanup); err != nil {
			return traceStreamerExportResult{}, fmt.Errorf("cleanup trace_streamer DB staging after publish: %w", err)
		}
		cleanup = nil
	}
	dbArtifact := Artifact{
		Type:      ArtifactTraceDB,
		Path:      dbPath,
		Bytes:     info.Size(),
		Converter: traceStreamerConverter,
		Caveats:   []string{"trace_streamer SQLite DB preserved as conversion provenance"},
	}
	if companion := dbPath + ".ohos.ts"; artifactPathExists(companion) {
		dbArtifact.Caveats = append(dbArtifact.Caveats, "timestamp_companion="+companion)
	}
	normalizeStart := progressStarted(opts, "trace_db_normalize", "normalizing trace_streamer SQLite DB to systrace", dbPath, output)
	systraceExport, systraceErr := exportTraceDBToSystraceWithLedger(ctx, dbPath, output, ledger)
	if ctxErr := ctx.Err(); ctxErr != nil {
		cleanupErr := cleanupTraceStreamerDBTarget(cleanup)
		cleanup = nil
		return traceStreamerExportResult{}, traceDBJoinPreservingSingle(ctxErr, cleanupErr)
	}
	if systraceErr != nil {
		progressFinished(opts, "trace_db_normalize", "trace_streamer SQLite DB normalization failed", dbPath, output, normalizeStart, ProgressStatusFailed)
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
		progressFinished(opts, "trace_db_normalize", "trace_streamer SQLite DB normalization produced no systrace rows", dbPath, output, normalizeStart, ProgressStatusFailed)
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
	progressFinished(opts, "trace_db_normalize", "normalized trace_streamer SQLite DB to systrace", dbPath, output, normalizeStart, ProgressStatusComplete)
	if cleanup != nil {
		if err := cleanupTraceStreamerDBTarget(cleanup); err != nil {
			return traceStreamerExportResult{}, err
		}
		cleanup = nil
		dbArtifact = Artifact{}
	}
	success := traceProviderSuccess(decision, systraceExport.Artifact)
	if caveats := dedupeStrings(lane.Caveats); len(caveats) > 0 {
		success.Caveat = strings.Join(caveats, " | ")
	}
	if keepDB || strings.TrimSpace(opts.TraceDBOutputPath) != "" {
		success.DBPath = dbPath
	}
	return traceStreamerExportResult{
		Artifact:          dbArtifact,
		SystraceArtifact:  systraceExport.Artifact,
		Decision:          success,
		Coverage:          systraceExport.Coverage,
		TraceCoverage:     systraceExport.TraceCoverage,
		Caveats:           dedupeStrings(append(append([]string(nil), lane.Caveats...), "trace_streamer DB export succeeded and was normalized to systrace for trace_query")),
		Ran:               true,
		EventsWritten:     systraceExport.EventsWritten,
		OutputBytes:       systraceExport.OutputBytes,
		FirstTimestampSec: systraceExport.FirstTimestampSec,
		LastTimestampSec:  systraceExport.LastTimestampSec,
	}, nil
}

func traceStreamerExportFailureError(export traceStreamerExportResult) error {
	caveat := strings.TrimSpace(export.Decision.Caveat)
	var failure error
	if export.Cause == nil {
		failure = errors.New(firstNonEmpty(caveat, "trace_streamer conversion failed"))
	} else if caveat == "" || caveat == export.Cause.Error() {
		failure = export.Cause
	} else {
		failure = fmt.Errorf("%s: %w", caveat, export.Cause)
	}
	if rolledBackDB := strings.TrimSpace(export.Decision.DBPath); rolledBackDB != "" {
		return fmt.Errorf("rolled_back_db=%q: %w", rolledBackDB, failure)
	}
	return failure
}

func traceStreamerProviderAttemptError(stage, code, caveat string, cause error) error {
	message := fmt.Sprintf("trace_streamer provider failed: stage=%s code=%s caveat=%s", stage, code, strconv.Quote(caveat))
	if cause == nil {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func traceStreamerFailureCaveats(lane traceProviderLanePlan, primary string) []string {
	caveats := append([]string(nil), lane.Caveats...)
	caveats = append(caveats, fmt.Sprintf("trace_streamer provider resolution: source=%s path=%s available=%t",
		firstNonEmpty(strings.TrimSpace(lane.Source), "unknown"), firstNonEmpty(strings.TrimSpace(lane.Path), "none"), lane.Available))
	caveats = append(caveats, primary)
	return dedupeStrings(caveats)
}

type traceStreamerDBTarget struct {
	StagingPath string
	FinalPath   string
	Retained    bool
	Cleanup     func() error
}

func prepareTraceStreamerDBTarget(opts Options, input, output string, keepDB bool) (traceStreamerDBTarget, error) {
	finalPath := strings.TrimSpace(opts.TraceDBOutputPath)
	if finalPath == "" && keepDB {
		finalPath = traceSidecarBase(input, output) + ".trace.db"
	}
	parent := ""
	if finalPath != "" {
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
		parent = filepath.Dir(finalPath)
		parentInfo, err := os.Stat(parent)
		if err != nil {
			return traceStreamerDBTarget{}, fmt.Errorf("inspect trace DB output directory %s: %w", parent, err)
		}
		if !parentInfo.IsDir() {
			return traceStreamerDBTarget{}, fmt.Errorf("trace DB output parent is not a directory: %s", parent)
		}
	}
	var (
		stagingDir string
		err        error
	)
	if parent != "" {
		stagingDir, err = os.MkdirTemp(parent, ".codrax-trace-db-*")
	} else {
		stagingDir, err = os.MkdirTemp("", "codrax-trace-streamer-*")
	}
	if err != nil {
		return traceStreamerDBTarget{}, err
	}
	stagingInfo, err := os.Lstat(stagingDir)
	if err != nil {
		return traceStreamerDBTarget{}, err
	}
	if !stagingInfo.IsDir() || stagingInfo.Mode().Perm() != 0o700 {
		primary := fmt.Errorf("trace DB staging path is not a private directory: %s mode=%s", stagingDir, stagingInfo.Mode())
		if stagingInfo.IsDir() {
			return traceStreamerDBTarget{}, traceDBJoinPreservingSingle(primary, removeOwnedConversionDir(stagingDir, stagingInfo))
		}
		return traceStreamerDBTarget{}, primary
	}
	cleanup := func() error { return removeOwnedConversionDir(stagingDir, stagingInfo) }
	return traceStreamerDBTarget{
		StagingPath: filepath.Join(stagingDir, "trace_streamer_export.db"),
		FinalPath:   finalPath,
		Retained:    finalPath != "",
		Cleanup:     cleanup,
	}, nil
}

func cleanupTraceStreamerDBTarget(cleanup func() error) error {
	if cleanup == nil {
		return nil
	}
	return cleanup()
}

func publishStagedTraceDB(target traceStreamerDBTarget, stagingInfo os.FileInfo, ledger *conversionFileLedger) error {
	if !target.Retained || strings.TrimSpace(target.FinalPath) == "" {
		return nil
	}
	if stagingInfo == nil || !stagingInfo.Mode().IsRegular() || stagingInfo.Size() <= 0 {
		return fmt.Errorf("trace DB staging file is not a non-empty regular file")
	}
	companionStaging := target.StagingPath + ".ohos.ts"
	var companionInfo os.FileInfo
	if info, err := os.Lstat(companionStaging); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("trace DB timestamp companion is not a regular file: %s", companionStaging)
		}
		companionInfo = info
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect trace DB timestamp companion %s: %w", companionStaging, err)
	}
	// Publish the optional timestamp companion first and the DB last. The DB is
	// the public commit marker: observers can never see a newly published DB
	// while its conversion-owned companion is still absent. Every publication
	// uses an atomic no-replace primitive, so a racing external owner is never
	// overwritten.
	if companionInfo != nil {
		if err := publishStagedTraceRegularFile(companionStaging, target.FinalPath+".ohos.ts", companionInfo, ledger); err != nil {
			return fmt.Errorf("publish trace DB timestamp companion: %w", err)
		}
	}
	if err := publishStagedTraceRegularFile(target.StagingPath, target.FinalPath, stagingInfo, ledger); err != nil {
		if companionInfo != nil {
			return traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(target.FinalPath+".ohos.ts"))
		}
		return err
	}
	return nil
}

func publishStagedTraceRegularFile(stagingPath, finalPath string, stagingInfo os.FileInfo, ledger *conversionFileLedger) error {
	stagingMoved, err := publishConversionFileNoReplace(stagingPath, finalPath)
	if err != nil {
		return err
	}
	if err := ledger.recordIdentity(finalPath, stagingInfo); err != nil {
		return traceDBJoinPreservingSingle(err, removeOwnedConversionPath(finalPath, stagingInfo))
	}
	finalInfo, err := os.Lstat(finalPath)
	if err != nil {
		return err
	}
	if !finalInfo.Mode().IsRegular() || !os.SameFile(stagingInfo, finalInfo) || finalInfo.Size() != stagingInfo.Size() {
		return fmt.Errorf("published trace DB failed identity/regular-file validation: %s", finalPath)
	}
	if err := ledger.sealOwnedPath(finalPath, finalInfo.Size()); err != nil {
		return err
	}
	if !stagingMoved {
		if err := removeOwnedConversionPath(stagingPath, stagingInfo); err != nil {
			return err
		}
	}
	return nil
}

func buildTraceStreamerExportCommand(ctx context.Context, traceStreamer, input, dbPath string, soDirs []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, traceStreamer, input, "-e", dbPath)
	for _, dir := range soDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		cmd.Args = append(cmd.Args, "--So_dir", dir)
	}
	return cmd
}
