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

func TestConvertRawPerfDataFileToPerfTraceRoundTripsThroughTraceQuery(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf.data")
	outPath := filepath.Join(dir, "raw.perftrace")
	if err := os.WriteFile(perfData, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), perfData, outPath); err != nil {
		t.Fatalf("convert raw perf.data: %v", err)
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
		"sample_weight=99",
		`event="config:0x0"`,
		`symbol="0x1234"`,
		`dso="/system/lib64/libfoo.so"`,
		`ip="0x1234"`,
		`callchain="0x1222;0x1111;0x1234"`,
		"source=raw_perfdata_fallback",
		"symbolization_status=unsymbolized",
		"cpu_known=true",
		"clock_confidence=assumed",
		"callchain_status=ip_only",
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
	if ev.Type != tracequery.EventPerfSample || ev.CPU != 5 || ev.PerfPID != 1234 || ev.PerfTID != 5678 || ev.PerfPeriod != 99 {
		t.Fatalf("bad perf sample fields: %+v", ev)
	}
	if ev.PerfSymbol != "0x1234" || ev.PerfDSO != "/system/lib64/libfoo.so" || ev.PerfSource != "raw_perfdata_fallback" || ev.PerfSymbolizationStatus != "unsymbolized" {
		t.Fatalf("bad raw perf metadata fields: %+v", ev)
	}
	if ev.PerfCPUKnown == nil || !*ev.PerfCPUKnown || ev.PerfClockConfidence != "assumed" || ev.PerfCallchainStatus != "ip_only" {
		t.Fatalf("bad raw perf quality fields: %+v", ev)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 1.0, TimeEnd: 2.0})
	if stats.PerfSamples == nil || len(stats.PerfSamples.TopSymbols) == 0 {
		t.Fatalf("missing perf sample stats: %+v", stats.PerfSamples)
	}
	if stats.PerfSamples.TopSymbols[0].SymbolizationStatus != "unsymbolized" || stats.PerfSamples.TopSymbols[0].Source != "raw_perfdata_fallback" {
		t.Fatalf("raw source/status should reach hotspot summaries: %+v", stats.PerfSamples.TopSymbols[0])
	}
	if stats.PerfSamples.Quality == nil || stats.PerfSamples.Quality.CPUKnownCount != 1 || len(stats.PerfSamples.Quality.Caveats) == 0 {
		t.Fatalf("raw perf quality should reach aggregate summary: %+v", stats.PerfSamples.Quality)
	}
}

func TestConvertRawPerfDataSkipsSafeExtraSampleFields(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf-extra.data")
	outPath := filepath.Join(dir, "raw-extra.perftrace")
	sampleType := uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod | perfSampleCallchain | perfSampleRaw | perfSampleWeight)
	if err := os.WriteFile(perfData, syntheticRawPerfDataWithSampleType(sampleType, 0), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readRawPerfData(context.Background(), perfData)
	if err != nil {
		t.Fatalf("read raw perf data with extra fields: %v", err)
	}
	if len(data.Caveats) == 0 || !strings.Contains(strings.Join(data.Caveats, "\n"), "raw,weight") {
		t.Fatalf("expected skipped-field caveat, got %+v", data.Caveats)
	}
	if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), perfData, outPath); err != nil {
		t.Fatalf("convert raw perf.data with extra fields: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`symbol="0x1234"`, `parser_caveats=`, `raw fallback skipped non-causal sample payload field(s): raw,weight`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("perftrace missing %q:\n%s", want, string(body))
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("parse perftrace: %v", err)
	}
	if len(idx.Events) != 1 || idx.Events[0].PerfSymbol != "0x1234" {
		t.Fatalf("extra fields should not disturb sample parsing: %+v", idx.Events)
	}
}

