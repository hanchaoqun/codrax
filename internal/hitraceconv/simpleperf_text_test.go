package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		"period=10000",
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
	if ev.Type != tracequery.EventPerfSample || ev.CPU != 5 || ev.PerfPID != 1234 || ev.PerfTID != 5678 || ev.PerfPeriod != 10000 {
		t.Fatalf("bad perf sample fields: %+v", ev)
	}
	if ev.PerfEvent != "cpu-cycles" || ev.PerfSymbol != "Foo::bar" || ev.PerfDSO != "/system/lib64/libfoo.so" {
		t.Fatalf("bad perf symbol fields: %+v", ev)
	}
	if ev.PerfCPUKnown == nil || !*ev.PerfCPUKnown || ev.PerfSymbolizationStatus != "symbolized" || ev.PerfClockConfidence != "assumed" || ev.PerfCallchainStatus != "symbolized" {
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
	if len(idx.Events) != 1 || idx.Events[0].PerfSymbol != "Foo::bar" {
		t.Fatalf("generated perftrace did not round-trip: %+v", idx.Events)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type": "perftrace"`, perfTrace.Path, `"perf_capability"`, `"provider_kind": "official_android"`, `"input_format": "linux_perf_data"`, `"provider_decisions"`, `"provider_name": "android_simpleperf_report_sample"`, `"succeeded": true`, `"perf_clock_alignments"`, `"confidence": "assumed"`} {
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
		"period=99",
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
	if ev.PerfSource != "simpleperf_report_proto" || ev.PerfSampleKind != "on_cpu" || ev.PerfCPUKnown == nil || *ev.PerfCPUKnown {
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
	if len(idx.Events) != 1 || idx.Events[0].PerfSampleKind != "off_cpu" {
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

	_, err := ConvertFile(context.Background(), Options{InputPath: input, SimpleperfReportPath: toolPath})
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
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    out="$1"
  fi
  shift
done
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
