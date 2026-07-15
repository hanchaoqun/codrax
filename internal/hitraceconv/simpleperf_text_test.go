package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestConvertSimpleperfReportFileToPerfTraceRoundTripsThroughTraceQuery(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "simpleperf.txt")
	outPath := filepath.Join(dir, "simpleperf.perftrace")
	if err := os.WriteFile(reportPath, []byte(syntheticSimpleperfReport()), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ConvertSimpleperfReportFileToPerfTrace(context.Background(), reportPath, outPath); err != nil {
		t.Fatalf("convert simpleperf report: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"perf_sample:",
		"cpu=5",
		"pid=1234",
		"tid=5678",
		"sample_weight=10000",
		`event="cpu-cycles"`,
		`symbol="Foo::bar"`,
		`dso="/system/lib64/libfoo.so"`,
		`ip="0x1234"`,
		`callchain="main@/system/lib64/libfoo.so;A@/system/lib64/libfoo.so;Foo::bar@/system/lib64/libfoo.so"`,
		"source=simpleperf_report_sample",
		"cpu_known=true",
		"symbolization_status=symbolized",
		"clock_confidence=assumed",
		"callchain_status=symbolized",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("perftrace missing %q:\n%s", want, string(body))
		}
	}

	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("parse perftrace: %v", err)
	}
	if len(idx.Events) != 1 {
		t.Fatalf("events: got %d want 1", len(idx.Events))
	}
	ev := idx.Events[0]
	if ev.Type != tracequery.EventPerfSample || ev.CPU != 5 || ev.PerfFields.PID != 1234 || ev.PerfFields.TID != 5678 || ev.PerfFields.Period != 10000 {
		t.Fatalf("bad perf sample fields: %+v", ev)
	}
	if ev.PerfFields.EventName != "cpu-cycles" || ev.PerfFields.Symbol != "Foo::bar" || ev.PerfFields.DSO != "/system/lib64/libfoo.so" {
		t.Fatalf("bad perf symbol fields: %+v", ev)
	}
	if ev.PerfFields.CPUKnown == nil || !*ev.PerfFields.CPUKnown || ev.PerfFields.SymbolizationStatus != "symbolized" || ev.PerfFields.ClockConfidence != "assumed" || ev.PerfFields.CallchainStatus != "symbolized" {
		t.Fatalf("bad perf quality fields: %+v", ev)
	}
}

