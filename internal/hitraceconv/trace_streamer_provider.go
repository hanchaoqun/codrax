package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	Cleanup           func()
	Ran               bool
	EventsWritten     int
	OutputBytes       int64
	FirstTimestampSec float64
	LastTimestampSec  float64
}

func convertTraceStreamerOnly(ctx context.Context, opts Options, input string, inputBytes int64, output string) (Result, error) {
	if err := ensureOutputDoesNotExist(output); err != nil {
		return Result{}, err
	}
	export, err := runTraceStreamerExport(ctx, opts, input, output, true)
	if err != nil {
		return Result{}, err
	}
	standaloneExtractOpts := standaloneExtractOptions{GeneratePerfTrace: true}
	if export.SystraceArtifact.Path != "" && traceDBCoverageHasPerfSamples(export.Coverage) {
		standaloneExtractOpts.GeneratePerfTrace = false
		standaloneExtractOpts.PrimaryPerfSource = "trace_streamer DB perf_sample rows in systrace"
	}
	standaloneArtifacts, standaloneCaveats, standaloneDecisions, err := extractStandaloneArtifactsWithOptions(ctx, opts, inputBytes, output, standaloneExtractOpts)
	if err != nil {
		if export.SystraceArtifact.Path != "" {
			_ = os.Remove(export.SystraceArtifact.Path)
		}
		return Result{}, err
	}
	artifacts := append([]Artifact(nil), standaloneArtifacts...)
	if export.SystraceArtifact.Path != "" {
		artifacts = append([]Artifact{export.SystraceArtifact}, artifacts...)
	}
	if export.Artifact.Path == "" {
		if export.Decision.Caveat != "" {
			return Result{}, fmt.Errorf("%s", export.Decision.Caveat)
		}
		return Result{}, fmt.Errorf("trace_streamer did not produce a trace DB artifact")
	}
	artifacts = append(artifacts, export.Artifact)
	caveats := append([]string(nil), export.Caveats...)
	caveats = append(caveats, standaloneCaveats...)
	result := Result{
		InputPath:          input,
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
	if bundleArtifact, err := writeTraceBundleWithAllCoverage(input, result.OutputPath, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage, result.TraceCoverage); err != nil {
		return Result{}, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	normalizeResultCollections(&result)
	return result, nil
}

func maybeRunTraceStreamerAuto(ctx context.Context, opts Options, input string, inputBytes int64, output string) (traceStreamerExportResult, error) {
	if selectedTraceEngineMode(opts.TraceEngine) != traceEngineTraceStreamer {
		return traceStreamerExportResult{}, nil
	}
	status, err := BuildTraceToolStatus(opts)
	if err != nil {
		return traceStreamerExportResult{}, err
	}
	hasTracePerfSidecar, err := inputContainsStandalonePerfSidecar(ctx, input, inputBytes)
	if err != nil {
		return traceStreamerExportResult{}, err
	}
	if !status.TraceStreamer.Available {
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
		return traceStreamerExportResult{Decision: decision, Caveats: []string{caveat}}, nil
	}
	return runTraceStreamerExport(ctx, opts, input, output, opts.KeepTraceDB)
}

func runTraceStreamerExport(ctx context.Context, opts Options, input, output string, keepDB bool) (traceStreamerExportResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	status, err := BuildTraceToolStatus(opts)
	if err != nil {
		return traceStreamerExportResult{}, err
	}
	decision := newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameTraceStreamer), opts, input, output)
	decision.Selected = true
	decision.Attempted = true
	if !status.TraceStreamer.Available || strings.TrimSpace(status.TraceStreamer.Path) == "" {
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_streamer_unavailable", "trace_streamer engine was selected but trace_streamer is not available"),
			Ran:      false,
		}, fmt.Errorf("trace_streamer is not available; run codrax trace convert --trace-tools-status or pass --trace-streamer /path/to/trace_streamer")
	}
	dbPath, cleanup, err := traceStreamerDBPath(opts, input, output, keepDB)
	if err != nil {
		return traceStreamerExportResult{}, err
	}
	if err := ensureOutputDoesNotExist(dbPath); err != nil {
		if cleanup != nil {
			cleanup()
		}
		return traceStreamerExportResult{}, err
	}
	cmd := buildTraceStreamerExportCommand(ctx, status.TraceStreamer.Path, input, dbPath, opts.TraceStreamerSoDirs)
	combined, runErr := runCommandWithProgress(opts, cmd, "trace_streamer_export", "running trace_streamer SQLite DB export")
	if runErr != nil {
		if cleanup != nil {
			cleanup()
		}
		caveat := fmt.Sprintf("trace_streamer DB export failed (%s)%s", runErr, boundedCommandOutput(combined))
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_streamer_failed", caveat),
			Caveats:  []string{caveat},
			Ran:      true,
		}, nil
	}
	info, statErr := os.Stat(dbPath)
	if statErr != nil {
		if cleanup != nil {
			cleanup()
		}
		caveat := fmt.Sprintf("trace_streamer DB export completed but output DB is not readable: %v", statErr)
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_db_missing", caveat),
			Caveats:  []string{caveat},
			Ran:      true,
		}, nil
	}
	if info.Size() == 0 {
		if cleanup != nil {
			cleanup()
		}
		caveat := "trace_streamer DB export completed but output DB is empty"
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_db_empty", caveat),
			Caveats:  []string{caveat},
			Ran:      true,
		}, nil
	}
	dbArtifact := Artifact{
		Type:      ArtifactTraceDB,
		Path:      dbPath,
		Bytes:     info.Size(),
		Converter: traceStreamerConverter,
		Caveats:   []string{"trace_streamer SQLite DB preserved as conversion provenance"},
	}
	normalizeStart := progressStarted(opts, "trace_db_normalize", "normalizing trace_streamer SQLite DB to systrace", dbPath, output)
	systraceExport, systraceErr := exportTraceDBToSystrace(ctx, dbPath, output)
	if systraceErr != nil {
		progressFinished(opts, "trace_db_normalize", "trace_streamer SQLite DB normalization failed", dbPath, output, normalizeStart, ProgressStatusFailed)
		if cleanup != nil && !keepDB {
			cleanup()
			cleanup = nil
		}
		caveat := fmt.Sprintf("trace_streamer DB export succeeded, but Codrax could not normalize the DB to systrace: %v", systraceErr)
		return traceStreamerExportResult{
			Artifact: dbArtifact,
			Decision: traceProviderFailure(
				decision,
				"trace_db_normalize_failed",
				caveat,
			),
			Coverage:      systraceExport.Coverage,
			TraceCoverage: systraceExport.TraceCoverage,
			Caveats:       []string{caveat},
			Cleanup:       cleanup,
			Ran:           true,
		}, nil
	}
	if systraceExport.Artifact.Path == "" {
		progressFinished(opts, "trace_db_normalize", "trace_streamer SQLite DB normalization produced no systrace rows", dbPath, output, normalizeStart, ProgressStatusFailed)
		if cleanup != nil && !keepDB {
			cleanup()
			cleanup = nil
		}
		caveat := "trace_streamer DB export succeeded, but no systrace-compatible rows were emitted; inspect trace_db_coverage for missing or empty tables"
		return traceStreamerExportResult{
			Artifact: dbArtifact,
			Decision: traceProviderFailure(
				decision,
				"trace_db_no_rows",
				caveat,
			),
			Coverage:      systraceExport.Coverage,
			TraceCoverage: systraceExport.TraceCoverage,
			Caveats:       []string{caveat},
			Cleanup:       cleanup,
			Ran:           true,
		}, nil
	}
	progressFinished(opts, "trace_db_normalize", "normalized trace_streamer SQLite DB to systrace", dbPath, output, normalizeStart, ProgressStatusComplete)
	success := traceProviderSuccess(decision, systraceExport.Artifact)
	if keepDB || strings.TrimSpace(opts.TraceDBOutputPath) != "" {
		success.DBPath = dbPath
	}
	return traceStreamerExportResult{
		Artifact:          dbArtifact,
		SystraceArtifact:  systraceExport.Artifact,
		Decision:          success,
		Coverage:          systraceExport.Coverage,
		TraceCoverage:     systraceExport.TraceCoverage,
		Caveats:           []string{"trace_streamer DB export succeeded and was normalized to systrace for trace_query"},
		Cleanup:           cleanup,
		Ran:               true,
		EventsWritten:     systraceExport.EventsWritten,
		OutputBytes:       systraceExport.OutputBytes,
		FirstTimestampSec: systraceExport.FirstTimestampSec,
		LastTimestampSec:  systraceExport.LastTimestampSec,
	}, nil
}

func traceStreamerDBPath(opts Options, input, output string, keepDB bool) (string, func(), error) {
	if path := strings.TrimSpace(opts.TraceDBOutputPath); path != "" {
		return path, nil, nil
	}
	if keepDB {
		return traceSidecarBase(input, output) + ".trace.db", nil, nil
	}
	dir, err := os.MkdirTemp("", "codrax-trace-streamer-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}
	return filepath.Join(dir, "trace_streamer_export.db"), cleanup, nil
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
