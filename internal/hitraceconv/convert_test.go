package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestDefaultOutputPathAppendsSystraceSuffix(t *testing.T) {
	if got, want := DefaultOutputPath("foo.htrace.bin"), "foo.htrace.bin.systrace"; got != want {
		t.Fatalf("default output: got %q, want %q", got, want)
	}
}

func TestConvertFileWritesTextSystraceAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.htrace.bin")
	if err := os.WriteFile(input, syntheticBinaryHitrace(t), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: input})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.OutputPath != input+".systrace" {
		t.Fatalf("output path: got %q", result.OutputPath)
	}
	if result.EventsWritten != 1 {
		t.Fatalf("events written: got %d", result.EventsWritten)
	}
	body, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"sched_wakeup: comm=com.tencent.mm pid=36379 prio=53 target_cpu=000",
		"2942.124416",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("converted trace missing %q:\n%s", want, text)
		}
	}

	idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
	if err != nil {
		t.Fatalf("tracequery parse converted output: %v", err)
	}
	if len(idx.Events) != 1 || idx.Events[0].WakeePID != 36379 {
		t.Fatalf("converted output did not round-trip through tracequery: %+v", idx.Events)
	}

	if _, err := ConvertFile(context.Background(), Options{InputPath: input}); err == nil ||
		!strings.Contains(err.Error(), "output file already exists") {
		t.Fatalf("expected existing output refusal, got %v", err)
	}
}

func TestConvertFilePreservesNoPerfSysBinaryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "donghu-no-perf.sys")
	output := filepath.Join(dir, "donghu-no-perf.systrace")
	if err := os.WriteFile(input, syntheticBinaryHitrace(t), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{
		InputPath:   input,
		OutputPath:  output,
		TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert sys binary trace: %v", err)
	}
	if result.OutputPath != output || result.EventsWritten != 1 {
		t.Fatalf("sys binary conversion lost systrace output: %+v", result)
	}
	if !hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinSys, true) {
		t.Fatalf("sys binary conversion should carry builtin sys provider provenance: %+v", result.TraceDecisions)
	}
	if containsString(result.Caveats, "archival") || containsString(result.Caveats, "will be removed") {
		t.Fatalf("sys binary conversion should not tell users the capability is archival: %+v", result.Caveats)
	}
	idx, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatalf("tracequery parse sys binary output: %v", err)
	}
	if len(idx.Events) != 1 || idx.Events[0].WakeePID != 36379 {
		t.Fatalf("sys binary output did not round-trip through tracequery: %+v", idx.Events)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"provider_kind": "builtin_sys_binary"`, `"provider_name": "codrax_builtin_sys_binary"`, `"trace_query_ready": true`} {
		if !strings.Contains(string(bundle), want) {
			t.Fatalf("sys binary bundle missing %q:\n%s", want, bundle)
		}
	}
}

func TestConvertFileExtractsStandaloneHiperfDataAndBundle(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample-with-perf.htrace")
	perfPayload := []byte("PERF-DATA-PAYLOAD")
	body := append(syntheticBinaryHitrace(t), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", perfPayload)...)
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.OutputPath != output || result.BundlePath != filepath.Join(dir, "out.tracebundle.json") {
		t.Fatalf("unexpected output/bundle paths: %+v", result)
	}
	var perf Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfData {
			perf = artifact
			break
		}
	}
	if perf.Path == "" || perf.PluginName != "hiperf-plugin" || perf.DataType != profilerDataTypeHiperf {
		t.Fatalf("missing perf_data artifact: %+v", result.Artifacts)
	}
	gotPayload, err := os.ReadFile(perf.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotPayload, perfPayload) {
		t.Fatalf("perf payload mismatch: got %q want %q", gotPayload, perfPayload)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type": "perf_data"`, `"plugin_name": "hiperf-plugin"`, `"systrace": "` + output + `"`} {
		if !strings.Contains(string(bundle), want) {
			t.Fatalf("bundle missing %q:\n%s", want, bundle)
		}
	}
}

func TestConvertFileReturnsStandalonePerfArtifactWithoutSystraceContainer(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "perf-only.htrace")
	perfPayload := []byte("ONLY-PERF-DATA")
	if err := os.WriteFile(input, syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", perfPayload), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: input})
	if err != nil {
		t.Fatalf("convert should return extracted sidecar instead of failing outright: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("standalone-only input should not claim systrace output: %+v", result)
	}
	if result.BundlePath == "" {
		t.Fatalf("standalone-only conversion should emit a bundle: %+v", result)
	}
	var perf Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfData {
			perf = artifact
			break
		}
	}
	if perf.Path == "" {
		t.Fatalf("missing perf_data artifact: %+v", result.Artifacts)
	}
	gotPayload, err := os.ReadFile(perf.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotPayload, perfPayload) {
		t.Fatalf("perf payload mismatch: got %q want %q", gotPayload, perfPayload)
	}
	if !containsString(result.Caveats, "systrace output was not produced") {
		t.Fatalf("partial conversion should carry caveat: %+v", result.Caveats)
	}
}

