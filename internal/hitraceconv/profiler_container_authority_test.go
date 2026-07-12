package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfilerContainerHeaderLengthIsAuthoritativeOverTrailingFrames(t *testing.T) {
	line := "worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|outside"
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(line)))
	binary.LittleEndian.PutUint64(body[8:16], profilerTraceHeaderSize)
	dir := t.TempDir()
	input := filepath.Join(dir, "declared-empty.htrace")
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert declared-empty profiler file: %v", err)
	}
	if result.EventsWritten != 0 || result.OutputPath != "" || result.UnknownEventCount != 0 {
		t.Fatalf("bytes beyond authoritative header length must not be parsed as plugin frames: %+v", result)
	}
}

func TestProfilerContainerOversizedUint64LengthRetainsTypedProvenance(t *testing.T) {
	for _, declared := range []uint64{uint64(1) << 63, ^uint64(0)} {
		t.Run(fmt.Sprintf("length_%d", declared), func(t *testing.T) {
			body := make([]byte, profilerTraceHeaderSize)
			binary.LittleEndian.PutUint64(body[0:8], profilerTraceHeaderMagic)
			binary.LittleEndian.PutUint64(body[8:16], declared)
			binary.LittleEndian.PutUint32(body[56:60], profilerDataTypeProtobuf)
			header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
			if !ok || header.Length != declared {
				t.Fatalf("official magic with uint64 length overflow must remain detected: ok=%t header=%+v", ok, header)
			}
			dir := t.TempDir()
			input := filepath.Join(dir, "overflow-length.htrace")
			if err := os.WriteFile(input, body, 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(context.Background(), Options{
				InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin,
			})
			if err != nil {
				t.Fatalf("convert overflow-length profiler header: %v", err)
			}
			if result.UnknownEventCount != 1 || !coverageTableHasSkipped(result.TraceCoverage, "__container_envelope__", "trace_file_declared_length_truncated") {
				t.Fatalf("uint64 length overflow must not fall through format detection: result=%+v coverage=%+v", result, result.TraceCoverage)
			}
		})
	}
}

func TestProfilerContainerUndersizedLengthRetainsTypedProvenance(t *testing.T) {
	for _, declared := range []uint64{0, 1, profilerTraceHeaderSize - 1} {
		t.Run(fmt.Sprintf("length_%d", declared), func(t *testing.T) {
			body := make([]byte, profilerTraceHeaderSize)
			binary.LittleEndian.PutUint64(body[0:8], profilerTraceHeaderMagic)
			binary.LittleEndian.PutUint64(body[8:16], declared)
			binary.LittleEndian.PutUint32(body[56:60], profilerDataTypeProtobuf)
			header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
			if !ok || header.Length != declared {
				t.Fatalf("official magic with undersized length must remain detected: ok=%t header=%+v", ok, header)
			}
			dir := t.TempDir()
			input := filepath.Join(dir, "undersized-length.htrace")
			if err := os.WriteFile(input, body, 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(context.Background(), Options{
				InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin,
			})
			if err != nil {
				t.Fatalf("convert undersized-length profiler header: %v", err)
			}
			if result.UnknownEventCount != 1 || !coverageTableHasSkipped(result.TraceCoverage, "__container_envelope__", "trace_file_declared_length_invalid") {
				t.Fatalf("undersized length must not fall through format detection: result=%+v coverage=%+v", result, result.TraceCoverage)
			}
		})
	}
}

func TestProfilerContainerTruncatedLengthPrefixIsCoverageOnly(t *testing.T) {
	line := "worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|good"
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(line)))
	body = append(body, 0x01, 0x02)
	binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)))
	dir := t.TempDir()
	input := filepath.Join(dir, "truncated-prefix.htrace")
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert truncated prefix: %v", err)
	}
	if result.EventsWritten != 1 || result.UnknownEventCount != 1 ||
		!coverageTableHasSkipped(result.TraceCoverage, "__container_envelope__", "plugin_length_prefix_truncated") {
		t.Fatalf("partial length prefix must be local typed coverage after the valid sibling: result=%+v coverage=%+v", result, result.TraceCoverage)
	}
}

