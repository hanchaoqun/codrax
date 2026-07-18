package hitraceconv

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

const simpleperfAdapterVersion = converterVersion + "+simpleperf-report-sample"

const simpleperfInputSnapshotLeaf = "simpleperf_input.perf.data"

var (
	simpleperfSampleHeaderRE = regexp.MustCompile(`^(.+)\t([0-9]+)/([0-9]+)\s+\[([0-9]+)\]\s+([0-9]+)\.([0-9]{6}):\s+([0-9]+)\s+(.+):$`)
	simpleperfSymbolLineRE   = regexp.MustCompile(`^\t[ \t]*([0-9a-fA-F]+)[ \t]+(.+)[ \t]+\((.*)\)[ \t]*$`)
)

type simpleperfSample struct {
	Comm        string
	PID         int64
	TID         int64
	CPU         int64
	TimestampNS uint64
	Period      uint64
	Event       string
	SampleKind  tracewire.PerfSampleKind
	Leaf        simpleperfFrame
	CallFrames  []simpleperfFrame
}

type simpleperfFrame struct {
	IP     string
	Symbol string
	DSO    string
}

type simpleperfReportError struct {
	Line   int
	Field  string
	Reason string
}

func (err *simpleperfReportError) Error() string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("simpleperf report rejected at line %d: field=%s reason=%s", err.Line, err.Field, err.Reason)
}

func maybeConvertDirectSimpleperfPerfData(ctx context.Context, opts Options, plan traceProviderPlan, input directPerfInputBinding, outputPath string, ledger *conversionFileLedger) (Result, bool, error) {
	if !plan.DirectPerf || plan.PreflightEngine != traceEngineDirectPerf {
		return Result{}, false, nil
	}
	if err := input.validate(); err != nil {
		return Result{}, true, err
	}
	if !simpleperfDirectRequested(input.inputFormat) {
		return Result{}, false, nil
	}
	base := traceSidecarBase(input.displayPath, outputPath)
	perfTracePath := base + ".perftrace"
	perfTrace, caveat, decisions, err := maybeConvertSimpleperfPerfData(ctx, opts, input, perfTracePath, perfProviderStageDirectInput, ledger)
	if err != nil {
		return Result{}, true, err
	}
	result := Result{
		InputPath:   input.displayPath,
		InputBytes:  input.inputSize,
		OutputPath:  "",
		OutputBytes: 0,
		Artifacts: []Artifact{{
			Type:      ArtifactPerfData,
			Path:      input.displayPath,
			Bytes:     input.inputSize,
			Converter: "external",
			Perf:      perfCapabilityForRawPerfDataArtifact(input.inputFormat),
			Caveats:   []string{"input perf.data preserved; normalized .perftrace readiness is bound to its validation receipt"},
		}},
		ProviderDecisions: decisions,
		TraceDecisions: []TraceProviderDecision{
			traceProviderSkipped(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameDirectPerf), opts, input.displayPath, ""),
				false,
				"direct_perf_input",
				"trace provider route is not applicable because the input is a typed standalone perf capture with no trace body",
			),
		},
	}
	if perfTrace.Path != "" {
		result.Artifacts = append(result.Artifacts, perfTrace)
		if perfTrace.Perf != nil && perfTrace.Perf.TraceQueryReady {
			result.Artifacts[0].Caveats = append(result.Artifacts[0].Caveats, "query-ready normalized .perftrace is the trace_query CPU-sample artifact")
		} else {
			result.Artifacts[0].Caveats = append(result.Artifacts[0].Caveats, "normalized .perftrace is capture-quality inventory only; no queryable CPU samples were accepted")
		}
	} else if caveat != "" {
		result.Caveats = append(result.Caveats, caveat)
		result.Artifacts[0].Caveats = append(result.Artifacts[0].Caveats, "official simpleperf adapter did not produce .perftrace")
	}
	if err := finalizeResultTraceBundleWithLedger(ctx, input.displayPath, "", &result, ledger); err != nil {
		return Result{}, true, err
	}
	return result, true, nil
}

func simpleperfDirectRequested(inputFormat perfInputFormat) bool {
	if inputFormat == perfInputLinuxPerfData || inputFormat == perfInputSimpleperfReportProto {
		return true
	}
	return false
}

