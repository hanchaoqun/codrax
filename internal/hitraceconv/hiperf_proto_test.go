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

func TestConvertHiperfProtoFileToPerfTraceRoundTripsThroughTraceQuery(t *testing.T) {
	dir := t.TempDir()
	protoPath := filepath.Join(dir, "perf.proto")
	outPath := filepath.Join(dir, "perf.perftrace")
	if err := os.WriteFile(protoPath, syntheticHiperfProtoStream(), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ConvertHiperfProtoFileToPerfTrace(context.Background(), protoPath, outPath); err != nil {
		t.Fatalf("convert hiperf proto: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"perf_sample:",
		"cpu=-1",
		"pid=1234",
		"tid=5678",
		"sample_weight=99",
		`event="cpu-cycles"`,
		`symbol="doWork"`,
		`dso="/system/lib64/libfoo.so"`,
		`callchain="main@/system/lib64/libfoo.so;doWork@/system/lib64/libfoo.so"`,
		"source=hiperf_proto",
		"cpu_known=false",
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
	if ev.Type != tracequery.EventPerfSample || ev.CPU != -1 || ev.PerfPID != 1234 || ev.PerfTID != 5678 || ev.PerfPeriod != 99 {
		t.Fatalf("bad perf sample fields: %+v", ev)
	}
	if ev.PerfEvent != "cpu-cycles" || ev.PerfSymbol != "doWork" || ev.PerfDSO != "/system/lib64/libfoo.so" {
		t.Fatalf("bad perf symbol fields: %+v", ev)
	}
	if ev.PerfCPUKnown == nil || *ev.PerfCPUKnown || ev.PerfSymbolizationStatus != "symbolized" || ev.PerfClockConfidence != "assumed" || ev.PerfCallchainStatus != "symbolized" {
		t.Fatalf("bad hiperf perf quality fields: %+v", ev)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 0, TimeEnd: 999})
	if stats.PerfSamples == nil || stats.PerfSamples.Quality == nil || stats.PerfSamples.Quality.CPUUnknownCount != 1 {
		t.Fatalf("hiperf CPU-unknown quality should reach aggregate summary: %+v", stats.PerfSamples)
	}
}

func TestConvertFileRunsConfiguredHiperfAdapter(t *testing.T) {
	dir := t.TempDir()
	protoPath := filepath.Join(dir, "fixture.proto")
	if err := os.WriteFile(protoPath, syntheticHiperfProtoStream(), 0o644); err != nil {
		t.Fatal(err)
	}
	toolPath := filepath.Join(dir, "hiperf_host")
	script := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    out="$1"
  fi
  shift
done
cp "$HIPERF_PROTO_FIXTURE" "$out"
`
	if err := os.WriteFile(toolPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HIPERF_PROTO_FIXTURE", protoPath)

	input := filepath.Join(dir, "sample-with-perf.htrace")
	body := append(syntheticBinaryHitrace(t), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", []byte("PERF-DATA"))...)
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, HiperfPath: toolPath})
	if err != nil {
		t.Fatalf("convert file: %v", err)
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
	if len(result.ProviderDecisions) != 1 {
		t.Fatalf("expected one hiperf provider decision: %+v", result.ProviderDecisions)
	}
	decision := result.ProviderDecisions[0]
	if decision.ProviderName != perfProviderNameHiperfProto || !decision.Selected || !decision.Attempted || !decision.Succeeded || !decision.TraceQueryReady {
		t.Fatalf("bad hiperf provider decision: %+v", decision)
	}
	idx, err := tracequery.BuildIndex(context.Background(), perfTrace.Path)
	if err != nil {
		t.Fatalf("parse generated perftrace: %v", err)
	}
	found := false
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventPerfSample && ev.PerfSymbol == "doWork" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("generated perftrace did not round-trip: %+v", idx.Events)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bundle), `"type": "perftrace"`) || !strings.Contains(string(bundle), perfTrace.Path) || !strings.Contains(string(bundle), `"provider_decisions"`) || !strings.Contains(string(bundle), `"provider_name": "openharmony_hiperf_report_proto"`) {
		t.Fatalf("bundle missing perftrace artifact:\n%s", string(bundle))
	}
}

func syntheticHiperfProtoStream() []byte {
	var b bytes.Buffer
	b.WriteString(hiperfProtoMagic)
	var version [2]byte
	binary.LittleEndian.PutUint16(version[:], hiperfProtoVersion)
	b.Write(version[:])
	writeHiperfRecord(&b, protoMessage(5,
		protoBytes(1, []byte("cpu-cycles")),
	))
	writeHiperfRecord(&b, protoMessage(3,
		protoVarint(1, 0),
		protoBytes(2, []byte("/system/lib64/libfoo.so")),
		protoBytes(3, []byte("main")),
		protoBytes(3, []byte("doWork")),
	))
	writeHiperfRecord(&b, protoMessage(4,
		protoVarint(1, 5678),
		protoVarint(2, 1234),
		protoBytes(3, []byte("Render Thread")),
	))
	leaf := protoMessage(3,
		protoVarint(1, 0x1234),
		protoVarint(2, 0),
		protoVarint(3, 1),
		protoVarint(4, 0x1000),
	)
	root := protoMessage(3,
		protoVarint(1, 0x1111),
		protoVarint(2, 0),
		protoVarint(3, 0),
		protoVarint(4, 0x1000),
	)
	writeHiperfRecord(&b, protoMessage(1,
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

func writeHiperfRecord(b *bytes.Buffer, record []byte) {
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(record)))
	b.Write(size[:])
	b.Write(record)
}

func protoMessage(field int, parts ...[]byte) []byte {
	var payload bytes.Buffer
	for _, part := range parts {
		payload.Write(part)
	}
	return protoBytes(field, payload.Bytes())
}

func protoBytes(field int, payload []byte) []byte {
	var b bytes.Buffer
	writeProtoVarint(&b, uint64(field<<3|2))
	writeProtoVarint(&b, uint64(len(payload)))
	b.Write(payload)
	return b.Bytes()
}

func protoVarint(field int, value uint64) []byte {
	var b bytes.Buffer
	writeProtoVarint(&b, uint64(field<<3))
	writeProtoVarint(&b, value)
	return b.Bytes()
}

func writeProtoVarint(b *bytes.Buffer, value uint64) {
	for value >= 0x80 {
		b.WriteByte(byte(value) | 0x80)
		value >>= 7
	}
	b.WriteByte(byte(value))
}