func TestConvertFileRunsConfiguredSimpleperfAdapterForDirectPerfDataByContent(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "capture.no_suffix")
	if err := os.WriteFile(perfData, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}
	toolPath := writeFakeSimpleperfReportTool(t, dir)

	output := filepath.Join(dir, "ignored.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: perfData, OutputPath: output, SimpleperfReportPath: toolPath})
	if err != nil {
		t.Fatalf("convert direct perf data: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("direct perf.data should be sidecar-only: %+v", result)
	}
	var perfTrace Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			perfTrace = artifact
			break
		}
	}
	if perfTrace.Path == "" {
		t.Fatalf("missing perftrace artifact: %+v", result.Artifacts)
	}
	if perfTrace.Perf == nil || perfTrace.Perf.ProviderKind != "official_android" || perfTrace.Perf.InputFormat != string(perfInputLinuxPerfData) {
		t.Fatalf("missing simpleperf capability: %+v", perfTrace.Perf)
	}
	if len(result.ProviderDecisions) != 1 {
		t.Fatalf("expected one simpleperf provider decision: %+v", result.ProviderDecisions)
	}
	decision := result.ProviderDecisions[0]
	if decision.ProviderName != perfProviderNameSimpleperfText || !decision.Selected || !decision.Attempted || !decision.Succeeded || !decision.TraceQueryReady {
		t.Fatalf("bad simpleperf provider decision: %+v", decision)
	}
	idx, err := tracequery.BuildIndex(context.Background(), perfTrace.Path)
	if err != nil {
		t.Fatalf("parse generated perftrace: %v", err)
	}
	if len(idx.Events) != 1 || idx.Events[0].PerfFields.Symbol != "Foo::bar" {
		t.Fatalf("generated perftrace did not round-trip: %+v", idx.Events)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type": "perftrace"`, perfTrace.Path, `"perf_capability"`, `"provider_kind": "official_android"`, `"input_format": "linux_perf_data"`, `"provider_decisions"`, `"provider_name": "android_simpleperf_report_sample"`, `"succeeded": true`, `"perf_clock_alignments"`, `"trace_time_domain": "missing_trace_body"`, `"confidence": "trace_body_missing"`} {
		if !strings.Contains(string(bundle), want) {
			t.Fatalf("bundle missing %q:\n%s", want, string(bundle))
		}
	}
	if _, err := os.Stat(output); err == nil {
		t.Fatalf("direct perf.data conversion should not create systrace output %s", output)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestReleaseSimpleperfAdapterConsumesPrivateAuthoritySnapshot(t *testing.T) {
	dir := t.TempDir()
	authorityBody := syntheticRawPerfData()
	publicBody := bytes.Replace(append([]byte(nil), authorityBody...), []byte("app\x00"), []byte("bad\x00"), 1)
	if bytes.Equal(authorityBody, publicBody) || len(authorityBody) != len(publicBody) {
		t.Fatal("fixture did not create a distinct equal-size public generation")
	}
	publicPath := filepath.Join(dir, "public.perf.data")
	expectedPath := filepath.Join(dir, "authority.perf.data")
	recordPath := filepath.Join(dir, "adapter-input.txt")
	if err := os.WriteFile(publicPath, publicBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(expectedPath, authorityBody, 0o600); err != nil {
		t.Fatal(err)
	}
	toolPath := writeFakeSimpleperfReportTool(t, dir)
	t.Setenv("SIMPLEPERF_EXPECTED_INPUT", expectedPath)
	t.Setenv("SIMPLEPERF_INPUT_RECORD", recordPath)

	view := newScriptedStandaloneInputView(publicPath, authorityBody)
	binding, err := newDirectPerfInputBinding(view, perfInputLinuxPerfData)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := ledger.cleanup(); cleanupErr != nil {
			t.Errorf("cleanup: %v", cleanupErr)
		}
	}()
	var progress []ProgressEvent
	result, ok, err := maybeConvertDirectSimpleperfPerfData(
		context.Background(),
		Options{SimpleperfReportPath: toolPath, Progress: func(event ProgressEvent) { progress = append(progress, event) }},
		traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
		binding,
		filepath.Join(dir, "unused.systrace"),
		ledger,
	)
	if err != nil || !ok {
		t.Fatalf("simpleperf lease conversion ok=%t err=%v", ok, err)
	}
	if len(result.ProviderDecisions) != 1 || result.ProviderDecisions[0].ProviderName != perfProviderNameSimpleperfText ||
		!result.ProviderDecisions[0].Attempted || !result.ProviderDecisions[0].Succeeded {
		t.Fatalf("authority snapshot fixture did not succeed through the official provider: %+v", result.ProviderDecisions)
	}
	perfTrace := directPerfArtifactByType(result.Artifacts, ArtifactPerfTrace)
	if perfTrace.Path == "" || perfTrace.Converter != simpleperfAdapterVersion {
		t.Fatalf("authority snapshot fixture silently fell back: %+v", result.Artifacts)
	}
	privateInputBytes, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	privateInput := strings.TrimSpace(string(privateInputBytes))
	if privateInput == "" || sameConversionCanonicalPath(privateInput, publicPath) || filepath.Base(privateInput) != "simpleperf_input.perf.data" {
		t.Fatalf("adapter input was not the fixed private lease: public=%q private=%q", publicPath, privateInput)
	}
	if _, err := os.Lstat(privateInput); !os.IsNotExist(err) {
		t.Fatalf("private adapter input survived provider cleanup: %v", err)
	}
	privateDir := filepath.Dir(privateInput)
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bundle), privateDir) || strings.Contains(string(bundle), filepath.ToSlash(privateDir)) {
		t.Fatalf("bundle leaked private adapter directory %q:\n%s", privateDir, bundle)
	}
	foundSnapshotComplete := false
	for _, event := range progress {
		if strings.Contains(event.Path, privateDir) || strings.Contains(event.OutputPath, privateDir) || strings.Contains(event.Message, privateDir) {
			t.Fatalf("progress leaked private adapter directory: %+v", event)
		}
		if event.Stage == "simpleperf_input_snapshot" && event.Status == ProgressStatusComplete {
			foundSnapshotComplete = true
		}
	}
	if !foundSnapshotComplete || view.reads == 0 || view.counts[conversionInputStageExternalTool] == 0 {
		t.Fatalf("snapshot authority/progress was not exercised: reads=%d gates=%v progress=%+v", view.reads, view.counts, progress)
	}
}