func TestConvertRawPerfDataSkipsUserRegsStackAndVendorBit(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf-user-stack.data")
	outPath := filepath.Join(dir, "raw-user-stack.perftrace")
	sampleType := uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod | perfSampleRegsUser | perfSampleStackUser | rawPerfVendorSampleBit31)
	if err := os.WriteFile(perfData, syntheticRawPerfDataWithSampleType(sampleType, 0), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readRawPerfData(context.Background(), perfData)
	if err != nil {
		t.Fatalf("read raw perf data with user regs/stack/vendor bit: %v", err)
	}
	joined := strings.Join(data.Caveats, "\n")
	for _, want := range []string{"regs_user", "stack_user", "unknown:0x80000000"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected skipped sample field %q in caveats: %+v", want, data.Caveats)
		}
	}
	if len(data.Samples) != 1 || data.Samples[0].PID != 1234 || data.Samples[0].CPU != 5 || data.Samples[0].Period != 99 {
		t.Fatalf("sample identity should survive skipped user stack fields: %+v", data.Samples)
	}
	if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), perfData, outPath); err != nil {
		t.Fatalf("convert raw perf.data with user regs/stack/vendor bit: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`pid=1234`, `cpu=5`, `raw fallback skipped non-causal sample payload field(s): regs_user,stack_user,unknown:0x80000000`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("perftrace missing %q:\n%s", want, body)
		}
	}
}

func TestConvertRawPerfDataMapsMultiAttrSampleIDToEvent(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf-multi.data")
	outPath := filepath.Join(dir, "raw-multi.perftrace")
	if err := os.WriteFile(perfData, syntheticRawPerfDataMultiAttrByIdentifier(), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readRawPerfData(context.Background(), perfData)
	if err != nil {
		t.Fatalf("read multi-attr raw perf data: %v", err)
	}
	if len(data.Attrs) != 2 {
		t.Fatalf("expected two attrs, got %+v", data.Attrs)
	}
	if len(data.Samples) != 1 || data.Samples[0].EventName != "config:0x2" {
		t.Fatalf("sample should map id to second attr event: %+v", data.Samples)
	}
	if got := strings.Join(data.Caveats, " "); !strings.Contains(got, "parsed 2 perf attrs") {
		t.Fatalf("multi-attr parse should remain visible as caveat: %+v", data.Caveats)
	}
	if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), perfData, outPath); err != nil {
		t.Fatalf("convert multi-attr raw perf.data: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`event="config:0x2"`, `parser_caveats=`, `maps sample ids to event names`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("multi-attr perftrace missing %q:\n%s", want, string(body))
		}
	}
}

func TestConvertRawPerfDataPreservesFeatureMetadataCaveats(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf-feature.data")
	outPath := filepath.Join(dir, "raw-feature.perftrace")
	if err := os.WriteFile(perfData, syntheticRawPerfDataWithFeatures(), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readRawPerfData(context.Background(), perfData)
	if err != nil {
		t.Fatalf("read feature raw perf data: %v", err)
	}
	if data.Features.Arch != "arm64" || strings.Join(data.Features.Cmdline, " ") != "simpleperf record" || data.Features.Meta["clockid"] != "monotonic" || data.Features.BuildIDCount != 1 || data.Features.BuildIDs["/system/lib64/libfoo.so"] == "" {
		t.Fatalf("feature metadata not parsed: %+v", data.Features)
	}
	if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), perfData, outPath); err != nil {
		t.Fatalf("convert feature raw perf.data: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`parser_caveats=`, `arch=arm64`, `cmdline=simpleperf record`, `meta.clockid=monotonic`, `meta.event_type_info=present`, `build_id_records=1`, `build_id_dso_labeling=exact_path`, `dso="/system/lib64/libfoo.so#build_id=0102030405060708090a0b0c0d0e0f1011121314"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("feature perftrace missing %q:\n%s", want, string(body))
		}
	}
}

