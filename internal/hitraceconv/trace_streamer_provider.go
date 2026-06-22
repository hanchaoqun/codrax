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
	standaloneArtifacts, standaloneCaveats, standaloneDecisions, err := extractStandaloneArtifacts(ctx, opts, inputBytes, output)
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
		Caveats:            caveats,
		EventsWritten:      export.EventsWritten,
		FirstTimestampSec:  export.FirstTimestampSec,
		LastTimestampSec:   export.LastTimestampSec,
		MissingFormatCount: 0,
		UnknownEventCount:  0,
	}
	if bundleArtifact, err := writeTraceBundleWithCoverage(input, result.OutputPath, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage); err != nil {
		return Result{}, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	return result, nil
}

func maybeRunTraceStreamerAuto(ctx context.Context, opts Options, input, output string) (traceStreamerExportResult, error) {
	if normalizeTraceEngineMode(opts.TraceEngine) != traceEngineAuto && normalizeTraceEngineMode(opts.TraceEngine) != "" {
		return traceStreamerExportResult{}, nil
	}
	status, err := BuildTraceToolStatus(opts)
	if err != nil {
		return traceStreamerExportResult{}, err
	}
	if !status.TraceStreamer.Available {
		decision := traceProviderSkipped(
			newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameTraceStreamer), opts, input, output),
			false,
			"trace_streamer_unavailable",
			"trace_streamer was not discovered; auto conversion uses built-in fallback",
		)
		return traceStreamerExportResult{Decision: decision}, nil
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
	combined, runErr := cmd.CombinedOutput()
	if runErr != nil {
		if cleanup != nil {
			cleanup()
		}
		caveat := fmt.Sprintf("trace_streamer DB export failed (%s)%s", runErr, boundedCommandOutput(combined))
		return traceStreamerExportResult{
			Decision: traceProviderFailure(decision, "trace_streamer_failed", caveat),
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
	systraceExport, systraceErr := exportTraceDBToSystrace(ctx, dbPath, output)
	if systraceErr != nil {
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
			Coverage: systraceExport.Coverage,
			Caveats:  []string{caveat},
			Cleanup:  cleanup,
			Ran:      true,
		}, nil
	}
	if systraceExport.Artifact.Path == "" {
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
			Coverage: systraceExport.Coverage,
			Caveats:  []string{caveat},
			Cleanup:  cleanup,
			Ran:      true,
		}, nil
	}
	success := traceProviderSuccess(decision, systraceExport.Artifact)
	if keepDB || strings.TrimSpace(opts.TraceDBOutputPath) != "" {
		success.DBPath = dbPath
	}
	return traceStreamerExportResult{
		Artifact:          dbArtifact,
		SystraceArtifact:  systraceExport.Artifact,
		Decision:          success,
		Coverage:          systraceExport.Coverage,
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