func TestReleaseSimpleperfAdapterPrivateReportIdentityFallsBackWithoutLeak(t *testing.T) {
	for _, mode := range []string{"private-report", "canonical-private-report"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			inputPath := filepath.Join(dir, "capture.perf.data")
			if err := os.WriteFile(inputPath, syntheticRawPerfData(), 0o600); err != nil {
				t.Fatal(err)
			}
			toolPath := writeFakeSimpleperfReportTool(t, dir)
			recordPath := filepath.Join(dir, "adapter-input.txt")
			t.Setenv("SIMPLEPERF_INPUT_RECORD", recordPath)
			t.Setenv("SIMPLEPERF_TEST_MODE", mode)

			result, err := ConvertFile(context.Background(), Options{InputPath: inputPath, SimpleperfReportPath: toolPath})
			if err != nil {
				t.Fatalf("private report identity should use raw fallback: %v", err)
			}
			if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].Reason != "official_output_unreadable" || !result.ProviderDecisions[1].Succeeded {
				t.Fatalf("private report identity did not take the typed raw fallback: %+v", result.ProviderDecisions)
			}
			privateInputBytes, err := os.ReadFile(recordPath)
			if err != nil {
				t.Fatal(err)
			}
			privateDir := filepath.Dir(strings.TrimSpace(string(privateInputBytes)))
			for _, path := range []string{result.BundlePath, directPerfArtifactByType(result.Artifacts, ArtifactPerfTrace).Path} {
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(strings.ToLower(string(body)), strings.ToLower(privateDir)) || strings.Contains(strings.ToLower(string(body)), strings.ToLower(filepath.ToSlash(privateDir))) {
					t.Fatalf("published output %s leaked private report identity %q:\n%s", path, privateDir, body)
				}
			}
		})
	}
}

func TestReleaseSimpleperfAdapterChildFailureSuppressesPrivateOutputAndFallsBack(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "capture.perf.data")
	if err := os.WriteFile(inputPath, syntheticRawPerfData(), 0o600); err != nil {
		t.Fatal(err)
	}
	toolPath := writeFakeSimpleperfReportTool(t, dir)
	recordPath := filepath.Join(dir, "adapter-input.txt")
	t.Setenv("SIMPLEPERF_INPUT_RECORD", recordPath)
	t.Setenv("SIMPLEPERF_TEST_MODE", "exit-error")

	result, err := ConvertFile(context.Background(), Options{InputPath: inputPath, SimpleperfReportPath: toolPath})
	if err != nil {
		t.Fatalf("child-only failure should retain raw fallback: %v", err)
	}
	if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].Reason != "official_adapter_failed" || !result.ProviderDecisions[1].Succeeded {
		t.Fatalf("child failure did not retain provider fallback provenance: %+v", result.ProviderDecisions)
	}
	privateInputBytes, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	privateDir := filepath.Dir(strings.TrimSpace(string(privateInputBytes)))
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bundle), privateDir) || !strings.Contains(string(bundle), "[simpleperf child output suppressed]") {
		t.Fatalf("child failure privacy disclosure drifted:\n%s", bundle)
	}
}