func TestProfilerContainerNestedMetadataDamageCountsAsUnsupported(t *testing.T) {
	payload := protoBytes(1, []byte{0x08, 0x80})
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", payload),
	)
	defer sink.cleanup()
	if extracted.UnsupportedFtrace != 1 || extracted.StructuredFtrace != 1 || sink.stats.RowsAccepted != 0 {
		t.Fatalf("nested metadata damage must be explicit unsupported input: extracted=%+v sink=%+v", extracted, sink.stats)
	}
	if !coverageTableHasSkipped(extracted.TraceCoverage, "__trace_plugin_metadata__", "ftrace_cpu_stats_malformed_wire") {
		t.Fatalf("nested metadata damage missing typed coverage: %+v", extracted.TraceCoverage)
	}
}

func TestProfilerContainerStructuredPayloadCannotMintEmbeddedTextRow(t *testing.T) {
	injected := "bad\nworker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|injected"
	payload := protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(5_000_000_000, 7, 7, injected, 2420, protoPayload(
			protoBytes(1, []byte("target")),
			protoVarint(2, 8),
			protoVarint(3, 120),
			protoVarint(5, 1),
		)),
	)
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", payload),
	)
	defer sink.cleanup()

	if extracted.StructuredFtrace != 1 || extracted.TextPluginMessages != 0 || extracted.TextRows != 0 || extracted.StructuredRows != 0 {
		t.Fatalf("structured payload must stay on typed lane: %+v", extracted)
	}
	if sink.stats.RowsAccepted != 0 {
		t.Fatalf("invalid structured comm must not mint its embedded text row: %+v", sink.stats)
	}
	if !coverageTableHasSkipped(extracted.TraceCoverage, "sched_wakeup", "envelope_comm_invalid") {
		t.Fatalf("structured comm rejection should remain event-envelope coverage: %+v", extracted.TraceCoverage)
	}
}

func TestProfilerContainerRejectedMessageDoesNotStarveValidSibling(t *testing.T) {
	bad := append(protoBytes(1, []byte("ftrace-plugin")), 0x80)
	goodLine := "worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|good"
	dir := t.TempDir()
	input := filepath.Join(dir, "siblings.htrace")
	if err := os.WriteFile(input, syntheticProfilerTraceFile(
		bad,
		syntheticProfilerPluginData("bytrace_plugin", []byte(goodLine)),
	), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert siblings: %v", err)
	}
	if result.EventsWritten != 1 || result.UnknownEventCount != 1 {
		t.Fatalf("bad plugin must be local coverage while good sibling survives: %+v", result)
	}
	body, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "B|7|good") {
		t.Fatalf("valid sibling row missing:\n%s", body)
	}
	if !coverageTableHasSkipped(result.TraceCoverage, "plugin:__rejected__", "plugin_message_malformed_wire") {
		t.Fatalf("rejected plugin message missing typed coverage: %+v", result.TraceCoverage)
	}
}

func TestProfilerContainerZeroLengthFrameDoesNotStarveValidSibling(t *testing.T) {
	goodLine := "worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|good"
	dir := t.TempDir()
	input := filepath.Join(dir, "zero-frame.htrace")
	if err := os.WriteFile(input, syntheticProfilerTraceFile(
		nil,
		syntheticProfilerPluginData("bytrace_plugin", []byte(goodLine)),
	), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert zero-length sibling: %v", err)
	}
	if result.EventsWritten != 1 || result.UnknownEventCount != 1 ||
		!coverageTableHasSkipped(result.TraceCoverage, "plugin:__rejected__", "plugin_frame_zero_length") {
		t.Fatalf("zero frame should be recoverable local coverage: %+v coverage=%+v", result, result.TraceCoverage)
	}
}

func TestProfilerContainerMalformedStructuredTailHasNoPartialOrTextRescue(t *testing.T) {
	line := "worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|must-not-leak"
	validPrefix := protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(5_000_000_000, 7, 7, "worker", 1109, protoBytes(1, []byte(line))),
	)
	payload := append(validPrefix, 0x12, 0x80)
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", payload),
	)
	defer sink.cleanup()

	if extracted.MalformedFtrace != 1 || extracted.StructuredFtrace != 0 || extracted.TextRows != 0 || extracted.StructuredRows != 0 || sink.stats.RowsAccepted != 0 {
		t.Fatalf("malformed structured tail must clear partial rows and prohibit text rescue: extracted=%+v sink=%+v", extracted, sink.stats)
	}
	if !coverageTableHasSkipped(extracted.TraceCoverage, "__trace_plugin_envelope__", "envelope_trace_plugin_malformed_wire") {
		t.Fatalf("malformed top-level payload must use trace-plugin envelope coverage: %+v", extracted.TraceCoverage)
	}
	if strings.Contains(strings.Join(extracted.Caveats, "\n"), "structured metadata: stats_messages=0") {
		t.Fatalf("malformed envelope must not publish unknown metadata as deterministic zero: %v", extracted.Caveats)
	}
}