func TestConvertRawPerfDataUsesSavedHiperfArkTSSymbols(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf-arkts-symbols.data")
	outPath := filepath.Join(dir, "raw-arkts-symbols.perftrace")
	if err := os.WriteFile(perfData, syntheticRawPerfDataWithHiperfArkTSSymbols(), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readRawPerfData(context.Background(), perfData)
	if err != nil {
		t.Fatalf("read raw perf data with hiperf symbols: %v", err)
	}
	if len(data.Features.SymbolFiles) != 1 || data.Features.SymbolCount != 2 {
		t.Fatalf("HIPERF_FILES_SYMBOL feature not parsed: %+v", data.Features)
	}
	if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), perfData, outPath); err != nil {
		t.Fatalf("convert raw perf.data with hiperf symbols: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		`symbol="Index.build:entry/src/main/ets/pages/Index.ets"`,
		`dso="/data/storage/el1/bundle/entry.hap"`,
		`callchain="renderButton@/data/storage/el1/bundle/entry.hap;0x1111;Index.build:entry/src/main/ets/pages/Index.ets@/data/storage/el1/bundle/entry.hap"`,
		"symbolization_status=symbolized",
		"callchain_status=symbolized",
		"symbol_source=hiperf_files_symbol",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("perftrace missing %q:\n%s", want, text)
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
	if ev.PerfSymbol != "Index.build:entry/src/main/ets/pages/Index.ets" || ev.PerfSymbolizationStatus != "symbolized" || ev.PerfCallchainStatus != "symbolized" {
		t.Fatalf("saved hiperf symbol should reach tracequery: %+v", ev)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 1.0, TimeEnd: 2.0})
	if stats.PerfSamples == nil || len(stats.PerfSamples.TopSymbols) == 0 || stats.PerfSamples.TopSymbols[0].Symbol != "Index.build:entry/src/main/ets/pages/Index.ets" {
		t.Fatalf("saved hiperf symbol should reach hotspot stats: %+v", stats.PerfSamples)
	}
}

func TestConvertRawPerfDataBindsCommByRecordLifetime(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf-comm-lifetime.data")
	outPath := filepath.Join(dir, "raw-comm-lifetime.perftrace")
	if err := os.WriteFile(perfData, syntheticRawPerfDataWithCommLifetime(), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readRawPerfData(context.Background(), perfData)
	if err != nil {
		t.Fatalf("read comm lifetime raw perf data: %v", err)
	}
	if len(data.Samples) != 2 || data.Samples[0].Comm != "before" || data.Samples[1].Comm != "after" {
		t.Fatalf("samples should preserve record-order comm lifetime: %+v", data.Samples)
	}
	if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), perfData, outPath); err != nil {
		t.Fatalf("convert comm lifetime raw perf.data: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `thread_comm="before"`) || !strings.Contains(text, `thread_comm="after"`) {
		t.Fatalf("perftrace should contain both comm lifetimes:\n%s", text)
	}
	if strings.Count(text, `thread_comm="after"`) != 1 {
		t.Fatalf("later COMM should not rewrite earlier sample:\n%s", text)
	}
}