func TestReleaseSimpleperfAdapterGenerationErrorDominatesChildExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows denies rewriting the held source; native handle tests cover its fail-closed rule")
	}
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "capture.perf.data")
	body := syntheticRawPerfData()
	changed := bytes.Replace(append([]byte(nil), body...), []byte("app\x00"), []byte("bad\x00"), 1)
	if bytes.Equal(body, changed) || len(body) != len(changed) {
		t.Fatal("same-size mutation fixture is invalid")
	}
	if err := os.WriteFile(inputPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	toolPath := writeFakeSimpleperfReportTool(t, dir)
	readyPath := filepath.Join(dir, "child.ready")
	continuePath := filepath.Join(dir, "child.continue")
	recordPath := filepath.Join(dir, "adapter-input.txt")
	t.Setenv("SIMPLEPERF_INPUT_RECORD", recordPath)
	t.Setenv("SIMPLEPERF_TEST_MODE", "barrier-exit")
	t.Setenv("SIMPLEPERF_READY", readyPath)
	t.Setenv("SIMPLEPERF_CONTINUE", continuePath)

	type conversionResult struct {
		result Result
		err    error
	}
	done := make(chan conversionResult, 1)
	var progress []ProgressEvent
	go func() {
		result, err := ConvertFile(context.Background(), Options{
			InputPath: inputPath, SimpleperfReportPath: toolPath,
			Progress: func(event ProgressEvent) { progress = append(progress, event) },
		})
		done <- conversionResult{result: result, err: err}
	}()
	defer func() { _ = os.WriteFile(continuePath, []byte("continue"), 0o600) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Lstat(readyPath); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("simpleperf child did not reach the mutation barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(inputPath, changed, originalInfo.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(inputPath, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(continuePath, []byte("continue"), 0o600); err != nil {
		t.Fatal(err)
	}
	var outcome conversionResult
	select {
	case outcome = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("simpleperf conversion did not leave the child boundary")
	}
	var inputErr *ConversionInputError
	if !errors.As(outcome.err, &inputErr) || inputErr.Code != ConversionInputCodeGenerationChanged || inputErr.Stage != conversionInputStageExternalTool.String() {
		t.Fatalf("generation error did not retain the external-tool verdict: %T %v", outcome.err, outcome.err)
	}
	if !strings.Contains(outcome.err.Error(), "exit status 7") || !reflect.DeepEqual(outcome.result, Result{}) {
		t.Fatalf("generation+child evidence/result drifted: result=%+v err=%v", outcome.result, outcome.err)
	}
	base := traceSidecarBase(inputPath, "")
	for _, path := range []string{base + ".perftrace", base + ".tracebundle.json"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("hard boundary failure retained publication %s: %v", path, err)
		}
	}
	privateInputBytes, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(strings.TrimSpace(string(privateInputBytes))); !os.IsNotExist(err) {
		t.Fatalf("hard boundary failure retained private input: %v", err)
	}
	terminalCount := 0
	for _, event := range progress {
		if event.Stage != "simpleperf_adapter" || (event.Status != ProgressStatusComplete && event.Status != ProgressStatusFailed) {
			continue
		}
		terminalCount++
		if event.Status != ProgressStatusFailed || event.Message != "simpleperf command boundary rejected" {
			t.Fatalf("hard boundary emitted a false adapter terminal: %+v", event)
		}
	}
	if terminalCount != 1 {
		t.Fatalf("hard boundary adapter terminal count=%d want=1: %+v", terminalCount, progress)
	}
}

