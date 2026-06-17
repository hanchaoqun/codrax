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
		"period=99",
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

func TestConvertFileUsesRawPerfParserForDirectPerfData(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf.data")
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
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			t.Fatalf("official-only mode without official adapter should not emit raw perftrace: %+v", result.Artifacts)
		}
	}
}

func syntheticRawPerfData() []byte {
	const headerSize = 104
	const attrSize = 48
	sampleType := uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod | perfSampleCallchain)
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

	out = append(out, records.Bytes()...)
	return out
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
	if sampleType&perfSampleIP != 0 {
		writeU64(0x1234)
	}
	if sampleType&perfSampleTID != 0 {
		writeU32Pair(1234, 5678)
	}
	if sampleType&perfSampleTime != 0 {
		writeU64(1_234_567_000)
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
	return out.Bytes()
}