func TestConvertFileUsesRawPerfParserForDirectPerfDataByContent(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "capture.bin")
	if err := os.WriteFile(perfData, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "ignored.systrace")

	result, err := ConvertFile(context.Background(), Options{InputPath: perfData, OutputPath: output, PerfParser: "raw"})
	if err != nil {
		t.Fatalf("convert direct raw perf.data: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("direct raw perf.data should be sidecar-only: %+v", result)
	}
	var perfTrace Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			perfTrace = artifact
			break
		}
	}
	if perfTrace.Path == "" || !strings.Contains(perfTrace.Converter, "raw-perfdata") {
		t.Fatalf("missing raw perftrace artifact: %+v", result.Artifacts)
	}
	if perfTrace.Perf == nil || perfTrace.Perf.ProviderKind != "raw_fallback" || perfTrace.Perf.InputFormat != string(perfInputLinuxPerfData) {
		t.Fatalf("missing raw perf capability: %+v", perfTrace.Perf)
	}
	if len(result.ProviderDecisions) != 1 {
		t.Fatalf("expected one raw provider decision: %+v", result.ProviderDecisions)
	}
	decision := result.ProviderDecisions[0]
	if decision.ProviderName != perfProviderNameRawFallback || !decision.Selected || !decision.Attempted || !decision.Succeeded || decision.Fallback || !decision.TraceQueryReady {
		t.Fatalf("bad raw provider decision: %+v", decision)
	}
	idx, err := tracequery.BuildIndex(context.Background(), perfTrace.Path)
	if err != nil {
		t.Fatalf("parse generated perftrace: %v", err)
	}
	if len(idx.Events) != 1 || idx.Events[0].PerfSource != "raw_perfdata_fallback" {
		t.Fatalf("generated raw perftrace did not round-trip: %+v", idx.Events)
	}
	if _, err := os.Stat(output); err == nil {
		t.Fatalf("direct perf.data conversion should not create systrace output %s", output)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestConvertFilePreservesDirectPerfDataWhenOfficialParserUnavailable(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf.data")
	if err := os.WriteFile(perfData, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "ignored.systrace")

	result, err := ConvertFile(context.Background(), Options{InputPath: perfData, OutputPath: output, PerfParser: "official"})
	if err != nil {
		t.Fatalf("convert direct perf.data without official parser should not fall through to hitrace parsing: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("direct perf.data should remain sidecar-only: %+v", result)
	}
	if result.BundlePath == "" || len(result.Caveats) == 0 {
		t.Fatalf("expected bundle and caveat for unavailable official parser: %+v", result)
	}
	if len(result.ProviderDecisions) != 2 {
		t.Fatalf("expected official skip plus raw disabled decisions: %+v", result.ProviderDecisions)
	}
	if result.ProviderDecisions[0].ProviderName != perfProviderNameSimpleperfText || result.ProviderDecisions[0].Attempted || result.ProviderDecisions[0].Succeeded || result.ProviderDecisions[0].Reason != "official_tool_unavailable" {
		t.Fatalf("bad official unavailable decision: %+v", result.ProviderDecisions[0])
	}
	if result.ProviderDecisions[1].ProviderName != perfProviderNameRawFallback || result.ProviderDecisions[1].Selected || result.ProviderDecisions[1].Attempted || result.ProviderDecisions[1].Reason != "disabled_by_parser_mode" {
		t.Fatalf("bad raw disabled decision: %+v", result.ProviderDecisions[1])
	}
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			t.Fatalf("official-only mode without official adapter should not emit raw perftrace: %+v", result.Artifacts)
		}
		if artifact.Type == ArtifactPerfData && (artifact.Perf == nil || artifact.Perf.InputFormat != string(perfInputLinuxPerfData)) {
			t.Fatalf("perf_data artifact should carry detected input capability: %+v", artifact.Perf)
		}
	}
}

func TestConvertFilePreservesDirectPerfDataWhenPerftraceDisabled(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "capture.no_suffix")
	if err := os.WriteFile(perfData, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: perfData, DisablePerfAdapter: true})
	if err != nil {
		t.Fatalf("convert direct perf.data with perftrace disabled: %v", err)
	}
	if result.OutputPath != "" || result.BundlePath == "" {
		t.Fatalf("direct perf.data should produce sidecar-only bundle: %+v", result)
	}
	if len(result.ProviderDecisions) != 1 {
		t.Fatalf("expected disabled provider decision: %+v", result.ProviderDecisions)
	}
	decision := result.ProviderDecisions[0]
	if decision.ProviderName != perfProviderNamePerftraceDisabled || !decision.Selected || decision.Attempted || decision.Succeeded || decision.Reason != "perftrace_generation_disabled" {
		t.Fatalf("bad disabled provider decision: %+v", decision)
	}
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			t.Fatalf("disabled perftrace generation should not emit perftrace: %+v", result.Artifacts)
		}
	}
}

