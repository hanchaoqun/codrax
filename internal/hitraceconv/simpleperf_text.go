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

func maybeConvertDirectSimpleperfPerfData(ctx context.Context, opts Options, input string, inputBytes int64, outputPath string) (Result, bool, error) {
	inputFormat := detectPerfInputFormat(input)
	if !simpleperfDirectRequested(inputFormat) {
		return Result{}, false, nil
	}
	base := traceSidecarBase(input, outputPath)
	perfTracePath := base + ".perftrace"
	perfTrace, caveat, decisions, err := maybeConvertSimpleperfPerfData(ctx, opts, input, perfTracePath, inputFormat, perfProviderStageDirectInput)
	if err != nil {
		return Result{}, true, err
	}
	result := Result{
		InputPath:   input,
		InputBytes:  inputBytes,
		OutputPath:  "",
		OutputBytes: 0,
		Artifacts: []Artifact{{
			Type:      ArtifactPerfData,
			Path:      input,
			Bytes:     inputBytes,
			Converter: "external",
			Perf:      perfCapabilityForRawPerfDataArtifact(inputFormat),
			Caveats:   []string{"input perf.data preserved; normalized .perftrace is the trace_query CPU-sample artifact"},
		}},
		ProviderDecisions: decisions,
	}
	if perfTrace.Path != "" {
		result.Artifacts = append(result.Artifacts, perfTrace)
	} else if caveat != "" {
		result.Caveats = append(result.Caveats, caveat)
		result.Artifacts[0].Caveats = append(result.Artifacts[0].Caveats, "official simpleperf adapter did not produce .perftrace")
	}
	if bundleArtifact, err := writeTraceBundle(input, "", result.Artifacts, result.Caveats, result.ProviderDecisions); err != nil {
		return Result{}, true, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	return result, true, nil
}

func simpleperfDirectRequested(inputFormat perfInputFormat) bool {
	if inputFormat == perfInputLinuxPerfData || inputFormat == perfInputSimpleperfReportProto {
		return true
	}
	return false
}

func maybeConvertSimpleperfPerfData(ctx context.Context, opts Options, perfPath, perfTracePath string, inputFormat perfInputFormat, stage string) (Artifact, string, []PerfProviderDecision, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.DisablePerfAdapter {
		caveat := "perf.data preserved; perftrace generation disabled, so .perftrace was not generated"
		decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNamePerftraceDisabled), opts, perfPath, inputFormat, perfTracePath)
		decision = perfProviderSkipped(decision, true, "perftrace_generation_disabled", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	if inputFormat == perfInputSimpleperfReportProto && !rawPerfParserRequired(opts) {
		return maybeConvertSimpleperfProtoWithDecision(ctx, opts, perfPath, perfTracePath, stage)
	}
	if rawPerfParserRequired(opts) {
		if inputFormat != perfInputLinuxPerfData {
			caveat := fmt.Sprintf("%s preserved; Codrax raw fallback supports %s only, so .perftrace was not generated", firstNonEmpty(string(inputFormat), "input"), perfInputLinuxPerfData)
			decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameRawFallback), opts, perfPath, inputFormat, perfTracePath)
			decision = perfProviderSkipped(decision, true, "unsupported_input_format", caveat)
			return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
		}
		return maybeConvertRawPerfDataWithDecision(ctx, opts, perfPath, perfTracePath, "", stage, inputFormat, false)
	}
	officialDecision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameSimpleperfText), opts, perfPath, inputFormat, perfTracePath)
	tool, python, source := resolveSimpleperfReportTool(opts)
	if tool == "" {
		caveat := "perf data preserved; no official simpleperf report_sample.py adapter was configured or found"
		officialDecision = perfProviderSkipped(officialDecision, true, "official_tool_unavailable", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackForSimpleperf(ctx, opts, perfPath, perfTracePath, inputFormat, caveat, stage)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	if err := ensureOutputDoesNotExist(perfTracePath); err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	reportFile, err := os.CreateTemp(filepath.Dir(perfTracePath), "."+filepath.Base(perfTracePath)+".*.simpleperf.txt")
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	reportPath := reportFile.Name()
	if err := reportFile.Close(); err != nil {
		_ = os.Remove(reportPath)
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	_ = os.Remove(reportPath)
	defer os.Remove(reportPath)

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
	output, runErr := cmd.CombinedOutput()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, ctxErr
	}
	if runErr != nil {
		caveat := fmt.Sprintf("official simpleperf adapter %q failed (%s)%s", tool, runErr, boundedCommandOutput(output))
		officialDecision = perfProviderFailure(officialDecision, "official_adapter_failed", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackForSimpleperf(ctx, opts, perfPath, perfTracePath, inputFormat, caveat, stage)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	if err := ConvertSimpleperfReportFileToPerfTrace(ctx, reportPath, perfTracePath); err != nil {
		_ = os.Remove(perfTracePath)
		caveat := fmt.Sprintf("official simpleperf adapter %q produced unreadable report (%v)", tool, err)
		officialDecision = perfProviderFailure(officialDecision, "official_output_unreadable", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackForSimpleperf(ctx, opts, perfPath, perfTracePath, inputFormat, caveat, stage)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	info, err := os.Stat(perfTracePath)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	artifact := Artifact{
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

func maybeRawPerfFallbackForSimpleperf(ctx context.Context, opts Options, perfPath, perfTracePath string, inputFormat perfInputFormat, prior string, stage string) (Artifact, string, []PerfProviderDecision, error) {
	if inputFormat == perfInputLinuxPerfData {
		return maybeRawPerfFallback(ctx, opts, perfPath, perfTracePath, prior, stage, inputFormat)
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
	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	writeErr := writeSimpleperfPerfTrace(ctx, w, samples)
	flushErr := w.Flush()
	closeErr := out.Close()
	if writeErr != nil {
		_ = os.Remove(outputPath)
		return writeErr
	}
	if flushErr != nil {
		_ = os.Remove(outputPath)
		return flushErr
	}
	if closeErr != nil {
		_ = os.Remove(outputPath)
		return closeErr
	}
	return nil
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
		if _, err := fmt.Fprintf(w, "%16s-%-6d (%5d) [%03d] .... %12.6f: perf_sample: cpu=%d cpu_known=true pid=%d tid=%d thread_comm=%s sample_weight=%d event=%s symbol=%s dso=%s ip=%s callchain=%s source=%s symbolization_status=%s clock=record clock_confidence=assumed callchain_status=%s\n",
			comm, sample.TID, sample.PID, sample.CPU, sample.Timestamp, sample.CPU, sample.PID, sample.TID, quoteTraceValue(firstNonEmpty(sample.Comm, comm)), sample.Period, quoteTraceValue(sample.Event), quoteTraceValue(symbol), quoteTraceValue(dso), quoteTraceValue(sample.Leaf.IP), quoteTraceValue(callchain), source, symbolizationStatus, callchainStatus); err != nil {
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