func TestConvertFileStandaloneRawFallbackProducesTextPerftraceAndRawPerfDataSidecar(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "perf-only.htrace")
	if err := os.WriteFile(input, syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.02", syntheticRawPerfData()), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: input})
	if err != nil {
		t.Fatalf("convert standalone raw perf fallback: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("perf-only standalone should not produce systrace: %+v", result)
	}
	var perfData Artifact
	var perfTrace Artifact
	for _, artifact := range result.Artifacts {
		switch artifact.Type {
		case ArtifactPerfData:
			perfData = artifact
		case ArtifactPerfTrace:
			perfTrace = artifact
		}
	}
	if perfData.Path == "" || perfTrace.Path == "" {
		t.Fatalf("expected both raw perf.data and normalized perftrace artifacts: %+v", result.Artifacts)
	}
	rawBody, err := os.ReadFile(perfData.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(rawBody, []byte(perfMagic2)) {
		t.Fatalf("perf_data artifact should preserve binary perf.data sidecar, got prefix %q", testPrefix(rawBody, len(perfMagic2)))
	}
	textBody, err := os.ReadFile(perfTrace.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(textBody, []byte("perf_sample:")) || bytes.HasPrefix(textBody, []byte(perfMagic2)) {
		t.Fatalf("perftrace artifact should be normalized text, got prefix %q", testPrefix(textBody, len(perfMagic2)))
	}
	if perfTrace.Perf == nil || perfTrace.Perf.ProviderKind != perfProviderKindRawFallback {
		t.Fatalf("expected raw fallback perftrace capability: %+v", perfTrace.Perf)
	}
	joined := strings.Join(result.Caveats, "\n")
	if !strings.Contains(joined, "through Codrax raw perf.data fallback") {
		t.Fatalf("summary should name raw fallback provider: %+v", result.Caveats)
	}
	if strings.Contains(joined, "through the official hiperf adapter") {
		t.Fatalf("raw fallback summary must not claim official adapter: %+v", result.Caveats)
	}
}

func TestConvertFileRendersOfficialProfilerTraceFileTextPayload(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler.htrace")
	textTrace := strings.Join([]string{
		"worker-1234  ( 1234) [005] ....     1.234567: sched_wakeup: comm=main pid=5678 prio=53 target_cpu=005",
		"main-5678    ( 5678) [005] ....     1.234890: tracing_mark_write: B|5678|doFrame",
		"Binder:43397_19-23088 (23088) [007] ....     1.235000: sched_wakeup: comm=main pid=5678 prio=53 target_cpu=007",
	}, "\n")
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(textTrace)))
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatalf("convert official profiler trace file: %v", err)
	}
	if result.OutputPath != output || result.EventsWritten != 3 || result.UnknownEventCount != 0 {
		t.Fatalf("bad profiler conversion result: %+v", result)
	}
	if !containsString(result.Caveats, "TraceFileHeader detected") || !containsString(result.Caveats, "extracted 3 systrace text row") {
		t.Fatalf("official profiler caveats should explain format and extraction: %+v", result.Caveats)
	}
	converted, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sched_wakeup: comm=main pid=5678", "tracing_mark_write: B|5678|doFrame"} {
		if !strings.Contains(string(converted), want) {
			t.Fatalf("converted profiler systrace missing %q:\n%s", want, converted)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatalf("tracequery parse profiler output: %v", err)
	}
	if len(idx.Events) != 3 {
		t.Fatalf("profiler output did not round-trip through tracequery: %+v", idx.Events)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"trace_provider_decisions"`, `"provider_name": "codrax_builtin_modern_profiler"`, `"trace_query_ready": true`} {
		if !strings.Contains(string(bundle), want) {
			t.Fatalf("profiler bundle missing trace provider provenance %q:\n%s", want, bundle)
		}
	}
	var meta struct {
		TraceCoverage []TraceDBCoverage `json:"trace_coverage"`
	}
	if err := json.Unmarshal(bundle, &meta); err != nil {
		t.Fatalf("decode tracebundle: %v", err)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_profiler", "plugin:bytrace_plugin", 3) {
		t.Fatalf("profiler bundle missing modern plugin coverage: %+v", meta.TraceCoverage)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_profiler", "__systrace_rows__", 3) {
		t.Fatalf("profiler bundle missing modern sorter coverage: %+v", meta.TraceCoverage)
	}
}

func TestProfilerTextPayloadUsesBoundedSorter(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler-large.htrace")
	textTrace := strings.Join([]string{
		"t-1  ( 1) [001] ....     3.000000: tracing_mark_write: B|1|late",
		"t-1  ( 1) [001] ....     1.000000: tracing_mark_write: B|1|early",
		"t-1  ( 1) [001] ....     2.000000: tracing_mark_write: B|1|middle",
		"t-1  ( 1) [001] ....     4.000000: tracing_mark_write: B|1|last",
		"t-1  ( 1) [001] ....     1.500000: tracing_mark_write: B|1|between",
	}, "\n")
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(textTrace)))
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink("", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	extracted, err := extractProfilerContainerSystraceRows(context.Background(), input, info.Size(), sink)
	if err != nil {
		t.Fatalf("extract profiler rows: %v", err)
	}
	if !extracted.Detected || extracted.TextRows != 5 || sink.stats.SpillChunks == 0 {
		t.Fatalf("expected bounded sorter extraction with spill: extracted=%+v stats=%+v", extracted, sink.stats)
	}
	var out bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &out)
	if err != nil {
		t.Fatalf("write sorted rows: %v", err)
	}
	if stats.RowsWritten != 5 || stats.PeakBufferedRows > 2 {
		t.Fatalf("bad sorter stats: %+v", stats)
	}
	text := out.String()
	early := strings.Index(text, "B|1|early")
	between := strings.Index(text, "B|1|between")
	middle := strings.Index(text, "B|1|middle")
	late := strings.Index(text, "B|1|late")
	if early < 0 || between < early || middle < between || late < middle {
		t.Fatalf("rows not globally sorted:\n%s", text)
	}
}

func TestConvertFileSummarizesOfficialProfilerStructuredFtraceMetadata(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler-structured.htrace")
	body := syntheticProfilerTraceFile(syntheticProfilerPluginDataWithTiming(
		"ftrace-plugin",
		syntheticTracePluginResultMetadata(),
		1,
		12,
		34,
		"1.03",
		250,
	))
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: filepath.Join(dir, "out.systrace")})
	if err != nil {
		t.Fatalf("convert structured profiler trace file: %v", err)
	}
	if result.OutputPath == "" || result.EventsWritten != 10 || result.UnknownEventCount != 0 {
		t.Fatalf("structured ftrace metadata should render supported systrace rows without unknown count: %+v", result)
	}
	if result.BundlePath == "" {
		t.Fatalf("structured ftrace partial result should still emit tracebundle coverage: %+v", result)
	}
	for _, want := range []string{
		"profiler plugin ftrace-plugin metadata: clock_id=MONOTONIC",
		"tv=12.000000034",
		"sample_interval_ms=250",
		"ftrace-plugin structured metadata:",
		"version=trace-plugin-v1",
		"stats_end=1",
		"trace_clock=boot",
		"end_dropped=3",
		"end_overrun=1",
		"end_commit_overrun=2",
		"structured_event_records=10",
		"detail_overwrite=4",
		"event_families=binder:2,block,cpu,f2fs,filemap,irq:2,sched,trace_marker",
		"event_names=",
		"symbols=1",
		"symbol_examples=0x1234=schedule",
		"clock_details=MONOTONIC(time=10.000000020/res=0.000000001)",
	} {
		if !containsString(result.Caveats, want) {
			t.Fatalf("structured profiler caveats missing %q:\n%v", want, result.Caveats)
		}
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		TraceCoverage []TraceDBCoverage `json:"trace_coverage"`
	}
	if err := json.Unmarshal(bundle, &meta); err != nil {
		t.Fatalf("decode tracebundle: %v", err)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_profiler", "plugin:ftrace-plugin", 2) {
		t.Fatalf("structured ftrace plugin coverage should count rendered rows: %+v", meta.TraceCoverage)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_ftrace:sched", "sched_switch", 1) {
		t.Fatalf("structured ftrace coverage should name sched_switch: %+v", meta.TraceCoverage)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_ftrace:block", "block_rq_issue", 1) {
		t.Fatalf("structured ftrace coverage should name block_rq_issue: %+v", meta.TraceCoverage)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_ftrace:binder", "binder_transaction", 1) {
		t.Fatalf("structured ftrace coverage should name binder_transaction: %+v", meta.TraceCoverage)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_ftrace:cpu", "cpu_frequency", 1) {
		t.Fatalf("structured ftrace coverage should name cpu_frequency: %+v", meta.TraceCoverage)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_ftrace:f2fs", "f2fs_write_begin", 1) {
		t.Fatalf("structured ftrace coverage should name f2fs_write_begin: %+v", meta.TraceCoverage)
	}
	converted, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sched_switch: prev_comm=prev", "block_rq_issue:", "binder_transaction: transaction=42", "cpu_frequency: state=2200000", "f2fs_write_begin: dev=", "print: B|100|Frame"} {
		if !strings.Contains(string(converted), want) {
			t.Fatalf("structured ftrace output missing %q:\n%s", want, converted)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
	if err != nil {
		t.Fatalf("tracequery parse structured ftrace output: %v", err)
	}
	if len(idx.Events) != 10 {
		t.Fatalf("structured ftrace output did not round-trip through tracequery: %+v", idx.Events)
	}
}

func TestConvertFileHandlesSessionJSONPackageWithPerfSidecar(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "hiprofiler_data.htrace")
	perfPayload := append([]byte("PERF-DATA-PAYLOAD"), bytes.Repeat([]byte{0xa5}, 9*1024*1024)...)
	var body bytes.Buffer
	body.Write([]byte("PKGHEAD0"))
	body.WriteString("SessionJSON-")
	body.WriteString(`{"plugins":["bytrace_plugin","hiperf-plugin"]}`)
	body.WriteByte('\n')
	body.WriteString("app-1000     ( 1000) [003] ....  1858.767865: tracing_mark_write: B|11029|bindApplication\n")
	body.Write(syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.02", perfPayload))
	if err := os.WriteFile(input, body.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "session.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatalf("convert SessionJSON package: %v", err)
	}
	if result.OutputPath != output || result.EventsWritten != 1 || result.BundlePath == "" {
		t.Fatalf("session package should produce systrace and bundle: %+v", result)
	}
	if !containsString(result.Caveats, "SessionJSON- detected") || containsString(result.Caveats, "invalid segment type") {
		t.Fatalf("session package should not surface sys binary parser error: %+v", result.Caveats)
	}
	var perf Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfData {
			perf = artifact
			break
		}
	}
	if perf.Path == "" || perf.PluginName != "hiperf-plugin" || perf.PluginVersion != "1.02" {
		t.Fatalf("session package should preserve HIPERF_DATA sidecar: %+v", result.Artifacts)
	}
	converted, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(converted), "bindApplication") {
		t.Fatalf("session systrace missing embedded text row:\n%s", converted)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		TraceCoverage []TraceDBCoverage `json:"trace_coverage"`
	}
	if err := json.Unmarshal(bundle, &meta); err != nil {
		t.Fatalf("decode tracebundle: %v", err)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_profiler", "session:SessionJSON", 1) {
		t.Fatalf("session bundle missing modern coverage: %+v", meta.TraceCoverage)
	}
}

func TestPerfClockAlignmentsForArtifactsCoversConfidenceStates(t *testing.T) {
	alignments := perfClockAlignmentsForArtifacts([]Artifact{
		{
			Type: ArtifactPerfTrace,
			Path: "assumed.perftrace",
			Perf: &PerfArtifactCapability{
				ProviderName:  "assumed_provider",
				TimeDomain:    "perf_time_ns",
				TimeAlignment: "assumed",
			},
		},
		{
			Type: ArtifactPerfTrace,
			Path: "calibrated.perftrace",
			Perf: &PerfArtifactCapability{
				ProviderName:  "calibrated_provider",
				TimeDomain:    "perf_time_ns",
				TimeAlignment: "calibrated",
			},
		},
		{
			Type: ArtifactPerfTrace,
			Path: "unknown.perftrace",
			Perf: &PerfArtifactCapability{
				ProviderName: "unknown_provider",
				TimeDomain:   "perf_time_ns",
			},
		},
		{Type: ArtifactSystrace, Path: "ignored.systrace"},
	})
	if len(alignments) != 3 {
		t.Fatalf("alignments = %+v", alignments)
	}
	if alignments[0].Confidence != "assumed" || alignments[0].Calibrated || len(alignments[0].Caveats) == 0 {
		t.Fatalf("assumed alignment should be uncalibrated with caveat: %+v", alignments[0])
	}
	if alignments[1].Confidence != "calibrated" || !alignments[1].Calibrated || len(alignments[1].Caveats) != 0 {
		t.Fatalf("calibrated alignment should be marked calibrated without caveat: %+v", alignments[1])
	}
	if alignments[2].Confidence != "unknown" || alignments[2].Calibrated || len(alignments[2].Caveats) == 0 {
		t.Fatalf("unknown alignment should be uncalibrated with caveat: %+v", alignments[2])
	}
}

func TestConvertFileSkipsMissingFormatRows(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "missing-format.htrace")
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("36379 com.tencent.mm\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPageForEventID(99))
	if err := os.WriteFile(input, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.EventsWritten != 0 || result.MissingFormatCount != 1 || result.UnknownEventCount != 0 {
		t.Fatalf("missing/unknown counts: %+v", result)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "unknown_event") || strings.Contains(text, "payload_hex") || strings.Contains(text, "raw_event=unparsed") {
		t.Fatalf("missing-format rows must not be written into official-compatible systrace output:\n%s", text)
	}
	if len(result.Caveats) == 0 || !strings.Contains(result.Caveats[0], "skipped") {
		t.Fatalf("missing-format skip should be surfaced as caveat: %+v", result.Caveats)
	}
}

func TestConvertFileWritesHeaderOnlyRowsWithoutOfficialRenderer(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "unsupported-format.htrace")
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticUnsupportedEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("36379 com.tencent.mm\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPageForEventID(20))
	if err := os.WriteFile(input, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.EventsWritten != 1 || result.MissingFormatCount != 0 || result.UnknownEventCount != 1 {
		t.Fatalf("unsupported row counts: %+v", result)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "vendor_numeric") || strings.Contains(text, "raw_event=unparsed") || strings.Contains(text, "foo=") {
		t.Fatalf("unsupported known-format rows must be header-only, not generically rendered:\n%s", text)
	}
	if !strings.Contains(text, "2942.124416:") {
		t.Fatalf("unsupported known-format row should preserve official-style header and timestamp:\n%s", text)
	}
	if len(result.Caveats) == 0 || !strings.Contains(result.Caveats[0], "header-only") {
		t.Fatalf("unsupported renderer skip should be surfaced as caveat: %+v", result.Caveats)
	}
}

func TestRenderHarmonySchedSwitchKeepsNextInfoAndCGroup(t *testing.T) {
	format := eventFormat{
		ID:   10,
		Name: "sched_switch",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "pname[16]", Offset: 8, Size: 16},
			{Name: "prev_tid", Offset: 24, Size: 4, Signed: true},
			{Name: "pprio", Offset: 28, Size: 4, Signed: true},
			{Name: "pstate", Offset: 32, Size: 4},
			{Name: "nname[16]", Offset: 36, Size: 16},
			{Name: "next_tid", Offset: 52, Size: 4, Signed: true},
			{Name: "nprio", Offset: 56, Size: 4, Signed: true},
			{Name: "ninfo[8]", Offset: 60, Size: 8},
			{Name: "cg[16]", Offset: 68, Size: 16},
		},
	}
	content := make([]byte, 84)
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	copy(content[8:24], []byte("app"))
	binary.LittleEndian.PutUint32(content[24:28], uint32(100))
	binary.LittleEndian.PutUint32(content[28:32], uint32(53))
	binary.LittleEndian.PutUint32(content[32:36], uint32(1))
	copy(content[36:52], []byte("worker"))
	binary.LittleEndian.PutUint32(content[52:56], uint32(200))
	binary.LittleEndian.PutUint32(content[56:60], uint32(80))
	binary.LittleEndian.PutUint32(content[60:64], uint32(0x0000000f))
	remaining := uint32(5) | uint32(2<<10) | uint32(1<<12) | uint32(3<<13) | uint32(17<<16)
	binary.LittleEndian.PutUint32(content[64:68], remaining)
	copy(content[68:84], []byte("top-app"))

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "app"}, tgids: map[int]int{100: 100}}, 1_234_567_000, 2, format, content)
	if !known {
		t.Fatalf("sched_switch should be known: %s", line)
	}
	for _, want := range []string{"next_info=f,10,2,1,3", "cg=top-app"} {
		if !strings.Contains(line, want) {
			t.Fatalf("rendered sched_switch missing %q:\n%s", want, line)
		}
	}
}

func TestRenderHarmonySchedSwitchNinfoIncludesCGIDWhenNoCGroup(t *testing.T) {
	format := eventFormat{
		ID:   11,
		Name: "sched_switch",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "pname[16]", Offset: 8, Size: 16},
			{Name: "prev_tid", Offset: 24, Size: 4, Signed: true},
			{Name: "pprio", Offset: 28, Size: 4, Signed: true},
			{Name: "pstate", Offset: 32, Size: 4},
			{Name: "nname[16]", Offset: 36, Size: 16},
			{Name: "next_tid", Offset: 52, Size: 4, Signed: true},
			{Name: "nprio", Offset: 56, Size: 4, Signed: true},
			{Name: "ninfo[8]", Offset: 60, Size: 8},
		},
	}
	content := make([]byte, 68)
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	copy(content[8:24], []byte("app"))
	binary.LittleEndian.PutUint32(content[24:28], uint32(100))
	binary.LittleEndian.PutUint32(content[28:32], uint32(53))
	copy(content[36:52], []byte("worker"))
	binary.LittleEndian.PutUint32(content[52:56], uint32(200))
	binary.LittleEndian.PutUint32(content[56:60], uint32(80))
	binary.LittleEndian.PutUint32(content[60:64], uint32(0x0000000f))
	remaining := uint32(5) | uint32(2<<10) | uint32(1<<12) | uint32(3<<13) | uint32(17<<16)
	binary.LittleEndian.PutUint32(content[64:68], remaining)

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "app"}, tgids: map[int]int{100: 100}}, 1_234_567_000, 2, format, content)
	if !known || !strings.Contains(line, "next_info=f,10,2,1,3,17") {
		t.Fatalf("ninfo with cgid not rendered: known=%v line=%s", known, line)
	}
}

func TestRenderMMFilemapPageCacheUsesNumericFields(t *testing.T) {
	format := eventFormat{
		ID:   30,
		Name: "mm_filemap_add_to_page_cache",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "s_dev", Offset: 8, Size: 8},
			{Name: "i_ino", Offset: 16, Size: 8},
			{Name: "index", Offset: 24, Size: 8},
			{Name: "pfn", Offset: 32, Size: 8},
			{Name: "pg", Offset: 40, Size: 8},
		},
	}
	content := make([]byte, 48)
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	binary.LittleEndian.PutUint64(content[8:16], uint64((12<<20)|48))
	binary.LittleEndian.PutUint64(content[16:24], uint64(0x60ffe))
	binary.LittleEndian.PutUint64(content[24:32], uint64(42))
	binary.LittleEndian.PutUint64(content[32:40], uint64(3062260))
	binary.LittleEndian.PutUint64(content[40:48], uint64(0))

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "worker"}, tgids: map[int]int{100: 100}}, 2_000_000_000, 1, format, content)
	for _, want := range []string{"dev 12:48", "ino 0x60ffe", "page=0x0", "pfn=3062260", "ofs=172032"} {
		if !known || !strings.Contains(line, want) {
			t.Fatalf("mm_filemap page cache missing %q: known=%v line=%s", want, known, line)
		}
	}
}

func TestGenericIntegerFieldsDoNotBecomePrintableStrings(t *testing.T) {
	format := eventFormat{
		ID:   31,
		Name: "vendor_numeric",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "pfn", Offset: 8, Size: 4},
		},
	}
	content := make([]byte, 12)
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	copy(content[8:12], []byte("ABCD"))

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "worker"}, tgids: map[int]int{100: 100}}, 2_000_000_000, 1, format, content)
	if known || strings.Contains(line, "pfn=ABCD") || !strings.Contains(line, "pfn=1145258561") {
		t.Fatalf("generic integer field rendered unsafely: known=%v line=%s", known, line)
	}
}

func TestRenderDataLocStrings(t *testing.T) {
	format := eventFormat{
		ID:   20,
		Name: "tracing_mark_write",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "__data_loc char[]", Name: "buf", Offset: 8, Size: 4},
		},
	}
	payload := []byte("B|100|RenderFrame\x00")
	content := make([]byte, 12+len(payload))
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	binary.LittleEndian.PutUint32(content[8:12], uint32((len(payload)<<16)|12))
	copy(content[12:], payload)

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "render"}, tgids: map[int]int{100: 100}}, 2_000_000_000, 1, format, content)
	if !known || !strings.Contains(line, "tracing_mark_write: B|100|RenderFrame") {
		t.Fatalf("data_loc trace payload not rendered: known=%v line=%s", known, line)
	}
}

func TestGenericDataLocStringIsPreserved(t *testing.T) {
	format := eventFormat{
		ID:   21,
		Name: "vendor_dynamic",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "__data_loc char[]", Name: "message", Offset: 8, Size: 4},
		},
	}
	payload := []byte("alpha=1 beta=needle\x00")
	content := make([]byte, 12+len(payload))
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	binary.LittleEndian.PutUint32(content[8:12], uint32((len(payload)<<16)|12))
	copy(content[12:], payload)

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "worker"}, tgids: map[int]int{100: 100}}, 2_000_000_000, 1, format, content)
	if known || !strings.Contains(line, "vendor_dynamic: message=alpha=1 beta=needle") {
		t.Fatalf("generic data_loc payload not preserved: known=%v line=%s", known, line)
	}
}

func TestOpenHarmonyPrintFmtCoverageManifest(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "openharmony_print_fmt_coverage.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 87 {
		t.Fatalf("coverage manifest should contain header + 86 OpenHarmony PRINT_FMT rows, got %d", len(lines))
	}
	seen := map[string]string{}
	for _, line := range lines[1:] {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("bad manifest row %q", line)
		}
		if parts[1] != "strong" {
			t.Fatalf("converter support for %s should be strong after official-format parity work, got %q", parts[0], parts[1])
		}
		seen[parts[0]] = parts[2]
		if parts[1] == "" || parts[2] == "" {
			t.Fatalf("coverage row must declare converter and trace_query support: %q", line)
		}
	}
	for name, lane := range map[string]string{
		"PRINT_FMT_SCHED_SWITCH_HM_NINFO_CG": "sched_switch",
		"PRINT_FMT_CPU_FREQUENCY_LIMITS":     "cpu_frequency_limits",
		"PRINT_FMT_UFSHCD_COMMAND":           "storage",
		"PRINT_FMT_EROFS_LOOKUP_START":       "filesystem",
		"PRINT_FMT_TRACING_MARK_WRITE":       "trace_mark",
	} {
		if seen[name] != lane {
			t.Fatalf("coverage manifest lane for %s: got %q want %q", name, seen[name], lane)
		}
	}
}

func TestOfficialSystraceLineFormatUsesCommonFlagsAndIdleName(t *testing.T) {
	format := eventFormat{
		ID:   50,
		Name: "irq_handler_exit",
		Fields: []eventField{
			{Name: "common_type", Offset: 0, Size: 2},
			{Name: "common_flags", Offset: 2, Size: 1},
			{Name: "common_preempt_count", Offset: 3, Size: 1},
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "irq", Offset: 8, Size: 4, Signed: true},
			{Name: "ret", Offset: 12, Size: 4, Signed: true},
		},
	}
	content := make([]byte, 16)
	content[2] = 0x0d // irqs-off + need-resched + hardirq
	content[3] = 2
	binary.LittleEndian.PutUint32(content[8:12], 7)
	binary.LittleEndian.PutUint32(content[12:16], 1)

	line, known := renderEventLine(renderContext{}, 1_000_001_000, 3, format, content)
	for _, want := range []string{"<idle>-0", "(-----)", "[003] dnh2", "1.000001: irq_handler_exit: irq=7 ret=handled"} {
		if !known || !strings.Contains(line, want) {
			t.Fatalf("official systrace line missing %q: known=%v line=%s", want, known, line)
		}
	}
}

func TestOfficialSubsystemRenderersMatchOpenHarmonyShapes(t *testing.T) {
	t.Run("cpu_frequency_limits", func(t *testing.T) {
		format := eventFormat{ID: 60, Name: "cpu_frequency_limits", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "min_freq", Offset: 8, Size: 4},
			{Name: "max_freq", Offset: 12, Size: 4},
			{Name: "cpu_id", Offset: 16, Size: 4},
		}}
		content := make([]byte, 20)
		binary.LittleEndian.PutUint32(content[8:12], 1000000)
		binary.LittleEndian.PutUint32(content[12:16], 2200000)
		binary.LittleEndian.PutUint32(content[16:20], 11)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "min=1000000 max=2200000 cpu_id=11" {
			t.Fatalf("cpu limits: known=%v body=%q", known, body)
		}
	})

	t.Run("i2c_write_dynamic_array", func(t *testing.T) {
		format := eventFormat{ID: 61, Name: "i2c_write", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "adapter_nr", Offset: 8, Size: 4, Signed: true},
			{Name: "msg_nr", Offset: 12, Size: 4},
			{Name: "addr", Offset: 16, Size: 4},
			{Name: "flags", Offset: 20, Size: 4},
			{Name: "len", Offset: 24, Size: 4},
			{Type: "__data_loc u8[]", Name: "__data_loc_buf", Offset: 28, Size: 4},
		}}
		content := make([]byte, 34)
		binary.LittleEndian.PutUint32(content[8:12], 2)
		binary.LittleEndian.PutUint32(content[12:16], 3)
		binary.LittleEndian.PutUint32(content[16:20], 0x5a)
		binary.LittleEndian.PutUint32(content[20:24], 1)
		binary.LittleEndian.PutUint32(content[24:28], 2)
		binary.LittleEndian.PutUint32(content[28:32], uint32((2<<16)|32))
		copy(content[32:34], []byte{0xab, 0xcd})
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "i2c-2 #3 a=05a f=0001 l=2 ab-cd]" {
			t.Fatalf("i2c write: known=%v body=%q", known, body)
		}
	})

	t.Run("ufshcd_command", func(t *testing.T) {
		format := eventFormat{ID: 62, Name: "ufshcd_command", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "__data_loc char[]", Name: "__data_loc_str", Offset: 8, Size: 4},
			{Type: "__data_loc char[]", Name: "__data_loc_dev_name", Offset: 12, Size: 4},
			{Name: "tag", Offset: 16, Size: 4},
			{Name: "doorbell", Offset: 20, Size: 4},
			{Name: "transfer_len", Offset: 24, Size: 4, Signed: true},
			{Name: "intr", Offset: 28, Size: 4},
			{Name: "lba", Offset: 32, Size: 8},
			{Name: "opcode", Offset: 40, Size: 4},
			{Name: "group_id", Offset: 44, Size: 4},
		}}
		payload1 := []byte("send\x00")
		payload2 := []byte("ufs0\x00")
		content := make([]byte, 48+len(payload1)+len(payload2))
		binary.LittleEndian.PutUint32(content[8:12], uint32((len(payload1)<<16)|48))
		binary.LittleEndian.PutUint32(content[12:16], uint32((len(payload2)<<16)|(48+len(payload1))))
		binary.LittleEndian.PutUint32(content[16:20], 4)
		binary.LittleEndian.PutUint32(content[20:24], 0xff)
		binary.LittleEndian.PutUint32(content[24:28], 4096)
		binary.LittleEndian.PutUint32(content[28:32], 1)
		binary.LittleEndian.PutUint64(content[32:40], 123)
		binary.LittleEndian.PutUint32(content[40:44], 0x28)
		binary.LittleEndian.PutUint32(content[44:48], 7)
		copy(content[48:], payload1)
		copy(content[48+len(payload1):], payload2)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		want := "send: ufs0: tag: 4, DB: 0xff, size: 4096, IS: 1, LBA: 123, opcode: 0x28 (READ_10), group_id: 0x7"
		if !known || body != want {
			t.Fatalf("ufshcd command: known=%v body=%q", known, body)
		}
	})

	t.Run("rss_stat_has_no_unit_suffix", func(t *testing.T) {
		format := eventFormat{ID: 63, Name: "rss_stat", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "mm_id", Offset: 8, Size: 4},
			{Name: "curr", Offset: 12, Size: 4},
			{Name: "member", Offset: 16, Size: 4, Signed: true},
			{Name: "size", Offset: 20, Size: 8, Signed: true},
		}}
		content := make([]byte, 28)
		binary.LittleEndian.PutUint32(content[8:12], 7)
		binary.LittleEndian.PutUint32(content[12:16], 3)
		binary.LittleEndian.PutUint32(content[16:20], uint32(2))
		binary.LittleEndian.PutUint64(content[20:28], uint64(4096))
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "mm_id=7 curr=3 member=2 size=4096" {
			t.Fatalf("rss_stat: known=%v body=%q", known, body)
		}
	})

	t.Run("official_sched_wakeup_new", func(t *testing.T) {
		format := eventFormat{ID: 65, Name: "sched_wakeup_new", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "char", Name: "comm[16]", Offset: 8, Size: 16},
			{Name: "pid", Offset: 24, Size: 4, Signed: true},
			{Name: "prio", Offset: 28, Size: 4, Signed: true},
			{Name: "target_cpu", Offset: 32, Size: 4, Signed: true},
		}}
		content := make([]byte, 36)
		binary.LittleEndian.PutUint16(content[0:2], 65)
		binary.LittleEndian.PutUint32(content[4:8], 10)
		copy(content[8:24], []byte("app\x00"))
		binary.LittleEndian.PutUint32(content[24:28], 20)
		binary.LittleEndian.PutUint32(content[28:32], 53)
		binary.LittleEndian.PutUint32(content[32:36], 2)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "comm=app pid=20 prio=53 target_cpu=002" {
			t.Fatalf("sched_wakeup_new: known=%v body=%q", known, body)
		}
	})

	t.Run("official_sched_stat_iowait", func(t *testing.T) {
		format := eventFormat{ID: 66, Name: "sched_stat_iowait", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "char", Name: "comm[16]", Offset: 8, Size: 16},
			{Name: "pid", Offset: 24, Size: 4, Signed: true},
			{Name: "delay", Offset: 28, Size: 8},
		}}
		content := make([]byte, 36)
		copy(content[8:24], []byte("worker\x00"))
		binary.LittleEndian.PutUint32(content[24:28], 30)
		binary.LittleEndian.PutUint64(content[28:36], 2500000)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "comm=worker pid=30 delay=2500000 [ns]" {
			t.Fatalf("sched_stat_iowait: known=%v body=%q", known, body)
		}
	})

	t.Run("official_sched_stat_runtime", func(t *testing.T) {
		format := eventFormat{ID: 67, Name: "sched_stat_runtime", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "char", Name: "comm[16]", Offset: 8, Size: 16},
			{Name: "pid", Offset: 24, Size: 4, Signed: true},
			{Name: "runtime", Offset: 28, Size: 8},
			{Name: "vruntime", Offset: 36, Size: 8},
		}}
		content := make([]byte, 44)
		copy(content[8:24], []byte("worker\x00"))
		binary.LittleEndian.PutUint32(content[24:28], 30)
		binary.LittleEndian.PutUint64(content[28:36], 1000000)
		binary.LittleEndian.PutUint64(content[36:44], 1200000)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "comm=worker pid=30 runtime=1000000 [ns] vruntime=1200000 [ns]" {
			t.Fatalf("sched_stat_runtime: known=%v body=%q", known, body)
		}
	})

	t.Run("official_ipi_raise", func(t *testing.T) {
		format := eventFormat{ID: 68, Name: "ipi_raise", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "target_cpus", Offset: 8, Size: 8},
			{Type: "char", Name: "reason[32]", Offset: 16, Size: 32},
		}}
		content := make([]byte, 48)
		binary.LittleEndian.PutUint64(content[8:16], 0x10)
		copy(content[16:48], []byte("Rescheduling interrupts\x00"))
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "target_mask=0x10 (Rescheduling interrupts)" {
			t.Fatalf("ipi_raise: known=%v body=%q", known, body)
		}
	})

	t.Run("data_loc_offset_field_does_not_render_raw_integer_bytes", func(t *testing.T) {
		const nameOffset = 0x4142
		format := eventFormat{ID: 64, Name: "clock_set_rate", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "unsigned int", Name: "name", Offset: 8, Size: 4},
			{Name: "state", Offset: 12, Size: 4},
			{Name: "cpu_id", Offset: 16, Size: 4},
		}}
		content := make([]byte, nameOffset+len("heca_info\x00"))
		binary.LittleEndian.PutUint32(content[8:12], nameOffset)
		binary.LittleEndian.PutUint32(content[12:16], 87047)
		binary.LittleEndian.PutUint32(content[16:20], 11)
		copy(content[nameOffset:], []byte("heca_info\x00"))
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "heca_info state=87047 cpu_id=11" {
			t.Fatalf("data_loc offset string: known=%v body=%q", known, body)
		}
	})

	t.Run("android_fs_file_io", func(t *testing.T) {
		format := eventFormat{ID: 70, Name: "android_fs_dataread_end", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "dev", Offset: 8, Size: 8},
			{Name: "ino", Offset: 16, Size: 8},
			{Name: "entry_name[16]", Offset: 24, Size: 16},
			{Name: "offset", Offset: 40, Size: 8, Signed: true},
			{Name: "bytes", Offset: 48, Size: 8},
			{Name: "rw[8]", Offset: 56, Size: 8},
			{Name: "ret", Offset: 64, Size: 4, Signed: true},
			{Name: "latency_us", Offset: 68, Size: 4},
		}}
		content := syntheticAndroidFSContent(70, "foo.db", "read", true)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		for _, want := range []string{"dev=260:136", "ino=0xb9b8e", "entry_name=foo.db", "offset=0", "bytes=4096", "rw=read", "ret=4096", "latency_us=700"} {
			if !known || !strings.Contains(body, want) {
				t.Fatalf("android fs body missing %q: known=%v body=%q", want, known, body)
			}
		}
	})

	t.Run("f2fs_direct_io", func(t *testing.T) {
		format := eventFormat{ID: 71, Name: "f2fs_direct_IO_exit", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "dev", Offset: 8, Size: 8},
			{Name: "ino", Offset: 16, Size: 8},
			{Name: "pos", Offset: 24, Size: 8, Signed: true},
			{Name: "len", Offset: 32, Size: 8},
			{Name: "rw[8]", Offset: 40, Size: 8},
			{Name: "ret", Offset: 48, Size: 4, Signed: true},
			{Name: "latency_us", Offset: 52, Size: 4},
		}}
		content := syntheticF2FSContent(71, true)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		for _, want := range []string{"dev=260:136", "ino=0xb9b8e", "offset=4096", "len=8192", "rw=write", "ret=8192", "latency_us=5700"} {
			if !known || !strings.Contains(body, want) {
				t.Fatalf("f2fs body missing %q: known=%v body=%q", want, known, body)
			}
		}
	})

	t.Run("ext4_direct_io_official_int_rw", func(t *testing.T) {
		format := eventFormat{ID: 73, Name: "ext4_direct_IO_exit", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "dev", Offset: 8, Size: 8},
			{Name: "ino", Offset: 16, Size: 8},
			{Name: "pos", Offset: 24, Size: 8, Signed: true},
			{Name: "len", Offset: 32, Size: 8},
			{Type: "int", Name: "rw", Offset: 40, Size: 4, Signed: true},
			{Type: "int", Name: "ret", Offset: 44, Size: 4, Signed: true},
		}}
		content := make([]byte, 48)
		binary.LittleEndian.PutUint16(content[0:2], 73)
		binary.LittleEndian.PutUint32(content[4:8], 20)
		binary.LittleEndian.PutUint64(content[8:16], syntheticDev(260, 136))
		binary.LittleEndian.PutUint64(content[16:24], 0xb9b8e)
		binary.LittleEndian.PutUint64(content[24:32], 4096)
		binary.LittleEndian.PutUint64(content[32:40], 8192)
		binary.LittleEndian.PutUint32(content[40:44], 1)
		binary.LittleEndian.PutUint32(content[44:48], 8192)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		for _, want := range []string{"dev=260:136", "ino=0xb9b8e", "offset=4096", "len=8192", "rw=write", "ret=8192"} {
			if !known || !strings.Contains(body, want) {
				t.Fatalf("ext4 body missing %q: known=%v body=%q", want, known, body)
			}
		}
	})

	t.Run("scsi_dispatch_cmd", func(t *testing.T) {
		format := eventFormat{ID: 72, Name: "scsi_dispatch_cmd_done", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "tag", Offset: 8, Size: 4, Signed: true},
			{Name: "dev", Offset: 12, Size: 8},
			{Name: "lba", Offset: 20, Size: 8},
			{Name: "len", Offset: 28, Size: 4},
			{Name: "opcode[16]", Offset: 32, Size: 16},
			{Name: "ret", Offset: 48, Size: 4, Signed: true},
			{Name: "latency_us", Offset: 52, Size: 4},
		}}
		content := syntheticSCSIContent(72, true)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		for _, want := range []string{"tag=7", "dev=12:80", "lba=4096", "len=8192", "opcode=WRITE", "ret=0", "latency_us=3500"} {
			if !known || !strings.Contains(body, want) {
				t.Fatalf("scsi body missing %q: known=%v body=%q", want, known, body)
			}
		}
	})
}

func TestConvertFileRoundTripsInodeIOThroughTraceQuery(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "inode-io.htrace")
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticIOEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("20 app\n"))
	writeSegment(&b, segmentTGIDs, []byte("20 20\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 80, OffsetNS: 1_000_000, Content: syntheticAndroidFSContent(80, "foo.db", "write", false)},
		{EventID: 81, OffsetNS: 2_000_000, Content: syntheticAndroidFSContent(81, "foo.db", "write", true)},
		{EventID: 82, OffsetNS: 3_000_000, Content: syntheticMMFilemapContent(82, 0)},
		{EventID: 83, OffsetNS: 4_000_000, Content: syntheticMMFilemapContent(83, 0)},
		{EventID: 84, OffsetNS: 5_000_000, Content: syntheticF2FSContent(84, false)},
		{EventID: 85, OffsetNS: 8_000_000, Content: syntheticF2FSContent(85, true)},
		{EventID: 86, OffsetNS: 9_000_000, Content: syntheticExt4DirectIOContent(86, false)},
		{EventID: 87, OffsetNS: 11_000_000, Content: syntheticExt4DirectIOContent(87, true)},
	}))
	if err := os.WriteFile(input, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.UnknownEventCount != 0 {
		t.Fatalf("IO renderer rows should not be header-only: %+v", result)
	}
	idx, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatalf("tracequery parse converted IO output: %v", err)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{PID: 20, TimeStart: 2942.124, TimeEnd: 2942.140})
	if len(stats.FileIOByInode) == 0 || stats.FileIOByInode[0].Inode != "0xb9b8e" || stats.FileIOByInode[0].Bytes == 0 || stats.FileIOByInode[0].CompletionCount == 0 || stats.FileIOByInode[0].MaxLatencyMs == 0 {
		t.Fatalf("converted file IO did not aggregate inode bytes/completion/latency: %+v", stats.FileIOByInode)
	}
	if len(stats.PageCacheByInode) == 0 || stats.PageCacheByInode[0].Inode != "0xb9b8e" || stats.PageCacheByInode[0].Churn != 2 {
		t.Fatalf("converted page-cache rows did not aggregate: %+v", stats.PageCacheByInode)
	}
	foundF2FS := false
	foundExt4 := false
	for _, item := range stats.StorageLatencyByLayer {
		if item.Layer == "f2fs" && item.PairedCount == 1 && item.MaxLatencyMs > 0 {
			foundF2FS = true
		}
		if item.Layer == "ext4" && item.PairedCount == 1 && item.Inode == "0xcafe" && item.MaxLatencyMs > 0 {
			foundExt4 = true
		}
	}
	if !foundF2FS {
		t.Fatalf("converted f2fs rows did not produce storage latency: %+v", stats.StorageLatencyByLayer)
	}
	if !foundExt4 {
		t.Fatalf("converted ext4 rows did not produce storage latency: %+v", stats.StorageLatencyByLayer)
	}
	if stats.IOPressureSummary == nil || stats.IOPressureSummary.TopInode != "0xb9b8e" {
		t.Fatalf("converted IO rows did not produce IO pressure summary: %+v", stats.IOPressureSummary)
	}
}

func syntheticBinaryHitrace(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("36379 com.tencent.mm\n"))
	writeSegment(&b, segmentTGIDs, []byte("36379 36379\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPage())
	return b.Bytes()
}

func syntheticStandaloneProfilerBlock(dataType uint32, pluginName, pluginVersion string, payload []byte) []byte {
	block := make([]byte, profilerTraceHeaderSize+len(payload))
	binary.LittleEndian.PutUint64(block[0:8], profilerTraceHeaderMagic)
	binary.LittleEndian.PutUint64(block[8:16], uint64(len(block)))
	binary.LittleEndian.PutUint32(block[16:20], 0x00010000)
	binary.LittleEndian.PutUint32(block[56:60], dataType)
	copy(block[profilerPluginNameOffset:profilerPluginNameOffset+profilerPluginNameSize], []byte(pluginName))
	copy(block[profilerPluginVersionOffset:profilerPluginVersionOffset+profilerPluginVersionSize], []byte(pluginVersion))
	copy(block[profilerTraceHeaderSize:], payload)
	return block
}

func syntheticProfilerTraceFile(messages ...[]byte) []byte {
	var payload bytes.Buffer
	for _, msg := range messages {
		var lenBuf [4]byte
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(msg)))
		payload.Write(lenBuf[:])
		payload.Write(msg)
	}
	body := make([]byte, profilerTraceHeaderSize+payload.Len())
	binary.LittleEndian.PutUint64(body[0:8], profilerTraceHeaderMagic)
	binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)))
	binary.LittleEndian.PutUint32(body[16:20], 0x00010000)
	binary.LittleEndian.PutUint32(body[20:24], uint32(len(messages)*2))
	binary.LittleEndian.PutUint32(body[56:60], profilerDataTypeProtobuf)
	copy(body[profilerTraceHeaderSize:], payload.Bytes())
	return body
}

func syntheticProfilerPluginData(name string, data []byte) []byte {
	return syntheticProfilerPluginDataWithTiming(name, data, 7, 1, 2, "1.02", 0)
}

func syntheticProfilerPluginDataWithTiming(name string, data []byte, clockID, tvSec, tvNsec uint64, version string, sampleInterval uint32) []byte {
	var out bytes.Buffer
	writeProtoStringField(&out, 1, name)
	writeProtoVarintField(&out, 2, 0)
	writeProtoBytesField(&out, 3, data)
	writeProtoVarintField(&out, 4, clockID)
	writeProtoVarintField(&out, 5, tvSec)
	writeProtoVarintField(&out, 6, tvNsec)
	writeProtoStringField(&out, 7, version)
	if sampleInterval != 0 {
		writeProtoVarintField(&out, 8, uint64(sampleInterval))
	}
	return out.Bytes()
}

func syntheticTracePluginResultMetadata() []byte {
	var out bytes.Buffer
	out.Write(protoMessage(1,
		protoVarint(1, 1),
		protoMessage(2,
			protoVarint(1, 0),
			protoVarint(2, 100),
			protoVarint(3, 1),
			protoVarint(4, 2),
			protoVarint(5, 4096),
			protoVarint(8, 3),
			protoVarint(9, 97),
		),
		protoBytes(3, []byte("boot")),
	))
	out.Write(protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(1, 100, 100, "prev", 2417, protoPayload(
			protoBytes(1, []byte("prev")),
			protoVarint(2, 100),
			protoVarint(3, 120),
			protoVarint(4, 1),
			protoBytes(5, []byte("next")),
			protoVarint(6, 101),
			protoVarint(7, 118),
		)),
		syntheticTracePluginFtraceEvent(2, 100, 100, "io", 211, protoPayload(
			protoVarint(1, 260),
			protoVarint(2, 4096),
			protoVarint(3, 8),
			protoVarint(4, 4096),
			protoBytes(5, []byte("WS")),
			protoBytes(6, []byte("io")),
		)),
		syntheticTracePluginFtraceEvent(3, 100, 100, "client", 113, protoPayload(
			protoVarint(1, 42),
			protoVarint(2, 7),
			protoVarint(3, 200),
			protoVarint(4, 201),
			protoVarint(5, 0),
			protoVarint(6, 3),
			protoVarint(7, 0x12),
		)),
		syntheticTracePluginFtraceEvent(4, 200, 201, "binder", 119, protoPayload(protoVarint(1, 42))),
		syntheticTracePluginFtraceEvent(5, 0, 0, "irq", 1500, protoPayload(protoVarint(1, 32), protoBytes(2, []byte("uart")))),
		syntheticTracePluginFtraceEvent(6, 0, 0, "irq", 1501, protoPayload(protoVarint(1, 32), protoVarint(2, 1))),
		syntheticTracePluginFtraceEvent(7, 0, 0, "idle", 2003, protoPayload(protoVarint(1, 2200000), protoVarint(2, 4))),
		syntheticTracePluginFtraceEvent(8, 100, 100, "io", 4011, protoPayload(protoVarint(1, 260), protoVarint(2, 0xb9b8e), protoVarint(3, 0), protoVarint(4, 4096))),
		syntheticTracePluginFtraceEvent(9, 100, 100, "io", 1000, protoPayload(protoVarint(1, 0x123), protoVarint(2, 0xb9b8e), protoVarint(3, 1), protoVarint(4, 260))),
		syntheticTracePluginFtraceEvent(10, 100, 100, "app", 1109, protoPayload(protoVarint(1, 0), protoBytes(2, []byte("B|100|Frame")))),
		protoVarint(3, 4),
	))
	out.Write(protoMessage(5,
		protoVarint(1, 0x1234),
		protoBytes(2, []byte("schedule")),
	))
	out.Write(protoMessage(6,
		protoVarint(1, 4),
		protoMessage(2,
			protoVarint(1, 10),
			protoVarint(2, 20),
		),
		protoMessage(3,
			protoVarint(1, 0),
			protoVarint(2, 1),
		),
	))
	out.Write(protoBytes(7, []byte("trace-plugin-v1")))
	return out.Bytes()
}

func syntheticTracePluginFtraceEvent(ts, tgid, pid uint64, comm string, eventField int, payload []byte) []byte {
	return protoMessage(2,
		protoVarint(1, ts),
		protoVarint(2, tgid),
		protoBytes(3, []byte(comm)),
		protoMessage(50, protoVarint(4, pid)),
		protoMessage(eventField, payload),
	)
}

func protoPayload(parts ...[]byte) []byte {
	var out bytes.Buffer
	for _, part := range parts {
		out.Write(part)
	}
	return out.Bytes()
}

func writeProtoStringField(out *bytes.Buffer, field int, value string) {
	writeProtoBytesField(out, field, []byte(value))
}

func writeProtoBytesField(out *bytes.Buffer, field int, value []byte) {
	writeProfilerProtoVarint(out, uint64(field<<3|2))
	writeProfilerProtoVarint(out, uint64(len(value)))
	out.Write(value)
}

func writeProtoVarintField(out *bytes.Buffer, field int, value uint64) {
	writeProfilerProtoVarint(out, uint64(field<<3))
	writeProfilerProtoVarint(out, value)
}

func writeProfilerProtoVarint(out *bytes.Buffer, value uint64) {
	for value >= 0x80 {
		out.WriteByte(byte(value) | 0x80)
		value >>= 7
	}
	out.WriteByte(byte(value))
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}

func coverageHasEmitted(coverage []TraceDBCoverage, family, table string, minRows int) bool {
	for _, item := range coverage {
		if item.Family == family && item.Table == table && item.RowsEmitted >= minRows {
			return true
		}
	}
	return false
}

func syntheticEventFormat() string {
	return strings.Join([]string{
		"name: sched_wakeup",
		"ID: 10",
		"format:",
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;",
		"\tfield:unsigned char common_flags;\toffset:2;\tsize:1;\tsigned:0;",
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;",
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;",
		"\tfield:char comm[16];\toffset:8;\tsize:16;\tsigned:0;",
		"\tfield:int pid;\toffset:24;\tsize:4;\tsigned:1;",
		"\tfield:int prio;\toffset:28;\tsize:4;\tsigned:1;",
		"\tfield:int target_cpu;\toffset:32;\tsize:4;\tsigned:1;",
		`print fmt: "comm=%s pid=%d prio=%d target_cpu=%03d"`,
		"",
	}, "\n")
}

func syntheticUnsupportedEventFormat() string {
	return strings.Join([]string{
		"name: vendor_numeric",
		"ID: 20",
		"format:",
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;",
		"\tfield:unsigned char common_flags;\toffset:2;\tsize:1;\tsigned:0;",
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;",
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;",
		"\tfield:int foo;\toffset:8;\tsize:4;\tsigned:1;",
		`print fmt: "foo=%d"`,
		"",
	}, "\n")
}

func syntheticIOEventFormat() string {
	var lines []string
	lines = append(lines, syntheticFormatBlock("android_fs_datawrite_start", 80, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "dev", 8, 8, false),
		syntheticField("unsigned long", "ino", 16, 8, false),
		syntheticField("char", "entry_name[16]", 24, 16, false),
		syntheticField("long", "offset", 40, 8, true),
		syntheticField("unsigned long", "bytes", 48, 8, false),
		syntheticField("char", "rw[8]", 56, 8, false),
		syntheticField("int", "ret", 64, 4, true),
		syntheticField("unsigned int", "latency_us", 68, 4, false),
	})...)
	lines = append(lines, syntheticFormatBlock("android_fs_datawrite_end", 81, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "dev", 8, 8, false),
		syntheticField("unsigned long", "ino", 16, 8, false),
		syntheticField("char", "entry_name[16]", 24, 16, false),
		syntheticField("long", "offset", 40, 8, true),
		syntheticField("unsigned long", "bytes", 48, 8, false),
		syntheticField("char", "rw[8]", 56, 8, false),
		syntheticField("int", "ret", 64, 4, true),
		syntheticField("unsigned int", "latency_us", 68, 4, false),
	})...)
	lines = append(lines, syntheticFormatBlock("mm_filemap_add_to_page_cache", 82, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "s_dev", 8, 8, false),
		syntheticField("unsigned long", "i_ino", 16, 8, false),
		syntheticField("unsigned long", "index", 24, 8, false),
		syntheticField("unsigned long", "pfn", 32, 8, false),
		syntheticField("unsigned long", "pg", 40, 8, false),
	})...)
	lines = append(lines, syntheticFormatBlock("mm_filemap_delete_from_page_cache", 83, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "s_dev", 8, 8, false),
		syntheticField("unsigned long", "i_ino", 16, 8, false),
		syntheticField("unsigned long", "index", 24, 8, false),
		syntheticField("unsigned long", "pfn", 32, 8, false),
		syntheticField("unsigned long", "pg", 40, 8, false),
	})...)
	lines = append(lines, syntheticFormatBlock("f2fs_direct_IO_enter", 84, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "dev", 8, 8, false),
		syntheticField("unsigned long", "ino", 16, 8, false),
		syntheticField("long", "pos", 24, 8, true),
		syntheticField("unsigned long", "len", 32, 8, false),
		syntheticField("char", "rw[8]", 40, 8, false),
		syntheticField("int", "ret", 48, 4, true),
		syntheticField("unsigned int", "latency_us", 52, 4, false),
	})...)
	lines = append(lines, syntheticFormatBlock("f2fs_direct_IO_exit", 85, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "dev", 8, 8, false),
		syntheticField("unsigned long", "ino", 16, 8, false),
		syntheticField("long", "pos", 24, 8, true),
		syntheticField("unsigned long", "len", 32, 8, false),
		syntheticField("char", "rw[8]", 40, 8, false),
		syntheticField("int", "ret", 48, 4, true),
		syntheticField("unsigned int", "latency_us", 52, 4, false),
	})...)
	lines = append(lines, syntheticFormatBlock("ext4_direct_IO_enter", 86, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "dev", 8, 8, false),
		syntheticField("unsigned long", "ino", 16, 8, false),
		syntheticField("unsigned long", "pos", 24, 8, false),
		syntheticField("unsigned long", "len", 32, 8, false),
		syntheticField("int", "rw", 40, 4, true),
	})...)
	lines = append(lines, syntheticFormatBlock("ext4_direct_IO_exit", 87, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "dev", 8, 8, false),
		syntheticField("unsigned long", "ino", 16, 8, false),
		syntheticField("unsigned long", "pos", 24, 8, false),
		syntheticField("unsigned long", "len", 32, 8, false),
		syntheticField("int", "rw", 40, 4, true),
		syntheticField("int", "ret", 44, 4, true),
	})...)
	return strings.Join(lines, "\n")
}

func syntheticFormatBlock(name string, id int, fields []string) []string {
	out := []string{nameLine(name), fmt.Sprintf("ID: %d", id), "format:"}
	out = append(out, fields...)
	out = append(out, `print fmt: "synthetic"`, "")
	return out
}

func nameLine(name string) string {
	return "name: " + name
}

func syntheticField(typ, name string, offset, size int, signed bool) string {
	signedFlag := 0
	if signed {
		signedFlag = 1
	}
	return fmt.Sprintf("\tfield:%s %s;\toffset:%d;\tsize:%d;\tsigned:%d;", typ, name, offset, size, signedFlag)
}

type syntheticRawEvent struct {
	EventID  uint16
	OffsetNS uint32
	Content  []byte
}

func syntheticRawPageEvents(events []syntheticRawEvent) []byte {
	page := make([]byte, tracePageSize)
	binary.LittleEndian.PutUint64(page[0:8], 2942124416000) // ns, renders 2942.124416
	binary.LittleEndian.PutUint64(page[8:16], tracePageSize)
	page[16] = 1
	off := pageHeaderSize
	for _, ev := range events {
		content := append([]byte(nil), ev.Content...)
		if len(content) < 2 {
			continue
		}
		binary.LittleEndian.PutUint16(content[0:2], ev.EventID)
		aligned := int((uint32(len(content)) + 3) &^ 3)
		if off+eventHeaderSize+aligned > len(page) {
			break
		}
		binary.LittleEndian.PutUint32(page[off:off+4], ev.OffsetNS)
		binary.LittleEndian.PutUint16(page[off+4:off+6], uint16(len(content)))
		copy(page[off+eventHeaderSize:], content)
		off += eventHeaderSize + aligned
	}
	return page
}

func syntheticAndroidFSContent(eventID uint16, entryName, rw string, done bool) []byte {
	content := make([]byte, 72)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[4:8], 20)
	binary.LittleEndian.PutUint64(content[8:16], syntheticDev(260, 136))
	binary.LittleEndian.PutUint64(content[16:24], 0xb9b8e)
	copy(content[24:40], []byte(entryName))
	binary.LittleEndian.PutUint64(content[40:48], 0)
	binary.LittleEndian.PutUint64(content[48:56], 4096)
	copy(content[56:64], []byte(rw))
	if done {
		binary.LittleEndian.PutUint32(content[64:68], 4096)
		binary.LittleEndian.PutUint32(content[68:72], 700)
	}
	return content
}

func syntheticF2FSContent(eventID uint16, done bool) []byte {
	content := make([]byte, 56)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[4:8], 20)
	binary.LittleEndian.PutUint64(content[8:16], syntheticDev(260, 136))
	binary.LittleEndian.PutUint64(content[16:24], 0xb9b8e)
	binary.LittleEndian.PutUint64(content[24:32], 4096)
	binary.LittleEndian.PutUint64(content[32:40], 8192)
	copy(content[40:48], []byte("write"))
	if done {
		binary.LittleEndian.PutUint32(content[48:52], 8192)
		binary.LittleEndian.PutUint32(content[52:56], 5700)
	}
	return content
}

func syntheticExt4DirectIOContent(eventID uint16, done bool) []byte {
	content := make([]byte, 48)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[4:8], 20)
	binary.LittleEndian.PutUint64(content[8:16], syntheticDev(260, 136))
	binary.LittleEndian.PutUint64(content[16:24], 0xcafe)
	binary.LittleEndian.PutUint64(content[24:32], 16384)
	binary.LittleEndian.PutUint64(content[32:40], 4096)
	binary.LittleEndian.PutUint32(content[40:44], 0)
	if done {
		binary.LittleEndian.PutUint32(content[44:48], 4096)
	}
	return content
}

func syntheticSCSIContent(eventID uint16, done bool) []byte {
	content := make([]byte, 56)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[4:8], 20)
	binary.LittleEndian.PutUint32(content[8:12], 7)
	binary.LittleEndian.PutUint64(content[12:20], syntheticDev(12, 80))
	binary.LittleEndian.PutUint64(content[20:28], 4096)
	binary.LittleEndian.PutUint32(content[28:32], 8192)
	copy(content[32:48], []byte("WRITE"))
	if done {
		binary.LittleEndian.PutUint32(content[52:56], 3500)
	}
	return content
}

func syntheticMMFilemapContent(eventID uint16, index uint64) []byte {
	content := make([]byte, 48)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[4:8], 20)
	binary.LittleEndian.PutUint64(content[8:16], syntheticDev(260, 136))
	binary.LittleEndian.PutUint64(content[16:24], 0xb9b8e)
	binary.LittleEndian.PutUint64(content[24:32], index)
	binary.LittleEndian.PutUint64(content[32:40], 3062260)
	return content
}

func syntheticDev(major, minor uint64) uint64 {
	return (major << 20) | minor
}

func testPrefix(data []byte, n int) []byte {
	if len(data) < n {
		return data
	}
	return data[:n]
}

func syntheticRawPage() []byte {
	return syntheticRawPageForEventID(10)
}

func syntheticRawPageForEventID(eventID uint16) []byte {
	page := make([]byte, tracePageSize)
	binary.LittleEndian.PutUint64(page[0:8], 2942124416000) // ns, renders 2942.124416
	binary.LittleEndian.PutUint64(page[8:16], tracePageSize)
	page[16] = 4

	content := make([]byte, 36)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	content[2] = 0
	content[3] = 0
	binary.LittleEndian.PutUint32(content[4:8], uint32(36379))
	copy(content[8:24], []byte("com.tencent.mm"))
	binary.LittleEndian.PutUint32(content[24:28], uint32(36379))
	binary.LittleEndian.PutUint32(content[28:32], uint32(53))
	binary.LittleEndian.PutUint32(content[32:36], uint32(0))

	off := pageHeaderSize
	binary.LittleEndian.PutUint32(page[off:off+4], 0)
	binary.LittleEndian.PutUint16(page[off+4:off+6], uint16(len(content)))
	copy(page[off+eventHeaderSize:], content)
	return page
}

func writeFileHeader(b *bytes.Buffer, cpuNum int) {
	buf := make([]byte, fileHeaderSize)
	binary.LittleEndian.PutUint16(buf[0:2], 0xace)
	buf[2] = 1
	binary.LittleEndian.PutUint16(buf[4:6], 1)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(cpuNum<<1))
	b.Write(buf)
}

func writeSegment(b *bytes.Buffer, typ int, payload []byte) {
	var hdr [segmentHdrSize]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(typ))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	b.Write(hdr[:])
	b.Write(payload)
}
