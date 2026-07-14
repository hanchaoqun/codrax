package hitraceconv

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const simpleperfAdapterVersion = converterVersion + "+simpleperf-report-sample"

var (
	simpleperfSampleHeaderRE = regexp.MustCompile(`^(.+)\t([0-9]+)/([0-9]+)\s+\[([0-9]+)\]\s+([0-9]+)\.([0-9]{6}):\s+([0-9]+)\s+(.+):$`)
	simpleperfSymbolLineRE   = regexp.MustCompile(`^\s*([0-9a-fA-F]+)\s+(.+)\s+\((.*)\)\s*$`)
)

type simpleperfSample struct {
	Comm       string
	PID        int
	TID        int
	CPU        int
	Timestamp  float64
	Period     uint64
	Event      string
	Leaf       simpleperfFrame
	CallFrames []simpleperfFrame
}

type simpleperfFrame struct {
	IP     string
	Symbol string
	DSO    string
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
			Caveats:   []string{"input perf.data preserved; normalized .perftrace is the trace_query CPU-sample artifact"},
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
	} else if caveat != "" {
		result.Caveats = append(result.Caveats, caveat)
		result.Artifacts[0].Caveats = append(result.Artifacts[0].Caveats, "official simpleperf adapter did not produce .perftrace")
	}
	normalizeResultCollections(&result)
	if bundleArtifact, err := writeTraceBundleWithLedger(input.displayPath, "", result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, ledger); err != nil {
		return Result{}, true, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	normalizeResultCollections(&result)
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
	tool, python, source := resolveSimpleperfReportTool(opts)
	if tool == "" {
		caveat := "perf data preserved; no official simpleperf report_sample.py adapter was configured or found"
		officialDecision = perfProviderSkipped(officialDecision, true, "official_tool_unavailable", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackForSimpleperf(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	if err := ensureOutputDoesNotExist(perfTracePath); err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	reportDir, err := os.MkdirTemp(filepath.Dir(perfTracePath), "."+filepath.Base(perfTracePath)+".*.simpleperf")
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	reportDirInfo, err := os.Lstat(reportDir)
	if err != nil || !reportDirInfo.IsDir() || reportDirInfo.Mode().Perm() != 0o700 {
		cleanupErr := removeOwnedConversionDir(reportDir, reportDirInfo)
		if err == nil {
			err = fmt.Errorf("simpleperf staging path is not a private directory: %s mode=%s", reportDir, reportDirInfo.Mode())
		}
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, traceDBJoinPreservingSingle(err, cleanupErr)
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, removeOwnedConversionDir(reportDir, reportDirInfo))
	}()
	reportPath := filepath.Join(reportDir, "report_sample.txt")

	args := []string{"-i", perfPath, "-o", reportPath}
	if symfs := strings.TrimSpace(opts.SimpleperfSymfsDir); symfs != "" {
		args = append(args, "--symfs", symfs)
	}
	if kallsyms := strings.TrimSpace(opts.SimpleperfKallsymsPath); kallsyms != "" {
		args = append(args, "--kallsyms", kallsyms)
	}
	cmdName := tool
	cmdArgs := args
	if python != "" {
		cmdName = python
		cmdArgs = append([]string{tool}, args...)
	}
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	output, runErr := runCommandWithProgress(opts, cmd, "simpleperf_adapter", "running official simpleperf adapter")
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, ctxErr
	}
	if runErr != nil {
		caveat := fmt.Sprintf("official simpleperf adapter %q failed (%s)%s", tool, runErr, boundedCommandOutput(output))
		officialDecision = perfProviderFailure(officialDecision, "official_adapter_failed", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackForSimpleperf(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	reportInfo, err := os.Lstat(reportPath)
	if err != nil || !reportInfo.Mode().IsRegular() {
		if err == nil {
			err = fmt.Errorf("official simpleperf adapter produced a non-regular report: %s", reportPath)
		}
		caveat := fmt.Sprintf("official simpleperf adapter %q produced unreadable report (%v)", tool, err)
		officialDecision = perfProviderFailure(officialDecision, "official_output_unreadable", caveat)
		artifact, rawCaveat, rawDecisions, fallbackErr := maybeRawPerfFallbackForSimpleperf(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), fallbackErr
	}
	if err := convertSimpleperfReportFileToPerfTraceWithLedger(ctx, reportPath, perfTracePath, ledger); err != nil {
		caveat := fmt.Sprintf("official simpleperf adapter %q produced unreadable report (%v)", tool, err)
		officialDecision = perfProviderFailure(officialDecision, "official_output_unreadable", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackForSimpleperf(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	info, err := os.Lstat(perfTracePath)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	artifact = Artifact{
		Type:      ArtifactPerfTrace,
		Path:      perfTracePath,
		Bytes:     info.Size(),
		Converter: simpleperfAdapterVersion,
		Perf:      perfCapabilityForSimpleperfReportSample(inputFormat, source),
		Caveats: []string{
			fmt.Sprintf("generated from perf data through %s; sample CPU comes from simpleperf SampleStruct.cpu", source),
		},
	}
	officialDecision = perfProviderSuccess(officialDecision, artifact)
	return artifact, "", []PerfProviderDecision{officialDecision}, nil
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
	if err := ensureOutputDoesNotExist(outputPath); err != nil {
		return err
	}
	out, err := openOwnedConversionFile(outputPath, ledger)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	writeErr := writeSimpleperfPerfTrace(ctx, w, samples)
	flushErr := w.Flush()
	_, err = finishOwnedConversionFile(outputPath, out, ledger, true, writeErr, flushErr)
	return err
}

func parseSimpleperfReport(ctx context.Context, r io.Reader) ([]simpleperfSample, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var samples []simpleperfSample
	var current *simpleperfSample
	lineNo := 0
	flush := func() {
		if current != nil && current.Leaf.Symbol != "" {
			samples = append(samples, *current)
		}
		current = nil
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "tracing data:") || strings.Contains(trimmed, " : ") {
			continue
		}
		if m := simpleperfSampleHeaderRE.FindStringSubmatch(line); len(m) == 9 {
			flush()
			pid, _ := strconv.Atoi(m[2])
			tid, _ := strconv.Atoi(m[3])
			cpu, _ := strconv.Atoi(m[4])
			sec, _ := strconv.ParseInt(m[5], 10, 64)
			usec, _ := strconv.ParseInt(m[6], 10, 64)
			period, _ := strconv.ParseUint(m[7], 10, 64)
			if period == 0 {
				period = 1
			}
			current = &simpleperfSample{
				Comm:      strings.TrimSpace(m[1]),
				PID:       pid,
				TID:       tid,
				CPU:       cpu,
				Timestamp: float64(sec) + float64(usec)/1e6,
				Period:    period,
				Event:     strings.TrimSpace(m[8]),
			}
			continue
		}
		frame, ok := parseSimpleperfFrame(line)
		if !ok {
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("simpleperf symbol row before sample header at line %d", lineNo)
		}
		if current.Leaf.Symbol == "" {
			current.Leaf = frame
		} else {
			current.CallFrames = append(current.CallFrames, frame)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return samples, nil
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
	if _, err := io.WriteString(w, systraceHeader); err != nil {
		return err
	}
	for _, sample := range samples {
		if err := ctx.Err(); err != nil {
			return err
		}
		comm := sanitizePerfTraceComm(firstNonEmpty(sample.Comm, fmt.Sprintf("tid%d", sample.TID)))
		callchain := simpleperfCallchain(sample)
		source := "simpleperf_report_sample"
		symbol := firstNonEmpty(sample.Leaf.Symbol, sample.Leaf.IP, "unknown")
		dso := firstNonEmpty(sample.Leaf.DSO, "unknown")
		symbolizationStatus := perfTraceSymbolizationStatus(symbol, dso, source)
		callchainStatus := perfTraceCallchainStatus(callchain, source)
		if _, err := fmt.Fprintf(w, "%16s-%-5d (%5d) [%03d] .... %12.6f: perf_sample: cpu=%d cpu_known=true pid=%d tid=%d thread_comm=%s sample_weight=%d event=%s symbol=%s dso=%s ip=%s callchain=%s source=%s symbolization_status=%s clock=record clock_confidence=assumed callchain_status=%s\n",
			perfTraceHeaderComm(comm), sample.TID, sample.PID, sample.CPU, sample.Timestamp, sample.CPU, sample.PID, sample.TID, quoteTraceValue(firstNonEmpty(sample.Comm, comm)), sample.Period, quoteTraceValue(sample.Event), quoteTraceValue(symbol), quoteTraceValue(dso), quoteTraceValue(sample.Leaf.IP), quoteTraceValue(callchain), source, symbolizationStatus, callchainStatus); err != nil {
			return err
		}
	}
	return nil
}

func simpleperfCallchain(sample simpleperfSample) string {
	frames := make([]simpleperfFrame, 0, len(sample.CallFrames)+1)
	for i := len(sample.CallFrames) - 1; i >= 0; i-- {
		frames = append(frames, sample.CallFrames[i])
	}
	frames = append(frames, sample.Leaf)
	parts := make([]string, 0, len(frames))
	for _, frame := range frames {
		label := firstNonEmpty(frame.Symbol, frame.IP, "unknown")
		if frame.DSO != "" && frame.DSO != "unknown" {
			label += "@" + frame.DSO
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ";")
}