func maybeConvertSimpleperfPerfData(ctx context.Context, opts Options, input directPerfInputBinding, perfTracePath string, stage string, ledger *conversionFileLedger) (artifact Artifact, caveat string, decisions []PerfProviderDecision, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := input.validate(); err != nil {
		return Artifact{}, "", nil, err
	}
	perfPath := input.displayPath
	inputFormat := input.inputFormat
	if opts.DisablePerfAdapter {
		caveat := "perf.data preserved; perftrace generation disabled, so .perftrace was not generated"
		decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNamePerftraceDisabled), opts, perfPath, inputFormat, perfTracePath)
		decision = perfProviderSkipped(decision, true, "perftrace_generation_disabled", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	if inputFormat == perfInputSimpleperfReportProto && !rawPerfParserRequired(opts) {
		return maybeConvertSimpleperfProtoFromInputWithDecision(ctx, opts, input, perfTracePath, stage, ledger)
	}
	if rawPerfParserRequired(opts) {
		if inputFormat != perfInputLinuxPerfData {
			caveat := fmt.Sprintf("%s preserved; Codrax raw fallback supports %s only, so .perftrace was not generated", firstNonEmpty(string(inputFormat), "input"), perfInputLinuxPerfData)
			decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameRawFallback), opts, perfPath, inputFormat, perfTracePath)
			decision = perfProviderSkipped(decision, true, "unsupported_input_format", caveat)
			return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
		}
		return maybeConvertRawPerfDataFromInputWithDecision(ctx, opts, input, perfTracePath, "", stage, false, ledger)
	}
	officialDecision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameSimpleperfText), opts, perfPath, inputFormat, perfTracePath)
	resolution := resolveSimpleperfProviderTool(opts)
	tool, python, source := resolution.Tool, resolution.Python, resolution.Source
	if tool == "" {
		caveat := "perf data preserved; no official simpleperf report_sample.py adapter was configured or found"
		officialDecision = perfProviderSkipped(officialDecision, true, "official_tool_unavailable", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackForSimpleperf(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	if err := ensureOutputDoesNotExist(perfTracePath); err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	profile, err := prepareSimpleperfTextProfile(ctx, opts, input, resolution)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	reportDir, err := newPrivateConversionDir(filepath.Dir(perfTracePath), "."+filepath.Base(perfTracePath)+".*.simpleperf")
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	privateStagingPath := reportDir.Path()
	privateStagingIdentity := capturePrivatePathIdentity(privateStagingPath)
	defer func() {
		redactPerfProviderPrivateOutputs(&artifact, &caveat, &decisions, &err, privateStagingIdentity)
	}()
	defer func() {
		err = traceDBJoinPreservingSingle(err, reportDir.FinalizeCleanup())
	}()
	reportPath, err := reportDir.ChildPath("report_sample.txt")
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}

	afterInput := []string{"-o", reportPath}
	if profile.mode == simpleperfTextProfileTraceOffCPU {
		afterInput = append(afterInput, "--trace-offcpu", "on-off-cpu")
	}
	if symfs := strings.TrimSpace(opts.SimpleperfSymfsDir); symfs != "" {
		afterInput = append(afterInput, "--symfs", symfs)
	}
	if kallsyms := strings.TrimSpace(opts.SimpleperfKallsymsPath); kallsyms != "" {
		afterInput = append(afterInput, "--kallsyms", kallsyms)
	}
	cmdName := tool
	beforeInput := []string{"-i"}
	if python != "" {
		cmdName = python
		beforeInput = []string{tool, "-i"}
	}
	if err := reportDir.Validate(); err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	inputLease, err := newExternalToolInputLeaseWithPublicProgress(
		ctx,
		opts,
		input.input,
		reportDir,
		simpleperfInputSnapshotLeaf,
		resolution.ExternalInputProfile,
		"simpleperf_input_snapshot",
		"simpleperf",
		perfPath,
		perfTracePath,
	)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	cmd, err := inputLease.Command(ctx, cmdName, beforeInput, afterInput)
	if err != nil {
		boundaryErr := finishExternalToolCommand(ctx, inputLease, reportDir, nil)
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, traceDBJoinPreservingSingle(err, boundaryErr)
	}
	output, runErr, commandStart, commandStarted := runCommandWithProgressUntilExit(opts, cmd, "simpleperf_adapter", "running official simpleperf adapter")
	if boundaryErr := finishExternalToolCommand(ctx, inputLease, reportDir, runErr); boundaryErr != nil {
		progressFinished(opts, "simpleperf_adapter", "simpleperf command boundary rejected", tool, "", commandStart, ProgressStatusFailed)
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, boundaryErr
	}
	commandStatus := ProgressStatusComplete
	if runErr != nil {
		commandStatus = ProgressStatusFailed
	}
	commandMessage := terminalProgressMessage("running official simpleperf adapter", commandStatus)
	if !commandStarted {
		commandMessage = "external command failed to start"
	}
	progressFinished(opts, "simpleperf_adapter", commandMessage, tool, "", commandStart, commandStatus)
	if runErr != nil {
		caveat := fmt.Sprintf("official simpleperf adapter %q failed (%s)%s", tool, runErr, boundedPerfAdapterCommandOutput(output, "simpleperf"))
		officialDecision = perfProviderFailure(officialDecision, "official_adapter_failed", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackForSimpleperf(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	sealedReport, err := reportDir.AdoptRegularChild("report_sample.txt", true)
	if err != nil {
		caveat := fmt.Sprintf("official simpleperf adapter %q produced unreadable report (%v)", tool, err)
		officialDecision = perfProviderFailure(officialDecision, "official_output_unreadable", caveat)
		artifact, rawCaveat, rawDecisions, fallbackErr := maybeRawPerfFallbackForSimpleperf(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), fallbackErr
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, sealedReport.Close())
	}()
	samples, parseErr := parseSimpleperfReport(ctx, sealedReport.Reader())
	parseErr = finishSealedConversionFile(sealedReport, parseErr)
	if parseErr == nil && len(samples) == 0 {
		parseErr = fmt.Errorf("simpleperf report contains no samples")
	}
	if parseErr == nil && simpleperfSamplesContainPrivatePath(samples, privateStagingIdentity) {
		parseErr = fmt.Errorf("simpleperf report contains a private adapter input identity")
	}
	if parseErr == nil {
		parseErr = applySimpleperfTextProfile(samples, profile)
	}
	if parseErr == nil {
		parseErr = writeSimpleperfSamplesToPerfTraceWithLedger(ctx, samples, perfTracePath, ledger)
	}
	if parseErr != nil {
		if ownedTraceOutputHardFailure(parseErr) {
			return Artifact{}, "", []PerfProviderDecision{officialDecision}, parseErr
		}
		caveat := fmt.Sprintf("official simpleperf adapter %q produced unreadable report (%v)", tool, parseErr)
		officialDecision = perfProviderFailure(officialDecision, "official_output_unreadable", caveat)
		artifact, rawCaveat, rawDecisions, fallbackErr := maybeRawPerfFallbackForSimpleperf(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), fallbackErr
	}
	artifact, err = newValidatedPerfTraceArtifact(
		ledger, perfTracePath, ownedTracePerfSimpleperfText, inputFormat, source, []string{
			fmt.Sprintf("generated from perf data through %s; sample CPU comes from simpleperf SampleStruct.cpu", source),
			profile.caveat(),
		},
	)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	officialDecision, err = perfProviderSuccess(officialDecision, artifact, ledger)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	return artifact, "", []PerfProviderDecision{officialDecision}, nil
}

func simpleperfSamplesContainPrivatePath(samples []simpleperfSample, privateIdentity privatePathIdentity) bool {
	prefixes := privateIdentity.prefixes
	contains := func(value string) bool {
		folded := strings.ToLower(value)
		if strings.Contains(folded, strings.ToLower(simpleperfInputSnapshotLeaf)) {
			return true
		}
		for _, prefix := range prefixes {
			if strings.Contains(folded, strings.ToLower(prefix)) {
				return true
			}
		}
		return false
	}
	frameContains := func(frame simpleperfFrame) bool {
		return contains(frame.IP) || contains(frame.Symbol) || contains(frame.DSO)
	}
	for _, sample := range samples {
		if contains(sample.Comm) || contains(sample.Event) || frameContains(sample.Leaf) {
			return true
		}
		for _, frame := range sample.CallFrames {
			if frameContains(frame) {
				return true
			}
		}
	}
	return false
}

type simpleperfToolResolution struct {
	Tool                 string
	Python               string
	Source               string
	ExternalInputProfile externalToolInputProfile
}

func resolveSimpleperfProviderTool(opts Options) simpleperfToolResolution {
	tool, python, source := resolveSimpleperfReportTool(opts)
	return simpleperfToolResolution{
		Tool:                 tool,
		Python:               python,
		Source:               source,
		ExternalInputProfile: externalToolInputSnapshotOnly,
	}
}

func maybeRawPerfFallbackForSimpleperf(ctx context.Context, opts Options, input directPerfInputBinding, perfTracePath string, prior string, stage string, ledger *conversionFileLedger) (Artifact, string, []PerfProviderDecision, error) {
	if err := input.validate(); err != nil {
		return Artifact{}, "", nil, err
	}
	perfPath := input.displayPath
	inputFormat := input.inputFormat
	if inputFormat == perfInputLinuxPerfData {
		return maybeRawPerfFallbackFromInput(ctx, opts, input, perfTracePath, prior, stage, ledger)
	}
	decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameRawFallback), opts, perfPath, inputFormat, perfTracePath)
	if !rawPerfParserAllowed(opts) {
		caveat := prior + "; raw perf.data fallback disabled by perf parser mode, so .perftrace was not generated"
		decision = perfProviderSkipped(decision, false, "disabled_by_parser_mode", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	if inputFormat == perfInputSimpleperfReportProto {
		caveat := prior + "; Codrax raw fallback does not parse Android SIMPLEPERF report-sample protobuf yet, so .perftrace was not generated"
		decision = perfProviderSkipped(decision, true, "unsupported_input_format", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	caveat := prior + "; unsupported perf input format for raw fallback, so .perftrace was not generated"
	decision = perfProviderSkipped(decision, true, "unsupported_input_format", caveat)
	return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
}

func resolveSimpleperfReportTool(opts Options) (tool string, python string, source string) {
	if path := strings.TrimSpace(opts.SimpleperfReportPath); path != "" {
		return resolveSimpleperfReportWrapper(opts, path, "configured simpleperf report_sample.py")
	}
	if path := strings.TrimSpace(os.Getenv("CODRAX_SIMPLEPERF_REPORT_SAMPLE")); path != "" {
		return resolveSimpleperfReportWrapper(opts, path, "CODRAX_SIMPLEPERF_REPORT_SAMPLE")
	}
	for _, name := range []string{"report_sample.py", "simpleperf_report_sample.py"} {
		if path, err := exec.LookPath(name); err == nil && strings.TrimSpace(path) != "" {
			return path, resolveSimpleperfPython(opts, path), name + " on PATH"
		}
	}
	for _, name := range []string{"simpleperf_report_lib.py"} {
		if path, err := exec.LookPath(name); err == nil && strings.TrimSpace(path) != "" {
			if wrapper := siblingSimpleperfReportSample(path); wrapper != "" {
				return wrapper, resolveSimpleperfPython(opts, wrapper), "report_sample.py next to " + name + " on PATH"
			}
		}
	}
	return "", "", ""
}

func resolveSimpleperfReportWrapper(opts Options, path, source string) (tool string, python string, resolvedSource string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", ""
	}
	if simpleperfPathLooksReportLibrary(path) {
		if wrapper := siblingSimpleperfReportSample(path); wrapper != "" {
			return wrapper, resolveSimpleperfPython(opts, wrapper), "report_sample.py next to " + source
		}
		return "", "", ""
	}
	return path, resolveSimpleperfPython(opts, path), source
}

func simpleperfPathLooksReportLibrary(path string) bool {
	return strings.EqualFold(filepath.Base(strings.TrimSpace(path)), "simpleperf_report_lib.py")
}

func siblingSimpleperfReportSample(path string) string {
	dir := filepath.Dir(strings.TrimSpace(path))
	for _, name := range []string{"report_sample.py", "simpleperf_report_sample.py", "report_sample"} {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func resolveSimpleperfPython(opts Options, script string) string {
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(script)), ".py") {
		return ""
	}
	if path := strings.TrimSpace(opts.SimpleperfPythonPath); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("CODRAX_SIMPLEPERF_PYTHON")); path != "" {
		return path
	}
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil && strings.TrimSpace(path) != "" {
			return path
		}
	}
	return ""
}

func ConvertSimpleperfReportFileToPerfTrace(ctx context.Context, inputPath, outputPath string) error {
	return runConversionFileTransaction(ctx, inputPath, func(ledger *conversionFileLedger) error {
		return convertSimpleperfReportFileToPerfTraceWithLedger(ctx, inputPath, outputPath, ledger)
	})
}

func convertSimpleperfReportFileToPerfTraceWithLedger(ctx context.Context, inputPath, outputPath string, ledger *conversionFileLedger) error {
	if ctx == nil {
		ctx = context.Background()
	}
	in, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer in.Close()
	samples, err := parseSimpleperfReport(ctx, in)
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		return fmt.Errorf("simpleperf report contains no samples")
	}
	return writeSimpleperfSamplesToPerfTraceWithLedger(ctx, samples, outputPath, ledger)
}

func writeSimpleperfSamplesToPerfTraceWithLedger(ctx context.Context, samples []simpleperfSample, outputPath string, ledger *conversionFileLedger) error {
	if len(samples) == 0 {
		return fmt.Errorf("simpleperf report contains no samples")
	}
	_, err := writeValidatedOwnedPerfTraceWithLedger(
		ctx, ownedPerfTraceWriteSpec{Profile: ownedTracePerfSimpleperfText, ExpectedRows: len(samples)}, outputPath, ledger,
		func(writer io.Writer) error { return writeSimpleperfPerfTrace(ctx, writer, samples) },
	)
	return err
}

func parseSimpleperfReport(ctx context.Context, r io.Reader) ([]simpleperfSample, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var samples []simpleperfSample
	var current *simpleperfSample
	currentHeaderLine := 0
	inTracingData := false
	lineNo := 0
	flush := func() error {
		if current == nil {
			return nil
		}
		if current.Leaf.Symbol == "" {
			return simpleperfReportParseError(currentHeaderLine, "sample", "missing_leaf_frame")
		}
		samples = append(samples, *current)
		current = nil
		currentHeaderLine = 0
		inTracingData = false
		return nil
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		if inTracingData {
			if simpleperfTracingDataField(line) {
				continue
			}
			return nil, simpleperfReportParseError(lineNo, "tracing_data", "unrecognized_field")
		}
		if m := simpleperfSampleHeaderRE.FindStringSubmatch(line); len(m) == 9 {
			if current != nil {
				if current.Leaf.Symbol == "" {
					return nil, simpleperfReportParseError(currentHeaderLine, "sample", "missing_leaf_frame")
				}
				return nil, simpleperfReportParseError(lineNo, "sample", "missing_sample_boundary")
			}
			sample, err := parseSimpleperfSampleHeader(m, lineNo)
			if err != nil {
				return nil, err
			}
			current = &sample
			currentHeaderLine = lineNo
			inTracingData = false
			continue
		}
		if simpleperfSampleHeaderCandidate(line) {
			return nil, simpleperfReportParseError(lineNo, "header", "malformed")
		}
		frame, ok := parseSimpleperfFrame(line)
		if ok {
			if current == nil {
				return nil, simpleperfReportParseError(lineNo, "frame", "before_sample_header")
			}
			if current.Leaf.Symbol == "" {
				current.Leaf = frame
			} else {
				current.CallFrames = append(current.CallFrames, frame)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			if current != nil {
				return nil, simpleperfReportParseError(lineNo, "row", "comment_inside_sample")
			}
			continue
		}
		if trimmed == "tracing data:" {
			if current == nil || current.Leaf.Symbol == "" || inTracingData {
				return nil, simpleperfReportParseError(lineNo, "tracing_data", "invalid_boundary")
			}
			inTracingData = true
			continue
		}
		return nil, simpleperfReportParseError(lineNo, "row", "unrecognized")
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return samples, nil
}

func parseSimpleperfSampleHeader(match []string, lineNo int) (simpleperfSample, error) {
	if len(match) != 9 {
		return simpleperfSample{}, simpleperfReportParseError(lineNo, "header", "malformed")
	}
	comm := strings.TrimSpace(match[1])
	if comm == "" {
		return simpleperfSample{}, simpleperfReportParseError(lineNo, "comm", "empty")
	}
	event := strings.TrimSpace(match[8])
	if event == "" {
		return simpleperfSample{}, simpleperfReportParseError(lineNo, "event", "empty")
	}
	pid, err := parseSimpleperfReportUint(match[2], math.MaxInt32, lineNo, "pid")
	if err != nil {
		return simpleperfSample{}, err
	}
	tid, err := parseSimpleperfReportUint(match[3], math.MaxInt32, lineNo, "tid")
	if err != nil {
		return simpleperfSample{}, err
	}
	cpu, err := parseSimpleperfReportUint(match[4], 4095, lineNo, "cpu")
	if err != nil {
		return simpleperfSample{}, err
	}
	timestampNS, err := parseSimpleperfReportTimestamp(match[5], match[6], lineNo)
	if err != nil {
		return simpleperfSample{}, err
	}
	period, err := parseSimpleperfReportUint(match[7], math.MaxInt64, lineNo, "period")
	if err != nil {
		return simpleperfSample{}, err
	}
	return simpleperfSample{
		Comm: comm, PID: int64(pid), TID: int64(tid), CPU: int64(cpu),
		TimestampNS: timestampNS, Period: period, Event: event,
	}, nil
}

func parseSimpleperfReportUint(raw string, maximum uint64, lineNo int, field string) (uint64, error) {
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, simpleperfReportParseError(lineNo, field, "invalid_unsigned_decimal")
	}
	if value > maximum {
		return 0, simpleperfReportParseError(lineNo, field, "out_of_range")
	}
	return value, nil
}

func parseSimpleperfReportTimestamp(secondsRaw, microsecondsRaw string, lineNo int) (uint64, error) {
	seconds, err := parseSimpleperfReportUint(secondsRaw, math.MaxUint64, lineNo, "timestamp_seconds")
	if err != nil {
		return 0, err
	}
	microseconds, err := parseSimpleperfReportUint(microsecondsRaw, 999_999, lineNo, "timestamp_microseconds")
	if err != nil {
		return 0, err
	}
	const nanosPerSecond = uint64(1_000_000_000)
	fraction := microseconds * 1_000
	if seconds > (math.MaxUint64-fraction)/nanosPerSecond {
		return 0, simpleperfReportParseError(lineNo, "timestamp", "out_of_range")
	}
	return seconds*nanosPerSecond + fraction, nil
}

func simpleperfTracingDataField(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "\t\t") {
		return false
	}
	key, _, ok := strings.Cut(strings.TrimSpace(trimmed), " : ")
	return ok && strings.TrimSpace(key) != ""
}

func simpleperfSampleHeaderCandidate(line string) bool {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		// Official frame rows are horizontally indented. Their symbol/DSO text
		// is variable data and must win over header-shaped substrings.
		return false
	}
	tab := strings.IndexByte(line, '\t')
	if tab <= 0 || tab+1 >= len(line) {
		return false
	}
	rest := line[tab+1:]
	open := strings.IndexByte(rest, '[')
	close := strings.IndexByte(rest, ']')
	return open > 0 && close > open && strings.Contains(rest[close+1:], ":")
}