func syntheticRawPerfDataWithHiperfArkTSSymbols() []byte {
	const headerSize = 104
	const attrSize = 48
	sampleType := uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod | perfSampleCallchain)
	hapPath := "/data/storage/el1/bundle/entry.hap"
	var records bytes.Buffer
	records.Write(rawPerfRecord(perfRecordComm, rawPerfCommPayload(1234, 5678, "app")))
	records.Write(rawPerfRecord(perfRecordMmap, rawPerfMmapPayload(1234, 5678, 0x1000, 0x1000, 0, hapPath)))
	records.Write(rawPerfRecord(perfRecordSample, rawPerfSamplePayload(sampleType)))

	dataOffset := headerSize + attrSize
	featureIDs := []int{perfFeatureHiperfFilesSymbol}
	descriptorOffset := dataOffset + records.Len()
	sectionOffset := descriptorOffset + len(featureIDs)*16
	sections := [][]byte{
		rawPerfFeatureHiperfArkTSSymbols(hapPath),
	}
	out := make([]byte, sectionOffset)
	copy(out[0:8], []byte(perfMagic2))
	binary.LittleEndian.PutUint64(out[8:16], headerSize)
	binary.LittleEndian.PutUint64(out[16:24], attrSize)
	binary.LittleEndian.PutUint64(out[24:32], headerSize)
	binary.LittleEndian.PutUint64(out[32:40], attrSize)
	binary.LittleEndian.PutUint64(out[40:48], uint64(dataOffset))
	binary.LittleEndian.PutUint64(out[48:56], uint64(records.Len()))
	for _, id := range featureIDs {
		out[72+id/8] |= 1 << (id % 8)
	}
	attr := out[headerSize:dataOffset]
	binary.LittleEndian.PutUint32(attr[0:4], 0)
	binary.LittleEndian.PutUint32(attr[4:8], 40)
	binary.LittleEndian.PutUint64(attr[8:16], 0)
	binary.LittleEndian.PutUint64(attr[24:32], sampleType)
	copy(out[dataOffset:descriptorOffset], records.Bytes())

	cur := sectionOffset
	for i, section := range sections {
		desc := out[descriptorOffset+i*16 : descriptorOffset+(i+1)*16]
		binary.LittleEndian.PutUint64(desc[0:8], uint64(cur))
		binary.LittleEndian.PutUint64(desc[8:16], uint64(len(section)))
		out = append(out, section...)
		cur += len(section)
	}
	return out
}

func rawPerfFeatureHiperfArkTSSymbols(path string) []byte {
	var out bytes.Buffer
	writeRawPerfTestU32(&out, 1)
	writeRawPerfTestString(&out, path)
	writeRawPerfTestU32(&out, rawPerfSymbolFileHAP)
	writeRawPerfTestU64(&out, 0)
	writeRawPerfTestU64(&out, 0)
	writeRawPerfTestString(&out, "010203")
	writeRawPerfTestU32(&out, 2)
	writeRawPerfTestU64(&out, 0x1210)
	writeRawPerfTestU32(&out, 0x30)
	writeRawPerfTestString(&out, "renderButton")
	writeRawPerfTestU64(&out, 0x1230)
	writeRawPerfTestU32(&out, 0x40)
	writeRawPerfTestString(&out, "Index.build:entry/src/main/ets/pages/Index.ets")
	return out.Bytes()
}

func syntheticRawPerfData() []byte {
	sampleType := uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod | perfSampleCallchain)
	return syntheticRawPerfDataWithSampleType(sampleType, 0)
}

func syntheticRawPerfDataWithSampleType(sampleType uint64, readFormat uint64) []byte {
	const headerSize = 104
	const attrSize = 48
	records := bytes.Buffer{}
	records.Write(rawPerfRecord(perfRecordComm, rawPerfCommPayload(1234, 5678, "app")))
	records.Write(rawPerfRecord(perfRecordMmap, rawPerfMmapPayload(1234, 5678, 0x1000, 0x1000, 0, "/system/lib64/libfoo.so")))
	records.Write(rawPerfRecord(perfRecordSample, rawPerfSamplePayload(sampleType)))

	dataOffset := headerSize + attrSize
	out := make([]byte, dataOffset)
	copy(out[0:8], []byte(perfMagic2))
	binary.LittleEndian.PutUint64(out[8:16], headerSize)
	binary.LittleEndian.PutUint64(out[16:24], attrSize)
	binary.LittleEndian.PutUint64(out[24:32], headerSize)
	binary.LittleEndian.PutUint64(out[32:40], attrSize)
	binary.LittleEndian.PutUint64(out[40:48], uint64(dataOffset))
	binary.LittleEndian.PutUint64(out[48:56], uint64(records.Len()))

	attr := out[headerSize:dataOffset]
	binary.LittleEndian.PutUint32(attr[0:4], 0)
	binary.LittleEndian.PutUint32(attr[4:8], 40)
	binary.LittleEndian.PutUint64(attr[8:16], 0)
	binary.LittleEndian.PutUint64(attr[24:32], sampleType)
	binary.LittleEndian.PutUint64(attr[32:40], readFormat)

	out = append(out, records.Bytes()...)
	return out
}