func TestProfilerContainerExactLegacyTextAndStructuredProvenanceStayDistinct(t *testing.T) {
	legacy := strings.Join([]string{
		"# tracer: nop",
		"worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|legacy",
	}, "\r\n")
	legacyExtracted, legacySink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", []byte(legacy)),
	)
	defer legacySink.cleanup()
	if legacyExtracted.TextRows != 1 || legacyExtracted.TextPluginMessages != 1 || legacyExtracted.StructuredRows != 0 {
		t.Fatalf("complete exact legacy payload should use strict text compatibility lane: %+v", legacyExtracted)
	}

	structured := protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(5_000_000_000, 7, 7, "worker", 1109, protoBytes(1, []byte("B|7|typed"))),
	)
	structuredExtracted, structuredSink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", structured),
	)
	defer structuredSink.cleanup()
	if structuredExtracted.StructuredRows != 1 || structuredExtracted.TextRows != 0 || structuredExtracted.TextPluginMessages != 0 {
		t.Fatalf("structured rows must not be reported as text: %+v", structuredExtracted)
	}
	joined := strings.Join(structuredExtracted.Caveats, "\n")
	if strings.Contains(joined, "text rows from 0") || !strings.Contains(joined, "rendered 1 structured trace row") {
		t.Fatalf("structured provenance wording drifted: %s", joined)
	}
}

func TestProfilerContainerMalformedProbeCannotUseCompleteTextOverlap(t *testing.T) {
	// '*' is the complete protobuf key for official top-level field 5/wire 2.
	// The following 'w' is an impossible length for this payload, so the same
	// bytes are attributable to a malformed TracePluginResult even though they
	// also satisfy the generic ftrace text grammar.
	overlap := "*worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|overlap"
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", []byte(overlap)),
	)
	defer sink.cleanup()
	if extracted.MalformedFtrace != 1 || extracted.StructuredFtrace != 0 || extracted.TextRows != 0 || extracted.TextPluginMessages != 0 || sink.stats.RowsAccepted != 0 {
		t.Fatalf("attributable malformed protobuf must never be rescued by an overlapping text grammar: extracted=%+v sink=%+v", extracted, sink.stats)
	}
	if !coverageTableHasSkipped(extracted.TraceCoverage, "__trace_plugin_envelope__", "envelope_trace_plugin_malformed_wire") {
		t.Fatalf("overlap must remain top-level typed coverage: %+v", extracted.TraceCoverage)
	}
}

func TestProfilerContainerNonCanonicalFtracePluginNameCannotEnterStructuredLane(t *testing.T) {
	payload := protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(5_000_000_000, 7, 7, "worker", 1109, protoBytes(1, []byte("B|7|typed"))),
	)
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("FTRACE-PLUGIN", payload),
	)
	defer sink.cleanup()
	if extracted.StructuredFtrace != 0 || extracted.TextRows != 0 || sink.stats.RowsAccepted != 0 || extracted.UnsupportedFtrace != 1 {
		t.Fatalf("structured hard route must require the exact official plugin name: extracted=%+v sink=%+v", extracted, sink.stats)
	}
}

func extractSyntheticProfilerContainer(t *testing.T, messages ...[]byte) (profilerContainerExtraction, *traceDBRowSink) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "input.htrace")
	if err := os.WriteFile(input, syntheticProfilerTraceFile(messages...), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink("", 128)
	if err != nil {
		t.Fatal(err)
	}
	extracted, err := extractProfilerContainerSystraceRows(context.Background(), input, info.Size(), sink)
	if err != nil {
		sink.cleanup()
		t.Fatalf("extract profiler container: %v", err)
	}
	return extracted, sink
}

func coverageTableHasSkipped(coverage []TraceDBCoverage, table, reason string) bool {
	for _, item := range coverage {
		if item.Table == table && strings.Contains(item.Skipped, reason) {
			return true
		}
	}
	return false
}
