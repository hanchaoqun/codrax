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
	Artifact Artifact
	Decision TraceProviderDecision
	Caveats  []string
	Cleanup  func()
	Ran      bool
}

func convertTraceStreamerOnly(ctx context.Context, opts Options, input string, inputBytes int64, output string) (Result, error) {
	export, err := runTraceStreamerExport(ctx, opts, input, output, true)
	if err != nil {
		return Result{}, err
	}
	if export.Artifact.Path == "" {
		if export.Decision.Caveat != "" {
			return Result{}, fmt.Errorf("%s", export.Decision.Caveat)
		}
		return Result{}, fmt.Errorf("trace_streamer did not produce a trace DB artifact")
	}
	result := Result{
		InputPath:         input,
		InputBytes:        inputBytes,
		Artifacts:         []Artifact{export.Artifact},
		TraceDecisions:    []TraceProviderDecision{export.Decision},
		Caveats:           append([]string(nil), export.Caveats...),
		EventsWritten:     0,
		OutputPath:        "",
		OutputBytes:       0,
		FirstTimestampSec: 0,
		LastTimestampSec:  0,
	}
	if bundleArtifact, err := writeTraceBundle(input, output, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions); err != nil {
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
	artifact := Artifact{
		Type:      ArtifactTraceDB,
		Path:      dbPath,
		Bytes:     info.Size(),
		Converter: traceStreamerConverter,
		Caveats:   []string{"trace_streamer DB exported; systrace/perftrace DB exporters are delivered by later batches"},
	}
	success := traceProviderSuccess(decision, artifact)
	success.DBPath = dbPath
	return traceStreamerExportResult{
		Artifact: artifact,
		Decision: success,
		Caveats:  []string{"trace_streamer DB export succeeded; systrace/perftrace DB exporters are delivered by later batches"},
		Cleanup:  cleanup,
		Ran:      true,
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