func syntheticRawPerfDataMultiAttrByIdentifier() []byte {
	const headerSize = 104
	const attrStructSize = 40
	const attrEntrySize = 56
	sampleType := uint64(perfSampleIdentifier | perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod)
	var records bytes.Buffer
	records.Write(rawPerfRecord(perfRecordComm, rawPerfCommPayload(1234, 5678, "app")))
	records.Write(rawPerfRecord(perfRecordSample, rawPerfSamplePayload(sampleType)))

	attrsSize := attrEntrySize * 2
	idsOffset := headerSize + attrsSize
	idsSize := 16
	dataOffset := idsOffset + idsSize
	out := make([]byte, dataOffset)
	copy(out[0:8], []byte(perfMagic2))
	binary.LittleEndian.PutUint64(out[8:16], headerSize)
	binary.LittleEndian.PutUint64(out[16:24], attrEntrySize)
	binary.LittleEndian.PutUint64(out[24:32], headerSize)
	binary.LittleEndian.PutUint64(out[32:40], uint64(attrsSize))
	binary.LittleEndian.PutUint64(out[40:48], uint64(dataOffset))
	binary.LittleEndian.PutUint64(out[48:56], uint64(records.Len()))

	for i, item := range []struct {
		config uint64
		id     uint64
	}{
		{config: 0x1, id: 101},
		{config: 0x2, id: 202},
	} {
		entry := out[headerSize+i*attrEntrySize : headerSize+(i+1)*attrEntrySize]
		binary.LittleEndian.PutUint32(entry[0:4], 0)
		binary.LittleEndian.PutUint32(entry[4:8], attrStructSize)
		binary.LittleEndian.PutUint64(entry[8:16], item.config)
		binary.LittleEndian.PutUint64(entry[24:32], sampleType)
		binary.LittleEndian.PutUint64(entry[attrStructSize:attrStructSize+8], uint64(idsOffset+i*8))
		binary.LittleEndian.PutUint64(entry[attrStructSize+8:attrStructSize+16], 8)
		binary.LittleEndian.PutUint64(out[idsOffset+i*8:idsOffset+(i+1)*8], item.id)
	}

	out = append(out, records.Bytes()...)
	return out
}