func TestReleaseSimpleperfPythonGrammarConsumesLeaseInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows child-open evidence is tracked separately")
	}
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "capture.perf.data")
	if err := os.WriteFile(inputPath, syntheticRawPerfData(), 0o600); err != nil {
		t.Fatal(err)
	}
	reportFixture := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(reportFixture, []byte(syntheticSimpleperfReport()), 0o600); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "report_sample.py")
	if err := os.WriteFile(scriptPath, []byte("# adapter placeholder\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(dir, "python-argv.txt")
	pythonPath := filepath.Join(dir, "python-wrapper")
	pythonBody := `#!/bin/sh
script="$1"
shift
in=""
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-i" ]; then shift; in="$1"; fi
  if [ "$1" = "-o" ]; then shift; out="$1"; fi
  shift
done
test "$script" = "$SIMPLEPERF_EXPECTED_SCRIPT" || exit 81
test -n "$in" && test -r "$in" || exit 82
dd if="$in" of=/dev/null bs=64 count=1 2>/dev/null || exit 83
printf '%s\n%s\n%s\n' "$script" "$in" "$out" > "$SIMPLEPERF_PYTHON_RECORD"
cp "$SIMPLEPERF_REPORT_FIXTURE" "$out"
`
	if err := os.WriteFile(pythonPath, []byte(pythonBody), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMPLEPERF_EXPECTED_SCRIPT", scriptPath)
	t.Setenv("SIMPLEPERF_PYTHON_RECORD", recordPath)
	t.Setenv("SIMPLEPERF_REPORT_FIXTURE", reportFixture)

	result, err := ConvertFile(context.Background(), Options{
		InputPath: inputPath, SimpleperfReportPath: scriptPath, SimpleperfPythonPath: pythonPath,
	})
	if err != nil {
		t.Fatalf("python simpleperf adapter: %v", err)
	}
	record, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(string(record)), "\n")
	if len(parts) != 3 || parts[0] != scriptPath || sameConversionCanonicalPath(parts[1], inputPath) || filepath.Base(parts[1]) != "simpleperf_input.perf.data" || filepath.Base(parts[2]) != "report_sample.txt" {
		t.Fatalf("python argv grammar drifted: %q", parts)
	}
	if _, err := os.Lstat(parts[1]); !os.IsNotExist(err) {
		t.Fatalf("python input snapshot survived cleanup: %v", err)
	}
	if directPerfArtifactByType(result.Artifacts, ArtifactPerfTrace).Path == "" {
		t.Fatalf("python adapter lost perftrace artifact: %+v", result.Artifacts)
	}
}

func TestSimpleperfPrivatePathScannerCoversEveryPublishedStringField(t *testing.T) {
	privatePath := filepath.Join("private", "adapter", "input.perf.data")
	setters := []struct {
		name string
		set  func(*simpleperfSample)
	}{
		{name: "comm", set: func(sample *simpleperfSample) { sample.Comm = privatePath }},
		{name: "event", set: func(sample *simpleperfSample) { sample.Event = privatePath }},
		{name: "leaf-ip", set: func(sample *simpleperfSample) { sample.Leaf.IP = privatePath }},
		{name: "leaf-symbol", set: func(sample *simpleperfSample) { sample.Leaf.Symbol = privatePath }},
		{name: "leaf-dso", set: func(sample *simpleperfSample) { sample.Leaf.DSO = privatePath }},
		{name: "frame-ip", set: func(sample *simpleperfSample) { sample.CallFrames = []simpleperfFrame{{IP: privatePath}} }},
		{name: "frame-symbol", set: func(sample *simpleperfSample) { sample.CallFrames = []simpleperfFrame{{Symbol: privatePath}} }},
		{name: "frame-dso", set: func(sample *simpleperfSample) { sample.CallFrames = []simpleperfFrame{{DSO: privatePath}} }},
	}
	for _, test := range setters {
		t.Run(test.name, func(t *testing.T) {
			sample := simpleperfSample{Comm: "app", Event: "cpu-cycles", Leaf: simpleperfFrame{IP: "1", Symbol: "foo", DSO: "lib.so"}}
			test.set(&sample)
			if !simpleperfSamplesContainPrivatePath([]simpleperfSample{sample}, capturePrivatePathIdentity(filepath.Dir(privatePath))) {
				t.Fatalf("private path in %s was not rejected: %+v", test.name, sample)
			}
		})
	}
	if simpleperfSamplesContainPrivatePath([]simpleperfSample{{Comm: "public-app", Event: "cpu-cycles"}}, capturePrivatePathIdentity(filepath.Dir(privatePath))) {
		t.Fatal("private path scanner rejected an unrelated sample")
	}
	if !simpleperfSamplesContainPrivatePath([]simpleperfSample{{Comm: "SIMPLEPERF_INPUT.PERF.DATA"}}, privatePathIdentity{}) {
		t.Fatal("private path scanner accepted a case-variant snapshot basename")
	}
}

func TestPrivatePathIdentityRedactionIsByteSafeAndCaseInsensitive(t *testing.T) {
	invalidUTF8Path := string([]byte{'/', 't', 'm', 'p', '/', 0xff, 'P', 'r', 'i', 'v', 'a', 't', 'e'})
	identity := capturePrivatePathIdentity(invalidUTF8Path)
	if len(identity.prefixes) == 0 {
		t.Fatal("invalid-UTF-8 Unix path lost its byte identity")
	}
	message := "failure at " + invalidUTF8Path
	redacted := message
	for _, prefix := range identity.prefixes {
		redacted = replaceAllASCIIPathFold(redacted, prefix, "<private>")
	}
	if strings.Contains(redacted, invalidUTF8Path) || !strings.Contains(redacted, "<private>") {
		t.Fatalf("invalid-UTF-8 path redaction drifted: %q", redacted)
	}
	windowsPath := `C:\Temp\Private\Input.perf.data`
	if got := replaceAllASCIIPathFold(`failed C:\TEMP\PRIVATE\INPUT.PERF.DATA`, windowsPath, "<private>"); got != "failed <private>" {
		t.Fatalf("Windows case-variant path was not redacted: %q", got)
	}
}

func TestConvertFileUsesReportSampleNextToConfiguredSimpleperfReportLib(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "capture.perfdata")
	if err := os.WriteFile(perfData, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}
	libPath := filepath.Join(dir, "simpleperf_report_lib.py")
	if err := os.WriteFile(libPath, []byte("# official library placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFakeSimpleperfReportTool(t, dir)

	result, err := ConvertFile(context.Background(), Options{InputPath: perfData, SimpleperfReportPath: libPath})
	if err != nil {
		t.Fatalf("convert direct perf data through sibling report_sample: %v", err)
	}
	var perfTrace Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			perfTrace = artifact
			break
		}
	}
	if perfTrace.Path == "" {
		t.Fatalf("missing perftrace artifact: %+v", result.Artifacts)
	}
	if perfTrace.Perf == nil || !strings.Contains(strings.Join(perfTrace.Perf.Caveats, " "), "report_sample.py next to configured") {
		t.Fatalf("provider source should explain report_lib sibling resolution: %+v", perfTrace.Perf)
	}
}