func simpleperfReportParseError(lineNo int, field, reason string) error {
	return &simpleperfReportError{Line: lineNo, Field: field, Reason: reason}
}

func parseSimpleperfFrame(line string) (simpleperfFrame, bool) {
	m := simpleperfSymbolLineRE.FindStringSubmatch(line)
	if len(m) != 4 {
		return simpleperfFrame{}, false
	}
	return simpleperfFrame{
		IP:     "0x" + strings.ToLower(strings.TrimSpace(m[1])),
		Symbol: strings.TrimSpace(m[2]),
		DSO:    strings.TrimSpace(m[3]),
	}, true
}

func writeSimpleperfPerfTrace(ctx context.Context, w io.Writer, samples []simpleperfSample) error {
	ordered := append([]simpleperfSample(nil), samples...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return ordered[left].TimestampNS < ordered[right].TimestampNS
	})
	for index := range ordered {
		if err := validateSimpleperfSampleForWrite(ordered[index]); err != nil {
			return fmt.Errorf("simpleperf sample %d: %w", index+1, err)
		}
	}
	if _, err := io.WriteString(w, systraceHeader); err != nil {
		return err
	}
	for _, sample := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		comm := sanitizePerfTraceComm(firstNonEmpty(sample.Comm, fmt.Sprintf("tid%d", sample.TID)))
		period := sample.Period
		if period == 0 {
			period = 1
		}
		weight, err := tracewire.CheckedPerfSampleWeight(period)
		if err != nil {
			return err
		}
		callchain, err := simpleperfCallchain(ctx, sample)
		if err != nil {
			return err
		}
		source := "simpleperf_report_sample"
		symbol := firstNonEmpty(sample.Leaf.Symbol, sample.Leaf.IP, "unknown")
		dso := firstNonEmpty(sample.Leaf.DSO, "unknown")
		symbolizationStatus := perfTraceSymbolizationStatus(symbol, dso, source)
		callchainStatus := perfTraceCallchainStatus(callchain, source)
		body, err := tracewire.BuildPerfSampleBody(tracewire.PerfSampleRow{
			Layout:              tracewire.PerfSampleLayoutBase,
			CPU:                 sample.CPU,
			CPUKnown:            true,
			PID:                 sample.PID,
			TID:                 sample.TID,
			ThreadComm:          firstNonEmpty(sample.Comm, comm),
			SampleWeight:        weight,
			Event:               sample.Event,
			Symbol:              symbol,
			DSO:                 dso,
			IP:                  sample.Leaf.IP,
			Callchain:           callchain,
			Source:              tracewire.PerfSampleSourceSimpleperfReportSample,
			SampleKind:          sample.SampleKind,
			SymbolizationStatus: tracewire.PerfSymbolizationStatus(symbolizationStatus),
			Clock:               tracewire.PerfSampleClockRecord,
			ClockConfidence:     tracewire.PerfClockConfidenceAssumed,
			CallchainStatus:     tracewire.PerfCallchainStatus(callchainStatus),
		})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%16s-%-5d (%5d) [%03d] .... %12s: %s\n",
			perfTraceHeaderComm(comm), sample.TID, sample.PID, sample.CPU, formatTimestamp(sample.TimestampNS), body); err != nil {
			return err
		}
	}
	return nil
}