func syntheticRawPerfDataWithFeatures() []byte {
	const headerSize = 104
	const attrSize = 48
	sampleType := uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod)
	var records bytes.Buffer
	records.Write(rawPerfRecord(perfRecordComm, rawPerfCommPayload(1234, 5678, "app")))
	records.Write(rawPerfRecord(perfRecordMmap, rawPerfMmapPayload(1234, 5678, 0x1000, 0x1000, 0, "/system/lib64/libfoo.so")))
	records.Write(rawPerfRecord(perfRecordSample, rawPerfSamplePayload(sampleType)))

	dataOffset := headerSize + attrSize
	featureIDs := []int{perfFeatureBuildID, perfFeatureArch, perfFeatureCmdline, perfFeatureMetaInfo}
	descriptorOffset := dataOffset + records.Len()
	sectionOffset := descriptorOffset + len(featureIDs)*16
	sections := [][]byte{
		rawPerfFeatureBuildIDSection(),
		rawPerfFeatureString("arm64"),
		rawPerfFeatureStringList("simpleperf", "record"),
		[]byte("clockid\x00monotonic\x00event_type_info\x00cpu-cycles\x00"),
	}
	out := make([]byte, sectionOffset)
	copy(out[0:8], []byte(perfMagic2))
	binary.LittleEndian.PutUint64(out[8:16], headerSize)
	binary.LittleEndian.PutUint64(out[16:24], attrSize)
	binary.LittleEndian.PutUint64(out[24:32], headerSize)
	binary.LittleEndian.PutUint64(out[32:40], attrSize)
	binary.LittleEndian.PutUint64(out[40:48], uint64(dataOffset))
	binary.LittleEndian.PutUint64(out[48:56], uint64(records.Len()))
	for _, id := range featureIDs {
		out[72+id/8] |= 1 << (id % 8)
	}
	attr := out[headerSize:dataOffset]
	binary.LittleEndian.PutUint32(attr[0:4], 0)
	binary.LittleEndian.PutUint32(attr[4:8], 40)
	binary.LittleEndian.PutUint64(attr[8:16], 0)
	binary.LittleEndian.PutUint64(attr[24:32], sampleType)
	copy(out[dataOffset:descriptorOffset], records.Bytes())

	cur := sectionOffset
	for i, section := range sections {
		desc := out[descriptorOffset+i*16 : descriptorOffset+(i+1)*16]
		binary.LittleEndian.PutUint64(desc[0:8], uint64(cur))
		binary.LittleEndian.PutUint64(desc[8:16], uint64(len(section)))
		out = append(out, section...)
		cur += len(section)
	}
	return out
}

func syntheticRawPerfDataWithCommLifetime() []byte {
	const headerSize = 104
	const attrSize = 48
	sampleType := uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod)
	var records bytes.Buffer
	records.Write(rawPerfRecord(perfRecordComm, rawPerfCommPayload(1234, 5678, "before")))
	records.Write(rawPerfRecord(perfRecordSample, rawPerfSamplePayload(sampleType)))
	records.Write(rawPerfRecord(perfRecordComm, rawPerfCommPayload(1234, 5678, "after")))
	records.Write(rawPerfRecord(perfRecordSample, rawPerfSamplePayload(sampleType)))

	dataOffset := headerSize + attrSize
	out := make([]byte, dataOffset)
	copy(out[0:8], []byte(perfMagic2))
	binary.LittleEndian.PutUint64(out[8:16], headerSize)
	binary.LittleEndian.PutUint64(out[16:24], attrSize)
	binary.LittleEndian.PutUint64(out[24:32], headerSize)
	binary.LittleEndian.PutUint64(out[32:40], attrSize)
	binary.LittleEndian.PutUint64(out[40:48], uint64(dataOffset))
	binary.LittleEndian.PutUint64(out[48:56], uint64(records.Len()))
	attr := out[headerSize:dataOffset]
	binary.LittleEndian.PutUint32(attr[0:4], 0)
	binary.LittleEndian.PutUint32(attr[4:8], 40)
	binary.LittleEndian.PutUint64(attr[8:16], 0)
	binary.LittleEndian.PutUint64(attr[24:32], sampleType)
	out = append(out, records.Bytes()...)
	return out
}

func rawPerfFeatureString(s string) []byte {
	var out bytes.Buffer
	writeRawPerfTestU32(&out, uint32(len(s)+1))
	out.WriteString(s)
	out.WriteByte(0)
	return out.Bytes()
}

func rawPerfFeatureStringList(values ...string) []byte {
	var out bytes.Buffer
	writeRawPerfTestU32(&out, uint32(len(values)))
	for _, value := range values {
		writeRawPerfTestU32(&out, uint32(len(value)+1))
		out.WriteString(value)
		out.WriteByte(0)
	}
	return out.Bytes()
}

func rawPerfFeatureBuildIDSection() []byte {
	var out bytes.Buffer
	filename := "/system/lib64/libfoo.so"
	recordSize := uint16(8 + 4 + 24 + 64)
	writeRawPerfTestU32(&out, 0)
	writeRawPerfTestU16(&out, 0)
	writeRawPerfTestU16(&out, recordSize)
	writeRawPerfTestU32(&out, 1234)
	for i := 1; i <= 20; i++ {
		out.WriteByte(byte(i))
	}
	out.Write(make([]byte, 4))
	out.WriteString(filename)
	out.WriteByte(0)
	for out.Len() < int(recordSize) {
		out.WriteByte(0)
	}
	return out.Bytes()
}