func TestConvertSimpleperfProtoFileToPerfTraceRoundTripsThroughTraceQuery(t *testing.T) {
	dir := t.TempDir()
	protoPath := filepath.Join(dir, "simpleperf.pb")
	outPath := filepath.Join(dir, "simpleperf.perftrace")
	if err := os.WriteFile(protoPath, syntheticSimpleperfProtoStream(true, true), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ConvertSimpleperfProtoFileToPerfTrace(context.Background(), protoPath, outPath); err != nil {
		t.Fatalf("convert simpleperf proto: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"perf_sample:",
		"cpu=-1",
		"cpu_known=false",
		"pid=1234",
		"tid=5678",
		"sample_weight=99",
		`event="cpu-cycles"`,
		`symbol="Foo::bar"`,
		`dso="/system/lib64/libfoo.so"`,
		`callchain="main@/system/lib64/libfoo.so;Foo::bar@/system/lib64/libfoo.so"`,
		"source=simpleperf_report_proto",
		"sample_kind=on_cpu",
		"symbolization_status=symbolized",
		"clock=simpleperf_record",
		"clock_confidence=assumed",
		"callchain_status=symbolized",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("perftrace missing %q:\n%s", want, string(body))
		}
	}

	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("parse perftrace: %v", err)
	}
	if len(idx.Events) != 1 {
		t.Fatalf("events: got %d want 1", len(idx.Events))
	}
	ev := idx.Events[0]
	if ev.PerfFields.Source != "simpleperf_report_proto" || ev.PerfFields.SampleKind != "on_cpu" || ev.PerfFields.CPUKnown == nil || *ev.PerfFields.CPUKnown {
		t.Fatalf("bad simpleperf proto quality fields: %+v", ev)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 1, TimeEnd: 2})
	if stats.PerfSamples == nil || stats.PerfSamples.Quality == nil || len(stats.PerfSamples.Quality.SampleKinds) == 0 || stats.PerfSamples.Quality.SampleKinds[0].Value != "on_cpu" {
		t.Fatalf("sample_kind should reach perf quality: %+v", stats.PerfSamples)
	}
}