func validateSimpleperfSampleForWrite(sample simpleperfSample) error {
	if sample.PID < 0 || sample.PID > math.MaxInt32 {
		return &tracewire.PerfWireBuildError{Field: "pid", Reason: "out_of_range", Limit: math.MaxInt32, Actual: nonNegativeSimpleperfScalar(sample.PID)}
	}
	if sample.TID < 0 || sample.TID > math.MaxInt32 {
		return &tracewire.PerfWireBuildError{Field: "tid", Reason: "out_of_range", Limit: math.MaxInt32, Actual: nonNegativeSimpleperfScalar(sample.TID)}
	}
	if sample.CPU < 0 || sample.CPU > 4095 {
		return &tracewire.PerfWireBuildError{Field: "cpu", Reason: "out_of_range", Limit: 4095, Actual: nonNegativeSimpleperfScalar(sample.CPU)}
	}
	if sample.TimestampNS%1_000 != 0 {
		return fmt.Errorf("timestamp is not representable by the six-decimal report grammar")
	}
	period := sample.Period
	if period == 0 {
		period = 1
	}
	if _, err := tracewire.CheckedPerfSampleWeight(period); err != nil {
		return err
	}
	if strings.TrimSpace(sample.Comm) == "" {
		return fmt.Errorf("comm is empty")
	}
	if strings.TrimSpace(sample.Event) == "" {
		return fmt.Errorf("event is empty")
	}
	if strings.TrimSpace(sample.Leaf.Symbol) == "" {
		return fmt.Errorf("leaf frame is missing")
	}
	return nil
}

func nonNegativeSimpleperfScalar(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func simpleperfCallchain(ctx context.Context, sample simpleperfSample) (string, error) {
	var builder tracewire.PerfCallchainBuilder
	for i := len(sample.CallFrames) - 1; i >= 0; i-- {
		if err := builder.AppendFrame(ctx, simpleperfCallchainFrameLabel(sample.CallFrames[i])); err != nil {
			return "", err
		}
	}
	if err := builder.AppendFrame(ctx, simpleperfCallchainFrameLabel(sample.Leaf)); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func simpleperfCallchainFrameLabel(frame simpleperfFrame) string {
	label := firstNonEmpty(frame.Symbol, frame.IP, "unknown")
	if frame.DSO != "" && frame.DSO != "unknown" {
		label += "@" + frame.DSO
	}
	return label
}