func writeRawPerfTestU32(out *bytes.Buffer, v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	out.Write(buf[:])
}

func writeRawPerfTestU64(out *bytes.Buffer, v uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	out.Write(buf[:])
}

func writeRawPerfTestString(out *bytes.Buffer, value string) {
	writeRawPerfTestU32(out, uint32(len(value)+1))
	out.WriteString(value)
	out.WriteByte(0)
}

func writeRawPerfTestU16(out *bytes.Buffer, v uint16) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	out.Write(buf[:])
}

func rawPerfRecord(typ int, payload []byte) []byte {
	size := 8 + len(payload)
	out := make([]byte, size)
	binary.LittleEndian.PutUint32(out[0:4], uint32(typ))
	binary.LittleEndian.PutUint16(out[4:6], 0)
	binary.LittleEndian.PutUint16(out[6:8], uint16(size))
	copy(out[8:], payload)
	return out
}

func rawPerfCommPayload(pid, tid int, name string) []byte {
	out := make([]byte, 8+len(name)+1)
	binary.LittleEndian.PutUint32(out[0:4], uint32(pid))
	binary.LittleEndian.PutUint32(out[4:8], uint32(tid))
	copy(out[8:], name)
	return out
}

func rawPerfMmapPayload(pid, tid int, addr, length, pgoff uint64, path string) []byte {
	out := make([]byte, 32+len(path)+1)
	binary.LittleEndian.PutUint32(out[0:4], uint32(pid))
	binary.LittleEndian.PutUint32(out[4:8], uint32(tid))
	binary.LittleEndian.PutUint64(out[8:16], addr)
	binary.LittleEndian.PutUint64(out[16:24], length)
	binary.LittleEndian.PutUint64(out[24:32], pgoff)
	copy(out[32:], path)
	return out
}

func rawPerfSamplePayload(sampleType uint64) []byte {
	var out bytes.Buffer
	writeU64 := func(v uint64) {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], v)
		out.Write(buf[:])
	}
	writeU32Pair := func(a, b uint32) {
		var buf [8]byte
		binary.LittleEndian.PutUint32(buf[0:4], a)
		binary.LittleEndian.PutUint32(buf[4:8], b)
		out.Write(buf[:])
	}
	if sampleType&perfSampleIdentifier != 0 {
		writeU64(202)
	}
	if sampleType&perfSampleIP != 0 {
		writeU64(0x1234)
	}
	if sampleType&perfSampleTID != 0 {
		writeU32Pair(1234, 5678)
	}
	if sampleType&perfSampleTime != 0 {
		writeU64(1_234_567_000)
	}
	if sampleType&perfSampleID != 0 {
		writeU64(202)
	}
	if sampleType&perfSampleStreamID != 0 {
		writeU64(202)
	}
	if sampleType&perfSampleCPU != 0 {
		writeU32Pair(5, 0)
	}
	if sampleType&perfSamplePeriod != 0 {
		writeU64(99)
	}
	if sampleType&perfSampleCallchain != 0 {
		writeU64(2)
		writeU64(0x1111)
		writeU64(0x1222)
	}
	if sampleType&perfSampleRaw != 0 {
		var size [4]byte
		binary.LittleEndian.PutUint32(size[:], 3)
		out.Write(size[:])
		out.Write([]byte{0xaa, 0xbb, 0xcc})
		for out.Len()%8 != 0 {
			out.WriteByte(0)
		}
	}
	if sampleType&perfSampleRegsUser != 0 {
		writeU64(0)
	}
	if sampleType&perfSampleStackUser != 0 {
		writeU64(3)
		out.Write([]byte{0xaa, 0xbb, 0xcc})
		writeU64(3)
	}
	if sampleType&perfSampleWeight != 0 {
		writeU64(123)
	}
	return out.Bytes()
}