func TestConvertSimpleperfProtoFileMarksOffCPUSampleKind(t *testing.T) {
	dir := t.TempDir()
	protoPath := filepath.Join(dir, "simpleperf-offcpu.pb")
	outPath := filepath.Join(dir, "simpleperf-offcpu.perftrace")
	if err := os.WriteFile(protoPath, syntheticSimpleperfProtoStream(true, false), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ConvertSimpleperfProtoFileToPerfTrace(context.Background(), protoPath, outPath); err != nil {
		t.Fatalf("convert simpleperf offcpu proto: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("parse perftrace: %v", err)
	}
	if len(idx.Events) != 1 || idx.Events[0].PerfFields.SampleKind != "off_cpu" {
		t.Fatalf("offcpu context switch should mark sample_kind=off_cpu: %+v", idx.Events)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 1, TimeEnd: 2})
	if stats.PerfSamples == nil || stats.PerfSamples.Quality == nil || !strings.Contains(strings.Join(stats.PerfSamples.Quality.Caveats, "\n"), "off_cpu") {
		t.Fatalf("off_cpu caveat should reach perf quality: %+v", stats.PerfSamples)
	}
}

func TestConvertFileRunsSimpleperfProtoProviderForDirectReportProtoByContent(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "capture.bin")
	if err := os.WriteFile(perfData, syntheticSimpleperfProtoStream(false, false), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: perfData})
	if err != nil {
		t.Fatalf("convert direct simpleperf report proto: %v", err)
	}
	var perfTrace Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			perfTrace = artifact
			break
		}
	}
	if perfTrace.Path == "" {
		t.Fatalf("missing perftrace artifact: %+v", result.Artifacts)
	}
	if perfTrace.Perf == nil || perfTrace.Perf.ProviderName != perfProviderNameSimpleperfProto || perfTrace.Perf.InputFormat != string(perfInputSimpleperfReportProto) {
		t.Fatalf("simpleperf report proto input format should reach capability: %+v", perfTrace.Perf)
	}
	if len(result.ProviderDecisions) != 1 || result.ProviderDecisions[0].ProviderName != perfProviderNameSimpleperfProto || result.ProviderDecisions[0].InputFormat != string(perfInputSimpleperfReportProto) || !result.ProviderDecisions[0].Succeeded {
		t.Fatalf("simpleperf proto input should reach provider decision: %+v", result.ProviderDecisions)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"perf_capability"`, `"provider_name": "android_simpleperf_report_proto"`, `"input_format": "simpleperf_report_sample_proto"`, `"trace_query_ready": true`, `"provider_decisions"`, `"perf_clock_alignments"`} {
		if !strings.Contains(string(bundle), want) {
			t.Fatalf("bundle missing %q:\n%s", want, string(bundle))
		}
	}
}

func TestConvertFileDoesNotTreatArbitraryInputAsPerfBecauseSimpleperfConfigured(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "not-perf.bin")
	if err := os.WriteFile(input, []byte("ANDROID-PERF-DATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	toolPath := filepath.Join(dir, "report_sample")
	if err := os.WriteFile(toolPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := ConvertFile(context.Background(), Options{InputPath: input, SimpleperfReportPath: toolPath, TraceEngine: traceEngineBuiltin})
	if err == nil || strings.Contains(err.Error(), "simpleperf") {
		t.Fatalf("arbitrary non-perf input should fall through to hitrace validation, got %v", err)
	}
}

func writeFakeSimpleperfReportTool(t *testing.T, dir string) string {
	t.Helper()
	reportFixture := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(reportFixture, []byte(syntheticSimpleperfReport()), 0o644); err != nil {
		t.Fatal(err)
	}
	toolPath := filepath.Join(dir, "report_sample")
	script := `#!/bin/sh
in=""
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-i" ]; then
    shift
    in="$1"
  fi
  if [ "$1" = "-o" ]; then
    shift
    out="$1"
  fi
  shift
done
test -n "$in" && test -r "$in" || exit 91
dd if="$in" of=/dev/null bs=64 count=1 2>/dev/null || exit 92
if [ -n "$SIMPLEPERF_EXPECTED_INPUT" ]; then
  cmp "$in" "$SIMPLEPERF_EXPECTED_INPUT" || exit 93
fi
if [ -n "$SIMPLEPERF_INPUT_RECORD" ]; then
  printf '%s\n' "$in" > "$SIMPLEPERF_INPUT_RECORD"
fi
if [ "$SIMPLEPERF_TEST_MODE" = "exit-error" ]; then
  printf '%s\n' "$in" >&2
  exit 7
fi
if [ "$SIMPLEPERF_TEST_MODE" = "barrier-exit" ]; then
  : > "$SIMPLEPERF_READY"
  while [ ! -f "$SIMPLEPERF_CONTINUE" ]; do
    sleep 0.01
  done
  printf '%s\n' "$in" >&2
  exit 7
fi
if [ "$SIMPLEPERF_TEST_MODE" = "private-report" ] || [ "$SIMPLEPERF_TEST_MODE" = "canonical-private-report" ]; then
  published="$in"
  if [ "$SIMPLEPERF_TEST_MODE" = "canonical-private-report" ]; then
    published="$(cd "$(dirname "$in")" && pwd -P)/$(basename "$in")"
  fi
  printf 'Render Thread\t1234/5678 [005] 928.081774: 10000 cpu-cycles:\n' > "$out"
  printf '\t            1234 Foo::bar (%s)\n' "$published" >> "$out"
  exit 0
fi
cp "$SIMPLEPERF_REPORT_FIXTURE" "$out"
`
	if err := os.WriteFile(toolPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMPLEPERF_REPORT_FIXTURE", reportFixture)
	return toolPath
}

func syntheticSimpleperfReport() string {
	return strings.Join([]string{
		"# ========",
		"# cmdline : simpleperf record -g",
		"Render Thread\t1234/5678 [005] 928.081774: 10000 cpu-cycles:",
		"\t            1234 Foo::bar (/system/lib64/libfoo.so)",
		"\t            2222 A (/system/lib64/libfoo.so)",
		"\t            1111 main (/system/lib64/libfoo.so)",
		"",
	}, "\n")
}

func syntheticSimpleperfProtoStream(traceOffCPU bool, switchOn bool) []byte {
	var b bytes.Buffer
	b.WriteString(simpleperfProtoMagic)
	var version [2]byte
	binary.LittleEndian.PutUint16(version[:], simpleperfProtoVersion)
	b.Write(version[:])
	metaParts := [][]byte{protoBytes(1, []byte("cpu-cycles"))}
	if traceOffCPU {
		metaParts = append(metaParts, protoVarint(6, 1))
	}
	writeSimpleperfProtoRecord(&b, protoMessage(5, metaParts...))
	writeSimpleperfProtoRecord(&b, protoMessage(3,
		protoVarint(1, 0),
		protoBytes(2, []byte("/system/lib64/libfoo.so")),
		protoBytes(3, []byte("main")),
		protoBytes(3, []byte("Foo::bar")),
	))
	writeSimpleperfProtoRecord(&b, protoMessage(4,
		protoVarint(1, 5678),
		protoVarint(2, 1234),
		protoBytes(3, []byte("Render Thread")),
	))
	if traceOffCPU {
		switchValue := uint64(0)
		if switchOn {
			switchValue = 1
		}
		writeSimpleperfProtoRecord(&b, protoMessage(6,
			protoVarint(1, switchValue),
			protoVarint(2, 1_234_566_000),
			protoVarint(3, 5678),
		))
	}
	leaf := protoMessage(3,
		protoVarint(1, 0x1234),
		protoVarint(2, 0),
		protoVarint(3, 1),
	)
	root := protoMessage(3,
		protoVarint(1, 0x1111),
		protoVarint(2, 0),
		protoVarint(3, 0),
	)
	writeSimpleperfProtoRecord(&b, protoMessage(1,
		protoVarint(1, 1_234_567_000),
		protoVarint(2, 5678),
		leaf,
		root,
		protoVarint(4, 99),
		protoVarint(5, 0),
	))
	var zero [4]byte
	b.Write(zero[:])
	return b.Bytes()
}

func writeSimpleperfProtoRecord(b *bytes.Buffer, record []byte) {
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(record)))
	b.Write(size[:])
	b.Write(record)
}
